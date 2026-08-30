# Enterprise MCP Gateway - Architecture & Technical Specification

**Document Version:** 1.0.0  
**Date:** 2026-08-29  
**Status:** Approved for Implementation  
**Target:** Production-Grade High-Performance MCP Gateway in Go  

---

## 1. Executive Summary & Objective

The **Enterprise Model Context Protocol (MCP) Gateway** is a single-binary, high-performance Go application designed to serve as a hardened, auditable bridge between AI clients/agents (Claude Desktop, Cursor, Gemini CLI, LangGraph/Temporal autonomous workers) and enterprise backends (Java/Spring Boot, Go microservices, relational databases).

### Key Architectural Pillars
1. **Single Static Binary:** Minimal memory footprint (<25MB resident set size), sub-millisecond dispatch overhead, zero external runtime dependencies.
2. **Strict Protocol Compliance:** Full JSON-RPC 2.0 & Anthropic MCP (protocol version `2024-11-05`) support across `stdio` and HTTP `SSE` transports.
3. **Role-Based Tool Governance (RBAC):** Least-privilege tool advertising and execution enforcement based on client identity/API tokens.
4. **Automated OpenAPI Connector:** Zero-code bridging of legacy REST/Swagger services into validated MCP tools.
5. **Fast PII/DLP Sanitization:** Zero-allocation stream & JSON-key masking of sensitive data (SSNs, cards, credentials, custom patterns) prior to sending output to LLMs.
6. **Enterprise Audit Trail:** Immutable structured JSON logging with cryptographic input/output hashing and SIEM-ready schemas.

---

## 2. System Architecture & Component Diagram

```
┌─────────────────────────────────────────────────────────────────────────┐
│                      AI Clients / Agent Frameworks                      │
│            (Claude Desktop / Cursor / Gemini CLI / LangGraph)           │
└────────────────────────────────────┬────────────────────────────────────┘
                                     │
                 JSON-RPC 2.0 over Stdio OR HTTP Server-Sent Events (SSE)
                                     │
                                     ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                        Enterprise MCP Gateway                           │
│                                                                         │
│  ┌───────────────────────────────────────────────────────────────────┐  │
│  │ 1. Transport Layer (pkg/mcp/transport)                            │  │
│  │    - Stdio Transceiver (line-delimited JSON-RPC)                  │  │
│  │    - HTTP SSE Transceiver (Session multiplexer + POST endpoint)   │  │
│  └─────────────────────────────────┬─────────────────────────────────┘  │
│                                    │                                    │
│  ┌─────────────────────────────────▼─────────────────────────────────┐  │
│  │ 2. Governance & Auth Middleware (pkg/governance/rbac)             │  │
│  │    - Token / Identity Resolver                                    │  │
│  │    - Role Filter for `tools/list` (tool visibility gating)        │  │
│  │    - Permission Enforcement for `tools/call`                      │  │
│  └─────────────────────────────────┬─────────────────────────────────┘  │
│                                    │                                    │
│  ┌─────────────────────────────────▼─────────────────────────────────┐  │
│  │ 3. Core Protocol Router & Tool Registry (pkg/mcp/protocol)        │  │
│  │    - JSON-RPC Dispatcher (`initialize`, `tools/list`, etc.)      │  │
│  │    - Tool Schema Registry & Argument Validation                   │  │
│  └─────────────────────────────────┬─────────────────────────────────┘  │
│                                    │                                    │
│  ┌─────────────────────────────────▼─────────────────────────────────┐  │
│  │ 4. Backend Connector Dispatcher (pkg/connector/openapi)           │  │
│  │    - OpenAPI 3.0 / Swagger Dynamic Parser                         │  │
│  │    - HTTP REST Invoker (Path/Query/Header/Body mapping)          │  │
│  │    - Connection Pooling, Circuit Breaker, Retries & Timeouts      │  │
│  └─────────────────────────────────┬─────────────────────────────────┘  │
│                                    │ Raw Response                       │
│  ┌─────────────────────────────────▼─────────────────────────────────┐  │
│  │ 5. PII / DLP Sanitization Engine (pkg/sanitizer/pii)              │  │
│  │    - Regex Matchers (Credit Cards with Luhn, SSN, Emails, Tokens) │  │
│  │    - JSON Key Redactor (`password`, `secret`, `token`, etc.)      │  │
│  │    - In-place stream masking                                      │  │
│  └─────────────────────────────────┬─────────────────────────────────┘  │
│                                    │ Sanitized Output                   │
│  ┌─────────────────────────────────▼─────────────────────────────────┐  │
│  │ 6. Structured Audit Logger (pkg/audit/logger)                     │  │
│  │    - JSON event stream with caller ID, tool name, hash, status    │  │
│  └─────────────────────────────────┬─────────────────────────────────┘  │
└────────────────────────────────────┼────────────────────────────────────┘
                                     │
                     Authorized & Upstream HTTP/REST
                                     │
                                     ▼
┌─────────────────────────────────────────────────────────────────────────┐
│               Enterprise Backend Services & Microservices               │
│               (Java/Spring Boot, Go, Node.js, Postgres)                 │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## 3. Detailed Component Specifications

### 3.1 `pkg/mcp/protocol` & `pkg/mcp/transport`
- **Protocol:** Anthropic MCP Specification (`2024-11-05`).
- **JSON-RPC Core:**
  - Standard JSON-RPC 2.0 framing (`jsonrpc: "2.0"`, `id`, `method`, `params`, `result`, `error`).
  - Standard Error Codes: `-32700` (Parse error), `-32600` (Invalid Request), `-32601` (Method not found), `-32602` (Invalid params), `-32603` (Internal error).
- **Core MCP Methods Supported:**
  - `initialize`: Client/server capability negotiation, protocol version verification, server metadata (`name: "enterprise-mcp-gateway"`, `version: "1.0.0"`).
  - `notifications/initialized`: Client acknowledgment.
  - `ping`: Health check.
  - `tools/list`: Return array of available tools filtered by caller's RBAC role.
  - `tools/call`: Execute a tool by name with parameter validation against JSON schema.
- **Transports:**
  - `StdioTransport`: Non-blocking line scanner reading from `io.Reader`, synchronous/safe mutex-protected writes to `io.Writer`.
  - `SSETransport`: HTTP handler supporting `GET /sse` (establishes SSE event stream with an endpoint URL containing `sessionId`) and `POST /message?sessionId=<id>` for incoming JSON-RPC payloads.

### 3.2 `pkg/governance/rbac`
- **Identity Model:**
  - Clients provide identity via Bearer Token in HTTP header, CLI flag (`--client-token`), or environment variable.
  - Token is mapped to a Role (e.g. `admin`, `support_agent`, `analytics_readonly`, `unauthenticated`).
- **Policy Engine:**
  - Configurable policy mapping roles to tool patterns (supports wildcards `*`, exact match, or regex).
  - `FilterTools(role, allTools) []Tool`: Enforces least-privilege tool advertising.
  - `Authorize(role, toolName) error`: Denies execution if client role lacks permission, returning a structured JSON-RPC error.

### 3.3 `pkg/connector/openapi`
- **Spec Parser:**
  - Ingests OpenAPI 3.0.x / 3.1.x and Swagger 2.0 specs in YAML or JSON format (local file path or remote URL).
  - Converts endpoints (`operationId` or `METHOD_path`) into MCP `Tool` structures with JSON Schema validation (`type: "object"`, `properties`, `required`).
- **Dynamic Invoker:**
  - Resolves path templating (`/v1/users/{id}` -> `/v1/users/123`).
  - Formats query parameters (`/v1/search?q=foo`).
  - Formats JSON request bodies for `POST`/`PUT`/`PATCH`.
  - Configurable Upstream Headers (e.g. `Authorization: Bearer <internal_token>`).
  - Timeout and retry handling via Go standard `net/http` with Keep-Alive connection pooling.

### 3.4 `pkg/sanitizer/pii`
- **Inspection Rules:**
  - **Payment Cards:** Master, Visa, Amex, Discover pattern matching + Luhn algorithm checksum validation.
  - **US Social Security Numbers (SSN):** `\b\d{3}-\d{2}-\d{4}\b`.
  - **Email Addresses:** Standard RFC 5322 matching.
  - **Phone Numbers:** International E.164 and North American Numbering Plan.
  - **Secrets/Tokens:** Bearer tokens, private keys (`-----BEGIN PRIVATE KEY-----`), AWS API keys (`AKIA[0-9A-Z]{16}`), GitHub PATs (`ghp_[A-Za-z0-9]{36}`).
  - **JSON Key Redaction:** Configurable key names (e.g. `password`, `secret`, `token`, `ssn`, `credit_card`) replaced with `"[REDACTED]"`.
- **Performance:** Pre-compiled regex patterns, streaming buffer replacements without unnecessary heap allocations.

### 3.5 `pkg/audit/logger`
- **Log Schema (JSON):**
  ```json
  {
    "timestamp": "2026-08-29T22:00:00.000Z",
    "request_id": "req-986426e5",
    "caller_id": "support-agent-01",
    "role": "support_agent",
    "tool": "getUserBillingDetails",
    "input_sha256": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
    "duration_ms": 42,
    "status": "SUCCESS",
    "pii_redacted_count": 2,
    "error": null
  }
  ```
- **Guarantees:** Non-blocking async channel or buffered synchronous writing with zero external telemetry dependencies.

---

## 4. Configuration Schema (`config.yaml`)

```yaml
server:
  name: "enterprise-mcp-gateway"
  version: "1.0.0"
  transport: "stdio" # "stdio" or "sse"
  port: 8080
  host: "0.0.0.0"

governance:
  enabled: true
  tokens:
    "agent-readonly-secret": "readonly_agent"
    "agent-support-secret": "support_agent"
    "admin-master-secret": "admin"
  roles:
    readonly_agent:
      allowed_tools:
        - "get*"
        - "list*"
        - "search*"
    support_agent:
      allowed_tools:
        - "get*"
        - "list*"
        - "search*"
        - "updateTicketStatus"
        - "resetUserPassword"
    admin:
      allowed_tools:
        - "*"

sanitizer:
  enabled: true
  mask_card_numbers: true
  mask_ssn: true
  mask_emails: false
  mask_phone_numbers: false
  mask_secrets: true
  custom_regex:
    - name: "Internal Employee ID"
      pattern: "\\bEMP-[0-9]{6}\\b"
      replacement: "[REDACTED-EMP-ID]"
  sensitive_keys:
    - "password"
    - "passwd"
    - "secret"
    - "token"
    - "apiKey"
    - "ssn"
    - "creditCard"

audit:
  enabled: true
  log_path: "stdout" # or "/var/log/mcp-gateway/audit.log"
  hash_inputs: true

connectors:
  - name: "enterprise-crm"
    type: "openapi"
    spec_file: "./examples/crm-openapi.yaml"
    base_url: "http://localhost:8081"
    headers:
      Authorization: "Bearer backend-internal-token"
      X-Gateway-Source: "enterprise-mcp-gateway"
```

---

## 5. Directory Structure

```
enterprise-mcp-gateway/
├── cmd/
│   ├── gateway/
│   │   └── main.go
│   └── mockserver/
│       └── main.go
├── docs/
│   ├── specs/
│   │   └── 01-architecture-spec.md
│   └── plans/
│       └── implementation-plan.md
├── examples/
│   ├── config.yaml
│   └── crm-openapi.yaml
├── pkg/
│   ├── audit/
│   │   ├── logger.go
│   │   └── logger_test.go
│   ├── config/
│   │   ├── config.go
│   │   └── config_test.go
│   ├── connector/
│   │   └── openapi/
│   │       ├── parser.go
│   │       ├── parser_test.go
│   │       ├── invoker.go
│   │       └── invoker_test.go
│   ├── governance/
│   │   └── rbac/
│   │       ├── rbac.go
│   │       └── rbac_test.go
│   ├── mcp/
│   │   ├── protocol/
│   │   │   ├── types.go
│   │   │   ├── server.go
│   │   │   └── server_test.go
│   │   └── transport/
│   │       ├── stdio.go
│   │       ├── stdio_test.go
│   │       ├── sse.go
│   │       └── sse_test.go
│   └── sanitizer/
│       └── pii/
│           ├── sanitizer.go
│           └── sanitizer_test.go
├── test/
│   └── integration/
│       └── gateway_e2e_test.go
├── Dockerfile
├── go.mod
├── go.sum
└── README.md
```
