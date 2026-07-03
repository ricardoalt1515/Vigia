# Archive Report: Issue #17 Remote MCP Server (tenant-scoped external AI-client integration)

## Executive summary

Issue #17 — the first Remote MCP Server slice — is fully planned, implemented,
and merged to `main`; GitHub issue #17 is closed. This change is archived.

## Change

`issue-17-remote-mcp-server`

## What was built

A local, external MCP-compatible adapter, kept strictly separate from the
internal Harness runtime:

- `cmd/vigia-mcp` — local stdio JSON-RPC command entrypoint (newline-delimited
  JSON-RPC on stdin, one response per line on stdout).
- `internal/mcp` — the adapter package: request/response types (`server.go`,
  `types.go`), a tenant-aware in-memory synthetic artifact index
  (`index.go`), a read-only permission gate (`permissions.go`), an in-memory
  bounded audit sink (`audit.go`), a static bearer authenticator for the
  local command (`static_auth.go`), and stdio transport wiring (`stdio.go`).
- Two read-only tools: `read_case_brief` and `read_evidence_manifest`, both
  returning redacted DTOs (no raw transcript text, no evidence-ledger
  mutation).
- `initialize`, `tools/list`, `tools/call` JSON-RPC methods; draft/authority
  tools are not present in the first-slice catalog.
- Tenant/API-key identity is resolved before any case/artifact lookup;
  missing/invalid auth fails closed before lookup.
- Cross-tenant, unknown, and stale case/artifact lookups all return the same
  external denied/not-found shape; audit records retain the precise internal
  reason without exposing it externally.
- Every call emits a bounded, non-PII audit record (call ID, tenant ID,
  actor/key ID, tool name, schema versions, input hash, case ID, policy
  decision, redaction profile, outcome, bounded error class) with no raw API
  keys, transcripts, or PII.
- Prompt-injection-shaped client content is treated as untrusted data;
  outputs remain redacted regardless of embedded instructions.
- `docs/remote-mcp.md` documents local usage, exposed tools, auth metadata,
  unavailable data, and explicit non-goals.

## Verification / completion evidence

No formal `verify-report.md` was produced for this change in
`openspec/changes/issue-17-remote-mcp-server/` prior to archive (unlike
`issue-4-llm-judge-tone-threat`, which has one). Completion evidence instead
comes from `apply-progress.md` (TDD RED/GREEN/REFACTOR evidence, changed
files, verification commands) plus the merged/closed state of the change:

- All "First slice tasks" sections (1-6) in `tasks.md` are checked complete
  (0 unchecked implementation tasks). The "Follow-up tasks outside first
  slice" section is explicitly out-of-scope future work (remote HTTP/SSE
  transport, DB-backed index, draft/write tools, richer docs), not deferred
  first-slice work — consistent with the follow-up convention used by
  `issue-4-llm-judge-tone-threat`.
- `apply-progress.md` records:
  - RED: `go test ./internal/mcp -count=1` failed at compile time before the
    adapter existed.
  - GREEN: `go test ./internal/mcp ./cmd/vigia-mcp -count=1` passed after
    implementation.
  - Full suite: `go test ./... -count=1` passed.
  - A fresh review found two blockers (JSON-RPC notifications incorrectly
    emitting error responses, and invalid raw `case_id` values reaching audit
    records before validation), both fixed with regression tests before the
    change was considered complete.
- The code is present on `main`: `internal/mcp` (audit.go, index.go,
  permissions.go, server.go, server_test.go, static_auth.go, stdio.go,
  types.go) and `cmd/vigia-mcp` (main.go, main_test.go) both exist in the
  repository, matching the "Verify claims against the actual code" check
  requested for this archive pass.
- GitHub issue #17 is closed.

## Specs merged

`openspec/changes/issue-17-remote-mcp-server/specs/remote-mcp-server/spec.md`
was a full new spec (no prior `openspec/specs/remote-mcp-server/spec.md`
existed), so it was copied directly (not delta-merged) to:

- `openspec/specs/remote-mcp-server/spec.md` — 6 requirements, 12 scenarios
  covering the local MCP entrypoint, tenant authentication before lookup,
  the tenant-aware synthetic artifact index and rejection of raw paths/tenant
  overrides, the two safe read-only tools, the permission/audit envelope, and
  prompt-injection-as-data handling.

## Archive contents

- `proposal.md` copied
- `design.md` copied
- `tasks.md` copied (52 first-slice tasks complete; 5 explicitly out-of-scope
  follow-up tasks correctly left unchecked, annotated with an archive note)
- `apply-progress.md` copied
- `specs/remote-mcp-server/spec.md` copied
- `archive-report.md` (this file)

Archived to:
`openspec/changes/archive/2026-07-03-issue-17-remote-mcp-server/`

## Filesystem limitation (risk — needs follow-up)

This archive pass was executed by an agent with **no shell/Bash tool
available** — only Read/Edit/Write/Glob and Engram memory tools. As a
result:

- The archive folder contents above were **written as new files** at the
  archive path (copies of the source content), NOT moved via `git mv`/`mv`.
- The original `openspec/changes/issue-17-remote-mcp-server/` directory
  **could not be deleted** and still exists on disk alongside the new
  archive copy. It must be removed (`git rm -r
  openspec/changes/issue-17-remote-mcp-server/`) by an agent or human with
  shell access before this is committed, to avoid a duplicate change folder
  in the repo.
- No `git add`/`git commit`/`git push` was performed for this archive work.
  A shell-capable agent (or the user) must run:
  ```
  git rm -r openspec/changes/issue-17-remote-mcp-server/
  git add openspec/specs/remote-mcp-server/spec.md \
          openspec/changes/archive/2026-07-03-issue-17-remote-mcp-server/
  git commit -m "docs(issue-17): archive remote-mcp-server change and merge spec"
  git push origin main
  ```

## Follow-ups

- Real remote HTTP/SSE transport for target external clients (currently
  local stdio JSON-RPC only).
- Database-backed tenant artifact index (currently in-memory synthetic).
- Production API-key store integration (currently a static local bearer
  authenticator).
- Draft/write tools returning `approval_required` or non-mutating drafts.
- Richer per-target-AI-client setup documentation.

## SDD cycle status

Planned (proposal/design/tasks) → Implemented (apply, TDD evidence in
apply-progress.md) → Merged to `main`, issue closed → **Archived** (this
report), pending the filesystem cleanup + git commit/push noted above by a
shell-capable agent.
