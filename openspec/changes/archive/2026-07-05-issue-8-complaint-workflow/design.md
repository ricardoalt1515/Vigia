# Design: Durable Complaint Workflow with SLA + Human-in-the-Loop

## Technical Approach

Poll-based durable state machine (Exploration Approach 1). A single River **periodic**
job is the correctness ground truth: it scans `complaint_cases` for cases still in `open`
(awaiting their review trigger), SLA-due and TTL-expired cases, and unprocessed
`human_reviews` rows, then enqueues **idempotent** `ComplaintTransition` jobs. Each
transition runs a compare-and-swap (CAS) UPDATE plus an
evidence-ledger append inside one `tenantdb.WithTenantTx`, exactly mirroring
`EvaluationStore.CreateEvaluation` (adapters.go). `complaint_cases.state` is the sole
source of truth; River job rows are never authoritative. Business-day arithmetic is a
pure function seam following the detector convention (`ContactHoursDetector.Evaluate`
takes the instant in; no `time.Now()` inside). **v1 scope decision**: every
non-escalated case routes through `awaiting_review` — there is no auto-resolve path,
so a human decision (`approve`/`override`) is required before any case reaches
`resolved` (see State Machine below and the Proposal's Scope section).

## Architecture Decisions

| Decision | Choice | Rejected | Rationale |
|---|---|---|---|
| Resume/SLA driver | River periodic poll job | LISTEN/NOTIFY | Deterministic, testable, one correctness surface; NOTIFY deferred (latency-only) |
| Source of truth | `complaint_cases.state` | River job state | Jobs are at-least-once; state must survive restarts and dedupe retries |
| Idempotency | `river.UniqueOpts{ByArgs:true}` on `(complaint_case_id, transition_kind)` + SQL CAS `WHERE tenant_id=$2 AND state=$from` | App-level locks | Double defense: unique enqueue dedupe **and** DB-level CAS no-op on re-run |
| Escalate-wins-tie mechanism | `approve`/`override` CAS carries a DB-clock temporal guard (`AND review_expires_at > now()`); a late approve matches 0 rows and no-ops | Ordering jobs by enqueue time / app-level tie-break | The predicate itself is the tie-break: at the DB's own clock, either the row is still `awaiting_review` and unexpired (approve wins) or it is not (approve no-ops); no race window depends on which job the poller enqueued first |
| Evidence atomicity | Append inside same `WithTenantTx` as state UPDATE | Two-phase / separate tx | Rollback leaves no partial state; reuses issue #3 ledger-chain-head lock pattern |
| Evidence schema | Extend the existing `evidence_records` hash chain via ALTER (add `record_kind`, nullable FKs, exactly-one-of CHECK) | New separate `complaint_evidence_records` table | Migration 00005 hard-codes `evaluation_id uuid NOT NULL` + `UNIQUE(tenant_id, evaluation_id)`; a second table would fork the hash chain instead of extending the one true per-tenant ledger |
| Case creation idempotency | `idempotency_key` column + `UNIQUE (tenant_id, idempotency_key)`, `INSERT ... ON CONFLICT DO NOTHING` | Client-side dedupe only | Matches canonical `ComplaintCase.IdempotencyKey` (technical-design.md ADR-01); makes `POST /v1/complaints` safely retriable |
| Holiday calendar | Static versioned table (LFT Art. 74) + weekends, seeded by migration; `calendar_version` pinned on each case | External service / dynamic decree feed | Reproducible, offline, counsel-confirmation TODO documented (see docs/regulatory-ruleset.md "Open legal items to confirm with counsel"; MX-REDECO-04A is a style precedent for how that note reads) |
| Ambiguity | Doubtful day = business day (earlier deadline) | Later deadline | Fail-closed: SLA expires sooner, never later |
| TTL expiry | Escalate | Auto-approve | Fail-closed: absence of a human decision is never consent |
| Approval TTL duration | **3 business days** (computed via the same `AddBusinessDays`/calendar as the SLA), configurable via a worker setting (`ComplaintReviewTTLBusinessDays`, default `3`) | Hard-coded constant with no override; a fixed wall-clock duration (e.g. 72h) | A default must exist for `review_expires_at` to be computable at all; reusing the business-day calculator keeps the TTL consistent with the SLA's fail-closed ambiguity handling (doubtful day counts as a business day, so the TTL is never longer than intended); configurability lets ops tune it without a code change if 3 days proves too short/long in practice |
| Holiday table scope | Global reference (no tenant_id, no RLS), `GRANT SELECT vigia_app` | Per-tenant | Statutory calendar is jurisdiction-wide, not tenant data |
| Tenant isolation enforcement | Explicit `tenant_id = $N` predicate in every CAS/UPDATE/INSERT/SELECT | Rely on RLS alone | Production pools (`cmd/api`, `cmd/worker`) connect via the owner role, which bypasses RLS (see `internal/tenantdb/tenantdb.go`: `WithTenantTx` only sets `app.tenant_id` for auditability, not enforcement); RLS stays on as defense-in-depth, `vigia_app`-role tests are the enforcement proof |
| Duplicate `human_reviews` bookkeeping | The committing transition marks **all** unprocessed rows for the case processed/superseded, referencing the winning row from the evidence record | Leave stale rows for the poller to skip | One transaction closes the audit trail deterministically; poll job never re-discovers a stale unprocessed row |
| Ledger hash binding for complaint content | Extend `ledger.Body` with a trailing `omitempty` `*ComplaintTransition` sub-struct (mirrors the existing `Judge` field) | Leave `Body` evaluation-only and rely on the row's `complaint_case_id`/`transition_kind` columns alone | `Body`/`canonicalBodyDTO` (internal/ledger/ledger.go) is the actual hashed content; its required `InteractionEventID`/`EvaluationID` strings carry no complaint fields today. Without extending `Body`, a tampered `complaint_case_id`, `transition_kind`, or `HumanReviewID` column would not change the hash — undetectable tamper, defeating the trust-core guarantee |
| NOT NULL sentinels for `overall_outcome`/`inputs_digest` on `complaint_transition` rows | `overall_outcome = to_state` (the transition's destination state); `inputs_digest = ""` (empty-string sentinel) | Leave both empty/undefined, or invent an unrelated placeholder like `'n/a'` | Both columns are `NOT NULL` with no default (migration 00005) and `Body.OverallOutcome`/`Body.InputsDigest` are required, non-`omitempty` strings — some value must be chosen. `to_state` reuses data the row already carries and is a meaningful terse outcome label (mirrors evaluation's `overall_outcome` semantics: a short result summary); `""` for `inputs_digest` follows the exact precedent already established for `PolicyBundleVersion`'s empty-string sentinel — a complaint transition has no detector "inputs" to digest, and the actual transition content is already captured (and hashed) via `Body.ComplaintTransition` itself, so digesting it again would be redundant |

## Canonical-Model Note (supersedes docs/technical-design.md §4)

This design's `ComplaintCase` and `HumanReview` shapes **supersede and extend** the
canonical structs sketched in `docs/technical-design.md` (lines ~222-228), which predate
the concrete schema work done here:

- Canonical `HumanReview` is keyed on `EvaluationID`; this design keys it on
  `ComplaintCaseID` instead — a human review in this workflow approves/overrides a
  *complaint case*, not an evaluation, so `EvaluationID` was never the correct key for
  this use case.
- Canonical `ComplaintCase` has no `ReviewExpiresAt`/`CalendarVersion` fields; this design
  adds both as load-bearing columns (`review_expires_at` drives the TTL fail-closed
  guard from Round 1; `calendar_version` pins the SLA computation for reproducibility).
- Canonical `IdempotencyKey`, `RedecoCause` are retained as-is (see Round 1 fixes);
  `Resolution`/`DespachoPenalty` remain out of scope for this change (issue #9).

`docs/technical-design.md` is **not** edited by this SDD change; reconciling the doc is a
follow-up doc-sync task at apply time (see File Changes manifest).

## State Machine

Terminal states: `resolved`, `escalated`. Transition applies only via CAS
(`UPDATE ... WHERE id=$1 AND tenant_id=$2 AND state=$from`); **0 rows affected ⇒ no-op
success** (already transitioned or terminal), which is what makes every job retry-safe.
`v1` deliberately routes **every** non-escalated case through `awaiting_review`: there is
no auto-resolve path, so a human decision is required on every complaint before it can
reach `resolved` (see Proposal Scope and the note below). Every `open` case requires
review — the `open → awaiting_review` transition is not optional or policy-gated in v1,
so the poll job's `open`-case scan (see River Jobs below) is what actually drives every
case toward human review; nothing else triggers `request_review`.

| From | Event (transition_kind) | To | Side effects (same tx) |
|---|---|---|---|
| — | `open` | `open` | insert case, `sla_due_at = AddBusinessDays(opened_at,10,cal)`, evidence append |
| `open` | `request_review` | `awaiting_review` | set `review_expires_at = AddBusinessDays(now, ComplaintReviewTTLBusinessDays, cal)` — default **3 business days**, evidence append — **triggered by the poll job's `open`-case scan**, not by case creation |
| `awaiting_review` | `approve` | `resolved` | CAS additionally requires `review_expires_at > now()`; mark **all** unprocessed `human_reviews` rows for the case processed/superseded (winning row referenced by the evidence record), `resolved_at`, evidence append |
| `awaiting_review` | `override` | `resolved` | same CAS temporal guard as `approve`; mark all unprocessed rows processed/superseded, `resolved_at`, evidence append (override note) |
| `awaiting_review` | `ttl_expired` | `escalated` | evidence append (fail-closed) |
| `open` \| `awaiting_review` | `sla_breach` | `escalated` | evidence append |

Invalid transitions never panic: the CAS predicate rejects them as no-ops. Evidence is
written by the **transition worker** (never the HTTP endpoint), at the moment the state
UPDATE commits — identical role/pool path as `EvaluationStore`.

**Escalate-wins-tie mechanism**: the `approve`/`override` CAS is
`UPDATE complaint_cases SET state='resolved', ... WHERE id=$1 AND tenant_id=$2 AND
state='awaiting_review' AND review_expires_at > now()`. If `ttl_expired` commits first,
the case is no longer `awaiting_review`, so a subsequent `approve` CAS matches 0 rows and
no-ops (the HTTP endpoint returns 409, see HTTP API below). If `review_expires_at` has
passed by the DB's own clock, the guard rejects the approve even if `ttl_expired` has not
yet been enqueued — the tie-break is a property of the predicate evaluated at commit time,
not of which job the poller happened to enqueue first.

## River Jobs

- **`ComplaintPollArgs`** — periodic (cadence **60s**, `river.PeriodicInterval`). Scans
  four independent findings each cycle and inserts one `ComplaintTransition` per finding:
  (1) cases in state `open` (every `open` case requires review in v1 — see State Machine
  above) → enqueue `request_review`; (2) due cases (`sla_due_at <= now`) → enqueue
  `sla_breach`; (3) expired reviews (`review_expires_at <= now` still `awaiting_review`)
  → enqueue `ttl_expired`; (4) unprocessed `human_reviews` → enqueue `approve`/`override`
  per the review's decision. Poll is stateless and re-scannable — missed ticks self-heal
  next cycle. This `open`-case scan is what closes the gap between case creation and
  human review: without it, a newly created case would sit in `open` until SLA breach,
  never reaching `awaiting_review`.
- **`ComplaintTransitionArgs{ComplaintCaseID, TenantID, TransitionKind, HumanReviewID
  *string}`** — does the CAS + evidence append via the store. `HumanReviewID` names the
  specific winning row for `approve`/`override` transitions; the store marks that row (and
  bulk-marks every other unprocessed `human_reviews` row for the case) processed/superseded
  in the same transaction, so the evidence record references a single deterministic winner
  and the poll job never re-discovers a stale unprocessed row. `InsertOpts` →
  `river.UniqueOpts{ByArgs:true}` so identical `(case_id, kind)` collapses to one job.
- **Worker registration**: extend `cmd/worker/main.go` — replace `NoopWorker` with
  `river.AddWorker(workers, NewComplaintTransitionWorker(store))` and register the
  periodic poll via `river.Config.PeriodicJobs`.
- **Retry semantics**: River retries failed/crashed jobs (at-least-once). Safe because
  the CAS makes any replay after commit a 0-row no-op — no double evidence write.

## Business-Day Calculator (pure seam)

```go
type Calendar struct { Version string; Holidays map[civil.Date]struct{} } // weekends implicit
func AddBusinessDays(from time.Time, n int, cal Calendar) time.Time // no time.Now()
func LoadCalendar(version string, rows []HolidayRow) Calendar
```

`opened_at` is passed in by the caller (open transition). `calendar_version` is written
onto the case row so an SLA can be recomputed/audited reproducibly even if the table is
later revised. Doubtful/edge day counts as a business day (earlier deadline).

## Ledger Hash Binding for Complaint Evidence

`ledger.Body` (`internal/ledger/ledger.go`) — not the `evidence_records` row — is the
actual hashed content; `canonicalBodyDTO` mirrors it field-for-field and field order is
load-bearing (per the package's own doc comment). Today `Body` requires
`InteractionEventID`/`EvaluationID` and carries no complaint fields, so a
`complaint_transition` record's `complaint_case_id`, `transition_kind`, or
`HumanReviewID` column would sit **outside** the hash — tampering on those columns
would be undetectable, defeating the ledger's trust-core guarantee.

Fix: extend `Body` with a trailing, `omitempty` pointer sub-struct, following the exact
precedent set by the `Judge` field (issue #4; see `TestHashGoldenValueUnchangedWithJudgeFieldAdded`):

```go
type ComplaintTransitionEvidence struct {
    ComplaintCaseID string  `json:"complaint_case_id"`
    TransitionKind  string  `json:"transition_kind"`
    FromState       string  `json:"from_state"`
    ToState         string  `json:"to_state"`
    HumanReviewID   *string `json:"human_review_id,omitempty"`
}

type Body struct {
    // ... existing fields, unchanged, in the existing order ...
    Judge               *JudgeEvidence                `json:"judge,omitempty"`
    ComplaintTransition *ComplaintTransitionEvidence  `json:"complaint_transition,omitempty"`
}
```

`ComplaintTransition` is trailing and `omitempty`, so evaluation-only bodies (nil
pointer) serialize byte-identically to today — the same no-diff guarantee `Judge`
established.

**`overall_outcome`/`inputs_digest` sentinels for `complaint_transition` rows**: both
columns are `NOT NULL` with no default (migration 00005:11-13; only
`policy_bundle_version` has `DEFAULT ''`), and `Body.OverallOutcome`/`Body.InputsDigest`
are required, non-`omitempty` strings — a value must be written and hashed either way.
This design writes `overall_outcome = to_state` (the transition's destination state,
e.g. `'awaiting_review'`, `'resolved'`, `'escalated'` — a terse outcome label, consistent
with how evaluation rows use `overall_outcome`) and `inputs_digest = ""` (the same
empty-string sentinel already established for `policy_bundle_version`; a complaint
transition has no detector "inputs" to digest, and its actual content is already bound
into the hash via `Body.ComplaintTransition`, so re-digesting it would be redundant).
Both values are written once at transition-commit time and read back verbatim — no
NULL-handling is needed for these two columns since they are always non-NULL by
construction.

A new golden-hash test
(`TestHashGoldenValueUnchangedWithComplaintTransitionFieldAdded`, mirroring
`TestHashGoldenValueUnchangedWithJudgeFieldAdded`) pins that the existing golden hash
is unchanged when `ComplaintTransition` is `nil`. `HumanReviewID` is itself `omitempty`
because `open`/`sla_breach`/`ttl_expired` transitions have no winning review row.

**Read path (NULL representation)**: `evidenceRowToRecord` (`internal/postgres/adapters.go`)
must reconstruct `Body` identically to what was hashed at write time. For an `evaluation`
row, `evaluation_id`/`interaction_event_id` are non-NULL and read back via `uuidString`
exactly as today. For a `complaint_transition` row those two columns are NULL; `uuidString`
on a NULL `pgtype.UUID` returns the empty string `""`, which is exactly the zero-value
`Body.EvaluationID`/`Body.InteractionEventID` the writer used for that record kind — so no
special-casing is needed on read, only that the writer and reader agree the empty string is
the canonical value for "not applicable to this record kind" (same discipline the empty-string
`PolicyBundleVersion` sentinel already uses, per `TestHashGoldenValueUnchangedWithEmptyPolicyBundleVersion`).
`Body.ComplaintTransition` is populated only when `record_kind='complaint_transition'`
(mirroring how `Body.Judge` is populated only when the `judge_*` columns are non-NULL).
`Body.OverallOutcome`/`Body.InputsDigest` need no NULL-handling on read for
`complaint_transition` rows: the columns are `NOT NULL` and were written at transition
time as `to_state`/`""` respectively (see the sentinel decision above), so `evidenceRowToRecord`
reads them back the same way it does for evaluation rows — no branch on `record_kind`
is needed for these two fields specifically.
`ChainVerifier.VerifyChain` and `cmd/ledger-verify` need no interface or logic change: they
already operate on the pure `[]ledger.EvidenceRecord`/`Body` shape and recompute
`Hash(prevHash, body)` generically, regardless of which record kind produced the body.

## HTTP API

`POST /v1/complaints` in `internal/httpapi/httpapi.go` is the case-creation trigger,
following the existing `handleReEvaluate` shape: `Authenticator.Authenticate` → tenant
scope → decode `{idempotency_key, interaction_id, redeco_cause, ...}` (client supplies the
idempotency key, header `Idempotency-Key` or body field). In one `tenantdb.WithTenantTx`:
`INSERT INTO complaint_cases (...) VALUES (...) ON CONFLICT (tenant_id, idempotency_key)
DO NOTHING RETURNING *` (on conflict — 0 rows returned — falls back to
`SELECT * FROM complaint_cases WHERE tenant_id = $1 AND idempotency_key = $2` to return the
existing case; the fallback SELECT filters on the same composite key as the UNIQUE
constraint, never on `idempotency_key` alone), computes `sla_due_at` via the business-day calculator with the pinned
`calendar_version`, and appends the `open` evidence record. The endpoint does **not**
enqueue anything itself — per the single-trigger invariant (see State Machine and River
Jobs above), the new row is simply picked up by the next periodic poll cycle's
`open`-case scan, which is what actually enqueues `request_review`. Response is
idempotent: `201 Created` for a fresh case, `200 OK` returning the existing case on a
repeated key.

`POST /v1/complaints/{id}/reviews`, same shape: `Authenticator.Authenticate` → tenant scope
→ decode `{decision: "approve"|"override", reviewer, notes}`. Validation: decision enum,
case belongs to tenant and is `awaiting_review`. Inserts a `human_reviews` row with
`review_expires_at`; it does **not** transition — the poll resumes the case. Race with TTL:
if the case has already left `awaiting_review` (escalated), the endpoint itself checks
current state and returns **409 Conflict** (the review is late) instead of inserting a
review it knows is moot; independently, if a review is inserted just before the deadline
and the corresponding `approve`/`override` transition job loses its CAS's
`review_expires_at > now()` guard to a concurrent `ttl_expired` commit, that job matches
0 rows and no-ops, fail-closed — the two checks are independent layers, not one relying on
the other.

## Concurrency & Failure Analysis

| Scenario | Resolution |
|---|---|
| Two workers pick same case | `UniqueOpts` dedupes enqueue; CAS + row serialization ⇒ one commits, other no-ops |
| Approve races escalation | Both CAS from `awaiting_review`; whichever CAS commits first flips the state, so the loser's CAS predicate (`state='awaiting_review' AND review_expires_at > now()`) matches 0 rows and no-ops — the DB-clock guard is what makes "escalate wins" deterministic, not job enqueue order |
| Crash between tx commit and job "completed" | River replays job; CAS no-op — state already moved, evidence not duplicated |
| Job runs after case resolved | CAS from-state mismatch ⇒ 0 rows ⇒ success no-op |
| Duplicate case-creation request (same idempotency key) | `ON CONFLICT (tenant_id, idempotency_key) DO NOTHING`; the endpoint returns the existing case, no duplicate SLA timer or evidence row |
| `approve` and `override` both enqueued for the same case | `UniqueOpts` is keyed on `(complaint_case_id, transition_kind)` — it does **not** dedupe across different `transition_kind`s, so both jobs may be enqueued and run. The CAS (`state='awaiting_review' AND review_expires_at > now()`) still makes whichever commits first the sole winner; the loser's CAS matches 0 rows and no-ops. **Accepted limitation**: which of the two decisions wins — and therefore which evidence row is the one bound into the hash chain — is race-dependent (whichever transaction's CAS commits first), not deterministic on submission order; this is an accepted audit-trail characteristic of allowing both `human_reviews` rows to exist, not a bug |
| `ttl_expired` wins over a late `approve`/`override` | The late transition's CAS no-ops (see Escalate-wins-tie mechanism above) and, because it never commits, it never runs the bulk-mark-processed/superseded step. **Accepted limitation**: the corresponding `human_reviews` row remains permanently `unprocessed` — it is not retroactively marked `superseded` by the escalation. This is intentional (the escalation transition has no reason to touch a row it never consumed) but is a known triage gap: an operator auditing "unprocessed reviews" will see this row forever unless a future follow-up adds a `superseded_reason`-style sweep that closes out unprocessed reviews on cases that reach a terminal state via a different transition |

## File Changes

> **Migration numbering note**: issue #7 has merged (`00008_deterministic_detectors.sql`
> is now in `db/migrations/`); `00009` is the next free number as of this rebase.

| File | Action | Est. LOC |
|---|---|---|
| `db/migrations/00009_complaint_workflow.sql` | Create — `complaint_cases` (columns: `id`, `tenant_id`, `interaction_id`, `redeco_cause`, `state`, `opened_at`, `sla_due_at`, `calendar_version`, `review_expires_at`, `resolved_at`, `idempotency_key`, `created_at`, `updated_at`; `UNIQUE(id, tenant_id)` — required for the `evidence_records` composite FK below, matching every existing tenant-scoped table's pattern, e.g. `evaluations`/`evidence_records` in 00005:16; `UNIQUE(tenant_id, idempotency_key)`), `human_reviews`, `business_day_holidays` (RLS on the 2 tenant tables), enum CHECKs, composite FKs, grants, holiday seed | ~180 |
| `db/migrations/00009_complaint_workflow.sql` (same file, ALTER section) | Modify — `ALTER TABLE evidence_records`: add `record_kind text NOT NULL DEFAULT 'evaluation'` discriminator, make `interaction_event_id`/`evaluation_id` nullable, add nullable `complaint_case_id` + composite FK `(complaint_case_id, tenant_id) REFERENCES complaint_cases(id, tenant_id)`, add `transition_kind text`, CHECK `(record_kind = 'evaluation' AND evaluation_id IS NOT NULL AND interaction_event_id IS NOT NULL AND complaint_case_id IS NULL) OR (record_kind = 'complaint_transition' AND complaint_case_id IS NOT NULL AND evaluation_id IS NULL AND interaction_event_id IS NULL)`, drop `UNIQUE(tenant_id, evaluation_id)` and replace with a partial unique index `WHERE record_kind='evaluation'`, add partial unique index on `(tenant_id, complaint_case_id, transition_kind) WHERE record_kind='complaint_transition'` (backstops transition idempotency); backfill existing rows `record_kind='evaluation'` | ~60 |
| `internal/ledger/ledger.go` | Modify — add `ComplaintTransitionEvidence` sub-struct + trailing `omitempty` `Body.ComplaintTransition` field (mirrors `Judge`); update `canonicalBodyDTO` in lockstep | ~30 |
| `internal/ledger/ledger_test.go` | Modify — add `TestHashGoldenValueUnchangedWithComplaintTransitionFieldAdded` (mirrors `TestHashGoldenValueUnchangedWithJudgeFieldAdded`) + a tamper-detection case for `ComplaintTransition` fields | ~40 |
| `internal/postgres/adapters.go` (`CreateEvaluation`) | Modify — set `record_kind='evaluation'` explicitly on the evidence-record insert | ~5 |
| `internal/postgres/adapters.go` (`evidenceRowToRecord`, `ListEvidenceRecordsByTenant`) | Modify — read path: reconstruct `Body.ComplaintTransition` from the new nullable columns (nil when `record_kind='evaluation'`); represent NULL `evaluation_id`/`interaction_event_id` as the empty string on read, matching what was hashed at write time (see Ledger Hash Binding section) | ~30 |
| `cmd/ledger-verify/main.go` | Modify — no interface change (`ChainVerifier.VerifyChain` signature is unchanged); document that a tenant's chain now interleaves `evaluation` and `complaint_transition` rows and verifies uniformly | ~5 |
| `db/queries/complaint_cases.sql` | Create — CAS transition (with `review_expires_at` guard), idempotent open (`ON CONFLICT DO NOTHING`), list-due, list-open-awaiting-triage | ~85 |
| `db/queries/human_reviews.sql` | Create — insert, list-unprocessed, bulk mark-processed/superseded | ~40 |
| `db/queries/business_day_holidays.sql` | Create — list by version | ~10 |
| `internal/orchestrator/calendar.go` | Create — pure business-day calc | ~90 |
| `internal/orchestrator/state.go` | Create — transition table + guards | ~110 |
| `internal/orchestrator/jobs.go` | Create — River args + workers (poll + transition); poll additionally scans `open` cases needing review | ~170 |
| `internal/postgres/complaint_store.go` | Create — `ComplaintCaseStore` (CAS + evidence append, idempotent create) | ~190 |
| `cmd/worker/main.go` | Modify — register real workers + periodic poll | ~40 |
| `internal/httpapi/httpapi.go` | Modify — `POST /v1/complaints` (create) + `POST /v1/complaints/{id}/reviews` endpoints + DTOs | ~110 |
| `docs/technical-design.md` (doc-sync, at apply time) | Modify — reconcile canonical `ComplaintCase`/`HumanReview` structs (§4, lines ~222-228) with this design's superseding shapes (see Canonical-Model Note above); not edited by this SDD change itself | ~15 |
| `go.mod` / `go.sum` | Modify — River `indirect`→direct (`go mod tidy`) | ~5 |
| Test files (per Strict TDD, below), incl. `rls_isolation_test.go`-style coverage for the new tables on `vigia_app` | Create | ~650 |

Greenfield package ≫ 400-line review budget ⇒ **chained PRs** (slice at sdd-tasks):
(1) migration+queries+calendar, (2) store+state, (3) jobs+worker, (4) HTTP endpoints
(create + review).

## Testing Strategy (Strict TDD — `go test ./...`)

| Layer | What | Approach |
|---|---|---|
| Unit | `AddBusinessDays` / SLA | Table tests: weekends, holiday boundaries, doubtful-day fail-closed, version pinning |
| Unit | State machine | Table tests: every valid/invalid transition, CAS no-op semantics |
| **Unit (gate)** | `ledger.Body` golden hash | `TestHashGoldenValueUnchangedWithComplaintTransitionFieldAdded` — the existing pinned golden hex is unchanged when `Body.ComplaintTransition` is `nil`; a companion tamper test proves changing `ComplaintCaseID`/`TransitionKind`/`HumanReviewID` changes the hash |
| **Integration (gate)** | `river.UniqueOpts` v0.39.0 | **Targeted test proving `ByArgs` dedupe on `(case_id, kind)` BEFORE relying on it** (mirrors `worker_integration_test.go`, skip on `-short`/no `DATABASE_URL`) |
| Integration | Store transition atomicity | Rollback leaves no case/evidence drift; evidence append + UPDATE commit together |
| Integration | Tenant isolation (app-level) | Every CAS/UPDATE/INSERT predicate includes `tenant_id = $N`; tenant A cannot read/transition tenant B's case even on the owner pool (RLS is defense-in-depth, not the primary guarantee) |
| Integration | Tenant isolation (RLS, defense-in-depth) | Under the restricted `vigia_app` role, tenant A cannot read/transition tenant B's case (mirror `rls_isolation_test.go`) |
| Integration | Idempotent case creation | Duplicate `POST /v1/complaints` with the same idempotency key returns the same case, no duplicate SLA timer/evidence row |
| Integration | Evidence schema (ALTER) | Exactly-one-of CHECK rejects rows with both/neither `evaluation_id`/`complaint_case_id`; partial unique indexes hold per `record_kind` |
| Integration | Mixed-kind chain verification | `VerifyChain` over a tenant chain interleaving `evaluation` and `complaint_transition` rows (via `evidenceRowToRecord`) succeeds end-to-end, proving the read path reconstructs `Body`/`Body.ComplaintTransition` identically to what was hashed at write time, including the `overall_outcome=to_state`/`inputs_digest=""` sentinels on `complaint_transition` rows |
| Integration | Poll enqueue | Due SLA / expired TTL / new review / **`open` case awaiting triage** each enqueue the correct transition (the last one enqueues `request_review`, moving the case to `awaiting_review`) |
| Integration | Duplicate human_reviews | Two unprocessed rows for one case ⇒ one transition commits; all rows marked processed/superseded in that transaction |
| HTTP | Create endpoint | Auth, idempotency-key handling (fresh vs. conflict), evidence+SLA computed in one tx |
| HTTP | Review endpoint | Auth, validation, `awaiting_review` guard, 409 on late review |

## Migration / Rollout

Additive only (new tables + one ALTER on `evidence_records`). Rollback: `goose down` of
`00009` + revert `internal/orchestrator`, worker registration, the `CreateEvaluation`
`record_kind` write, and the `internal/ledger` `ComplaintTransition` field addition (the
`omitempty` pointer reverts to always-nil, byte-identical to today, so reverting it is
safe even after evaluation records have been written post-deploy). No existing table or
behavior altered beyond the additive, nullable-column ALTER (existing evaluation rows
backfill `record_kind='evaluation'` and remain valid/queryable exactly as before; the
table stays append-only).

## Open Questions

- [ ] Counsel confirmation of the seeded LFT Art. 74 holiday set (tracked as TODO under docs/regulatory-ruleset.md's "Open legal items to confirm with counsel"; fail-closed ambiguity mitigates interim risk).
- [ ] Product confirmation: seed full holiday table now vs weekends-only v1 (proposal assumes full table).
