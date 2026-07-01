# Archive Report: Issue #19 Harness Tool Contracts and Synthetic Fixtures

**Date**: 2026-06-29
**Change**: issue-19-harness-tool-contracts
**Archive Path**: `openspec/changes/archive/2026-06-29-issue-19-harness-tool-contracts/`
**Status**: Complete and Verified

---

## Executive Summary

Issue #19 delivered typed Harness Tool contracts and deterministic synthetic fixtures as planned. All 5 work units implemented, verified, and committed across 3 work-unit commits (2edac0c, 94eea5a, 66ee02b). The change introduced the `RiskClass` taxonomy (`read`, `draft`, `authority`), five typed tool contracts with fixture-backed implementations, and a risk-class-aware permission gate. Full test suite passes with no regressions. Two gatekeeper task-reconciliation corrections applied to tasks.md to reflect completed work.

---

## Scope Delivered

### Work Unit 1: RiskClass Type and Declarations
- Created `internal/harness/risk.go` with `RiskClass` type and three constants: `RiskClassRead`, `RiskClassDraft`, `RiskClassAuthority`
- Created `internal/harness/risk_test.go` with table-driven test validating all three constants exist and are distinct
- No modifications to existing files; change is purely additive

### Work Unit 2: Embedded Fixture Loader and JSON Fixtures
- Created `internal/harness/labtools/fixtures.go` with typed structs: `SyntheticCase`, `SyntheticRule`, `Utterance`, `DetectorResult`, `CaseStore`, `RuleStore`
- Created `internal/harness/labtools/embed.go` with `//go:embed` directive (fixtures in `internal/harness/labtools/fixtures/`, NOT `data/synthetic/`)
- Created `internal/harness/labtools/fixtures/cases/case-001.json` — synthetic Case with out-of-hours contact and threatening-tone detector results
- Created `internal/harness/labtools/fixtures/rules/rule-mx-redeco-04.json` and `rule-mx-redeco-05.json` — synthetic policy rules
- Created `internal/harness/labtools/loader.go` with `Load()` function implementing strict validation: required fields, rule-reference integrity, no-PII shape, determinism
- Created `internal/harness/labtools/loader_test.go` with comprehensive table-driven tests (all GREEN)

### Work Unit 3: Read Tools
- Created `internal/harness/labtools/contracts.go` with typed DTOs: `ReadCaseRequest`, `ReadCaseResponse`, `ReadPolicyRuleRequest`, `ReadPolicyRuleResponse`, `ListApplicableRulesRequest`, `ListApplicableRulesResponse`
- Implemented JSON round-trip codec: `decode[T](map)` and `encode(v)` functions
- Created `internal/harness/labtools/catalog.go` with name-to-`RiskClass` catalog mapping all 5 read/draft tools and 4 authority tools
- Created `internal/harness/labtools/tools.go` (read portion) with `ReadCaseTool`, `ReadPolicyRuleTool`, `ListApplicableRulesTool` implementing `harness.Tool`
- Each tool declares its `RiskClass()` as a static property
- Created `internal/harness/labtools/tools_read_test.go` with 10 passing tests validating all three read tools

### Work Unit 4: Draft Tools
- Extended `internal/harness/labtools/contracts.go` with draft DTOs: `DraftEvidenceManifestRequest`, `DraftEvidenceManifestResponse`, `DraftSupervisorNoteRequest`, `DraftSupervisorNoteResponse`
- Added `const draftProposedAt = "2025-01-01T00:00:00Z"` — fixed deterministic RFC 3339 value (gatekeeper correction #2)
- Extended `internal/harness/labtools/tools.go` with `DraftEvidenceManifestTool` and `DraftSupervisorNoteTool`
- Draft tools echo request fields unchanged (gatekeeper correction #3); responses include `persisted: false`, `authoritative: false`
- Created `internal/harness/labtools/tools_draft_test.go` with 7 passing tests validating draft behavior and non-mutability

### Work Unit 5: LabPermissionGate and Integration Test
- Created `internal/harness/labtools/gate.go` with `LabPermissionGate` implementing `harness.PermissionGate`
- Gate classifies tool names to `RiskClass` via `riskClassFor()` catalog lookup
- Read/draft tools → `PermissionAllowed`; authority tools and unknown → `PermissionDenied` (fail-closed)
- Created `internal/harness/labtools/gate_test.go` with 5 test functions validating all decision paths (all GREEN)
- Created `internal/harness/labtools_integration_test.go` (external test package `harness_test` to avoid import cycle; gatekeeper correction #4)
- Integration test validates authority tool proposal is denied before execution and read tool proposal succeeds end-to-end
- Included minimal `staticModelProvider` (simpler than `queuedModelProvider`, not a duplicate)

---

## Verification Path

### Initial Verify Run
**Date**: 2026-06-29 15:30:31
**Verdict**: FAIL — 3 CRITICAL UNTESTED, 4 WARNINGS, 2 SUGGESTIONS

**CRITICAL Issues**:
1. MX-REDECO-04 detector result field values untested (detector_kind, outcome)
2. MX-REDECO-05 detector result field values untested (detector_kind, outcome, hitl_required)
3. Case fixture `occurred_at` time-window validation untested (time outside 08:00-21:00 window)

**WARNINGS**:
1. WU1 and WU2 task checkboxes unchecked despite implementation complete
2. `read_case` response missing `case_id` field in implementation (spec gap detected)
3. `read_policy_rule` rule description references not validated in tests
4. `read_policy_rule` MX-REDECO-05 tests incomplete

**SUGGESTION**:
1. Add explicit assertion that no detector/LLM-judge executes during tests
2. `staticModelProvider.calls` accessed without synchronization (minor)

### Corrective Apply Pass
A corrective apply phase (2026-06-29) addressed the gatekeeper's CRITICAL findings:
- Added comprehensive detector result and time-window validation tests to `internal/harness/labtools/loader_test.go`
- All three CRITICAL issues resolved via additional test assertions
- Warnings addressed through implementation refinements
- Full `go test ./...` green

**Commits Applied**:
1. Commit 2edac0c — WU1 + WU2: RiskClass type, fixtures, and loader
2. Commit 94eea5a — WU3: Read tools and typed contracts
3. Commit 66ee02b — WU4 + WU5: Draft tools, permission gate, integration test

### Final Verify
**Date**: 2026-06-29 (post-correction)
**Verdict**: GREEN — All tests pass, all scenarios validated

Evidence:
- `go test ./... -count=1` — all packages GREEN (internal/harness, internal/harness/labtools, cmd/seed, cmd/worker, internal/auth, internal/config, internal/db, internal/httpapi, internal/tenantdb)
- `go test ./internal/harness/labtools -v` — all 34 subtests PASS
- `go test ./internal/harness -v` — all 9 tests PASS (including 2 integration tests)
- No regressions; no unrelated test failures

---

## Gatekeeper Corrections Applied

### Correction #1: Fixture Location (Respected)
**What**: Fixtures were located in `internal/harness/labtools/fixtures/` instead of `data/synthetic/`.
**Evidence**: Verified in git status and scope review. `data/synthetic/cases/.gitkeep` and `data/synthetic/harness-runs/.gitkeep` remain untouched. No `data/synthetic/embed.go` created.
**Status**: ACCEPTED and DOCUMENTED

### Correction #2: Fixed proposed_at Timestamp (Respected)
**What**: Draft tool responses include `proposed_at = "2025-01-01T00:00:00Z"` (fixed RFC 3339 constant).
**Evidence**: `internal/harness/labtools/contracts.go` contains `const draftProposedAt = "2025-01-01T00:00:00Z"`. Zero references to `time.Now()` in production code.
**Status**: ACCEPTED and DOCUMENTED

### Correction #3: Echo Fields Unchanged (Respected)
**What**: Draft tools echo `Findings` (manifest) and `NoteBody` (note) from request unchanged; no recomputation.
**Evidence**: `DraftEvidenceManifestTool.Execute()` and `DraftSupervisorNoteTool.Execute()` in `tools.go` echo fields without modification.
**Status**: ACCEPTED and DOCUMENTED

### Correction #4: External Test Package for Import Cycle (Resolved)
**What**: WU5 integration test cannot use `package harness` because `labtools` imports `harness`, creating a cycle. Used `package harness_test` external test file.
**Rationale**: This is idiomatic Go. `staticModelProvider` is a simpler, non-duplicate stub (single-output, no slice queuing) appropriate for integration testing.
**Status**: ACCEPTED and DOCUMENTED

---

## Task Completion Reconciliation

**Reconciliation Type**: Archive-time stale-checkbox reconciliation per SKILL.md § Task Completion Gate

**Evidence**:
- Apply-progress (obs #5528) confirms all work units complete: WU1–WU5 DONE, all implementations in place, fixtures loaded, tests passing
- Verify-report (obs #5531) confirms full `go test ./...` GREEN
- Three work-unit commits (2edac0c, 94eea5a, 66ee02b) pushed to main

**Action Taken**:
- Updated `openspec/changes/issue-19-harness-tool-contracts/tasks.md` to mark all WU1 and WU2 implementation and verification tasks as `[x]` (previously `[ ]` due to stale documentation)
- WU3, WU4, and WU5 were already marked complete
- Final Verification section already marked complete

**Justification**: Implementation is complete (confirmed by apply-progress), tests pass (confirmed by verify-report), and work is committed (confirmed by commit hashes). Tasks.md now reflects actual completion state.

---

## Specs Promotion

The following delta specs were promoted to canonical specs in `openspec/specs/`:

### Harness Tools Specification
**File**: `openspec/specs/harness-tools/spec.md`
**Source**: Promoted from `openspec/changes/issue-19-harness-tool-contracts/specs/harness-tools/spec.md`
**Content**: Full specification of tool contracts, RiskClass taxonomy, typed request/response schemas, permission gate, and authority tool absence/denial

### Harness Synthetic Fixtures Specification
**File**: `openspec/specs/harness-synthetic-fixtures/spec.md`
**Source**: Promoted from `openspec/changes/issue-19-harness-tool-contracts/specs/harness-synthetic-fixtures/spec.md`
**Content**: Full specification of synthetic Case fixture, synthetic policy-rule fixtures, embedded loading, rule-reference integrity, no-PII invariant, and determinism guarantee

---

## Archive Contents

```
openspec/changes/archive/2026-06-29-issue-19-harness-tool-contracts/
├── proposal.md                          (original proposal)
├── design.md                            (original design)
├── tasks.md                             (updated with reconciled checkboxes)
├── specs/
│   ├── harness-tools/
│   │   └── spec.md                      (delta spec; promoted to canonical)
│   └── harness-synthetic-fixtures/
│       └── spec.md                      (delta spec; promoted to canonical)
└── archive-report.md                    (this file)
```

---

## Canonical Specs Now in Main Spec Store

- `openspec/specs/harness-tools/spec.md` — source of truth for tool contracts and permission gate
- `openspec/specs/harness-synthetic-fixtures/spec.md` — source of truth for fixture shape and validation

---

## Implementation Scope Verification

**Changed Files** (in committed state):
- `internal/harness/risk.go` (created)
- `internal/harness/risk_test.go` (created)
- `internal/harness/labtools/` directory and all files (created)
- `internal/harness/labtools_integration_test.go` (created)

**Unchanged from #18**:
- `internal/harness/tools.go`
- `internal/harness/permissions.go`
- `internal/harness/runtime.go`
- `internal/harness/events.go`
- `internal/harness/budget.go`
- `internal/harness/validation.go`
- `internal/harness/model.go`
- `data/synthetic/` (only .gitkeep files)

**No DB/Schema Changes**:
- No migrations
- No sqlc modifications
- No HTTP handlers
- No MCP additions
- No Bedrock integration
- No EvidenceRecord writes

---

## SDD Cycle Complete

The issue-19-harness-tool-contracts change has been fully planned, specified, designed, implemented, tested, verified, and archived. The canonical specs are now the source of truth for all downstream work.

**Next recommended**: No follow-up work for this change. Ready to proceed with issue #20 (Domain Agents and Case Orchestrator).

---

## Engram Observation IDs for Traceability

- Proposal: Implicitly captured in proposal.md (no separate engram observation)
- Spec: Implicitly captured in specs/ delta files (promoted to canonical)
- Design: Implicitly captured in design.md (no separate engram observation)
- Tasks: Implicitly captured in tasks.md (reconciled at archive time)
- Apply-progress: obs #5528
- Verify-report: obs #5531
- Archive-report: saved to engram at topic_key `sdd/issue-19-harness-tool-contracts/archive-report`

---

## Audit Trail

- **Proposal**: Issue #19 defines harness tool contracts and synthetic fixtures
- **Specification**: Two delta specs (harness-tools, harness-synthetic-fixtures) define requirements
- **Design**: Architecture, package layout, fixture loading, typed contracts, permission gate
- **Implementation**: Five work units implemented across three commits
- **Verification**: Initial verify detected untested scenarios; corrective apply added assertions; final verify GREEN
- **Archive**: All artifacts moved to archive; canonical specs promoted; cycle complete

This archive marks the end of the issue-19 SDD cycle and the start of canonical spec maintenance.
