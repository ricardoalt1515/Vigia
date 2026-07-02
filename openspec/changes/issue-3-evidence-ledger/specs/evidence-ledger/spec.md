# Evidence Ledger Specification

## Purpose

Define the testable requirements for issue #3 — the append-only,
hash-chained evidence ledger that turns issue #2's `Evaluation` +
`detector_result_rows` spine into tamper-evident proof. Every evaluation MUST
produce exactly one immutable `EvidenceRecord`, chained by hash to the
tenant's previous record, verifiable independently (`VerifyChain`), and
exportable as a self-contained, DB-free-verifiable package
(`VerifyPackage`) for a single interaction. This is the trust core: without
it, Vigía has verdicts but no proof that the verdicts were not altered after
the fact.

## Testing mode note

Strict TDD applies to all Go components. Requirements marked `[unit]` MUST
run with no external dependencies (pure hashing/canonicalization/verify
logic, table-driven tests). Requirements marked `[integration]` require a
real Postgres instance and MUST be skippable with `testing.Short()`.
Requirements marked `[manual-demo]` are validated by a human running the
local dev environment (seed + export + CLI verify).

---

## Requirement: Evidence Append Is Atomic and Exactly-Once

Appending an `EvidenceRecord` MUST happen inside the same
`tenantdb.WithTenantTx` transaction that persists the `evaluations` header
and `detector_result_rows` children for that evaluation — never as a
separate, later step. `evidence_records` MUST carry
`UNIQUE (tenant_id, evaluation_id)`, guaranteeing at most one record per
evaluation. If the transaction rolls back for any reason, neither the
evaluation header, its detector rows, nor the evidence record MUST exist,
and the tenant's chain sequence MUST show no gap.

### Scenario: Successful evaluation produces exactly one evidence record `[integration]`

- GIVEN a seeded interaction exists for a tenant with no prior evaluation
- WHEN the evaluation path runs and commits successfully
- THEN exactly one `evidence_records` row MUST exist with that evaluation's
  `evaluation_id`
- AND a second attempt to insert an evidence record for the same
  `evaluation_id` MUST fail on `UNIQUE (tenant_id, evaluation_id)`.

### Scenario: Evidence append shares the evaluation's transaction `[integration]`

- GIVEN the `CreateEvaluation` code path is reviewed
- WHEN it writes the `evaluations` header, `detector_result_rows` children,
  and the `EvidenceRecord`
- THEN all three writes MUST occur inside one `tenantdb.WithTenantTx` call
- AND no evidence write MUST occur outside that transaction or after it
  commits.

### Scenario: Rollback leaves no evaluation, no evidence, and no sequence gap `[integration]`

- GIVEN an evaluation transaction is forced to fail after the
  `ledger_chain_heads` row has been locked but before commit (e.g. a
  simulated error on the detector-row insert)
- WHEN the transaction rolls back
- THEN no `evaluations` row, no `detector_result_rows` row, and no
  `evidence_records` row MUST exist for that attempt
- AND the tenant's `ledger_chain_heads.last_seq` MUST be unchanged
- AND the next successful evaluation for that tenant MUST receive
  `seq = last_seq + 1` with no gap from the failed attempt.

---

## Requirement: Chain Linkage and Per-Tenant Monotonic Sequence

Each tenant's evidence records MUST form a strictly increasing sequence
(`seq`) with no gaps under normal operation. Each record's `prev_hash` MUST
equal the immediately preceding record's `hash` for that tenant. The first
record ever appended for a tenant MUST use the fixed genesis sentinel
`prev_hash = ""` (empty string) — there is no prior record to reference.
Concurrent appends for the same tenant MUST serialize through a
`SELECT ... FOR UPDATE` (or equivalent locking upsert) on that tenant's
`ledger_chain_heads` row, so two concurrent evaluations for one tenant can
never produce the same `seq` or a forked `prev_hash`.

### Scenario: First record for a tenant uses the genesis prev_hash `[integration]`

- GIVEN a tenant has never had an evidence record appended
- WHEN its first evaluation is appended
- THEN the resulting record MUST have `seq = 1`
- AND `prev_hash` MUST equal the empty string `""`.

### Scenario: Subsequent records chain to the previous hash `[integration]`

- GIVEN a tenant already has an evidence record with `seq = N` and `hash = H`
- WHEN a new evaluation is appended for that tenant
- THEN the new record MUST have `seq = N + 1`
- AND the new record's `prev_hash` MUST equal `H`.

### Scenario: Sequence has no gaps under normal operation `[integration]`

- GIVEN a tenant has appended K evidence records via successful evaluations
- WHEN the records are listed ordered by `seq`
- THEN the sequence MUST be exactly `1, 2, ..., K` with no missing or
  duplicate values.

### Scenario: Concurrent appends for one tenant never fork the chain `[integration]`

- GIVEN two evaluations for the same tenant are submitted concurrently
- WHEN both transactions attempt to append an evidence record
- THEN the `ledger_chain_heads` row lock MUST serialize the two appends
- AND the resulting records MUST have distinct, sequential `seq` values with
  no duplicate `seq` and no forked `prev_hash`
- AND `UNIQUE (tenant_id, seq)` MUST reject any attempt that would violate
  this.

### Scenario: Concurrent appends across different tenants proceed independently `[integration]`

- GIVEN two evaluations for two different tenants are submitted concurrently
- WHEN both transactions attempt to append an evidence record
- THEN neither append MUST block on the other tenant's `ledger_chain_heads`
  row
- AND each tenant's chain MUST proceed independently and correctly.

---

## Requirement: Canonical Hashing Is Deterministic

Each record's `hash` MUST equal `sha256(prev_hash || canonical(body))`,
hex-encoded, where `body` is a fixed Go struct marshaled via
`encoding/json` in declaration order (never a map). `inputs_digest` MUST
equal `sha256(canonical(detector_results))` where `detector_results` are
sorted by `detector_code` before serialization, and each entry contributes
`code + outcome + severity + rationale`. The same inputs MUST always
produce the same hash.

### Scenario: Golden-hash test pins the exact canonical bytes `[unit]`

- GIVEN a fixed, hard-coded `EvidenceRecord` body (prev_hash, seq,
  overall_outcome, policy_bundle_version, inputs_digest)
- WHEN `Hash` is computed for that body
- THEN the resulting hex-encoded hash MUST equal a pinned golden value
- AND any accidental change to field order, field set, or serialization
  format MUST make this test fail.

### Scenario: Same inputs always yield the same hash `[unit]`

- GIVEN two `EvidenceRecord` bodies with identical field values
- WHEN `Hash` is computed for each independently
- THEN both computed hashes MUST be identical.

### Scenario: Detector results are sorted by detector_code before digesting `[unit]`

- GIVEN a set of detector results is provided in two different input orders
  but with the same members
- WHEN `inputs_digest` is computed for each input order
- THEN both computed digests MUST be identical
- AND the sort key MUST be `detector_code`.

### Scenario: inputs_digest changes when a detector result field changes `[unit]`

- GIVEN a baseline set of detector results and its `inputs_digest`
- WHEN any single field (`code`, `outcome`, `severity`, or `rationale`) of
  one detector result is changed
- THEN the recomputed `inputs_digest` MUST differ from the baseline.

---

## Requirement: VerifyChain Detects the First Break

`VerifyChain` MUST accept a tenant's evidence records ordered by `seq` and
recompute each record's `hash` from its own `prev_hash` and body, comparing
against the stored `hash` and confirming `prev_hash` matches the previous
record's stored `hash`. It MUST pass on an intact chain and MUST report the
first `seq` at which the chain breaks when any hash-contributing field
(`overall_outcome`, `inputs_digest`, `prev_hash`, `seq`, `policy_bundle_version`)
has been altered directly in storage. It MUST handle an empty chain and a
single-record chain without error.

### Scenario: VerifyChain passes on an intact chain `[unit]`

- GIVEN a tenant's evidence records were produced honestly via the append
  path (no direct storage edits)
- WHEN `VerifyChain` runs over those records ordered by `seq`
- THEN it MUST report the chain as intact with no break.

### Scenario: VerifyChain detects a tampered overall_outcome `[integration]`

- GIVEN a tenant has an intact chain of at least three evidence records
- WHEN one record's `overall_outcome` is altered by a direct SQL statement
  bypassing the application (e.g. issued with elevated privileges)
- THEN `VerifyChain` MUST report a break at that record's `seq`
- AND MUST NOT report the chain as intact.

### Scenario: VerifyChain detects a tampered inputs_digest `[integration]`

- GIVEN a tenant has an intact chain
- WHEN one record's `inputs_digest` is altered directly in storage
- THEN `VerifyChain` MUST report a break at that record's `seq`.

### Scenario: VerifyChain detects a tampered prev_hash `[integration]`

- GIVEN a tenant has an intact chain of at least two records
- WHEN a record's `prev_hash` is altered directly in storage to no longer
  match the preceding record's `hash`
- THEN `VerifyChain` MUST report a break at that record's `seq`.

### Scenario: VerifyChain detects a tampered seq `[integration]`

- GIVEN a tenant has an intact chain
- WHEN a record's `seq` value is altered directly in storage
- THEN `VerifyChain` MUST report a break, either at the altered record or at
  the point where sequence contiguity is violated.

### Scenario: VerifyChain handles an empty chain `[unit]`

- GIVEN a tenant has zero evidence records
- WHEN `VerifyChain` runs for that tenant
- THEN it MUST return a defined "no records" result
- AND it MUST NOT error or panic.

### Scenario: VerifyChain handles a single-record chain `[unit]`

- GIVEN a tenant has exactly one evidence record with `seq = 1` and
  `prev_hash = ""`
- WHEN `VerifyChain` runs for that tenant
- THEN it MUST verify that single record's hash against its own body and
  genesis `prev_hash`
- AND MUST report the chain as intact if the hash matches.

---

## Requirement: Evidence Records Are Write-Once

The application layer MUST have no code path (query, adapter method, or
sqlc-generated statement) that updates or deletes `evidence_records` rows.
A database-level `BEFORE UPDATE OR DELETE` trigger on `evidence_records`
MUST unconditionally `RAISE EXCEPTION`, regardless of the executing role or
row ownership.

### Scenario: Application layer exposes no update or delete path `[unit]`

- GIVEN the `internal/postgres` adapters and generated sqlc queries for
  `evidence_records` are reviewed
- WHEN searching for UPDATE or DELETE statements targeting
  `evidence_records`
- THEN none MUST exist.

### Scenario: Direct SQL UPDATE against evidence_records fails `[integration]`

- GIVEN an existing `evidence_records` row
- WHEN a direct SQL `UPDATE evidence_records SET ... WHERE id = ...`
  statement is executed against it, using the same role the application
  uses
- THEN the statement MUST fail with an exception raised by the write-once
  trigger
- AND the row's stored values MUST remain unchanged.

### Scenario: Direct SQL DELETE against evidence_records fails `[integration]`

- GIVEN an existing `evidence_records` row
- WHEN a direct SQL `DELETE FROM evidence_records WHERE id = ...` statement
  is executed against it, using the same role the application uses
- THEN the statement MUST fail with an exception raised by the write-once
  trigger
- AND the row MUST still exist afterward.

---

## Requirement: Evidence Package Export Is Self-Contained and Independently Verifiable

`GET /v1/interactions/{id}/evidence` MUST return a self-contained JSON
package for one interaction: the interaction, its evaluation, its detector
results, the applied versions embedded in the evidence body, and the chain
proof (`seq`, `prev_hash`, `hash`). `VerifyPackage` MUST re-derive the
record's `hash` from the package's own body and `prev_hash` alone, with no
database access. The endpoint MUST be tenant-isolated using the same
`Authorization`-header auth as existing interaction routes. An interaction
with no evaluation or evidence record MUST return a defined not-found or
empty shape — never a fabricated package.

### Scenario: Evidence package exports and independently verifies `[integration]`

- GIVEN a seeded interaction has been evaluated and has an evidence record
- WHEN `GET /v1/interactions/{id}/evidence` is called with a valid tenant
  API key
- THEN the response MUST include the interaction, evaluation, detector
  results, applied versions, and chain proof (`seq`, `prev_hash`, `hash`)
- AND calling `VerifyPackage` on the returned JSON alone (no database
  access) MUST succeed by recomputing the same `hash`.

### Scenario: Export is tenant-isolated `[integration]`

- GIVEN tenant A has an evaluated interaction with an evidence record
- WHEN tenant B calls `GET /v1/interactions/{id}/evidence` using tenant A's
  interaction id and tenant B's own API key
- THEN the response MUST be a not-found response
- AND MUST NOT leak tenant A's evidence data.

### Scenario: Interaction without evaluation returns a defined empty shape `[integration]`

- GIVEN a seeded interaction has not yet been evaluated
- WHEN `GET /v1/interactions/{id}/evidence` is called for that interaction
- THEN the response MUST return a defined not-found or empty shape
- AND MUST NOT fabricate a package with invented outcome, hash, or chain
  proof values.

### Scenario: VerifyPackage detects a tampered exported package `[unit]`

- GIVEN a valid evidence package JSON has been exported
- WHEN a field contributing to the hash (e.g. `overall_outcome` or
  `inputs_digest`) is altered in the JSON before calling `VerifyPackage`
- THEN `VerifyPackage` MUST report the package as invalid
- AND MUST NOT report it as verified.

---

## Requirement: No Backfill for Pre-Existing Evaluations

Evaluations created before migration `00005_evidence_ledger.sql` deploys
MUST NOT receive a retroactively fabricated `EvidenceRecord`. The chain for
each tenant begins with the first evaluation created after deployment.
`VerifyChain` and the export endpoint MUST handle interactions whose
evaluation predates the ledger gracefully, without error and without
fabricating a record.

### Scenario: Pre-migration evaluation has no evidence record `[integration]`

- GIVEN an `evaluations` row exists that was created before migration
  `00005_evidence_ledger.sql` was applied (simulated via direct insert
  bypassing the ledger append path)
- WHEN `evidence_records` is queried for that evaluation's id
- THEN no matching row MUST exist.

### Scenario: Export handles a pre-ledger evaluation without evidence `[integration]`

- GIVEN an interaction has an `evaluations` row but no corresponding
  `evidence_records` row (pre-ledger evaluation)
- WHEN `GET /v1/interactions/{id}/evidence` is called for that interaction
- THEN the response MUST return a defined not-found or partial shape
  indicating no evidence exists
- AND MUST NOT fabricate chain-proof fields (`seq`, `prev_hash`, `hash`).

---

## Requirement: Seed Produces Evidence Records for Demo Data

`cmd/seed dev-data` MUST run seeded evaluations through the same
evidence-append path as production evaluations, so seeded evaluations
produce real `evidence_records` rows that the export endpoint and the
verify CLI can demonstrate against.

### Scenario: Seeded evaluations produce evidence records `[integration]`

- GIVEN `cmd/seed dev-data` is executed against a fresh database
- WHEN the seeded interactions are evaluated
- THEN each resulting `evaluations` row MUST have a corresponding
  `evidence_records` row with `seq` starting at 1 for that tenant.

### Scenario: Export endpoint demonstrates real evidence with dev data `[manual-demo]`

- GIVEN `cmd/seed dev-data` has run
- WHEN a developer calls `GET /v1/interactions/{id}/evidence` for a seeded,
  evaluated interaction
- THEN the response MUST return a real, non-fabricated evidence package
  that `VerifyPackage` accepts.

### Scenario: Verify CLI demonstrates an intact chain with dev data `[manual-demo]`

- GIVEN `cmd/seed dev-data` has run and produced evidence records for the
  demo tenant
- WHEN a developer runs the verify CLI verb against the demo tenant
- THEN it MUST report the chain as intact.

---

## Non-goals (hardened by this spec)

The following behaviors are explicitly out of scope and MUST NOT be
introduced as part of this change. Any pull request that introduces them
MUST be rejected.

- Merkle trees, RFC-3161 timestamps, external anchoring, or a scheduled
  automated verify job (issue #12). This spec covers on-demand `VerifyChain`
  and per-record `created_at` only.
- Cryptographic signing or asymmetric signatures. The export is
  hash-verifiable (tamper-evident by recomputation), not signed
  (non-repudiable). No signing-key infrastructure MUST be introduced.
- `HumanOverride` / HITL routing fields or behavior (issue #8). The evidence
  body MUST NOT carry a `human_override` field.
- `PolicyBundle` resolution semantics (issue #6). `policy_bundle_version`
  stays an inert, always-empty passthrough embedded for evidence fidelity
  only — no resolution logic.
- LLM-judge fields (`JudgeModelID`, `JudgePromptVersion` — issue #4). These
  MUST NOT appear in the evidence body.
- Backfilling evidence for evaluations created before this migration. No
  code path MUST fabricate an `EvidenceRecord` for a pre-existing
  evaluation.
- A verify HTTP endpoint. Verify MUST remain library + CLI only, not exposed
  over authenticated HTTP.

---

## Dependency alignment

This spec depends on the following prior work being stable and unmodified:

- **Issue #2**: `internal/evaluation/service.go` `EvaluateInteraction`,
  `EvaluationStore.CreateEvaluation`, the `evaluations` +
  `detector_result_rows` schema, and the existing
  `tenantdb.WithTenantTx` + `internal/postgres` adapter pattern.
- **Issue #13**: schema, RLS foundations, generated `internal/db` layer.
- **Issue #14**: tenant API key auth, `tenantdb.WithTenantTx`, and the
  `GET /v1/interactions` route pattern this export endpoint follows.

No requirement in this spec modifies those boundaries beyond the additive
evidence-append call inside `CreateEvaluation`'s existing transaction and
the new `GET /v1/interactions/{id}/evidence` route.
