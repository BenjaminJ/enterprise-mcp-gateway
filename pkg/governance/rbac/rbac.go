package rbac

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	"github.com/goschan/enterprise-mcp-gateway/pkg/mcp/protocol"
)

var (
	ErrUnauthorized = errors.New("unauthorized: caller role does not have permission to execute this tool")
	ErrNoIdentity   = errors.New("unauthorized: no caller identity provided")
)

type contextKey string

const callerIdentityKey contextKey = "mcp_caller_identity"

// CallerIdentity encapsulates the authenticated caller's metadata.
type CallerIdentity struct {
	Token    string `json:"token,omitempty"`
	CallerID string `json:"caller_id"`
	Role     string `json:"role"`
}

// WithCallerIdentity stores the caller identity in the context.
func WithCallerIdentity(ctx context.Context, id CallerIdentity) context.Context {
	return context.WithValue(ctx, callerIdentityKey, id)
}

// GetCallerIdentity retrieves the caller identity from the context.
func GetCallerIdentity(ctx context.Context) (CallerIdentity, bool) {
	id, ok := ctx.Value(callerIdentityKey).(CallerIdentity)
	return id, ok
}

// Role defines tool access permissions for a specific role.
type Role struct {
	Name         string   `json:"name"`
	AllowedTools []string `json:"allowed_tools"`
}

// Manager governs role-based tool visibility and execution permissions.
type Manager struct {
	tokens       map[string]string // token -> roleName
	roles        map[string]Role   // roleName -> Role
	defaultRole  string
	enabled      bool
}

// NewManager creates a new RBAC manager.
func NewManager(rolePerms map[string][]string, tokens map[string]string, defaultRole string, enabled bool) *Manager {
	roles := make(map[string]Role)
	for name, patterns := range rolePerms {
		roles[name] = Role{
			Name:         name,
			AllowedTools: patterns,
		}
	}

	return &Manager{
		tokens:      tokens,
		roles:       roles,
		defaultRole: defaultRole,
		enabled:     enabled,
	}
}

// ResolveRole looks up the role associated with a token, or falls back to defaultRole.
func (m *Manager) ResolveRole(token string) string {
	if !m.enabled {
		return "admin"
	}
	if role, ok := m.tokens[token]; ok {
		return role
	}
	return m.defaultRole
}

// IsToolAllowed checks if a role has permission to access a tool name.
func (m *Manager) IsToolAllowed(roleName string, toolName string) bool {
	if !m.enabled {
		return true
	}
	role, ok := m.roles[roleName]
	if !ok {
		return false
	}

	for _, pattern := range role.AllowedTools {
		if pattern == "*" {
			return true
		}
		// Support glob matching (e.g. get*, *_read)
		if matched, _ := filepath.Match(pattern, toolName); matched {
			return true
		}
		// Exact match
		if strings.EqualFold(pattern, toolName) {
			return true
		}
	}
	return false
}

// FilterTools returns only the tools permitted for the given role.
func (m *Manager) FilterTools(roleName string, tools []protocol.Tool) []protocol.Tool {
	if !m.enabled {
		return tools
	}

	filtered := make([]protocol.Tool, 0, len(tools))
	for _, tool := range tools {
		if m.IsToolAllowed(roleName, tool.Name) {
			filtered = append(filtered, tool)
		}
	}
	return filtered
}

// Authorize validates whether the caller in the context is allowed to invoke the tool.
func (m *Manager) Authorize(ctx context.Context, toolName string) error {
	if !m.enabled {
		return nil
	}

	identity, ok := GetCallerIdentity(ctx)
	if !ok {
		if m.defaultRole == "" {
			return ErrNoIdentity
		}
		identity = CallerIdentity{
			Role:     m.defaultRole,
			CallerID: "anonymous",
		}
	}

	if !m.IsToolAllowed(identity.Role, toolName) {
		return ErrUnauthorized
	}
	return nil
}
