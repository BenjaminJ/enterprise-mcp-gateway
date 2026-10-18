---
name: enterprise-mcp-gateway
description: Use when connecting AI coding agents (Claude, Cursor, Copilot, Gemini CLI, Antigravity) to internal REST or OpenAPI services, especially when enterprise policies require RBAC tool permissions, PII/secret masking, or audit logging.
---

# Enterprise MCP Gateway

## Overview
The Enterprise MCP Gateway bridges AI coding agents to internal REST/OpenAPI services through a single, secure Go binary implementing the Model Context Protocol (MCP).

**Core Principle:** Expose internal enterprise APIs as governed, secure MCP tools with token-based RBAC, zero-alloc in-flight PII redaction, and immutable JSON audit logging.

---

## When to Use
- Connecting AI agents (Claude, Cursor, Copilot, Antigravity) to internal REST or OpenAPI APIs.
- Enforcing granular, tool-level permissions per agent or team via RBAC tokens.
- Masking sensitive data (credit cards via Luhn check, SSNs, secrets, API keys) before responses reach the LLM.
- Emitting tamper-resistant JSON audit logs for enterprise security and SIEM compliance.
- Running MCP locally via `stdio` (IDE/CLI agents) or centrally via `SSE` (shared server across teams).

### When NOT to Use
- Connecting to native MCP servers that do not require an OpenAPI/REST bridge or enterprise governance.
- Calling external public APIs without compliance, RBAC, or PII redaction requirements.
- Backends using GraphQL or gRPC (the gateway currently supports OpenAPI 3.0 REST specifications).

---

## Quick Reference

| Flag / Option | Default | Description |
|---|---|---|
| `--config <path>` | `config.yaml` | Path to gateway YAML configuration file |
| `--token <key>` | `""` | Caller token used for RBAC role mapping in `stdio` mode |
| `--transport <mode>` | `stdio` | Transport protocol: `stdio` or `sse` |
| `--port <int>` | `8080` | HTTP listen port for SSE mode |
| `--host <string>` | `0.0.0.0` | HTTP listen address for SSE mode |

*Full template and options:* [reference/config.example.yaml](reference/config.example.yaml)  
*Automated installer:* [scripts/install.sh](scripts/install.sh)

---

## Client Configurations

### Claude Code CLI
Add to your project's `.mcp.json` or run:
```bash
claude mcp add enterprise-gateway -- /absolute/path/to/bin/mcp-gateway --config /absolute/path/to/config.yaml --token agent-support-key
```

### Claude Desktop (`claude_desktop_config.json`) / Cursor (`.cursor/mcp.json`)
```json
{
  "mcpServers": {
    "enterprise-gateway": {
      "command": "/absolute/path/to/bin/mcp-gateway",
      "args": [
        "--config", "/absolute/path/to/config.yaml",
        "--token", "agent-support-key"
      ]
    }
  }
}
```

### Antigravity & Gemini CLI
Add to `~/.gemini/antigravity-cli/mcp/enterprise-gateway.json`:
```json
{
  "command": "/absolute/path/to/bin/mcp-gateway",
  "args": [
    "--config", "/absolute/path/to/config.yaml",
    "--token", "agent-support-key"
  ]
}
```

### Remote Server (SSE Mode)
Start the gateway:
```bash
./bin/mcp-gateway --config config.yaml --transport sse --port 8080
```
Agents connect to: `http://<host>:8080/sse?token=agent-support-key`

---

## Common Mistakes

| Problem | Root Cause | Fix |
|---|---|---|
| **MCP handshake hangs / protocol error** | Audit logging set to `stdout` in `stdio` mode, polluting JSON-RPC stream | Set `audit.log_path: "stderr"` or a file path in `config.yaml`. |
| **"Failed to load OpenAPI connector"** | `spec_file` uses a relative path not resolvable from agent CWD | Always use absolute paths (e.g. `/opt/mcp/crm-openapi.yaml`) in `config.yaml`. |
| **Agent sees 0 tools or gets 403 Forbidden** | Token is missing or maps to a role without tool permissions | Verify `--token` matches a key in `governance.tokens` and role's `allowed_tools` glob matches the tool names. |
| **Tool call returns connection refused** | Backend REST server is down or `base_url` has wrong port/scheme | Verify upstream service health (e.g. `curl http://127.0.0.1:8081/health`) and check `connectors[].base_url`. |
| **PII unmasked in tool response** | Field name not in `sensitive_keys` or format does not match regex | Enable `mask_secrets: true` and add custom patterns under `sanitizer.custom_regex`. |
