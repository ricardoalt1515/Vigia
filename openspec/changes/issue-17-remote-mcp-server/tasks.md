# Tasks: Remote MCP Server tenant-scoped external AI-client integration

## Objective

Implement the first reviewable slice of issue #17: a local MCP-compatible JSON-RPC adapter with tenant-scoped read-only synthetic artifact tools, redacted outputs, permission envelope, and audit records.

## Implementation discipline

- Strict TDD is active.
- Write behavior-focused tests before production code where meaningful.
- Do not add tests that merely restate implementation.
- Do not commit, push, or open a PR during apply.
- Preserve unrelated untracked `.pi/` and `triage/` files.

## Review Workload Forecast

- Chained PRs recommended: Yes, for the full #17 scope.
- First apply slice: local stdio JSON-RPC adapter only.
- 400-line budget risk: High.
- Decision needed before apply: No. The first slice intentionally avoids external MCP dependencies to minimize risk and review size.
- Size exception may be needed if tests + adapter exceed 400 changed lines; keep follow-ups out.

## First slice tasks

### 1. RED: MCP tool catalog and initialize behavior

- [x] Add tests proving the server handles `initialize` without tenant lookup.
- [x] Add tests proving `tools/list` exposes only `read_case_brief` and `read_evidence_manifest`.
- [x] Add tests proving authority/draft tools are not executable in the first catalog.

### 2. RED: tenant auth and artifact lookup boundary

- [x] Add tests proving missing/invalid auth fails before case lookup.
- [x] Add tests proving tenant A can read its synthetic case brief.
- [x] Add tests proving tenant B requesting tenant A's case receives the same external denied/not-found shape as an unknown case.
- [x] Add tests proving raw artifact path and tenant override fields are rejected.

### 3. GREEN: implement minimal adapter package

- [x] Add an internal MCP adapter package with JSON-RPC request/response types.
- [x] Add an in-memory authenticator seam for tests and a production-facing interface compatible with existing tenant auth concepts.
- [x] Add a tenant-aware synthetic artifact index.
- [x] Add handlers for `initialize`, `tools/list`, and `tools/call`.
- [x] Add implementations for `read_case_brief` and `read_evidence_manifest` over synthetic data/contracts.

### 4. RED/GREEN: audit and redaction guarantees

- [x] Add tests proving every call emits audit metadata with bounded fields.
- [x] Add tests proving audit records exclude raw API keys, raw transcripts, and raw PII.
- [x] Add tests proving external case brief output omits raw transcript text by default.
- [x] Add tests proving prompt-injection-shaped client content is treated as data and does not bypass redaction/policy.

### 5. Command entrypoint and docs

- [x] Add a local command entrypoint for the MCP server if it can stay within the slice.
- [x] Add smoke tests for command/server behavior where practical.
- [x] Document local usage, exposed tools, auth metadata, unavailable data, and non-goals.

### 6. Verification and progress artifact

- [x] Run focused tests for the new MCP package/command.
- [x] Run `go test ./... -count=1`.
- [x] Write `openspec/changes/issue-17-remote-mcp-server/apply-progress.md` with TDD evidence, changed files, and deferred follow-ups.

## Follow-up tasks outside first slice

- [ ] Real remote HTTP/SSE transport if required by target external clients.
- [ ] Database-backed tenant artifact index.
- [ ] Draft/write tools returning `approval_required` or non-mutating drafts.
- [ ] Richer client setup docs per target AI client.
- [ ] Archive/sync specs after verification.
