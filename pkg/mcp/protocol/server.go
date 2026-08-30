package protocol

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

// ToolHandler defines a callback function that executes an MCP tool.
type ToolHandler func(ctx context.Context, name string, args map[string]interface{}) (*CallToolResult, error)

// ToolFilter allows filtering the list of advertised tools based on caller context (e.g. RBAC).
type ToolFilter func(ctx context.Context, tools []Tool) []Tool

// Server manages MCP tools and handles JSON-RPC 2.0 requests.
type Server struct {
	mu              sync.RWMutex
	info            Implementation
	protocolVersion string
	instructions    string
	tools           map[string]Tool
	toolHandler     ToolHandler
	toolFilter      ToolFilter
	initialized     bool
}

// NewServer creates a new Server instance.
func NewServer(info Implementation) *Server {
	return &Server{
		info:            info,
		protocolVersion: LatestProtocolVersion,
		tools:           make(map[string]Tool),
	}
}

// SetInstructions sets general server instructions returned during initialization.
func (s *Server) SetInstructions(instructions string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.instructions = instructions
}

// RegisterTool registers an MCP tool definition.
func (s *Server) RegisterTool(tool Tool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tools[tool.Name] = tool
}

// RegisterTools registers multiple MCP tool definitions.
func (s *Server) RegisterTools(tools []Tool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, tool := range tools {
		s.tools[tool.Name] = tool
	}
}

// GetTool retrieves a tool definition by name.
func (s *Server) GetTool(name string) (Tool, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	tool, exists := s.tools[name]
	return tool, exists
}

// ListTools returns all registered tools.
func (s *Server) ListTools() []Tool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]Tool, 0, len(s.tools))
	for _, t := range s.tools {
		result = append(result, t)
	}
	return result
}

// SetToolHandler configures the tool execution handler.
func (s *Server) SetToolHandler(handler ToolHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.toolHandler = handler
}

// SetToolFilter configures a context-aware tool filter (e.g. for RBAC).
func (s *Server) SetToolFilter(filter ToolFilter) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.toolFilter = filter
}

// HandleMessage parses a raw JSON-RPC frame, processes it, and returns the serialized response.
// If the message is a notification (id == nil) that does not require a response, it returns (nil, nil).
func (s *Server) HandleMessage(ctx context.Context, data []byte) ([]byte, error) {
	var req JSONRPCRequest
	if err := json.Unmarshal(data, &req); err != nil {
		resp := NewErrorResponse(nil, CodeParseError, "Parse error: invalid JSON", err.Error())
		return json.Marshal(resp)
	}

	resp, err := s.HandleRequest(ctx, &req)
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, nil // Notification, no response
	}

	return json.Marshal(resp)
}

// HandleRequest processes a parsed JSON-RPC request.
func (s *Server) HandleRequest(ctx context.Context, req *JSONRPCRequest) (*JSONRPCResponse, error) {
	switch req.Method {
	case "initialize":
		return s.handleInitialize(ctx, req)
	case "notifications/initialized":
		s.mu.Lock()
		s.initialized = true
		s.mu.Unlock()
		return nil, nil // Notifications do not generate a response
	case "ping":
		return NewSuccessResponse(req.ID, map[string]interface{}{}), nil
	case "tools/list":
		return s.handleToolsList(ctx, req)
	case "tools/call":
		return s.handleToolsCall(ctx, req)
	default:
		// Unknown method
		if req.ID == nil {
			// Notification with unknown method - ignore
			return nil, nil
		}
		return NewErrorResponse(req.ID, CodeMethodNotFound, fmt.Sprintf("Method '%s' not found", req.Method), nil), nil
	}
}

func (s *Server) handleInitialize(ctx context.Context, req *JSONRPCRequest) (*JSONRPCResponse, error) {
	var params InitializeParams
	if len(req.Params) > 0 {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return NewErrorResponse(req.ID, CodeInvalidParams, "Invalid initialize params", err.Error()), nil
		}
	}

	res := InitializeResult{
		ProtocolVersion: s.protocolVersion,
		Capabilities: ServerCapabilities{
			Tools: &ToolsCapability{
				ListChanged: false,
			},
			Logging: &LoggingCapability{},
		},
		ServerInfo:   s.info,
		Instructions: s.instructions,
	}

	return NewSuccessResponse(req.ID, res), nil
}

func (s *Server) handleToolsList(ctx context.Context, req *JSONRPCRequest) (*JSONRPCResponse, error) {
	tools := s.ListTools()

	s.mu.RLock()
	filter := s.toolFilter
	s.mu.RUnlock()

	if filter != nil {
		tools = filter(ctx, tools)
	}

	return NewSuccessResponse(req.ID, ListToolsResult{
		Tools: tools,
	}), nil
}

func (s *Server) handleToolsCall(ctx context.Context, req *JSONRPCRequest) (*JSONRPCResponse, error) {
	var params CallToolParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return NewErrorResponse(req.ID, CodeInvalidParams, "Invalid tools/call params", err.Error()), nil
	}

	if params.Name == "" {
		return NewErrorResponse(req.ID, CodeInvalidParams, "Missing tool name", nil), nil
	}

	s.mu.RLock()
	_, exists := s.tools[params.Name]
	handler := s.toolHandler
	s.mu.RUnlock()

	if !exists {
		return NewErrorResponse(req.ID, CodeInvalidParams, fmt.Sprintf("Tool '%s' not found", params.Name), nil), nil
	}

	if handler == nil {
		return NewErrorResponse(req.ID, CodeInternalError, "Tool handler not configured", nil), nil
	}

	result, err := handler(ctx, params.Name, params.Arguments)
	if err != nil {
		return NewSuccessResponse(req.ID, CallToolResult{
			Content: []Content{
				NewTextContent(fmt.Sprintf("Error executing tool '%s': %v", params.Name, err)),
			},
			IsError: true,
		}), nil
	}

	if result == nil {
		result = &CallToolResult{
			Content: []Content{},
		}
	}

	return NewSuccessResponse(req.ID, result), nil
}
