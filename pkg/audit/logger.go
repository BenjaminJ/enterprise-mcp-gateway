package audit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"sync"
	"time"
)

// AuditStatus constants
const (
	StatusSuccess      = "SUCCESS"
	StatusUnauthorized = "UNAUTHORIZED"
	StatusError        = "ERROR"
)

// AuditEvent represents a single immutable structured audit record.
type AuditEvent struct {
	Timestamp        time.Time              `json:"timestamp"`
	RequestID        string                 `json:"request_id,omitempty"`
	CallerID         string                 `json:"caller_id"`
	Role             string                 `json:"role"`
	Tool             string                 `json:"tool"`
	InputSHA256      string                 `json:"input_sha256,omitempty"`
	DurationMs       int64                  `json:"duration_ms"`
	Status           string                 `json:"status"`
	PIIRedactedCount int                    `json:"pii_redacted_count"`
	Error            string                 `json:"error,omitempty"`
	Metadata         map[string]interface{} `json:"metadata,omitempty"`
}

// Logger provides thread-safe structured audit logging.
type Logger struct {
	mu         sync.Mutex
	writer     io.Writer
	closer     io.Closer
	enabled    bool
	hashInputs bool
}

// NewLogger creates a new audit Logger.
func NewLogger(w io.Writer, enabled bool, hashInputs bool) *Logger {
	var closer io.Closer
	if c, ok := w.(io.Closer); ok {
		closer = c
	}

	return &Logger{
		writer:     w,
		closer:     closer,
		enabled:    enabled,
		hashInputs: hashInputs,
	}
}

// NewFileLogger creates a logger writing to a specified file path.
func NewFileLogger(path string, enabled bool, hashInputs bool) (*Logger, error) {
	if !enabled || path == "" || path == "stdout" {
		return NewLogger(os.Stdout, enabled, hashInputs), nil
	}
	if path == "stderr" {
		return NewLogger(os.Stderr, enabled, hashInputs), nil
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return nil, err
	}
	return NewLogger(f, enabled, hashInputs), nil
}

// Log writes an audit event as a single line JSON.
func (l *Logger) Log(ctx context.Context, event AuditEvent) error {
	if !l.enabled || l.writer == nil {
		return nil
	}

	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}

	data, err := json.Marshal(event)
	if err != nil {
		return err
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	if _, err := l.writer.Write(data); err != nil {
		return err
	}
	if _, err := l.writer.Write([]byte("\n")); err != nil {
		return err
	}
	return nil
}

// HashInput computes a SHA-256 hash of an arbitrary input object for audit logging.
func (l *Logger) HashInput(input any) string {
	if !l.hashInputs || input == nil {
		return ""
	}

	var data []byte
	switch v := input.(type) {
	case []byte:
		data = v
	case string:
		data = []byte(v)
	default:
		marshaled, err := json.Marshal(v)
		if err != nil {
			return ""
		}
		data = marshaled
	}

	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

// Close closes any underlying file writer if open.
func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closer != nil && l.closer != os.Stdout && l.closer != os.Stderr {
		return l.closer.Close()
	}
	return nil
}
