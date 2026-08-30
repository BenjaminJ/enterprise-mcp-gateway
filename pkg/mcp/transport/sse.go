package transport

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"sync"
)

// Session represents an active SSE client connection.
type Session struct {
	ID        string
	Events    chan []byte
	Done      chan struct{}
	CreatedAt int64
}

// SSETransport manages MCP communication over HTTP Server-Sent Events.
type SSETransport struct {
	server   MessageHandler
	sessions map[string]*Session
	mu       sync.RWMutex
}

// NewSSETransport creates a new SSE transport handler.
func NewSSETransport(srv MessageHandler) *SSETransport {
	return &SSETransport{
		server:   srv,
		sessions: make(map[string]*Session),
	}
}

// RegisterRoutes registers the /sse and /message handlers on a standard http.ServeMux.
func (t *SSETransport) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/sse", t.HandleSSE)
	mux.HandleFunc("/message", t.HandleMessage)
}

// HandleSSE handles incoming SSE connection requests (GET /sse).
func (t *SSETransport) HandleSSE(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	sessionID := generateSessionID()
	session := &Session{
		ID:     sessionID,
		Events: make(chan []byte, 100),
		Done:   make(chan struct{}),
	}

	t.mu.Lock()
	t.sessions[sessionID] = session
	t.mu.Unlock()

	defer func() {
		t.mu.Lock()
		delete(t.sessions, sessionID)
		t.mu.Unlock()
		close(session.Done)
	}()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Send endpoint event with session endpoint URI
	endpointURL := fmt.Sprintf("/message?sessionId=%s", sessionID)
	fmt.Fprintf(w, "event: endpoint\ndata: %s\n\n", endpointURL)
	flusher.Flush()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-session.Events:
			if !ok {
				return
			}
			fmt.Fprintf(w, "event: message\ndata: %s\n\n", string(msg))
			flusher.Flush()
		}
	}
}

// HandleMessage handles client JSON-RPC messages posted to /message?sessionId=...
func (t *SSETransport) HandleMessage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	sessionID := r.URL.Query().Get("sessionId")
	if sessionID == "" {
		http.Error(w, "Missing sessionId parameter", http.StatusBadRequest)
		return
	}

	t.mu.RLock()
	session, exists := t.sessions[sessionID]
	t.mu.RUnlock()

	if !exists {
		http.Error(w, "Session not found or expired", http.StatusNotFound)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	resp, err := t.server.HandleMessage(r.Context(), body)
	if err != nil {
		http.Error(w, fmt.Sprintf("Handler error: %v", err), http.StatusInternalServerError)
		return
	}

	// If there is a response, send it through the SSE event channel
	if len(resp) > 0 {
		select {
		case session.Events <- resp:
		case <-session.Done:
			http.Error(w, "Session closed", http.StatusGone)
			return
		}
	}

	w.WriteHeader(http.StatusAccepted)
	_, _ = w.Write([]byte("Accepted"))
}

func generateSessionID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
