# Outbound Guardrails Specification

## Purpose

Define the authority-bearing outbound guardrail behavior for realtime sends. The runtime, not the model, is the enforcement point for outbound utterances: the model may propose a typed action, but the runtime MUST validate context, evaluate REDECO policy, decide allow or deny before send, emit structured events, and record blocked decisions in the evidence ledger.

## Requirements

### Requirement: Runtime owns the final authority decision for outbound sends

The system MUST treat every outbound utterance as a typed proposed action until the runtime completes policy enforcement. An authority-bearing outbound send MUST NOT execute unless the runtime returns an allow decision.

#### Scenario: Compliant outbound proposal is allowed before send

- GIVEN a typed outbound send proposal with valid schema, tenant context, recipient context, channel context, policy bundle version, and required audit context
- AND deterministic and semantic policy checks all pass
- WHEN the runtime evaluates the proposal
- THEN the runtime MUST return an allow decision before the external send is attempted
- AND the send path MUST use that allow decision as the authority to continue

#### Scenario: Denied outbound proposal never executes the external send

- GIVEN a typed outbound send proposal that violates at least one REDECO rule
- WHEN the runtime evaluates the proposal
- THEN the runtime MUST return a deny decision before any external send is attempted
- AND the external send implementation MUST NOT execute

### Requirement: Authority-bearing outbound sends fail closed on missing or ambiguous context

The system MUST deny-by-default when authority context required for an outbound send is missing, ambiguous, stale, schema-invalid, or otherwise insufficient to prove compliance.

#### Scenario: Missing debtor timezone blocks the send

- GIVEN an outbound send proposal requires debtor-local contact-hours evaluation
- AND the debtor timezone is missing or unresolvable
- WHEN the runtime evaluates the proposal
- THEN the runtime MUST return a deny decision
- AND the denial MUST identify the missing timezone context as the reason for the fail-closed result

#### Scenario: Ambiguous recipient or channel blocks the send

- GIVEN an outbound send proposal does not resolve to one unambiguous recipient or one unambiguous outbound channel
- WHEN the runtime evaluates the proposal
- THEN the runtime MUST return a deny decision
- AND the denial MUST identify the ambiguous authority context that prevented compliance verification

#### Scenario: Unknown policy bundle blocks the send

- GIVEN an outbound send proposal cannot be evaluated against a resolved policy bundle version
- WHEN the runtime evaluates the proposal
- THEN the runtime MUST return a deny decision
- AND the denial MUST identify the unresolved policy bundle as the reason for the fail-closed result

### Requirement: Outbound policy enforcement is deterministic-first and uses the judge seam only for semantic tone or threat checks

The runtime MUST evaluate deterministic REDECO checks before any semantic judge call. Contact hours, third-party contact constraints, payment-routing constraints, and channel or recipient context checks MUST be evaluated deterministically where possible. Tone or threat checks MAY use a semantic judge only through the dedicated judge seam and MUST NOT use the Harness model provider as the judge.

#### Scenario: Out-of-hours proposal is blocked without invoking the judge

- GIVEN an outbound send proposal resolves to a debtor-local time outside the permitted contact window
- WHEN the runtime evaluates the proposal
- THEN the runtime MUST return a deny decision for the contact-hours rule
- AND the runtime MUST NOT require a semantic judge result to deny the send

#### Scenario: Payment-routing violation is blocked deterministically

- GIVEN an outbound send proposal includes payment-routing instructions that do not match the allowed creditor-routing policy
- WHEN the runtime evaluates the proposal
- THEN the runtime MUST return a deny decision for the payment-routing rule
- AND the decision MUST be produced by deterministic policy evaluation rather than a semantic judge

#### Scenario: Threatening language is evaluated through the judge seam only

- GIVEN an outbound send proposal passes deterministic schema and context checks
- AND the remaining unresolved question is whether the proposed language is threatening, abusive, or coercive
- WHEN the runtime evaluates the proposal
- THEN any semantic tone or threat decision MUST be obtained through the dedicated judge seam
- AND the runtime MUST NOT use the Harness model provider port as the judge implementation

#### Scenario: Required judge result unavailable blocks the send

- GIVEN an outbound send proposal requires a semantic tone or threat decision
- AND the judge result is unavailable, invalid, or inconclusive
- WHEN the runtime evaluates the proposal
- THEN the runtime MUST return a deny decision
- AND the denial MUST record that a required check could not prove compliance

### Requirement: Outbound decisions reuse the permission-gate and structured event-log contract

The system MUST reuse the #16 Harness permission-decision and structured event-log contract for authority-bearing outbound actions, while adding rule-aware decision metadata needed for compliance enforcement.

#### Scenario: Allowed outbound decision emits structured decision events

- GIVEN an outbound send proposal is allowed
- WHEN the runtime returns the decision
- THEN the structured event log MUST include the permission decision event
- AND the event metadata MUST identify the decision outcome, the policy bundle version, and the evaluated action context
- AND the event log MUST NOT expose hidden model reasoning

#### Scenario: Blocked outbound decision emits rule-aware denial events

- GIVEN an outbound send proposal is denied
- WHEN the runtime returns the decision
- THEN the structured event log MUST include the permission decision event
- AND the event metadata MUST identify the violated rule code or fail-closed reason
- AND the event metadata MUST include enough references to correlate the denial with later evidence records

### Requirement: Blocked outbound decisions are recorded in the evidence ledger

A denied authority-bearing outbound decision MUST produce an evidence-ledger record capturing the auditable enforcement outcome without storing hidden reasoning.

#### Scenario: Blocked third-party contact decision reaches the ledger

- GIVEN an outbound send proposal is denied for a third-party contact violation
- WHEN the runtime finalizes the denial
- THEN the evidence ledger MUST record the blocked authority decision
- AND the record MUST include the violated rule code, policy bundle version, decision outcome, and related event reference
- AND the record MUST NOT include hidden chain-of-thought

#### Scenario: Fail-closed denial records the missing-context reason

- GIVEN an outbound send proposal is denied because required authority context is missing
- WHEN the runtime finalizes the denial
- THEN the evidence ledger MUST record that the decision was fail-closed
- AND the record MUST identify the missing or ambiguous context that prevented compliance verification

### Requirement: Denials return actionable remediation without automatically sending rewrites

A denied outbound decision MUST return actionable remediation guidance and MAY include an optional compliant rewrite suggestion as draft output. Any rewrite suggestion MUST remain a draft and MUST NOT be sent automatically.

#### Scenario: Denial returns a draft rewrite suggestion only

- GIVEN an outbound send proposal is denied
- AND the runtime provides a compliant rewrite suggestion
- WHEN the denial result is returned
- THEN the result MUST mark the rewrite as a draft suggestion
- AND the system MUST require a separate outbound send decision before any rewritten content can be sent

#### Scenario: Denial returns remediation even when no rewrite is available

- GIVEN an outbound send proposal is denied
- AND no compliant rewrite suggestion is available
- WHEN the denial result is returned
- THEN the result MUST still identify the violated rule or fail-closed reason
- AND the result MUST include actionable remediation guidance for the operator or caller
