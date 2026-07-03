# Apply Progress: issue-17-remote-mcp-server

## Implementation discipline

**Objective:** Implement the first reviewable issue #17 slice: local MCP-compatible JSON-RPC adapter, `initialize`, `tools/list`, `tools/call`, two tenant-scoped read-only synthetic artifact tools, permission/audit envelope, redacted outputs, and fail-closed validation.

**Non-goals:** No HTTP/SSE transport, no external MCP dependency, no database-backed artifact index, no draft/write execution, no authority-bearing actions, no evidence-ledger mutation, no commits/pushes/PRs, and no cleanup of unrelated `.pi/` or `triage/` files.

**Required patterns:** Strict TDD, behavior-focused Go tests, stdlib JSON-RPC handling, external adapter boundary separate from the internal Harness runtime, existing `auth.TenantContext` and `harness.PermissionDecision` concepts, tenant lookup before artifact lookup, fail-closed validation, bounded audit metadata, and redacted output DTOs.

**Forbidden antipatterns:** Raw artifact paths, client-provided tenant IDs, broad tool catalogs, raw database selectors, raw transcript/PII/API-key leakage, prompt-only security controls, hidden side effects, MCP replacing internal Harness runtime, and external dependencies for the first slice.

**Verification command:** `go test ./internal/mcp ./cmd/vigia-mcp -count=1` and `go test ./... -count=1`.

**Stop conditions:** Stop if the implementation requires an external MCP dependency choice, DB-backed artifact index, HTTP/SSE transport, draft/write semantics, external services in tests, or a scope increase beyond the first slice.

## TDD evidence

### RED

- Added behavior tests in `internal/mcp/server_test.go` before production MCP adapter code.
- Initial focused run failed as expected because `NewServer`, `Config`, `MemoryAuditSink`, request/response types, index types, and error constants were undefined.

Command:

```bash
go test ./internal/mcp -count=1
```

Result: failed at compile time with missing MCP adapter symbols.

### GREEN

Implemented the minimal local MCP adapter:

- `internal/mcp` JSON-RPC request/response handling.
- `initialize`, `tools/list`, and `tools/call` handlers.
- `read_case_brief` and `read_evidence_manifest` read-only tools.
- Tenant-aware synthetic index over existing lab fixtures.
- Authenticator interface compatible with `auth.TenantContext` and a local static bearer authenticator for the command.
- Read-only permission gate.
- In-memory bounded audit sink.
- Redacted output DTOs that omit raw transcript text by default.
- Fail-closed validation for missing/invalid auth, unknown/forbidden tools, raw artifact path fields, tenant override fields, and tenant-mismatched/unknown case lookups.

Focused command:

```bash
go test ./internal/mcp ./cmd/vigia-mcp -count=1
```

Result: passed.

Full command:

```bash
go test ./... -count=1
```

Result: passed.

### TRIANGULATE / REFACTOR

- Added command smoke coverage in `cmd/vigia-mcp/main_test.go` to prove the stdio command handles `initialize` and `tools/list` without exposing draft tools.
- Manual security review adjusted `tools/call` ordering so client arguments are validated before they are handed to the permission gate.
- Fresh review found two blockers: JSON-RPC notifications emitted error responses, and invalid raw `case_id` values could be copied into audit records before validation.
- Added regression tests for notification no-response behavior and invalid/oversized/PII-shaped `case_id` audit non-leakage.
- Fixed stdio notification handling and constrained external `case_id` shape before audit persistence.
- Ran `gofmt` and `git diff --check`.
- Re-ran focused and full tests after formatting and review fixes.

## Changed files

- `cmd/vigia-mcp/main.go`
- `cmd/vigia-mcp/main_test.go`
- `docs/remote-mcp.md`
- `internal/mcp/audit.go`
- `internal/mcp/index.go`
- `internal/mcp/permissions.go`
- `internal/mcp/server.go`
- `internal/mcp/server_test.go`
- `internal/mcp/static_auth.go`
- `internal/mcp/stdio.go`
- `internal/mcp/types.go`
- `openspec/changes/issue-17-remote-mcp-server/apply-progress.md`
- `openspec/changes/issue-17-remote-mcp-server/design.md`
- `openspec/changes/issue-17-remote-mcp-server/proposal.md`
- `openspec/changes/issue-17-remote-mcp-server/specs/remote-mcp-server/spec.md`
- `openspec/changes/issue-17-remote-mcp-server/tasks.md`

## Deferred follow-ups

- Real remote HTTP/SSE transport for target external clients.
- Database-backed tenant artifact index.
- Integration with the production API-key store for non-synthetic deployments.
- Draft/write tools that return `approval_required` or non-mutating draft outputs.
- Richer per-client setup documentation.
- SDD verify/archive after review.

## Residual risks

- The command uses a local static bearer authenticator for the synthetic first slice; production remote deployment still needs a DB-backed authenticator/store wiring.
- The first-slice adapter is MCP-compatible JSON-RPC for local stdio, not a full remote HTTP/SSE MCP transport.
- Review workload is high because this slice adds tests, adapter code, docs, and SDD artifacts together.
