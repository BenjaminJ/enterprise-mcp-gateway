package transport

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/goschan/enterprise-mcp-gateway/pkg/mcp/protocol"
)

type mockHandler struct {
	handled [][]byte
}

func (m *mockHandler) HandleMessage(ctx context.Context, data []byte) ([]byte, error) {
	m.handled = append(m.handled, data)
	var req protocol.JSONRPCRequest
	_ = json.Unmarshal(data, &req)
	resp := protocol.NewSuccessResponse(req.ID, map[string]string{"status": "ok"})
	return json.Marshal(resp)
}

func TestStdioTransport(t *testing.T) {
	inBuf := &bytes.Buffer{}
	outBuf := &bytes.Buffer{}

	inBuf.WriteString(`{"jsonrpc":"2.0","id":1,"method":"ping"}` + "\n")
	inBuf.WriteString(`{"jsonrpc":"2.0","id":2,"method":"ping"}` + "\n")

	handler := &mockHandler{}
	tr := NewStdioTransport(inBuf, outBuf, handler)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := tr.Start(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(handler.handled) != 2 {
		t.Fatalf("expected 2 messages handled, got %d", len(handler.handled))
	}

	lines := strings.Split(strings.TrimSpace(outBuf.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 output lines, got %d. Output: %s", len(lines), outBuf.String())
	}

	var resp1 protocol.JSONRPCResponse
	if err := json.Unmarshal([]byte(lines[0]), &resp1); err != nil {
		t.Fatalf("failed unmarshaling resp1: %v", err)
	}
	if resp1.ID != float64(1) { // JSON unmarshals un-typed numbers to float64
		t.Errorf("expected ID 1, got %v", resp1.ID)
	}
}
