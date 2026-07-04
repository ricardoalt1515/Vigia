# Archive Report: Issue #6 Versioned, Immutable PolicyBundle with Reproducible Evaluations

## Executive summary

Issue #6 — versioned, immutable `PolicyBundle` snapshots with reproducible
evaluations — is fully planned, implemented, verified (PASS), merged to `main`
via PR #45 (squash commit `74af584`), and issue #6 is closed. This change is
archived.

## Change

`issue-6-policy-bundle`

## Commits (branch `feat/issue-6-policy-bundle`, squash-merged as `74af584`)

7 work-unit commits (Work Units 1–7 per `tasks.md`, `size:exception`
pre-approved single-PR delivery) plus 1 judgment-day fix commit:

- Work Unit 1 — Migration 00007 + sqlc regen + catalog test
- Work Unit 2 — Core types + BundleResolver seam + evaluation stamping
- Work Unit 3 — CreateBundleVersion + trigger/TRUNCATE/concurrency/RLS integration tests
- Work Unit 4 — ReEvaluateInteraction + reevaluate endpoint
- Work Unit 5 — Ledger golden-hash regression guard
- Work Unit 6 — Console surfaces the judging bundle version
- Work Unit 7 — `cmd/seed` compatibility check
- `e909044` — `fix(issue-6): address judgment-day findings` (round-1 and round-2 design fixes applied, post-design pre-verify)

Squash-merged to `main` as `74af584` via PR #45.

## Judgment-day outcome

A fresh-context adversarial review after design found **3 rounds of judgment**:

**Design phase (3 judgment-day rounds):**

1. (Round 1) **2 CRITICAL findings** on the original design:
   - No schema-level guarantee of single active bundle per tenant+name (only app-level intent).
   - Append-only requirement didn't block `TRUNCATE` (only `UPDATE`/`DELETE` on rows).
   - No restricted user role carve-out on the append-only triggers.

   Fixed by adding `policy_bundles_one_active_per_tenant_name` partial unique index (`WHERE status='active'`) and statement-level `BEFORE TRUNCATE` triggers on both tables with carve-out logic.

2. (Round 2) **1 CRITICAL finding**:
   - Partial unique indexes are non-deferrable in Postgres, so INSERT-before-UPDATE would violate the index on every ordinary rule edit (AC3 base case).

   Fixed by reversing `CreateBundleVersion` operation order: supersede-prior-FIRST-then-INSERT-new (both under `SELECT ... FOR UPDATE`, one transaction).

3. (Round 3) **Approved** — design fully conformant after rounds 1–2 fixes.

**Code phase (2 judgment-day rounds):**

1. (Round 1) **2 findings** (post-apply, pre-verify commit `e909044`):
   - Tenant check in `ReEvaluateInteraction` must precede pipeline execution, not after.
   - Resolver errors must be logged distinctly from the not-found case.

   Both fixed in commit `e909044`; tests added to verify the fixes.

2. (Round 2) **Approved** — all findings resolved.

## Verify verdict

**PASS** — 0 CRITICAL, 0 WARNING, 0 SUGGESTION. `go test ./... -count=1`
green against local docker-compose Postgres (22/22 packages, integration tests
ran for real, no short-mode skip). All 32 tasks verified against the diff. All
5 GitHub issue #6 acceptance criteria met. Both judgment-day round-1 code fixes
present and covered by dedicated tests. Full detail:
`openspec/changes/archive/2026-07-04-issue-6-policy-bundle/verify-report.md`.

## Specs merged

`openspec/changes/issue-6-policy-bundle/specs/policy-bundle/spec.md` was a
full new spec (no prior `openspec/specs/policy-bundle/spec.md` existed), so it
was copied directly (not delta-merged) to:

- `openspec/specs/policy-bundle/spec.md` — 6 requirements, ~18 scenarios
  covering the policy bundle versioning mechanism, append-only immutability
  with status-transition carve-out, effective-date/legal-basis tracking on
  rule snapshots, bundle lifecycle (draft→active→superseded), evaluation
  stamping with bundle version+FK, reproducible re-evaluation against
  historical bundle versions, and console list-page exposure of bundle version
  per interaction.

## Archive contents

- `proposal.md` ✅
- `design.md` ✅
- `tasks.md` ✅ (32/32 tasks complete, 0 unchecked)
- `specs/policy-bundle/spec.md` ✅
- `verify-report.md` ✅ (PASS verdict, full test/scenario mapping)
- `archive-report.md` ✅ (this file)

Archived to: `openspec/changes/archive/2026-07-04-issue-6-policy-bundle/`

## Engram observations (hybrid mode artifact store)

The following artifact observation IDs are recorded for traceability:

- Proposal: #5669
- Spec: #5670 (Judgment Day round-1 fixes: issue-6-policy-bundle spec)
- Design: #5672 (Round-2 judgment fix applied to Decision 6)
- Tasks: #5677
- Verify-report: #5681

## Filesystem limitation (risk — needs follow-up)

This archive pass was executed by an agent with **no shell/Bash tool
available** — only Read/Edit/Write/Glob and Engram memory tools. As a result:

- The archive folder contents above were **written as new files** at the
  archive path (copies of the source content), NOT moved via `git mv`/`mv`.
- The original `openspec/changes/issue-6-policy-bundle/` directory **could
  not be deleted** and still exists on disk alongside the new archive copy. It
  must be removed (`git rm -r
  openspec/changes/issue-6-policy-bundle/`) by an agent or human with shell
  access before this is committed, to avoid a duplicate change folder in the
  repo.
- No `git add`/`git commit`/`git push` was performed for this archive work. A
  shell-capable agent (or the user) must run:
  ```
  git rm -r openspec/changes/issue-6-policy-bundle/
  git add openspec/specs/policy-bundle/spec.md \
          openspec/changes/archive/2026-07-04-issue-6-policy-bundle/
  git commit -m "docs(issue-6): archive policy-bundle change and merge spec"
  git push origin main
  ```

## Follow-ups

- **Issue #7** (data-driven rule interpretation, extending `policy_bundle_rules`
  content to select detectors/judge rubric) is an explicit follow-up mentioned
  in this change's proposal and spec non-goals.
- The console surfaces the bundle version via the list column only; a detail
  page (AC5 scope boundary) was explicitly out of scope and remains a
  documented follow-up.
- The `BundleResolver` seam is injection-ready for multi-bundle-per-tenant
  use cases (rule tagging, tenant A vs tenant B isolation); single-active is
  the v1 policy, extensible without spec/code rework.

## SDD cycle status

Planned (proposal/spec/design/tasks) → Implemented (apply, 7 work-unit commits
+ 1 judgment-day fix commit) → Verified (PASS) → **Archived** (this report),
pending the filesystem cleanup + git commit/push noted above by a shell-capable
agent.
