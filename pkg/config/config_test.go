package config

import (
	"os"
	"testing"
)

const sampleConfigYAML = `
server:
  name: "enterprise-mcp-gateway"
  version: "1.2.0"
  transport: "sse"
  port: 9090
governance:
  enabled: true
  default_role: "readonly"
  tokens:
    "tok-1": "admin"
    "tok-2": "readonly"
  roles:
    readonly:
      allowed_tools:
        - "get*"
    admin:
      allowed_tools:
        - "*"
sanitizer:
  enabled: true
  mask_card_numbers: true
  mask_ssn: true
  sensitive_keys:
    - "password"
    - "token"
  custom_regex:
    - name: "AccountID"
      pattern: "\\bACC-[0-9]{4}\\b"
      replacement: "[REDACTED-ACC]"
audit:
  enabled: true
  log_path: "stdout"
  hash_inputs: true
`

func TestLoadConfigFromBytes(t *testing.T) {
	cfg, err := LoadConfigFromBytes([]byte(sampleConfigYAML))
	if err != nil {
		t.Fatalf("unexpected error loading config: %v", err)
	}

	if cfg.Server.Transport != "sse" {
		t.Errorf("expected transport 'sse', got '%s'", cfg.Server.Transport)
	}
	if cfg.Server.Port != 9090 {
		t.Errorf("expected port 9090, got %d", cfg.Server.Port)
	}
	if !cfg.Governance.Enabled {
		t.Errorf("expected governance to be enabled")
	}
	if len(cfg.Governance.Roles) != 2 {
		t.Errorf("expected 2 roles, got %d", len(cfg.Governance.Roles))
	}

	// Verify converters
	rbacMgr := cfg.ToRBACManager()
	if rbacMgr.ResolveRole("tok-1") != "admin" {
		t.Errorf("expected admin role for tok-1, got %s", rbacMgr.ResolveRole("tok-1"))
	}

	sanitizer, err := cfg.ToPIISanitizer()
	if err != nil {
		t.Fatalf("failed creating sanitizer: %v", err)
	}
	cleaned, count := sanitizer.SanitizeText("Account ACC-1234 active")
	if count != 1 || cleaned != "Account [REDACTED-ACC] active" {
		t.Errorf("unexpected custom pattern sanitization: %s (count: %d)", cleaned, count)
	}

	logger, err := cfg.ToAuditLogger()
	if err != nil {
		t.Fatalf("failed creating audit logger: %v", err)
	}
	_ = logger.Close()
}

func TestEnvOverrides(t *testing.T) {
	_ = os.Setenv("MCP_PORT", "9999")
	_ = os.Setenv("MCP_TRANSPORT", "sse")
	defer func() {
		_ = os.Unsetenv("MCP_PORT")
		_ = os.Unsetenv("MCP_TRANSPORT")
	}()

	cfg, err := LoadConfigFromBytes([]byte(`
server:
  port: 8080
  transport: "stdio"
`))
	if err != nil {
		t.Fatalf("failed loading config: %v", err)
	}

	if cfg.Server.Port != 9999 {
		t.Errorf("expected env override port 9999, got %d", cfg.Server.Port)
	}
	if cfg.Server.Transport != "sse" {
		t.Errorf("expected env override transport 'sse', got '%s'", cfg.Server.Transport)
	}
}
