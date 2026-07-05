# Verify Report: Issue #7 Remaining Deterministic Detectors + Dashboards

**What**: Verified issue-7-deterministic-detectors (PR1-PR4 stacked, branch
feat/issue-7-pr4-console-dashboards) against proposal/design/4
specs/tasks.md.

**Why**: SDD verify phase before archive.

**Where**: `openspec/changes/issue-7-deterministic-detectors/{proposal,design,tasks}.md`
+ `specs/{deterministic-detectors,despacho-registry,compliance-dashboards,contact-hours-detector}/spec.md`;
code: `internal/detection/*`, `internal/evaluation/service.go`,
`db/migrations/00008_deterministic_detectors.sql`, `db/queries/dashboards.sql`,
`internal/postgres/adapters.go` + `dashboards_integration_test.go`,
`internal/httpapi/httpapi.go` + `httpapi_test.go`,
`apps/console/src/lib/api.ts` + `app/dashboards/*`.

## Results

- `go build ./...` clean; `go test ./... -short -count=1` all green
  (21 packages ok, 0 fail); integration tests correctly skip via
  `testing.Short()` (no local docker/DATABASE_URL — did not start docker per
  instructions).
- All 5 new detectors (third-party MX-REDECO-06, protected-population
  MX-REDECO-07, authorized-channel MX-REDECO-11, payment-routing
  MX-REDECO-10, disclosure MX-REDECO-03) + ContactHours rename to
  MX-REDECO-04: full unit test suites pass with exact scenario coverage
  (boundary ages, OccurredAt-relative, DST border zone, fail-closed
  rationale).
- `db/queries/dashboards.sql` matches compliance-dashboards spec verbatim:
  `COUNT(DISTINCT interaction_event_id)` interaction-grain (not row-grain),
  `NULLIF(total,0)` zero-guard, `COALESCE` 'unattributed' bucket, tie-break by
  `despacho_name` ASC, violations = fail-only (excludes review/warn), separate
  warnings count for MX-REDECO-03.
- Console: `tsc --noEmit` clean; `npm run build` succeeds; both
  `/dashboards/by-despacho` and `/dashboards/by-cause` render as dynamic (ƒ)
  routes, server-fetched, no client-side aggregation — matches spec.
- `tasks.md` on disk: 27/28 checked, only 4.4 (manual-demo) unchecked with
  accurate justification — matches apply-progress artifact (Engram #5702).
  An older Engram tasks artifact (#5701) is a stale revision showing Phase
  3/4 unchecked; disk + apply-progress are authoritative and agree.
- Accepted deviations confirmed present and correctly documented: one-way
  `detector_code` backfill `Down` migration (00008 lines 77-83),
  legal-counsel-pending age thresholds (18/60) in `protected_population.go`,
  task 4.4 manual-demo gap.
- Assertion quality audit: no tautologies/ghost-loops/mock-heavy tests found
  in sampled dashboard/detector/evaluation tests;
  `TestDashboardInteractionGrainNotRowGrain` specifically pins the grain
  invariant against a 2-fail-row fixture.

## Verdict

**PASS WITH WARNINGS**. 0 CRITICAL, 1 WARNING (integration-tagged scenarios —
despacho RLS/cardinality, dashboard SQL aggregates, seed idempotency — exist
and are correctly written/skip-gated but unexecuted locally due to no
Postgres; should run in CI before final archive confidence), 0 SUGGESTION.

## Learned

`mem_search` for the tasks topic_key can surface a stale revision even when
newer upserts exist under the same key — cross-check against the on-disk
artifact (or the most recent related artifact like apply-progress) when
counts disagree, rather than trusting the first Engram hit blindly.

## Post-verify update (archive-time)

Per user instruction at archive time: a full `make test-db` run against live
Postgres was subsequently executed and closed out the integration-test
WARNING above (see Engram observations #5716/#5717). All 6 stacked PRs
(#47-#52) merged to `main`; GitHub issue #7 closed. Task 4.4 remains the only
open item — a manual console demo, documented as a non-code gap (both
console pages compile, build, and render as dynamic server-fetched routes;
only the human-in-the-browser visual confirmation step is outstanding).

Engram observation IDs for traceability:
- Proposal: #5692
- Spec (round-4 judgment-day fixes): #5693
- Design (round-4 judgment-day fixes): #5694
- Tasks: #5701 (superseded on disk by the checked-off version reproduced in
  this archived `tasks.md`)
- Verify-report: #5716
