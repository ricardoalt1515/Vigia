# Harness Case Brief Output Specification

## Purpose

The Case Brief OUTPUT contract turns the #20 in-memory `caseflow.CaseBrief` (interface-typed
`Stages[].Handoff`, not directly JSON-serializable) into three reviewable files: a schema-valid
`.brief.json`, a Spanish `.brief.md`, and an English `.jsonl` operational event log. `#21` owns this
rendering; it MUST NOT alter #20 types.

## ADDED Requirements

### Requirement: Case Brief JSON is produced by a forward-only serialization DTO that flattens each Handoff via Kind()

The CLI SHALL define a serialization DTO and a forward marshaler that type-switches on each stage's
`Handoff.Kind()` and marshals the concrete handoff struct (`PolicyExplanation`, `CaseInvestigation`,
`EvidenceManifestDraft`, `SupervisorNoteDraft`) into the flattened `.brief.json` output. The marshaler is
forward-only: it MUST NOT support unmarshaling JSON back into the interface-typed `CaseBrief`.

#### Scenario: All four handoff kinds are flattened correctly

- **GIVEN** a completed `CaseBrief` with all four stages present
- **WHEN** the marshaler serializes it
- **THEN** `.brief.json` contains four stage entries
- **AND** each stage entry's fields correspond to the concrete fields of its handoff kind
  (`PolicyExplanation`, `CaseInvestigation`, `EvidenceManifestDraft`, or `SupervisorNoteDraft`), selected
  via `Kind()`

#### Scenario: Incomplete run flattens FailedAgent and FailureReason

- **GIVEN** a `CaseBrief` with `Status: incomplete`, a populated `FailedAgent`, and a populated
  `FailureReason`
- **WHEN** the marshaler serializes it
- **THEN** `.brief.json` includes `status: "incomplete"`, the failed agent name, and the failure reason
- **AND** `.brief.json` includes only the stages that were successfully produced before the failure

### Requirement: `.brief.json` MUST validate against a committed JSON Schema

The repository SHALL include a committed JSON Schema describing the Case Brief output contract
(`case_id`, `status`, ordered `stages` with per-kind handoff fields, optional `failed_agent` /
`failure_reason`). Every `.brief.json` produced by a CLI run MUST validate against this schema.

#### Scenario: Generated brief.json validates against the schema

- **GIVEN** a completed or incomplete orchestrator run
- **WHEN** `.brief.json` is written
- **THEN** validating it against the committed JSON Schema succeeds with no errors

### Requirement: `.brief.md` is rendered in neutral professional Spanish and carries a mandatory review disclaimer

The CLI SHALL render `<case_id>.brief.md` in neutral professional Spanish. It MUST include a disclaimer
stating the output is a DRAFT requiring review by a Compliance Supervisor before any action is taken. This
is the only Spanish artifact the CLI produces; all other artifacts stay English.

#### Scenario: brief.md is Spanish and carries the disclaimer

- **GIVEN** a completed orchestrator run
- **WHEN** `.brief.md` is rendered
- **THEN** the document body is written in neutral professional Spanish
- **AND** it contains a visible disclaimer stating the brief is a DRAFT pending Compliance Supervisor
  review

#### Scenario: brief.md does not leak English JSON keys as prose

- **GIVEN** a completed orchestrator run
- **WHEN** `.brief.md` is rendered
- **THEN** raw JSON field names (e.g. `case_id`, `failed_agent`, `failure_reason`) do not appear as
  unlabeled prose text; any such values are presented through Spanish labels

### Requirement: Untrusted transcript and debtor text is rendered as data, never as instructions

Transcript excerpts, debtor speech, or other collection-agent content rendered into `.brief.md` or
`.brief.json` SHALL be treated strictly as untrusted display data. Rendering MUST NOT interpret such
content as formatting directives, template control sequences, or instructions to the renderer.

#### Scenario: Transcript content is displayed verbatim, not interpreted

- **GIVEN** a handoff field containing debtor/collector transcript text with markup-like or
  instruction-like substrings
- **WHEN** `.brief.md` is rendered
- **THEN** the text appears verbatim as quoted/display content
- **AND** it does not alter the disclaimer, document structure, or other rendered sections

### Requirement: The JSONL event log contains only structured operational events, never chain-of-thought

The CLI SHALL write `<case_id>.jsonl` containing one JSON object per line, where each line is a structured
`harness.Event` of one of the eight operational types (`agent_started`, `plan_created`, `tool_proposed`,
`permission_decision`, `tool_result`, `validation_failure`, `budget_exceeded`, `agent_completed`). Each
line MUST be annotated with the producing `agent_name` and a monotonic `sequence` number for the run. The
log MUST NOT contain raw, unstructured model chain-of-thought.

#### Scenario: JSONL contains only known operational event types, annotated per agent

- **GIVEN** a completed orchestrator run across all four agents
- **WHEN** `.jsonl` is written
- **THEN** every line parses as JSON and its `type` field is one of the eight known operational event
  types
- **AND** every line carries an `agent_name` and a `sequence` value
- **AND** sequence numbers are monotonically increasing across the whole run

#### Scenario: Incomplete run's event log stops at the failing agent

- **GIVEN** a Fake provider script causing one agent to fail validation twice
- **WHEN** the orchestrator stops the run
- **THEN** `.jsonl` contains events only up to and including that agent's terminal `validation_failure` and
  `agent_completed`/stop events
- **AND** no events from downstream, un-invoked agents are present

### Requirement: Repeated runs with the same Fake provider script produce structurally identical output

For a fixed Fake provider script and fixed `case_id`, running the CLI twice SHALL produce
`.brief.json` and `.jsonl` content that is structurally identical between runs (field values, ordering,
and structure), excluding any wall-clock timestamp fields which MUST be either omitted or explicitly
normalized before comparison.

#### Scenario: Two consecutive runs produce identical brief.json and jsonl content

- **GIVEN** the same Fake provider script and the same `case_id`
- **WHEN** the CLI is run twice in sequence
- **THEN** the two `.brief.json` outputs are structurally identical apart from any timestamp fields
- **AND** the two `.jsonl` outputs are structurally identical apart from any timestamp fields
