# Proposal: Issue #7 Remaining Deterministic Detectors + Despacho/Cause Dashboards

## Intent

Issue #2 shipped one deterministic detector (contact-hours, MX-REDECO-04) and
issue #4 shipped the LLM judge (MX-REDECO-05). Issue #6 made policy bundles
versioned and immutable. Regulatory coverage is still thin: five prohibited
behaviors REDECO forbids have NO hard-block protection today. Compliance owners
also cannot see WHICH despacho or WHICH REDECO cause drives violations, so they
cannot rank collection firms by risk or target remediation.

This change closes the deterministic-coverage gap with five new pure detectors,
folds them into the versioned policy bundle, and adds the two aggregation
dashboards the issue requires. Success = four of the five prohibited cases
hard-block (third-party contact, protected population, authorized channel,
payment routing) with table-driven proof, the fifth (disclosure presence,
`MX-REDECO-03`) emits a `warn`-level outcome per its catalog action (WARN, not
HARD BLOCK), the rules are part of a versioned bundle, and compliance can rank
despachos by violation rate and break violations down by REDECO cause per
tenant.

## Why now

- Regulatory exposure: without third-party-contact, protected-population,
  authorized-channel, payment-routing, and disclosure-presence checks, Vigía
  passes interactions that REDECO explicitly prohibits.
- Issue #6 (merged) delivered the versioned-bundle infrastructure these rules
  are meant to live in; the seeding path for real rule content does not exist
  yet and must be established here.
- Despacho attribution is a confirmed product need (see Decisions).

## Decisions locked (do not re-open)

- **Despacho cardinality: 1 tenant (creditor) contracts N despachos.** A
  `despachos` table + FK on interactions is required. `docs/technical-design.md`
  is aspirational; only the 1:N cardinality is adopted from it.
- **The by-despacho dashboard is in scope** for this change.
- **Delivery is chained** (schema → detectors → API aggregates → console),
  detectors before dashboards, minimal nullable schema per detector — NOT the
  full aspirational schema.

## Open questions resolved by reading code

- **Rule seeding path**: There is NO production seeding of `policy_rules` /
  `policy_bundle_rules` content anywhere — not in migrations (DDL only) nor in
  `cmd/seed`. MX-REDECO-04/05 exist ONLY as `NamedDetector`/`NamedJudge` `Code`
  strings; the sqlc `CreatePolicyRule` + `PolicyBundleStore.CreateBundleVersion`
  path is exercised only by integration tests. This change must ESTABLISH the
  seeding convention (seed catalog rows + an active bundle snapshot in
  `cmd/seed`), covering the existing two rules and the five new ones.
- **Transcript structure**: `InteractionEvent.TranscriptRef *string` is only a
  reference pointer; no structured transcript text or extracted markers exist in
  `internal/core/types.go`. Disclosure-presence and authorized-channel detection
  therefore require minimal nullable structured fields on the interaction (not a
  transcript-mining pipeline, which is out of scope).
- **Code standardization**: recommend `NamedDetector.Code` use REDECO rule codes
  (`MX-REDECO-06/07/10/11` + `MX-REDECO-03` for deterministic disclosure, per
  `docs/regulatory-ruleset.md`) and rename the
  existing `"contact-hours"` wiring string to `"MX-REDECO-04"`, since golden-eval
  already keys on rule codes and both dashboards group by rule code.

## Scope

### In scope

- Five pure `Detector` implementations mirroring `ContactHoursDetector`
  (fail-closed on missing data, `Evaluate(Interaction) Result`, no I/O):
  third-party contact (MX-REDECO-06, hard-block), protected population /
  minor-always-protected, elderly-unless-debtor (MX-REDECO-07, hard-block),
  authorized channel/source (MX-REDECO-11, hard-block), payment routing /
  creditor-only-recipient (MX-REDECO-10, hard-block), disclosure presence
  (`MX-REDECO-03`, warn-level per its catalog action; the LLM-judge
  `MX-REDECO-02` disclosure rubric stays out of scope). Each with table-driven
  tests + a `TestXNoIO` purity proof.
- Minimal nullable schema additions on `Interaction`/`InteractionEvent`/`Debtor`
  feeding the detectors (contact-party relationship, debtor DOB/age + is-debtor
  flag, authorized-channel list + channel used, payment-recipient designation,
  disclosure markers).
- `despachos` table + `Despacho` Go type + despacho FK on interaction events
  (1 tenant : N despachos), RLS/grant treatment consistent with `00004`/`00007`.
- New detectors wired into `Detectors []NamedDetector` in BOTH `cmd/api/main.go`
  and `cmd/seed/main.go`; `NamedDetector.Code` standardized to rule codes.
- Rule-catalog + bundle seeding path: five `policy_rules` rows + active
  `policy_bundle_rules` snapshot (with `LegalBasis`/`EffectiveDate`) via
  `CreatePolicyRule` + `CreateBundleVersion`, establishing the convention.
- Two SQL-aggregate endpoints following the `CountOutOfHours` convention:
  by-despacho violation-rate ranking, by-REDECO-cause breakdown per tenant.
- Two console dashboard pages consuming those endpoints (greenfield).

### Out of scope

- Transcript ingestion / STT / NLP extraction pipeline. Disclosure and channel
  detection consume structured fields, not raw transcript mining.
- Full aspirational `docs/technical-design.md` schema (RFC, contract URIs,
  despacho status lifecycle) beyond the minimal despacho identity + FK.
- Data-driven rule interpretation (which detectors run per bundle) — remains the
  documented issue #6 follow-up; wiring stays static in `Service`.
- Golden-eval fixtures for the new detectors (optional stretch; the gate is
  hard-coded to two rule codes and stays green without them).
- Console detail/drill-down UX beyond the two ranking/breakdown views.

## Capabilities

### New Capabilities

- `deterministic-detectors`: the five new pure fail-closed detectors, their
  minimal input fields, standardized rule-code registration, and their seeding
  into the active versioned policy bundle.
- `despacho-registry`: the `despachos` table, `Despacho` type, and despacho FK
  establishing 1-tenant-to-N-despachos as a scoping/attribution dimension.
- `compliance-dashboards`: by-despacho violation-rate ranking and by-REDECO-cause
  breakdown SQL aggregates plus their console views.

### Modified Capabilities

- `contact-hours-detector`: registration code standardized from `"contact-hours"`
  to `"MX-REDECO-04"` and the existing rule seeded into the catalog/bundle. No
  detection-logic requirement change.
- `policy-bundle`: no requirement change — this change is the first CONSUMER that
  seeds real rule catalog/snapshot content through the existing mechanism.

## Approach

Adopt exploration Approach 2 (chained PRs), informed by Approach 3. Slice along
the dependency the codebase already shows: schema before detectors, detectors
before dashboards. Reuse the `ContactHoursDetector` shape (pure, fail-closed) and
the `CountOutOfHours` SQL-aggregate convention (aggregation in SQL, never in Go).
Add only nullable/minimal columns per detector, following `00007`'s migration-
safety pattern. Seed catalog + bundle content in `cmd/seed`, establishing the
convention for all seven rules. Keep the golden-eval gate and the LLM-judge
pipeline untouched.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `internal/detection/detector.go` | Modified | Grow `Interaction` with optional detector-input fields (non-breaking) |
| `internal/detection/*.go` (5 new) | New | Five pure fail-closed `Detector` implementations |
| `internal/detection/*_test.go` (5 new) | New | Table-driven + `TestXNoIO` per detector |
| `internal/core/types.go` | Modified | New `Despacho` type; DOB/authorized-channel/payment/disclosure fields |
| `db/migrations/000NN_*.sql` | New | `despachos` table + FK; nullable detector-input columns; RLS/grants |
| `db/queries/*.sql`, `internal/db/*.sql.go` | Modified | Aggregate queries + sqlc regeneration |
| `cmd/api/main.go`, `cmd/seed/main.go` | Modified | Wire 5 detectors; standardize codes (rename `"contact-hours"` → `"MX-REDECO-04"` here — the two production wiring sites); seed rule catalog + bundle |
| `internal/evaluation/service.go` | Modified | Restructure the binary block/else outcome fold (currently: `block → DetectorOutcomeFail`/`SeverityHigh`/overall fail, else → `Pass`/`SeverityLow`) into a 3-way branch: `detection.OutcomeWarn → core.DetectorOutcomeWarn` with persisted severity `medium`, never touching `overallOutcome`. Also add a per-detector `RequiresHITL` flag (true only for MX-REDECO-07) OR'd into the evaluation's `requiresHITL` when that detector blocks |
| `internal/httpapi/httpapi.go` + adapters | Modified | Two tenant-scoped aggregate readers/handlers |
| `apps/console/src/lib/api.ts`, `apps/console/src/app/**` | New | Two greenfield dashboard pages |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Despacho schema under/over-built | Med | Cardinality locked (1:N); ship minimal identity + FK, defer lifecycle fields |
| Detectors need transcript data not captured structurally | Med | Consume nullable structured fields, fail-closed when absent; transcript pipeline out of scope |
| Establishing rule seeding retroactively covers only 2 old rules inconsistently | Med | Seed all 7 rules (2 existing + 5 new) through one path in `cmd/seed` |
| Code rename `"contact-hours"`→`"MX-REDECO-04"` breaks wiring/tests | Low | Mechanical rename scoped to the two production wiring sites (`cmd/api`, `cmd/seed`) and any test that wires or asserts on the real `ContactHoursDetector`'s registration code — explicitly including `cmd/seed/devdata_integration_test.go`, which wires the real `ContactHoursDetector` with code `"contact-hours"` and persists rows to Postgres; a repo-wide grep shows ~14 other test files use the literal string `"contact-hours"` as an arbitrary fixture label for unrelated fake/stub detectors — those remain explicitly out of scope for this rename |
| Full issue exceeds the 400-line PR budget | High | Chained delivery, detectors splittable into 2-3 PRs (see Delivery slicing) |
| Adding to `Interaction` breaks `ContactHoursDetector` purity | Low | Fields are additive/optional; existing detector ignores them; `TestXNoIO` retained |

## Delivery slicing (does NOT fit one 400-line PR)

Chained PRs, each with independent verification and rollback:

1. **Despacho registry + detector-input schema** (~150–250 lines): `despachos`
   table + FK, `Despacho` type, nullable detector-input columns, migration +
   sqlc regen. De-risks the biggest unknown first.
2. **Detectors + seeding** (~400–750 lines total; SPLIT into 2–3 PRs of 1–2
   detectors each): pure detectors + table-driven tests + wiring + code
   standardization + rule-catalog/bundle seeding.
3. **API aggregate endpoints** (~150–200 lines): by-despacho + by-cause SQL
   aggregates, readers, handlers.
4. **Console dashboards** (~150–250 lines): two greenfield pages.

The by-REDECO-cause dashboard has no despacho dependency and can ship ahead of
by-despacho if PR 1 stalls. Detectors (PR 2) MUST land before dashboards (PR 3–4)
since the dashboards aggregate over detector output.

## Rollback Plan

- Each slice rolls back independently. Schema slices revert via `make
  migrate-down` (drops `despachos`, FK, nullable columns, grants).
- Detector slice: remove new files, revert the `Detectors` slice wiring in both
  cmd entrypoints, revert the code rename, revert catalog/bundle seed rows.
  Revert the `internal/evaluation/service.go` 3-way outcome fold and the
  `RequiresHITL` wiring back to the binary block/else fold.
- Migration `00008`'s `Down` reverses the `detector_code` backfill:
  `UPDATE detector_result_rows SET detector_code = 'contact-hours' WHERE
  detector_code = 'MX-REDECO-04' AND ...` (scoped to rows the forward
  migration touched). This reversal is safe only pre-production — acceptable
  because, per the already-stated assumption, no production traffic predates
  the rename.
- API/console slices: remove endpoints/readers and pages; no data migration.
- LLM judge, golden-eval gate, and issue #6 bundle immutability are untouched.

## Dependencies

- Issue #6 versioned/immutable `policy_bundle` infrastructure (merged).
- `ContactHoursDetector` pure-detector pattern (issue #2) and `CountOutOfHours`
  SQL-aggregate convention.
- goose + sqlc workflow (`make migrate-up`, `make sqlc`).

## Success Criteria

- [x] Four of the five prohibited cases (third-party contact, protected
      population, authorized channel, payment routing) HARD-BLOCK with
      table-driven tests + a `TestXNoIO` purity proof per detector; the
      fifth (disclosure presence, `MX-REDECO-03`) emits `warn` per its
      WARN-level catalog action, also with table-driven tests + a `TestXNoIO`
      purity proof.
- [x] All five new rules are seeded as `policy_rules` + active
      `policy_bundle_rules` snapshot (versioned bundle), via a single seeding path
      that also covers MX-REDECO-04/05.
- [x] `NamedDetector.Code` uses REDECO rule codes consistently (including the
      renamed contact-hours detector).
- [x] By-despacho dashboard ranks despachos by violation rate (1 tenant : N
      despachos).
- [x] By-REDECO-cause breakdown available per tenant.
- [x] Golden-eval gate and LLM-judge pipeline remain green/unchanged.
