## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | 700-900 |
| 400-line budget risk | High |
| Chained PRs recommended | Yes |
| Suggested split | PR 1 → PR 2 → PR 3 → PR 4 → PR 5 |
| Delivery strategy | ask-on-risk |
| Chain strategy | stacked-to-main |

Decision needed before apply: Yes
Chained PRs recommended: Yes
Chain strategy: stacked-to-main
400-line budget risk: High

## Implementation work units

- [x] **Outbound decision core + harness metadata propagation**
  - Files/targets: `internal/harness/permissions.go`, `internal/harness/runtime.go`, new `internal/outbound/*.go` (proposal, decision, context resolver, deterministic policy evaluator, remediation/draft model), plus behavior tests beside those packages.
  - TDD order: RED with behavior tests for missing timezone, unknown bundle, and compliant allow; GREEN by wiring the new decider; TRIANGULATE with focused package tests; REFACTOR only after `go test` is green.
  - Non-goals: no judge seam yet, no evidence ledger writes, no campaign preflight, no HTTP/CLI entrypoint.
  - Verification: `go test ./internal/harness ./internal/outbound` then `go test ./...`.

- [x] **Realtime enforcement completeness + judge seam**
  - Files/targets: new `internal/harness/outboundgate/*.go`, extend `internal/outbound/*.go` to cover recipient/channel ambiguity, third-party, payment-routing, and tone/threat via `internal/judge.Judge`; tests for deny-before-send, judge-spy short-circuiting, fail-closed judge errors, and draft-only rewrite suggestions.
  - TDD order: RED with cases that prove deterministic blocks do not call the judge and semantic blocks do not use the Harness model provider; GREEN by adding the gate adapter and judge port wiring; TRIANGULATE with focused package tests; REFACTOR after the adapter is stable.
  - Non-goals: no persistence changes, no campaign preflight, no dashboard/UI work.
  - Verification: `go test ./internal/outbound ./internal/harness` then `go test ./...`.

- [x] **High-risk evidence persistence for blocked realtime decisions**
  - Files/targets: `internal/postgres/adapters.go`, `internal/db/evaluations.sql.go` and any adjacent sqlc outputs/schema/query files that must carry rule-aware metadata; integration coverage in `internal/postgres/evidence_integration_test.go` and `internal/postgres/evaluation_integration_test.go`.
  - TDD order: RED with persistence tests that assert blocked authority decisions write the expected evaluation/evidence chain and preserve rollback behavior; GREEN by threading the new metadata through the existing ledger path; TRIANGULATE with targeted integration tests; REFACTOR only after the transaction story is clean.
  - Non-goals: no new ledger table unless the current chain cannot hold the required metadata; no dry-run preflight writes; no UI changes.
  - Verification: `go test ./internal/postgres -run 'TestEvidence|TestEvaluation'` then `go test ./...`.

- [x] **Campaign preflight dry-run module**
  - Files/targets: new `internal/outbound/preflight/*.go` or `internal/outbound/campaign_preflight.go`, plus fixture-heavy tests in the same package; use the same decider in dry-run mode and return an actionable brief with pass/fail, rule codes, bundle version, context gaps, and dry-run refs.
  - TDD order: RED with complete-campaign pass/fail scenarios and dry-run reference separation; GREEN by expanding the campaign artifact and folding step decisions; TRIANGULATE with focused package tests; REFACTOR after the brief shape is stable.
  - Non-goals: no send-provider side effects, no campaign launch UI, no partial-campaign optimization for this slice.
  - Verification: `go test ./internal/outbound -run 'Preflight|Campaign'` then `go test ./...`.

- [x] **Narrow preflight entrypoint wiring + request/response tests**
  - Files/targets: inspect `internal/httpapi/httpapi.go` first for the smallest existing tenant-authenticated seam; implement there if it is the lightest path, otherwise use one minimal `cmd/*` entrypoint only. Add request/response tests in `internal/httpapi/httpapi_test.go` or the chosen `cmd/*` test file.
  - TDD order: RED with request-shape, auth-scope, and brief-return tests; GREEN by wiring one endpoint/command to the preflight service; TRIANGULATE with focused package tests; REFACTOR only after the entrypoint stays narrow.
  - Non-goals: no campaign-management UI, no broad routing surface, no extra workflow orchestration.
  - Verification: `go test ./internal/httpapi ./...` (or `go test ./cmd/... ./...` if the CLI seam wins discovery) then `go test ./...`.

## Slice boundary notes

- Keep each slice independently reviewable and reversible.
- Preserve the design order: core decisioning → realtime enforcement completeness → evidence persistence → campaign preflight → entrypoint wiring.
- Do not add tests that merely restate implementation details; every test should prove a user-visible behavior or a boundary contract.
- Treat slice 3 as the ledger-risk hotspot and review it separately before any apply phase.
