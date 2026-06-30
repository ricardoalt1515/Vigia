# Tasks: Issue #19 Harness Tool Contracts and Synthetic Fixtures

## Work Unit 1 — `RiskClass` type and per-tool declarations

### RED: failing type-contract test
- [x] **[TEST FIRST]** Add `internal/harness/risk_test.go` — table-driven test asserting the three `RiskClass` constants (`read`, `draft`, `authority`) exist, are of type `RiskClass`, and are mutually distinct.
  - Test must fail before `risk.go` exists.
  - Satisfies: `harness-tools/spec.md` § "RiskClass taxonomy as a static tool-contract property" (all three scenarios).

### GREEN: minimal RiskClass implementation
- [x] Create `internal/harness/risk.go` — declare `RiskClass` type (string alias) and three constants: `RiskClassRead`, `RiskClassDraft`, `RiskClassAuthority`.
  - No logic, no imports beyond the bare minimum.
  - Constraint: do not modify any existing file in `internal/harness/`.

### Verify WU1
- [x] Run `go test ./internal/harness` — all tests green including new risk_test.go.
- [x] Run `go test ./...` — no regressions.

---

## Work Unit 2 — Embedded fixture loader and JSON fixtures

### Fixtures (data files — no Go logic yet)
- [x] Create directories `internal/harness/labtools/fixtures/cases/` and `internal/harness/labtools/fixtures/rules/`.
  - **Gatekeeper correction #1**: fixtures do NOT go under `data/synthetic/`. `data/synthetic/cases/.gitkeep` and `data/synthetic/harness-runs/.gitkeep` remain untouched. No `data/synthetic/embed.go` is created.
  - Both directories must exist before `//go:embed` compiles; add the JSON files (below) before adding embed.go.

- [x] Create `internal/harness/labtools/fixtures/cases/case-001.json` — synthetic Case fixture. Must carry all required fields non-empty:
  - `case_id` — unique fixture identifier (e.g., `CASE-SYN-001`).
  - `tenant_id` — synthetic, SYN-prefixed (e.g., `SYN-TENANT-001`).
  - `debtor` — structured value with synthetic display label `Debtor-Synthetic-001`; no real CURP, RFC, address, phone, or email.
  - `collector` — structured value with despacho identifier (e.g., `DESPACHO-SYN-01`) and display label.
  - `transcript` — array of 2+ utterances, each with `speaker` and `text` string fields; includes debtor speech (untrusted data, inert).
  - `channel` — e.g., `voice`.
  - `occurred_at` — RFC 3339 timestamp that places the contact outside 08:00–21:00 in `debtor_timezone` (e.g., `2024-03-15T23:30:00-06:00`).
  - `debtor_timezone` — `America/Mexico_City`.
  - `detector_results` — two entries: `{rule_code: "MX-REDECO-04", detector_kind: "deterministic", outcome: "hard_block"}` and `{rule_code: "MX-REDECO-05", detector_kind: "llm_judge", outcome: "hard_block", hitl_required: true}`.
  - `applicable_rule_ids` — `["MX-REDECO-04", "MX-REDECO-05"]`.
  - `evidence_metadata` — placeholder object (e.g., `{"status": "pending", "record_id": null}`); NOT a committed EvidenceRecord.
  - Satisfies: all requirements in `harness-synthetic-fixtures/spec.md`.

- [x] Create `internal/harness/labtools/fixtures/rules/rule-mx-redeco-04.json`:
  - `code`: `MX-REDECO-04`
  - `title`: a title referencing the contact-hours restriction
  - `description`: states contact permitted only on business days between 08:00 and 21:00 in debtor's timezone; contact outside this window is a hard block
  - `severity`: `hard_block`

- [x] Create `internal/harness/labtools/fixtures/rules/rule-mx-redeco-05.json`:
  - `code`: `MX-REDECO-05`
  - `title`: a title referencing the threatening-tone prohibition
  - `description`: states threats, offense, intimidation, and harassment are prohibited; violations trigger a hard block with mandatory human review
  - `severity`: `hard_block`

### Go types and embed file (no loader logic yet)
- [x] Create `internal/harness/labtools/fixtures.go` (package `labtools`) — declare Go structs mirroring the JSON shape:
  - `Utterance{Speaker, Text string}`
  - `DetectorResult{RuleCode, DetectorKind, Outcome string; HITLRequired bool}`
  - `EvidenceMetadata` as `map[string]any` (or a minimal placeholder struct)
  - `Debtor{Label string}` and `Collector{DespachoID, Label string}`
  - `SyntheticCase{CaseID, TenantID string; Debtor Debtor; Collector Collector; Transcript []Utterance; Channel, OccurredAt, DebtorTimezone string; DetectorResults []DetectorResult; ApplicableRuleIDs []string; EvidenceMetadata map[string]any}`
  - `SyntheticRule{Code, Title, Description, Severity string}`
  - `CaseStore map[string]SyntheticCase` (keyed by CaseID)
  - `RuleStore map[string]SyntheticRule` (keyed by Code)

- [x] Create `internal/harness/labtools/embed.go` (package `labtools`):
  - `//go:embed fixtures/cases/*.json fixtures/rules/*.json`
  - `var fixtureFS embed.FS`
  - No other logic. Per gatekeeper correction #1: this is in `package labtools`, not a separate `synthfixtures` package.

### RED: failing loader tests
- [x] **[TEST FIRST]** Create `internal/harness/labtools/loader_test.go` — all tests must fail before `loader.go` exists. Cover:
  - Valid embedded load: `Load()` succeeds; `CaseStore` contains `case-001`; `RuleStore` contains both `MX-REDECO-04` and `MX-REDECO-05`.
  - Required field missing (inline-mutated JSON bytes passed to a test-only loader variant): load returns a non-nil error.
  - Dangling rule reference (applicable_rule_id not present in RuleStore): load returns a non-nil error.
  - PII-shaped value in debtor field (email regex or phone regex match): load returns a non-nil error.
  - Rule-reference integrity: every `applicable_rule_id` in the loaded Case resolves to a non-nil `SyntheticRule`.
  - No orphan rules: every rule in `RuleStore` appears in the Case's `applicable_rule_ids`.
  - Determinism: calling `Load()` twice returns structurally equal `CaseStore` and `RuleStore`.
  - Fixture loads without external services (structural: embedded data, always true in clean env).
  - Satisfies: `harness-synthetic-fixtures/spec.md` § Rule-reference integrity; § Fixtures embedded; § No-PII.

### GREEN: loader implementation
- [x] Create `internal/harness/labtools/loader.go` — implement `Load() (CaseStore, RuleStore, error)`:
  - Read all `fixtures/cases/*.json` entries from `fixtureFS`; unmarshal each into `SyntheticCase`.
  - Read all `fixtures/rules/*.json` entries from `fixtureFS`; unmarshal each into `SyntheticRule`.
  - Validate required fields: non-empty `case_id`, `tenant_id`, `channel`, `occurred_at`, `debtor_timezone`; non-empty `transcript`; each utterance has non-empty `speaker` and `text`; each rule has non-empty `code`, `title`, `severity`.
  - Validate rule-reference integrity: every code in `applicable_rule_ids` and every `detector_result.rule_code` must resolve to a loaded `SyntheticRule`; no dangling refs allowed.
  - Validate no-PII shape: `debtor` display label must not match an email or phone number regex; any field matching a real-identity pattern fails load.
  - Build and return `CaseStore` and `RuleStore`.
  - Fail-closed: any validation error aborts immediately and returns a descriptive error.
  - No `time.Now()` at load; no filesystem access beyond `fixtureFS`.

### Verify WU2
- [x] Run `go test ./internal/harness/labtools` — all loader tests green.
- [x] Run `go test ./...` — no regressions.

---

## Work Unit 3 — Read tools: `read_case`, `read_policy_rule`, `list_applicable_rules`

### RED: failing read-tool tests
- [x] **[TEST FIRST]** Create `internal/harness/labtools/tools_read_test.go` — all tests must fail before tool impls exist. Cover:
  - `read_case` with fixture `case_id` → `ToolStatusSuccess`; response carries non-empty `tenant_id`, `debtor`, `collector`, `transcript`, `channel`, `occurred_at`, `debtor_timezone`, `detector_results`, `applicable_rule_ids`, `evidence_metadata`; `tenant_id` matches fixture value; `transcript` is a non-empty typed `[]Utterance`.
  - `read_case` with unknown `case_id` → result status is not `success`; `Reason` is non-empty.
  - `read_policy_rule` with `MX-REDECO-04` → `ToolStatusSuccess`; response `code = "MX-REDECO-04"`; `severity = "hard_block"`; `title` and `description` non-empty.
  - `read_policy_rule` with `MX-REDECO-05` → `ToolStatusSuccess`; `severity = "hard_block"`.
  - `read_policy_rule` with unknown `rule_code` → result status not `success`; `Reason` non-empty.
  - `list_applicable_rules` with fixture `case_id` → `ToolStatusSuccess`; rules list includes summaries for both `MX-REDECO-04` and `MX-REDECO-05` with `code`, `title`, `severity`; order matches `applicable_rule_ids` from fixture.
  - `list_applicable_rules` with unknown `case_id` → result status not `success`; `Reason` non-empty.
  - Determinism: calling each read tool twice with the same input returns `reflect.DeepEqual` results.
  - Transcript content is inert typed data: each element is `Utterance{Speaker, Text string}`; test asserts result type (structural, not content-routing assertion).
  - Satisfies: `harness-tools/spec.md` § read_case; § read_policy_rule; § list_applicable_rules; § deterministic fixture-backed; § tenant scoping; § untrusted-data invariant.

### GREEN: typed contracts, catalog, and read tool impls
- [x] Create `internal/harness/labtools/contracts.go` — typed Request/Response DTOs and JSON-roundtrip codec:
  - `ReadCaseRequest{CaseID string}` / `ReadCaseResponse` carrying a `SyntheticCaseView` with all spec-required fields (`TenantID`, `Debtor`, `Collector`, `Transcript []Utterance`, `Channel`, `OccurredAt`, `DebtorTimezone`, `DetectorResults []DetectorResult`, `ApplicableRuleIDs []string`, `EvidenceMetadata map[string]any`).
  - `ReadPolicyRuleRequest{RuleCode string}` / `ReadPolicyRuleResponse{Rule SyntheticRuleView}` where `SyntheticRuleView{Code, Title, Description, Severity string}`.
  - `ListApplicableRulesRequest{CaseID string}` / `ListApplicableRulesResponse{Rules []RuleSummary}` where `RuleSummary{Code, Title, Severity string}`.
  - Draft DTOs (stubs only; extended in WU4): `DraftEvidenceManifestRequest`, `DraftSupervisorNoteRequest` — declare empty structs for now so catalog and tools.go compile.
  - `decode[T any](m map[string]any) (T, error)` — JSON round-trip (marshal map → unmarshal into T).
  - `encode(v any) (map[string]any, error)` — JSON round-trip (marshal v → unmarshal into map).
  - No new dependencies; stdlib `encoding/json` only.

- [x] Create `internal/harness/labtools/catalog.go` — name-to-`RiskClass` catalog:
  - `var toolRiskClasses = map[string]harness.RiskClass{...}` mapping all 5 read/draft tool names to their class and all 4 authority names (`append_evidence`, `update_case_state`, `submit_report`, `block_campaign`) to `harness.RiskClassAuthority`.
  - `riskClassFor(name string) (harness.RiskClass, bool)` — returns `(class, true)` if found, `("", false)` if not.

- [x] Create `internal/harness/labtools/tools.go` (read tools portion) — `ReadCaseTool`, `ReadPolicyRuleTool`, `ListApplicableRulesTool` implementing `harness.Tool`:
  - Each struct embeds `CaseStore` and/or `RuleStore` (passed in at construction, not global state).
  - `RiskClass() harness.RiskClass` method on each concrete type returning the appropriate constant (satisfies spec "static RiskClass property" on the contract).
  - `Execute(ctx, call)`: decode `call.Input` via `decode[T]`, validate non-empty required fields, query store, build typed response, encode via `encode`, return `ToolResult{Status: ToolStatusSuccess, Output: encoded}`.
  - Unknown CaseID/RuleCode → `ToolResult{Status: ToolStatusNotFound, Reason: "case not found: <id>"}` (or rule equivalent).
  - Transcript passed through as `[]Utterance`; never parsed as instructions or control flow.
  - `list_applicable_rules` filters and orders by the Case's `ApplicableRuleIDs` slice (intersection with RuleStore, original order preserved).
  - `Registry(stores) harness.ToolRegistry` — returns a `ToolRegistry` with only the three read tools registered (draft tools added in WU4).

### Verify WU3
- [x] Run `go test ./internal/harness/labtools` — all read-tool tests green.
- [x] Run `go test ./...` — no regressions.

---

## Work Unit 4 — Draft tools: `draft_evidence_manifest`, `draft_supervisor_note`

### RED: failing draft-tool tests
- [x] **[TEST FIRST]** Create `internal/harness/labtools/tools_draft_test.go` — all tests must fail before draft tool impls exist. Cover:
  - `draft_evidence_manifest` with valid `{case_id, rule_codes: ["MX-REDECO-04"], findings: "Out-of-hours contact detected"}` → `ToolStatusSuccess`; response `case_id` equals request `case_id`; response `rule_codes` equals request `rule_codes`; response `findings` equals request `findings` unchanged (gatekeeper correction #3: echo, do not recompute); `authoritative = false`; `persisted = false`.
  - `draft_evidence_manifest` → response `proposed_at` equals the package-level fixed RFC 3339 constant (not wall clock; gatekeeper correction #2).
  - `draft_supervisor_note` with valid `{case_id, rule_codes: ["MX-REDECO-05"], note_body: "Supervisor review required"}` → `ToolStatusSuccess`; response `case_id` equals request; response `rule_codes` equals request; response `note_body` equals request `note_body` unchanged (gatekeeper correction #3); `authoritative = false`; `persisted = false`.
  - `draft_supervisor_note` → response `proposed_at` equals the same fixed RFC 3339 constant.
  - Determinism: calling each draft tool twice with the same input returns `reflect.DeepEqual` results (guaranteed by constant `proposed_at` + echo fields).
  - No mutation: `CaseStore` and `RuleStore` are structurally unchanged before and after a draft tool call.
  - Satisfies: `harness-tools/spec.md` § draft_evidence_manifest; § draft_supervisor_note; plus gatekeeper corrections #2 and #3.

### GREEN: draft DTO extensions and draft tool impls
- [x] Extend `internal/harness/labtools/contracts.go`:
  - Fill in `DraftEvidenceManifestRequest{CaseID string; RuleCodes []string; Findings string}`.
  - `DraftEvidenceManifestResponse{CaseID string; RuleCodes []string; Findings string; ProposedAt string; Authoritative bool; Persisted bool}`.
  - `DraftSupervisorNoteRequest{CaseID string; RuleCodes []string; NoteBody string}`.
  - `DraftSupervisorNoteResponse{CaseID string; RuleCodes []string; NoteBody string; ProposedAt string; Authoritative bool; Persisted bool}`.
  - `const draftProposedAt = "2025-01-01T00:00:00Z"` — fixed deterministic RFC 3339 value (gatekeeper correction #2: NOT `time.Now()`, not omitted; satisfies RFC-3339 requirement and determinism simultaneously).

- [x] Extend `internal/harness/labtools/tools.go` with draft tools:
  - `DraftEvidenceManifestTool` implementing `harness.Tool`:
    - `RiskClass() harness.RiskClass` → `harness.RiskClassDraft`.
    - `Execute`: decode input into `DraftEvidenceManifestRequest`; echo `CaseID`, `RuleCodes`, and `Findings` from request unchanged; set `ProposedAt = draftProposedAt`; `Authoritative = false`; `Persisted = false`; encode response.
    - No reads from CaseStore, no writes anywhere.
  - `DraftSupervisorNoteTool` implementing `harness.Tool`:
    - `RiskClass() harness.RiskClass` → `harness.RiskClassDraft`.
    - `Execute`: decode input into `DraftSupervisorNoteRequest`; echo `CaseID`, `RuleCodes`, and `NoteBody` from request unchanged; set `ProposedAt = draftProposedAt`; `Authoritative = false`; `Persisted = false`; encode response.
    - No reads from CaseStore, no writes anywhere.
  - Update `Registry(stores)` to include all five tools (three read + two draft).

### Verify WU4
- [x] Run `go test ./internal/harness/labtools` — all draft-tool tests green.
- [x] Run `go test ./...` — no regressions.

---

## Work Unit 5 — `LabPermissionGate` and authority-gate integration test

### RED: failing gate unit tests
- [x] **[TEST FIRST]** Create `internal/harness/labtools/gate_test.go` — table-driven matrix; all tests must fail before `gate.go` exists. Cover:
  - `read_case` → decision kind `allowed`.
  - `read_policy_rule` → decision kind `allowed`.
  - `list_applicable_rules` → decision kind `allowed`.
  - `draft_evidence_manifest` → decision kind `allowed`.
  - `draft_supervisor_note` → decision kind `allowed`.
  - `append_evidence` → decision kind `denied`; never `allowed`.
  - `update_case_state` → decision kind `denied`.
  - `submit_report` → decision kind `denied`.
  - `block_campaign` → decision kind `denied`.
  - Unknown name (e.g., `mystery_tool`) → decision kind `denied` (fail-closed).
  - Determinism: `Decide` called twice with the same `ToolCall` returns equal `PermissionDecision` values.
  - Satisfies: `harness-tools/spec.md` § Risk-class-aware lab permission gate; § Authority-bearing tools absent and never execute.

### RED: failing integration test
- [x] **[TEST FIRST]** Create `internal/harness/labtools_integration_test.go` (file lives in `internal/harness/`, package `harness_test`) — NOTE: Go import-cycle constraint required using external test package instead of `package harness`; a minimal `staticModelProvider` is defined locally (not a duplicate of `queuedModelProvider` — it is simpler and has no slice queuing). All tests must fail before `gate.go` is in place. Cover:
  - Authority proposal: model returns `ToolCall{Name: "append_evidence"}` → `RunStep` returns `StepStatusPermissionDenied`; `result.ToolResult.Status == ToolStatusDenied`.
  - Read proposal: model returns `ToolCall{Name: "read_case", Input: map[string]any{"case_id": "CASE-SYN-001"}}` → `RunStep` returns `StepStatusCompleted`; `result.ToolResult.Status == ToolStatusSuccess`; output contains `tenant_id`.
  - Runtime is constructed with `labtools.Registry(stores)` and `labtools.NewLabPermissionGate()` wired into `harness.Runtime`.
  - `stores` is loaded via `labtools.Load()` at test setup; no external service needed.
  - Satisfies: `harness-tools/spec.md` § Authority tool call stopped and never executes; § read_case returns complete fixture data; end-to-end LabPermissionGate + harness.Runtime integration.

### GREEN: `LabPermissionGate` implementation
- [x] Create `internal/harness/labtools/gate.go` — `LabPermissionGate` implementing `harness.PermissionGate`:
  - `NewLabPermissionGate() *LabPermissionGate` constructor (no config needed).
  - `Decide(ctx context.Context, call harness.ToolCall) harness.PermissionDecision`:
    1. Look up `call.Name` in `riskClassFor()` from `catalog.go`.
    2. If class is `harness.RiskClassRead` or `harness.RiskClassDraft` → `PermissionDecision{Kind: harness.PermissionAllowed}`.
    3. If class is `harness.RiskClassAuthority` or name not found (fail-closed) → `PermissionDecision{Kind: harness.PermissionDenied, Reason: "authority-bearing or unregistered tool"}`.
  - No external calls; pure in-memory catalog lookup. Deterministic.

### TRIANGULATE WU5
- [x] Confirm integration test: verify that `result.ToolResult.Status == ToolStatusDenied` propagates correctly from the runtime for the authority tool call (cross-checks `harness.Runtime` seam behavior from #18 is unchanged).

### Verify WU5
- [x] Run `go test ./internal/harness/labtools` — gate unit tests green.
- [x] Run `go test ./internal/harness` — integration test green.
- [x] Run `go test ./...` — full suite green; no regressions.

---

## Final Verification

- [x] Run full Go verification: `go test ./...`. All packages green.
- [x] Review scope: confirm changed files are limited to `internal/harness/risk.go`, `internal/harness/risk_test.go`, `internal/harness/labtools_integration_test.go`, `internal/harness/labtools/**`, and SDD artifacts. No DB migrations, no sqlc, no HTTP handlers, no MCP, no Bedrock, no EvidenceRecord writes, no modifications to any file that existed before this change (`tools.go`, `permissions.go`, `runtime.go`, `events.go`, `budget.go`, `validation.go`, `model.go`, `data/synthetic/**`).

---

## Apply Notes

- Strict TDD is active. Author the failing test before any production code for each behavior slice.
- Tests prove behavioral contracts and runtime invariants, not field declarations or type existence alone.
- **Gatekeeper correction #1**: fixtures live in `internal/harness/labtools/fixtures/{cases,rules}/`; `embed.go` is in `package labtools`; `data/synthetic/cases/.gitkeep` is untouched; no `data/synthetic/embed.go` is created.
- **Gatekeeper correction #2**: `proposed_at` in all draft tool responses is a package-level fixed RFC 3339 constant — never `time.Now()`, never omitted.
- **Gatekeeper correction #3**: draft tool responses echo `findings` (manifest) and `note_body` (note) from the request unchanged; do not replace with a computed summary.
- **Gatekeeper correction #4**: the WU5 integration test file lives in `internal/harness/` as `package harness`, co-located with `runtime_test.go`, so it can access `queuedModelProvider` without introducing a duplicate model stub.
- Do not modify any file from #18: `internal/harness/tools.go`, `permissions.go`, `runtime.go`, `events.go`, `budget.go`, `validation.go`, `model.go`. ADD only.
- Stop before editing anything outside `internal/harness/**` (excluding SDD artifacts) unless explicitly approved.

---

## Review Workload Forecast

Estimated changed lines by work unit:

| Work Unit | Files | Est. Lines |
|-----------|-------|------------|
| WU1 | risk.go + risk_test.go | ~30 |
| WU2 | embed.go + fixtures.go + loader.go + loader_test.go + 3 JSON fixtures (~60 lines each) | ~500 |
| WU3 | contracts.go + catalog.go + tools.go (read portion) + tools_read_test.go | ~320 |
| WU4 | contracts.go (draft extension) + tools.go (draft portion) + tools_draft_test.go | ~200 |
| WU5 | gate.go + gate_test.go + labtools_integration_test.go | ~180 |
| **Total** | | **~1,230** |

**Chained PRs recommended: Yes**

**400-line budget risk: High** (~3x the 400-line budget)

**Decision needed before apply: Yes**

Proposed PR split (stacked-to-main):

| PR | Work Units | Estimated Lines | Budget Risk |
|----|------------|-----------------|-------------|
| PR1 | WU1 + WU2 (RiskClass type + fixtures + loader) | ~530 | Medium-High |
| PR2 | WU3 (read tools + typed contracts + catalog) | ~320 | Low |
| PR3 | WU4 + WU5 (draft tools + permission gate + integration) | ~380 | Low |

PR1 slightly exceeds 400 lines; the loader and its fixtures are truly atomic (test cannot pass without fixture, fixture cannot compile without embed, embed cannot compile without directory structure). PR2 and PR3 stay within budget. Each PR ends at a fully green `go test ./...` and each is independently reviewable: PR1 is pure data + validation, PR2 is read-path tool contracts, PR3 is draft-path + authority enforcement.
