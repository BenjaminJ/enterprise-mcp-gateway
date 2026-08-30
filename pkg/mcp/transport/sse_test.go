package transport

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/goschan/enterprise-mcp-gateway/pkg/mcp/protocol"
)

func TestSSETransport(t *testing.T) {
	handler := &mockHandler{}
	tr := NewSSETransport(handler)

	mux := http.NewServeMux()
	tr.RegisterRoutes(mux)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Step 1: Connect to SSE stream
	req, err := http.NewRequestWithContext(ctx, "GET", ts.URL+"/sse", nil)
	if err != nil {
		t.Fatalf("failed creating SSE request: %v", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed connecting to SSE: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	reader := bufio.NewReader(resp.Body)

	// Read initial endpoint event
	var endpointURL string
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("failed reading SSE stream: %v", err)
		}
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "data: /message?sessionId=") {
			endpointURL = strings.TrimPrefix(line, "data: ")
			break
		}
	}

	if endpointURL == "" {
		t.Fatalf("no endpoint event received")
	}

	// Step 2: Post JSON-RPC request to the session endpoint
	postPayload := `{"jsonrpc":"2.0","id":100,"method":"ping"}`
	postResp, err := http.Post(ts.URL+endpointURL, "application/json", strings.NewReader(postPayload))
	if err != nil {
		t.Fatalf("failed posting to session endpoint: %v", err)
	}
	defer postResp.Body.Close()

	if postResp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected status 202 Accepted, got %d", postResp.StatusCode)
	}

	// Step 3: Verify SSE receives the response message
	var msgData string
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("failed reading message from SSE: %v", err)
		}
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "data: ") {
			msgData = strings.TrimPrefix(line, "data: ")
			break
		}
	}

	var jsonResp protocol.JSONRPCResponse
	if err := json.Unmarshal([]byte(msgData), &jsonResp); err != nil {
		t.Fatalf("failed unmarshaling SSE data: %v (raw: %s)", err, msgData)
	}

	if jsonResp.ID != float64(100) {
		t.Errorf("expected ID 100, got %v", jsonResp.ID)
	}
}
