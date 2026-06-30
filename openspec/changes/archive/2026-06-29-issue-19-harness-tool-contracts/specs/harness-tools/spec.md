# Harness Tools Delta Specification

## ADDED Requirements

### Requirement: RiskClass taxonomy as a static tool-contract property

Every Harness tool contract SHALL declare a static `RiskClass` property. The three classes are
`read`, `draft`, and `authority`. The class is a property of the contract itself, not of any
runtime invocation, and MUST NOT change between calls. The five read/draft tools and the four
authority-bearing tools each carry exactly one class per the table in the proposal.

#### Scenario: Read tools carry RiskClass "read"

- **Given** the tool contracts for `read_case`, `read_policy_rule`, and `list_applicable_rules`
- **When** the declared `RiskClass` property of each contract is inspected
- **Then** each contract's RiskClass is `read`

#### Scenario: Draft tools carry RiskClass "draft"

- **Given** the tool contracts for `draft_evidence_manifest` and `draft_supervisor_note`
- **When** the declared `RiskClass` property of each contract is inspected
- **Then** each contract's RiskClass is `draft`

#### Scenario: Authority-bearing tools are associated with RiskClass "authority"

- **Given** the conceptual tool descriptions for `append_evidence`, `update_case_state`,
  `submit_report`, and `block_campaign`
- **When** the RiskClass that would apply to each is described
- **Then** all four carry RiskClass `authority`
- **And** no registered tool implementation for any of them exists in the #19 tool registry

---

### Requirement: Typed request and response schema for read_case

`read_case` SHALL accept a typed request with the following field:
- `case_id` — non-empty string identifier for the target Case

It SHALL return a typed response carrying:
- `case_id` — echoed from the request
- `tenant_id` — the owning tenant identifier from the fixture
- `debtor` — structured value with at minimum a synthetic display label (no real PII)
- `collector` — structured value with a despacho identifier and display label
- `transcript` — an ordered, non-empty sequence of utterances; each utterance carries `speaker`
  and `text` as typed string fields
- `channel` — contact channel label (e.g., `voice`, `sms`)
- `occurred_at` — RFC 3339 timestamp of the contact
- `debtor_timezone` — IANA timezone string (e.g., `America/Mexico_City`)
- `detector_results` — list of pre-computed outcome records, each with `rule_code`,
  `detector_kind`, and `outcome`
- `applicable_rule_ids` — ordered list of rule code strings
- `evidence_metadata` — placeholder object representing where an EvidenceRecord reference would
  appear; MUST NOT be a committed EvidenceRecord

#### Scenario: read_case returns complete fixture data for the synthetic case

- **Given** a registered `read_case` tool backed by the embedded synthetic Case fixture
- **And** a typed request with `case_id` set to the synthetic fixture's identifier
- **When** the tool executes
- **Then** the result status is `success`
- **And** the response includes `tenant_id`, `debtor`, `collector`, `transcript`, `channel`,
  `occurred_at`, `debtor_timezone`, `detector_results`, `applicable_rule_ids`, and
  `evidence_metadata`
- **And** `transcript` is a non-empty sequence of typed utterances with `speaker` and `text`
- **And** no network or database call is made

#### Scenario: read_case with an unrecognized case_id returns a non-success status

- **Given** a registered `read_case` tool backed by the embedded synthetic Case fixture
- **And** a typed request with a `case_id` that matches no loaded fixture
- **When** the tool executes
- **Then** the result status is not `success`
- **And** the response includes a `reason` describing the lookup failure

---

### Requirement: Typed request and response schema for read_policy_rule

`read_policy_rule` SHALL accept a typed request with the following field:
- `rule_code` — non-empty string identifying the policy rule (e.g., `MX-REDECO-04`)

It SHALL return a typed response carrying:
- `code` — the rule identifier, equal to the request `rule_code`
- `title` — human-readable rule title
- `description` — regulatory description of the prohibited or required behavior
- `severity` — rule severity label consistent with the rule's system action (e.g., `hard_block`,
  `warn`)

#### Scenario: read_policy_rule returns fixture data for MX-REDECO-04

- **Given** a registered `read_policy_rule` tool backed by embedded synthetic rule fixtures
- **And** a typed request with `rule_code` set to `MX-REDECO-04`
- **When** the tool executes
- **Then** the result status is `success`
- **And** the response `code` equals `MX-REDECO-04`
- **And** `title`, `description`, and `severity` are non-empty
- **And** no network or database call is made

#### Scenario: read_policy_rule returns fixture data for MX-REDECO-05

- **Given** a registered `read_policy_rule` tool backed by embedded synthetic rule fixtures
- **And** a typed request with `rule_code` set to `MX-REDECO-05`
- **When** the tool executes
- **Then** the result status is `success`
- **And** the response `code` equals `MX-REDECO-05`
- **And** `severity` is `hard_block`

#### Scenario: read_policy_rule with an unrecognized rule_code returns a non-success status

- **Given** a registered `read_policy_rule` tool backed by embedded synthetic rule fixtures
- **And** a typed request with a `rule_code` that matches no loaded fixture
- **When** the tool executes
- **Then** the result status is not `success`
- **And** the response includes a `reason` describing the lookup failure

---

### Requirement: Typed request and response schema for list_applicable_rules

`list_applicable_rules` SHALL accept a typed request with the following field:
- `case_id` — non-empty string identifier for the target Case

It SHALL return a typed response carrying:
- `rules` — an ordered list of rule summaries; each summary carries `code`, `title`, and
  `severity`; the list is the intersection of the Case's `applicable_rule_ids` and the loaded
  synthetic rule fixtures, preserving the order of `applicable_rule_ids`

#### Scenario: list_applicable_rules returns all applicable rules for the synthetic case

- **Given** a registered `list_applicable_rules` tool backed by the embedded synthetic Case and
  rule fixtures
- **And** a typed request with `case_id` set to the synthetic fixture's identifier
- **When** the tool executes
- **Then** the result status is `success`
- **And** the `rules` list includes a summary for `MX-REDECO-04` and one for `MX-REDECO-05`
- **And** each summary includes `code`, `title`, and `severity`
- **And** no network or database call is made

---

### Requirement: Typed request and response schema for draft_evidence_manifest

`draft_evidence_manifest` SHALL accept a typed request with the following fields:
- `case_id` — non-empty string
- `rule_codes` — non-empty list of rule code strings
- `findings` — non-empty string describing the findings to be recorded

It SHALL return a typed response carrying:
- `case_id` — echoed from the request, establishing Case traceability
- `rule_codes` — echoed from the request, establishing rule traceability
- `findings` — echoed from the request
- `proposed_at` — RFC 3339 timestamp of when the draft was produced
- `authoritative` — boolean, always `false`
- `persisted` — boolean, always `false`

The tool MUST NOT write to any store, database, file, or EvidenceRecord. The response is a
proposed artifact only.

#### Scenario: draft_evidence_manifest returns a non-persisted proposed manifest

- **Given** a registered `draft_evidence_manifest` tool
- **And** a typed request with a valid `case_id`, `rule_codes` containing `MX-REDECO-04`, and a
  non-empty `findings` string
- **When** the tool executes
- **Then** the result status is `success`
- **And** the response `case_id` matches the request `case_id`
- **And** the response `rule_codes` matches the request `rule_codes`
- **And** `authoritative` is `false`
- **And** `persisted` is `false`
- **And** no write to any persistence layer occurs

---

### Requirement: Typed request and response schema for draft_supervisor_note

`draft_supervisor_note` SHALL accept a typed request with the following fields:
- `case_id` — non-empty string
- `rule_codes` — non-empty list of rule code strings
- `note_body` — non-empty string with the text of the proposed supervisor note

It SHALL return a typed response carrying:
- `case_id` — echoed from the request, establishing Case traceability
- `rule_codes` — echoed from the request, establishing rule traceability
- `note_body` — echoed from the request
- `proposed_at` — RFC 3339 timestamp of when the draft was produced
- `authoritative` — boolean, always `false`
- `persisted` — boolean, always `false`

The tool MUST NOT write to any store. The response is a proposed note only.

#### Scenario: draft_supervisor_note returns a non-persisted proposed note

- **Given** a registered `draft_supervisor_note` tool
- **And** a typed request with a valid `case_id`, non-empty `rule_codes`, and a non-empty
  `note_body`
- **When** the tool executes
- **Then** the result status is `success`
- **And** the response `case_id` matches the request `case_id`
- **And** `authoritative` is `false`
- **And** `persisted` is `false`
- **And** no write to any persistence layer occurs

---

### Requirement: Read tools are deterministic and fixture-backed with no external dependencies

Read tool implementations (`read_case`, `read_policy_rule`, `list_applicable_rules`) SHALL source
all data exclusively from embedded synthetic fixtures. They SHALL NOT make network calls, open
database connections, or read from the filesystem at runtime. Results are deterministic: the same
request always returns structurally equal data.

#### Scenario: Read tools operate without network or database

- **Given** a clean test environment with no running external service or database
- **When** `read_case`, `read_policy_rule`, and `list_applicable_rules` are each called with valid
  inputs in table-driven tests
- **Then** each returns a successful result
- **And** no network call is made
- **And** no database connection is opened
- **And** the result is identical on repeated invocations with the same input

---

### Requirement: Read tools scope results to the Case's fixture tenant

A read tool response MUST be consistent with the tenant identified in the referenced Case fixture.
Requesting data via a `case_id` must not surface data attributed to a different tenant identity.

#### Scenario: read_case response tenant matches the Case fixture tenant

- **Given** the embedded synthetic Case fixture with a declared `tenant_id`
- **When** `read_case` is called with that fixture's `case_id`
- **Then** the response `tenant_id` matches the fixture's `tenant_id`
- **And** the response does not include data fields from a different tenant's fixture

---

### Requirement: Risk-class-aware lab permission gate

The lab permission gate SHALL implement the existing `PermissionGate` interface and decide
permission solely based on the tool's declared `RiskClass`:
- RiskClass `read` → `PermissionAllowed`
- RiskClass `draft` → `PermissionAllowed`
- RiskClass `authority` → `PermissionDenied` or `PermissionApprovalRequired`

The gate MUST be deterministic given the same tool name; it MUST NOT call any external service to
make its decision.

#### Scenario: Read tool call is allowed by the lab permission gate

- **Given** the lab permission gate
- **And** a proposed tool call for `read_case` (RiskClass: `read`)
- **When** the gate evaluates the call
- **Then** the decision kind is `allowed`

#### Scenario: Draft tool call is allowed by the lab permission gate

- **Given** the lab permission gate
- **And** a proposed tool call for `draft_evidence_manifest` (RiskClass: `draft`)
- **When** the gate evaluates the call
- **Then** the decision kind is `allowed`

#### Scenario: Authority tool call is denied or marked approval_required by the lab permission gate

- **Given** the lab permission gate
- **And** a proposed tool call whose declared RiskClass is `authority` (e.g., a call named
  `append_evidence`)
- **When** the gate evaluates the call
- **Then** the decision kind is `denied` or `approval_required`
- **And** the decision kind is never `allowed`

---

### Requirement: Authority-bearing tools are absent from the #19 registry and never execute

The four authority-bearing tools (`append_evidence`, `update_case_state`, `submit_report`,
`block_campaign`) SHALL NOT be registered as callable implementations in the #19 tool registry.
A proposed tool call for any of these tools SHALL be stopped before reaching any implementation
that could produce regulatory side effects.

#### Scenario: Proposed authority tool call is stopped and never executes

- **Given** the #19 lab tool registry containing only read and draft tool implementations
- **And** a proposed tool call for `append_evidence`
- **When** the runtime evaluates the call with the lab permission gate
- **Then** the call does not reach any tool implementation
- **And** the result status is `denied`, `approval_required`, or `not_found`
- **And** no evidence record, case state change, report artifact, or campaign action is produced

---

### Requirement: Untrusted-data invariant for transcript and debtor speech

Transcript content and debtor speech returned in tool responses SHALL be surfaced only as typed
data fields. Tool implementations MUST NOT parse, evaluate, or forward transcript text as
instructions, tool names, or any form of control flow input.

#### Scenario: Transcript content in read_case is inert typed data

- **Given** the embedded synthetic Case fixture whose transcript includes utterances from the
  debtor
- **When** `read_case` executes and the response transcript is inspected
- **Then** each utterance is a typed struct with `speaker` and `text` string fields
- **And** no part of any `text` value influences which tool is called or how the tool routes
- **And** the tool implementation does not evaluate transcript text as instructions
