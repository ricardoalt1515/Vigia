# Apply Progress: issue-10-realtime-guardrails-preflight

## Structured status consumed

- Project: `vigia`
- Workspace root: `/Users/ricardoaltamirano/Developer/vigia`
- Change: `issue-10-realtime-guardrails-preflight`
- Artifact store: `openspec` (authoritative)
- Allowed edit root: `/Users/ricardoaltamirano/Developer/vigia`
- Strict TDD: active; runner `go test ./...`
- Delivery boundary: PR 1 only; PR 2/3/4/5 explicitly deferred
- Workload guard: tasks forecast `Decision needed before apply: Yes`, `Chained PRs recommended: Yes`, `400-line budget risk: High`; parent supplied resolved PR 1 boundary.

## Completed tasks

- [x] `Outbound decision core + harness metadata propagation`
  - Persisted checkbox confirmed in `openspec/changes/issue-10-realtime-guardrails-preflight/tasks.md`.
  - Implemented bounded `PermissionDecision.Metadata` propagation into Harness permission-decision events and denied/approval-required tool-result events.
  - Added `internal/outbound` typed proposal, decision, context, bundle resolver, authority context resolver, and decider models.
  - Added deterministic PR 1 behavior:
    - missing debtor timezone denies fail-closed with `missing_debtor_timezone`;
    - unknown active policy bundle denies fail-closed with `unknown_policy_bundle`;
    - out-of-hours contact denies with `MX-REDECO-02`;
    - compliant proposal allows and carries policy bundle version.

## Files changed

- `internal/harness/permissions.go`
- `internal/harness/runtime.go`
- `internal/harness/runtime_test.go`
- `internal/outbound/types.go`
- `internal/outbound/decider.go`
- `internal/outbound/decider_test.go`
- `openspec/changes/issue-10-realtime-guardrails-preflight/tasks.md`
- `openspec/changes/issue-10-realtime-guardrails-preflight/apply-progress.md`

## TDD Cycle Evidence

| Task | Test File | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
|------|-----------|-------|------------|-----|-------|-------------|----------|
| Harness metadata propagation | `internal/harness/runtime_test.go` | Unit | ✅ `go test ./internal/harness` | ✅ test failed on missing `Metadata` and helper | ✅ `go test ./internal/harness ./internal/outbound -run 'TestDecider|TestRunStepDeniedToolPropagates'` | ✅ permission event + denied tool-result metadata assertions | ✅ helper extraction; focused and full suites passed |
| Outbound decision core | `internal/outbound/decider_test.go` | Unit | N/A (new package) | ✅ tests failed on missing `Decider`, proposal/decision/context models | ✅ focused tests passed after minimal decider | ✅ missing timezone, unknown bundle, resolver gap, out-of-hours, compliant allow cases | ✅ table-driven tests; focused and full suites passed |

## Test commands run

- `go test ./internal/harness` — passed baseline before modifying Harness files.
- `go test ./internal/outbound ./internal/harness -run 'TestDecider|TestRunStepDeniedToolPropagates'` — failed RED before production implementation.
- `go test ./internal/outbound ./internal/harness -run 'TestDecider|TestRunStepDeniedToolPropagates'` — passed GREEN.
- `go test ./internal/harness ./internal/outbound` — passed.
- `go test ./...` — passed.
- `go test ./internal/harness ./internal/outbound && go test ./...` — passed after refactor.

## Deviations from design

- The PR 1 slice intentionally does not add `internal/harness/outboundgate`; that belongs to PR 2 per the assigned boundary.
- Evidence refs/event refs, judge integration, campaign fields, and persistence wiring are deferred to later PRs to keep this slice reviewable.
- The outbound core currently uses a compact first-slice model and can be expanded at PR 2/PR 4 seams without coupling to Harness.

## PR 1 Review Remediation

Fresh reliability review found PR 1 blockers before continuing to PR 2:

- invalid/unresolvable debtor timezone was classified as a contact-hours rule
  violation instead of a fail-closed authority-context gap;
- `Decision.ID` was not populated from request/proposal correlation data;
- zero `ProposedAt` fell back to wall-clock time, weakening deterministic replay.

Remediation applied inside PR 1 only:

- added/kept RED tests for invalid timezone, zero proposed time, request-id
  correlation, and proposal-id fallback correlation;
- invalid timezone now denies with `invalid_debtor_timezone` as a
  fail-closed context gap;
- zero proposed time now denies with `missing_proposed_at`; no hidden
  `time.Now()` fallback remains;
- `Decision.ID` and metadata `decision_id` now use `RequestID`, falling back
  to `ProposalID`.

Verification after remediation:

- `go test ./internal/outbound` — passed;
- `go test ./internal/harness ./internal/outbound && go test ./...` — passed.

## Remaining tasks after PR 1

- [x] **Realtime enforcement completeness + judge seam** (completed in PR 2 update below)
- [ ] **High-risk evidence persistence for blocked realtime decisions**
- [ ] **Campaign preflight dry-run module**
- [ ] **Narrow preflight entrypoint wiring + request/response tests**

## Workload / PR boundary

- Completed only PR 1.
- Approximate PR 1 implementation size is slightly above the 400-line review budget when counting new outbound package files plus Harness diffs; no PR 2+ scope was added.
- Recommended next apply should continue with PR 2 only: realtime enforcement adapter + judge seam.

## PR 2 Apply Update: Realtime enforcement completeness + judge seam

## Structured status consumed for PR 2

- Project: `vigia`
- Workspace root: `/Users/ricardoaltamirano/Developer/vigia`
- Change: `issue-10-realtime-guardrails-preflight`
- Artifact store: `openspec` (authoritative)
- Allowed edit root: `/Users/ricardoaltamirano/Developer/vigia`
- Strict TDD: active; runner `go test ./...`
- Delivery boundary: PR 2 only; PR 3/4/5 explicitly deferred
- Workload guard: tasks forecast `Decision needed before apply: Yes`, `Chained PRs recommended: Yes`, `400-line budget risk: High`; parent supplied resolved PR 2 boundary and 400-line budget limit.

## Completed tasks

- [x] `Realtime enforcement completeness + judge seam`
  - Persisted checkbox confirmed in `openspec/changes/issue-10-realtime-guardrails-preflight/tasks.md` after implementation.
  - Added `internal/harness/outboundgate` adapter that maps `send_outbound_utterance` tool calls into enforcement-mode outbound decisions and delegates non-send tools to fallback.
  - Extended outbound decision core for ambiguous recipient/channel fail-closed decisions.
  - Extended deterministic outbound coverage for third-party relationship, authorized-channel, and payment-routing violations using existing pure detectors.
  - Added semantic tone/threat evaluation through `internal/judge.Judge` only; deterministic blocks short-circuit before judge calls.
  - Added fail-closed handling for judge errors and inconclusive low-confidence judge results.
  - Added draft-only rewrite suggestion metadata for semantic tone/threat blocks.

## Files changed in PR 2

- `internal/harness/outboundgate/gate.go`
- `internal/harness/outboundgate/gate_test.go`
- `internal/outbound/types.go`
- `internal/outbound/decider.go`
- `internal/outbound/decider_test.go`
- `openspec/changes/issue-10-realtime-guardrails-preflight/tasks.md`
- `openspec/changes/issue-10-realtime-guardrails-preflight/apply-progress.md`

## TDD Cycle Evidence for PR 2

| Task | Test File | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
|------|-----------|-------|------------|-----|-------|-------------|----------|
| OutboundGate send mapping and fallback delegation | `internal/harness/outboundgate/gate_test.go` | Unit | N/A (new package) | ✅ compile failed on missing `NewGate`, `Config`, and proposal `PaymentTarget` | ✅ focused gate tests passed | ✅ send mapping, non-send fallback, invalid schema deny-before-decider | ✅ gofmt and package suites passed |
| Deterministic realtime enforcement completeness | `internal/outbound/decider_test.go` | Unit | ✅ `go test ./internal/outbound ./internal/harness` passed before modifications | ✅ compile failed on missing authority-context fields and judge wiring | ✅ focused outbound tests passed | ✅ ambiguous recipient, ambiguous channel, third-party, channel, and payment-routing cases; judge spy asserted zero calls | ✅ helper extraction; focused and full suites passed |
| Judge seam and draft-only remediation | `internal/outbound/decider_test.go` | Unit | ✅ `go test ./internal/outbound ./internal/harness` passed before modifications | ✅ tests referenced missing `ToneJudge` / `ToneRubric` fields | ✅ focused outbound tests passed | ✅ pass, block with draft suggestion, judge error, and inconclusive result cases | ✅ gofmt and package suites passed |

## Test commands run for PR 2

- `go test ./internal/outbound ./internal/harness` — passed baseline before PR 2 production modifications.
- `go test ./internal/outbound ./internal/harness -run 'TestDeciderDeniesRecipient|TestDeciderUsesJudge|TestGate'` — RED failed on missing fields/seams.
- `go test ./internal/outbound ./internal/harness/outboundgate -run 'TestDeciderDeniesRecipient|TestDeciderUsesJudge|TestGate'` — passed GREEN.
- `go test ./internal/outbound ./internal/harness` — passed required focused command.
- `go test ./internal/harness/... ./internal/outbound && go test ./...` — passed after refactor; includes the new `internal/harness/outboundgate` subpackage.
- `go test ./internal/outbound ./internal/harness && go test ./internal/harness/... ./internal/outbound && go test ./...` — passed final verification after gofmt.

## Deviations from design in PR 2

- No evidence ledger persistence was added; this remains PR 3 by boundary.
- No campaign preflight, HTTP/CLI entrypoint, live send provider, or dashboard/UI changes were added.
- `draft_outbound_utterance` is not specially handled in the new adapter yet; non-send tools delegate to fallback per the PR 2 gate tests and the assigned non-goal boundary.
- Semantic rewrite suggestion is a bounded draft-only placeholder, not an auto-approved/sendable rewrite.

## PR 2 Review Remediation

Fresh reliability review found PR 2 blockers before continuing to PR 3:

- semantic tone/threat judge absence could allow a send after deterministic
  checks passed;
- `send_outbound_utterance` schema decoding accepted unsupported channel
  strings before decider evaluation;
- permission metadata lacked minimal action-context fields needed for later
  evidence correlation.

Remediation applied inside PR 2 only:

- added a fail-closed test for missing required tone judge;
- missing `ToneJudge` now denies with `judge_unavailable` after deterministic
  checks pass;
- unsupported send channels are rejected at `outboundgate` schema decoding
  before calling the decider;
- decision metadata now includes `action_kind` and `proposal_id` when present.

Verification after remediation:

- `go test ./internal/outbound ./internal/harness/...` — passed;
- `go test ./...` — passed.

Second reliability review found two remaining PR 2 blockers:

- high-confidence unknown judge outcomes still failed open;
- action/proposal metadata was not test-protected and invalid-schema denials
  lacked action context.

Second remediation applied inside PR 2 only:

- unknown judge outcomes now deny with `judge_invalid_result`;
- decider tests assert `action_kind` and `proposal_id` metadata;
- invalid send-schema denials now include `action_kind` and `proposal_id`
  metadata where the input supplies the proposal id;
- outboundgate tests assert schema-denial action metadata.

Verification after second remediation:

- `go test ./internal/outbound ./internal/harness/...` — passed;
- `go test ./...` — passed.

Final reliability review found two more PR 2 fail-closed/metadata gaps:

- nil authority context or bundle resolver dependencies could panic instead of
  returning a structured denial;
- decider-missing and decider-error gate paths returned bare denials without
  stable action/proposal metadata.

Final remediation applied inside PR 2 only:

- missing `ContextResolver` denies with `authority_context_unresolved`;
- missing `BundleResolver` denies with `policy_bundle_unresolved`;
- outboundgate decider-unconfigured and decider-error paths now return mapped
  outbound decisions with `action_kind`, `proposal_id`, and fail-closed codes;
- tests cover those paths.

Verification after final remediation:

- `go test ./internal/outbound ./internal/harness/...` — passed;
- `go test ./...` — passed.

## Remaining tasks after PR 2

- [ ] **High-risk evidence persistence for blocked realtime decisions**
- [ ] **Campaign preflight dry-run module**
- [ ] **Narrow preflight entrypoint wiring + request/response tests**

## Workload / PR boundary for PR 2

- Completed only PR 2.
- No PR 3/4/5 scope was implemented.
- New adapter subpackage means `go test ./internal/harness` does not include `internal/harness/outboundgate`; final verification also ran `go test ./internal/harness/...` to cover the adapter.

## Action context warnings for PR 2

- OpenSpec artifacts are untracked and belong to this active SDD scope.
- No files outside `/Users/ricardoaltamirano/Developer/vigia` were edited.
- No commit, push, or PR was created.

## Action context warnings

- OpenSpec artifacts are untracked and belong to this active SDD scope.
- No files outside `/Users/ricardoaltamirano/Developer/vigia` were edited.
- No commit, push, or PR was created.

## PR 3 Apply Update: High-risk evidence persistence for blocked realtime decisions

## Structured status consumed for PR 3

- Project: `vigia`
- Workspace root: `/Users/ricardoaltamirano/Developer/vigia`
- Change: `issue-10-realtime-guardrails-preflight`
- Artifact store: `openspec` (authoritative)
- Allowed edit root: `/Users/ricardoaltamirano/Developer/vigia`
- Strict TDD: active; runner `go test ./...`
- Delivery boundary: PR 3 only; PR 4/PR 5 explicitly deferred
- Workload guard: tasks forecast `Decision needed before apply: Yes`, `Chained PRs recommended: Yes`, `400-line budget risk: High`; parent supplied resolved PR 3 boundary and 400-line budget limit.

## Completed tasks

- [x] `High-risk evidence persistence for blocked realtime decisions`
  - Persisted checkbox confirmed in `openspec/changes/issue-10-realtime-guardrails-preflight/tasks.md` after implementation.
  - Added an `outbound.DecisionRecorder` seam and `RecordedDecision` / `DecisionRef` metadata so denied or approval-required enforcement decisions can attach interaction/evidence references.
  - Wired `outbound.Decider` to record only enforcement-mode denied/approval-required decisions; allowed decisions and dry-run denials do not append realtime enforcement evidence.
  - Added `postgres.OutboundDecisionRecorder` that reuses the existing `interaction_events -> evaluations -> detector_result_rows -> evidence_records` chain with `status = blocked_before_send` and append-only ledger semantics.
  - Persisted rule/fail-closed metadata as detector result rows, policy bundle version/ID through the existing evaluation/evidence fields, decision outcome as `overall_outcome = fail`, decision/proposal correlation in the bounded outbound transcript reference, and returned event/evidence refs.
  - Refactored the existing evaluation ledger append into a shared in-transaction helper so outbound interaction creation and evaluation/evidence append commit or roll back together.

## Files changed in PR 3

- `internal/outbound/types.go`
- `internal/outbound/decider.go`
- `internal/outbound/decider_test.go`
- `internal/postgres/adapters.go`
- `internal/postgres/evidence_integration_test.go`
- `openspec/changes/issue-10-realtime-guardrails-preflight/tasks.md`
- `openspec/changes/issue-10-realtime-guardrails-preflight/apply-progress.md`

## TDD Cycle Evidence for PR 3

| Task | Test File | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
|------|-----------|-------|------------|-----|-------|-------------|----------|
| Outbound decision recorder seam | `internal/outbound/decider_test.go` | Unit | ✅ prior `go test ./internal/outbound` coverage existed | ✅ `go test ./internal/outbound -run 'TestDeciderRecords|TestDeciderDoesNotRecord'` failed on missing `RecordedDecision`,`DecisionRef`, and`Decider.Recorder` | ✅ focused tests passed after adding recorder seam and metadata refs | ✅ covered blocked enforcement recording plus no recording for allowed enforcement/dry-run blocked decisions | ✅ gofmt and full suite passed |
| Postgres blocked-authority evidence chain | `internal/postgres/evidence_integration_test.go` | Integration | ✅ existing evidence/evaluation integration tests covered ledger append invariants | ✅ `go test ./internal/postgres -run 'TestOutboundAuthorityDecisionRecorder'` failed on missing `NewOutboundDecisionRecorderFromPool` | ✅ focused tests passed after adding the Postgres recorder | ✅ covered blocked interaction status, rule row, policy version, event/evidence refs, and rollback on FK failure | ✅ shared transaction helper extracted; required and full suites passed |

## Test commands run for PR 3

- `go test ./internal/outbound -run 'TestDeciderRecords|TestDeciderDoesNotRecord'` — RED failed on missing recorder seam/types.
- `go test ./internal/outbound -run 'TestDeciderRecords|TestDeciderDoesNotRecord'` — passed GREEN.
- `go test ./internal/postgres -run 'TestOutboundAuthorityDecisionRecorder'` — RED failed on missing Postgres recorder constructor.
- `go test ./internal/outbound ./internal/postgres -run 'TestDeciderRecords|TestDeciderDoesNotRecord|TestOutboundAuthorityDecisionRecorder'` — passed GREEN.
- `go test ./internal/postgres -run 'TestEvidence|TestEvaluation|Outbound|Authority'` — passed required focused verification.
- `go test ./...` — passed full verification.
- `go test ./internal/postgres -run 'TestEvidence|TestEvaluation|Outbound|Authority' && go test ./...` — passed after refactor.

## Deviations from design in PR 3

- No new ledger table was added; the existing interaction/evaluation/evidence chain represented the required metadata.
- The blocked outbound interaction stores a bounded correlation reference (`outbound:decision/{id}/proposal/{id}`) rather than raw proposed message text, preserving the design constraint against hidden reasoning or broad content persistence.
- No campaign preflight dry-run module, HTTP/CLI entrypoint, UI/dashboard change, or live send-provider integration was added.

## Remaining tasks after PR 3

- [ ] **Campaign preflight dry-run module**
- [ ] **Narrow preflight entrypoint wiring + request/response tests**

## Workload / PR boundary for PR 3

- Completed only PR 3.
- PR 4/PR 5 scope remains untouched.
- Implementation stayed on the existing ledger chain rather than introducing a new table.

## Action context warnings for PR 3

- OpenSpec artifacts are untracked and belong to this active SDD scope.
- No files outside `/Users/ricardoaltamirano/Developer/vigia` were edited.
- No commit, push, or PR was created.

## PR 4 Apply Update: Campaign preflight dry-run module

## Structured status consumed for PR 4

- Project: `vigia`
- Workspace root: `/Users/ricardoaltamirano/Developer/vigia`
- Change: `issue-10-realtime-guardrails-preflight`
- Artifact store: `openspec` (authoritative)
- Allowed edit root: `/Users/ricardoaltamirano/Developer/vigia`
- Strict TDD: active; runner `go test ./...`
- Delivery boundary: PR 4 only; PR 5 entrypoint explicitly deferred
- Workload guard: tasks forecast `Decision needed before apply: Yes`, `Chained PRs recommended: Yes`, `400-line budget risk: High`; parent supplied resolved PR 4 boundary.

## Completed tasks

- [x] `Campaign preflight dry-run module`
  - Persisted checkbox confirmed in `openspec/changes/issue-10-realtime-guardrails-preflight/tasks.md` after implementation.
  - Added a complete campaign artifact model in `internal/outbound` with campaign, recipient, step, schedule, brief, and finding types.
  - Added `PreflightService.Run` to expand every recipient/step pair into dry-run `DecisionRequest` values using `DecisionModeDryRun` and the existing outbound decider seam.
  - Returned actionable pass/fail briefs with rule codes, policy bundle version, component refs, remediation, context gaps, dry-run refs, and optional draft-only suggestions.
  - Kept dry-run references as `ref_type = dry_run_decision`, `mode = dry_run`; dry-run preflight does not create enforcement evidence refs.
  - No send/provider dependency or HTTP/CLI/UI entrypoint was added.

## Files changed in PR 4

- `internal/outbound/types.go`
- `internal/outbound/campaign_preflight.go`
- `internal/outbound/campaign_preflight_test.go`
- `openspec/changes/issue-10-realtime-guardrails-preflight/tasks.md`
- `openspec/changes/issue-10-realtime-guardrails-preflight/apply-progress.md`

## TDD Cycle Evidence for PR 4

| Task | Test File | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
|------|-----------|-------|------------|-----|-------|-------------|----------|
| Campaign preflight dry-run module | `internal/outbound/campaign_preflight_test.go` | Unit | ✅ prior outbound tests existed | ✅ `go test ./internal/outbound -run 'Preflight|Campaign'` failed on missing `PreflightService`, campaign artifact/brief types, and proposal campaign fields | ✅ focused preflight tests passed after adding the module | ✅ covered non-compliant complete campaign failure, compliant complete campaign pass, every recipient/step expansion, dry-run mode, dry-run ref separation from enforcement evidence refs, context gaps, and deterministic short-circuit before judge | ✅ gofmt and required full suite passed |

## Test commands run for PR 4

- `go test ./internal/outbound -run 'Preflight|Campaign'` — RED failed on missing preflight module/types.
- `go test ./internal/outbound -run 'Preflight|Campaign'` — passed GREEN.
- `go test ./internal/outbound -run 'Preflight|Campaign' && go test ./...` — passed required verification.

## Deviations from design in PR 4

- Implemented the preflight module directly in `internal/outbound/campaign_preflight.go` rather than a subpackage, preserving a small seam over the existing outbound decider.
- Dry-run refs are generated by the preflight module and returned as event refs; no dry-run ledger persistence was added.
- No provider integration exists in this module, so provider side effects are structurally absent and test-covered through the decider-only seam.
- No HTTP/CLI/API entrypoint, UI, launch flow, or live send-provider integration was added.

## Remaining tasks after PR 4

- [x] **Narrow preflight entrypoint wiring + request/response tests** (completed in PR 5 update below)

## Workload / PR boundary for PR 4

- Completed only PR 4.
- PR 5 entrypoint wiring remains untouched; `internal/httpapi` and `cmd/*` were not edited for this slice.
- No commit, push, or PR was created.

## Action context warnings for PR 4

- OpenSpec artifacts are untracked and belong to this active SDD scope.
- No files outside `/Users/ricardoaltamirano/Developer/vigia` were edited.
- Repository already contained prior PR1/PR2/PR3 working-tree changes before this PR4 slice.

## PR 5 Apply Update: Narrow preflight entrypoint wiring + request/response tests

## Structured status consumed for PR 5

- Project: `vigia`
- Workspace root: `/Users/ricardoaltamirano/Developer/vigia`
- Change: `issue-10-realtime-guardrails-preflight`
- Artifact store: `openspec` (authoritative)
- Strict TDD: active; runner `go test ./...`
- Delivery boundary: PR 5 only; build on PR1-PR4 and do not add campaign-management UI, broad routing, workflow orchestration, or live provider integration.

## Completed tasks

- [x] `Narrow preflight entrypoint wiring + request/response tests`
  - Persisted checkbox confirmed in `openspec/changes/issue-10-realtime-guardrails-preflight/tasks.md` after verification.
  - Inspected `internal/httpapi/httpapi.go` and selected HTTP as the smallest existing tenant-authenticated seam.
  - Added `POST /v1/campaigns/preflight` behind existing bearer API-key authentication.
  - Added `CampaignPreflightRunner` seam and `SetCampaignPreflight` wiring method for narrow service injection.
  - Maps request JSON into `outbound.CampaignArtifact`, forcing trusted tenant and actor scope from authentication.
  - Returns the PR4 preflight brief as JSON without adding campaign-management CRUD, UI, workflow orchestration, or live provider integration.

## Files changed in PR 5

- `internal/httpapi/httpapi.go`
- `internal/httpapi/httpapi_test.go`
- `openspec/changes/issue-10-realtime-guardrails-preflight/tasks.md`
- `openspec/changes/issue-10-realtime-guardrails-preflight/apply-progress.md`

## TDD Cycle Evidence for PR 5

| Task | Test File | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
|------|-----------|-------|------------|-----|-------|-------------|----------|
| Campaign preflight HTTP entrypoint | `internal/httpapi/httpapi_test.go` | HTTP unit | ✅ existing HTTP auth tests and PR4 preflight tests existed | ✅ test failed on missing route/service wiring before handler implementation | ✅ focused preflight HTTP tests passed after route, request mapping, and response handling | ✅ covered unauthorized requests before preflight, authenticated tenant scope, malformed request shape, and actionable brief response with dry-run refs/no evidence refs | ✅ gofmt and full suite passed |

## Test commands run for PR 5

- `go test ./internal/httpapi -run 'Preflight|Campaign'` — passed.
- `go test ./...` — passed.

## Deviations from design in PR 5

- Chose HTTP rather than a `cmd/*` entrypoint because `internal/httpapi` already contains tenant-authenticated API routes and tests.
- Added only one narrow route: `POST /v1/campaigns/preflight`.
- No campaign-management UI, CRUD, workflow orchestration, live provider integration, or commit was added.

## PR 5 Review Remediation

Fresh reliability review found PR 5 blockers before final verification:

- request body `actor_id` could override the authenticated API key actor;
- the endpoint encoded `outbound.PreflightBrief` without JSON tags and tests
  did not protect the public snake_case response contract.

Remediation applied inside PR 5 only:

- request body `actor_id` is ignored for authority scoping; `ActorID` now
  remains the authenticated API key id;
- added snake_case JSON tags for preflight brief/finding/ref/context-gap/draft
  response fields;
- HTTP tests now include a spoofed `actor_id` and assert authenticated actor
  scoping wins;
- HTTP tests now assert wire JSON uses snake_case keys and not PascalCase keys.

Verification after PR 5 remediation:

- `go test ./internal/httpapi -run 'Preflight|Campaign'` — passed;
- `go test ./...` — passed.

Second reliability review found the endpoint was test-only wired: `cmd/api`
constructed the HTTP server without a real preflight runner, so production would
return 500 for `POST /v1/campaigns/preflight`.

Second remediation applied inside PR 5 only:

- added `outbound.ProposalContextResolver` for trusted, expanded campaign
  dry-run proposals;
- campaign expansion now carries recipient timezone, relationship, authorized
  channels, and payment target into the proposal context;
- `postgres.BundleResolverAdapter` now also implements
  `outbound.ActiveBundleResolver` for authority/preflight decisions;
- `cmd/api/main.go` wires a real `outbound.PreflightService` using the
  proposal context resolver, active bundle resolver, contact-hours window, and
  configured tone judge/rubric.

Verification after second PR 5 remediation:

- `go test ./internal/outbound -run 'Preflight|Campaign|Proposal'` — passed;
- `go test ./internal/httpapi -run 'Preflight|Campaign'` — passed;
- `go test ./cmd/api ./internal/postgres ./...` — passed.

## Post-archive blocker remediation

A subsequent fresh 4R review found blockers after the initial archive:

- realtime `send_outbound_utterance` gate was not production-wired;
- production decider did not include the enforcement recorder;
- schema-invalid denials were not test-protected for evidence recording when
  enough proposal context exists;
- production preflight trusted request-supplied authority context too broadly;
- outbound evidence metadata did not have test-protected decision provenance.

Remediation applied:

- added `POST /v1/outbound/guardrails/decide`, which authenticates the API key
  and evaluates the proposed send through `outboundgate.NewGate`;
- `cmd/api` now wires a real outbound decider with
  `postgres.NewOutboundAuthorityContextResolverFromPool`, active bundle
  resolver, tone judge/rubric, and `OutboundDecisionRecorder`;
- campaign preflight now uses the same production decider;
- `OutboundAuthorityContextResolver` validates the debtor through a
  tenant-scoped Postgres lookup and uses the DB debtor timezone;
- `outboundgate` records schema-invalid denials through the recorder when
  tenant/debtor/proposed-at context is available;
- outbound evidence rows now always include an `authority_decision` detector row
  with `decision_id`, `decision_mode`, `decision_outcome`, `action_kind`, and
  `proposal_id` provenance;
- tests now cover the realtime decision endpoint, schema-denial recording,
  tenant-scoped context resolution/cross-tenant failure, and authority decision
  metadata.

Post-remediation verification:

- `go test ./internal/postgres -run 'OutboundAuthority'` — passed;
- `go test ./internal/harness/outboundgate ./internal/httpapi ./internal/postgres ./cmd/api` — passed;
- `go test ./...` — passed;
- LSP diagnostics on key files — no errors;
- final focused reliability review — no findings.

## Remaining tasks after PR 5

No unchecked implementation work units remain in `tasks.md` for issue #10.

## Workload / PR boundary for PR 5

- Completed only PR 5.
- Existing PR1-PR4 working-tree changes were preserved.
- No commit, push, or PR was created.
