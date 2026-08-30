package protocol

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func TestServerInitialize(t *testing.T) {
	srv := NewServer(Implementation{
		Name:    "test-gateway",
		Version: "0.1.0",
	})

	initReq := &JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "initialize",
		Params:  json.RawMessage(`{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test-client","version":"1.0"}}`),
	}

	resp, err := srv.HandleRequest(context.Background(), initReq)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Error != nil {
		t.Fatalf("unexpected jsonrpc error: %v", resp.Error)
	}

	res, ok := resp.Result.(InitializeResult)
	if !ok {
		t.Fatalf("expected InitializeResult, got %T", resp.Result)
	}

	if res.ServerInfo.Name != "test-gateway" {
		t.Errorf("expected server name 'test-gateway', got '%s'", res.ServerInfo.Name)
	}
	if res.ProtocolVersion != LatestProtocolVersion {
		t.Errorf("expected protocol version '%s', got '%s'", LatestProtocolVersion, res.ProtocolVersion)
	}
}

func TestServerToolsListAndFilter(t *testing.T) {
	srv := NewServer(Implementation{Name: "test-gateway", Version: "0.1.0"})

	tool1 := Tool{
		Name:        "getUsers",
		Description: "List all users",
		InputSchema: ToolInputSchema{Type: "object"},
	}
	tool2 := Tool{
		Name:        "deleteUser",
		Description: "Delete user",
		InputSchema: ToolInputSchema{Type: "object"},
	}

	srv.RegisterTools([]Tool{tool1, tool2})

	// Without filter
	req := &JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      2,
		Method:  "tools/list",
	}

	resp, err := srv.HandleRequest(context.Background(), req)
	if err != nil || resp.Error != nil {
		t.Fatalf("failed listing tools: %v, %v", err, resp.Error)
	}

	listRes, ok := resp.Result.(ListToolsResult)
	if !ok || len(listRes.Tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(listRes.Tools))
	}

	// With RBAC filter allowing only "get*"
	srv.SetToolFilter(func(ctx context.Context, tools []Tool) []Tool {
		var filtered []Tool
		for _, tool := range tools {
			if tool.Name == "getUsers" {
				filtered = append(filtered, tool)
			}
		}
		return filtered
	})

	resp2, err := srv.HandleRequest(context.Background(), req)
	if err != nil || resp2.Error != nil {
		t.Fatalf("failed listing filtered tools: %v", err)
	}
	listRes2 := resp2.Result.(ListToolsResult)
	if len(listRes2.Tools) != 1 || listRes2.Tools[0].Name != "getUsers" {
		t.Fatalf("expected 1 tool 'getUsers', got %v", listRes2.Tools)
	}
}

func TestServerToolsCall(t *testing.T) {
	srv := NewServer(Implementation{Name: "test-gateway", Version: "0.1.0"})

	tool := Tool{
		Name:        "echo",
		Description: "Echo message",
		InputSchema: ToolInputSchema{
			Type: "object",
			Properties: map[string]interface{}{
				"message": map[string]interface{}{"type": "string"},
			},
		},
	}
	srv.RegisterTool(tool)

	srv.SetToolHandler(func(ctx context.Context, name string, args map[string]interface{}) (*CallToolResult, error) {
		if name == "echo" {
			msg, _ := args["message"].(string)
			return &CallToolResult{
				Content: []Content{
					NewTextContent("echo: " + msg),
				},
			}, nil
		}
		return nil, errors.New("unknown tool")
	})

	callReq := &JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      3,
		Method:  "tools/call",
		Params:  json.RawMessage(`{"name":"echo","arguments":{"message":"hello world"}}`),
	}

	resp, err := srv.HandleRequest(context.Background(), callReq)
	if err != nil || resp.Error != nil {
		t.Fatalf("failed tools/call: %v, %v", err, resp.Error)
	}

	callRes, ok := resp.Result.(*CallToolResult)
	if !ok || len(callRes.Content) != 1 {
		t.Fatalf("expected 1 content, got %v", callRes)
	}
	if callRes.Content[0].Text != "echo: hello world" {
		t.Errorf("unexpected content: %s", callRes.Content[0].Text)
	}

	// Unknown tool call
	badReq := &JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      4,
		Method:  "tools/call",
		Params:  json.RawMessage(`{"name":"nonexistent"}`),
	}
	badResp, err := srv.HandleRequest(context.Background(), badReq)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if badResp.Error == nil || badResp.Error.Code != CodeInvalidParams {
		t.Errorf("expected InvalidParams error code, got %v", badResp.Error)
	}
}

func TestServerHandleMessage(t *testing.T) {
	srv := NewServer(Implementation{Name: "test-gateway", Version: "0.1.0"})

	// Test ping
	rawPing := []byte(`{"jsonrpc":"2.0","id":"ping-1","method":"ping"}`)
	out, err := srv.HandleMessage(context.Background(), rawPing)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var resp JSONRPCResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		t.Fatalf("failed unmarshaling ping response: %v", err)
	}
	if resp.ID != "ping-1" {
		t.Errorf("expected ID 'ping-1', got %v", resp.ID)
	}

	// Test notification (should return nil response)
	rawNotif := []byte(`{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	notifOut, err := srv.HandleMessage(context.Background(), rawNotif)
	if err != nil {
		t.Fatalf("unexpected error on notification: %v", err)
	}
	if notifOut != nil {
		t.Errorf("expected nil output for notification, got %s", string(notifOut))
	}

	// Test invalid JSON parse error
	badJSON := []byte(`{invalid-json`)
	badOut, err := srv.HandleMessage(context.Background(), badJSON)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var errResp JSONRPCResponse
	if err := json.Unmarshal(badOut, &errResp); err != nil {
		t.Fatalf("failed unmarshaling error response: %v", err)
	}
	if errResp.Error == nil || errResp.Error.Code != CodeParseError {
		t.Errorf("expected CodeParseError (-32700), got %v", errResp.Error)
	}
}
