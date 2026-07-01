# Archive Report: issue-22-bedrock-claude-provider

## Status

Archived — archive preconditions passed, delta specs merged to canonical openspec/specs/ files, and the active change folder will be moved to the dated OpenSpec archive path.

## Summary

**Change**: issue-22-bedrock-claude-provider  
**Artifact store**: openspec (file-based)  
**Completed**: Yes (verification PASS with gap-closure test applied)  
**Implementation commits**: e626ef9 (WU1: bedrock package), 639296d (WU2: CLI wiring), 5f38398 (gap-closure test for WARNING)

## Specifications Synced

### NEW Specifications Created

- **`openspec/specs/harness-bedrock-provider/spec.md`** — Complete specification for the opt-in Amazon Bedrock Claude provider adapter for the Harness ModelProvider port. Covers injectable invoker seam, request/response normalization, error taxonomy, usage metadata reporting, factory constructor with fail-fast config validation, and testing gap documentation (live-Bedrock round-trip not automated).

### MODIFIED Specifications

- **`openspec/specs/harness-demo-cli/spec.md`** — Enhanced with new `--provider {fake|bedrock}` flag requirements. Appended four new requirement sections for provider selection, bedrock configuration/credential handling, and fixed agent invocation order. All pre-existing CLI requirements (--case handling, output artifacts, exit-code contract) preserved byte-for-byte.

## Implementation Verification

### Verification Report Status

**PASS WITH WARNINGS**

- **Verification executed**: 2026-06-30 23:17:05 UTC
- **Build/test status**: All passing (`go build`, `go vet`, `go test ./...`)
- **Task completion**: All 38 tasks.md checkboxes marked `[x]` across both work units
- **Import boundaries**: Zero AWS SDK imports in core harness packages (confirmed via `go list -deps`)
- **CLI runtime**: Manual invocation of `--provider bedrock` with missing config correctly fails with exit code 2 and descriptive error

### Warning Addressed

**WARNING**: Spec scenario "Normalized adapter errors reach the orchestrator failure-reason path" initially lacked an end-to-end integration test wiring bedrock.Provider into caseflow.Orchestrator. **Resolution**: Follow-up commit 5f38398 added `TestOrchestrator_BedrockAdapterErrorReachesFailureReason` in `internal/harness/bedrock/orchestrator_integration_test.go`, wiring a real Bedrock adapter with a fake invoker into the orchestrator to verify error normalization flows through to FailureReason. WARNING closed.

### No CRITICAL Issues

Zero CRITICAL blockers. All implementation requirements met, all scenarios covered by tests (including the gap-closure integration test), and all 38 tasks completed.

## Archive Artifacts

- `openspec/changes/issue-22-bedrock-claude-provider/proposal.md` ✅
- `openspec/changes/issue-22-bedrock-claude-provider/specs/harness-bedrock-provider/spec.md` ✅
- `openspec/changes/issue-22-bedrock-claude-provider/specs/harness-demo-cli/spec.md` ✅
- `openspec/changes/issue-22-bedrock-claude-provider/design.md` ✅
- `openspec/changes/issue-22-bedrock-claude-provider/tasks.md` ✅ (38/38 complete)
- `openspec/changes/issue-22-bedrock-claude-provider/apply-progress.md` ✅
- `openspec/changes/issue-22-bedrock-claude-provider/verify-report.md` ✅

## Source of Truth Updated

The following canonical specs in `openspec/specs/` now reflect issue-22's behavior:

- `openspec/specs/harness-bedrock-provider/spec.md` — NEW
- `openspec/specs/harness-demo-cli/spec.md` — MODIFIED (--provider requirements added)

## SDD Cycle Complete

The change has been fully planned (proposal), specified (specs/harness-bedrock-provider + delta merge for harness-demo-cli), designed, implemented (commits e626ef9, 639296d, 5f38398), verified (PASS with WARNING closed), and archived.

Issue #22 is ready for integration with upstream branches.

---

**Session**: 019f1bcd-c21c-7f2c-9bea-54aaeccd8944  
**Project**: vigia  
**Archived**: 2026-06-30 23:30 UTC  
**Canonical specs synced to**: openspec/specs/{harness-bedrock-provider,harness-demo-cli}/spec.md
