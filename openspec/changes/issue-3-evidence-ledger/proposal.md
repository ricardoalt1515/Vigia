# Proposal: Issue #3 Evidence Ledger (append-only, hash-chained EvidenceRecord + verify + export)

## Problem / motivation

Vigía's regulatory promise is that every debtor interaction can be checked
against Mexican collection rules and that the verdict is *explainable,
reproducible, and tamper-evident*. Issue #2 delivered the evaluation spine
(evaluate → persist `Evaluation` header + `detector_result_rows` → surface
outcome). But those rows are ordinary, mutable database rows: nothing proves
that a stored verdict is the same verdict the system originally produced.
An owner, a bad migration, or a rogue code path could silently rewrite an
`overall_outcome` from `fail` to `pass`, and no artifact would betray the edit.

For a compliance product this is the load-bearing gap. The whole value
proposition — "we can PROVE what we decided and when" — collapses if the
evidence can be altered without trace. This is the **trust core** (ADR-02,
technical-design §5.3): an append-only, hash-chained ledger where each
`EvidenceRecord` embeds the previous record's hash, so any later edit to any
record breaks the chain and is detectable by recomputation. It also produces a
self-contained evidence package for a single interaction that a third party can
verify **from the export alone**, without trusting Vigía's database.

Without this, Vigía has verdicts but no proof; with it, the verdicts become
defensible evidence.

## Intent

Deliver the minimal, correct trust core on top of the #2 evaluation spine:

1. When an `Evaluation` is produced, append **exactly one** immutable
   `EvidenceRecord` that embeds the previous record's hash and a monotonic
   per-tenant sequence — persisted in the **same transaction** as the
   evaluation header + detector rows, so an evaluation never exists without its
   evidence and vice versa.
2. A `VerifyChain` routine recomputes the chain for a tenant and reports the
   first break (or confirms integrity), so a deliberate edit to any stored
   record is detected.
3. A single-interaction evidence package exports as self-contained JSON that
   independently re-verifies (recompute the record's hash from its own body +
   `prev_hash`) with **no database access**.
4. Records are write-once: the application layer never issues UPDATE/DELETE, and
   a database trigger enforces this regardless of role or ownership.

The change is "done" when the four acceptance criteria (below) pass under
table-driven and integration tests.

## Current behavior

- `internal/evaluation/service.go` `Service.EvaluateInteraction` runs detectors,
  computes `overall_outcome`, and calls `Store.CreateEvaluation`.
- `internal/postgres/adapters.go` `EvaluationStore.CreateEvaluation` opens **one**
  `tenantdb.WithTenantTx`, inserts the `evaluations` header + N
  `detector_result_rows` children, and commits — exactly the transaction the
  evidence append must join.
- `db/migrations/00003_contact_hours.sql` created `evaluations`
  (`id`, `tenant_id`, `interaction_event_id`, `overall_outcome`,
  `policy_bundle_version text default ''`, `created_at`;
  `UNIQUE (tenant_id, interaction_event_id)` — at most one evaluation per
  interaction). RLS is a single permissive `USING (tenant_id = …)` policy with
  no `FOR` clause, i.e. tenant isolation only, **not** append-only.
- `internal/core/types.go` `Evaluation` is `{ID, TenantID, InteractionEventID,
  OverallOutcome, PolicyBundleVersion, CreatedAt}` — simpler than the
  aspirational `technical-design.md` struct (no `RequiresHITL`, no judge fields).
- `internal/ledger/` exists but is an empty placeholder (`.gitkeep` only).
- Crypto precedent: `internal/auth/auth.go` and `internal/mcp/server.go` already
  use `crypto/sha256`. No hash-chaining or canonical serialization exists yet.
- `docker-compose.yml` runs Postgres as a single owner role (`vigia`); table
  owners bypass GRANT/REVOKE, so privilege-based write-once would silently fail.
- `internal/httpapi/httpapi.go` wires routes via Go 1.22 pattern mux
  (`GET /v1/interactions`, `GET /v1/summary`), auth via `Authorization` header →
  `auth.Authenticator.Authenticate` → tenant. This is the pattern a
  `GET /v1/interactions/{id}/evidence` export endpoint would follow.

## Desired behavior

- A new `evidence_records` table stores one immutable record per evaluation:
  identity (`id`, `tenant_id`, `interaction_event_id`, `evaluation_id`),
  chain fields (`seq bigint`, `prev_hash`, `hash`), the embedded verdict body
  (`overall_outcome`, `policy_bundle_version`, `inputs_digest`), and
  `created_at`. RLS keeps it tenant-isolated; a `BEFORE UPDATE OR DELETE` trigger
  makes it append-only.
- A per-tenant `ledger_chain_heads (tenant_id PK, last_seq bigint, last_hash text)`
  row is the serialization point: appending locks and updates this one row inside
  the transaction, giving O(1) `prev_hash`/`seq` without scanning the growing
  ledger.
- `internal/ledger` is a deep module exposing a small interface:
  `Hash(record)`  (canonical `sha256(prev_hash || canonical(body))`),
  `VerifyChain(records)` (or `VerifyChain(ctx, tenantID)` against the store), and
  `BuildPackage`/`VerifyPackage` for the single-interaction export. All hashing
  logic is hidden behind these functions.
- `EvaluationStore.CreateEvaluation` appends the `EvidenceRecord` as one more
  write inside its existing `WithTenantTx`, after the header + detector rows.
- `GET /v1/interactions/{id}/evidence` returns the evidence package JSON for one
  interaction, tenant-scoped by the same auth as existing routes.
- A `VerifyChain` library function plus a thin operator CLI verb recompute a
  tenant's chain and report the first break.

## Scope

### In scope

- Migration `00005_evidence_ledger.sql`:
  - `evidence_records` table (tenant-scoped, RLS tenant-isolation policy matching
    the existing `nullif(current_setting('app.tenant_id', true), '')::uuid`
    pattern), composite FKs to `interaction_events(id, tenant_id)` and
    `evaluations(id, tenant_id)`, `UNIQUE (tenant_id, seq)` and
    `UNIQUE (tenant_id, evaluation_id)` (one record per evaluation).
  - `ledger_chain_heads (tenant_id PK, last_seq bigint NOT NULL, last_hash text
    NOT NULL)` head-row table (RLS tenant-isolated).
  - A `BEFORE UPDATE OR DELETE` trigger on `evidence_records` that
    unconditionally `RAISE EXCEPTION` (write-once, defense in depth).
- `internal/ledger` package: `EvidenceRecord` type, canonical `Hash`,
  `VerifyChain`, and `BuildPackage` / `VerifyPackage` for the export, with
  table-driven tests (intact chain, tampered record, empty chain, single record,
  deterministic canonicalization) and an integration test against real Postgres.
- Extend `EvaluationStore.CreateEvaluation` to append the `EvidenceRecord` inside
  the existing `WithTenantTx` (lock/update `ledger_chain_heads`, compute
  `prev_hash`/`seq`, insert `evidence_records`).
- sqlc queries for `evidence_records` (insert, list-by-tenant-ordered-by-seq,
  get-by-interaction) and `ledger_chain_heads` (select-for-update, upsert).
- `GET /v1/interactions/{id}/evidence` export endpoint following the existing
  `Server`/reader-interface/`mux.HandleFunc` + `r.PathValue("id")` pattern.
- A thin operator verify surface: a `VerifyChain` library function (the AC's
  "verify routine") plus a small CLI verb to invoke it against a tenant.
- Seed/dev-data: ensure appending evidence for seeded evaluations works so the
  export endpoint and verify command return real data in dev.

### Out of scope

- Merkle trees, RFC-3161 timestamps, external anchoring, automated daily verify
  job (issue #12). Per-record `created_at` and on-demand verify are #3; batch
  anchoring and scheduled jobs are #12.
- Cryptographic signing / asymmetric signatures. No signing-key infrastructure
  exists; "signed JSON" in §5.3 is aspirational. #3 delivers **hash-verifiable**
  (tamper-evident by recomputation), **not** signed (non-repudiation). See
  Decision 5.
- Complaint workflow / HITL routing / `HumanOverride` (issue #8). The record
  omits `human_override`.
- `PolicyBundle` versioning semantics (issue #6). `policy_bundle_version` is
  embedded in the record body as an inert passthrough of the current
  (always `''`) column — carried for evidence fidelity, no resolution logic.
- LLM-judge fields (`JudgeModelID`, `JudgePromptVersion`) — issue #4 is not built;
  omitted from the record.
- Audio/transcript WORM object-lock (no audio ingestion exists).
- **Backfilling evidence for pre-existing evaluations.** See Decision 7: the
  chain starts at deployment; evaluations created before this migration get no
  `EvidenceRecord` and are outside every chain.

### Delivery

Single PR. `size:exception` is acceptable (user decision) — the migration,
ledger module, persistence hook, export endpoint, and verify surface form one
coherent trust-core slice; splitting them would ship a half-wired ledger that
can append but not verify (or verify nothing).

## Resolved decisions

### Decision 1 — Hook point: append inside the same `WithTenantTx` as the evaluation

**Decision (adopt exploration + ADR-01/§5.3).** The `EvidenceRecord` append
happens inside `EvaluationStore.CreateEvaluation`'s existing
`tenantdb.WithTenantTx`, after the `evaluations` header and `detector_result_rows`
children, as part of the same commit. Not a separate post-commit step.

**Rationale.** ADR-01 (exactly-once) and §5.3 require the evidence append to be
atomic with the workflow state transition. A separate step opens a window where
an evaluation exists without evidence (crash between commits) or evidence exists
for a rolled-back evaluation. One transaction closes both windows and makes
"exactly one record per evaluation" a transactional guarantee, backed by
`UNIQUE (tenant_id, evaluation_id)`. *(Flagged decision — resolved: adopt.)*

### Decision 2 — Per-tenant sequence via a `ledger_chain_heads` head-row table

**Decision (adopt exploration option b).** Serialize appends per tenant through a
`ledger_chain_heads (tenant_id PK, last_seq, last_hash)` row: `SELECT … FOR
UPDATE` (or upsert-returning) the tenant's head inside the transaction, derive
`seq = last_seq + 1` and `prev_hash = last_hash`, insert the record, then update
the head. Reject `MAX(seq)+1` scans (cost grows with the append-only table) and
Postgres `SEQUENCE`s (not rollback-safe — gaps on abort — and global, not
per-tenant).

**Rationale.** The head row gives O(1) `prev_hash`/`seq` lookup and a natural,
per-tenant row-level lock that serializes concurrent appends without locking the
whole ledger or blocking other tenants. It is rollback-safe (the head update is
part of the same transaction, so an aborted evaluation leaves no gap) and fits
the codebase's existing small-header-table conventions. *(Flagged decision —
resolved: adopt option b.)*

### Decision 3 — Write-once = application discipline **and** a DB trigger (defense in depth)

**Decision.** Enforce write-once at BOTH layers, resolving the apparent
issue-vs-exploration conflict as complementary, not contradictory:
- **Application layer** (satisfies the literal AC wording): the ledger code path
  only ever INSERTs `evidence_records`; there is no update/delete query, adapter
  method, or sqlc query that mutates a record.
- **Database layer** (defense in depth): a `BEFORE UPDATE OR DELETE` trigger on
  `evidence_records` unconditionally `RAISE EXCEPTION`.

Reject REVOKE/GRANT-based enforcement and RLS `USING (false)` policies as the
primary guard.

**Rationale.** The issue says "write-once at the application layer"; the
exploration warns that this is insufficient because the local single-owner
Postgres role bypasses GRANT/REVOKE and (without `FORCE ROW LEVEL SECURITY`)
RLS, so privilege-based approaches silently fail to protect against the very role
the app uses. There is no real conflict: app-layer discipline satisfies the AC,
and the trigger is the *only* mechanism that reliably blocks mutation regardless
of role or ownership — cheap, migration-level, testable, and independent of
future role-separation work. For the trust core, defense in depth is the right
default; a ledger that can be silently UPDATEd is not a ledger. *(Flagged
decision — resolved: both layers.)*

### Decision 4 — Canonical serialization: fixed-struct JSON with sorted detector inputs

**Decision.** Hash `Hash = sha256(prev_hash || canonical(body))`, hex-encoded
(matching the existing `crypto/sha256` precedent), where:
- `body` is a **fixed Go struct** marshaled with `encoding/json`. Struct
  marshaling emits fields in declaration order (not map-random), so output is
  deterministic as long as field order and set never change silently. A golden-
  hash test pins the exact bytes so any accidental field/order change fails loudly.
- The body carries the **applied versions that exist today**: the detector
  `code`s/results and the (inert) `policy_bundle_version`. There are no judge
  model/prompt versions yet (#4 not built), so those fields are omitted, not
  nulled.
- `inputs_digest = sha256(canonical(detector_results))`, where detector results
  are **sorted by `detector_code`** before serialization — an explicit,
  test-covered ordering invariant. Each entry contributes
  `code + outcome + severity + rationale`.

**Rationale.** Determinism is the whole point: the same inputs must always yield
the same hash, or verification is meaningless. Go's struct marshaling is already
deterministic; the real hazard is the detector-result ordering, which today
follows the `s.Detectors` slice (deterministic by construction but not
guaranteed). Pinning it with an explicit sort + invariant test removes a hidden
dependency before it is baked into hashes. Embedding `policy_bundle_version` even
while inert satisfies the AC's "applied versions" requirement and future-proofs
the body for #6 without adding resolution logic now. *(Flagged decision —
resolved.)*

### Decision 5 — Export is hash-verifiable, not signed; served at `GET /v1/interactions/{id}/evidence`

**Decision.** The evidence package is **hash-verifiable**: the exported JSON
contains everything needed to recompute the record's hash — the interaction, the
evaluation, the applied versions, the detector results, and the chain proof
(`seq`, `prev_hash`, `hash`) — so `VerifyPackage(json)` re-derives `hash` from
`prev_hash || canonical(body)` and compares, **with no database access**. It is
**not** cryptographically signed. The surface is a tenant-scoped HTTP endpoint
`GET /v1/interactions/{id}/evidence` following the existing httpapi pattern
(`r.PathValue("id")`, `Authorization`-header tenant auth), **not** a CLI, since
export is a per-interaction, per-tenant read that belongs with the other
interaction routes.

**Rationale.** The AC asks only for independent verification (tamper-detection by
recomputation), which hashing fully satisfies. True signing needs asymmetric
key-management infrastructure that does not exist and would be its own ADR/issue —
pulling it forward is scope creep for no AC benefit. "Independently verifiable
from the export alone" means the JSON is self-contained for hash recomputation;
that is exactly what the package provides. The HTTP endpoint reuses the proven
`Server`/auth/mux seam and keeps tenant isolation where it already lives.
*(Flagged decision — resolved: hash-verifiable, HTTP export.)*

### Decision 6 — Verify routine: library function (the AC surface) + thin operator CLI

**Decision.** Implement verify as an exported `internal/ledger` function
(`VerifyChain(records)` for the pure form, plus a store-backed
`VerifyChain(ctx, tenantID)` that loads a tenant's records ordered by `seq` and
recomputes), covered by table-driven + integration tests — this **is** the AC's
"verify routine". Add a **thin operator CLI verb** (seed-style subcommand) to run
it on demand against a tenant. Do **not** add a verify HTTP endpoint.

**Rationale.** The AC requires "a verify routine", satisfiable by a tested library
function — no CLI/API is strictly required, and there is no existing verify-verb
precedent to copy. The library function is the deep, testable core (callers and
tests cross the same interface). A thin CLI verb gives operators an on-demand
integrity check before #12 introduces the automated daily job, without exposing
whole-chain recomputation over authenticated HTTP (a heavier, tenant-wide scan
that does not belong next to per-interaction reads). *(Flagged decision —
resolved: library + CLI, no endpoint.)*

### Decision 7 — Chain starts at deployment; no backfill of historical evaluations

**Decision.** The chain begins with the first evaluation created **after** this
migration deploys. Pre-existing `evaluations` (from #2/dev seed) get **no**
`EvidenceRecord` and are outside every chain. No backfill.

**Rationale.** Backfilled records would be fabricated after the fact — hashing
data that was never chained at the time it was produced gives false assurance and
undermines the ledger's meaning ("this is what we decided *when* we decided it").
Starting clean at deployment keeps every record an honest, at-the-moment append.
Historical evaluations remain valid `Evaluation` rows; they simply predate the
trust core. *(Flagged decision — resolved: no backfill, explicit.)*

## Acceptance criteria and how they are satisfied

| Acceptance criterion | How #3 satisfies it |
|---|---|
| Each evaluation appends exactly one append-only `EvidenceRecord` with prev-hash linkage | Append runs inside `CreateEvaluation`'s `WithTenantTx` (Decision 1); `UNIQUE (tenant_id, evaluation_id)` guarantees exactly one; `prev_hash`/`seq` come from the locked `ledger_chain_heads` row (Decision 2). |
| Verify passes on intact chain, fails when any stored record is altered | `VerifyChain` recomputes `sha256(prev_hash || canonical(body))` per record in `seq` order (Decision 4); a golden-hash + tamper integration test proves an edited `overall_outcome`/`inputs_digest` breaks the chain (Decision 6). |
| Evidence package for one interaction exports and independently verifies | `GET /v1/interactions/{id}/evidence` returns self-contained JSON; `VerifyPackage(json)` re-derives the hash with no DB access (Decision 5). |
| Records are write-once at the application layer (no updates or deletes) | No update/delete query or adapter path exists for `evidence_records`; a `BEFORE UPDATE OR DELETE` trigger enforces it at the DB layer regardless of role (Decision 3). |

## Architecture / ADR alignment

- **ADR-02 (append-only + hash-chain, trust core):** delivers the append-only,
  hash-chained ledger; Merkle/RFC-3161/daily-verify explicitly deferred to #12.
- **ADR-01 (exactly-once) / §5.3:** evidence append is atomic with the evaluation
  transition — same `WithTenantTx`, one commit (Decision 1).
- **Clean / hexagonal + deep modules:** `internal/ledger` is a deep module — a
  large amount of hashing/verification behind a small interface (`Hash`,
  `VerifyChain`, `BuildPackage`/`VerifyPackage`); persistence stays in
  `internal/postgres` adapters behind the existing `EvaluationStore` port; the
  export endpoint reuses the httpapi seam.
- **Tenant isolation:** `evidence_records` and `ledger_chain_heads` carry RLS
  tenant-isolation policies matching the existing pattern; all reads/writes flow
  through `tenantdb.WithTenantTx`.
- **SQL-first persistence:** schema lands as goose migration
  `00005_evidence_ledger.sql`; sqlc regenerates access code. No ORM.
- **Schema fidelity:** the `EvidenceRecord` is re-derived from the **real**
  `evaluations`/`detector_result_rows` schema, not the aspirational
  `technical-design.md` struct (no judge/HITL/per-rule-score fields).

## Risks and mitigations

| Risk | Severity | Mitigation |
|---|---:|---|
| Non-deterministic canonicalization makes verify flaky / hashes unstable | High | Fixed-struct JSON + explicit detector-result sort by `detector_code` + golden-hash invariant test (Decision 4). |
| Write-once relies on GRANT/REVOKE and silently fails against the single owner role | High | DB `BEFORE UPDATE OR DELETE` trigger, immune to role/ownership; app layer never mutates (Decision 3). |
| Concurrent appends for one tenant race on `seq`/`prev_hash`, forking the chain | High | Per-tenant `ledger_chain_heads` row locked `FOR UPDATE` inside the tx serializes appends (Decision 2); `UNIQUE (tenant_id, seq)` backstops. |
| Append added outside the evaluation tx → evaluation without evidence or orphan record | High | Single `WithTenantTx` commit (Decision 1); `UNIQUE (tenant_id, evaluation_id)`. |
| Copying the aspirational technical-design struct bakes in fields that do not exist (`RequiresHITL`, judge fields) | Medium | Record re-derived from real schema; judge/HITL/policy-resolution fields omitted or inert. |
| Export mistaken for cryptographically signed / non-repudiable | Medium | Explicitly hash-verifiable only (Decision 5); documented; signing deferred to a later ADR/issue. |
| Backfilling historical evaluations would fabricate after-the-fact proofs | Medium | No backfill; chain starts at deployment (Decision 7), stated explicitly. |
| Verify over HTTP exposes heavy tenant-wide recomputation next to per-interaction reads | Low | Verify is library + CLI only; no endpoint (Decision 6). |
| Single-PR size exceeds 400-line budget | Medium | `size:exception` pre-approved; slice is coherent and splitting ships a half-wired ledger. |

## Rollback

- Roll back migration `00005_evidence_ledger.sql` via `make migrate-down` (drops
  the trigger, `evidence_records`, and `ledger_chain_heads`).
- Delete `internal/ledger` implementation and the append call added to
  `EvaluationStore.CreateEvaluation` (revert to the #2 header + detector-rows
  transaction).
- Remove the `GET /v1/interactions/{id}/evidence` route/handler and the verify
  CLI verb.
- Revert the sqlc-generated queries for the two new tables.
- Issues #1/#2 (seed, worker, console, evaluation spine) and #13/#14 auth/RLS
  remain untouched; existing `Evaluation` rows are unaffected.

## Proposal question round

No interactive question round was run; the orchestrator supplied the six open
decisions with an instruction to recommend-and-resolve. The proposal resolves all
six plus the backfill question (Decision 7). Assumptions the spec/design should
confirm:

- The evidence append extends the existing synchronous `CreateEvaluation` path
  (no River job); it inherits #2's synchronous evaluation model.
- `inputs_digest` covers the full detector result set
  (`code + outcome + severity + rationale`), sorted by `detector_code` — chosen
  for evidence completeness over a minimal identity set.
- `policy_bundle_version` is embedded but inert (always `''`) until #6.
- The export endpoint reuses plain `Authorization`-header tenant auth; no
  distinct read-only audit scope is introduced in this slice.

If any of these should change (e.g. add an audit-scoped role, or narrow
`inputs_digest`), raise it before spec.

## Next recommended phase

Spec and Design (can run in parallel).
