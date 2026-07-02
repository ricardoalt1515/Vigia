# Design: Issue #3 Evidence Ledger (append-only, hash-chained EvidenceRecord + verify + export)

## Technical Approach

Add a deep `internal/ledger` module — a large amount of hashing / canonicalization /
verification behind a tiny, **pure** interface (`Hash`, `ComputeInputsDigest`,
`VerifyChain`, `BuildPackage`, `VerifyPackage`) with **zero I/O**. Persistence stays in
`internal/postgres`: `EvaluationStore.CreateEvaluation` appends exactly one
`evidence_records` row **inside its existing `tenantdb.WithTenantTx`**, serialized per
tenant through a locked `ledger_chain_heads` head row (Decision 2). A migration
`00004_evidence_ledger.sql` adds the two tables and a `BEFORE UPDATE OR DELETE` trigger
that makes records write-once regardless of role (Decision 3). A new tenant-scoped
`GET /v1/interactions/{id}/evidence` endpoint exports a self-contained evidence package
that `VerifyPackage` re-verifies with no DB access (Decision 5). A thin
`cmd/ledger-verify` operator binary (seed-style `run(ctx,args)` seam) runs a store-backed
`VerifyChain` for one tenant (Decision 6). No River job — the append inherits #2's
synchronous evaluation path.

The single load-bearing hazard is **determinism**: the same inputs must always yield the
same hash, or verification is meaningless. Every canonicalization choice below is pinned
and golden-tested.

## Architecture Decisions

| Decision | Choice | Rejected | Rationale |
|---|---|---|---|
| Append hook | One more INSERT inside `CreateEvaluation`'s existing `WithTenantTx`, after header + detector rows | Post-commit append; separate service | ADR-01 exactly-once; closes the window where an evaluation exists without evidence or vice versa. `UNIQUE(tenant_id, evaluation_id)` makes "exactly one" transactional. |
| Sequence / prev-hash source | Per-tenant `ledger_chain_heads(tenant_id PK, last_seq, last_hash)` locked in-tx | `MAX(seq)+1 FOR UPDATE` scan; Postgres `SEQUENCE` | O(1); natural per-tenant row lock serializes concurrent appends; rollback-safe (head update is in the same tx, an aborted evaluation leaves no gap); fits existing small-header-table conventions. |
| First-append race | `LockChainHead` = `INSERT ... ON CONFLICT (tenant_id) DO UPDATE SET last_seq = ledger_chain_heads.last_seq RETURNING last_seq, last_hash` | Plain `SELECT ... FOR UPDATE` (no row to lock on first append → two concurrent first-appends both compute seq=1) | The `ON CONFLICT` insert serializes on the PK: the first append inserts the genesis head `(0, genesis)`; concurrent first-appends conflict and take the row lock via the no-op self-update. Every path returns a locked head. |
| Write-once | App layer never issues UPDATE/DELETE **and** a `BEFORE UPDATE OR DELETE` trigger `RAISE EXCEPTION` | REVOKE/GRANT; RLS `USING(false)` | Single-owner local role bypasses GRANT/REVOKE and (without FORCE) RLS; the trigger is the only guard immune to role/ownership. Defense in depth for the trust core. |
| Canonicalization | Fixed Go struct marshaled with `encoding/json` (declaration-order fields); detector results **sorted by `code`**; `created_at` pinned to a fixed microsecond UTC format | `map[string]any` (random key order); RFC3339Nano (trailing-zero variance); DB `now()` default (round-trip precision drift) | Struct marshaling is deterministic; the real hazards are detector ordering and timestamp precision — both pinned + golden-tested. |
| Hash | `Hash = hex(sha256(prev_hash_ascii || canonical(body)))` | New crypto dep; binary encoding | Matches `auth.HashAPIKey` / `mcp` sha256+hex precedent; hex keeps the export human-diffable. |
| `created_at` authority | Ledger generates `time.Now().UTC().Truncate(time.Microsecond)` and inserts it **explicitly** (column has no default) | DB `DEFAULT now()` | Postgres timestamptz is microsecond; a nanosecond Go value hashed at insert but read back truncated would never re-verify. The ledger owns the exact hashed value. |
| Verify surface | Pure `VerifyChain([]EvidenceRecord)` (the AC routine) + store-backed adapter + `cmd/ledger-verify` binary | HTTP verify endpoint | Library is the deep testable core; CLI gives on-demand operator check before #12's daily job; whole-chain recompute does not belong next to per-interaction HTTP reads. |
| Export | Hash-verifiable self-contained JSON at `GET /v1/interactions/{id}/evidence`, plain tenant auth | Cryptographic signing; CLI export; audit-scoped role | AC asks only for tamper-detection by recomputation; signing needs key infra that does not exist (its own ADR). Reuses the proven httpapi/auth seam. |
| Parent deletion | FK `ON DELETE CASCADE` kept for schema consistency; the trigger supersedes it — deleting a parent with evidence fails loudly | `ON DELETE RESTRICT`; no FK | An immutable ledger *should* block destruction of the audit trail; a failed cascade is the correct, honest outcome. Flagged below. |

## Genesis

The first record of a tenant's chain uses a pinned genesis previous hash — the
empty string sentinel `""`:

```go
// GenesisPrevHash is the prev_hash of a tenant's first EvidenceRecord: the empty
// string "". Pinned — the golden-hash test depends on it.
const GenesisPrevHash = ""
```

The genesis head row inserted by `LockChainHead` is `(tenant_id, last_seq=0,
last_hash=GenesisPrevHash)`, so the first real append derives `seq = 1`,
`prev_hash = GenesisPrevHash`. Because the sentinel is empty, the genesis
record's hash reduces to `sha256("" || canonical(body)) = sha256(canonical(body))`
— no special-casing is needed; the standard hash formula already produces it.

## `internal/ledger` package (pure — no I/O)

```go
package ledger

const GenesisPrevHash = "" // empty-string genesis sentinel

// DetectorResult is one detector's contribution to inputs_digest.
type DetectorResult struct {
    Code      string
    Outcome   string // core.DetectorOutcome value, e.g. "pass" | "fail"
    Severity  string
    Rationale string
}

// Body is the hashed content of a record. FIELD ORDER IS LOAD-BEARING:
// encoding/json emits struct fields in declaration order, and that order is
// baked into every stored hash. Do not reorder, add, or remove fields without
// a migration + a new golden hash. created_at is serialized by a fixed
// microsecond UTC formatter (see canonicalBody), NOT time.Time's default.
type Body struct {
    TenantID            string    `json:"tenant_id"`
    InteractionEventID  string    `json:"interaction_event_id"`
    EvaluationID        string    `json:"evaluation_id"`
    Seq                 int64     `json:"seq"`
    OverallOutcome      string    `json:"overall_outcome"`
    PolicyBundleVersion string    `json:"policy_bundle_version"`
    InputsDigest        string    `json:"inputs_digest"`
    CreatedAt           time.Time `json:"created_at"`
}

// EvidenceRecord is a persisted, hashed ledger entry.
type EvidenceRecord struct {
    ID       string
    Body     Body
    PrevHash string
    Hash     string
}

// ComputeInputsDigest = hex(sha256(canonical(sorted results))). Results are
// sorted by Code first (the ordering invariant); each entry contributes
// code+outcome+severity+rationale.
func ComputeInputsDigest(results []DetectorResult) string

// Hash = hex(sha256(prevHash-ASCII bytes || canonicalBody(body))). Pure.
func Hash(prevHash string, body Body) string

// VerifyChain replays records that MUST already be ordered by Seq ascending.
// Empty and single-record chains are valid. Checks, in order per record:
//   record[0].PrevHash == GenesisPrevHash
//   record[i].Seq      == record[i-1].Seq + 1     (no gap / no fork)
//   record[i].PrevHash == record[i-1].Hash        (linkage)
//   Hash(record[i].PrevHash, record[i].Body) == record[i].Hash  (integrity)
func VerifyChain(records []EvidenceRecord) VerifyResult

// BuildPackage assembles the self-contained export DTO (pure).
func BuildPackage(rec EvidenceRecord, interaction PackageInteraction,
    eval PackageEvaluation, results []DetectorResult) Package

// VerifyPackage re-verifies an export with NO DB access: cross-validates the
// Evaluation/Interaction display blocks against the verified Record, then
// recomputes inputs_digest from the package's detector_results and the
// record hash from prev_hash || canonical(body), comparing both against the
// embedded values.
func VerifyPackage(pkg Package) VerifyResult

type VerifyResult struct {
    OK          bool
    Count       int
    BreakAtSeq  int64  // first broken seq; 0 when OK
    BreakReason string // "" | "genesis prev_hash" | "seq gap" | "prev_hash linkage" | "hash mismatch" | "inputs_digest mismatch"
}
```

Internal (unexported, package-private seam used only by its own tests):

```go
// canonicalBody marshals Body deterministically. created_at is rendered by a
// FIXED formatter so DB round-trips re-verify: always UTC, always 6-digit
// microseconds, always trailing 'Z'.
const canonicalTimeLayout = "2006-01-02T15:04:05.000000Z07:00"
func canonicalBody(b Body) []byte
```

Error taxonomy: `VerifyChain` / `VerifyPackage` return a typed `VerifyResult`
(no error) so callers get the first-break `Seq` + reason without string-parsing.
Argument/shape failures (nil package, malformed record) return a wrapped
`error`. No panics on tampered input.

**Depth check (deletion test):** delete `internal/ledger` and the hash-chain
determinism, ordering invariant, and self-verification logic reappear scattered
across the adapter, handler, and CLI. It earns its keep.

## Migration — `db/migrations/00004_evidence_ledger.sql`

```sql
-- +goose Up
-- +goose StatementBegin
CREATE TABLE evidence_records (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    interaction_event_id uuid NOT NULL,
    evaluation_id uuid NOT NULL,
    seq bigint NOT NULL,
    prev_hash text NOT NULL,
    hash text NOT NULL,
    overall_outcome text NOT NULL,
    policy_bundle_version text NOT NULL DEFAULT '',
    inputs_digest text NOT NULL,
    -- No DEFAULT: the ledger inserts the exact microsecond-truncated value it hashed.
    created_at timestamptz NOT NULL,
    UNIQUE (id, tenant_id),
    UNIQUE (tenant_id, seq),            -- monotonic per tenant; backstops the head lock
    UNIQUE (tenant_id, evaluation_id),  -- exactly one record per evaluation
    FOREIGN KEY (interaction_event_id, tenant_id)
        REFERENCES interaction_events(id, tenant_id) ON DELETE CASCADE,
    FOREIGN KEY (evaluation_id, tenant_id)
        REFERENCES evaluations(id, tenant_id) ON DELETE CASCADE
);

-- Composite FKs do not auto-index. get-by-interaction leads with
-- interaction_event_id; UNIQUE(tenant_id, seq) already covers list-by-seq and
-- UNIQUE(tenant_id, evaluation_id) covers evaluation lookup.
CREATE INDEX idx_evidence_records_interaction_event_id
    ON evidence_records (interaction_event_id);

ALTER TABLE evidence_records ENABLE ROW LEVEL SECURITY;
CREATE POLICY evidence_records_tenant_isolation ON evidence_records
    USING (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);

CREATE TABLE ledger_chain_heads (
    tenant_id uuid PRIMARY KEY REFERENCES tenants(id) ON DELETE CASCADE,
    last_seq bigint NOT NULL,
    last_hash text NOT NULL
);
ALTER TABLE ledger_chain_heads ENABLE ROW LEVEL SECURITY;
CREATE POLICY ledger_chain_heads_tenant_isolation ON ledger_chain_heads
    USING (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION evidence_records_block_mutation() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'evidence_records is append-only: % is not permitted', TG_OP
        USING ERRCODE = 'restrict_violation';
END;
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER evidence_records_no_update_delete
    BEFORE UPDATE OR DELETE ON evidence_records
    FOR EACH ROW EXECUTE FUNCTION evidence_records_block_mutation();
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TRIGGER IF EXISTS evidence_records_no_update_delete ON evidence_records;
DROP FUNCTION IF EXISTS evidence_records_block_mutation();
DROP TABLE IF EXISTS evidence_records;
DROP TABLE IF EXISTS ledger_chain_heads;
-- +goose StatementEnd
```

**`ledger_chain_heads` deliberately has NO write-once trigger** — it MUST be
updatable (the head advances on every append). This is safe because the head is
a **derivable cache**: it is fully recomputable from `evidence_records`
(`MAX(seq)` and that row's `hash`), and `VerifyChain` never reads it — verify
replays the records themselves ordered by `seq`. A corrupted or lost head can be
rebuilt and cannot forge a passing verify; the worst it can do is misassign the
next `seq`/`prev_hash`, which `UNIQUE(tenant_id, seq)` + linkage checks catch on
the next append or verify.

**Parent-deletion consequence (flagged):** the `ON DELETE CASCADE` FKs are kept
for house consistency, but the trigger fires on the cascade's DELETE and aborts
it. Net effect: **a tenant/interaction/evaluation with evidence cannot be
deleted** — the audit trail is permanent. This is intentional for a trust-core
ledger. No deletion flow exists in scope; spec should note it so a future
tenant-offboarding path plans an explicit, audited archival instead of a silent
cascade.

## Persistence hook — `EvaluationStore.CreateEvaluation`

Extend the existing `WithTenantTx` closure (adapters.go ~181-236). After the
header + detector rows, before returning:

```
1. header  := q.CreateEvaluation(...)                 // existing
2. for dr  := q.CreateDetectorResultRow(...)          // existing (loop)
3. head    := q.LockChainHead(tenant, GenesisPrevHash) // INSERT..ON CONFLICT..RETURNING (locks)
4. seq      = head.LastSeq + 1
   prevHash = head.LastHash
   createdAt = time.Now().UTC().Truncate(time.Microsecond)
   body     = ledger.Body{tenant, interaction, header.ID, seq, header.OverallOutcome,
                          header.PolicyBundleVersion,
                          ledger.ComputeInputsDigest(in.DetectorResults→[]ledger.DetectorResult),
                          createdAt}
   hash     = ledger.Hash(prevHash, body)
5. q.InsertEvidenceRecord(tenant, interaction, header.ID, seq, prevHash, hash,
                          body.OverallOutcome, body.PolicyBundleVersion,
                          body.InputsDigest, createdAt)
6. q.UpdateChainHead(tenant, seq, hash)
```

The detector results needed for `inputs_digest` are already in scope as
`in.DetectorResults` (`{DetectorCode, Outcome, Severity, Rationale}`) — a direct
map to `[]ledger.DetectorResult`. All of steps 3-6 are in the same tx as 1-2, so
a rollback anywhere leaves **no** evidence row and **no** head advance (gap-free
guarantee). `UNIQUE(tenant_id, seq)` backstops the head lock; `UNIQUE(tenant_id,
evaluation_id)` guarantees exactly one record per evaluation.

### sqlc queries (`db/queries/evidence_records.sql`, `ledger_chain_heads.sql`)

```sql
-- name: LockChainHead :one
-- Insert-or-lock: first append inserts the genesis head; later appends take the
-- row lock via the no-op self-update. Either way returns the locked head.
INSERT INTO ledger_chain_heads (tenant_id, last_seq, last_hash)
VALUES ($1, 0, $2)                       -- $2 = GenesisPrevHash
ON CONFLICT (tenant_id)
DO UPDATE SET last_seq = ledger_chain_heads.last_seq
RETURNING last_seq, last_hash;

-- name: UpdateChainHead :exec
UPDATE ledger_chain_heads SET last_seq = $2, last_hash = $3 WHERE tenant_id = $1;

-- name: InsertEvidenceRecord :one
INSERT INTO evidence_records (tenant_id, interaction_event_id, evaluation_id, seq,
    prev_hash, hash, overall_outcome, policy_bundle_version, inputs_digest, created_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
RETURNING id, tenant_id, interaction_event_id, evaluation_id, seq, prev_hash, hash,
    overall_outcome, policy_bundle_version, inputs_digest, created_at;

-- name: ListEvidenceRecordsByTenant :many   -- store-backed VerifyChain
SELECT id, tenant_id, interaction_event_id, evaluation_id, seq, prev_hash, hash,
    overall_outcome, policy_bundle_version, inputs_digest, created_at
FROM evidence_records WHERE tenant_id = $1 ORDER BY seq ASC;

-- name: GetEvidenceRecordByInteraction :one -- export endpoint
SELECT id, tenant_id, interaction_event_id, evaluation_id, seq, prev_hash, hash,
    overall_outcome, policy_bundle_version, inputs_digest, created_at
FROM evidence_records WHERE tenant_id = $1 AND interaction_event_id = $2;

-- name: ListDetectorResultRowsByEvaluation :many -- package detector layer
SELECT detector_code, outcome, severity, result_payload
FROM detector_result_rows WHERE evaluation_id = $1 ORDER BY detector_code ASC;
```

`No update/delete query for evidence_records exists` — the app-layer half of
Decision 3 is enforced by the absence of any mutating query.

## Export endpoint — `GET /v1/interactions/{id}/evidence`

Follows the existing `Server` / reader-interface / `mux.HandleFunc` pattern.

New reader port + wiring:

```go
// internal/httpapi
type EvidenceReader interface {
    GetEvidencePackage(ctx context.Context, tenantID, interactionID string) (ledger.Package, error)
}
// ErrEvidenceNotFound (sentinel) → 404. NewServer gains an EvidenceReader arg;
// route: s.mux.HandleFunc("GET /v1/interactions/{id}/evidence", s.handleGetEvidence)
```

Handler: `Authenticate(Authorization)` → tenant (same as existing routes);
`id := r.PathValue("id")`; `reader.GetEvidencePackage(ctx, tenant.TenantID, id)`;
on `ErrEvidenceNotFound` → 404, else 500, else JSON-encode the package.

Adapter (`internal/postgres`): `EvidenceReader` runs one `WithTenantTx`, loads
the record (`GetEvidenceRecordByInteraction`), the interaction, the evaluation,
and the detector rows (`ListDetectorResultRowsByEvaluation`, unmarshaling
`result_payload->>'rationale'`), then `ledger.BuildPackage(...)`. Any missing
piece → `ErrEvidenceNotFound`.

**Package DTO (`ledger.Package`, `schema_version: "vigia.evidence.v1"`):**

```jsonc
{
  "schema_version": "vigia.evidence.v1",
  "interaction": { "id", "tenant_id", "channel", "direction", "occurred_at" },
  "evaluation":  { "id", "overall_outcome", "policy_bundle_version", "created_at" },
  "detector_results": [ { "code", "outcome", "severity", "rationale" } ],  // sorted by code
  "record": {                                   // exactly the hashed Body + chain proof
    "tenant_id", "interaction_event_id", "evaluation_id", "seq",
    "overall_outcome", "policy_bundle_version", "inputs_digest",
    "created_at",                               // canonical microsecond-UTC string
    "prev_hash", "hash"
  }
}
```

`VerifyPackage` consumes exactly this DTO: (1) recompute `inputs_digest` from
`detector_results`, compare to `record.inputs_digest`; (2) rebuild `Body` from
`record.*` (parsing `created_at` with `canonicalTimeLayout`) and check
`Hash(record.prev_hash, body) == record.hash`. Both `record.*` fields fully
determine the hash; `detector_results` is the extra layer proving the digest.

**404 uniformity:** RLS scopes the read to the caller's tenant, so a
cross-tenant `id` returns no rows — **indistinguishable** from a nonexistent
interaction or one that predates the ledger (Decision 7: no backfill → no
record). All three collapse to a single generic 404, leaking nothing about
other tenants' data or which case occurred.

## Verify CLI — `cmd/ledger-verify`

New small binary following `cmd/seed`'s testable `run(ctx, args)` **style**
(Decision 6 says "seed-style"; a dedicated binary keeps operator verify separate
from dev-only seeding rather than overloading `cmd/seed`). No HTTP endpoint.

- Flags: `-tenant-id` (required, UUID).
- Behavior: open pool from `config.LoadFromEnv()`; store-backed
  `VerifyChain(ctx, tenantID)` loads `ListEvidenceRecordsByTenant` (ordered by
  `seq`) inside `WithTenantTx`, maps to `[]ledger.EvidenceRecord`, calls
  `ledger.VerifyChain`.
- Output: human line, e.g. `chain intact: tenant=<id> records=<n>` or
  `chain BROKEN: tenant=<id> first_break_seq=<seq> reason=<reason>`.
- Exit codes: `0` intact · `1` broken chain · `2` usage/operational error
  (missing flag, DB/connect failure). The exit-code split lets #12's daily job
  or CI shell out and branch on integrity vs. infrastructure failure.

## Seed / dev-data

No new seed logic. Evidence is produced **automatically** by the persistence
hook whenever `SeedDevData` evaluates a fixture (`evaluator.EvaluateInteraction`
→ `CreateEvaluation` → append). Both seed paths append correctly:

- **Fresh interactions** → evaluated immediately → one evidence record each.
- **Backfill path** (re-run, pre-existing interaction lacking an evaluation) →
  `GetEvaluationByInteractionEventID` returns no rows → `EvaluateInteraction`
  runs → appends evidence. The `(tenant_id, interaction_event_id)` uniqueness on
  `evaluations` plus the existence check keeps re-runs idempotent, so **no
  duplicate seq / no duplicate record** — a second seed run over already-seeded
  data creates neither a second evaluation nor a second evidence row.

Ordering caveat (non-blocking): seed evaluates fixtures in slice order, so the
`seq` assigned to each seeded interaction follows seed iteration order, not
`occurred_at`. That is fine — `seq` is an append-order ledger index, not a
business timestamp; `created_at` carries the wall-clock. Doc-only follow-up:
mention `cmd/ledger-verify -tenant-id <demo>` in dev docs so the ledger is
exercisable against seed data.

## File Changes

| File | Action |
|---|---|
| `db/migrations/00004_evidence_ledger.sql` | Create (tables, RLS, trigger, Down) |
| `db/queries/evidence_records.sql` | Create (Lock/Insert/List/Get + detector-by-eval) |
| `db/queries/ledger_chain_heads.sql` | Create (or fold into evidence_records.sql) |
| `internal/db/*` | Regenerate via sqlc (`make sqlc`) |
| `internal/ledger/{ledger.go, package.go, verify.go, *_test.go}` | Create (pure module) |
| `internal/postgres/adapters.go` | Modify: append inside `CreateEvaluation`; add `EvidenceReader` + store-backed `VerifyChain` adapters |
| `internal/httpapi/httpapi.go` | Modify: `EvidenceReader` port, `NewServer` arg, `/v1/interactions/{id}/evidence` route + handler, `ErrEvidenceNotFound`→404 |
| `cmd/api/main.go` | Modify: wire `EvidenceReader` into `NewServer` |
| `cmd/ledger-verify/main.go` (+ `main_test.go`) | Create (operator verify binary) |
| `docs/` (dev) | Modify: mention verify command |

`internal/core/types.go` needs no change — the record shape lives in
`internal/ledger`, re-derived from the real `evaluations` / `detector_result_rows`
schema (no `RequiresHITL`, no judge fields, `policy_bundle_version` inert).

## Testing Strategy (Strict TDD — `make test`)

| Layer | What | How |
|---|---|---|
| Unit — ledger hashing | Golden hash | Pin one `Body{tenant, interaction, eval fixed UUIDs, seq=1, "fail", "", inputs_digest, fixed created_at}` + `prev=GenesisPrevHash`; assert an **exact hardcoded hex** `Hash`. Any field/order/format drift fails loudly. Compute once, hardcode. |
| Unit — inputs_digest | Ordering invariant | Table-driven: same results in shuffled input order → identical digest (proves sort-by-code); changing any of code/outcome/severity/rationale changes it. |
| Unit — VerifyChain | intact / tampered / empty / single | Table-driven over `[]EvidenceRecord`: intact→OK; flip an `overall_outcome`→`hash mismatch` at that `seq`; drop a record→`seq gap`; rewrite `prev_hash`→`prev_hash linkage`; empty→OK count 0; single→OK; bad genesis→`genesis prev_hash`. |
| Unit — VerifyPackage | self-contained | Build a package, verify OK; tamper `record.hash`→fail; tamper a `detector_results.rationale` without updating `inputs_digest`→`inputs_digest mismatch`. |
| Integration — append atomicity | rollback leaves no gap | Real Postgres (`testing.Short()` skip, `DATABASE_URL`): force an error after the append (or inject a failing detector row) → assert no `evidence_records` row **and** `ledger_chain_heads` unchanged (gap-free). |
| Integration — trigger | write-once immune to owner | Using the **owner** conn (`DATABASE_URL`, bypasses RLS): `UPDATE evidence_records SET hash=...` and `DELETE` both raise the append-only exception. Proves role-independence. |
| Integration — concurrency | serialized appends | Two goroutines append for the **same tenant** concurrently; assert no duplicate `seq`, chain verifies, `last_seq == 2`. Second tenant unaffected (no cross-tenant lock). |
| Integration — RLS isolation | tenant scoping | `APP_DATABASE_URL` restricted role (per `internal/db/rls_isolation_test.go`): tenant A cannot read tenant B's `evidence_records` / `ledger_chain_heads`; store-backed `VerifyChain` under A sees only A's chain. |
| Integration — migration | RLS + tenant_id present | Extend the catalog check so `evidence_records` and `ledger_chain_heads` appear with non-null uuid `tenant_id` + RLS enabled (mirrors `migration_test.go`). |
| Handler — httpapi | shape / 404 / isolation | `httptest`: valid id → package JSON shape + `VerifyPackage` OK on the response body; unknown id, pre-ledger id, and cross-tenant id all → identical 404. |
| CLI — ledger-verify | run seam | `run(ctx, args, store)` with a fake store: intact→exit 0; broken→exit 1 + first-break line; missing `-tenant-id`→exit 2. |

## Assumptions — confirmed / challenged

- **Synchronous append (no River):** CONFIRMED. Inherits #2; the append is one
  more write in the existing `WithTenantTx`. No queue, no post-commit step.
- **Full `inputs_digest` (code+outcome+severity+rationale, sorted by code):**
  CONFIRMED. Chosen for evidence completeness; the sort is the explicit,
  test-covered ordering invariant.
- **`policy_bundle_version` inert:** CONFIRMED. Read from
  `evaluations.policy_bundle_version` (always `''` today), embedded for evidence
  fidelity and #6 future-proofing, no resolution logic.
- **Plain tenant auth on export:** CONFIRMED. Reuses `Authorization`-header
  tenant auth; no read-only audit scope this slice.
- **CHALLENGE — `created_at` authority:** the proposal implies the record's
  timestamp but does not pin who generates it. Design pins it to the **ledger
  (Go), microsecond-truncated, no DB default** — otherwise a `now()` default
  hashed at insert would never re-verify after a microsecond round-trip. Load-
  bearing; flagged for spec.
- **CHALLENGE — parent deletion:** kept `ON DELETE CASCADE` but the write-once
  trigger makes evidence (and thus its parents) undeletable. Intentional for a
  ledger; spec should state it so future offboarding plans explicit archival.

## Open Questions

- None blocking. Spec should record the two challenges (created_at authority,
  undeletable-parent consequence) as explicit behaviors.
```