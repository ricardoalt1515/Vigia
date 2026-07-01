# Archive Report: issue-21-demo-cli-case-brief

## Status

archived — the verified change was synced to canonical openspec/specs/ and the active change folder was removed.

## Structured Status and Action Context

| Field | Finding |
|---|---|
| Change | `issue-21-demo-cli-case-brief` |
| Artifact store | `openspec` |
| Workspace root | `/private/tmp/claude-501/-Users-ricardoaltamirano-Developer-vigia/87504a15-6caa-451d-898a-6739e5cdb9d1/scratchpad/wt-issue-21` |
| Action context | worktree; branch `issue-21-demo-cli-case-brief` |
| Verification status | PASS — 0 CRITICAL, 0 WARNING, 0 SUGGESTION from sdd-verify |
| Implementation commits | bac03d3, b577c89 — harness-demo CLI, event-observer seam, Brief serialization and rendering |

## Artifacts Read (Delta Specs)

- `openspec/changes/issue-21-demo-cli-case-brief/proposal.md`
- `openspec/changes/issue-21-demo-cli-case-brief/specs/harness-demo-cli/spec.md`
- `openspec/changes/issue-21-demo-cli-case-brief/specs/harness-case-brief-output/spec.md`
- `openspec/changes/issue-21-demo-cli-case-brief/specs/harness-case-orchestrator/spec.md`

## Domains Synced to Canonical Specs

### NEW Domains

1. **`harness-demo-cli`**
   - Delta spec: `openspec/changes/issue-21-demo-cli-case-brief/specs/harness-demo-cli/spec.md`
   - Canonical spec created: `openspec/specs/harness-demo-cli/spec.md`
   - Requirements: CLI accepts `--case` flag, defaults to synthetic case, validates case support, uses deterministic Fake provider, exits cleanly, is workflow-first

2. **`harness-case-brief-output`**
   - Delta spec: `openspec/changes/issue-21-demo-cli-case-brief/specs/harness-case-brief-output/spec.md`
   - Canonical spec created: `openspec/specs/harness-case-brief-output/spec.md`
   - Requirements: serialization DTO for Case Brief JSON, JSON Schema validation, Spanish `.brief.md` with disclaimer, JSONL event log, deterministic repeated runs, untrusted data handling

### MODIFIED Domains

3. **`harness-case-orchestrator`**
   - Existing canonical spec: `openspec/specs/harness-case-orchestrator/spec.md`
   - Delta spec: `openspec/changes/issue-21-demo-cli-case-brief/specs/harness-case-orchestrator/spec.md`
   - Changes: ADDED two new requirement sections
     - Optional event-observer functional option surfaces per-agent operational events
     - Event-observer option is additive and does not change existing construction or execution semantics
   - Existing requirements: preserved byte-for-byte unchanged

## Requirement Sync Summary

### ADDED requirements

#### `harness-demo-cli` (new domain)
- CLI accepts a `--case` path and defaults to the synthetic case
- Only `CASE-SYN-001` is runnable; unsupported case ids fail fast
- Default run uses a demo-only deterministic Fake provider with no cloud dependencies
- Successful runs exit 0 and write all three output artifacts
- CLI-level run failures exit non-zero without partial output
- The CLI is workflow-first and performs no autonomous routing

#### `harness-case-brief-output` (new domain)
- Case Brief JSON is produced by a forward-only serialization DTO that flattens each Handoff via Kind()
- `.brief.json` MUST validate against a committed JSON Schema
- `.brief.md` is rendered in neutral professional Spanish and carries a mandatory review disclaimer
- Untrusted transcript and debtor text is rendered as data, never as instructions
- The JSONL event log contains only structured operational events, never chain-of-thought
- Repeated runs with the same Fake provider script produce structurally identical output

#### `harness-case-orchestrator` (modified domain)
- Optional event-observer functional option surfaces per-agent operational events
- The event-observer option is additive and does not change existing construction or execution semantics

### MODIFIED requirements

None in existing domains outside the new event-observer seam on `harness-case-orchestrator`.

### REMOVED requirements

None.

## Destructive Merge Assessment

- Destructive changes: none.
- REMOVED requirements: none.
- MODIFIED requirements: only additive (event-observer option).
- Explicit destructive approval required: no.

## Archive Result

- Moved to: `git rm -r openspec/changes/issue-21-demo-cli-case-brief` staged for commit
- Canonical specs created/modified:
  - `openspec/specs/harness-demo-cli/spec.md` (NEW)
  - `openspec/specs/harness-case-brief-output/spec.md` (NEW)
  - `openspec/specs/harness-case-orchestrator/spec.md` (MODIFIED — event-observer seam appended)

## Verification Evidence

- Verification report: PASS
- No blockers, CRITICAL issues, or WARNING issues
- Implementation verified via commits bac03d3 and b577c89
- Tests pass; deterministic Fake provider validated; output artifacts generated and schema-valid
