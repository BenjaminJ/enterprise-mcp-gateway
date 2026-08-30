package audit

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestAuditLogger(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := NewLogger(buf, true, true)

	inputArgs := map[string]interface{}{"userId": 12345, "query": "orders"}
	inputHash := logger.HashInput(inputArgs)

	if inputHash == "" {
		t.Fatalf("expected non-empty input hash")
	}

	event := AuditEvent{
		RequestID:        "req-1",
		CallerID:         "agent-007",
		Role:             "support_agent",
		Tool:             "getUserOrders",
		InputSHA256:      inputHash,
		DurationMs:       42,
		Status:           StatusSuccess,
		PIIRedactedCount: 1,
	}

	err := logger.Log(context.Background(), event)
	if err != nil {
		t.Fatalf("unexpected error logging event: %v", err)
	}

	output := strings.TrimSpace(buf.String())
	if output == "" {
		t.Fatalf("expected log output, got empty")
	}

	var parsed AuditEvent
	if err := json.Unmarshal([]byte(output), &parsed); err != nil {
		t.Fatalf("failed unmarshaling audit log output: %v", err)
	}

	if parsed.RequestID != "req-1" {
		t.Errorf("expected RequestID 'req-1', got '%s'", parsed.RequestID)
	}
	if parsed.Role != "support_agent" {
		t.Errorf("expected Role 'support_agent', got '%s'", parsed.Role)
	}
	if parsed.InputSHA256 != inputHash {
		t.Errorf("expected InputSHA256 '%s', got '%s'", inputHash, parsed.InputSHA256)
	}
	if parsed.Status != StatusSuccess {
		t.Errorf("expected Status SUCCESS, got '%s'", parsed.Status)
	}
}

func TestAuditLoggerDisabled(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := NewLogger(buf, false, true)

	err := logger.Log(context.Background(), AuditEvent{
		Tool:   "testTool",
		Status: StatusSuccess,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if buf.Len() != 0 {
		t.Errorf("disabled logger should not produce output, got %s", buf.String())
	}
}
