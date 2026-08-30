package integration

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/goschan/enterprise-mcp-gateway/pkg/audit"
	"github.com/goschan/enterprise-mcp-gateway/pkg/connector/openapi"
	"github.com/goschan/enterprise-mcp-gateway/pkg/governance/rbac"
	"github.com/goschan/enterprise-mcp-gateway/pkg/mcp/protocol"
	"github.com/goschan/enterprise-mcp-gateway/pkg/mcp/transport"
	"github.com/goschan/enterprise-mcp-gateway/pkg/sanitizer/pii"
)

// setupMockEnterpriseBackend launches a local HTTP server mimicking CRM microservice
func setupMockEnterpriseBackend() *httptest.Server {
	mux := http.NewServeMux()

	mux.HandleFunc("/v1/customers", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`[{"id":"cust-001","name":"Alice Smith","email":"alice@enterprise.com"}]`))
			return
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"cust-new","status":"created"}`))
	})

	mux.HandleFunc("/v1/customers/cust-001", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Returns real SSN and Credit Card
		_, _ = w.Write([]byte(`{
			"id": "cust-001",
			"name": "Alice Smith",
			"email": "alice@enterprise.com",
			"ssn": "123-45-6789",
			"creditCard": "4111-1111-1111-1111",
			"status": "active"
		}`))
	})

	mux.HandleFunc("/v1/admin/reset-password", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","message":"password reset"}`))
	})

	return httptest.NewServer(mux)
}

func TestGatewayFullE2EPipeline(t *testing.T) {
	mockBackend := setupMockEnterpriseBackend()
	defer mockBackend.Close()

	// 1. Audit logger
	auditBuf := &bytes.Buffer{}
	auditLogger := audit.NewLogger(auditBuf, true, true)

	// 2. PII Sanitizer
	sanitizer := pii.NewSanitizer(pii.DefaultConfig())

	// 3. RBAC Manager
	roles := map[string][]string{
		"readonly_agent": {"list*", "get*"},
		"admin":          {"*"},
	}
	tokens := map[string]string{
		"token-ro":    "readonly_agent",
		"token-admin": "admin",
	}
	rbacMgr := rbac.NewManager(roles, tokens, "readonly_agent", true)

	// 4. OpenAPI Parser & Invoker
	tools, endpoints, err := openapi.ParseSpecFile("../../examples/crm-openapi.yaml")
	if err != nil {
		t.Fatalf("failed parsing OpenAPI spec: %v", err)
	}

	inv := openapi.NewInvoker(mockBackend.URL, nil, endpoints, 5*time.Second)

	// 5. MCP Protocol Server
	mcpServer := protocol.NewServer(protocol.Implementation{
		Name:    "enterprise-mcp-gateway-test",
		Version: "1.0.0",
	})
	mcpServer.RegisterTools(tools)

	// Tool filter for RBAC
	mcpServer.SetToolFilter(func(ctx context.Context, tools []protocol.Tool) []protocol.Tool {
		identity, ok := rbac.GetCallerIdentity(ctx)
		role := "readonly_agent"
		if ok {
			role = identity.Role
		}
		return rbacMgr.FilterTools(role, tools)
	})

	// Tool handler pipeline
	mcpServer.SetToolHandler(func(ctx context.Context, name string, args map[string]interface{}) (*protocol.CallToolResult, error) {
		start := time.Now()
		identity, ok := rbac.GetCallerIdentity(ctx)
		if !ok {
			identity = rbac.CallerIdentity{CallerID: "test-client", Role: "readonly_agent"}
			ctx = rbac.WithCallerIdentity(ctx, identity)
		}

		inputHash := auditLogger.HashInput(args)

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
			return nil, err
		}

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

	// -------------------------------------------------------------
	// Test 1: Initialize handshake
	// -------------------------------------------------------------
	initReq := []byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"claude","version":"1.0"}}}`)
	initRespRaw, err := mcpServer.HandleMessage(context.Background(), initReq)
	if err != nil {
		t.Fatalf("unexpected init error: %v", err)
	}

	var initResp protocol.JSONRPCResponse
	_ = json.Unmarshal(initRespRaw, &initResp)
	if initResp.Error != nil {
		t.Fatalf("init returned error: %v", initResp.Error)
	}

	// -------------------------------------------------------------
	// Test 2: RBAC filtered tools/list for readonly_agent
	// -------------------------------------------------------------
	roCtx := rbac.WithCallerIdentity(context.Background(), rbac.CallerIdentity{
		Role:     "readonly_agent",
		CallerID: "agent-ro",
	})
	listReq := []byte(`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
	listRespRaw, err := mcpServer.HandleMessage(roCtx, listReq)
	if err != nil {
		t.Fatalf("failed tools/list: %v", err)
	}

	var listResp protocol.JSONRPCResponse
	_ = json.Unmarshal(listRespRaw, &listResp)
	listBytes, _ := json.Marshal(listResp.Result)
	var toolsResult protocol.ListToolsResult
	_ = json.Unmarshal(listBytes, &toolsResult)

	for _, tool := range toolsResult.Tools {
		if tool.Name == "adminResetPassword" || tool.Name == "createCustomer" {
			t.Errorf("unauthorized tool '%s' was advertised to readonly_agent", tool.Name)
		}
	}

	// -------------------------------------------------------------
	// Test 3: Tool execution & PII Masking
	// -------------------------------------------------------------
	callReq := []byte(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"getCustomerDetails","arguments":{"customerId":"cust-001"}}}`)
	callRespRaw, err := mcpServer.HandleMessage(roCtx, callReq)
	if err != nil {
		t.Fatalf("failed tools/call: %v", err)
	}

	var callResp protocol.JSONRPCResponse
	_ = json.Unmarshal(callRespRaw, &callResp)
	callBytes, _ := json.Marshal(callResp.Result)
	var callResult protocol.CallToolResult
	_ = json.Unmarshal(callBytes, &callResult)

	if len(callResult.Content) == 0 {
		t.Fatalf("expected tool content, got empty")
	}

	responseText := callResult.Content[0].Text

	// Verify SSN and Credit Card are redacted
	if strings.Contains(responseText, "123-45-6789") {
		t.Errorf("SSN was NOT redacted! Output: %s", responseText)
	}
	if strings.Contains(responseText, "4111-1111-1111-1111") {
		t.Errorf("Credit card was NOT redacted! Output: %s", responseText)
	}
	if !strings.Contains(responseText, "[REDACTED-SSN]") && !strings.Contains(responseText, "[REDACTED]") {
		t.Errorf("expected redacted SSN indicator in %s", responseText)
	}
	if !strings.Contains(responseText, "[REDACTED-CARD]") && !strings.Contains(responseText, "[REDACTED]") {
		t.Errorf("expected redacted Card indicator in %s", responseText)
	}

	// -------------------------------------------------------------
	// Test 4: RBAC Unauthorized Denial
	// -------------------------------------------------------------
	adminCallReq := []byte(`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"adminResetPassword","arguments":{"username":"admin1"}}}`)
	adminDenyRespRaw, err := mcpServer.HandleMessage(roCtx, adminCallReq)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var adminDenyResp protocol.JSONRPCResponse
	_ = json.Unmarshal(adminDenyRespRaw, &adminDenyResp)
	denyBytes, _ := json.Marshal(adminDenyResp.Result)
	var denyResult protocol.CallToolResult
	_ = json.Unmarshal(denyBytes, &denyResult)

	if !denyResult.IsError {
		t.Errorf("expected IsError=true when calling admin tool as readonly_agent")
	}

	// -------------------------------------------------------------
	// Test 5: Verify Audit Log Events
	// -------------------------------------------------------------
	auditLogs := strings.TrimSpace(auditBuf.String())
	if auditLogs == "" {
		t.Fatalf("audit logger produced no logs")
	}
	if !strings.Contains(auditLogs, "getCustomerDetails") {
		t.Errorf("audit log missing getCustomerDetails entry")
	}
	if !strings.Contains(auditLogs, "SUCCESS") {
		t.Errorf("audit log missing SUCCESS entry")
	}
}

func TestGatewaySSETransportE2E(t *testing.T) {
	mockBackend := setupMockEnterpriseBackend()
	defer mockBackend.Close()

	tools, endpoints, _ := openapi.ParseSpecFile("../../examples/crm-openapi.yaml")
	inv := openapi.NewInvoker(mockBackend.URL, nil, endpoints, 5*time.Second)

	mcpServer := protocol.NewServer(protocol.Implementation{Name: "sse-gateway", Version: "1.0"})
	mcpServer.RegisterTools(tools)
	mcpServer.SetToolHandler(func(ctx context.Context, name string, args map[string]interface{}) (*protocol.CallToolResult, error) {
		return inv.Invoke(ctx, name, args)
	})

	sseTransport := transport.NewSSETransport(mcpServer)
	mux := http.NewServeMux()
	sseTransport.RegisterRoutes(mux)

	gatewayServer := httptest.NewServer(mux)
	defer gatewayServer.Close()

	// 1. Establish SSE Connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	sseReq, _ := http.NewRequestWithContext(ctx, "GET", gatewayServer.URL+"/sse", nil)
	sseResp, err := http.DefaultClient.Do(sseReq)
	if err != nil {
		t.Fatalf("failed connecting to SSE: %v", err)
	}
	defer sseResp.Body.Close()

	reader := bufio.NewReader(sseResp.Body)

	// Read initial endpoint event
	var endpointURL string
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("failed reading endpoint event: %v", err)
		}
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "data: /message?sessionId=") {
			endpointURL = strings.TrimPrefix(line, "data: ")
			break
		}
	}

	// 2. Post listCustomers tool call to endpoint
	postPayload := `{"jsonrpc":"2.0","id":99,"method":"tools/call","params":{"name":"listCustomers","arguments":{}}}`
	postResp, err := http.Post(gatewayServer.URL+endpointURL, "application/json", strings.NewReader(postPayload))
	if err != nil {
		t.Fatalf("failed posting to session endpoint: %v", err)
	}
	defer postResp.Body.Close()

	if postResp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202 Accepted, got %d", postResp.StatusCode)
	}

	// 3. Receive message on SSE stream
	var sseData string
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("failed reading SSE response: %v", err)
		}
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "data: ") {
			sseData = strings.TrimPrefix(line, "data: ")
			break
		}
	}

	var jsonResp protocol.JSONRPCResponse
	if err := json.Unmarshal([]byte(sseData), &jsonResp); err != nil {
		t.Fatalf("failed unmarshaling SSE json response: %v (raw: %s)", err, sseData)
	}

	if jsonResp.ID != float64(99) {
		t.Errorf("expected ID 99, got %v", jsonResp.ID)
	}
}
