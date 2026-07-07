# Proposal: Issue #10 Realtime Guardrails + Campaign Preflight Simulator

## Problem / motivation

Vigía can already evaluate interactions after the fact, produce evidence, and demonstrate a sandboxed Agent Harness Lab. That is not enough for authority-bearing outbound activity. A compliance product must prevent prohibited outbound contact before it is sent, not merely explain the violation later.

The current gap is that an agent can draft or propose an outbound utterance, but the authority boundary that decides whether the utterance may actually be sent is not yet connected to REDECO policy enforcement, structured event logging, and the evidence ledger. Campaign teams also need a way to discover non-compliant templates, schedules, and sequences before a campaign launches.

As of July 2026, the modern standard for agentic guardrails is runtime policy enforcement outside the model: the model proposes a typed action, the harness validates schema and context, deterministic policy checks run first, semantic/judge checks run only where needed, and the runtime returns `allow`, `deny`, or `approval_required`. Prompt-only safety and separate autonomous guardrail agents are not sufficient for authority-bearing external action.

## Intent

Graduate the sandboxed Harness Lab into authority-bearing realtime guardrails by making the application runtime the policy enforcement point for outbound utterances and campaign preflight.

The change should deliver two product outcomes:

1. **Realtime outbound guardrails:** every proposed outbound agent utterance is validated before send against the active REDECO policy bundle and the existing Harness permission-gate/event-log contract. Violations are blocked before send, logged with the violated rule, and recorded in the evidence ledger.
2. **Full-campaign preflight:** a campaign-level simulator runs a complete campaign artifact in dry-run mode against the same ruleset and returns an actionable brief before launch.

The runtime, not the model, owns the final authority decision. Drafting and sending remain separate actions.

## Desired behavior

### Realtime outbound guardrails

- A model or agent may propose an outbound utterance only as a typed proposed action.
- Before any external send, the runtime validates:
  - action schema;
  - tenant and campaign/case context;
  - debtor/contact timezone and allowed contact window;
  - channel and recipient context;
  - policy bundle/version;
  - required evidence/audit context.
- The runtime evaluates REDECO rules, including at minimum:
  - contact hours / out-of-hours restrictions;
  - threatening, abusive, or coercive tone;
  - third-party disclosure/contact constraints;
  - prohibited or misleading payment-routing instructions.
- Deterministic checks run first. LLM judge checks may be used only for semantic/tone cases through the existing judge seam, separate from the Harness model provider.
- If context is missing, ambiguous, stale, or schema-invalid for an authority-bearing send, the runtime fails closed.
- A violation returns a blocked/denied result before send, including the violated rule, actionable remediation, and an optional compliant rewrite as a draft suggestion. The system must not automatically send the rewrite.
- Every decision emits structured operational events. Blocked authority actions must reach the evidence ledger without exposing hidden model reasoning.

### Campaign preflight simulator

- The first preflight unit is the **complete campaign**, not a single isolated sequence.
- Preflight runs the campaign's templates/scripts/sequences/schedules in dry-run mode through the same policy engine used by realtime outbound guardrails.
- Preflight produces an actionable brief as the primary output.
- The brief should identify:
  - campaign-level pass/fail status;
  - violating template/script/sequence steps;
  - violated rule codes and policy bundle version;
  - evidence/event references;
  - missing context that caused fail-closed decisions;
  - concrete remediation guidance;
  - optional compliant rewrite drafts where safe.
- A compliant campaign passes. A non-compliant campaign is flagged before launch.

## Scope

### In scope

- Reuse the #16 Harness permission-gate and structured event-log contract for authority-bearing outbound actions.
- Add or adapt an outbound-send policy enforcement path where the model proposes a typed utterance/action and the runtime decides before send.
- Integrate the current policy bundle/version into the outbound decision so results are auditable and reproducible.
- Evaluate REDECO outbound constraints for hours, tone/threat, third-party, and payment-routing violations.
- Reuse the #4 judge capability only as a semantic/tone judge port where deterministic checks are insufficient; keep the judge port separate from Harness model/provider ports.
- Log blocked outbound decisions with violated rule metadata and persist the required evidence ledger record.
- Add a full-campaign preflight simulator that exercises complete campaign artifacts in dry-run mode against the same policy engine.
- Produce an actionable preflight brief for campaign operators.
- Behavior-focused tests for blocked and compliant utterances, ledger logging, non-compliant campaign preflight, and compliant campaign preflight.

### Out of scope / non-goals

- No code implementation in this proposal phase.
- No prompt-only safety mechanism as the authority boundary.
- No separate autonomous guardrail agent that decides outside the runtime policy engine.
- No automatic send of model-generated rewrites; rewrites are drafts/suggestions only.
- No external MCP behavior or MCP authorization changes; keep internal Harness runtime distinct from external MCP.
- No broad campaign launch UI, rule-authoring UI, or policy-bundle editor.
- No live SMS/email/telephony provider integration unless later spec/design identifies an already-existing narrow send seam that must be wrapped.
- No hidden chain-of-thought storage in events, evidence, briefs, or logs.
- No full data-driven rule interpreter beyond the policy checks required for this issue unless an existing policy engine already provides it.

## Affected areas

| Area | Impact | Notes |
|------|--------|-------|
| Internal Harness runtime / authority tool path | Modified | Reuse permission gate and event-log contract for outbound send proposals. |
| Policy enforcement engine | Modified | Serve as runtime policy enforcement point for realtime send and dry-run preflight decisions. |
| Judge seam | Reused / possibly extended | Semantic tone/threat checks stay behind the judge port, separate from Harness model providers. |
| Evidence ledger | Modified / integrated | Blocked outbound authority decisions must produce auditable evidence records. |
| Campaign/preflight domain | New | Evaluate complete campaign artifacts and produce actionable briefs before launch. |
| Synthetic fixtures / tests | Modified / new | Need compliant and non-compliant utterance/campaign cases. |
| External MCP | Not modified | Explicit boundary: MCP must not become the guardrail runtime or bypass it. |

## Policy and product rules

- **Authority-bearing outbound action is deny-by-default** on missing context, invalid schema, unknown policy bundle, ambiguous recipient/channel, or unavailable required checks.
- **Drafting is not sending.** A draft utterance may be produced before policy enforcement, but any send-like action must pass the runtime decision first.
- **Runtime decisions are auditable.** Decision records include rule IDs, policy bundle version, context used, decision outcome, event IDs, and evidence references, but never hidden model reasoning.
- **Deterministic-first.** Hours, recipient/context, payment-routing structure, and schema checks should be deterministic where possible. Judges are reserved for semantic/tone cases.
- **Same engine, two modes.** Realtime send uses enforcement mode; campaign preflight uses dry-run mode with the same policy logic and no external side effects.
- **Operator UX is actionable.** Blocking should show what rule was violated and what to fix, not just a generic denial.

## Dependencies

- #16 Agent Harness Lab / Harness runtime stack: permission gate, tool result shapes, and structured event log contract.
- #6 Policy Bundle: versioned policy bundle resolution and reproducible policy metadata.
- #4 LLM Judge Tone/Threat: semantic tone/threat judgment through a separate judge port.
- #3 Evidence Ledger: append-only evidence records for blocked authority decisions.
- #2 Contact Hours Detector or equivalent hours policy logic for out-of-hours checks.

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Guardrails become prompt-only instead of runtime-enforced | High | Treat the runtime as the policy enforcement point; model output is only a proposal. |
| Missing context accidentally allows an outbound send | High | Fail closed for all authority-bearing sends and record the context gap. |
| Campaign preflight drifts from realtime enforcement | Medium | Run preflight through the same policy engine in dry-run mode. |
| Judge and Harness model ports get conflated | Medium | Keep semantic judge behind the judge port; Harness model proposes actions only. |
| Evidence/logs expose hidden reasoning or sensitive raw content | Medium | Store bounded decision metadata, rule IDs, excerpts only when safe, and evidence references; no hidden reasoning. |
| Preflight scope grows into full campaign launch management | Medium | Limit first slice to simulation and actionable brief, not launch orchestration or UI expansion. |
| Compliant rewrite suggestions are mistaken for approved sends | Medium | Mark rewrites as draft-only and require a separate send decision. |

## Rollback plan

- Disable or remove the outbound pre-send enforcement wrapper, returning authority-bearing outbound tools to their previous non-production/sandbox behavior.
- Remove campaign preflight entrypoints and brief generation while preserving campaign definitions/templates.
- Revert evidence-ledger writes and structured events introduced specifically for outbound guardrail decisions.
- Leave existing policy bundles, judge behavior, contact-hours detection, and Harness Lab contracts intact.
- Preserve already-created evidence records as append-only audit history; rollback stops new records rather than mutating prior ledger entries.

## Success criteria

- [ ] A threatening outbound utterance is blocked before send with the violated tone/threat rule identified.
- [ ] An out-of-hours outbound utterance is blocked before send with the violated contact-hours rule identified.
- [ ] A blocked outbound action emits structured events and reaches the evidence ledger with violated rule metadata.
- [ ] A non-compliant complete campaign preflight is flagged before launch and returns an actionable brief.
- [ ] A compliant utterance passes the realtime guardrail path.
- [ ] A compliant complete campaign passes preflight.
- [ ] Authority-bearing tools reuse the #16 permission-gate/event-log contract.
- [ ] Ambiguous or incomplete authority context fails closed.
- [ ] Draft rewrite suggestions are never automatically sent.

## Proposal question round

The orchestrator supplied product answers before this proposal, so no additional blocking question round was run inside the delegated phase. Incorporated assumptions:

- Violating outbound utterances use modern runtime policy enforcement outside the model.
- External authority actions are deny-by-default on missing or ambiguous context.
- The UX blocks before send, shows the violated rule and actionable remediation, and may provide a compliant rewrite only as a draft suggestion.
- The first preflight unit is a complete campaign.
- The primary preflight output is an actionable brief.
- Ambiguous cases and incomplete context fail closed.

Residual assumptions for user review before spec/design:

- The first implementation can expose preflight through the narrowest existing product/API/CLI seam rather than requiring a new campaign-management UI.
- Payment-routing and third-party checks can start with deterministic policy checks plus fixtures unless design proves a semantic judge is required.
- Preflight evidence can be represented as dry-run decision/evidence records or references, clearly distinguished from actual sent-message enforcement records.

## Next recommended phase

Proceed to SDD spec for `issue-10-realtime-guardrails-preflight`, then design. The spec should translate the realtime guardrail and full-campaign preflight outcomes into concrete requirements and scenarios.
