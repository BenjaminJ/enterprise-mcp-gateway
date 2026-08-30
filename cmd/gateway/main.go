package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/goschan/enterprise-mcp-gateway/pkg/audit"
	"github.com/goschan/enterprise-mcp-gateway/pkg/config"
	"github.com/goschan/enterprise-mcp-gateway/pkg/connector/mock"
	"github.com/goschan/enterprise-mcp-gateway/pkg/connector/openapi"
	"github.com/goschan/enterprise-mcp-gateway/pkg/governance/rbac"
	"github.com/goschan/enterprise-mcp-gateway/pkg/mcp/protocol"
	"github.com/goschan/enterprise-mcp-gateway/pkg/mcp/transport"
)

func main() {
	configPath := flag.String("config", "config.yaml", "Path to gateway configuration YAML file")
	transportFlag := flag.String("transport", "", "Transport mode: 'stdio' or 'sse'")
	portFlag := flag.Int("port", 0, "HTTP server port (for SSE transport)")
	hostFlag := flag.String("host", "", "HTTP server host")
	clientTokenFlag := flag.String("token", "", "Default client token for stdio mode")
	flag.Parse()

	// 1. Load Configuration
	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		log.Printf("[WARN] Failed to load config file '%s': %v (falling back to defaults)", *configPath, err)
		cfg = config.DefaultConfig()
	}

	if *transportFlag != "" {
		cfg.Server.Transport = *transportFlag
	}
	if *portFlag != 0 {
		cfg.Server.Port = *portFlag
	}
	if *hostFlag != "" {
		cfg.Server.Host = *hostFlag
	}

	// 2. Initialize Subsystems
	auditLogger, err := cfg.ToAuditLogger()
	if err != nil {
		log.Fatalf("Failed to initialize audit logger: %v", err)
	}
	defer auditLogger.Close()

	sanitizer, err := cfg.ToPIISanitizer()
	if err != nil {
		log.Fatalf("Failed to initialize PII sanitizer: %v", err)
	}

	rbacMgr := cfg.ToRBACManager()

	// 3. Initialize MCP Protocol Server
	mcpServer := protocol.NewServer(protocol.Implementation{
		Name:    cfg.Server.Name,
		Version: cfg.Server.Version,
	})

	// 4. Load Connectors & Register Tools
	invokers := make([]*openapi.Invoker, 0)
	toolToInvoker := make(map[string]*openapi.Invoker)

	for _, conn := range cfg.Connectors {
		if strings.Contains(conn.BaseURL, ":8081") {
			mock.StartAutoMockServer(8081)
		}
		if conn.Type == "openapi" && conn.SpecFile != "" {
			tools, endpoints, err := openapi.ParseSpecFile(conn.SpecFile)
			if err != nil {
				log.Printf("[ERROR] Failed to load OpenAPI connector '%s' from %s: %v", conn.Name, conn.SpecFile, err)
				continue
			}

			timeout := time.Duration(conn.TimeoutSeconds) * time.Second
			inv := openapi.NewInvoker(conn.BaseURL, conn.Headers, endpoints, timeout)
			invokers = append(invokers, inv)

			for _, t := range tools {
				mcpServer.RegisterTool(t)
				toolToInvoker[t.Name] = inv
			}
			log.Printf("[INFO] Registered %d tools from connector '%s'", len(tools), conn.Name)
		}
	}

	// 5. Configure RBAC Tool Filter
	mcpServer.SetToolFilter(func(ctx context.Context, tools []protocol.Tool) []protocol.Tool {
		identity, ok := rbac.GetCallerIdentity(ctx)
		role := cfg.Governance.DefaultRole
		if ok {
			role = identity.Role
		}
		return rbacMgr.FilterTools(role, tools)
	})

	// 6. Configure Tool Execution Pipeline
	mcpServer.SetToolHandler(func(ctx context.Context, name string, args map[string]interface{}) (*protocol.CallToolResult, error) {
		start := time.Now()
		identity, ok := rbac.GetCallerIdentity(ctx)
		if !ok {
			identity = rbac.CallerIdentity{
				CallerID: "stdio-client",
				Role:     rbacMgr.ResolveRole(*clientTokenFlag),
			}
			ctx = rbac.WithCallerIdentity(ctx, identity)
		}

		inputHash := auditLogger.HashInput(args)

		// Authorize via RBAC
		if err := rbacMgr.Authorize(ctx, name); err != nil {
			_ = auditLogger.Log(ctx, audit.AuditEvent{
				CallerID:    identity.CallerID,
				Role:        identity.Role,
				Tool:        name,
				InputSHA256: inputHash,
				DurationMs:  time.Since(start).Milliseconds(),
				Status:      audit.StatusUnauthorized,
				Error:       err.Error(),
			})
			return nil, fmt.Errorf("RBAC permission denied: %w", err)
		}

		inv, exists := toolToInvoker[name]
		if !exists {
			return nil, fmt.Errorf("no backend invoker found for tool '%s'", name)
		}

		// Execute upstream request
		result, err := inv.Invoke(ctx, name, args)
		duration := time.Since(start).Milliseconds()

		if err != nil {
			_ = auditLogger.Log(ctx, audit.AuditEvent{
				CallerID:    identity.CallerID,
				Role:        identity.Role,
				Tool:        name,
				InputSHA256: inputHash,
				DurationMs:  duration,
				Status:      audit.StatusError,
				Error:       err.Error(),
			})
			return nil, err
		}

		// Sanitize PII / secrets from tool output
		redactedCount := sanitizer.SanitizeResult(result)

		_ = auditLogger.Log(ctx, audit.AuditEvent{
			CallerID:         identity.CallerID,
			Role:             identity.Role,
			Tool:             name,
			InputSHA256:      inputHash,
			DurationMs:       duration,
			Status:           audit.StatusSuccess,
			PIIRedactedCount: redactedCount,
		})

		return result, nil
	})

	// 7. Start Transport
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if cfg.Server.Transport == "sse" {
		sseTransport := transport.NewSSETransport(mcpServer)
		mux := http.NewServeMux()

		// Wrap with token extraction middleware
		authMiddleware := func(next http.HandlerFunc) http.HandlerFunc {
			return func(w http.ResponseWriter, r *http.Request) {
				authHeader := r.Header.Get("Authorization")
				token := strings.TrimPrefix(authHeader, "Bearer ")
				if token == "" {
					token = r.URL.Query().Get("token")
				}
				role := rbacMgr.ResolveRole(token)
				callerID := r.Header.Get("X-Caller-ID")
				if callerID == "" {
					callerID = "sse-client"
				}

				reqCtx := rbac.WithCallerIdentity(r.Context(), rbac.CallerIdentity{
					Token:    token,
					CallerID: callerID,
					Role:     role,
				})
				next(w, r.WithContext(reqCtx))
			}
		}

		mux.HandleFunc("/sse", authMiddleware(sseTransport.HandleSSE))
		mux.HandleFunc("/message", authMiddleware(sseTransport.HandleMessage))
		mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"ok","version":"` + cfg.Server.Version + `"}`))
		})

		addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
		httpServer := &http.Server{
			Addr:    addr,
			Handler: mux,
		}

		go func() {
			<-ctx.Done()
			shutdownCtx, sCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer sCancel()
			_ = httpServer.Shutdown(shutdownCtx)
		}()

		log.Printf("[INFO] Enterprise MCP Gateway listening on HTTP SSE http://%s", addr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server error: %v", err)
		}
	} else {
		// Stdio Transport
		// Inject default caller identity into context for stdio
		stdioCtx := rbac.WithCallerIdentity(ctx, rbac.CallerIdentity{
			Token:    *clientTokenFlag,
			CallerID: "stdio-client",
			Role:     rbacMgr.ResolveRole(*clientTokenFlag),
		})

		stdioTransport := transport.NewStdioTransport(os.Stdin, os.Stdout, mcpServer)
		if err := stdioTransport.Start(stdioCtx); err != nil && err != context.Canceled {
			log.Fatalf("Stdio transport error: %v", err)
		}
	}
}
