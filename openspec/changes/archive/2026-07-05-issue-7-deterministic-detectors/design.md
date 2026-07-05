# Design: Issue #7 Remaining Deterministic Detectors + Dashboards

## Technical Approach

Extend the proven seams unchanged: pure `detection.Detector` (fail-closed like
`ContactHoursDetector`), the `Service` folding loop (block→fail/high), the
`CountOutOfHours` SQL-aggregate pattern (aggregation in SQL, never Go), and
`00007`'s add-nullable migration safety. Grow `detection.Interaction` with
optional inputs (existing detector ignores them; `TestXNoIO` retained).
Snapshot per-interaction inputs onto `interaction_events` exactly like
`debtor_timezone`, so detectors stay self-contained and pure. Seed the rule
catalog + active bundle in `cmd/seed`, establishing the convention for all 7
rules. Delivered as chained PRs: schema → detectors+seeding → API → console.

## Architecture Decisions

### Decision: Detector-input schema placement
**Choice**: All per-interaction inputs are nullable columns snapshotted on
`interaction_events`; `debtors.date_of_birth` is the durable DOB source,
snapshotted to `interaction_events.contacted_party_dob` at ingest.
**Alternatives**: (a) join debtor fields live in detection — breaks purity;
(b) full `docs/technical-design.md` schema — rejected (aspirational).
**Rationale**: Mirrors the `debtor_timezone` precedent (`00003`): the pure
detector reads one self-contained `Interaction`, no I/O, no joins.

Columns (all nullable, additive; NULL/empty → fail-closed BLOCK, except
`disclosure_provided` which fails closed to `warn` per MX-REDECO-03's
WARN-level catalog action — see Decision below):
| Column | Detector |
|--------|----------|
| `contact_party_relationship text` (`debtor`/`authorized`/`third_party`) | MX-REDECO-06, MX-REDECO-07 (debtor short-circuit) |
| `contacted_party_dob date` | MX-REDECO-07 |
| `authorized_channels text[]` | MX-REDECO-11 |
| `payment_recipient text` (`creditor`/other) | MX-REDECO-10 |
| `disclosure_provided boolean` | MX-REDECO-03 (disclosure presence, warn-level) |

`contact_party_relationship` is the single source of truth for the contacted
party's relationship to the debtor; there is no separate
`contacted_party_is_debtor` column (that would duplicate the same fact with
no consistency invariant between the two). MX-REDECO-07's debtor
short-circuit is `contact_party_relationship = 'debtor'`.

`authorized_channels text[]` is the per-interaction snapshot of channels the
debtor authorized. MX-REDECO-11 compares it against the channel actually
used, already typed as call/message/email on `core.InteractionEvent.Channel`
(`internal/core/types.go`) — no redundant `channel_used` column is added on
the database side. `detection.Interaction` (`internal/detection/detector.go`)
does NOT currently carry a `Channel` field — it deliberately avoids
`core.InteractionEvent` to stay independent of persistence types (only
`OccurredAt`/`DebtorTimezone` exist on it today, per the
`debtor_timezone`/`00003` precedent). This change therefore explicitly adds
`Channel` (typed as `core.InteractionChannel`, or an equivalent detection-local type) as
a new field on `detection.Interaction`, mirroring how `OccurredAt` and
`DebtorTimezone` were added for `ContactHoursDetector` — snapshotted from
`InteractionEvent.Channel` the same way `DebtorTimezone` is snapshotted from
the debtor's resolved timezone. `AuthorizedChannels []string` is added
alongside it for the comparison.

### Decision: MX-REDECO-03 (disclosure presence) is `warn`-level, not hard-block
**Choice**: `docs/regulatory-ruleset.md:33` defines MX-REDECO-03's action as
"WARN + negative score; BLOCK new campaigns whose template omits it" —
campaign-template blocking is a separate, out-of-scope control surface. This
change implements only the per-interaction WARN. `detection.Outcome` gains a
third value, `OutcomeWarn = "warn"`, alongside `OutcomePass` and
`OutcomeBlock`. The disclosure-presence detector's `Evaluate` returns `warn`
(never `block`) when the required UNE/complaints-unit disclosure was not
stated, and `warn` (fail-closed, not `block`) when `disclosure_provided` is
`NULL` — since the catalog action for this rule is warn-level, fail-closed
cannot escalate it to a block-level outcome. It returns `pass` when the
disclosure was stated.

`core.DetectorOutcome` gains a matching `DetectorOutcomeWarn = "warn"` value
on `detector_result_rows.outcome`. The current folding logic in
`internal/evaluation/service.go` (`runDetectorsAndJudges`, ~lines 247-254) is
a binary `if res.Outcome == detection.OutcomeBlock` / `else`: the block
branch sets `core.DetectorOutcomeFail` + `core.SeverityHigh` + flips
`overallOutcome` to `"fail"`; the else branch sets `core.DetectorOutcomePass`
+ `core.SeverityLow` for everything that isn't a block — which today
silently includes any future non-block, non-pass outcome. This non-exhaustive
if/else is a hazard: introducing `detection.OutcomeWarn` without restructuring
the fold would fall into the `else` branch and be silently persisted as
`DetectorOutcomePass`/`SeverityLow`, indistinguishable from a genuine pass.

This change restructures the fold into an explicit 3-way branch:
- `detection.OutcomeBlock` → `core.DetectorOutcomeFail`, severity `high`,
  `overallOutcome = "fail"` (unchanged).
- `detection.OutcomeWarn` → `core.DetectorOutcomeWarn`, severity `medium`,
  `overallOutcome` is NOT touched (a warn row alone never flips the
  interaction to `"fail"`).
- `detection.OutcomePass` → `core.DetectorOutcomePass`, severity `low`
  (unchanged).

`detector_result_rows.outcome` has no `CHECK` constraint in `00001`
(verified: the column is `text NOT NULL` with no enum constraint), so the new
`00008` migration does not need to alter any constraint to accept the `warn`
value.

Both dashboard aggregates keep their violation predicate at strictly
`outcome = 'fail'` (unchanged from the existing Decision below): `warn` rows
are excluded from `violations` counts in both `by-despacho` and `by-cause`,
the same as `review` rows. The `by-cause` endpoint additionally exposes a
`warnings` count per rule code (`COUNT(*) FILTER (WHERE outcome = 'warn')`) so
compliance can see MX-REDECO-03 activity without it inflating the violation
rate; `by-despacho` stays fail-only, since ranking despachos by combined
warn+fail volume would blur a WARN-level disclosure gap with a HARD-BLOCK
violation of equal weight.

The `policy_rules` catalog row for MX-REDECO-03 is seeded with
`severity = 'medium'` (using the existing `core.Severity` enum:
`low`/`medium`/`high`/`critical`), reflecting its warn-level catalog action;
the four hard-block rules (`MX-REDECO-06/07/10/11`) are seeded with
`severity = 'high'`. Runtime `detector_result_rows.severity` (folding-loop-
controlled) now matches this per-outcome: block rows persist severity `high`,
warn rows persist severity `medium`, and pass rows persist severity `low` —
see the restructured 3-way fold above. Warn rows MUST NOT be silently
foldable into a `pass`/`low` row via a non-exhaustive if/else.
**Alternatives**: (a) keep disclosure presence as a hard `block` — rejected:
contradicts the ruleset's explicit WARN action and would over-block traffic
the regulation only asks to be flagged; (b) fold disclosure into the existing
`review` outcome — rejected: `review` means judge uncertainty, not a
confirmed-but-warn-level policy signal; conflating the two would make
`review` counts ambiguous.
**Rationale**: The detector's outcome vocabulary must be able to express the
regulation's own action taxonomy (HARD BLOCK vs. WARN) rather than collapsing
every deterministic detector into a binary pass/block, or the disclosure rule
would either over-block or be silently dropped.

### Decision: despachos table shape (1 tenant : N despachos)
**Choice**: Minimal identity table mirroring `debtors` (id, tenant_id,
external_ref, display_name), `UNIQUE (id, tenant_id)` + `UNIQUE (tenant_id,
external_ref)`. Nullable composite FK `interaction_events.despacho_id` →
`despachos(id, tenant_id)` + index. RLS `tenant_isolation` policy + `GRANT
SELECT TO vigia_app` (writes owner-only, per `00004`/`00007`).
**Alternatives**: despacho status/contract lifecycle — deferred (out of scope).
**Rationale**: Cardinality is locked; ship attribution dimension only.

### Decision: rule codes + severity
**Choice**: `NamedDetector.Code` = `MX-REDECO-06/07/10/11` + `MX-REDECO-03`;
rename existing `"contact-hours"` → `"MX-REDECO-04"` in both cmd entrypoints
and any test that wires or asserts on the real
`ContactHoursDetector`'s registration code (grep-verified: a repo-wide grep
shows the literal string `"contact-hours"` also appears in ~14 test files as
an arbitrary fixture label for unrelated fake/stub detectors — those are out
of scope and are NOT renamed, EXCEPT `cmd/seed/devdata_integration_test.go`,
which wires the real `ContactHoursDetector` with code `"contact-hours"` and
persists rows to Postgres and is therefore IN scope for the rename). Runtime
`detector_result_rows.severity` now varies by outcome per the restructured
3-way fold (block → `high`, warn → `medium`, pass → `low`); per-rule catalog
severity lives in the `policy_rules` table separately, where MX-REDECO-03 is
seeded as `medium` (see the warn-level Decision above) and the four
hard-block rules as `high`.
**Rationale**: Golden-eval and both dashboards key on rule codes; the
restructured 3-way fold (see the warn-level Decision above) is the only
folding-logic change this proposal makes, and it is scoped narrowly to
support the `warn` outcome and per-rule HITL (see the next Decision) — it
does not otherwise alter block/pass folding behavior.

### Decision: MX-REDECO-07 (protected population) block requires HITL
**Choice**: `docs/regulatory-ruleset.md:38` mandates "HARD BLOCK + HITL" for
protected-population contact, not a plain hard block. Today `requiresHITL` on
the evaluation is set only by the LLM-judge loop (a judge failure/timeout
forces `requiresHITL = true`); the deterministic-detector fold never touches
it. This change adds a `RequiresHITL bool` field to `NamedDetector`, set
`true` only for the `MX-REDECO-07` entry (all other detectors leave it at its
zero value, `false`). In the restructured 3-way fold in
`internal/evaluation/service.go`, when a detector's outcome is `block` AND
`nd.RequiresHITL` is `true`, the evaluation's `requiresHITL` is OR'd to
`true` (never unset by any other detector or by MX-REDECO-07 passing).
**Alternatives**: (a) a standalone per-rule-code HITL lookup table outside
`NamedDetector` — rejected: adds an indirection with no other consumer, when
a single boolean field on the existing wiring struct is sufficient; (b)
forcing HITL for ALL hard-block detectors — rejected: the catalog only
mandates HITL for the protected-population rule, and over-applying it would
flag routine third-party/channel/payment blocks for human review with no
regulatory basis.
**Rationale**: The catalog's HITL requirement is a per-rule regulatory
obligation, not a generic hard-block side effect; the minimal design is a
boolean flag on the existing detector-wiring struct, OR'd into the same
`requiresHITL` field the judge loop already populates.

### Decision: catalog + bundle seeding idempotency
**Choice**: Add `UpsertPolicyRule` (`INSERT ... ON CONFLICT (code) DO UPDATE`)
for the 7 global rules. Seed one active bundle `redeco-baseline` via
`CreateBundleVersion` only when `GetActiveBundleByTenant` finds none (guards
re-seed from stacking v2, v3…).
**Alternatives**: plain `CreatePolicyRule` — violates the `code` UNIQUE on
re-seed. **Rationale**: Success criteria require idempotent single-path seeding.

### Decision: aggregate endpoints (SQL, RLS-scoped)
**Choice**: `GET /v1/dashboards/by-despacho` (violation-rate ranking) and `GET
/v1/dashboards/by-cause` (per-code breakdown). Both aggregate in SQL, tenant
scoping via RLS `current_setting('app.tenant_id')` (no explicit filter, like
`CountOutOfHoursEvaluations`), through `tenantdb.WithTenantTx`.
**Rationale**: Reuses the established `SummaryReader` reader/handler shape.

### Decision: age reference instant for protected-population detection
**Choice**: MX-REDECO-07 MUST compute the contacted party's age from
`contacted_party_dob` relative to `Interaction.OccurredAt` — never
`time.Now()` or any wall-clock read. Age = `OccurredAt` − `contacted_party_dob`.
**Alternatives**: computing age relative to evaluation time (`time.Now()`) —
rejected: breaks detector purity (`TestXNoIO`) and violates the issue #6
`ReEvaluateInteraction` byte-reproducibility guarantee, since the same
interaction would silently switch outcome (BLOCK → PASS) as the contacted
party ages past majority between the original evaluation and any later
re-evaluation.
**Rationale**: A pure detector's outcome for a given `Interaction` value MUST
be deterministic and stable across time; only fields already present on the
`Interaction` may influence the outcome.

### Decision: age thresholds are named constants, pending legal confirmation
**Choice**: The protected-population detector defines two named constants:
`legalMajorityAge = 18` (years) and `elderlyAge = 60` (years, per the *Ley de
los Derechos de las Personas Adultas Mayores*'s definition of "persona adulta
mayor" as 60 years or older). Both are plain Go constants in the detector
package, not configuration or database-driven values.
**Alternatives**: making the thresholds configurable per tenant/bundle —
rejected: no product requirement for tenant-specific age thresholds exists
yet, and REDECO/the cited statute define them as fixed legal ages, not
tenant policy choices.
**Rationale, and open item**: these values MUST be confirmed by legal counsel
before production use, mirroring the open-items practice already established
in `docs/regulatory-ruleset.md`. This design pins them as the best-available
reading of the cited statute so implementation can proceed, not as a final
legal determination.

### Decision: unattributed despacho bucket
**Choice**: `GET /v1/dashboards/by-despacho` reports interactions with
`despacho_id IS NULL` under an explicit synthetic bucket: `despacho_id: null,
despacho_name: "unattributed"`, included in the ranked response alongside
named despachos (not a separate endpoint or a silently dropped denominator).
**Alternatives**: (a) omit unattributed interactions entirely — rejected:
silently shrinks the denominator and hides attribution-coverage gaps from
compliance; (b) fold them into an arbitrary named despacho — rejected:
misattributes violations.
**Rationale**: Compliance owners need visibility into how much of their
violation volume is *not yet* attributed to a despacho, not just a clean
ranking of the ones that are.

### Decision: violation predicate for dashboard aggregates
**Choice**: Both aggregate endpoints count a row as a violation strictly when
`detector_result_rows.outcome = 'fail'`. This is explicitly NOT
`outcome != 'pass'`.
**Rationale**: `review` rows represent LLM-judge uncertainty (timeout,
malformed judge output) — not a confirmed policy violation. Treating `review`
as a violation would inflate violation rates with unresolved judge failures
rather than actual REDECO breaches. `warn` rows (MX-REDECO-03) are excluded
for the same reason: they are a confirmed warn-level signal, not a
hard-block-equivalent violation, and blending them into `violations` would
misrepresent a WARN-level disclosure gap as a HARD-BLOCK breach (see the
warn-level Decision above).

### Decision: debtor-relationship precedence in protected-population detection
**Choice**: `docs/regulatory-ruleset.md:38` (MX-REDECO-07) prohibits contact
with minors OR elderly persons; the debtor exemption applies ONLY to the
elderly case ("unless the elderly person is the debtor") — a minor is
protected regardless of `contact_party_relationship`. The detector's decision
table is therefore:
- **Age below `legalMajorityAge` (18) as of `OccurredAt`** → `BLOCK` always,
  regardless of `contact_party_relationship` (including `'debtor'` — a minor
  debtor is still protected; the debtor exemption never applies to minors).
- **Age at/above `elderlyAge` (60) as of `OccurredAt`** → `BLOCK`, UNLESS
  `contact_party_relationship = 'debtor'`, in which case `PASS` (the elderly-
  debtor exemption).
- **Age between the two thresholds (18 ≤ age < 60)** → `PASS`, regardless of
  relationship.
- **`contacted_party_dob` missing (NULL) and `contact_party_relationship =
  'debtor'`** → `PASS`. Age cannot be computed, but blocking all debtor
  contact lacking a DOB would block essentially all existing traffic (DOB is
  a new, sparsely-populated field), and the elderly-debtor case already
  passes regardless of age; the undetectable residual risk — a minor debtor
  contacted with no DOB on file — is accepted and documented here rather than
  silently over-blocking.
- **`contacted_party_dob` missing (NULL) and `contact_party_relationship` is
  `'authorized'`, `'third_party'`, or unset/`NULL`** → `BLOCK` (fail closed,
  unchanged: age cannot be verified for a non-debtor contact).
**Alternatives**: (a) the debtor exemption bypassing the minor check entirely
(prior draft) — rejected as a confirmed regulatory-compliance bug: it would
let a minor debtor be contacted freely, contradicting
`docs/regulatory-ruleset.md:38`; (b) blocking ALL debtor contact with a
missing DOB — rejected: would block nearly all traffic today, since DOB is a
newly-added, largely unbackfilled field.
**Rationale**: Reusing `contact_party_relationship` (rather than a separate
`contacted_party_is_debtor` boolean) removes the risk of the two fields
disagreeing, and the decision table above is the minimum logic that honors
the statute's asymmetric treatment of minors vs. the elderly while keeping
fail-closed behavior for genuinely unverified non-debtor contacts.

## Data Flow

    ingest ─► interaction_events (+ snapshot detector inputs, despacho_id)
                     │
    Service ─► 6 detectors + 1 judge ─► detector_result_rows / evaluations
                     │
    by-despacho / by-cause SQL aggregates ─► httpapi ─► console pages

## File Changes

| File | Action | Description |
|------|--------|-------------|
| `db/migrations/00008_deterministic_detectors.sql` | Create | despachos + FK, nullable detector columns, `debtors.date_of_birth`, RLS/grants |
| `internal/detection/detector.go` | Modify | Grow `Interaction` with optional fields: `Channel` (new — not previously present, since `Interaction` deliberately avoids `core.InteractionEvent`), `ContactPartyRelationship`, `ContactedPartyDOB`, `AuthorizedChannels`, `PaymentRecipient`, `DisclosureProvided`; add `OutcomeWarn` to the `Outcome` enum |
| `internal/detection/{third_party,protected_population,authorized_channel,payment_routing,disclosure}.go` (+_test) | Create | 5 pure fail-closed detectors + table-driven + `TestXNoIO` |
| `internal/core/types.go` | Modify | `Despacho` type; DOB/channel/payment/disclosure fields |
| `db/queries/{despachos,dashboards,policies}.sql`, `internal/db/*` | Modify | despacho CRUD, 2 aggregates, `UpsertPolicyRule`; `make sqlc` regen |
| `cmd/api/main.go`, `cmd/seed/main.go` | Modify | Wire 5 detectors, rename code, seed catalog + bundle, set `RequiresHITL: true` on the `MX-REDECO-07` `NamedDetector` entry |
| `cmd/seed/devdata_integration_test.go` | Modify | In rename scope: wires the real `ContactHoursDetector` with code `"contact-hours"` and persists rows to Postgres — update to `"MX-REDECO-04"` |
| `internal/evaluation/service.go` | Modify | Restructure the binary block/else outcome fold (~lines 247-254) into a 3-way branch (block/warn/pass); add `RequiresHITL bool` to `NamedDetector`; OR it into the evaluation's `requiresHITL` when an `MX-REDECO-07` block occurs |
| `internal/httpapi/httpapi.go`, `internal/postgres/adapters.go` | Modify | 2 readers + handlers |
| `apps/console/src/lib/api.ts`, `apps/console/src/app/dashboards/**` | Create | 2 greenfield pages |

## Interfaces / Contracts

```go
type DashboardReader interface {
    ByDespacho(ctx context.Context, tenantID string) ([]DespachoRate, error)
    ByCause(ctx context.Context, tenantID string) ([]CauseCount, error)
}
```
`by-despacho`: `[{despacho_id, despacho_name, total, violations, violation_rate}]`,
where an unattributed row is shaped `{despacho_id: null, despacho_name:
"unattributed", total, violations, violation_rate}`. `violations` in both
`by-despacho` and `by-cause` counts rows where
`detector_result_rows.outcome = 'fail'` only (never `outcome != 'pass'`, so
both `review` and `warn` rows are excluded).
`by-cause`: `[{rule_code, violations, warnings}]` — `warnings` is a separate
count of `outcome = 'warn'` rows per rule code (in practice, non-zero only
for `MX-REDECO-03`), added so MX-REDECO-03 activity is visible without
inflating `violations`. `by-despacho` does NOT gain a `warnings` column: the
despacho ranking stays fail-only, since blending warn-level disclosure gaps
into a despacho's ranked violation rate would understate or overstate risk
relative to despachos with only hard-block violations.

**Violation-rate denominator granularity (by-despacho)**: both `total` and
`violations` MUST be counted at interaction grain, never row grain. A single
evaluated interaction produces up to 7 `detector_result_rows` (one per rule),
so dividing at row grain would let rates exceed 100% or dilute a real
violation rate. Concretely:
- `total` = `COUNT(DISTINCT interaction_events.id)` for interactions that
  have been evaluated (one evaluation per interaction), scoped per despacho.
- `violations` = `COUNT(DISTINCT interaction_events.id)` for interactions
  that have at least one `detector_result_rows` row with `outcome = 'fail'`
  (an `EXISTS` correlated subquery or a pre-aggregated `GROUP BY
  interaction_events.id HAVING bool_or(outcome = 'fail')`), never
  `COUNT(detector_result_rows.*)`.
- `violation_rate = violations::numeric / NULLIF(total, 0)`, guarding the
  zero-total case (an unattributed/empty despacho bucket) from a
  divide-by-zero error.
- Ties in `violation_rate` MUST tie-break by `despacho_name` ascending, so
  the ranking is deterministic across repeated calls.

## Testing Strategy

| Layer | What | Approach |
|-------|------|----------|
| Unit | 5 detectors incl. fail-closed on each missing input | Table-driven + `TestXNoIO` purity proof |
| Integration | Aggregates, RLS scoping, seeding idempotency | Postgres `requireDB` (existing harness) |
| E2E | Console pages render aggregate contract | Server-component fetch, mirror interactions page |

## Migration / Rollout

Single migration `00008`; `make migrate-up` + `make sqlc`. Each PR slice rolls
back independently; `migrate-down` drops despachos/FK/columns/grants. LLM judge,
golden-eval gate, and issue #6 immutability untouched.

The same `00008` migration that renames the wiring `"contact-hours"` →
`"MX-REDECO-04"` also includes a one-time backfill:
`UPDATE detector_result_rows SET detector_code = 'MX-REDECO-04' WHERE
detector_code = 'contact-hours'`. `detector_result_rows` has no append-only
guard, so this in-place backfill is safe and prevents the by-cause dashboard
from showing a split `contact-hours` / `MX-REDECO-04` bucket for
pre-migration rows. Assumption: this only matters for pre-production data —
no production traffic predates this rename.

This backfill is intentionally **one-way**: `00008`'s `Down` does NOT reverse
it. Once the wiring rename ships (a later PR of this change),
`detector_result_rows` rows with `detector_code = 'MX-REDECO-04'` can be
genuine post-rename rows that were never `'contact-hours'`, and there is no
reliable predicate to distinguish "backfilled from contact-hours" from
"inserted as MX-REDECO-04" after that point. A `Down` that blindly rewrote
every `MX-REDECO-04` row back to `contact-hours` would corrupt those genuine
rows. `Down` therefore leaves `detector_code` untouched; this is acceptable
because this migration only matters for pre-production data, per the same
assumption above.

Two additional clarifications:
- `UpsertPolicyRule`'s `INSERT ... ON CONFLICT (code) DO UPDATE` idempotency
  is safe because `policy_rules` has no append-only guard (only
  `policy_bundles`/`policy_bundle_rules` do, per `00007`).
- The DOB/despacho snapshot-at-ingest reads run under the owner/migration
  role, the same as `cmd/seed` today; `vigia_app` has no `SELECT` grant on
  `debtors`, so a future HTTP ingest endpoint performing this snapshot would
  need that grant added explicitly.

## Open Questions

None. The disclosure rule code is confirmed as `MX-REDECO-03` (Disclosure,
Deterministic: "Provide the UNE / complaints-unit contact ... and that a
complaint can be filed in REDECO") against `docs/regulatory-ruleset.md:33`.
`MX-REDECO-02` is the LLM-judge disclosure rule (`docs/regulatory-ruleset.md:32`),
reserved by the merged `openspec/specs/llm-judge/spec.md`.
