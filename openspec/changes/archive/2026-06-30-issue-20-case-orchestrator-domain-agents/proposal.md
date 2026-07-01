# Proposal: Issue #20 Deterministic Case Orchestrator and Domain Agents

## Why

The #18 runtime executes one sandboxed agent step and #19 added typed read/draft tools plus the
synthetic Case (`CASE-SYN-001`). Nothing yet composes those pieces into an end-to-end Case workflow.
#20 must compose four sandboxed Domain Agents behind a DETERMINISTIC Case orchestrator using explicit
Handoff Artifacts — not shared hidden context and not model-based routing.

Doing this NOW, before the demo CLI (#21) and Bedrock (#22), keeps composition, authority boundaries,
and inter-agent contracts in one reviewable slice and proves the workflow-first authority model with the
Fake Model Provider before any output surface or live model exists. The authority risk is real:
collections agents must never assert approval, campaign blocking, ledger updates, or override-to-compliant.
This slice encodes that as machine-checked validation, not prompt text.

This proposal covers GitHub issue #20 only.

## Goal

Add a deterministic, fixed-order Case orchestrator that invokes four Domain Agents
(`PolicyExplainer` → `CaseInvestigator` → `EvidencePackager` → `SupervisorNoteDrafter`), each with
isolated context, an explicit tool allowlist (subset of #19 tools), a typed/JSON-validatable output
schema, and output validation with retry-once-with-feedback. Failed validation twice yields a typed,
terminal incomplete Case Brief. Reuse the #18 runtime and #19 tools/gate/fixtures unchanged.

## What Changes

- Add a new sub-package `internal/harness/caseflow` for the orchestrator, the four Domain Agent
  definitions, the Handoff Artifact types, and the Case Brief terminal state. It imports
  `internal/harness` (runtime) and `internal/harness/labtools` (tools, gate, loader).
- Represent each agent as Go data: an `AgentDefinition` (name, static instructions, tool allowlist,
  budget, typed Validator, handoff decoder). The orchestrator materializes each definition into a
  configured `harness.Runtime` per invocation (allowlist-filtered `ToolRegistry` + reused
  `LabPermissionGate` + agent Validator + injected Fake `ModelProvider`). No runtime/interface changes.
- Add a deterministic orchestrator that invokes the four agents in fixed Go order, passing only the
  approved Case input + prior Handoff Artifacts to each agent. No model routing, no shared mutable state.
- Add four typed Handoff Artifacts (Go structs with JSON tags) that chain forward, each agent consuming
  prior artifacts.
- Add per-agent output validation that REJECTS forbidden authority claims (approval, campaign blocking,
  ledger update / `persisted=true`, override-to-compliant), reusing the #19 authority-boundary philosophy.
- Add retry-once-with-feedback at the handoff-synthesis step, mapped onto the existing `harness.Validator`
  + `StepResult`/`validation_failure` seam; a second failure stops that agent.
- Add a typed `CaseBrief` terminal state (`complete` | `incomplete`) carrying the ordered completed
  handoffs and, on failure, the failing agent and reason.

## Scope

### In Scope
- `internal/harness/caseflow` package: orchestrator + four `AgentDefinition`s + handoff types + Case Brief.
- Fixed-order composition with isolated per-agent context (approved input + prior handoffs only).
- Four JSON/Go-struct Handoff Artifacts that chain forward.
- Output validation rejecting the four forbidden authority claims; retry-once-with-feedback.
- Typed incomplete Case Brief on second validation failure.
- Behavior-focused, table-driven tests proving fixed order, isolated handoffs, forbidden-claim rejection,
  and retry-once behavior with the Fake Model Provider. Network/DB/Bedrock-free.

### Out of Scope (Non-Goals)
- No Bedrock or live model provider (only the injected Fake/deterministic `ModelProvider`).
- No CLI, file outputs, or stdout rendering of the Case Brief — it is an in-memory typed value (#21).
- No MCP server. No dynamic/model-based routing or autonomous agent loop.
- No real database, RLS, EvidenceRecord, hash chain, or evidence-ledger writes.
- No new tools, no changes to #18 runtime interfaces or #19 tool contracts.

## Domain Agents and Handoff Chain

| # | Agent | Tool allowlist (subset of #19) | Consumes | Produces (Handoff Artifact) |
|---|-------|-------------------------------|----------|-----------------------------|
| 1 | `PolicyExplainer` | `list_applicable_rules`, `read_policy_rule` | approved Case ref (`case_id`) | `PolicyExplanation` — applicable rules + plain-language requirement per rule |
| 2 | `CaseInvestigator` | `read_case` | `PolicyExplanation` | `CaseInvestigation` — findings linking transcript/detector evidence to rule codes; analysis only |
| 3 | `EvidencePackager` | `draft_evidence_manifest` | `PolicyExplanation` + `CaseInvestigation` | `EvidenceManifestDraft` — non-authoritative, non-persisted draft |
| 4 | `SupervisorNoteDrafter` | `draft_supervisor_note` | all prior handoffs | `SupervisorNoteDraft` — non-authoritative, non-persisted draft |

Final assembled output: `CaseBrief` = ordered handoffs + status. Agents draft/analyze only; none asserts
authority. The `LabPermissionGate` already denies authority + unregistered tools fail-closed; the allowlist
further narrows each agent's registry.

## Capabilities

### New Capabilities
- `harness-domain-agents`: the four Domain Agent definitions (instructions, tool allowlist, output schema,
  handoff type), isolated context, and per-agent output validation — schema completeness, forbidden
  authority-claim rejection, and retry-once-with-feedback.
- `harness-case-orchestrator`: the deterministic fixed-order orchestrator composing the four agents via
  Handoff Artifacts, the chaining contract, and the typed Case Brief terminal state (complete / incomplete).

### Modified Capabilities
- None. `harness-runtime` (#18), `harness-tools`, and `harness-synthetic-fixtures` (#19) are reused
  unchanged; #20 only adds conforming composition above them.

## Open Questions Resolved

1. **Package layout** — A new `internal/harness/caseflow` package holds the orchestrator, the four agent
   definitions, the handoff types, and the Case Brief. It reuses `internal/harness` and
   `internal/harness/labtools`. Name justification: it is the bounded compliance Case workflow; `caseflow`
   reads better than the generic `domain`/`agents` and avoids colliding with harness/runtime concepts.
2. **Agent definition representation** — Go data: `AgentDefinition{ Name, Instructions, ToolAllowlist
   []string, Budget harness.Budget, Validator harness.Validator, decode handoff }`. The orchestrator turns
   each definition into a configured `harness.Runtime` per invocation, driving the EXISTING runtime — no
   new runtime interface.
3. **Handoff schemas and chaining** — Four typed structs (`PolicyExplanation`, `CaseInvestigation`,
   `EvidenceManifestDraft`, `SupervisorNoteDraft`), serialized as JSON in the agent's final output and
   decoded/validated by the next agent. Each agent receives only the approved Case input + prior handoffs;
   chaining is explicit data passing, never shared hidden state.
4. **Retry-once-with-feedback mapping** — Reuse the #18 validation seam: the agent's typed handoff Validator
   is a `harness.Validator` that inspects `FinalOutput`. The orchestrator owns the retry — on the first
   `validation_failure`, it re-invokes the synthesis step once with the validator feedback (read from the
   `EventValidationFailure` event) appended to the agent input; a second failure stops that agent. This
   honors "with feedback" without modifying #18 (the runtime's internal retry re-sends identical input).
5. **Incomplete Case Brief** — A typed terminal state: `CaseBrief{ CaseID, Status: complete|incomplete,
   Stages []HandoffArtifact (in order), FailedAgent, FailureReason }`. On second validation failure the
   orchestrator stops, marks `incomplete`, and records which agent failed and why.

## Impact

| Area | Impact | Description |
|------|--------|-------------|
| `internal/harness/caseflow` | New | Orchestrator, four `AgentDefinition`s, handoff types, Case Brief |
| `internal/harness` (runtime) | Reused | `Runtime`, `RunStep`, `StepResult`, `Validator`, events — unchanged |
| `internal/harness/labtools` | Reused | `Registry`, `LabPermissionGate`, `Load`, fixtures — unchanged |

## Constraints

- Workflow-first: the orchestrator is a deterministic fixed-order workflow, NOT an autonomous loop and NOT
  model-based routing. Non-negotiable.
- Each Domain Agent is sandboxed through the existing runtime + Fake Model Provider, with an allowlist and
  a validated output schema; context is isolated to approved input + prior handoffs.
- Handoff Artifacts are the ONLY inter-agent communication — typed, machine-validatable, no hidden context.
- Treat transcript/debtor speech as untrusted DATA in agent instructions, never instructions.
- Authority boundary: validation rejects approval, campaign-block, ledger-update, and override-to-compliant
  claims. Agents draft/analyze only.
- Keep Judge and Harness model ports separate; MCP external-only; Bedrock opt-in only (all out of scope).
- Strict TDD active: behavior-focused, table-driven tests before production code.
- Smallest viable change that preserves architecture; no future-proofing beyond #20.

## Rollback Plan

The change is additive and isolated to the new `internal/harness/caseflow` package plus its tests. Revert
by removing that package. No migrations, no schema, no changes to #18 runtime interfaces or #19 tool
contracts to undo.

## Dependencies

- #18 runtime skeleton (`Runtime`, `RunStep`, `StepResult`, `Validator`, event contract) — landed.
- #19 lab tools, `LabPermissionGate`, embedded synthetic fixtures (`CASE-SYN-001`) — landed.

## Decision Boundaries

- #20 owns Domain Agents, the deterministic orchestrator, Handoff Artifacts, output validation, and the
  in-memory Case Brief terminal state.
- #21 owns the demo CLI and any Case Brief file/stdout rendering. #20 produces only an in-memory value.
- #22 owns the Bedrock opt-in provider. #20 uses only the injected Fake/deterministic `ModelProvider` via
  the existing model seam.
- Real policy-rule persistence, EvidenceRecord, RLS, and authority-bearing tools remain in their own slices.

## Review Workload

This slice is LARGER than #19: orchestrator + four agent definitions + four handoff types + Case Brief +
validators (schema + forbidden-claim scanner) + per-agent `RunStep` loop + tests. It will likely exceed the
400 changed-line budget. Recommend chained/stacked PRs, for example: (1) handoff types + Case Brief +
`AgentDefinition` + validators; (2) deterministic orchestrator + fixed-order composition + per-agent
`RunStep` loop + retry-once; (3) the four concrete agent definitions + end-to-end tests. `sdd-tasks` should
forecast and confirm the split. Grounding flag for design: `RunStep` is single-step (it returns after a
tool call and does not feed tool results back to the model), so per agent the orchestrator must drive a
bounded, deterministic loop over `RunStep` — allowlisted tool steps then a final handoff-synthesis step,
capped by the agent budget. This is bounded and deterministic, not an autonomous loop.

## Success Criteria

- [ ] The orchestrator invokes the four agents in the fixed order and stops on first agent failure.
- [ ] Each Domain Agent has isolated context and receives only approved input + prior Handoff Artifacts.
- [ ] Handoff Artifacts are machine-validatable JSON/Go structs that chain forward.
- [ ] Validation rejects approval, campaign-block, ledger-update, and override-to-compliant claims.
- [ ] Validation retries once with feedback; a second failure stops that agent and marks the run incomplete.
- [ ] Tests prove fixed order, isolated handoffs, forbidden-claim rejection, and retry-once with the Fake
      Model Provider; `go test ./...` passes or any failure is explicitly unrelated and evidenced.

## Recommended Next Phase

Proceed to SDD spec for `issue-20-case-orchestrator-domain-agents`, translating `harness-domain-agents` and
`harness-case-orchestrator` into requirements and scenarios. Spec and design may run in parallel.
