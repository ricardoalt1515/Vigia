# Verify Report: issue-8-complaint-workflow

Status: PASS
Result: PASS
Verdict: PASS

## Status

PASS

The issue #8 implementation completed all four planned work units and passes focused and full Go verification locally. Verification warnings are environmental: DB-backed integration branches that require `DATABASE_URL` / `APP_DATABASE_URL` were not exercised in this local shell because those variables are unset. The tests follow the repository convention for skipping DB-backed paths locally, and PR3 added CI-style gating for the River unique integration proof.

## Task Completion

| Work unit | Status |
|---|---|
| PR 1 — Migration, query layer, and business-day calendar | PASS |
| PR 2 — Complaint store, state machine, and ledger binding | PASS |
| PR 3 — River jobs and worker registration | PASS |
| PR 4 — HTTP endpoints and design doc sync | PASS |

All tasks in `tasks.md` are checked.

## Acceptance Criteria

| Acceptance criterion | Result | Evidence |
|---|---|---|
| Opening a complaint starts a durable case with a 10-business-day SLA timer and escalation. | PASS | `POST /v1/complaints` computes SLA with `AddBusinessDays`; store creates durable `complaint_cases`; River poll/transition jobs cover request-review and SLA breach; focused orchestrator/db/http tests passed. |
| `POST /v1/complaints` is idempotent: repeated same idempotency key returns existing case, no duplicate SLA timer/evidence row. | PASS | HTTP tests cover idempotent repeat; store tests cover idempotent create; migration/query layer has unique `(tenant_id, idempotency_key)`. Header/body idempotency behavior is covered, including mismatch `400`. |
| Case pauses at `awaiting_review` and resumes after console approve/override without lost/duplicated state. | PASS | Poll job enqueues `request_review`; review endpoint inserts `human_reviews`; store requires valid unprocessed same-tenant/same-case review with matching decision; duplicate reviews are processed/superseded in the same transition transaction. |
| Job retries are idempotent: no double evidence writes; evidence append + state transition are atomic. | PASS | Store uses CAS no-op semantics and appends evidence in the same `tenantdb.WithTenantTx`; River args use `UniqueOpts{ByArgs:true}` and unique tags on case ID + transition kind. DB-backed River uniqueness proof requires DB env. |
| Approval TTL bounds wait; expiry escalates fail-closed, never auto-approves, enforced by CAS temporal guard. | PASS | Store tests cover late approval not resolving/processing review; request-review TTL test covers default 3 business days using pinned calendar/version. |
| Tenant isolation enforced by explicit tenant predicates and RLS defense-in-depth tests. | PASS/WARNING | SQL queries use tenant predicates; migration/query tests include DB-backed RLS/constraint behavior when DB env is available. Local DB env was unset, so those branches were skipped locally. |
| Every newly created case reaches `awaiting_review` via poll job's open-case scan. | PASS | PR3 poll selection tests cover open case → request_review and worker TTL computation. |
| Complaint-transition evidence is hash-bound and tamper-detectable. | PASS | Ledger tests cover golden hash compatibility and complaint-transition tamper detection; store/adapters reconstruct complaint evidence for mixed-kind chain verification. |

## Verification Commands

```bash
go test ./internal/orchestrator ./internal/db ./internal/postgres ./internal/ledger -run 'Complaint|HumanReview|Plan|Hash|UnprocessedReviewList' -count=1
# PASS: internal/orchestrator, internal/db, internal/postgres, internal/ledger

go test ./cmd/worker -run 'Complaint|Worker|Noop' -count=1
# PASS

go test ./internal/httpapi -run 'TestComplaintEndpoints' -count=1
# PASS

go test ./...
# PASS

git status --short
# Shows cumulative uncommitted issue #8 changes in /Users/ricardoaltamirano/Developer/vigia-issue8
```

## Environment

```text
DATABASE_URL=unset
APP_DATABASE_URL=unset
```

DB-backed integration subtests that require these variables were not exercised locally. This is a verification warning, not a functional failure, because the full Go suite passes and the repository already uses this skip convention. Before merge/PR finalization, run the DB-backed suite in CI or a local environment with those variables set.

## Review Evidence

Fresh review lenses were used during apply slices:

- PR1: review-risk + review-reliability approved after remediation.
- PR2: review-risk + review-reliability approved after human-review integrity remediations; final warning was remediated inline and tested.
- PR3: review-risk no findings; review-reliability/review-resilience approved after stale-review starvation, River unique coverage, and TTL test remediations.
- PR4: review-risk no findings; review-reliability/readability approved after Idempotency-Key header support and explicit wiring remediation. A matching header+body test was added afterward and `go test ./...` passed.

## Warnings / Risks

- Local DB-backed integration paths were skipped because `DATABASE_URL` and `APP_DATABASE_URL` are unset.
- The worktree contains the full uncommitted chained issue #8 implementation; no commits were made.
- The dedicated `sdd-verify` subagent failed to start twice in this Pi runtime, so verification was performed by the parent session with the same required commands and this report was written directly.

## Next Recommended

Run SDD sync for `issue-8-complaint-workflow` after confirming whether to proceed. If preparing a PR, first run DB-backed verification in an environment with `DATABASE_URL` and `APP_DATABASE_URL` set, or rely on CI evidence if configured.

## Post-Archive Remediation Note — 2026-07-05

Final PR-readiness remediation addressed idempotency replay mismatches, restricted app-role complaint workflow write grants, and complaint poll worker starvation on tenant/scan/enqueue errors. DB-backed verification was run locally against a disposable Postgres 17 container on port 55432 with `DATABASE_URL` and `APP_DATABASE_URL` set; `go test ./...` passed in that DB-backed environment.
