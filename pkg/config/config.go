package config

import (
	"fmt"
	"os"
	"regexp"
	"strconv"

	"github.com/goschan/enterprise-mcp-gateway/pkg/audit"
	"github.com/goschan/enterprise-mcp-gateway/pkg/governance/rbac"
	"github.com/goschan/enterprise-mcp-gateway/pkg/sanitizer/pii"
	"gopkg.in/yaml.v3"
)

// Config represents the top-level gateway configuration.
type Config struct {
	Server     ServerConfig      `yaml:"server"`
	Governance GovernanceConfig  `yaml:"governance"`
	Sanitizer  SanitizerConfig   `yaml:"sanitizer"`
	Audit      AuditConfig       `yaml:"audit"`
	Connectors []ConnectorConfig `yaml:"connectors"`
}

// ServerConfig holds HTTP/stdio server settings.
type ServerConfig struct {
	Name      string `yaml:"name"`
	Version   string `yaml:"version"`
	Transport string `yaml:"transport"` // "stdio" or "sse"
	Host      string `yaml:"host"`
	Port      int    `yaml:"port"`
}

// GovernanceConfig holds RBAC tokens and role definitions.
type GovernanceConfig struct {
	Enabled     bool                  `yaml:"enabled"`
	DefaultRole string                `yaml:"default_role"`
	Tokens      map[string]string     `yaml:"tokens"`
	Roles       map[string]RoleConfig `yaml:"roles"`
}

// RoleConfig defines permissions for a role.
type RoleConfig struct {
	AllowedTools []string `yaml:"allowed_tools"`
}

// SanitizerConfig holds PII/secret redaction settings.
type SanitizerConfig struct {
	Enabled          bool                  `yaml:"enabled"`
	MaskCardNumbers  bool                  `yaml:"mask_card_numbers"`
	MaskSSN          bool                  `yaml:"mask_ssn"`
	MaskEmails       bool                  `yaml:"mask_emails"`
	MaskPhoneNumbers bool                  `yaml:"mask_phone_numbers"`
	MaskSecrets      bool                  `yaml:"mask_secrets"`
	SensitiveKeys    []string              `yaml:"sensitive_keys"`
	CustomRegex      []CustomPatternConfig `yaml:"custom_regex"`
}

// CustomPatternConfig defines a regex mask rule.
type CustomPatternConfig struct {
	Name        string `yaml:"name"`
	Pattern     string `yaml:"pattern"`
	Replacement string `yaml:"replacement"`
}

// AuditConfig holds audit logging settings.
type AuditConfig struct {
	Enabled    bool   `yaml:"enabled"`
	LogPath    string `yaml:"log_path"`
	HashInputs bool   `yaml:"hash_inputs"`
}

// ConnectorConfig defines an external API connector.
type ConnectorConfig struct {
	Name           string            `yaml:"name"`
	Type           string            `yaml:"type"` // "openapi"
	SpecFile       string            `yaml:"spec_file"`
	BaseURL        string            `yaml:"base_url"`
	Headers        map[string]string `yaml:"headers"`
	TimeoutSeconds int               `yaml:"timeout_seconds"`
}

// DefaultConfig returns safe, production-ready defaults.
func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Name:      "enterprise-mcp-gateway",
			Version:   "1.0.0",
			Transport: "stdio",
			Host:      "0.0.0.0",
			Port:      8080,
		},
		Governance: GovernanceConfig{
			Enabled:     false,
			DefaultRole: "admin",
			Tokens:      make(map[string]string),
			Roles:       make(map[string]RoleConfig),
		},
		Sanitizer: SanitizerConfig{
			Enabled:          true,
			MaskCardNumbers:  true,
			MaskSSN:          true,
			MaskEmails:       false,
			MaskPhoneNumbers: false,
			MaskSecrets:      true,
			SensitiveKeys: []string{
				"password", "passwd", "secret", "token", "apiKey", "access_token", "ssn", "creditCard",
			},
		},
		Audit: AuditConfig{
			Enabled:    true,
			LogPath:    "stdout",
			HashInputs: true,
		},
		Connectors: make([]ConnectorConfig, 0),
	}
}

// LoadConfig loads configuration from a YAML file with environment variable overrides.
func LoadConfig(filePath string) (*Config, error) {
	cfg := DefaultConfig()

	if filePath != "" {
		data, err := os.ReadFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("failed to read config file '%s': %w", filePath, err)
		}
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("failed to parse config YAML: %w", err)
		}
	}

	applyEnvOverrides(cfg)
	return cfg, nil
}

// LoadConfigFromBytes parses YAML configuration bytes.
func LoadConfigFromBytes(data []byte) (*Config, error) {
	cfg := DefaultConfig()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config YAML: %w", err)
	}
	applyEnvOverrides(cfg)
	return cfg, nil
}

func applyEnvOverrides(cfg *Config) {
	if transport := os.Getenv("MCP_TRANSPORT"); transport != "" {
		cfg.Server.Transport = transport
	}
	if portStr := os.Getenv("MCP_PORT"); portStr != "" {
		if p, err := strconv.Atoi(portStr); err == nil {
			cfg.Server.Port = p
		}
	}
	if logPath := os.Getenv("MCP_LOG_PATH"); logPath != "" {
		cfg.Audit.LogPath = logPath
	}
}

// ToRBACManager builds an RBAC Manager from configuration.
func (cfg *Config) ToRBACManager() *rbac.Manager {
	roles := make(map[string][]string)
	for name, role := range cfg.Governance.Roles {
		roles[name] = role.AllowedTools
	}
	return rbac.NewManager(roles, cfg.Governance.Tokens, cfg.Governance.DefaultRole, cfg.Governance.Enabled)
}

// ToPIISanitizer builds a PII Sanitizer from configuration.
func (cfg *Config) ToPIISanitizer() (*pii.Sanitizer, error) {
	var custom []pii.CustomPattern
	for _, cp := range cfg.Sanitizer.CustomRegex {
		if cp.Pattern != "" {
			re, err := regexp.Compile(cp.Pattern)
			if err != nil {
				return nil, fmt.Errorf("invalid custom regex pattern '%s': %w", cp.Pattern, err)
			}
			custom = append(custom, pii.CustomPattern{
				Name:        cp.Name,
				Regex:       re,
				Replacement: cp.Replacement,
			})
		}
	}

	piiCfg := pii.Config{
		Enabled:          cfg.Sanitizer.Enabled,
		MaskCardNumbers:  cfg.Sanitizer.MaskCardNumbers,
		MaskSSN:          cfg.Sanitizer.MaskSSN,
		MaskEmails:       cfg.Sanitizer.MaskEmails,
		MaskPhoneNumbers: cfg.Sanitizer.MaskPhoneNumbers,
		MaskSecrets:      cfg.Sanitizer.MaskSecrets,
		SensitiveKeys:    cfg.Sanitizer.SensitiveKeys,
		CustomPatterns:   custom,
	}

	return pii.NewSanitizer(piiCfg), nil
}

// ToAuditLogger builds an Audit Logger from configuration.
func (cfg *Config) ToAuditLogger() (*audit.Logger, error) {
	return audit.NewFileLogger(cfg.Audit.LogPath, cfg.Audit.Enabled, cfg.Audit.HashInputs)
}
