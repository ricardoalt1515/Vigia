## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | 850–1,100 |
| 400-line budget risk | High |
| Chained PRs recommended | Yes |
| Suggested split | PR 1 → PR 2 → PR 3 → PR 4 |
| Delivery strategy | ask-on-risk |
| Chain strategy | pending |

Decision needed before apply: Yes
Chained PRs recommended: Yes
Chain strategy: pending
400-line budget risk: High

## Work Units

- [x] **PR 1 — Migration, query layer, and business-day calendar**
  - Start: no complaint workflow schema, no complaint query files, no business-day calendar package.
  - Add RED tests first in `internal/orchestrator/calendar_test.go` for weekend skipping, seeded holiday skipping, and fail-closed ambiguity; add migration/query integration coverage for `db/migrations/00009_complaint_workflow.sql` and the new SQL files.
  - Implement `db/migrations/00009_complaint_workflow.sql`, `db/queries/complaint_cases.sql`, `db/queries/human_reviews.sql`, `db/queries/business_day_holidays.sql`, and `internal/orchestrator/calendar.go`.
  - Finish when the seeded holiday set, version pinning, `AddBusinessDays`, and the complaint query surfaces are in place and sqlc generation succeeds.
  - Verify with focused Go tests for `internal/orchestrator` plus migration/query checks against the new SQL files.
  - Roll back by reverting the migration and query files plus `internal/orchestrator/calendar.go`.

- [x] **PR 2 — Complaint store, state machine, and ledger binding**
  - Start: calendar/query layer exists, but no complaint store, no state machine, and no complaint-transition hash binding.
  - Add RED tests first in `internal/ledger/ledger_test.go` for the trailing `ComplaintTransition` hash stability and tamper detection; add store/state tests for idempotent create, CAS no-op semantics, tenant predicates, and atomic evidence append.
  - Implement `internal/ledger/ledger.go`, `internal/postgres/complaint_store.go`, `internal/postgres/adapters.go`, and `internal/orchestrator/state.go`.
  - Update `cmd/ledger-verify/main.go` only as needed for doc clarity; do not change its verification contract.
  - Finish when complaint-case create/transition logic, evidence-chain extension, and read-path reconstruction are consistent with the design.
  - Verify with unit tests for ledger hashing and state transitions plus integration coverage for store atomicity and tenant isolation.
  - Roll back by reverting the store/state/ledger changes and the adapter adjustments.

- [x] **PR 3 — River jobs and worker registration**
  - Start: complaint persistence exists, but no periodic poll, no transition worker, and no worker registration.
  - Add RED tests first for poll selection (`open`, `sla_due_at`, `review_expires_at`, and unprocessed `human_reviews`), `UniqueOpts` dedupe behavior, and duplicate-review processing.
  - Implement `internal/orchestrator/jobs.go`, `cmd/worker/main.go`, and any required `go.mod`/`go.sum` River dependency promotion.
  - Finish when the poller can enqueue request-review, SLA-breach, TTL-expiry, and human-review transitions, and the worker is registered with idempotent execution semantics.
  - Verify with focused River/job tests and the broader Go suite for the touched packages.
  - Roll back by reverting the job/worker files and dependency promotion.

- [x] **PR 4 — HTTP endpoints and design doc sync**
  - Start: workflow is wired in storage and jobs, but no HTTP surface and no doc reconciliation.
  - Add RED endpoint tests first in `internal/httpapi/httpapi_test.go` for `POST /v1/complaints`, idempotent repeat creation, `POST /v1/complaints/{id}/reviews`, and late-review `409 Conflict` behavior.
  - Implement `internal/httpapi/httpapi.go` and update `docs/technical-design.md` to reconcile the canonical complaint-case and human-review shapes with this change.
  - Finish when the API can create cases, accept review decisions, and reflect the superseding model in the docs.
  - Verify with HTTP-focused tests and a final `go test ./...` pass for the affected packages.
  - Roll back by reverting the HTTP handler changes and the doc-sync update.
