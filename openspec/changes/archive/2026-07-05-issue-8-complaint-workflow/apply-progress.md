# Apply Progress: issue-8-complaint-workflow

## Workload / PR Boundary

- Delivery path: chained PR slice approved by parent prompt because Review Workload Forecast is High and chained PRs are recommended.
- Applied slice: PR 1 only — migration, query layer, and business-day calendar.
- Not implemented in this slice: complaint store, state machine, ledger binding, River jobs, worker registration, HTTP endpoints, docs sync.

## Structured Status Consumed

- Change: `issue-8-complaint-workflow`
- Artifact store: `openspec`
- Worktree: `/Users/ricardoaltamirano/Developer/vigia-issue8`
- Authoritative workspace/edit root: `/Users/ricardoaltamirano/Developer/vigia-issue8`
- Native status from parent: proposal/specs/design/tasks done; apply ready.
- Strict TDD: active (`go test ./...`).
- Action-context warning: main checkout `/Users/ricardoaltamirano/Developer/vigia` was not touched.
- Review workload guard: High / chained PRs recommended / decision needed in `tasks.md`; parent resolved this by assigning PR 1 only.

## TDD Cycle Evidence

| Cycle | RED | GREEN | TRIANGULATE | REFACTOR |
|---|---|---|---|---|
| Calendar | `go test ./internal/orchestrator` failed with undefined `Calendar`, `AddBusinessDays`, `HolidayRow`, `LoadCalendar` after adding behavior tests. | Added `internal/orchestrator/calendar.go`; `go test ./internal/orchestrator ./internal/db -run 'TestAddBusinessDays|TestLoadCalendar|ComplaintWorkflow'` passed. | Tests cover weekend skipping, seeded holiday skipping, fail-closed unseeded/ambiguous day behavior, and version-pinned loading. | Kept implementation pure: no `time.Now()`, weekends implicit, holidays explicit by version. |
| Migration/query surface | `go test ./internal/db -run ComplaintWorkflow` failed because `db/migrations/00009_complaint_workflow.sql` did not exist. | Added migration and query files; generated sqlc successfully with `go tool sqlc generate`; focused tests passed. | Static migration checks cover tenant-scoped cases/reviews, RLS, versioned holiday seed, counsel-confirmation note, evidence-record extension fragments, and partial unique index names. | Updated `db/queries/evidence_records.sql` to return new generated model columns so existing adapter call sites continue compiling without implementing PR 2 store/ledger behavior. |
| Review remediation — behavioral migration/query coverage | Added RED tests in `internal/db/complaint_workflow_migration_test.go` for Postgres-backed complaint query behavior, CHECK/partial unique constraints, and restricted-role RLS coverage. The first focused run failed on rollback-safety before production changes: `go test ./internal/db -run 'ComplaintWorkflow'` reported the unsafe down migration disabling the append-only trigger and deleting complaint evidence. | Replaced the down migration's trigger-disable/delete path with a guarded rollback precondition that raises `restrict_violation` when `complaint_transition` evidence exists; focused tests passed. | Behavioral tests execute sqlc complaint queries and direct constraint probes against Postgres when `DATABASE_URL`/`APP_DATABASE_URL` are configured; otherwise they skip like existing integration tests. Static down-migration guard prevents regression even without a database. | Kept remediation within PR 1 schema/query scope; no complaint store, state machine, ledger, jobs, or HTTP behavior was implemented. |

## Completed Tasks

- [x] PR 1 — Migration, query layer, and business-day calendar.
  - Persisted checkbox updated in `openspec/changes/issue-8-complaint-workflow/tasks.md` and re-read to confirm it is visibly marked `- [x]`.

## Files Changed

- `db/migrations/00009_complaint_workflow.sql`
- `db/queries/business_day_holidays.sql`
- `db/queries/complaint_cases.sql`
- `db/queries/human_reviews.sql`
- `db/queries/evidence_records.sql`
- `internal/db/business_day_holidays.sql.go` (generated)
- `internal/db/complaint_cases.sql.go` (generated)
- `internal/db/human_reviews.sql.go` (generated)
- `internal/db/evidence_records.sql.go` (generated)
- `internal/db/models.go` (generated)
- `internal/db/querier.go` (generated)
- `internal/db/complaint_workflow_migration_test.go`
- `internal/orchestrator/calendar.go`
- `internal/orchestrator/calendar_test.go`
- `openspec/changes/issue-8-complaint-workflow/tasks.md`
- `openspec/changes/issue-8-complaint-workflow/apply-progress.md`

## Verification Evidence

- RED: `go test ./internal/orchestrator` failed before implementation with undefined calendar symbols.
- RED: `go test ./internal/db -run ComplaintWorkflow` failed before migration implementation because `00009_complaint_workflow.sql` did not exist.
- GREEN/focused: `go test ./internal/orchestrator ./internal/db -run 'TestAddBusinessDays|TestLoadCalendar|ComplaintWorkflow'` passed.
- Generation: `go tool sqlc generate` succeeded.
- Focused migration re-check after rollback-safety adjustment: `go test ./internal/db -run ComplaintWorkflow` passed.
- Remediation RED: after adding review tests, `go test ./internal/db -run 'ComplaintWorkflow'` failed on `TestComplaintWorkflowDownMigrationRefusesToEraseComplaintEvidence` because the down migration disabled `evidence_records_no_update_delete` and deleted `record_kind='complaint_transition'` rows.
- Remediation GREEN/focused: `go test ./internal/db -run 'ComplaintWorkflow'` passed after replacing the unsafe rollback with a guarded refusal and adding Postgres-backed query/constraint/RLS tests. Integration portions require `DATABASE_URL`/`APP_DATABASE_URL` and skip when unset, consistent with existing repo tests.
- Full suite after remediation: `go test ./...` passed.
- End state command: `git status --short` run after implementation; see final result summary for current status.

## Deviations From Design

- PR 1 intentionally stops at schema/query/calendar surfaces. It does not implement the PR 2 ledger `ComplaintTransition` hash-binding code, store/state code, or read-path reconstruction beyond making generated evidence query rows include the new model columns so the current code compiles.
- Holiday seed is the static `mx-lft-art-74-2026a` set with an explicit pending-counsel-confirmation note in the migration.
- Migration Down now refuses rollback with `restrict_violation` if any `record_kind='complaint_transition'` evidence exists. It no longer disables the append-only trigger and no longer deletes complaint evidence, preserving the per-tenant hash chain after PR 2+ writes those rows.

## Remaining Tasks

Exact unchecked work-unit lines remaining in `tasks.md`:

```text
- [ ] **PR 2 — Complaint store, state machine, and ledger binding**
- [ ] **PR 3 — River jobs and worker registration**
- [ ] **PR 4 — HTTP endpoints and design doc sync**
```

## PR 2 Apply Update — Complaint store, state machine, and ledger binding

### Workload / PR Boundary

- Applied slice: PR 2 only — complaint store, state machine, and complaint-transition ledger binding.
- Preserved PR 1 migration/query/calendar work already present in the worktree.
- Not implemented in this slice: PR 3 River jobs/worker registration and PR 4 HTTP endpoints/doc sync.

### Structured Status Consumed

- Change: `issue-8-complaint-workflow`
- Artifact store: `openspec`
- Worktree/edit root: `/Users/ricardoaltamirano/Developer/vigia-issue8`
- Main checkout guard: `/Users/ricardoaltamirano/Developer/vigia` was not touched.
- Strict TDD: active (`go test ./...`).
- Review workload guard: High / chained PRs recommended / decision needed in `tasks.md`; parent resolved this retry by assigning PR 2 / Work Unit 2 only.

### TDD Cycle Evidence

| Cycle | RED | GREEN | TRIANGULATE | REFACTOR |
|---|---|---|---|---|
| Ledger complaint-transition hash binding | `go test ./internal/ledger ./internal/orchestrator ./internal/postgres -run 'Complaint|Plan'` failed with missing `Body.ComplaintTransition` and `ComplaintTransitionEvidence`. | Added trailing `ComplaintTransitionEvidence` to `ledger.Body`/canonical DTO; focused tests passed. | Tests prove the existing golden hash is unchanged when the field is nil and changing case id, transition kind, from/to state, or human review id changes the hash. | Kept the new field trailing and `omitempty` to preserve evaluation-only canonical bytes. |
| State machine | Same RED run failed with undefined complaint state/transition types and planner. | Added `internal/orchestrator/state.go`; focused tests passed. | Tests cover valid transitions and invalid/terminal no-op planning semantics. | Kept state logic pure and independent from River/HTTP. |
| Complaint store + evidence append | Same RED run failed with missing `NewComplaintCaseStoreFromPool`, create/transition inputs, and transition methods. | Added `internal/postgres/complaint_store.go`, generated SQL for complaint-transition evidence inserts, and adapter read-path reconstruction; focused tests and `go test ./...` passed. | Integration tests cover idempotent create, CAS no-op evidence suppression, tenant predicate no-op, and open + request_review evidence chain verification when `DATABASE_URL` is configured; otherwise they skip per repo convention. | Added transition `from_state`/`to_state`/`human_review_id` evidence columns so read-path reconstruction can rebuild the same hash body that the writer hashed. |

### Completed Tasks

- [x] PR 2 — Complaint store, state machine, and ledger binding.
  - Persisted checkbox updated in `openspec/changes/issue-8-complaint-workflow/tasks.md` and re-read to confirm it is visibly marked `- [x]`.

### Files Changed in PR 2

- `internal/ledger/ledger.go`
- `internal/ledger/ledger_test.go`
- `internal/orchestrator/state.go`
- `internal/orchestrator/state_test.go`
- `internal/postgres/complaint_store.go`
- `internal/postgres/complaint_store_integration_test.go`
- `internal/postgres/adapters.go`
- `cmd/ledger-verify/main.go`
- `db/queries/evidence_records.sql`
- `internal/db/evidence_records.sql.go` (generated)
- `internal/db/models.go` (generated)
- `internal/db/querier.go` (generated)
- `db/migrations/00009_complaint_workflow.sql` (PR 1 file adjusted to persist from/to/review evidence fields required for PR 2 read-path reconstruction)
- `openspec/changes/issue-8-complaint-workflow/tasks.md`
- `openspec/changes/issue-8-complaint-workflow/apply-progress.md`

### Verification Evidence

- RED: `go test ./internal/ledger ./internal/orchestrator ./internal/postgres -run 'Complaint|Plan'` failed before implementation with undefined complaint ledger/state/store symbols.
- GREEN/focused: `go test ./internal/ledger ./internal/orchestrator ./internal/postgres -run 'Complaint|Plan'` passed.
- Generation: `go tool sqlc generate` succeeded after adding `InsertComplaintTransitionEvidenceRecord`.
- Full suite: `go test ./...` passed.
- End state command: `git status --short` run after implementation; see final result summary.

### Deviations From Design

- Added `transition_from_state`, `transition_to_state`, and `human_review_id` columns to the PR 1 migration/evidence query surface. This is a corrective implementation detail: without persisted from/to/review fields, `evidenceRowToRecord` could not reconstruct the exact `ledger.Body.ComplaintTransition` that was hashed at write time.
- PR 2 intentionally does not enqueue River jobs, register workers, or expose HTTP endpoints.

### Remaining Tasks

Exact unchecked work-unit lines remaining in `tasks.md`:

```text
- [ ] **PR 3 — River jobs and worker registration**
- [ ] **PR 4 — HTTP endpoints and design doc sync**
```

## PR 2 Review Remediation — Human Review Binding Blockers

### Workload / PR Boundary

- Applied remediation inside PR 2 only — complaint store/state/ledger binding.
- Did not implement PR 3 River jobs/worker registration or PR 4 HTTP endpoints/doc sync.
- Main checkout guard: only `/Users/ricardoaltamirano/Developer/vigia-issue8` was touched.

### Structured Status Consumed

- Change: `issue-8-complaint-workflow`
- Artifact store: `openspec`
- Worktree/edit root: `/Users/ricardoaltamirano/Developer/vigia-issue8`
- Strict TDD: active (`go test ./...`).
- Parent review findings: PR2 `review-risk` and `review-reliability` requested changes for missing `HumanReviewID` resolution guard, wrong-case review winner validation, and missing behavior tests.
- Review workload guard: parent assigned PR2 remediation only; PR3/PR4 remain out of scope.

### TDD Cycle Evidence

| Cycle | RED | GREEN | TRIANGULATE | REFACTOR |
|---|---|---|---|---|
| Human review same-case winner guard | `go test ./internal/db -run TestComplaintWorkflowHumanReviewWinnerQueryRequiresSameCaseAndTenant` failed because `MarkWinningHumanReviewProcessed` filtered by tenant + review id only and did not require `complaint_case_id`. | Updated `db/queries/human_reviews.sql`, regenerated sqlc, and changed the store to validate `approve`/`override` against an unprocessed same-case review before CAS; the focused db test passed. | Added store behavior coverage for missing review id, wrong-case review id, same-case winner processing with duplicate superseding, and late approval after escalation. | Centralized resolution-review validation in `validateHumanReviewForResolution`; kept bookkeeping after successful CAS and before evidence append in the same transaction. |

### Completed Tasks

- [x] PR 2 — Complaint store, state machine, and ledger binding remains complete after remediation.
  - Persisted checkbox remained checked in `openspec/changes/issue-8-complaint-workflow/tasks.md` and was re-read after remediation.

### Files Changed in Remediation

- `db/queries/human_reviews.sql`
- `internal/db/human_reviews.sql.go` (generated)
- `internal/db/querier.go` (generated)
- `internal/db/complaint_workflow_migration_test.go`
- `internal/postgres/complaint_store.go`
- `internal/postgres/complaint_store_integration_test.go`
- `openspec/changes/issue-8-complaint-workflow/apply-progress.md`

### Verification Evidence

- RED: `go test ./internal/db -run TestComplaintWorkflowHumanReviewWinnerQueryRequiresSameCaseAndTenant` failed before remediation because `MarkWinningHumanReviewProcessed` did not include `complaint_case_id` in its predicate.
- GREEN/focused: `go test ./internal/db -run TestComplaintWorkflowHumanReviewWinnerQueryRequiresSameCaseAndTenant` passed.
- Behavior-focused remediation tests: `go test ./internal/postgres -run 'TestComplaintStoreApproveRequiresHumanReviewID|TestComplaintStoreApproveRejectsWrongCaseHumanReviewID|TestComplaintStoreApproveProcessesWinnerAndSupersedesDuplicateReviews|TestComplaintStoreLateApprovalAfterEscalationDoesNotResolveOrProcessReview'` passed. These integration tests skip when `DATABASE_URL` is unset, consistent with existing repo convention.
- Focused slice: `go test ./internal/ledger ./internal/orchestrator ./internal/postgres ./internal/db -run 'Complaint|Plan|HumanReview'` passed.
- Full suite: `go test ./...` passed.

### Deviations From Design

- No deviation. The remediation aligns PR2 with the design requirement that `approve`/`override` cannot resolve without a valid, unprocessed `human_reviews` row for the same tenant and complaint case.

### Remaining Tasks

Exact unchecked work-unit lines remaining in `tasks.md`:

```text
- [ ] **PR 3 — River jobs and worker registration**
- [ ] **PR 4 — HTTP endpoints and design doc sync**
```

## PR 2 Second Review Remediation — Decision-Kind Binding + Deterministic Expiry Fixtures

### Workload / PR Boundary

- Applied remediation inside PR 2 only — complaint store/query boundary and PR2 store tests.
- Did not implement PR 3 River jobs/worker registration or PR 4 HTTP endpoints/doc sync.
- Main checkout guard: only `/Users/ricardoaltamirano/Developer/vigia-issue8` was touched.

### Structured Status Consumed

```yaml
schemaName: spec-driven
changeName: issue-8-complaint-workflow
artifactStore: openspec
planningHome:
  root: /Users/ricardoaltamirano/Developer/vigia-issue8/openspec
  changesDir: /Users/ricardoaltamirano/Developer/vigia-issue8/openspec/changes
changeRoot: /Users/ricardoaltamirano/Developer/vigia-issue8/openspec/changes/issue-8-complaint-workflow
artifacts:
  proposal: done
  specs: done
  design: done
  tasks: done
  applyProgress: done
applyState: ready
strictTDD: true
actionContext:
  mode: repo-local
  workspaceRoot: /Users/ricardoaltamirano/Developer/vigia-issue8
  allowedEditRoots:
    - /Users/ricardoaltamirano/Developer/vigia-issue8
  warnings:
    - Do not touch main checkout /Users/ricardoaltamirano/Developer/vigia.
```

- Review workload guard: High / chained PRs recommended / decision needed in `tasks.md`; parent resolved this by assigning only the PR2 second-review remediation.

### TDD Cycle Evidence

| Cycle | RED | GREEN | TRIANGULATE | REFACTOR |
|---|---|---|---|---|
| Human review decision-kind binding | Added `TestComplaintWorkflowHumanReviewLookupRequiresMatchingDecision`, which failed because `GetUnprocessedHumanReviewForCase` did not require `AND decision = $4`. Added behavior tests for approve-with-override-review and override-with-approve-review mismatches before code changes. | Added the SQL predicate, regenerated sqlc, and passed the decision lookup and mismatch behavior tests. | Two mismatch cases cover both transition/review inversions; existing success coverage still proves matching approve reviews resolve and process/supersede rows. | Kept validation at the store/query boundary by passing `Decision: string(in.Kind)` into `GetUnprocessedHumanReviewForCase`; no PR3/PR4 changes. |
| Approval expiry test determinism | Existing successful approval fixtures used fixed `review_expires_at` derived from July 2026, while production CAS uses DB `now()`. | Updated successful approval fixture helper to set expiry one year after the current database clock; focused and full suites passed. | Late-approval semantics are preserved with an explicit expired fixture relative to DB `now()`. | Added `databaseNow` helper and split the awaiting-review fixture so tests state whether they need future or expired review TTLs. |

### Completed Tasks

- [x] PR 2 — Complaint store, state machine, and ledger binding remains complete after second-review remediation.
  - Persisted checkbox remains checked in `openspec/changes/issue-8-complaint-workflow/tasks.md` and was re-read after remediation.

### Files Changed in This Remediation

- `db/queries/human_reviews.sql`
- `internal/db/human_reviews.sql.go` (generated)
- `internal/db/querier.go` (generated timestamp/order unaffected except interface regeneration)
- `internal/db/complaint_workflow_migration_test.go`
- `internal/postgres/complaint_store.go`
- `internal/postgres/complaint_store_integration_test.go`
- `openspec/changes/issue-8-complaint-workflow/apply-progress.md`

### Verification Evidence

- Safety net before edits: `go test ./internal/postgres ./internal/db -run 'HumanReview|Approve|Override|ComplaintWorkflow'` passed.
- RED: `go test ./internal/db -run TestComplaintWorkflowHumanReviewLookupRequiresMatchingDecision` failed before implementation because the lookup query did not require `AND decision = $4`.
- GREEN/focused: `go test ./internal/db -run TestComplaintWorkflowHumanReviewLookupRequiresMatchingDecision` passed after adding the SQL predicate and regenerating sqlc.
- Behavior-focused: `go test ./internal/postgres -run 'TestComplaintStoreApproveRejectsOverrideHumanReviewDecision|TestComplaintStoreOverrideRejectsApproveHumanReviewDecision|TestComplaintStoreApproveProcessesWinnerAndSupersedesDuplicateReviews|TestComplaintStoreLateApprovalAfterEscalationDoesNotResolveOrProcessReview'` passed.
- Focused slice after gofmt: `go test ./internal/db ./internal/postgres -run 'ComplaintWorkflow|ComplaintStore.*(Approve|Override|LateApproval|HumanReview)'` passed.
- Full suite: `go test ./...` passed.
- End state command: `git status --short` run after implementation; see final result summary.

### Deviations From Design

- No deviation. The remediation tightens the design's human-decision binding: an `approve` transition can only consume an unprocessed `approve` review for the same tenant/case, and an `override` transition can only consume an unprocessed `override` review for the same tenant/case.

### Remaining Tasks

Exact unchecked work-unit lines remaining in `tasks.md`:

```text
- [ ] **PR 3 — River jobs and worker registration**
- [ ] **PR 4 — HTTP endpoints and design doc sync**
```

## PR 2 Third Review Remediation — Non-Human Transition HumanReviewID Semantics

### Workload / PR Boundary

- Applied remediation inside PR 2 only — complaint store/state/ledger binding semantics.
- Did not implement PR 3 River jobs/worker registration or PR 4 HTTP endpoints/doc sync.
- Main checkout guard: only `/Users/ricardoaltamirano/Developer/vigia-issue8` was touched.

### Structured Status Consumed

```yaml
schemaName: spec-driven
changeName: issue-8-complaint-workflow
artifactStore: openspec
planningHome:
  root: /Users/ricardoaltamirano/Developer/vigia-issue8/openspec
  changesDir: /Users/ricardoaltamirano/Developer/vigia-issue8/openspec/changes
changeRoot: /Users/ricardoaltamirano/Developer/vigia-issue8/openspec/changes/issue-8-complaint-workflow
artifacts:
  proposal: done
  specs: done
  design: done
  tasks: done
  applyProgress: done
applyState: ready
strictTDD: true
verificationCommand: go test ./...
actionContext:
  mode: repo-local
  workspaceRoot: /Users/ricardoaltamirano/Developer/vigia-issue8
  allowedEditRoots:
    - /Users/ricardoaltamirano/Developer/vigia-issue8
  warnings:
    - Do not touch main checkout /Users/ricardoaltamirano/Developer/vigia.
```

- Review workload guard: High / chained PRs recommended / decision needed in `tasks.md`; parent resolved this by assigning only the non-blocking PR2 audit-semantics remediation.

### TDD Cycle Evidence

| Cycle | RED | GREEN | TRIANGULATE | REFACTOR |
|---|---|---|---|---|
| Non-human transition HumanReviewID semantics | Added `TestComplaintTransitionKindAllowsHumanReviewIDOnlyForHumanResolutions`; focused RED failed because `ComplaintTransitionKind.AllowsHumanReviewID` did not exist. Added store behavior coverage for rejecting `HumanReviewID` on `request_review`, `ttl_expired`, and `sla_breach` before evidence append. | Added `AllowsHumanReviewID`, rejected non-human `HumanReviewID` at `ApplyTransition` entry, and passed only an approve/override review id into `appendComplaintEvidence`. Focused tests passed. | Triangulated all non-human transition kinds reachable through `ApplyTransition` (`request_review`, `ttl_expired`, `sla_breach`) plus `open` in the pure kind table; integration assertions verify no evidence row writes `human_review_id` and arbitrary reviews remain unprocessed. | Reused the shared predicate from store validation so approve/override semantics stay centralized; no PR3/PR4 code was touched. |

### Completed Tasks

- [x] PR 2 — Complaint store, state machine, and ledger binding remains complete after audit-semantics remediation.
  - Persisted checkbox remains checked in `openspec/changes/issue-8-complaint-workflow/tasks.md` and was re-read after remediation.

### Files Changed in This Remediation

- `internal/orchestrator/state.go`
- `internal/orchestrator/state_test.go`
- `internal/postgres/complaint_store.go`
- `internal/postgres/complaint_store_integration_test.go`
- `openspec/changes/issue-8-complaint-workflow/apply-progress.md`

### Verification Evidence

- Safety net: `go test ./internal/postgres -run 'TestComplaintStore.*(RequestReview|TTL|SLA|HumanReview|Approve|Override)'` passed before production changes.
- RED: `go test ./internal/orchestrator -run TestComplaintTransitionKindAllowsHumanReviewIDOnlyForHumanResolutions -count=1` failed before implementation because `AllowsHumanReviewID` was undefined.
- GREEN/focused: `go test ./internal/orchestrator -run TestComplaintTransitionKindAllowsHumanReviewIDOnlyForHumanResolutions -count=1` passed.
- Behavior-focused: `go test ./internal/postgres -run 'TestComplaintStoreRejectsNonHumanHumanReviewIDBeforeDatabaseAccess|TestComplaintStoreNonHumanTransitionsRejectHumanReviewID' -count=1` passed. Postgres-backed subtest coverage follows the repo convention and skips when `DATABASE_URL` is unset; the before-database-access test always runs.
- Focused slice: `go test ./internal/orchestrator ./internal/postgres -run 'HumanReviewID|NonHuman|ComplaintStore.*(Approve|Override|LateApproval)' -count=1` passed.
- Full suite: `go test ./...` passed.
- End state command: `git status --short` run after implementation; see final result summary.

### Deviations From Design

- No deviation. The remediation tightens audit semantics so `human_review_id` can only be accepted and hash-bound for approve/override transitions.

### Remaining Tasks

Exact unchecked work-unit lines remaining in `tasks.md`:

```text
- [ ] **PR 3 — River jobs and worker registration**
- [ ] **PR 4 — HTTP endpoints and design doc sync**
```

## PR 3 Apply Update — River jobs and worker registration

### Workload / PR Boundary

- Applied slice: PR 3 only — River poll/transition jobs and worker registration.
- Preserved PR 1 and PR 2 work already present in the worktree.
- Not implemented in this slice: PR 4 HTTP endpoints and design doc sync.
- Main checkout guard: only `/Users/ricardoaltamirano/Developer/vigia-issue8` was touched; `/Users/ricardoaltamirano/Developer/vigia` was not touched.

### Structured Status Consumed

```yaml
schemaName: spec-driven
changeName: issue-8-complaint-workflow
artifactStore: openspec
planningHome:
  root: /Users/ricardoaltamirano/Developer/vigia-issue8/openspec
  changesDir: /Users/ricardoaltamirano/Developer/vigia-issue8/openspec/changes
changeRoot: /Users/ricardoaltamirano/Developer/vigia-issue8/openspec/changes/issue-8-complaint-workflow
artifacts:
  proposal: done
  specs: done
  design: done
  tasks: done
  applyProgress: done
applyState: ready
strictTDD: true
verificationCommand: go test ./...
actionContext:
  mode: repo-local
  workspaceRoot: /Users/ricardoaltamirano/Developer/vigia-issue8
  allowedEditRoots:
    - /Users/ricardoaltamirano/Developer/vigia-issue8
  warnings:
    - Do not touch main checkout /Users/ricardoaltamirano/Developer/vigia.
```

- Review workload guard: High / chained PRs recommended / decision needed in `tasks.md`; parent resolved this by assigning only PR 3 / Work Unit 3.

### TDD Cycle Evidence

| Cycle | RED | GREEN | TRIANGULATE | REFACTOR |
|---|---|---|---|---|
| Poll selection + transition args | `go test ./internal/orchestrator -run 'ComplaintPoll|ComplaintTransition.*Unique|ComplaintTransitionWorker' -count=1` failed with undefined `HumanReview`, `NewComplaintPollWorker`, `ComplaintJobSettings`, `ComplaintPollArgs`, and `ComplaintTransitionArgs`. | Added `internal/orchestrator/jobs.go`; focused orchestrator tests passed. | Poll test covers all four findings: `open` -> `request_review`, `sla_due_at` -> `sla_breach`, `review_expires_at` -> `ttl_expired`, and unprocessed `human_reviews` -> approve/override transition with the review id. | Kept selection behavior behind narrow store/enqueuer interfaces so poll logic is unit-testable without River or Postgres. |
| UniqueOpts dedupe + duplicate review processing | RED from the same focused run covered missing transition args and unique-key behavior before implementation. | Added `ComplaintTransitionArgs.InsertOpts()` with `river.UniqueOpts{ByArgs:true}` and `river:"unique"` tags on case id and transition kind only; duplicate review IDs share the same unique key, while different transition kinds do not. Added an optional River integration test that inserts duplicate approve reviews and expects one job when `DATABASE_URL` is configured. | Unit coverage proves duplicate reviews for the same `(complaint_case_id, transition_kind)` dedupe without collapsing approve vs override. Transition-worker coverage invokes duplicate deliveries and expects no error, relying on the PR2 store CAS/bookkeeping for idempotent no-op semantics. | `HumanReviewID` is intentionally excluded from the River unique dimensions so duplicate review rows do not produce distinct transition jobs. |
| Worker registration + Postgres adapter methods | Focused package tests initially failed until the worker had real complaint workers and the store implemented poll-listing/calendar methods. | Updated `cmd/worker/main.go` to register complaint poll and transition workers plus the periodic poll job; added Postgres store methods for tenant enumeration, open/SLA/TTL/review scans, case lookup, and holiday loading. Focused and full suites passed. | Transition worker computes `request_review` TTL with `AddBusinessDays` and the case's pinned `calendar_version`; River poll insertion uses `river.ClientFromContext[pgx.Tx]` from the worker context. | River dependencies were promoted to direct `go.mod` requirements and `go mod tidy` updated `go.sum`. |

### Completed Tasks

- [x] PR 3 — River jobs and worker registration.
  - Persisted checkbox updated in `openspec/changes/issue-8-complaint-workflow/tasks.md` and re-read to confirm it is visibly marked `- [x]`.

### Files Changed in PR 3

- `internal/orchestrator/jobs.go`
- `internal/orchestrator/jobs_test.go`
- `internal/postgres/complaint_store.go`
- `cmd/worker/main.go`
- `cmd/worker/complaint_jobs_integration_test.go`
- `go.mod`
- `go.sum`
- `openspec/changes/issue-8-complaint-workflow/tasks.md`
- `openspec/changes/issue-8-complaint-workflow/apply-progress.md`

### Verification Evidence

- RED: `go test ./internal/orchestrator -run 'ComplaintPoll|ComplaintTransition.*Unique|ComplaintTransitionWorker' -count=1` failed before implementation with undefined job/poll/transition symbols.
- GREEN/focused: `go test ./internal/orchestrator -run 'ComplaintPoll|ComplaintTransition.*Unique|ComplaintTransitionWorker' -count=1` passed after adding jobs and tests.
- Focused slice: `go test ./internal/orchestrator ./internal/postgres ./cmd/worker -run 'Complaint|Worker|Noop' -count=1` passed.
- Worker/River focused: `go test ./cmd/worker -run 'ComplaintTransitionUnique|Worker|Noop' -count=1` passed; the River duplicate insert portion skips when `DATABASE_URL` is unset, following the repo's existing integration-test convention.
- Full suite: `go test ./...` passed.
- End state command: `git status --short` run after implementation; see final result summary.

### Deviations From Design

- No PR4 HTTP endpoints or doc sync were implemented.
- `NoopJob` test scaffolding remains in `cmd/worker`, but `cmd/worker/main.go` no longer registers or enqueues it; production worker registration now uses complaint poll and transition workers.

### Remaining Tasks

Exact unchecked work-unit lines remaining in `tasks.md`:

```text
- [ ] **PR 4 — HTTP endpoints and design doc sync**
```

## PR 3 Review Remediation — Poll Starvation, River Unique Coverage, Request-Review TTL

### Workload / PR Boundary

- Applied remediation inside PR 3 only — River jobs, poll/store query behavior, worker tests, and River unique coverage.
- Did not implement PR 4 HTTP endpoints or design doc sync.
- Main checkout guard: only `/Users/ricardoaltamirano/Developer/vigia-issue8` was touched; `/Users/ricardoaltamirano/Developer/vigia` was not touched.

### Structured Status Consumed

```yaml
schemaName: spec-driven
changeName: issue-8-complaint-workflow
artifactStore: openspec
planningHome:
  root: /Users/ricardoaltamirano/Developer/vigia-issue8/openspec
  changesDir: /Users/ricardoaltamirano/Developer/vigia-issue8/openspec/changes
changeRoot: /Users/ricardoaltamirano/Developer/vigia-issue8/openspec/changes/issue-8-complaint-workflow
artifacts:
  proposal: done
  specs: done
  design: done
  tasks: done
  applyProgress: done
applyState: ready
strictTDD: true
verificationCommand: go test ./...
actionContext:
  mode: repo-local
  workspaceRoot: /Users/ricardoaltamirano/Developer/vigia-issue8
  allowedEditRoots:
    - /Users/ricardoaltamirano/Developer/vigia-issue8
  warnings:
    - Do not touch main checkout /Users/ricardoaltamirano/Developer/vigia.
```

- Review workload guard: High / chained PRs recommended / decision needed in `tasks.md`; parent resolved this by assigning only PR 3 review remediation and explicitly not PR 4.

### TDD Cycle Evidence

| Cycle | RED | GREEN | TRIANGULATE | REFACTOR |
|---|---|---|---|---|
| Stale terminal human-review starvation | Added `TestComplaintWorkflowUnprocessedReviewListRequiresAwaitingReviewCase` and `TestComplaintWorkflowUnprocessedReviewListSkipsTerminalCasesUnderLimitPressure`; focused RED `go test ./internal/orchestrator ./internal/db ./cmd/worker -run 'UnprocessedReviewList|UniqueTags|ComputesRequestReviewTTL|ComplaintTransitionUnique' -count=1` failed because `ListUnprocessedHumanReviews` selected from `human_reviews` by tenant only. | Joined `human_reviews` to `complaint_cases` on same case/tenant and filtered `cc.state = 'awaiting_review'`; regenerated sqlc; `go test ./internal/db -run 'UnprocessedReviewList' -count=1` passed. | Static query coverage pins the join/filter and Postgres behavior coverage proves an older stale terminal-case review cannot consume `LIMIT 1` ahead of a valid active review. | Kept the store API unchanged; fixed the data-source query so the poller receives only actionable reviews. |
| River unique contract coverage | Added always-running reflective unit test `TestComplaintTransitionArgsRiverUniqueTagsAreCaseAndKindOnly` to pin that only `ComplaintCaseID` and `TransitionKind` have `river:"unique"`, alongside existing ByArgs/unique-key coverage. | Focused orchestrator unique tests passed without requiring a database. | Coverage now proves the static tag set, the helper unique key semantics, and keeps the DB-backed River insertion test for actual dedupe behavior. | Strengthened `cmd/worker` River integration gating so missing `DATABASE_URL` fails in CI (`CI`, `GITHUB_ACTIONS`, or `BUILDKITE`) instead of silently skipping where DB-backed proof is expected. |
| Request-review default TTL | Added `TestComplaintTransitionWorkerComputesRequestReviewTTLWithDefaultBusinessDays` before production changes to prove the worker's `request_review` branch computes `review_expires_at` from the pinned calendar and default 3 business days. | Existing production branch satisfied the test; focused orchestrator tests passed. | The test uses a Friday start plus a pinned Monday holiday, proving weekend skipping, calendar filtering by version, and the default 3-business-day TTL path. | No production refactor was needed for this branch. |

### Completed Tasks

- [x] PR 3 — River jobs and worker registration remains complete after review remediation.
  - Persisted checkbox remains checked in `openspec/changes/issue-8-complaint-workflow/tasks.md` and was re-read after remediation.

### Files Changed in This Remediation

- `db/queries/human_reviews.sql`
- `internal/db/human_reviews.sql.go` (generated)
- `internal/db/complaint_workflow_migration_test.go`
- `internal/orchestrator/jobs_test.go`
- `cmd/worker/complaint_jobs_integration_test.go`
- `openspec/changes/issue-8-complaint-workflow/apply-progress.md`

### Verification Evidence

- Safety net before edits: `go test ./internal/orchestrator ./internal/db ./cmd/worker -run 'Complaint|Worker|Noop' -count=1` passed.
- RED: `go test ./internal/orchestrator ./internal/db ./cmd/worker -run 'UnprocessedReviewList|UniqueTags|ComputesRequestReviewTTL|ComplaintTransitionUnique' -count=1` failed in `internal/db` because `ListUnprocessedHumanReviews` did not join/filter awaiting-review cases.
- Generation: `go tool sqlc generate` succeeded after updating `db/queries/human_reviews.sql`.
- GREEN/focused: `go test ./internal/db -run 'UnprocessedReviewList' -count=1` passed.
- Focused remediation suite: `go test ./internal/orchestrator ./internal/db ./cmd/worker -run 'UnprocessedReviewList|UniqueTags|UniqueOpts|ComputesRequestReviewTTL|ComplaintTransitionUnique' -count=1` passed.
- Full suite: `go test ./...` passed.
- End state command: `git status --short` run after implementation; see final result summary.

### Deviations From Design

- No design deviation. The poll query now matches the design/spec invariant that human-review polling resumes only cases currently in `awaiting_review` for the same tenant.
- No PR4 HTTP endpoints or doc sync were implemented.

### Remaining Tasks

Exact unchecked work-unit lines remaining in `tasks.md`:

```text
- [ ] **PR 4 — HTTP endpoints and design doc sync**
```

## PR 4 Apply Update — HTTP endpoints and design doc sync

### Workload / PR Boundary

- Applied slice: PR 4 only — HTTP complaint endpoints and technical-design doc reconciliation.
- Preserved PR 1/PR 2/PR 3 work already present in the worktree.
- Did not modify the main checkout `/Users/ricardoaltamirano/Developer/vigia`; all edits were under `/Users/ricardoaltamirano/Developer/vigia-issue8`.

### Structured Status Consumed

```yaml
schemaName: spec-driven
changeName: issue-8-complaint-workflow
artifactStore: openspec
planningHome:
  root: /Users/ricardoaltamirano/Developer/vigia-issue8/openspec
  changesDir: /Users/ricardoaltamirano/Developer/vigia-issue8/openspec/changes
changeRoot: /Users/ricardoaltamirano/Developer/vigia-issue8/openspec/changes/issue-8-complaint-workflow
artifacts:
  proposal: done
  specs: done
  design: done
  tasks: done
  applyProgress: done
applyState: ready
strictTDD: true
verificationCommand: go test ./...
actionContext:
  mode: repo-local
  workspaceRoot: /Users/ricardoaltamirano/Developer/vigia-issue8
  allowedEditRoots:
    - /Users/ricardoaltamirano/Developer/vigia-issue8
  warnings:
    - Do not touch main checkout /Users/ricardoaltamirano/Developer/vigia.
```

- Review workload guard: High / chained PRs recommended / decision needed in `tasks.md`; parent resolved this by approving only PR 4 / Work Unit 4.

### TDD Cycle Evidence

| Cycle | RED | GREEN | TRIANGULATE | REFACTOR |
|---|---|---|---|---|
| Complaint HTTP endpoints | Added RED endpoint tests in `internal/httpapi/httpapi_test.go` for complaint creation, idempotent repeat creation, review submission, and late-review 409 behavior. `go test ./internal/httpapi -run 'TestComplaintEndpoints' -count=1` failed before production changes with undefined `defaultComplaintCalendarVersion`, `complaintCaseResponse`, `HumanReview`, `CreateHumanReviewInput`, `ErrComplaintReviewConflict`, and a missing `NewServer` complaint dependency. | Added `POST /v1/complaints` and `POST /v1/complaints/{id}/reviews`, a complaint workflow port, request/response DTOs, decision validation, 201-vs-200 idempotent create handling, 202 review acceptance, and 409 conflict mapping for late/non-awaiting reviews. Focused endpoint test passed. | Tests assert tenant scoping into the workflow port, SLA computation through the business-day calendar, repeated create returning 200 for an existing case, and late review returning 409. | Kept the HTTP endpoint as a thin port adapter: case creation computes SLA and delegates persistence/evidence append to the existing store; review submission inserts `human_reviews` only and does not transition the case. |
| Postgres/API wiring + doc sync | Endpoint tests required a concrete complaint workflow implementation for production wiring. | Added `ComplaintCaseStore.CreateHumanReview`, expanded `orchestrator.HumanReview` with reviewer/notes/created fields for API responses, wired `cmd/api` with `postgres.NewComplaintCaseStoreFromPool`, and updated `docs/technical-design.md` canonical `ComplaintCase`/`HumanReview` shapes plus endpoint notes. | Review endpoint uses the existing SQL guard that inserts only when the case is tenant-scoped and `awaiting_review`; `pgx.ErrNoRows` maps to HTTP 409. | Used type aliases in `internal/httpapi` for shared orchestrator DTOs to avoid a postgres → httpapi dependency. |

### Completed Tasks

- [x] PR 4 — HTTP endpoints and design doc sync.
  - Persisted checkbox updated in `openspec/changes/issue-8-complaint-workflow/tasks.md` and re-read to confirm it is visibly marked `- [x]`.

### Files Changed in PR 4

- `internal/httpapi/httpapi.go`
- `internal/httpapi/httpapi_test.go`
- `internal/postgres/complaint_store.go`
- `internal/orchestrator/state.go`
- `internal/orchestrator/jobs.go`
- `cmd/api/main.go`
- `docs/technical-design.md`
- `openspec/changes/issue-8-complaint-workflow/tasks.md`
- `openspec/changes/issue-8-complaint-workflow/apply-progress.md`

### Verification Evidence

- RED: `go test ./internal/httpapi -run 'TestComplaintEndpoints' -count=1` failed before implementation with missing complaint endpoint/API symbols and the old `NewServer` dependency shape.
- GREEN/focused: `go test ./internal/httpapi -run 'TestComplaintEndpoints' -count=1` passed.
- HTTP package: `go test ./internal/httpapi -count=1` passed.
- Full suite: `go test ./...` passed.
- End state command: `git status --short` run after implementation; see final result summary.

### Deviations From Design

- No design deviation. The endpoint creates/open cases idempotently, accepts human review decisions only through `human_reviews`, returns 409 for late/non-awaiting reviews, and leaves transitions to the periodic poll/worker path.

### Remaining Tasks

Exact unchecked work-unit lines remaining in `tasks.md`:

```text
(none)
```

## PR 4 Review Remediation — Idempotency Header Support + Wiring Cleanup

### Workload / PR Boundary

- Applied remediation inside PR 4 only — HTTP complaint endpoint behavior/tests plus minimal server wiring cleanup.
- Did not implement beyond PR4 and did not touch orchestrator job cadence; the 60s cadence constant suggestion was left unchanged to avoid expanding the strict-TDD remediation surface.
- Main checkout guard: only `/Users/ricardoaltamirano/Developer/vigia-issue8` was touched; `/Users/ricardoaltamirano/Developer/vigia` was not touched.

### Structured Status Consumed

```yaml
schemaName: spec-driven
changeName: issue-8-complaint-workflow
artifactStore: openspec
planningHome:
  root: /Users/ricardoaltamirano/Developer/vigia-issue8/openspec
  changesDir: /Users/ricardoaltamirano/Developer/vigia-issue8/openspec/changes
changeRoot: /Users/ricardoaltamirano/Developer/vigia-issue8/openspec/changes/issue-8-complaint-workflow
artifacts:
  proposal: done
  specs: done
  design: done
  tasks: done
  applyProgress: done
applyState: ready-for-remediation
strictTDD: true
verificationCommand: go test ./...
actionContext:
  mode: repo-local
  workspaceRoot: /Users/ricardoaltamirano/Developer/vigia-issue8
  allowedEditRoots:
    - /Users/ricardoaltamirano/Developer/vigia-issue8
  warnings:
    - Do not touch main checkout /Users/ricardoaltamirano/Developer/vigia.
```

- Review workload guard: High / chained PRs recommended / decision needed in `tasks.md`; parent resolved this by assigning only PR4 review remediation.

### TDD Cycle Evidence

| Cycle | RED | GREEN | TRIANGULATE | REFACTOR |
|---|---|---|---|---|
| Complaint create idempotency header | Safety net `go test ./internal/httpapi -run 'TestComplaintEndpoints' -count=1` passed before edits. Added RED HTTP test for `Idempotency-Key` header with body omitting `idempotency_key`; focused run failed with `400`, proving the endpoint only accepted the body field. | Resolved the idempotency key from body/header, accepted either location, and passed the header value to `CreateComplaintCase`; `go test ./internal/httpapi -run 'TestComplaintEndpoints' -count=1` passed. | Added mismatch coverage: when header and body keys are both present but differ, endpoint returns `400 Bad Request` and does not call the complaint workflow. | Trimmed header/body idempotency values and kept tenant/auth behavior unchanged. |
| Server complaint dependency wiring | The same focused compile/test cycle covered the `NewServer` constructor change. Existing non-complaint tests initially needed explicit nil wiring at call sites. | Made `NewServer` take the complaint workflow as an explicit parameter instead of a variadic optional dependency; cmd/api continues to pass the real store, older non-complaint tests pass `nil`. | Triangulation comes from both complaint tests with a real fake workflow and existing non-complaint endpoint tests with `nil`, proving route behavior remains guarded by the existing nil check. | Removed the variadic dependency ambiguity while avoiding broader API/server refactors. |

### Completed Tasks

- [x] PR 4 — HTTP endpoints and design doc sync remains complete after review remediation.
  - Persisted checkbox remained checked in `openspec/changes/issue-8-complaint-workflow/tasks.md` and was re-read after remediation.

### Files Changed in This Remediation

- `internal/httpapi/httpapi.go`
- `internal/httpapi/httpapi_test.go`
- `openspec/changes/issue-8-complaint-workflow/apply-progress.md`

### Verification Evidence

- Safety net: `go test ./internal/httpapi -run 'TestComplaintEndpoints' -count=1` passed before edits.
- RED: `go test ./internal/httpapi -run 'TestComplaintEndpoints' -count=1` failed after adding header/mismatch tests because header-only idempotency returned `400` and mismatched header/body keys were silently accepted.
- GREEN/focused: `go test ./internal/httpapi -run 'TestComplaintEndpoints' -count=1` passed after resolving header/body idempotency and rejecting mismatches.
- HTTP package: `go test ./internal/httpapi -count=1` passed.
- Full suite: `go test ./...` passed.
- End state command: `git status --short` run after implementation; see final result summary.

### Deviations From Design

- No design deviation for idempotency: `POST /v1/complaints` now accepts `Idempotency-Key` header or JSON `idempotency_key`; when both are present and differ, the endpoint fails closed with `400 Bad Request` instead of silently choosing one.
- Did not name the complaint poll 60s cadence constant in this remediation because no orchestrator production code was otherwise touched and adding it would widen the strict-TDD surface beyond the PR4 HTTP review findings.

### Remaining Tasks

Exact unchecked work-unit lines remaining in `tasks.md`:

```text
(none)
```

### PR4 post-review test pinning

- Added explicit HTTP coverage for matching `Idempotency-Key` header + JSON `idempotency_key` values, after review noted the implementation accepted this case but the contract was not pinned.
- Verification: `go test ./internal/httpapi -run 'TestComplaintEndpoints' -count=1` passed.
- Verification: `go test ./...` passed.
- LSP diagnostics on `internal/httpapi/httpapi.go`, `internal/httpapi/httpapi_test.go`, and `cmd/api/main.go` showed only existing ast-grep test-name warnings.
