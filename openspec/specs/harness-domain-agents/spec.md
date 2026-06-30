# Harness Domain Agents Specification

## ADDED Requirements

### Requirement: Each Domain Agent has a static definition with instructions, tool allowlist, typed output schema, and validator

Each of the four Domain Agents SHALL be represented as a static `AgentDefinition` value carrying:
- `Name` — unique agent identifier
- `Instructions` — immutable system prompt string; MUST NOT change between invocations
- `ToolAllowlist` — ordered list of tool name strings the agent is permitted to call; the runtime MUST
  filter the tool registry to this list before invoking the agent
- `Budget` — maximum step budget for the agent's run loop
- `Validator` — a typed function conforming to `harness.Validator` that inspects the agent's final
  output and returns either success or a failure with feedback text
- A `decode` function that deserializes the agent's final output string into the agent's typed Handoff
  Artifact

| Agent | ToolAllowlist |
|-------|--------------|
| `PolicyExplainer` | `list_applicable_rules`, `read_policy_rule` |
| `CaseInvestigator` | `read_case` |
| `EvidencePackager` | `draft_evidence_manifest` |
| `SupervisorNoteDrafter` | `draft_supervisor_note` |

#### Scenario: PolicyExplainer AgentDefinition carries correct allowlist, non-empty instructions, and non-nil validator

- **Given** the `PolicyExplainer` `AgentDefinition`
- **When** its fields are inspected
- **Then** `ToolAllowlist` is exactly `["list_applicable_rules", "read_policy_rule"]`
- **And** `Instructions` is a non-empty string
- **And** `Validator` and `decode` are non-nil

#### Scenario: CaseInvestigator AgentDefinition is limited to read_case

- **Given** the `CaseInvestigator` `AgentDefinition`
- **When** its `ToolAllowlist` is inspected
- **Then** the list contains exactly one entry: `"read_case"`

#### Scenario: EvidencePackager AgentDefinition is limited to draft_evidence_manifest

- **Given** the `EvidencePackager` `AgentDefinition`
- **When** its `ToolAllowlist` is inspected
- **Then** the list contains exactly one entry: `"draft_evidence_manifest"`

#### Scenario: SupervisorNoteDrafter AgentDefinition is limited to draft_supervisor_note

- **Given** the `SupervisorNoteDrafter` `AgentDefinition`
- **When** its `ToolAllowlist` is inspected
- **Then** the list contains exactly one entry: `"draft_supervisor_note"`

---

### Requirement: Handoff Artifacts are typed Go structs with JSON tags; each chains forward and is schema-validated

The four Handoff Artifact types — `PolicyExplanation`, `CaseInvestigation`, `EvidenceManifestDraft`,
`SupervisorNoteDraft` — SHALL be defined as Go structs with JSON field tags. Each artifact SHALL carry a
`CaseID` string field for Case traceability. Handoff Artifacts are the ONLY data forwarded from a completed
agent to the next agent in the chain. Handoff Artifacts MUST NOT be free-form strings or markdown-only
blobs; all semantically meaningful content SHALL occupy named and typed fields. Each artifact is decoded and
validated by the consuming agent's definition before use.

#### Scenario: A valid final output decodes to the expected typed Handoff Artifact

- **Given** a Fake Model Provider configured to emit a syntactically valid JSON string for `PolicyExplainer`
- **When** the `PolicyExplainer` `decode` function is applied to that output
- **Then** the decoded `PolicyExplanation` struct is non-nil and carries a non-empty `CaseID`
- **And** all required fields in the struct are populated with non-zero values

#### Scenario: A malformed final output fails artifact schema validation

- **Given** a Fake Model Provider configured to emit JSON missing a required field for `PolicyExplanation`
- **When** the `PolicyExplainer` validator processes the final output
- **Then** the validation result is a failure
- **And** the failure carries a reason string identifying the missing or invalid field

---

### Requirement: Forbidden authority-claim validation uses typed-field checks and a bounded, enumerated claim-term list

Handoff Artifact validation MUST reject authority claims using two complementary, statically defined
mechanisms:

1. **Typed-field checks**: validation MUST return failure if any decoded artifact field named `authoritative`
   is `true`, or any decoded artifact field named `persisted` is `true`.
2. **Bounded claim-term list**: validation MUST check nominated string fields against the following exact,
   fixed set of forbidden tokens (case-insensitive, whole-token match): `approved`, `approval_granted`,
   `block_campaign`, `campaign_blocked`, `override_to_compliant`, `ledger_committed`. This check MUST NOT
   use NLP classifiers, ML models, or free-text sentiment analysis; the comparison is a string containment
   check against the enumerated list only.

#### Scenario: Artifact with authoritative=true typed field is rejected (approval category)

- **Given** a Fake Model Provider producing a final output whose decoded artifact carries `authoritative == true`
- **When** the agent validator runs
- **Then** the validation result is a failure
- **And** the failure reason identifies the `authoritative` field as the cause

#### Scenario: Artifact with persisted=true typed field is rejected (ledger-update category)

- **Given** a Fake Model Provider producing a final output whose decoded artifact carries `persisted == true`
- **When** the agent validator runs
- **Then** the validation result is a failure
- **And** the failure reason identifies the `persisted` field as the cause

#### Scenario: Artifact containing a campaign-blocking claim token is rejected

- **Given** a Fake Model Provider producing a final output whose decoded artifact's string field contains the
  token `block_campaign` or `campaign_blocked`
- **When** the agent validator scans the bounded claim-term list
- **Then** the validation result is a failure
- **And** the failure reason names the matched forbidden token

#### Scenario: Artifact containing an override-to-compliant claim token is rejected

- **Given** a Fake Model Provider producing a final output whose decoded artifact's string field contains the
  token `override_to_compliant`
- **When** the agent validator scans the bounded claim-term list
- **Then** the validation result is a failure
- **And** the failure reason names the matched forbidden token

---

### Requirement: Per-agent validation retries once with derived feedback; second failure stops the agent

On the first validation failure for an agent's handoff synthesis, the orchestrator SHALL re-invoke that
agent exactly once. The re-invocation input MUST append the feedback text extracted from the
`EventValidationFailure` event produced during the first run. The retry MUST NOT be a blind identical
re-send of the original input; the feedback must be derived from the first failure event. A second
consecutive validation failure SHALL stop that agent with no further retry.

#### Scenario: Retry with feedback succeeds on the second attempt

- **Given** a Fake Model Provider configured to fail validation on the first synthesis attempt and pass on
  the second
- **When** the orchestrator runs the agent
- **Then** the agent synthesis step is invoked exactly twice
- **And** the second invocation includes the validation failure feedback derived from the first failure event
- **And** the final result carries a valid, decoded Handoff Artifact from the second attempt

#### Scenario: Retry fails on the second attempt; agent is stopped without a handoff

- **Given** a Fake Model Provider configured to fail validation on both the first and second synthesis
  attempts
- **When** the orchestrator runs the agent
- **Then** the agent synthesis step is invoked exactly twice and no third invocation occurs
- **And** no Handoff Artifact is produced
- **And** the failure result carries the agent name and the reason from the second failure event

---

### Requirement: Transcript and debtor speech in agent inputs is untrusted data

Transcript content and debtor utterances passed to a Domain Agent in its input context SHALL be treated as
inert typed data. Agent instructions MUST NOT direct the model to evaluate, follow, or treat transcript
content as executable instructions, tool calls, or control flow input. This invariant applies regardless of
the linguistic form of the debtor's utterance.

#### Scenario: Instruction-like text in debtor utterances does not alter tool dispatch

- **Given** a `CaseInvestigator` input where the `read_case` response contains debtor utterances with
  instruction-like phrases
- **When** the Fake Model Provider processes the agent step
- **Then** no tool call is attempted outside the `CaseInvestigator` ToolAllowlist (`read_case`)
- **And** the debtor utterance text appears only as the `text` string field of a typed transcript utterance
  struct and does not influence which tools are called
