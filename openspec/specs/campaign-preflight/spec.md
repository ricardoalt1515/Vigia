# Campaign Preflight Specification

## Purpose

Define the dry-run campaign preflight behavior that evaluates a complete campaign against the same policy engine used for realtime outbound guardrails, without creating external side effects. Preflight exists to flag non-compliant campaign artifacts before launch and return an actionable operator brief.

## Requirements

### Requirement: Preflight evaluates the complete campaign in dry-run mode

The system MUST evaluate the complete campaign artifact in dry-run mode, not just an isolated template or single step. Dry-run preflight MUST use the same policy logic as realtime outbound guardrails while preventing any external send or provider side effect.

#### Scenario: Non-compliant complete campaign fails preflight

- GIVEN a campaign containing templates, sequence steps, recipients, channels, or schedules that would violate one or more outbound guardrail rules
- WHEN preflight evaluates the complete campaign in dry-run mode
- THEN the campaign MUST be reported as failed before launch
- AND the failing result MUST identify the violating campaign components
- AND no external send or live provider action MUST occur

#### Scenario: Compliant complete campaign passes preflight

- GIVEN a campaign whose evaluated templates, sequence steps, recipients, channels, and schedules satisfy the outbound guardrail rules
- WHEN preflight evaluates the complete campaign in dry-run mode
- THEN the campaign MUST be reported as passed
- AND no external send or live provider action MUST occur

### Requirement: Preflight uses the same fail-closed policy engine as realtime enforcement

Preflight MUST use the same policy engine and rule set as realtime outbound enforcement, including deterministic-first evaluation and fail-closed behavior on missing or ambiguous required context.

#### Scenario: Missing campaign context causes fail-closed preflight failure

- GIVEN a campaign cannot supply required authority context for one or more evaluated steps
- WHEN preflight evaluates the campaign
- THEN the affected steps MUST be treated as failed by fail-closed policy enforcement
- AND the preflight result MUST identify the missing context that prevented compliance verification

#### Scenario: Deterministic rule failure is reported without requiring a semantic judge

- GIVEN a campaign step is scheduled outside the permitted contact window or targets an unauthorized recipient or channel
- WHEN preflight evaluates the campaign
- THEN the step MUST fail for the deterministic rule violation
- AND the result MUST NOT require a semantic judge to establish that deterministic failure

### Requirement: Preflight returns an actionable brief as the primary output

Preflight MUST return an actionable brief summarizing campaign-level compliance status and the operator actions needed to remediate failures.

#### Scenario: Failed preflight brief includes rule, artifact, and remediation details

- GIVEN a campaign fails preflight
- WHEN the brief is returned
- THEN the brief MUST include the overall pass or fail status
- AND the brief MUST identify the violating template, script, or sequence step for each failure
- AND the brief MUST include the violated rule code and policy bundle version for each failure
- AND the brief MUST include concrete remediation guidance

#### Scenario: Brief includes event or evidence references for denied decisions

- GIVEN one or more evaluated campaign steps produce denied decisions during preflight
- WHEN the brief is returned
- THEN the brief MUST include the related event references
- AND the brief MUST include any available evidence references for those denied decisions
- AND the brief MUST distinguish dry-run references from realtime send enforcement records

#### Scenario: Brief surfaces missing-context failures separately from rule violations

- GIVEN some campaign steps fail because required context is missing or ambiguous
- WHEN the brief is returned
- THEN the brief MUST identify those failures as fail-closed context gaps
- AND the brief MUST describe what context must be supplied or corrected before launch

### Requirement: Preflight rewrite suggestions remain draft-only

Preflight MAY include optional compliant rewrite suggestions for failing campaign content, but those suggestions MUST remain draft output and MUST NOT authorize or trigger sending.

#### Scenario: Preflight brief includes optional draft rewrites only

- GIVEN a failing campaign step has a safe compliant rewrite suggestion
- WHEN the preflight brief is returned
- THEN the rewrite MUST be marked as a draft suggestion for operator review
- AND the existence of the rewrite MUST NOT change the step's failed preflight status until the campaign is separately corrected and re-evaluated
