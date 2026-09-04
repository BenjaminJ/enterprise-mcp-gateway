# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- GitHub Actions CI workflow with scheduled weekly test runs and multi-stage Docker build verification.
- Dependabot configuration for automated weekly Go modules, Docker, and GitHub Actions updates.
- CI status and last commit freshness badges to README.

## [1.0.0] - 2025-01-15

### Added
- Production-grade MCP Gateway with single static binary distribution.
- Role-Based Access Control (RBAC) engine for token-to-role resolution and tool filtering.
- High-performance zero-alloc PII, credit card (Luhn validation), and secret redaction engine.
- Dynamic OpenAPI 3.0 / Swagger connector for automated tool generation.
- Tamper-resistant structured JSON audit logger.
- Dual transport support: `stdio` and HTTP Server-Sent Events (`SSE`).
