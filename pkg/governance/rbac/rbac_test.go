package rbac

import (
	"context"
	"testing"

	"github.com/goschan/enterprise-mcp-gateway/pkg/mcp/protocol"
)

func TestRBACPolicyMatching(t *testing.T) {
	roles := map[string][]string{
		"readonly_agent": {"get*", "list*", "search*"},
		"support_agent":  {"get*", "list*", "search*", "updateTicket", "resetPassword"},
		"admin":          {"*"},
	}
	tokens := map[string]string{
		"token-ro":      "readonly_agent",
		"token-support": "support_agent",
		"token-admin":   "admin",
	}

	mgr := NewManager(roles, tokens, "readonly_agent", true)

	// Test ResolveRole
	if mgr.ResolveRole("token-ro") != "readonly_agent" {
		t.Errorf("expected readonly_agent, got %s", mgr.ResolveRole("token-ro"))
	}
	if mgr.ResolveRole("token-support") != "support_agent" {
		t.Errorf("expected support_agent, got %s", mgr.ResolveRole("token-support"))
	}
	if mgr.ResolveRole("unknown") != "readonly_agent" {
		t.Errorf("expected fallback defaultRole readonly_agent, got %s", mgr.ResolveRole("unknown"))
	}

	// Test IsToolAllowed
	if !mgr.IsToolAllowed("readonly_agent", "getUser") {
		t.Errorf("readonly_agent should be allowed to call getUser")
	}
	if mgr.IsToolAllowed("readonly_agent", "deleteUser") {
		t.Errorf("readonly_agent should NOT be allowed to call deleteUser")
	}
	if !mgr.IsToolAllowed("support_agent", "updateTicket") {
		t.Errorf("support_agent should be allowed to call updateTicket")
	}
	if mgr.IsToolAllowed("support_agent", "deleteUser") {
		t.Errorf("support_agent should NOT be allowed to call deleteUser")
	}
	if !mgr.IsToolAllowed("admin", "deleteUser") {
		t.Errorf("admin should be allowed to call deleteUser")
	}
}

func TestRBACFilterTools(t *testing.T) {
	roles := map[string][]string{
		"readonly": {"get*"},
	}
	mgr := NewManager(roles, nil, "", true)

	allTools := []protocol.Tool{
		{Name: "getUser"},
		{Name: "getOrder"},
		{Name: "createOrder"},
		{Name: "deleteOrder"},
	}

	filtered := mgr.FilterTools("readonly", allTools)
	if len(filtered) != 2 {
		t.Fatalf("expected 2 tools filtered, got %d", len(filtered))
	}
	for _, tool := range filtered {
		if tool.Name != "getUser" && tool.Name != "getOrder" {
			t.Errorf("unexpected tool in filtered list: %s", tool.Name)
		}
	}
}

func TestRBACAuthorizeContext(t *testing.T) {
	roles := map[string][]string{
		"viewer": {"get*"},
		"editor": {"get*", "create*"},
	}
	mgr := NewManager(roles, nil, "", true)

	ctxViewer := WithCallerIdentity(context.Background(), CallerIdentity{
		Role:     "viewer",
		CallerID: "user-1",
	})
	ctxEditor := WithCallerIdentity(context.Background(), CallerIdentity{
		Role:     "editor",
		CallerID: "user-2",
	})

	if err := mgr.Authorize(ctxViewer, "getUser"); err != nil {
		t.Errorf("viewer should be authorized for getUser: %v", err)
	}
	if err := mgr.Authorize(ctxViewer, "createUser"); err == nil {
		t.Errorf("viewer should NOT be authorized for createUser")
	}
	if err := mgr.Authorize(ctxEditor, "createUser"); err != nil {
		t.Errorf("editor should be authorized for createUser: %v", err)
	}
}
