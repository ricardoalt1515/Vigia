# Archive Report: Issue #7 Remaining Deterministic Detectors + Despacho/Cause Dashboards

## Executive summary

Issue #7 — the five remaining deterministic REDECO detectors, the despacho
registry, and the two compliance dashboards — is fully planned, implemented,
verified (PASS WITH WARNINGS, warnings since closed), merged to `main` across
6 stacked PRs (#47-#52), and GitHub issue #7 is closed. This change is
archived.

## Change

`issue-7-deterministic-detectors`

## What shipped

- **5 new pure, fail-closed detectors** in `internal/detection`: third-party
  contact (`MX-REDECO-06`, hard-block), protected population (`MX-REDECO-07`,
  hard-block + HITL), authorized channel (`MX-REDECO-11`, hard-block),
  payment routing (`MX-REDECO-10`, hard-block), disclosure presence
  (`MX-REDECO-03`, warn-level).
- **`ContactHoursDetector` rename**: `"contact-hours"` → `"MX-REDECO-04"`
  wiring code, with a one-way `detector_code` backfill in migration `00008`.
- **3-way outcome fold** in `internal/evaluation/service.go`
  (block/warn/pass), replacing the prior binary block/else fold, plus a
  `RequiresHITL` field on `NamedDetector` (true only for `MX-REDECO-07`).
- **Despacho registry**: `despachos` table (tenant-scoped, RLS, 1:N
  cardinality), `Despacho` Go type, nullable composite FK on
  `interaction_events`.
- **Rule catalog + bundle seeding**: `cmd/seed` now seeds all 7
  `policy_rules` (2 existing + 5 new) and one active `redeco-baseline`
  `policy_bundle_rules` snapshot, idempotently, establishing the first
  production seeding path for issue #6's versioned-bundle infrastructure.
- **Compliance dashboards**: `GET /v1/dashboards/by-despacho`
  (violation-rate ranking, interaction-grain, unattributed bucket, tie-break
  by name) and `GET /v1/dashboards/by-cause` (per-rule-code
  violations/warnings breakdown), both SQL aggregates under
  `tenantdb.WithTenantTx` + RLS.
- **Console pages**: `apps/console/src/app/dashboards/by-despacho/page.tsx`
  and `.../by-cause/page.tsx`, server-fetched, no client-side aggregation.

## Delivery

6 stacked PRs merged to `main`:

| PR | Scope |
|----|-------|
| #47 (PR1) | Despacho registry + detector-input schema |
| #48 (PR2a) | Third-party + protected-population detectors, 3-way fold, HITL |
| #49 (PR2b) | Authorized-channel + payment-routing detectors + judgment-day snapshot-plumbing fast-follow |
| #50 (PR2c) | Disclosure (warn) detector + contact-hours rename + 7-rule seeding + judgment-day fixture-determinism fast-follow |
| #51 (PR3) | API aggregate endpoints (by-despacho, by-cause) |
| #52 (PR4) | Console dashboards |

Two judgment-day fast-follows were absorbed mid-delivery (not left as
follow-up debt): a CRITICAL detector-input snapshot plumbing gap (PR2b) and a
fixture-timestamp non-determinism bug (PR2c) — both fixed and covered by
dedicated tests, documented in `tasks.md`.

## Verify verdict

**PASS WITH WARNINGS** at verify time (Engram `sdd/issue-7-deterministic-detectors/verify-report`,
observation #5716): 0 CRITICAL, 1 WARNING (integration-tagged scenarios —
despacho RLS/cardinality, dashboard SQL aggregates, seed idempotency —
existed and were correctly written/skip-gated but unexecuted locally, no
Postgres available at verify time), 0 SUGGESTION. Per user instruction at
archive time, a full `make test-db` run was subsequently executed against
live Postgres, closing out that WARNING (Engram observations #5716/#5717).
`go build ./...` clean; full test suite green; console `tsc --noEmit` and
`npm run build` both clean.

## Specs synced

| Domain | Action | Details |
|--------|--------|---------|
| `deterministic-detectors` | Created | Full new spec (no prior main spec existed) — 6 requirements, ~30 scenarios |
| `despacho-registry` | Created | Full new spec (no prior main spec existed) — 4 requirements, ~10 scenarios |
| `compliance-dashboards` | Created | Full new spec (no prior main spec existed) — 3 requirements, ~9 scenarios |
| `contact-hours-detector` | Modified | 1 requirement (`Contact-Hours Detector Is a Pure, Fail-Closed Function`) updated: registration code standardized to `MX-REDECO-04`, 1 scenario added (`Registration code uses the REDECO rule code`); all other requirements preserved unmodified |

Source of truth now updated at:
- `openspec/specs/deterministic-detectors/spec.md`
- `openspec/specs/despacho-registry/spec.md`
- `openspec/specs/compliance-dashboards/spec.md`
- `openspec/specs/contact-hours-detector/spec.md`

## Archive contents

- `proposal.md` (Y)
- `design.md` (Y)
- `tasks.md` (Y) (27/28 tasks complete; 1 intentionally unchecked — see below)
- `specs/deterministic-detectors/spec.md` (Y)
- `specs/despacho-registry/spec.md` (Y)
- `specs/compliance-dashboards/spec.md` (Y)
- `specs/contact-hours-detector/spec.md` (Y) (delta, as merged)
- `verify-report.md` (Y)
- `archive-report.md` (Y, this file)

Archived to: `openspec/changes/archive/2026-07-05-issue-7-deterministic-detectors/`

## Open item: task 4.4 (intentional partial, accepted)

Task 4.4 ("[manual-demo] Verify both pages render seeded demo-tenant data per
spec scenarios") remains unchecked in `tasks.md`. This is a **non-code
manual-demo gap**, not a stale checkbox or incomplete implementation:

- Both console dashboard pages compile cleanly (`tsc --noEmit`), build
  successfully (`npm run build`), and render as dynamic (ƒ), server-fetched
  routes with no client-side aggregation — verified at verify time.
- The remaining step is a human visually opening the pages against a running
  local dev environment with migrated + seeded Postgres and the API server,
  which is not performable in a non-interactive agent session.
- This gap is explicitly accepted and documented per the orchestrator's
  instruction at archive time (all 6 PRs merged, issue #7 closed, verify
  passed with only this manual-demo item outstanding). No code, test, or
  spec-conformance issue is being deferred — only the human click-through
  demo step.

Per the Strict-vs-OpenSpec Archive Policy, this is recorded here as an
intentional partial archive: task 4.4 blocks nothing structurally (it has no
code deliverable), and the orchestrator explicitly directed archiving to
proceed with this item documented as a non-code gap.

## Engram observations (hybrid mode artifact store)

The following artifact observation IDs are recorded for traceability:

- Proposal: #5692
- Spec (round-4 judgment-day fixes, deterministic-detectors + contact-hours-detector): #5693
- Design (round-4 judgment-day fixes): #5694
- Tasks: #5701 (Engram-side revision is stale — shows Phase 3/4 unchecked;
  the on-disk `tasks.md` reproduced in this archive, 27/28 checked, is
  authoritative and matches apply-progress and verify-report)
- Verify-report: #5716

## Filesystem limitation (risk — needs follow-up)

This archive pass was executed by an agent with **no shell/Bash tool
available** — only Read/Edit/Write/Glob and Engram memory tools, matching the
same limitation documented in the issue-6 archive
(`openspec/changes/archive/2026-07-04-issue-6-policy-bundle/archive-report.md`).
As a result:

- The archive folder contents above were **written as new files** at the
  archive path (copies of the source content), NOT moved via `git mv`/`mv`.
- The original `openspec/changes/issue-7-deterministic-detectors/` directory
  **could not be deleted** and still exists on disk alongside the new
  archive copy. It must be removed by an agent or human with shell access
  before this is committed, to avoid a duplicate change folder in the repo.
- No `git add`/`git commit`/`git push` was performed for this archive work
  (per explicit instruction: leave the working tree for the orchestrator to
  review and commit). A shell-capable agent (or the user) must run:
  ```
  git rm -r openspec/changes/issue-7-deterministic-detectors/
  git add openspec/specs/deterministic-detectors/ \
          openspec/specs/despacho-registry/ \
          openspec/specs/compliance-dashboards/ \
          openspec/specs/contact-hours-detector/spec.md \
          openspec/changes/archive/2026-07-05-issue-7-deterministic-detectors/
  git commit -m "docs(issue-7): archive deterministic-detectors change and merge specs"
  ```

## Follow-ups

- Task 4.4 (manual console demo) should be performed by a developer running
  the local dev environment before relying on the dashboards in a
  stakeholder-facing demo, though it blocks nothing structurally.
- Data-driven rule interpretation (which detectors run per bundle) remains
  the documented issue #6 follow-up; detector wiring stays static in
  `Service`.
- Age thresholds (`legalMajorityAge=18`, `elderlyAge=60`) in
  `protected_population.go` are pinned as the best-available reading of the
  cited statute and are flagged as pending legal-counsel confirmation before
  production use (per design.md's documented open item).
- Golden-eval fixtures for the 5 new detectors are an optional stretch, not
  yet added.

## SDD cycle status

Planned (proposal/spec/design/tasks) -> Implemented (apply, 6 stacked PRs +
2 judgment-day fast-follows) -> Verified (PASS WITH WARNINGS, warning closed
post-verify via live `make test-db`) -> **Archived** (this report), pending
the filesystem cleanup + git commit noted above by a shell-capable agent.
