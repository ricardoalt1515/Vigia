# Archive Report: Issue #14 Tenant Auth to RLS Tenant Context

## Executive summary

Issue #14 — the first runtime tenant boundary (API-key authentication ->
transaction-scoped PostgreSQL RLS) — is fully planned, implemented across 3
chained PRs, and merged to `main`; GitHub issue #14 is closed. This change is
archived.

## Change

`issue-14-tenant-auth-rls-context`

## What was built

A minimal, reviewable runtime tenant boundary built on top of the issue #13
schema/RLS foundation, delivered as 3 stacked PRs:

- **PR 1 — auth core**: `internal/auth` parses `Authorization: Bearer <key>`,
  hashes presented keys (SHA-256) before lookup, and resolves an active key
  to exactly one tenant. `internal/tenantdb` provides a transaction helper
  that sets `app.tenant_id` via `SELECT set_config('app.tenant_id', $1,
  true)` (a deviation from the design's literal `SET LOCAL` syntax, chosen
  because parameterized `set_config` is safer and gives equivalent
  transaction-local scoping — documented and confirmed correct) inside
  `BeginTx`, and clears on commit/rollback.
- **PR 2 — protected endpoint + RLS proof**: `cmd/api` and `internal/httpapi`
  wire a minimal `GET /v1/interactions` endpoint returning `401` for
  unauthorized credentials and `200` with only the authenticated tenant's
  rows for valid credentials. `db/queries`/`internal/db`/`internal/postgres`
  add a current-tenant interactions read that intentionally omits an
  explicit `tenant_id` predicate, relying on PostgreSQL RLS for isolation.
  An integration test (`internal/db/rls_isolation_test.go`) proves tenant A
  cannot read tenant B rows; it skips safely without `DATABASE_URL` +
  `APP_DATABASE_URL`, avoiding mutation of unavailable services.
- **PR 3 — key issuance**: `cmd/seed` issues high-entropy tenant API keys,
  persists only the hash, and prints plaintext once to stdout for local/dev
  setup.

## Verification / completion evidence

No formal `verify-report.md` was produced for this change in
`openspec/changes/issue-14-tenant-auth-rls-context/` prior to archive
(unlike `issue-4-llm-judge-tone-threat`, which has one). Completion evidence
instead comes from `apply-progress.md` and the merged/closed state:

- `tasks.md`: all "Blockers / stop conditions", PR 1, PR 2, PR 3, and "Final
  validation" checkboxes are `[x]` — 0 unchecked implementation tasks.
- `apply-progress.md`'s "Remaining tasks" section explicitly states: "No
  unchecked implementation tasks remain in `tasks.md` for issue #14."
- TDD cycle evidence table records RED (compile failures on undefined
  symbols: `HashAPIKey`, `WithTenantTx`, `Interaction`,
  `ListCurrentTenantInteractions`, seed issuer types) followed by GREEN
  (implementation) for each of: auth, tenant transaction context, HTTP
  interactions endpoint, RLS current-tenant read, and key issuance/seed.
- Verification commands recorded as passing: focused package tests per PR,
  `go test ./internal/auth ./internal/tenantdb ./internal/httpapi
  ./cmd/seed -count=1`, full `go test ./...`, and `git diff --check`.
- The code is present on `main`: `internal/auth`, `internal/tenantdb`,
  `internal/httpapi`, `internal/postgres`, `cmd/api`, `cmd/seed` all exist in
  the repository, matching the "Verify claims against the actual code"
  check requested for this archive pass.
- Session-memory records (Engram #5504, #5509, #5510) confirm three separate
  commits: `1f50e25` (PR1 auth core), `84854a2` (PR2 RLS endpoint), `b65803b`
  (PR3 key issuance).
- GitHub issue #14 is closed.

## Specs merged

`openspec/changes/issue-14-tenant-auth-rls-context/specs/tenant-auth-rls-context/spec.md`
was a full new spec (no prior `openspec/specs/tenant-auth-rls-context/spec.md`
existed). `openspec/specs/foundation-bootstrap/spec.md` (issue #13) was
checked for overlap: it explicitly scopes out runtime auth/RLS-session
behavior as issue #14's responsibility ("Runtime tenant isolation remains
issue #14" scenario), so there was no additive-merge conflict. The delta spec
was copied directly (not delta-merged) to:

- `openspec/specs/tenant-auth-rls-context/spec.md` — 6 requirements covering
  tenant API-key authentication, API-key secret hashing/one-time plaintext,
  transaction-scoped `SET LOCAL app.tenant_id` RLS context, the protected
  `GET /v1/interactions` endpoint, the RLS cross-tenant isolation proof, and
  the issue #13/#14/#17 dependency-and-scope boundary.

## Archive contents

- `explore.md` copied
- `proposal.md` copied
- `design.md` copied (with an added "Deviation note" documenting the
  `set_config`-vs-`SET LOCAL` implementation choice)
- `tasks.md` copied (all implementation tasks complete)
- `apply-progress.md` copied
- `specs/tenant-auth-rls-context/spec.md` copied
- `archive-report.md` (this file)

Archived to:
`openspec/changes/archive/2026-07-03-issue-14-tenant-auth-rls-context/`

## Filesystem limitation (risk — needs follow-up)

This archive pass was executed by an agent with **no shell/Bash tool
available** — only Read/Edit/Write/Glob and Engram memory tools. As a
result:

- The archive folder contents above were **written as new files** at the
  archive path (copies of the source content, with the design.md deviation
  note added), NOT moved via `git mv`/`mv`.
- The original `openspec/changes/issue-14-tenant-auth-rls-context/` directory
  **could not be deleted** and still exists on disk alongside the new
  archive copy. It must be removed (`git rm -r
  openspec/changes/issue-14-tenant-auth-rls-context/`) by an agent or human
  with shell access before this is committed, to avoid a duplicate change
  folder in the repo.
- No `git add`/`git commit`/`git push` was performed for this archive work.
  A shell-capable agent (or the user) must run:
  ```
  git rm -r openspec/changes/issue-14-tenant-auth-rls-context/
  git add openspec/specs/tenant-auth-rls-context/spec.md \
          openspec/changes/archive/2026-07-03-issue-14-tenant-auth-rls-context/
  git commit -m "docs(issue-14): archive tenant-auth-rls-context change and merge spec"
  git push origin main
  ```

## Follow-ups

- The live RLS cross-tenant isolation proof requires both `DATABASE_URL` and
  `APP_DATABASE_URL` to run as a real integration test; without both it
  skips rather than mutating unavailable services — a human/CI environment
  with both configured should periodically confirm this proof still runs
  green, not just compiles.
- Issue #17 (Remote MCP) already reuses this tenant-auth boundary concept
  (its own auth seam is `auth.TenantContext`-compatible per its
  apply-progress.md) — confirms the design's intended reuse worked in
  practice.
- Issue #1 (walking skeleton) can build the interaction-list console on top
  of this authenticated API without redesigning tenant auth, per the
  original success criteria.

## SDD cycle status

Planned (explore/proposal/design/tasks) → Implemented (apply, 3 chained PRs
with TDD evidence in apply-progress.md) → Merged to `main`, issue closed →
**Archived** (this report), pending the filesystem cleanup + git
commit/push noted above by a shell-capable agent.
