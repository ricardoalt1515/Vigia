# Tasks: Issue #7 Remaining Deterministic Detectors + Dashboards

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~900-1400 total across all slices |
| 400-line budget risk | High |
| Chained PRs recommended | Yes |
| Suggested split | PR1 → PR2a → PR2b → PR2c → PR3 → PR4 |
| Delivery strategy | ask-on-risk |
| Chain strategy | stacked-to-main |

Decision needed before apply: Yes
Chained PRs recommended: Yes
Chain strategy: stacked-to-main
400-line budget risk: High

### Suggested Work Units

| Unit | Goal | Likely PR | Notes |
|------|------|-----------|-------|
| 1 | Despacho registry + detector-input schema | PR 1 | Base: main. Migration 00008 (initial part) + types + sqlc. |
| 2 | Third-party (06) + Protected population (07) detectors, service.go fold, HITL | PR 2a | Base: PR1 branch. Introduces `warn` outcome + 3-way fold. |
| 3 | Authorized channel (11) + Payment routing (10) detectors | PR 2b | Base: PR2a branch. |
| 4 | Disclosure (03, warn) + rename + seeding (7 rules) | PR 2c | Base: PR2b branch. |
| 5 | API aggregate endpoints (by-despacho, by-cause) | PR 3 | Base: PR2c branch (or main once merged). |
| 6 | Console dashboards | PR 4 | Base: PR3 branch (or main once merged). |

## Phase 1 (PR1): Despacho Registry + Detector-Input Schema

- [x] 1.1 Write migration `db/migrations/00008_deterministic_detectors.sql`: `despachos` table (id, tenant_id, external_ref, display_name), `UNIQUE(id,tenant_id)`, `UNIQUE(tenant_id,external_ref)`, RLS + grants (despacho-registry spec: RLS/cardinality scenarios).
- [x] 1.2 Add nullable `interaction_events.despacho_id` composite FK + index; nullable detector-input columns (`contact_party_relationship`, `contacted_party_dob` snapshot col, `authorized_channels text[]`, `payment_recipient`, `disclosure_provided`); `debtors.date_of_birth`.
- [x] 1.3 Add `Despacho` type to `internal/core/types.go` (ID, TenantID, ExternalRef, DisplayName) + new interaction/debtor fields.
- [x] 1.4 Add `db/queries/despachos.sql` CRUD + `make sqlc` regen.
- [x] 1.5 [integration] Test: despacho RLS isolation, 1:N cardinality, nullable-FK backward-compat, tenant-consistency FK rejection (despacho-registry scenarios).
- [x] 1.6 [unit] Test: `Despacho` round-trips via sqlc create/read.

## Phase 2a (PR2a): Third-Party + Protected Population, Outcome/HITL Plumbing

- [x] 2a.1 Add `Channel`, `ContactPartyRelationship`, `ContactedPartyDOB`, `AuthorizedChannels`, `PaymentRecipient`, `DisclosureProvided` to `detection.Interaction`; add `OutcomeWarn` to `Outcome` enum (`internal/detection/detector.go`).
- [x] 2a.2 [RED] Write table-driven tests + `TestXNoIO` for third-party detector (MX-REDECO-06): debtor/authorized/unknown/missing scenarios.
- [x] 2a.3 [GREEN] Implement `internal/detection/third_party.go`.
- [x] 2a.4 [RED] Write table-driven tests + `TestXNoIO` for protected-population detector (MX-REDECO-07): minor-always-block, elderly-debtor-pass, elderly-non-debtor-block, adult-pass, debtor-missing-DOB-pass, non-debtor-missing-DOB-block, OccurredAt-relative age.
- [x] 2a.5 [GREEN] Implement `internal/detection/protected_population.go` with `legalMajorityAge=18`, `elderlyAge=60` constants.
- [x] 2a.6 Add `core.DetectorOutcomeWarn` (`internal/core/types.go`).
- [x] 2a.7 Restructure `internal/evaluation/service.go` fold into 3-way branch (block/warn/pass); add `RequiresHITL bool` to `NamedDetector`, OR into `requiresHITL` on MX-REDECO-07 block only.
- [x] 2a.8 Wire both detectors in `cmd/api/main.go` + `cmd/seed/main.go` with `RequiresHITL: true` on MX-REDECO-07 only.
- [x] 2a.9 [integration] Test: MX-REDECO-07 block sets `requires_hitl=true`; other blocks leave it `false`. (Implemented as pure Service-level unit tests with a fake EvaluationStore — no real Postgres needed; see `internal/evaluation/service_test.go`.)

## Phase 2b (PR2b): Authorized Channel + Payment Routing

- [x] 2b.1 [RED] Table-driven tests + `TestXNoIO` for authorized-channel detector (MX-REDECO-11): listed/unlisted/missing-list scenarios.
- [x] 2b.2 [GREEN] Implement `internal/detection/authorized_channel.go`.
- [x] 2b.3 [RED] Table-driven tests + `TestXNoIO` for payment-routing detector (MX-REDECO-10): creditor/non-creditor/missing scenarios.
- [x] 2b.4 [GREEN] Implement `internal/detection/payment_routing.go`.
- [x] 2b.5 Wire both in `cmd/api/main.go` + `cmd/seed/main.go`, `RequiresHITL: false`.
- [x] 2b.6 (judgment-day fast-follow, discovered in PR2b review — CRITICAL, empirically reproduced) Plumb the detector-input snapshot columns end to end: neither `CreateInteractionEvent`/`GetInteractionEventByID` (sqlc) nor `cmd/seed/devdata.go`'s fixtures nor `internal/postgres/adapters.go`'s `GetInteractionForReEvaluation` populated `detection.Interaction`'s new fields (`Channel`, `ContactPartyRelationship`, `ContactedPartyDOB`, `AuthorizedChannels`, `PaymentRecipient`, `DisclosureProvided`) from persisted data — root cause predates PR2b (introduced in PR2a's wiring) but every interaction evaluated via `cmd/seed dev-data` or `ReEvaluateInteraction` saw zero-valued detector input and MX-REDECO-06/07/10/11 always fail-closed to BLOCK with `requires_hitl=true`. Fixed: extended `db/queries/{debtors,interaction_events}.sql` (+ `make sqlc` regen, verified idempotent) so `CreateDebtor`/`CreateInteractionEvent`/`GetInteractionEventByID` read/write the 00008 snapshot columns; `cmd/seed/devdata.go` now snapshots the debtor's DOB at creation and gives every fixture explicit compliant defaults (relationship=debtor, its own channel authorized, recipient=creditor, adult DOB) via a new `compliantFixture` helper, plus one demo-violation fixture per new detector (third-party, protected-minor, unauthorized-channel, payment-routing) that overrides only the field that detector reads, isolating each BLOCK; `internal/postgres/adapters.go`'s `GetInteractionForReEvaluation` now maps `Channel` (previously missing entirely) and all detector-input columns onto `detection.Interaction`. Extended `cmd/seed/devdata_integration_test.go` with real-Postgres assertions: compliant fixture passes all 4 new detectors, each violation fixture blocks only its own detector, MX-REDECO-07 sets `requires_hitl`, and `GetInteractionForReEvaluation`'s returned snapshot re-evaluates to the same `pass` outcomes (reproducibility). Verified via `go run ./cmd/seed dev-data` against a migrated dev DB: per-detector-row inspection confirmed the exact designed pass/fail mix, not universal fail.

## Phase 2c (PR2c): Disclosure (warn) + Rename + Seeding

- [x] 2c.1 [RED] Table-driven tests + `TestXNoIO` for disclosure detector (MX-REDECO-03): stated-pass, not-stated-warn, missing-warn (fail-closed to warn, not block). `internal/detection/disclosure_test.go`.
- [x] 2c.2 [GREEN] Implement `internal/detection/disclosure.go`.
- [x] 2c.3 Rename `"contact-hours"` → `"MX-REDECO-04"` in `cmd/api/main.go`, `cmd/seed/main.go`, and `cmd/seed/devdata_integration_test.go` (all three sites wired the real `ContactHoursDetector`; no other `"contact-hours"` fixture-label strings existed in those three files). Also wired `MX-REDECO-03`/`DisclosureDetector` alongside the rename in all three files' `Detectors` slices.
- [x] 2c.4 Add `detector_code` backfill (forward + reversible `Down`) in migration 00008. (Completed in PR1.)
- [x] 2c.5 Add `UpsertPolicyRule` (`ON CONFLICT (code) DO UPDATE`) to `db/queries/policies.sql` + sqlc regen (verified idempotent — 2nd `make sqlc` run produced identical diff).
- [x] 2c.6 Implement `cmd/seed` seeding: `cmd/seed/policy_seed.go`'s `SeedPolicyCatalogAndBundle` upserts all 7 rules (`policyRuleCatalog()`, titles/descriptions/legal-basis sourced from `docs/regulatory-ruleset.md`) via `UpsertPolicyRule`, MX-REDECO-03 severity `medium`, the other six `high`; creates one active `redeco-baseline` bundle via `postgres.PolicyBundleStore.CreateBundleVersion` ONLY when `GetActiveBundleByTenant` finds none (idempotent guard — catalog upsert always runs, bundle-snapshot creation is guarded). Wired into `cmd/seed/main.go`'s `runDevData` after `SeedDevData` succeeds.
- [x] 2c.7 [integration] Test: `cmd/seed/policy_seed_integration_test.go` — fresh (uniquely-slugged) tenant per run proves `created=true` on first call / `created=false` on second (idempotency, no re-seed stacking of bundle versions), asserts exactly 7 `policy_rules` rows with correct severities (MX-REDECO-03 medium, others high) and non-empty title/description, and the active bundle's `policy_bundle_rules` snapshot has exactly 7 rows with non-null `LegalBasis`/`EffectiveDate`. Plus `cmd/seed/policy_seed_test.go`'s pure unit tests (fake upserter/checker/bundle-creator) for the same idempotency branch logic.
- [x] 2c.8 [integration] Test: warn-only interaction stays overall `pass`; warn+block coexisting yields overall `fail`. Satisfied two ways: (1) PR2a's `internal/evaluation/service_test.go` — "warn outcome maps to warn severity medium and does not flip overall outcome" + "warn row coexisting with a hard-block row yields overall fail" — pure Service-level tests with a fake `EvaluationStore`, mirroring the 2a.9 HITL precedent; (2) `cmd/seed/devdata_integration_test.go`'s disclosure-warn demo fixture (`seed/demo/call-09-disclosure-warn`) now proves the SAME scenario end to end against real Postgres, including the overall_outcome=`pass` assertion — see the judgment-day round-1 fast-follow note below (fixed-date fixture timestamps made this assertion safe).
  - **Judgment-day round 1 fast-follow (found via empirical repro: seed run at ~21:58 America/Mexico_City showed even COMPLIANT fixtures at overall_outcome='fail')**: root cause was that every `cmd/seed/devdata.go` fixture's `OccurredAt` was anchored to `time.Now()` offsets (`now.Add(-N)`), so seeded outcomes were non-deterministic — any fixture's evaluation could flip depending on what real wall-clock time the debtor-local instant landed on relative to the `[08:00,21:00)` contact-hours window. The original apply run had (incorrectly) "fixed" this by removing the overall_outcome=pass integration assertion instead of fixing the root cause, which under-proved 2c.8 and left tasks.md overclaiming coverage. Fixed properly: added `fixtureInstant(loc, hour, minute)` pinning every fixture to one fixed calendar date (2026-06-15, a Monday) at a fixed debtor-local hour — every fixture now uses an in-window hour (10:00-14:30 range) EXCEPT `call-02-after-hours`, which is deliberately pinned to 22:00 (outside the window, to keep demonstrating the HARD BLOCK). `protectedMinorDemo`'s minor DOB is derived from its own pinned `OccurredAt.AddDate(-15,0,0)`, preserving age semantics. Removed the now-unused `now time.Time` parameter from `devDataFixtures` and the dynamic `afterHoursInstant` helper (replaced by the fixed 22:00 instant). Restored the direct Postgres-backed assertions this enables: `call-09-disclosure-warn` → `overall_outcome='pass'`, `MX-REDECO-03` row `outcome='warn'`/`severity='medium'`, `requires_hitl=false`; `call-01` (compliant) → `overall_outcome='pass'`. Verified deterministic: `go test ./... -count=1` run twice back-to-back, both green, and `go run ./cmd/seed dev-data` repro run twice against a fresh tenant showed the same pass/fail mix both times (call-01/message-01/email-01/call-04-neutral/call-09-disclosure-warn all `pass`; call-02-after-hours/call-03-threatening/call-05-third-party/call-06-protected-minor/call-07-unauthorized-channel/message-08-payment-routing all `fail`, each isolated to its own detector). Existing dev DBs seeded before this fix keep their old now-relative rows (transcript_ref-keyed idempotency means re-running the seed inserts nothing new for those refs) — documented in a `devDataFixtures` doc comment.

## Phase 3 (PR3): API Aggregate Endpoints

- [x] 3.1 Write `db/queries/dashboards.sql`: by-despacho (interaction-grain `total`/`violations`, unattributed bucket, tie-break by name), by-cause (`violations`+`warnings` per rule code); sqlc regen.
- [x] 3.2 Add `DashboardReader` interface + `internal/postgres/adapters.go` implementation using `tenantdb.WithTenantTx`.
- [x] 3.3 Add `GET /v1/dashboards/by-despacho` and `/by-cause` handlers to `internal/httpapi/httpapi.go`.
- [x] 3.4 [integration] Test: by-despacho ranking, unattributed bucket, tenant isolation (compliance-dashboards scenarios).
- [x] 3.5 [integration] Test: by-cause per-code breakdown, warnings-separate-from-violations, tenant isolation.

## Phase 4 (PR4): Console Dashboards

- [x] 4.1 Add `apps/console/src/lib/api.ts` client methods for both endpoints.
- [x] 4.2 Create `apps/console/src/app/dashboards/by-despacho/page.tsx` (server-component fetch, mirror interactions page).
- [x] 4.3 Create `apps/console/src/app/dashboards/by-cause/page.tsx`.
- [ ] 4.4 [manual-demo] Verify both pages render seeded demo-tenant data per spec scenarios. (Requires a developer running the local dev environment with the migrated + seeded Postgres and API server; not verifiable in this non-interactive apply session. `npx tsc --noEmit` and `npm run build` both pass, confirming both pages compile and are correctly dynamic-rendered.)
