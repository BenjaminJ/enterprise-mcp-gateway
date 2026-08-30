package transport

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"sync"

	"github.com/goschan/enterprise-mcp-gateway/pkg/mcp/protocol"
)

// MessageHandler interface for processing JSON-RPC messages.
type MessageHandler interface {
	HandleMessage(ctx context.Context, data []byte) ([]byte, error)
}

// StdioTransport handles communication over Stdin and Stdout.
type StdioTransport struct {
	reader io.Reader
	writer io.Writer
	server MessageHandler
	mu     sync.Mutex
}

// NewStdioTransport creates a new stdio transport.
func NewStdioTransport(r io.Reader, w io.Writer, srv MessageHandler) *StdioTransport {
	return &StdioTransport{
		reader: r,
		writer: w,
		server: srv,
	}
}

// Start starts listening on stdin and responding on stdout until EOF or ctx is done.
func (t *StdioTransport) Start(ctx context.Context) error {
	scanner := bufio.NewScanner(t.reader)
	// Allow large payloads (up to 16MB)
	const maxScanCapacity = 16 * 1024 * 1024
	buf := make([]byte, 64*1024)
	scanner.Buffer(buf, maxScanCapacity)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		// Make a copy of line bytes for safety
		lineCopy := make([]byte, len(line))
		copy(lineCopy, line)

		resp, err := t.server.HandleMessage(ctx, lineCopy)
		if err != nil {
			errResp := protocol.NewErrorResponse(nil, protocol.CodeInternalError, "Internal error processing message", err.Error())
			errBytes, _ := json.Marshal(errResp)
			_ = t.writeRaw(errBytes)
			continue
		}

		if len(resp) > 0 {
			if err := t.writeRaw(resp); err != nil {
				return err
			}
		}
	}

	if err := scanner.Err(); err != nil && err != io.EOF {
		return err
	}
	return nil
}

func (t *StdioTransport) writeRaw(data []byte) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if _, err := t.writer.Write(data); err != nil {
		return err
	}
	if _, err := t.writer.Write([]byte("\n")); err != nil {
		return err
	}
	return nil
}
