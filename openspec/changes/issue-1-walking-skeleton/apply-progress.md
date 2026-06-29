# Apply Progress — Issue #1 Walking Skeleton

## Slice 1: Go seed dev-data subcommand (PR 1)

**Status:** COMPLETE
**Branch:** issue-1-seed-dev-data
**Completed:** 2026-06-29

### Tasks completed

- [x] T1.1 RED: `TestSeedDevData/fresh_run_creates_all_entities` written and failing
- [x] T1.2 GREEN: `cmd/seed/devdata.go` implemented (`SeedQuerier` port, `SeedDevData`, fixtures)
- [x] T1.3 RED: `TestSeedDevData/idempotent_rerun` written and failing
- [x] T1.4 GREEN: `GetTenantBySlug` / list-and-match guards added; idempotency confirmed
- [x] T1.5 TRIANGULATE: `TestSeedDevData/partial_state_missing_interactions` written and passing
- [x] T1.6 RED: `TestSeedDispatch` written (routing test via `routeArgs`) and failing
- [x] T1.7 GREEN: `routeArgs` + dispatch in `cmd/seed/main.go`; `defaultKeyIssuer` adapter wired
- [x] T1.8 REFACTOR: confirmed `devdata.go` imports only `internal/db`, `internal/core`, `pgx/v5`; no raw SQL; no `pgxpool` reference in `devdata.go`
- [x] T1.9 Integration test: `cmd/seed/devdata_integration_test.go` added; skips on `-short` or missing `DATABASE_URL`

### Files created / modified

| File | Action |
|------|--------|
| `cmd/seed/devdata.go` | Created — `SeedQuerier`, `KeyIssuer`, `DevDataParams`, `DevDataResult`, `DevDataCounts`, `SeedDevData`, `devDataFixtures`, `isNotFound`, `uuidToString` |
| `cmd/seed/devdata_test.go` | Created — table-driven unit tests: `TestSeedDevData/{fresh_run,idempotent_rerun,partial_state}`, `TestSeedDispatch/{dev-data,no_subcommand,empty_args}` |
| `cmd/seed/devdata_integration_test.go` | Created — `TestSeedDevDataIntegration` (skippable; mirrors `rls_isolation_test.go` skip pattern) |
| `cmd/seed/main.go` | Modified — added `defaultKeyIssuer`, `routeArgs`, `run` dispatch, `runDevData`, `runKeyIssuance` (backward compatible) |

### Verification output

```
=== RUN   TestSeedDevDataIntegration
    devdata_integration_test.go:28: DATABASE_URL is required for the seed integration test
--- SKIP: TestSeedDevDataIntegration (0.00s)
=== RUN   TestSeedDevData
=== RUN   TestSeedDevData/fresh_run_creates_all_entities
=== RUN   TestSeedDevData/idempotent_rerun
=== RUN   TestSeedDevData/partial_state_missing_interactions
--- PASS: TestSeedDevData (0.00s)
    --- PASS: TestSeedDevData/fresh_run_creates_all_entities (0.00s)
    --- PASS: TestSeedDevData/idempotent_rerun (0.00s)
    --- PASS: TestSeedDevData/partial_state_missing_interactions (0.00s)
=== RUN   TestSeedDispatch
=== RUN   TestSeedDispatch/dev-data_routes_to_seed
=== RUN   TestSeedDispatch/no_subcommand_routes_to_key_issuance
=== RUN   TestSeedDispatch/empty_args_routes_to_key_issuance
--- PASS: TestSeedDispatch (0.00s)
    --- PASS: TestSeedDispatch/dev-data_routes_to_seed (0.00s)
    --- PASS: TestSeedDispatch/no_subcommand_routes_to_key_issuance (0.00s)
    --- PASS: TestSeedDispatch/empty_args_routes_to_key_issuance (0.00s)
=== RUN   TestIssueTenantAPIKey
--- PASS: TestIssueTenantAPIKey (0.00s)
PASS
ok  github.com/ricardoalt1515/vigia/cmd/seed 0.336s

go build ./... → exit 0
go test ./... -short -count=1 → all packages PASS
```

### Design decisions applied

- **Owner-role RLS bypass**: seed inserts through `DATABASE_URL` (migration/owner role) with no `WithTenantTx`, matching the proven `rls_isolation_test.go` seeding pattern.
- **`SeedQuerier` minimal port**: 6-method interface (subset of `db.Querier`) so unit tests use an in-memory fake — no Docker required for the unit suite.
- **`KeyIssuer` interface**: wraps the existing `IssueTenantAPIKey` free function via `defaultKeyIssuer` adapter; key issuance runs on every call (plaintext not recoverable from hash).
- **Idempotency guards**: `GetTenantBySlug` → `isNotFound(pgx.ErrNoRows)` → create; debtor matched by `external_ref` in list; interactions matched by `transcript_ref` in list.
- **FK order enforced**: tenant → debtor → interaction_events → API key (asserted by call-order check in test).
- **Backward compatibility**: existing key-issuance path (`run` with `--tenant-id`) preserved verbatim in `runKeyIssuance`.

---

## Slice 2: River worker + goose migration (PR 2)

**Status:** NOT STARTED

## Slice 3: Next.js console + Makefile targets (PR 3)

**Status:** NOT STARTED
