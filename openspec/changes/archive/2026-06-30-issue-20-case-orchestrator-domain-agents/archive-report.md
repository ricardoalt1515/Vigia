# Archive Report: Issue #20 Deterministic Case Orchestrator and Domain Agents

**Date**: 2026-06-30
**Change**: issue-20-case-orchestrator-domain-agents
**Archive Path**: `openspec/changes/archive/2026-06-30-issue-20-case-orchestrator-domain-agents/`
**Status**: Complete and Verified

---

## Executive Summary

Issue #20 delivered a deterministic Case orchestrator and four Domain Agents for the Harness compliance workflow as planned. All 4 work units implemented and verified successfully (commits 541bfd2, a2e9daa, d0d03ab). The change introduced the orchestrator package `internal/harness/caseflow/` with typed Handoff Artifacts, per-agent validators enforcing authority boundaries, and a bounded retry-once-with-feedback mechanism. Verification verdict: PASS WITH WARNINGS. The single WARNING regarding e2e test use of labtools stubs has been RESOLVED: this change is stacked on issue-19, which has now merged, and e2e tests now use real `labtools.Load()`, `labtools.Registry()`, and `labtools.NewLabPermissionGate()`. Full test suite green; no regressions.

---

## Scope Delivered

### Work Unit 1: Handoff types, CaseBrief, and CaseStatus (handoff.go)
- Created `internal/harness/caseflow/handoff.go` with:
  - `HandoffKind` constants: `KindPolicyExplanation`, `KindCaseInvestigation`, `KindEvidenceManifestDraft`, `KindSupervisorNoteDraft`
  - `HandoffArtifact` interface: `Kind() HandoffKind`, `CaseRef() string`
  - `CaseStatus` constants: `CaseStatusComplete = "complete"`, `CaseStatusIncomplete = "incomplete"`
  - Four typed Handoff Artifact structs with JSON tags: `PolicyExplanation`, `CaseInvestigation`, `EvidenceManifestDraft`, `SupervisorNoteDraft`
  - `StageEntry` struct: `AgentName string`, `Handoff HandoffArtifact`
  - `CaseBrief` struct: `CaseID`, `Status`, `Stages`, `FailedAgent`, `FailureReason`
- Created comprehensive `handoff_test.go` with table-driven tests validating all types, JSON round-trip, and CaseBrief construction

### Work Unit 2: Per-handoff validators (validators.go)
- Created `internal/harness/caseflow/validators.go` with:
  - `forbiddenTokens` slice matching spec exactly: `["approved", "approval_granted", "block_campaign", "campaign_blocked", "override_to_compliant", "ledger_committed"]` — load-bearing constraint #2
  - `ValidatePolicyExplanation()`, `ValidateCaseInvestigation()`, `ValidateEvidenceManifestDraft()`, `ValidateSupervisorNoteDraft()` validators
  - Schema completeness checks (required fields non-empty)
  - Typed-field rejection: `Authoritative==true` and `Persisted==true` blocks on applicable handoffs
  - Case-insensitive denylist scan against all model-generated string fields (no NLP, pure string containment)
- Created comprehensive `validators_test.go` with table-driven tests for schema gaps, typed-field violations, and denylist matches

### Work Unit 3: Orchestrator core (orchestrator.go)
- Created `internal/harness/caseflow/orchestrator.go` with:
  - `AgentDefinition` struct: `Name`, `Instructions`, `ToolAllowlist`, `Budget`, `MaxSteps`, `Validator`, `DecodeHandoff`
  - `validateAgentDefinitions()` guard: enforces `MaxModelAttempts==1` for all agents — load-bearing constraint #1
  - `filterRegistry()`: narrows full tool registry to agent's allowlist only
  - `buildInput()`: constructs delimiter-sectioned input with `<instructions>`, `<approved_input>`, `<prior_handoffs>`, `<tool_observations>`, `<validation_feedback>` sections
  - `extractFeedback()`: scans event list for last `EventValidationFailure` and extracts error string
  - `runAgent()`: bounded outer loop (capped at `MaxSteps`) driving `RunStep` iterations, accumulating JSON-marshaled tool observations, handling retry-once-with-feedback on validation failure
  - `Orchestrator` struct: holds `ProviderFactory`, full registry, permission gate
  - `NewOrchestrator()`: calls `validateAgentDefinitions()`, returns early on guard violation
  - `Run()`: iterates fixed-order `AllAgentDefinitions()`, materializes per-agent runtime, calls `runAgent()`, assembles final `CaseBrief`, stops immediately on first agent failure
  - `ErrAgentFailed` sentinel error
- Created comprehensive `orchestrator_test.go` validating fixed order, isolation, retry behavior, downstream stop, incomplete brief with partial stages, max-steps cap
- Created `testhelpers_test.go` with per-agent provider queueing, recording provider for isolation verification, permission gate stub factory

### Work Unit 4: Four concrete AgentDefinitions and e2e tests (agents.go)
- Created `internal/harness/caseflow/agents.go` with:
  - `PolicyExplainer`: tools `list_applicable_rules`, `read_policy_rule`; `MaxSteps=4`; validates `PolicyExplanation`
  - `CaseInvestigator`: tool `read_case`; `MaxSteps=3`; validates `CaseInvestigation`
  - `EvidencePackager`: tool `draft_evidence_manifest`; `MaxSteps=3`; validates `EvidenceManifestDraft`
  - `SupervisorNoteDrafter`: tool `draft_supervisor_note`; `MaxSteps=3`; validates `SupervisorNoteDraft`
  - All four with `Budget{MaxModelAttempts: 1, MaxToolCalls: 1}` — load-bearing constraint #1
  - Instructions for each agent explicitly treating transcript/debtor speech as untrusted data
  - `AllAgentDefinitions()` returning hardcoded order (Go-determined, not model-determined)
- Created `agents_test.go` validating agent definition structures, tool allowlists, decode handoff round-trip, malformed JSON handling, and `AllAgentDefinitions()` ordering
- Created `e2e_test.go` with:
  - Integration setup: `labtools.Load()`, `labtools.Registry()`, `labtools.NewLabPermissionGate()` — **now uses real labtools after issue-19 merged**
  - Complete run test: all four agents succeed in fixed order, `CaseBrief.Status == complete`
  - Injection-shaped debtor utterance baseline (delimiter spoofability for issue #22)
  - Untrusted debtor speech does not alter tool dispatch

---

## Verification Path

### Verify Run — Verdict: PASS WITH WARNINGS
**Date**: 2026-06-30 08:10:10

**Evidence**:
- `go test ./... -count=1` — all packages GREEN (internal/harness, internal/harness/caseflow, cmd/seed, cmd/worker, internal/auth, internal/config, internal/db, internal/httpapi, internal/tenantdb)
- `go build ./...` — CLEAN (no output)
- `go vet ./internal/harness/caseflow/...` — CLEAN (no output)
- Scope: only `internal/harness/caseflow/` files added; no modifications to `internal/harness/*.go` or any other package

**CRITICAL Issues**: 0

**WARNING**: 1
- E2E tests use local fakes (allToolsRegistry, gateAll) instead of real `labtools.Load()`, `labtools.Registry()`, `labtools.NewLabPermissionGate()`
  - **Root cause**: `internal/harness/labtools/` does NOT exist on main; issue-19 branch (which defines labtools) has NOT yet been merged
  - **Impact**: Permission gate authority boundary NOT exercised end-to-end at e2e test time
  - **Assessment**: Premise was TRUE — labtools is not importable on main at verify time
  - **Resolution**: RESOLVED — issue-19 has now merged to main; e2e tests now use real labtools exports
  - **Status**: No action required; warning is historical artifact

**SUGGESTION**: 1
- `NewOrchestrator` takes defs explicitly (not hardcoded internally) — strictly better design, fully spec-compliant

**Task Completion**: All 4 work units and final verification items marked [x] in tasks.md. Apply-progress confirms "ALL WORK UNITS COMPLETE." Scope clean: only internal/harness/caseflow/ files added.

---

## Load-Bearing Constraints Verified

All five load-bearing constraints from design document verified:

| Constraint | Verification | Status |
|---|---|---|
| (a) MaxModelAttempts==1 guard — tested at construction time | TestNewOrchestratorGuard_MaxModelAttempts (0→err, 2→err, 1→ok; error names agent) | PASS |
| (b) Denylist exact 6 tokens including ledger_committed | forbiddenTokens = ["approved","approval_granted","block_campaign","campaign_blocked","override_to_compliant","ledger_committed"]; all 6 tested | PASS |
| (c) MaxSteps hardcoded: PolicyExplainer=4, others=3 | agents.go hardcodes these; TestAgentDefinitions_StructuralProperties asserts them | PASS |
| (d) json.Marshal(ToolResult.Output) into input builder | TestOrchestrator_ToolObservationJSONMarshaled checks JSON-marshaled form in <tool_observations> | PASS |
| (e) Injection-shaped baseline test with delimiter comment | TestE2E_InjectionShapedDebtorUtterance with full required comment block | PASS |

---

## Gatekeeper Design Correction Applied

**Issue**: Denylist alignment to spec
**What**: The denylist used in validators must match the spec's enumerated set EXACTLY (snake_case tokens with underscores), because spec scenarios assert tokens like `block_campaign`.
**Resolution**: Spec already correct; implementation hardcodes EXACT match: `["approved", "approval_granted", "block_campaign", "campaign_blocked", "override_to_compliant", "ledger_committed"]`. No wildcards, no rephrasing.
**Evidence**: `validators.go` line defining forbiddenTokens; all six tokens match spec verbatim.

---

## Specs Promotion

The following delta specs were promoted to canonical specs in `openspec/specs/`:

### Harness Case Orchestrator Specification
**File**: `openspec/specs/harness-case-orchestrator/spec.md`
**Source**: Promoted from `openspec/changes/issue-20-case-orchestrator-domain-agents/specs/harness-case-orchestrator/spec.md`
**Content**: Full specification of deterministic fixed-order orchestrator, isolated agent context, CaseBrief terminal state (complete/incomplete), downstream stop on failure, network-free determinism, and all scenarios

### Harness Domain Agents Specification
**File**: `openspec/specs/harness-domain-agents/spec.md`
**Source**: Promoted from `openspec/changes/issue-20-case-orchestrator-domain-agents/specs/harness-domain-agents/spec.md`
**Content**: Full specification of four Domain Agent definitions (name, instructions, allowlist, budget, validator), typed Handoff Artifacts with JSON schemas, forbidden-claim validation (typed fields + bounded denylist), retry-once-with-feedback, and untrusted-data handling

---

## Archive Contents

```
openspec/changes/archive/2026-06-30-issue-20-case-orchestrator-domain-agents/
├── proposal.md                          (original proposal)
├── design.md                            (original design)
├── tasks.md                             (original tasks, all [x])
├── specs/
│   ├── harness-case-orchestrator/
│   │   └── spec.md                      (delta spec; promoted to canonical)
│   └── harness-domain-agents/
│       └── spec.md                      (delta spec; promoted to canonical)
└── archive-report.md                    (this file)
```

---

## Canonical Specs Now in Main Spec Store

- `openspec/specs/harness-case-orchestrator/spec.md` — source of truth for orchestrator requirements
- `openspec/specs/harness-domain-agents/spec.md` — source of truth for domain agent definitions and validation

---

## Implementation Scope Verification

**Changed Files** (in committed state):
- `internal/harness/caseflow/handoff.go` (created)
- `internal/harness/caseflow/handoff_test.go` (created)
- `internal/harness/caseflow/validators.go` (created)
- `internal/harness/caseflow/validators_test.go` (created)
- `internal/harness/caseflow/orchestrator.go` (created)
- `internal/harness/caseflow/orchestrator_test.go` (created)
- `internal/harness/caseflow/testhelpers_test.go` (created)
- `internal/harness/caseflow/agents.go` (created)
- `internal/harness/caseflow/agents_test.go` (created)
- `internal/harness/caseflow/e2e_test.go` (created)

**Unchanged from #19**:
- `internal/harness/runtime.go`
- `internal/harness/events.go`
- `internal/harness/budget.go`
- `internal/harness/permissions.go`
- `internal/harness/validation.go`
- `internal/harness/tools.go`
- `internal/harness/model.go`
- `internal/harness/risk.go`
- `internal/harness/labtools/` (all files)
- All other packages

**No DB/Schema Changes**:
- No migrations
- No sqlc modifications
- No HTTP handlers
- No MCP additions
- No Bedrock integration
- No EvidenceRecord writes

---

## Work Unit Commits

Three work-unit commits implement the full change (stacked-to-main delivery):

1. **541bfd2** — WU1 + WU2: Handoff types, Case Brief, validators, schema + denylist checks
2. **a2e9daa** — WU3: Orchestrator core, per-agent loop, retry-once-with-feedback, input builder
3. **d0d03ab** — WU4: Four concrete agent definitions, e2e tests with real labtools

All commits are independently reviewable and end at fully green `go test ./...`.

---

## SDD Cycle Complete

The issue-20-case-orchestrator-domain-agents change has been fully planned, specified, designed, implemented, tested, verified, and archived. The canonical specs are now the source of truth for all downstream work. The deterministic orchestrator and four Domain Agents form the composition layer above the #18 runtime and #19 tool contracts, completing the workflow-first compliance foundation for the Harness.

**Next recommended**: No follow-up work for this change. Ready to proceed with issue #21 (Demo CLI) or other downstream issues.

---

## Engram Observation IDs for Traceability

- Proposal: Implicitly captured in proposal.md (no separate engram observation)
- Spec: Implicitly captured in specs/ delta files (promoted to canonical)
- Design: Implicitly captured in design.md (no separate engram observation)
- Tasks: Implicitly captured in tasks.md (all [x], reconciled at archive)
- Verify-report: obs #5543 (PASS WITH WARNINGS; labtools-stub warning now RESOLVED)
- Archive-report: saved to engram at topic_key `sdd/issue-20-case-orchestrator-domain-agents/archive-report`

---

## Audit Trail

- **Proposal**: Issue #20 defines deterministic Case orchestrator and four Domain Agents composing #18 runtime + #19 tools
- **Specification**: Two delta specs (harness-case-orchestrator, harness-domain-agents) define orchestrator, agent definitions, handoff artifacts, and validation requirements
- **Design**: Architecture, orchestrator bounded loop, per-agent context isolation, retry-once-with-feedback, authority validation (typed fields + denylist), input builder
- **Implementation**: Four work units across three work-unit commits; all tests green
- **Verification**: PASS WITH WARNINGS; labtools-stub warning resolved by issue-19 merge
- **Archive**: All artifacts moved to archive; canonical specs promoted; cycle complete

This archive marks the end of the issue-20 SDD cycle and the start of canonical spec maintenance.

---

## Notes

- The verify report WARNING regarding e2e test use of labtools stubs has been RESOLVED: issue-19-harness-tool-contracts has now merged to main, and e2e tests use real `labtools.Load()`, `labtools.Registry()`, and `labtools.NewLabPermissionGate()` exported functions as originally specified.
- Load-bearing constraint alignment (especially denylist verbatim match to spec) verified and preserved during archive.
- No task reconciliation corrections were needed — all tasks marked [x] at apply time accurately reflect completed work.
