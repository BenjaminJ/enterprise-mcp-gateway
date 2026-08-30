package pii

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	"github.com/goschan/enterprise-mcp-gateway/pkg/mcp/protocol"
)

func TestSanitizeCreditCards(t *testing.T) {
	cfg := DefaultConfig()
	s := NewSanitizer(cfg)

	// Valid Visa test number (4111 1111 1111 1111)
	validVisa := "Customer card is 4111-1111-1111-1111 and it expires soon."
	cleaned, count := s.SanitizeText(validVisa)
	if count != 1 {
		t.Errorf("expected 1 redacted card, got %d", count)
	}
	if strings.Contains(cleaned, "4111") {
		t.Errorf("expected card to be redacted, got %s", cleaned)
	}
	if !strings.Contains(cleaned, "[REDACTED-CARD]") {
		t.Errorf("expected placeholder [REDACTED-CARD], got %s", cleaned)
	}

	// Invalid Luhn number should NOT be masked
	invalidCard := "Order number is 4111-1111-1111-1112 in the database."
	cleaned2, count2 := s.SanitizeText(invalidCard)
	if count2 != 0 {
		t.Errorf("invalid card should not be redacted, got count %d (text: %s)", count2, cleaned2)
	}
}

func TestSanitizeSSN(t *testing.T) {
	cfg := DefaultConfig()
	s := NewSanitizer(cfg)

	input := "User SSN is 123-45-6789."
	cleaned, count := s.SanitizeText(input)
	if count != 1 {
		t.Errorf("expected 1 redacted SSN, got %d", count)
	}
	if !strings.Contains(cleaned, "[REDACTED-SSN]") {
		t.Errorf("expected [REDACTED-SSN], got %s", cleaned)
	}
}

func TestSanitizeSecrets(t *testing.T) {
	cfg := DefaultConfig()
	s := NewSanitizer(cfg)

	// AWS key
	awsInput := "Found AWS key AKIAIOSFODNN7EXAMPLE in config."
	cleaned, count := s.SanitizeText(awsInput)
	if count != 1 || !strings.Contains(cleaned, "[REDACTED-AWS-KEY]") {
		t.Errorf("failed redacting AWS key: %s (count: %d)", cleaned, count)
	}

	// GitHub token
	ghInput := "GitHub token is ghp_1234567890abcdefghijklmnopqrstuvwxyzAB"
	cleanedGH, countGH := s.SanitizeText(ghInput)
	if countGH != 1 || !strings.Contains(cleanedGH, "[REDACTED-GITHUB-TOKEN]") {
		t.Errorf("failed redacting GitHub token: %s (count: %d)", cleanedGH, countGH)
	}
}

func TestSanitizeJSONKeys(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaskEmails = true
	s := NewSanitizer(cfg)

	rawJSON := `{
		"username": "johndoe",
		"password": "SuperSecretPassword123!",
		"email": "john@example.com",
		"api_key": "secret-key-xyz",
		"nested": {
			"token": "token-abc",
			"ssn": "123-45-6789",
			"notes": "safe note"
		}
	}`

	cleaned, count := s.SanitizeJSON([]byte(rawJSON))
	if count < 4 {
		t.Errorf("expected at least 4 items redacted, got %d", count)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(cleaned, &parsed); err != nil {
		t.Fatalf("failed unmarshaling sanitized json: %v", err)
	}

	if parsed["password"] != "[REDACTED]" {
		t.Errorf("expected password to be [REDACTED], got %v", parsed["password"])
	}
	if parsed["api_key"] != "[REDACTED]" {
		t.Errorf("expected api_key to be [REDACTED], got %v", parsed["api_key"])
	}
	if parsed["email"] != "[REDACTED-EMAIL]" {
		t.Errorf("expected email to be [REDACTED-EMAIL], got %v", parsed["email"])
	}
}

func TestSanitizeCustomPattern(t *testing.T) {
	cfg := DefaultConfig()
	cfg.CustomPatterns = []CustomPattern{
		{
			Name:        "Employee ID",
			Regex:       regexp.MustCompile(`\bEMP-[0-9]{6}\b`),
			Replacement: "[REDACTED-EMP-ID]",
		},
	}
	s := NewSanitizer(cfg)

	input := "Employee EMP-998877 logged into system."
	cleaned, count := s.SanitizeText(input)
	if count != 1 || !strings.Contains(cleaned, "[REDACTED-EMP-ID]") {
		t.Errorf("failed custom regex redaction: %s (count: %d)", cleaned, count)
	}
}

func TestSanitizeResult(t *testing.T) {
	cfg := DefaultConfig()
	s := NewSanitizer(cfg)

	res := &protocol.CallToolResult{
		Content: []protocol.Content{
			protocol.NewTextContent(`{"user_id":123,"password":"secretPassword"}`),
			protocol.NewTextContent("Call returned SSN: 123-45-6789"),
		},
	}

	count := s.SanitizeResult(res)
	if count < 2 {
		t.Errorf("expected at least 2 redactions, got %d", count)
	}

	if strings.Contains(res.Content[0].Text, "secretPassword") {
		t.Errorf("content 0 was not redacted: %s", res.Content[0].Text)
	}
	if strings.Contains(res.Content[1].Text, "123-45-6789") {
		t.Errorf("content 1 was not redacted: %s", res.Content[1].Text)
	}
}
