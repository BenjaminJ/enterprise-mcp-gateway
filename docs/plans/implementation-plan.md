# Enterprise MCP Gateway Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build a production-ready, high-performance Go-based Model Context Protocol (MCP) gateway with zero external runtime dependencies, PII sanitization, RBAC governance, OpenAPI auto-connector, and structured audit logging.

**Architecture:** A modular pipeline where client requests arrive via Stdio or HTTP-SSE, pass through RBAC authentication and tool-filtering middleware, resolve upstream calls through a dynamic OpenAPI connector, get scrubbed for PII/secrets, and log structured cryptographic audit events.

**Tech Stack:** Go 1.24, JSON-RPC 2.0, MCP Specification `2024-11-05`, standard library (`net/http`, `crypto/sha256`, `regexp`), `gopkg.in/yaml.v3` for YAML parsing.

---

### Task 1: Protocol Types & Core MCP Server Dispatcher

**Files:**
- Create: `pkg/mcp/protocol/types.go`
- Create: `pkg/mcp/protocol/server.go`
- Test: `pkg/mcp/protocol/server_test.go`

**Step 1: Write the failing test**
Create `server_test.go` testing MCP `initialize`, `ping`, `tools/list`, and `tools/call` JSON-RPC requests.

**Step 2: Run test to verify it fails**
Run: `go test ./pkg/mcp/protocol/... -v`

**Step 3: Implement minimal protocol types and server dispatcher**
Implement standard MCP types (`JSONRPCRequest`, `JSONRPCResponse`, `InitializeParams`, `Tool`, `CallToolParams`, `CallToolResult`, `TextContent`) and `Server` struct.

**Step 4: Run test to verify it passes**
Run: `go test ./pkg/mcp/protocol/... -v`

**Step 5: Commit**
`git add pkg/mcp/protocol && git commit -m "feat(protocol): implement MCP core types and JSON-RPC dispatcher"`

---

### Task 2: Stdio and HTTP Server-Sent Events (SSE) Transports

**Files:**
- Create: `pkg/mcp/transport/stdio.go`
- Create: `pkg/mcp/transport/sse.go`
- Test: `pkg/mcp/transport/stdio_test.go`
- Test: `pkg/mcp/transport/sse_test.go`

**Step 1: Write the failing tests**
Test `StdioTransport` scanning line-delimited JSON-RPC messages and writing responses.
Test `SSETransport` `GET /sse` endpoint establishing stream and `POST /message?sessionId=...` dispatching calls.

**Step 2: Run test to verify it fails**
Run: `go test ./pkg/mcp/transport/... -v`

**Step 3: Implement Stdio and SSE Transports**
Implement thread-safe `StdioTransport` and session-multiplexing `SSETransport`.

**Step 4: Run test to verify it passes**
Run: `go test ./pkg/mcp/transport/... -v`

**Step 5: Commit**
`git add pkg/mcp/transport && git commit -m "feat(transport): implement Stdio and HTTP-SSE transports"`

---

### Task 3: RBAC Governance & Tool Visibility Filter

**Files:**
- Create: `pkg/governance/rbac/rbac.go`
- Test: `pkg/governance/rbac/rbac_test.go`

**Step 1: Write the failing test**
Test token-to-role resolution, wildcard/pattern-based tool authorization, and filtering of `tools/list` payloads.

**Step 2: Run test to verify it fails**
Run: `go test ./pkg/governance/rbac/... -v`

**Step 3: Implement RBAC Policy Engine**
Implement role definitions, pattern matching (`*`, prefix `get*`, exact), token lookup, and tool filtering.

**Step 4: Run test to verify it passes**
Run: `go test ./pkg/governance/rbac/... -v`

**Step 5: Commit**
`git add pkg/governance/rbac && git commit -m "feat(rbac): implement role-based access control and tool filtering"`

---

### Task 4: High-Performance PII & Secret Redaction Engine

**Files:**
- Create: `pkg/sanitizer/pii/sanitizer.go`
- Test: `pkg/sanitizer/pii/sanitizer_test.go`

**Step 1: Write the failing test**
Test masking of Credit Cards (with Luhn check), SSNs, emails, phone numbers, API keys/JWTs, custom regex rules, and recursive JSON key redaction.

**Step 2: Run test to verify it fails**
Run: `go test ./pkg/sanitizer/pii/... -v`

**Step 3: Implement PII Sanitizer**
Implement pre-compiled regex engine, Luhn algorithm, JSON walker, and stream replacement.

**Step 4: Run test to verify it passes**
Run: `go test ./pkg/sanitizer/pii/... -v`

**Step 5: Commit**
`git add pkg/sanitizer/pii && git commit -m "feat(sanitizer): implement PII and secret redaction engine"`

---

### Task 5: Structured JSON Audit Logger

**Files:**
- Create: `pkg/audit/logger.go`
- Test: `pkg/audit/logger_test.go`

**Step 1: Write the failing test**
Test emission of structured JSON audit records containing timestamps, caller identity, tool name, SHA-256 hashed inputs, execution duration, and status.

**Step 2: Run test to verify it fails**
Run: `go test ./pkg/audit/... -v`

**Step 3: Implement Structured Audit Logger**
Implement thread-safe, buffered audit logger with SHA-256 hashing and custom output writers.

**Step 4: Run test to verify it passes**
Run: `go test ./pkg/audit/... -v`

**Step 5: Commit**
`git add pkg/audit && git commit -m "feat(audit): implement structured JSON audit logger"`

---

### Task 6: OpenAPI 3.0 / Swagger Dynamic Connector & HTTP Invoker

**Files:**
- Create: `pkg/connector/openapi/parser.go`
- Create: `pkg/connector/openapi/invoker.go`
- Test: `pkg/connector/openapi/parser_test.go`
- Test: `pkg/connector/openapi/invoker_test.go`

**Step 1: Write the failing tests**
Test parsing OpenAPI YAML/JSON specs into MCP tools. Test mapping tool invocation arguments to HTTP path/query/headers/body and executing against a mock backend.

**Step 2: Run test to verify it fails**
Run: `go test ./pkg/connector/openapi/... -v`

**Step 3: Implement OpenAPI parser and invoker**
Implement YAML/JSON schema reader, tool converter, URL path interpolator, and HTTP client dispatcher.

**Step 4: Run test to verify it passes**
Run: `go test ./pkg/connector/openapi/... -v`

**Step 5: Commit**
`git add pkg/connector/openapi && git commit -m "feat(connector): implement dynamic OpenAPI tool parser and invoker"`

---

### Task 7: Configuration Management

**Files:**
- Create: `pkg/config/config.go`
- Test: `pkg/config/config_test.go`

**Step 1: Write the failing test**
Test loading configuration from YAML file and environment variable overrides.

**Step 2: Run test to verify it fails**
Run: `go test ./pkg/config/... -v`

**Step 3: Implement Config Loader**
Implement config parsing, default values, and validation.

**Step 4: Run test to verify it passes**
Run: `go test ./pkg/config/... -v`

**Step 5: Commit**
`git add pkg/config && git commit -m "feat(config): implement YAML config loader and validator"`

---

### Task 8: Gateway Main Entrypoint & Wiring

**Files:**
- Create: `cmd/gateway/main.go`
- Test: `test/integration/gateway_e2e_test.go`

**Step 1: Write E2E integration test**
Test end-to-end flow: config loading -> transport initialization -> mock backend invocation -> RBAC enforcement -> PII redaction -> audit log output.

**Step 2: Implement main.go CLI entrypoint**
Support flags (`--config`, `--transport`, `--port`, `--token`), graceful shutdown, signal handling, and runtime execution.

**Step 3: Run full test suite**
Run: `go test -v -race ./...`

**Step 4: Commit**
`git add cmd/gateway test/integration && git commit -m "feat(gateway): implement main gateway entrypoint and integration tests"`

---

### Task 9: Mock Enterprise Backend, Examples, Dockerfile & Documentation

**Files:**
- Create: `cmd/mockserver/main.go`
- Create: `examples/crm-openapi.yaml`
- Create: `examples/config.yaml`
- Create: `Dockerfile`
- Create: `README.md`

**Step 1: Implement Mock Server & OpenAPI Spec**
Implement a mock CRM REST service (Users, Billing, Tickets) with OpenAPI 3.0 spec.

**Step 2: Create sample configurations and Dockerfile**
Minimal multi-stage scratch/alpine container build.

**Step 3: Write comprehensive README.md**
Detailed setup guide, Inspector testing instructions, Claude Desktop config examples, and architecture documentation.

**Step 4: Verify complete build and test suite**
Run: `go build -o bin/mcp-gateway ./cmd/gateway` and `go test ./...`

**Step 5: Commit**
`git add . && git commit -m "docs: add mockserver, examples, Dockerfile, and comprehensive README"`
