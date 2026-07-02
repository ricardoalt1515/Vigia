# Tasks: Issue #3 Evidence Ledger

Delivery: single-pr (user decision, size:exception pre-approved). No PR
chaining. Strict TDD: `make test` (`go test ./...`) must pass after every
task marked `[unit]`/`[integration]`. Tasks are grouped into work units per
`work-unit-commits`; each work unit keeps its tests (and docs, where
user-visible) in the same commit.

Spec scenario references quote the spec's own scenario titles (`spec.md`
§Requirement headers). `[unit]`/`[integration]`/`[manual-demo]` tags mirror
the spec's testing-mode annotations. Genesis sentinel is the empty string
`prev_hash = ""` throughout — do not substitute a placeholder hash.

---

## Work Unit 1 — Schema migration + sqlc regeneration

Satisfies: *Evidence Records Are Write-Once* (schema half), foundational
tables for all later units.

- [x] 1.1 Write `db/migrations/00004_evidence_ledger.sql` (Up + Down)
      exactly per design.md §Migration:
      - `evidence_records` table: `id`, `tenant_id` FK `tenants(id)
        ON DELETE CASCADE`, `interaction_event_id`, `evaluation_id`, `seq
        bigint`, `prev_hash text`, `hash text`, `overall_outcome text`,
        `policy_bundle_version text NOT NULL DEFAULT ''`, `inputs_digest
        text`, `created_at timestamptz NOT NULL` (**no DEFAULT** — Go
        inserts the exact hashed microsecond value)
      - `UNIQUE (id, tenant_id)`, `UNIQUE (tenant_id, seq)`,
        `UNIQUE (tenant_id, evaluation_id)`
      - composite FKs to `interaction_events(id, tenant_id)` and
        `evaluations(id, tenant_id)`, both `ON DELETE CASCADE`
      - `idx_evidence_records_interaction_event_id` index
      - RLS enabled + `evidence_records_tenant_isolation` policy mirroring
        existing tenant-isolation policies
      - `ledger_chain_heads` table (`tenant_id` PK FK `tenants(id)`,
        `last_seq bigint`, `last_hash text`), RLS enabled +
        `ledger_chain_heads_tenant_isolation` policy — **no write-once
        trigger on this table** (it is a derivable, updatable cache)
      - `evidence_records_block_mutation()` trigger function:
        `RAISE EXCEPTION` unconditionally on `BEFORE UPDATE OR DELETE`,
        `ERRCODE = 'restrict_violation'`
      - `evidence_records_no_update_delete` trigger wiring the function
      - Down: drop trigger, drop function, drop both tables
- [x] 1.2 Run `make migrate-up` against local Postgres; verify no errors
      and that existing issue #1/#2 seed/tests still pass against the
      migrated schema.
- [x] 1.3 Create `db/queries/evidence_records.sql`:
      `InsertEvidenceRecord :one`, `ListEvidenceRecordsByTenant :many`
      (ordered by `seq ASC`), `GetEvidenceRecordByInteraction :one`,
      `ListDetectorResultRowsByEvaluation :many` (ordered by
      `detector_code ASC`). **No UPDATE or DELETE query targeting
      `evidence_records` MUST exist in this file or anywhere else in the
      repo** — this is the app-layer half of write-once.
- [x] 1.4 Create `db/queries/ledger_chain_heads.sql`: `LockChainHead :one`
      (`INSERT ... ON CONFLICT (tenant_id) DO UPDATE SET last_seq =
      ledger_chain_heads.last_seq RETURNING last_seq, last_hash`),
      `UpdateChainHead :exec`.
- [x] 1.5 Run sqlc regeneration (`sqlc generate` / repo's generate target,
      e.g. `make sqlc`) to produce generated code for both new query
      files under `internal/db`. Verify `go build ./...` succeeds with the
      new generated types (`InsertEvidenceRecordParams`,
      `LockChainHeadRow`, etc.).
- [x] 1.6 [unit] Write a grep-based repo test (or extend an existing
      static-check test, e.g. `internal/db/no_mutation_test.go`) asserting
      no `UPDATE evidence_records` / `DELETE FROM evidence_records`
      substring exists anywhere under `db/queries/` or generated
      `internal/db/*.sql.go`. Satisfies *Application layer exposes no
      update or delete path* `[unit]`.
- [x] 1.7 Extend the migration/RLS catalog test (mirrors
      `internal/db/migration_test.go` pattern) to assert `evidence_records`
      and `ledger_chain_heads` both appear with a non-null `tenant_id`
      column and RLS enabled.

Verification: `make migrate-up` succeeds; `go build ./...` succeeds;
`go test ./internal/db/... -short` green (grep-based write-once test +
migration/RLS catalog test).

---

## Work Unit 2 — `internal/ledger` pure package (test-first, no I/O)

Satisfies: *Canonical Hashing Is Deterministic*, *VerifyChain Detects the
First Break* (`[unit]` scenarios), *Evidence Package Export* (`[unit]`
scenario: `VerifyPackage` detects tampering).

- [x] 2.1 [unit] Write `internal/ledger/ledger_test.go` before
      `ledger.go` exists: define `Body`, `EvidenceRecord`,
      `GenesisPrevHash = ""` fixtures. Write the **golden-hash test**
      first (it must fail to compile until 2.2/2.3 exist): construct one
      fixed `Body{TenantID, InteractionEventID, EvaluationID: hardcoded
      UUIDs, Seq: 1, OverallOutcome: "fail", PolicyBundleVersion: "",
      InputsDigest: <computed in 2.4>, CreatedAt: fixed UTC instant}` with
      `prev_hash = GenesisPrevHash`. Satisfies *Golden-hash test pins the
      exact canonical bytes* `[unit]`.
- [x] 2.2 Implement `internal/ledger/ledger.go`: `GenesisPrevHash`, `Body`,
      `EvidenceRecord`, `canonicalBody` (declaration-order `encoding/json`
      marshal, `created_at` rendered via the fixed
      `canonicalTimeLayout = "2006-01-02T15:04:05.000000Z07:00"`
      formatter), `Hash(prevHash string, body Body) string` =
      `hex(sha256(prevHash-ASCII || canonicalBody(body)))`.
- [x] 2.3 [unit] **Compute-once, hardcode consciously**: run the golden
      test once against the real implementation, copy the printed/asserted
      hex hash into the test as the pinned literal expected value (do not
      leave it computed at test-run time), then re-run the test suite to
      confirm it passes against the hardcoded literal. Add a one-line
      comment above the literal: `// pinned via 2.3 — any diff means
      canonicalization drifted, do not silently update`.
- [x] 2.4 [unit] Write table-driven cases in `ledger_test.go` (extend or
      new `internal/ledger/digest_test.go`) for `ComputeInputsDigest`
      before implementing it: same results in two shuffled input orders →
      identical digest; changing `code`/`outcome`/`severity`/`rationale`
      of one entry → digest changes. Satisfies *Same inputs always yield
      the same hash*, *Detector results are sorted by detector_code before
      digesting*, *inputs_digest changes when a detector result field
      changes* `[unit]`.
- [x] 2.5 Implement `ComputeInputsDigest(results []DetectorResult) string`
      in `internal/ledger/ledger.go`: sort by `Code`, canonical-marshal,
      `hex(sha256(...))`.
- [x] 2.6 [unit] Write `internal/ledger/verify_test.go` before
      `verify.go` exists: table-driven `VerifyChain` cases — intact chain
      → `OK`; empty chain → `OK`, `Count: 0`; single-record chain (`seq=1`,
      `prev_hash=""`) → `OK`; flipped `overall_outcome` → break at that
      `seq`, reason `hash mismatch`; dropped record (seq gap) → reason
      `seq gap`; rewritten `prev_hash` → reason `prev_hash linkage`; bad
      genesis `prev_hash` on record 0 → reason `genesis prev_hash`.
      Satisfies *VerifyChain passes on an intact chain*, *VerifyChain
      handles an empty chain*, *VerifyChain handles a single-record chain*
      `[unit]` (tampered-field scenarios are re-verified against real
      Postgres in Work Unit 3's integration tests).
- [x] 2.7 Implement `internal/ledger/verify.go`: `VerifyResult` struct
      (`OK`, `Count`, `BreakAtSeq`, `BreakReason`), `VerifyChain(records
      []EvidenceRecord) VerifyResult` per design.md's per-record check
      order (genesis → seq contiguity → prev_hash linkage → hash
      integrity).
- [x] 2.8 [unit] Write `internal/ledger/package_test.go` before
      `package.go` exists: build a `Package` via `BuildPackage`, assert
      `VerifyPackage` returns `OK`; tamper `record.hash` → not OK; tamper
      one `detector_results[].rationale` without updating `inputs_digest`
      → `BreakReason: "inputs_digest mismatch"`. Satisfies *VerifyPackage
      detects a tampered exported package* `[unit]`.
- [x] 2.9 Implement `internal/ledger/package.go`: `Package` DTO
      (`schema_version: "vigia.evidence.v1"`, `interaction`, `evaluation`,
      `detector_results`, `record`), `BuildPackage(rec EvidenceRecord,
      interaction PackageInteraction, eval PackageEvaluation, results
      []DetectorResult) Package`, `VerifyPackage(pkg Package)
      VerifyResult` (recomputes `inputs_digest` from `detector_results`,
      rebuilds `Body` from `record.*` parsing `created_at` with
      `canonicalTimeLayout`, checks `Hash(record.prev_hash, body) ==
      record.hash`).

Verification: `go test ./internal/ledger/... -v` green, zero external
dependencies (no `DATABASE_URL`, no network); `go vet ./internal/ledger/...`
clean.

---

## Work Unit 3 — Persistence hook + integration tests (test-first)

Satisfies: *Evidence Append Is Atomic and Exactly-Once*, *Chain Linkage and
Per-Tenant Monotonic Sequence*, *Evidence Records Are Write-Once*
(`[integration]` half), *No Backfill for Pre-Existing Evaluations*.

- [ ] 3.1 [integration] Write
      `internal/postgres/evidence_integration_test.go` (`testing.Short()`
      skip, requires `DATABASE_URL`) covering, before the hook exists:
      - a successful evaluation produces exactly one `evidence_records`
        row for that `evaluation_id`; a second insert attempt for the
        same `evaluation_id` fails on `UNIQUE (tenant_id, evaluation_id)`
        (*Successful evaluation produces exactly one evidence record*)
      - all three writes (`evaluations` header, `detector_result_rows`,
        `evidence_records`) occur inside one `tenantdb.WithTenantTx` call
        — verify via code review assertion plus a forced-failure case
        (*Evidence append shares the evaluation's transaction*)
      - forcing an error after the chain head lock but before commit
        (e.g. inject a failing detector-row insert) leaves no
        `evaluations`, no `detector_result_rows`, no `evidence_records`
        row, and `ledger_chain_heads.last_seq` unchanged; the next
        successful evaluation receives `seq = last_seq + 1` with no gap
        (*Rollback leaves no evaluation, no evidence, and no sequence
        gap*)
      - a tenant's first evaluation produces `seq = 1`, `prev_hash = ""`
        (*First record for a tenant uses the genesis prev_hash*)
      - a second evaluation for the same tenant produces `seq = N + 1`,
        `prev_hash` equal to the prior record's `hash` (*Subsequent
        records chain to the previous hash*)
      - K sequential evaluations for a tenant produce `seq` values
        exactly `1..K` with no gaps or duplicates (*Sequence has no gaps
        under normal operation*)
      - two goroutines appending concurrently for the **same** tenant
        produce distinct sequential `seq` values, no fork, `last_seq ==
        2` after both complete (*Concurrent appends for one tenant never
        fork the chain*)
      - two goroutines appending concurrently for **different** tenants
        do not block on each other and each tenant's chain is correct
        independently (*Concurrent appends across different tenants
        proceed independently*)
- [ ] 3.2 Implement the append hook inside
      `EvaluationStore.CreateEvaluation` in
      `internal/postgres/adapters.go`, extending the existing
      `tenantdb.WithTenantTx` closure per design.md §Persistence hook:
      after the header + detector-row writes, call `LockChainHead`,
      compute `seq = head.LastSeq + 1`, `prevHash = head.LastHash`,
      `createdAt = time.Now().UTC().Truncate(time.Microsecond)`, build
      `ledger.Body` from the evaluation + mapped
      `[]ledger.DetectorResult`, compute `hash = ledger.Hash(prevHash,
      body)`, call `InsertEvidenceRecord`, then `UpdateChainHead`. Add
      `EvidenceReader`/store-backed `VerifyChain` adapter (loads
      `ListEvidenceRecordsByTenant` inside `WithTenantTx`, maps rows to
      `[]ledger.EvidenceRecord`, calls `ledger.VerifyChain`) used by Work
      Unit 5's CLI.
- [ ] 3.3 [integration] Extend the same test file with the write-once
      trigger cases, using the DB **owner** connection (bypasses RLS,
      proving role-independence): direct `UPDATE evidence_records SET
      hash = ...` on an existing row fails with the append-only exception
      and the row's values remain unchanged (*Direct SQL UPDATE against
      evidence_records fails*); direct `DELETE FROM evidence_records
      WHERE id = ...` fails and the row still exists afterward (*Direct
      SQL DELETE against evidence_records fails*).
- [ ] 3.4 [integration] Extend the same test file with tampered-chain
      detection using the owner connection to bypass the write-once
      trigger's normal path is not possible (trigger blocks it
      unconditionally) — instead simulate tampering the way the spec
      describes for a pre-ledger chain audit: seed an intact 3+ record
      chain, use `ALTER TABLE ... DISABLE TRIGGER` scoped to the test
      transaction (or a direct catalog-level bypass documented in the
      test) to alter one record's `overall_outcome` / `inputs_digest` /
      `prev_hash` / `seq` directly, re-enable the trigger, then call the
      store-backed `VerifyChain` adapter and assert it reports a break at
      the correct `seq` for each of the four tampered-field cases
      (*VerifyChain detects a tampered overall_outcome / inputs_digest /
      prev_hash / seq*).
- [ ] 3.5 [integration] Add a case simulating a pre-migration evaluation:
      insert an `evaluations` row directly (bypassing the ledger append
      path) and assert no matching `evidence_records` row exists for its
      `evaluation_id` (*Pre-migration evaluation has no evidence record*).

Verification: `go test ./internal/postgres/... -run Evidence -v` green
against local Postgres; confirms clean skip under `go test ./... -short`.

---

## Work Unit 4 — Export endpoint (test-first)

Satisfies: *Evidence Package Export Is Self-Contained and Independently
Verifiable*, *No Backfill for Pre-Existing Evaluations* (export half).

- [ ] 4.1 [integration] Extend `internal/httpapi/httpapi_test.go`
      (`httptest`, before wiring) with cases: a seeded, evaluated
      interaction → `GET /v1/interactions/{id}/evidence` returns
      interaction + evaluation + detector results + applied versions +
      chain proof (`seq`, `prev_hash`, `hash`), and calling
      `ledger.VerifyPackage` on the decoded response body succeeds
      (*Evidence package exports and independently verifies*); tenant B
      calling with tenant A's interaction id returns a generic 404 and
      leaks no data (*Export is tenant-isolated*); an unevaluated
      interaction returns a generic 404/empty shape with no fabricated
      outcome/hash/chain fields (*Interaction without evaluation returns a
      defined empty shape*); a pre-ledger evaluation (evaluation row
      exists, no evidence row) also returns the same generic
      404/empty shape without fabricating chain-proof fields (*Export
      handles a pre-ledger evaluation without evidence*).
- [ ] 4.2 Implement `EvidenceReader` port + `ErrEvidenceNotFound` sentinel
      in `internal/httpapi/httpapi.go`; add `NewServer` constructor arg;
      wire `GET /v1/interactions/{id}/evidence` route; handler:
      authenticate via existing `Authorization`-header tenant auth, call
      `reader.GetEvidencePackage(ctx, tenantID, id)`, map
      `ErrEvidenceNotFound` → 404, other errors → 500, success → JSON.
- [ ] 4.3 Implement the `EvidenceReader` adapter in
      `internal/postgres/adapters.go`: one `WithTenantTx` loading
      `GetEvidenceRecordByInteraction`, the interaction, the evaluation,
      and `ListDetectorResultRowsByEvaluation`, then
      `ledger.BuildPackage(...)`; any missing piece (no evaluation, no
      evidence record) returns `ErrEvidenceNotFound` — collapsing
      cross-tenant, nonexistent, unevaluated, and pre-ledger cases into
      one indistinguishable 404.
- [ ] 4.4 Wire the new `EvidenceReader` into `NewServer` in
      `cmd/api/main.go`.

Verification: `go test ./internal/httpapi/... -v` green.

---

## Work Unit 5 — Verify CLI (test-first)

Satisfies: *VerifyChain Detects the First Break* (operator surface),
*Seed Produces Evidence Records for Demo Data* (`[manual-demo]` verify
scenario).

- [ ] 5.1 [unit] Write `cmd/ledger-verify/main_test.go` before
      `main.go` exists, following `cmd/seed`'s testable `run(ctx, args,
      store)` seam style with a fake store implementing the
      store-backed-`VerifyChain` interface: intact chain → exit code 0 and
      an "intact" line; broken chain → exit code 1 and a line naming the
      first-break `seq` + reason; missing `-tenant-id` flag → exit code 2.
- [ ] 5.2 Implement `cmd/ledger-verify/main.go`: `-tenant-id` flag
      (required, UUID), `run(ctx, args, store) int` seam, wiring
      `config.LoadFromEnv()` + a real Postgres pool in `main()`, using the
      Work Unit 3 store-backed `VerifyChain` adapter. Output format and
      exit codes exactly per design.md §Verify CLI (`0` intact, `1`
      broken, `2` usage/operational error).

Verification: `go test ./cmd/ledger-verify/... -v` green (fake-store unit
tests only, no `DATABASE_URL` required for 5.1).

---

## Work Unit 6 — Seed demonstrates real evidence + docs

Satisfies: *Seed Produces Evidence Records for Demo Data* (all scenarios).

- [ ] 6.1 [integration] Extend the seed integration test
      (`cmd/seed/devdata_test.go` or equivalent, `testing.Short()` skip)
      asserting: after `cmd/seed dev-data` runs against a fresh database,
      each resulting `evaluations` row has a corresponding
      `evidence_records` row, with `seq` starting at `1` for that tenant
      (*Seeded evaluations produce evidence records*). No new seed logic
      is required beyond what Work Unit 3's hook already provides — this
      task only asserts the existing seed path (`EvaluateInteraction` →
      `CreateEvaluation` → append) produces evidence automatically.
- [ ] 6.2 Update dev docs (`docs/` — whichever file documents
      `cmd/seed dev-data` / local dev workflow) to mention
      `cmd/ledger-verify -tenant-id <demo-tenant-id>` so the ledger is
      exercisable against seed data, and to mention
      `GET /v1/interactions/{id}/evidence` as the export endpoint.
      Satisfies *Export endpoint demonstrates real evidence with dev data*
      and *Verify CLI demonstrates an intact chain with dev data*
      `[manual-demo]` — validated by a developer running seed + export +
      CLI locally, not by an automated test.

Verification: `go test ./cmd/seed/... -v` green; manual demo: run
`cmd/seed dev-data`, call the export endpoint for a seeded interaction,
confirm `VerifyPackage` accepts it, then run
`cmd/ledger-verify -tenant-id <demo>` and confirm it reports the chain
intact.

---

## Sequencing summary

1. Work Unit 1 (migration + sqlc) — no dependencies, must land first.
2. Work Unit 2 (pure `internal/ledger`) — no DB dependency; can be
   developed in parallel with Work Unit 1, but its golden-hash literal
   (2.3) should be pinned before Work Unit 3 depends on `ledger.Hash`
   producing stable output.
3. Work Unit 3 (persistence hook + integration tests) — depends on
   Work Unit 1 (generated sqlc code, migrated schema) and Work Unit 2
   (`ledger.Hash`, `ledger.Body`, `ledger.ComputeInputsDigest`,
   `ledger.VerifyChain`).
4. Work Unit 4 (export endpoint) — depends on Work Unit 3 (needs
   `EvidenceReader`'s data source: `GetEvidenceRecordByInteraction`,
   `ListDetectorResultRowsByEvaluation`) and Work Unit 2
   (`ledger.BuildPackage`, `ledger.VerifyPackage`).
5. Work Unit 5 (verify CLI) — depends on Work Unit 3's store-backed
   `VerifyChain` adapter; its unit tests (5.1) can be written against a
   fake store in parallel with Work Unit 3, but 5.2's real wiring needs
   Work Unit 3 merged.
6. Work Unit 6 (seed demo + docs) — depends on Work Unit 3 (evidence
   append must exist for seeded evaluations to produce records) and
   Work Unit 5 (docs reference the verify CLI); lands last since it
   exercises the full spine end to end.

Parallelizable: Work Unit 2 (pure, zero I/O) can be developed in parallel
with Work Unit 1 by a second contributor if this were split across
people; for a single-PR single-author delivery, sequence 1 → 2 → 3 → 4 →
5 → 6 to keep the failing-test-then-implementation rhythm clean per
commit.

---

## Review Workload Forecast

- **Estimated changed lines (rough)**: ~900–1100 lines total across:
  - migration SQL + two query files (~110 lines)
  - sqlc-regenerated `internal/db` code (~150–200 lines, mostly generated
    boilerplate)
  - `internal/ledger` (ledger.go, verify.go, package.go + three test
    files) (~350–420 lines — the deep, heavily-tested pure core)
  - `internal/postgres` adapter additions + integration tests (~220 lines)
  - `internal/httpapi` DTO/route additions + tests (~130 lines)
  - `cmd/ledger-verify` (main.go + main_test.go) (~90 lines)
  - `cmd/seed` test extension + docs (~40 lines)
- **Chained PRs recommended**: No — user already selected `single-pr`
  with pre-approved `size:exception`. This section is informational only
  per the Review Workload Guard; no action is required from `sdd-apply`.
- **400-line budget risk**: High (total estimate exceeds 400 lines), but
  already covered by the pre-approved `size:exception`. Work-unit commits
  (per `work-unit-commits` skill) should still be used internally so the
  single PR reads as a clear story — the `internal/ledger` unit (Work
  Unit 2) is the natural place to slow down in review since it is the
  trust-core hashing logic.
- **Decision needed before apply**: No. The single-pr + size:exception
  decision was already made by the user; `sdd-apply` should proceed
  directly using the work-unit commit sequence above.
