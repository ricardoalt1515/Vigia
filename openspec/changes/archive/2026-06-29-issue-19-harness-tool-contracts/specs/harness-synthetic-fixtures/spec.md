# Harness Synthetic Fixtures Delta Specification

## ADDED Requirements

### Requirement: Synthetic Case fixture carries all required fields

The synthetic Case fixture SHALL be a valid Case document with all of the following fields
present and non-empty:
- `case_id` — unique fixture identifier
- `tenant_id` — synthetic, non-real tenant identifier
- `debtor` — structured value with at minimum a synthetic display label (no real PII)
- `collector` — structured value with a despacho identifier and display label
- `transcript` — non-empty ordered sequence of typed utterances (each with `speaker` and `text`)
- `channel` — contact channel label
- `occurred_at` — RFC 3339 timestamp that places the contact outside the 08:00–21:00 allowed
  window in the debtor's declared timezone
- `debtor_timezone` — valid IANA timezone string (e.g., `America/Mexico_City`)
- `detector_results` — non-empty list of pre-computed outcome records, at minimum one per
  applicable rule
- `applicable_rule_ids` — non-empty ordered list that includes at minimum `MX-REDECO-04` and
  `MX-REDECO-05`
- `evidence_metadata` — placeholder object; MUST NOT be a committed EvidenceRecord or hash-chain
  entry

#### Scenario: Loaded Case fixture passes required-field validation

- **Given** the embedded synthetic Case fixture loaded via the fixture loader
- **When** all fields are inspected against the required field list
- **Then** every required field is present and non-empty
- **And** `occurred_at` places the contact outside 08:00–21:00 in the declared `debtor_timezone`
- **And** `applicable_rule_ids` contains both `MX-REDECO-04` and `MX-REDECO-05`
- **And** `detector_results` contains at least one outcome record per entry in
  `applicable_rule_ids`
- **And** `evidence_metadata` is a placeholder object, not a committed EvidenceRecord

---

### Requirement: Synthetic Case fixture contains no real PII

The synthetic Case fixture SHALL NOT contain real debtor names, surnames, addresses, phone
numbers, email addresses, CURP, RFC, account numbers, or any other attribute traceable to a
real person or financial account. All personally identifiable fields MUST use clearly synthetic
placeholder values.

#### Scenario: Debtor field contains only synthetic placeholder data

- **Given** the embedded synthetic Case fixture
- **When** the `debtor` field is inspected
- **Then** the display label is a clearly synthetic identifier (e.g., `Debtor-Synthetic-001`)
- **And** no field value contains a real CURP, RFC, address, phone number, or email address

---

### Requirement: Detector results in the Case fixture are static pre-computed data

The `detector_results` field in the synthetic Case fixture carries pre-computed, static outcome
records. In #19 no detector executes and no LLM-judge invocation occurs; the fixture carries the
expected outcomes as data so that tool behavior and runtime safety invariants can be tested
without live processing.

The fixture MUST carry a detector result for `MX-REDECO-04` with:
- `rule_code`: `MX-REDECO-04`
- `detector_kind`: `deterministic`
- `outcome`: `hard_block` — the `occurred_at` timestamp is outside the 08:00–21:00 window in
  the fixture's declared `debtor_timezone`

The fixture MUST carry a detector result for `MX-REDECO-05` with:
- `rule_code`: `MX-REDECO-05`
- `detector_kind`: `llm_judge`
- `outcome`: `hard_block` with `hitl_required` set to `true` — a threatening-tone candidate
  that demands mandatory human review

#### Scenario: MX-REDECO-04 detector result records a hard_block for out-of-hours contact

- **Given** the embedded synthetic Case fixture
- **When** the `detector_results` list is inspected for the entry with `rule_code`
  `MX-REDECO-04`
- **Then** `detector_kind` is `deterministic`
- **And** `outcome` is `hard_block`
- **And** the fixture's `occurred_at` timestamp is verifiably outside 08:00–21:00 in
  `debtor_timezone`

#### Scenario: MX-REDECO-05 detector result records a hard_block with mandatory HITL

- **Given** the embedded synthetic Case fixture
- **When** the `detector_results` list is inspected for the entry with `rule_code`
  `MX-REDECO-05`
- **Then** `detector_kind` is `llm_judge`
- **And** `outcome` is `hard_block`
- **And** `hitl_required` is `true`

#### Scenario: No detector or LLM-judge executes during fixture-based tests

- **Given** a test exercising read or draft tools backed by the synthetic Case fixture
- **When** the test runs
- **Then** no deterministic detector logic executes
- **And** no LLM-judge or model provider is called
- **And** detector outcomes are read directly from the pre-computed `detector_results` field

---

### Requirement: Synthetic policy-rule fixtures for MX-REDECO-04 and MX-REDECO-05

Two minimal synthetic policy-rule fixtures SHALL exist, one for `MX-REDECO-04` and one for
`MX-REDECO-05`. Each fixture carries the following fields:
- `code` — rule identifier string
- `title` — human-readable rule title
- `description` — regulatory description of the prohibited or required behavior
- `severity` — severity label consistent with the rule's system action

These synthetic fixtures are compatible in spirit with the `core.PolicyRule` shape
(`code`, `title`, `description`, `severity`) but are NOT persisted to the Postgres
`policy_rules` table and do NOT depend on sqlc, migrations, or any database persistence path.

The `MX-REDECO-04` synthetic rule fixture SHALL specify:
- `code`: `MX-REDECO-04`
- `title`: a title referencing the contact-hours restriction
- `description`: a description stating that contact is permitted only on business days between
  08:00 and 21:00 in the debtor's timezone, and that contact outside this window is a hard block
- `severity`: `hard_block`

The `MX-REDECO-05` synthetic rule fixture SHALL specify:
- `code`: `MX-REDECO-05`
- `title`: a title referencing the threatening-tone prohibition
- `description`: a description stating that threats, offense, intimidation, and harassment are
  prohibited, and that violations trigger a hard block with mandatory human review
- `severity`: `hard_block`

#### Scenario: MX-REDECO-04 synthetic rule fixture is loadable and structurally correct

- **Given** the embedded synthetic rule fixtures
- **When** the rule fixture with `code` `MX-REDECO-04` is loaded
- **Then** `code` equals `MX-REDECO-04`
- **And** `severity` is `hard_block`
- **And** `description` references the 08:00–21:00 contact-hours restriction
- **And** `title` and `description` are non-empty strings

#### Scenario: MX-REDECO-05 synthetic rule fixture is loadable and structurally correct

- **Given** the embedded synthetic rule fixtures
- **When** the rule fixture with `code` `MX-REDECO-05` is loaded
- **Then** `code` equals `MX-REDECO-05`
- **And** `severity` is `hard_block`
- **And** `description` references the prohibition on threats and intimidation
- **And** `title` and `description` are non-empty strings

---

### Requirement: Fixtures are embedded and require no filesystem or network dependency at test time

All synthetic fixture data (Case fixture and rule fixtures) SHALL be embedded in the compiled
artifact (e.g., via `//go:embed` in Go) so they are available without any filesystem path
traversal outside the embedded data or network access. The fixture loader MUST load successfully
in a clean test environment with no running services.

#### Scenario: Fixtures load in a clean test environment with no external service

- **Given** a clean test environment without a running database, network access, or MinIO
- **When** the synthetic Case fixture and both synthetic rule fixtures are loaded via the fixture
  loader
- **Then** all three load successfully
- **And** no network call is made
- **And** no filesystem path is traversed outside the embedded data

#### Scenario: Fixture data is identical on repeated loads

- **Given** the synthetic Case fixture embedded in the binary
- **When** the fixture loader is called twice with the same fixture identifier
- **Then** both calls return structurally equal data with identical field values

---

### Requirement: Rule-reference integrity between the Case fixture and the rule fixtures

Every rule code listed in the synthetic Case fixture's `applicable_rule_ids` field SHALL resolve
to a corresponding synthetic policy-rule fixture. No dangling references are permitted.

#### Scenario: All applicable_rule_ids resolve to loaded synthetic rule fixtures

- **Given** the embedded synthetic Case fixture with `applicable_rule_ids` containing
  `MX-REDECO-04` and `MX-REDECO-05`
- **And** the embedded set of synthetic rule fixtures
- **When** each `applicable_rule_id` is looked up in the synthetic rule fixture set
- **Then** every lookup succeeds with a non-nil rule fixture
- **And** no `applicable_rule_id` is left unresolved

#### Scenario: Synthetic rule fixture set does not include rules absent from the Case fixture

- **Given** the embedded set of synthetic rule fixtures for #19 (MX-REDECO-04 and MX-REDECO-05)
- **When** the fixture set is enumerated
- **Then** both fixture entries appear in the Case fixture's `applicable_rule_ids`
- **And** no synthetic rule fixture is an orphan unreachable from the Case fixture's rule list
