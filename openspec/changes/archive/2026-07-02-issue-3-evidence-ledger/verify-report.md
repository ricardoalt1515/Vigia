# Verify Report — issue-3-evidence-ledger

**Change**: issue-3-evidence-ledger
**Branch**: feat/issue-3-evidence-ledger (8 commits over main)
**Mode**: Full artifacts (proposal, spec, design, tasks, apply-progress all present)
**Verdict**: PASS

## 1. Test Execution Evidence

Ran against local Postgres (docker compose, migration 00005 already at `current version: 5`):

```
DATABASE_URL=postgres://vigia:vigia@localhost:5432/vigia?sslmode=disable go build ./...   → clean
DATABASE_URL=... go test ./... -v                                                          → ALL PACKAGES PASS
```

Package summary (zero FAIL, zero panics):
```
ok  cmd/harness-demo, cmd/ledger-verify, cmd/seed, cmd/vigia-mcp, cmd/worker
ok  internal/auth, internal/config, internal/db (1.08s), internal/detection
ok  internal/evaluation (0.47s), internal/harness(+bedrock,+caseflow,+labtools)
ok  internal/httpapi, internal/ledger, internal/mcp
ok  internal/postgres (1.50s), internal/tenantdb
```

Two RLS-role tests skip (documented, repo convention — no `APP_DATABASE_URL` configured):
- `internal/db/rls_isolation_test.go:22` `TestRLSIsolationForCurrentTenantInteractions`
- `internal/postgres/evidence_integration_test.go:497` `TestEvidenceRLSIsolationAcrossTenants`
- `internal/evaluation/evaluation_integration_test.go` `TestEvaluationRLSIsolationAcrossTenants`

All three skip with an explicit `t.Skip("... APP_DATABASE_URL ... required ...")` message — not silent, matches existing repo convention.

## 2. Spec Scenario → Test Mapping

| Requirement / Scenario | Tag | Test | Result |
|---|---|---|---|
| Successful evaluation produces exactly one evidence record | [integration] | `TestEvidenceAppendProducesExactlyOneRecord` | PASS |
| Evidence append shares the evaluation's transaction | [integration] | `TestEvidenceAppendSharesEvaluationTransaction` | PASS |
| Rollback leaves no evaluation/evidence/gap | [integration] | part of `evidence_integration_test.go` (rollback case) | PASS |
| First record uses genesis prev_hash="" | [integration] | `TestEvidenceChainLinkageAndSequence` (+ `TestGenesisPrevHashIsEmptyString` unit) | PASS |
| Subsequent records chain to previous hash | [integration] | `TestEvidenceChainLinkageAndSequence` | PASS |
| Sequence has no gaps | [integration] | `TestEvidenceChainLinkageAndSequence` | PASS |
| Concurrent appends, same tenant, never fork | [integration] | `TestEvidenceConcurrentAppendsSameTenantNeverFork` (real goroutines, asserts `last_seq==2`, no dup seq) | PASS |
| Concurrent appends, different tenants, independent | [integration] | `TestEvidenceConcurrentAppendsDifferentTenantsIndependent` | PASS |
| Golden-hash pins exact canonical bytes | [unit] | `TestHashGoldenValue` (pinned hex `4479342d...`) | PASS |
| Same inputs → same hash | [unit] | `TestHashIsDeterministic` | PASS |
| Detector results sorted by detector_code | [unit] | `TestComputeInputsDigestOrderingInvariant` | PASS |
| inputs_digest changes on field change | [unit] | `TestComputeInputsDigestChangesWithFieldChange` (code/outcome/severity/rationale) | PASS |
| VerifyChain passes on intact chain | [unit] | `TestVerifyChainIntact` | PASS |
| VerifyChain tampered overall_outcome/inputs_digest/prev_hash/seq (direct SQL) | [integration] | `TestVerifyChainDetectsTamperedFields` (4 subtests, `ALTER TABLE...DISABLE TRIGGER` to bypass write-once, then re-enable) | PASS |
| VerifyChain empty / single-record chains | [unit] | `TestVerifyChainEmpty`, `TestVerifyChainSingleRecord` | PASS |
| Application layer exposes no update/delete path | [unit] | `TestNoMutationQueriesAgainstEvidenceRecords` (grep-based over `db/queries` and generated `.sql.go`) | PASS |
| Direct SQL UPDATE/DELETE fails (trigger, owner conn) | [integration] | `TestEvidenceRecordsAreWriteOnceAgainstOwnerConnection` | PASS |
| Export self-contained + VerifyPackage (no DB) | [integration]+[unit] | `TestGetEvidence/evaluated_interaction_exports_and_independently_verifies` (httptest) + manual real-server confirmation below | PASS |
| Export tenant-isolated | [integration] | `TestGetEvidence/cross-tenant_interaction_id_returns_a_generic_404...` | PASS |
| Unevaluated interaction → defined not-found, no fabrication | [integration] | `TestGetEvidence/unevaluated_interaction_returns_a_generic_404...` | PASS |
| VerifyPackage detects tampered export | [unit] | `TestVerifyPackageDetectsTamperedHash`, `TestVerifyPackageDetectsTamperedDetectorResultWithoutDigestUpdate` | PASS |
| Pre-migration evaluation has no evidence record | [integration] | `TestPreMigrationEvaluationHasNoEvidenceRecord` | PASS |
| Export handles pre-ledger evaluation gracefully | [integration] | covered by `TestGetEvidence` unevaluated/pre-ledger case (same 404 path — `ErrEvidenceNotFound` collapses both) | PASS |
| Seeded evaluations produce evidence records, seq starts at 1 | [integration] | `TestSeedDevDataIntegration` | PASS |
| Export + verify CLI demonstrate real evidence with dev data | [manual-demo] | performed live below | PASS |

Every spec scenario, including all four concurrency and all four tamper-field cases, has a real passing covering test. No `UNTESTED`/`FAILING` scenarios.

## 3. Design Conformance

- Migration `00005_evidence_ledger.sql` read directly and matches design byte-for-byte: `UNIQUE(tenant_id, seq)`, `UNIQUE(tenant_id, evaluation_id)`, `UNIQUE(id, tenant_id)`, composite FKs to `interaction_events(id, tenant_id)` and `evaluations(id, tenant_id)` both `ON DELETE CASCADE`, RLS policies on both tables, `created_at timestamptz NOT NULL` with **no DEFAULT** (comment present), unconditional `BEFORE UPDATE OR DELETE` trigger raising `restrict_violation` regardless of role.
- `internal/postgres/adapters.go`: append hook calls `LockChainHead` → `InsertEvidenceRecord` → `UpdateChainHead` in that order inside the existing `WithTenantTx`, exactly per design's persistence-hook pseudocode; `createdAt := time.Now().UTC().Truncate(time.Microsecond)` confirmed generated in Go and fed into both the hash and the INSERT.
- Hash formula: `Hash(prevHash, body) = hex(sha256(prevHash-ASCII || canonicalBody(body)))`, `Body` is a fixed declaration-ordered struct marshaled via `encoding/json` (verified in `internal/ledger/ledger.go`) — no `map[string]any`.
- `internal/ledger` package is pure: no imports of `database/sql`, `pgx`, or `net/http` in `ledger.go`/`verify.go`/`package.go` (confirmed by reading the package; only `encoding/json`, `crypto/sha256`, `sort`, `time`, `fmt`).
- `cmd/ledger-verify/main.go` follows the `run(ctx, args, store, out) int` seam with sensible exit codes (0 intact / 1 broken / 2 usage-operational), confirmed by direct read.
- Export DTO (`ledger.Package`) carries `interaction`, `evaluation`, `detector_results`, and `record` (the full hashed `Body` + `prev_hash`/`hash`) — every hash input is present in the exported JSON, confirmed against a live response (§5).

## 4. Non-Goals Respected

Grepped the full diff (`git diff main..feat/issue-3-evidence-ledger`) for Merkle/RFC-3161/anchoring/signing/HITL/PolicyBundle-resolution/LLM-judge fields/backfill/verify-HTTP-endpoint: no matches in implementation code. No `HumanOverride`/`human_override`, no `JudgeModelID`/`JudgePromptVersion`, no signing-key infra, no scheduled job, no verify HTTP route — `cmd/ledger-verify` remains CLI-only, `internal/httpapi` adds only the export GET route.

## 5. Manual-Demo Scenarios (executed live, not just smoke-tested)

Ran the real path end to end against the migrated Postgres instance:

1. `go run ./cmd/seed dev-data` (with required `APP_ENV`/`OBJECT_STORE_*` env vars) → succeeded, printed `tenant_api_key=vigia_tenant_...`.
2. Queried demo tenant (`2747c71a-b1c4-4cff-b932-df8bfa064b7f`) — 4 interactions, evidence records with `seq` 1..4, `prev_hash`/`hash` chained.
3. Started `cmd/api` for real (not httptest), called `GET /v1/interactions/{id}/evidence` with the real tenant API key over HTTP:
   ```
   HTTP/1.1 200 OK
   {"schema_version":"vigia.evidence.v1", ..., "record":{"seq":1,"prev_hash":"","hash":"da01d24c..."}}
   ```
4. Fed the real HTTP response body (no DB access) into `ledger.VerifyPackage` via a throwaway `cmd/` program (created, run, then deleted — no source files modified/committed): result `{OK:true Count:1 BreakAtSeq:0 BreakReason:}`.
5. Ran `go run ./cmd/ledger-verify -tenant-id 2747c71a-b1c4-4cff-b932-df8bfa064b7f` → `chain intact: tenant=2747c71a-b1c4-4cff-b932-df8bfa064b7f records=4`, exit 0.

All three manual-demo scenarios from the spec (export demonstrates real evidence, verify CLI demonstrates intact chain, seed produces evidence) are confirmed against real seeded data through the real HTTP/CLI paths — this closes the gap flagged in apply-progress.

## 6. Noted Deviation — GetInteractionEventByID

`db/queries/interaction_events.sql` adds:
```sql
-- name: GetInteractionEventByID :one
SELECT id, tenant_id, debtor_id, channel, direction, status, occurred_at, transcript_ref, debtor_timezone, created_at
FROM interaction_events
WHERE id = $1 AND tenant_id = $2;
```
Confirmed tenant-scoped (`WHERE id = $1 AND tenant_id = $2`) and minimal — required by the `EvidenceReader` adapter to assemble the export package's `interaction` slice, not named explicitly in design.md but a small, justified, read-only addition consistent with existing tenant-scoping conventions.

## 7. TDD Evidence

Work-unit commits each carry tests with implementation:
- `cca3e74` (WU1: migration/sqlc/write-once guards) — includes `no_mutation_test.go`, extended `migration_test.go`
- `c7122c0` (WU2: pure ledger core) — includes `ledger_test.go`, `digest_test.go`, `verify_test.go`, `package_test.go`
- `c2725dd` + `4c552ca` (WU3: persistence hook) — includes `evidence_integration_test.go` (atomicity, chain linkage, concurrency, write-once, tamper detection, pre-migration, RLS)
- `5b345c9` (WU4: export endpoint) — includes extended `httpapi_test.go`
- `8d751fa` (WU5: verify CLI) — includes `main_test.go`
- `fce32bf` (WU6: seed demo + docs) — includes extended seed integration test + `HANDOFF.md`

Golden-hash literal (`TestHashGoldenValue`) was computed once and hardcoded with a "do not silently update" comment, per strict-TDD compute-once discipline.

## 8. Regression Check

- `internal/evaluation` (#2), `internal/db` migration/RLS catalog tests (#13), `internal/httpapi` existing interaction routes (#14) all still green — evaluations flow is unchanged apart from the additive evidence-append call inside the existing transaction.
- `go build ./...` clean; no new lint/vet issues surfaced.

## 9. Tasks Completion

All 46 checkboxes in `tasks.md` are marked `[x]`. Cross-checked against code: every task's described artifact exists and is exercised by the corresponding test (see §2 mapping). No unchecked or partially-implemented tasks found.

## Issues

**CRITICAL**: None.

**WARNING**: None.

**SUGGESTION**:
1. `cmd/ledger-verify` requires the full `config.LoadFromEnv()` (including `OBJECT_STORE_*` vars unrelated to ledger verification) even though it only uses `DatabaseURL`. A future cleanup could accept a narrower config surface (e.g. a `DATABASE_URL`-only loader) so the operator CLI doesn't need object-store credentials to run. Non-blocking — confirmed working once the full env is supplied, and this matches the existing `config.LoadFromEnv()` pattern used elsewhere in the repo.
2. `docs/`/`HANDOFF.md` now mentions `cmd/ledger-verify` and the export endpoint per task 6.2 — worth double-checking in a future PR that the object-store env dependency noted above is called out there too, so a new contributor running the manual demo isn't surprised by the `APP_ENV`/`OBJECT_STORE_*` requirement for a DB-only CLI.

## Final Verdict: PASS

All spec scenarios have passing, real covering tests (including concurrency and tamper-detection scenarios verified against real Postgres). Design decisions (migration DDL, head-lock pattern, hash formula, pure ledger package, CLI seam) match design.md. Non-goals are respected. All 46 tasks are complete and match code state. Manual-demo scenarios were executed live end-to-end (seed → real HTTP export → VerifyPackage with no DB → CLI verify), closing the gap flagged in apply-progress. No regressions in #1/#2/#14. Ready for `sdd-archive`.
