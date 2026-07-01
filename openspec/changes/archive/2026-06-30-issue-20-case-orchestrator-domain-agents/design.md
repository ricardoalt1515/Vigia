# Design: Issue #20 Deterministic Case Orchestrator and Domain Agents

## Technical Approach

Add one package `internal/harness/caseflow` that composes the unchanged #18 runtime and #19
tools/gate/fixtures into a fixed-order, four-agent Case workflow. The WORKFLOW (agent order +
handoff chaining + authority validation) is deterministic Go; the model only proposes tool calls and
drafts text inside one sandboxed agent. Each agent is a freshly materialized `harness.Runtime`
(allowlist-filtered registry + reused `LabPermissionGate` + agent `Validator` + injected provider).
Because `RunStep` is single-step and never feeds tool results back, the orchestrator drives each agent
as a bounded loop over `RunStep`, accumulating tool observations, then a synthesis step that emits the
typed Handoff Artifact. This honors workflow-first: authority and routing live in code, not the model.

## Architecture Decisions

### Per-agent bounded RunStep loop (Lock 1)
**Choice**: Orchestrator loops `RunStep` per agent, capped by a new `AgentDefinition.MaxSteps`.
**Discriminator** after each step: `Completed` + `ToolResult.Status != ""` + empty `FinalOutput` = tool
step → append `ToolResult.Output` to the agent's observation buffer, continue; `Completed` +
`FinalOutput != ""` = synthesis → decode handoff, agent done; `ValidationFailed` = trigger retry (Lock
3); `ToolNotFound`/`PermissionDenied`/`BudgetExceeded` or loop hits `MaxSteps` = agent fails.
**Feed-back mechanism**: orchestrator rebuilds `StepInput.Input` each step via a delimiter-sectioned
builder — `<instructions>`, `<approved_input>` (case_id), `<prior_handoffs>` (JSON), `<tool_observations>`
(accumulated JSON), `<validation_feedback>` (retry only). Tool number is provider-driven but bounded,
allowlisted, and gated — not autonomous; agent ORDER is hardcoded Go.
**Rejected**: modifying `RunStep` to loop internally (violates "reuse #18 unchanged").

### MaxModelAttempts = 1 per agent (Lock 1 + 3, load-bearing)
**Choice**: every agent `Budget.MaxModelAttempts = 1`, `MaxToolCalls = 1`.
**Rationale**: with `>1`, `RunStep` re-sends IDENTICAL input with NO feedback on validation failure,
silently consuming the scripted corrected output and breaking the orchestrator's feedback contract.
Forcing 1 makes `RunStep` return `ValidationFailed` immediately so the orchestrator owns the only retry.

### Provider injection (Lock 2)
**Choice**: orchestrator takes `ProviderFactory func(agentName string) harness.ModelProvider`.
Tests pass a caseflow-local queued provider (same shape as `queuedModelProvider`, which is unexported)
keyed per agent; production (#22) returns one shared real provider. Fully deterministic, no network.
**Rejected**: a single global provider (cannot script distinct per-agent behavior cleanly).

### Retry-once-with-feedback owned by orchestrator (Lock 3)
On synthesis `ValidationFailed`, scan `result.Events` for the last `EventValidationFailure`, read
`Data["error"].(string)`, append it under `<validation_feedback>`, call `RunStep` ONCE more. Second
failure stops the agent → incomplete. No #18 change.

### AgentDefinition + materialization (Lock 4)
```go
type AgentDefinition struct {
    Name          string
    Instructions  string
    ToolAllowlist []string
    Budget        harness.Budget // MaxModelAttempts MUST be 1
    MaxSteps      int            // outer-loop cap
    Validator     harness.Validator
    DecodeHandoff func(finalOutput string) (HandoffArtifact, error)
}
```
Materialize: `Runtime{Model: factory(def.Name), Tools: filterRegistry(full, def.ToolAllowlist),
Permissions: gate, Validator: def.Validator, Budget: def.Budget}`. `filterRegistry` copies only
allowlisted names; out-of-allowlist calls return `ToolNotFound`.

### Handoff schemas + chaining (Lock 5)
| Agent | Produces (JSON struct) | Consumes |
|-------|------------------------|----------|
| PolicyExplainer | `PolicyExplanation{CaseID, Rules[]{Code,Title,Severity,PlainLanguage}}` | case_id |
| CaseInvestigator | `CaseInvestigation{CaseID, Findings[]{RuleCode,Evidence,Analysis}}` | PolicyExplanation |
| EvidencePackager | `EvidenceManifestDraft{CaseID,RuleCodes,Findings,ProposedAt,Authoritative,Persisted}` | PolicyExplanation+CaseInvestigation |
| SupervisorNoteDrafter | `SupervisorNoteDraft{CaseID,RuleCodes,NoteBody,ProposedAt,Authoritative,Persisted}` | all prior |

Draft structs mirror #19 `Draft*Response`. All implement `HandoffArtifact interface{ Kind()
HandoffKind; CaseRef() string }`. Terminal: `CaseBrief{CaseID, Status (complete|incomplete),
Stages []HandoffArtifact, FailedAgent, FailureReason}` — in-memory only (#21 renders).

### Forbidden-authority-claim validation (Lock 6)
Two layers, both deterministic — explicitly NOT NLP/sentiment.
| Handoff | Typed-field reject | Free-text denylist scan |
|---------|-------------------|--------------------------|
| EvidenceManifestDraft / SupervisorNoteDraft | `Authoritative==true` OR `Persisted==true` | `Findings` / `NoteBody` |
| PolicyExplanation | n/a (no such fields) | each `PlainLanguage` |
| CaseInvestigation | n/a | each `Evidence`, `Analysis` |

Denylist = the EXACT, fixed enumerated token set from the spec (no wildcards, no rephrasing):
`approved`, `approval_granted`, `block_campaign`, `campaign_blocked`, `override_to_compliant`,
`ledger_committed`. Case-insensitive substring match over model-generated string fields only — NOT NLP.
The typed-field checks (`Authoritative==true` / `Persisted==true`) remain the precise primary guard; the
denylist is the secondary free-text scan. Validator also rejects empty required fields (schema
completeness) and undecodable JSON.

> Gatekeeper correction (design re-gate): the denylist MUST match the spec's enumerated set verbatim
> (snake_case tokens with underscores), because the spec scenarios assert tokens like `block_campaign`.
> Dropping `ledger_committed`, using `approv*` wildcards, or substituting space-separated phrases would
> fail verify. If a broader natural-language denylist is ever wanted, change the spec first, not silently.

### Isolated context (Lock 7)
Each agent gets a FRESH `Runtime` + fresh input string built ONLY from `case_id` + the specific prior
handoffs it consumes. The observation buffer and `StepResult.Events` are loop-local and discarded after
the handoff is produced — never passed to another agent. Handoff Artifacts are the sole inter-agent
channel; no shared mutable state.

## Data Flow

    case_id ─→ PolicyExplainer ─(PolicyExplanation)─→ CaseInvestigator ─(+CaseInvestigation)─→
    EvidencePackager ─(+EvidenceManifestDraft)─→ SupervisorNoteDrafter ─→ CaseBrief
       each agent: [RunStep tool*]→[RunStep synthesis]→Validator→(retry once)→handoff

## File Changes
| File | Action | Description |
|------|--------|-------------|
| `internal/harness/caseflow/handoff.go` | Create | 4 handoff structs, `HandoffArtifact`, `CaseBrief`, `CaseStatus` |
| `internal/harness/caseflow/validators.go` | Create | per-handoff schema + denylist validators |
| `internal/harness/caseflow/agents.go` | Create | the four concrete `AgentDefinition`s |
| `internal/harness/caseflow/orchestrator.go` | Create | `Orchestrator`, `ProviderFactory`, fixed-order loop, per-agent driver, retry, brief assembly |
| `internal/harness/caseflow/*_test.go` | Create | table-driven behavior tests |

## Testing Strategy (Lock 8)
| Layer | What | Approach |
|-------|------|----------|
| Unit | each validator: 4 forbidden categories × {typed, free-text}, schema gaps | table-driven `Validate` cases |
| Unit | retry: invalid→valid (assert feedback substring in 2nd input, call count) and invalid→invalid→incomplete | scripted queued provider |
| Integration | fixed order, isolation (recording provider asserts no leaked observations), allowlist (out-of-list → ToolNotFound), incomplete brief (FailedAgent/Reason, partial Stages) | `labtools.Load()` + per-agent fakes |

No network/DB/Bedrock; gate + provider in-memory; `testing.Short` not required.

## Migration / Rollout
No migration. Additive package; revert by deletion. Aligns with proposal's 3-PR split: (1) handoff types
+ brief + AgentDefinition + validators; (2) orchestrator loop + retry; (3) four agents + e2e tests.

## Open Questions
- [ ] Denylist false-positive risk when an agent quotes untrusted transcript containing a denylist term;
  resolved fail-closed for #20 (authority correctness > convenience) — flagged for spec acknowledgement.
