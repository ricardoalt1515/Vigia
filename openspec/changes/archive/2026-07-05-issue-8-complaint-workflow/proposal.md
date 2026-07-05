# Proposal: Durable Complaint Workflow with SLA + Human-in-the-Loop

## Intent

REDECO (MX-REDECO-13, Art. 122) requires responding to complaints within 10 business
days. Today there is no durable case lifecycle: `internal/orchestrator` does not exist,
River hosts only a `NoopJob`, and no business-day/SLA logic exists anywhere. Compliance
staff need a case that survives process restarts, tracks its SLA deadline, escalates on
breach, and pauses for human approval without losing or duplicating state. This closes
the gap between the documented design (technical-design.md §5.4, ADR-01) and reality.

## Scope

### In Scope
- `POST /v1/complaints`: idempotent case creation (`idempotency_key` supplied by the
  client), computing the SLA deadline and appending the `open` evidence record in one
  `tenantdb.WithTenantTx`.
- `complaint_cases` durable state machine: `open → awaiting_review → escalated → resolved`.
  v1 deliberately routes **every** non-escalated case through `awaiting_review` — there is
  no auto-resolve path; a human decision (`approve`/`override`) is required on every
  complaint that isn't escalated. This is a scope decision, not an oversight.
- 10-business-day SLA timer + escalation, driven by a River periodic poll job.
- Poll-based HITL resume: console approve/override inserts a `human_reviews` row; the
  poll job resumes the paused case (LISTEN/NOTIFY deferred).
- Idempotency: River `UniqueOpts` keyed on `(complaint_case_id, transition_kind)`; case
  creation idempotency via `UNIQUE (tenant_id, idempotency_key)` + `ON CONFLICT DO NOTHING`.
- Atomic evidence-append + state transition inside one `tenantdb.WithTenantTx`, extending
  the existing per-tenant hash chain (`evidence_records`) rather than a separate table.
- Approval TTL (`review_expires_at`), default **3 business days** (configurable):
  expiry escalates (fail-closed), never auto-approves; the `approve`/`override` CAS
  carries the same temporal guard so a late approve is a deterministic no-op (see
  design.md).
- Static, versioned Mexican statutory-holiday table (LFT Art. 74) + weekends, seeded via
  migration; documented as pending counsel confirmation (see docs/regulatory-ruleset.md
  "Open legal items to confirm with counsel"; MX-REDECO-04A is a style precedent only).

### Out of Scope
- LISTEN/NOTIFY low-latency resume (follow-up optimization).
- Console review/approve UI beyond the write endpoint (P2).
- Dynamic/decree-driven holiday updates; external calendar service.
- Any dependency on issue #7 (merged separately; evaluation-time HITL — distinct concern from the complaint case lifecycle).
- `ComplaintCase.Resolution` and `DespachoPenalty` (canonical fields per
  technical-design.md §4): these are populated by the REDECO report / despacho
  penalization workflow, tracked under issue #9. This change only creates and transitions
  the case; it does not compute or persist a resolution outcome or penalty.

## Capabilities

### New Capabilities
- `complaint-workflow`: durable complaint case lifecycle, SLA timer, escalation,
  human-review pause/resume, idempotent transitions.
- `business-day-calendar`: static versioned holiday table + business-day arithmetic for
  SLA due-date computation.

### Modified Capabilities
None.

## Approach

Exploration Approach 1 (poll-based SLA + poll-based HITL resume). A single River periodic
job is the correctness ground truth: it scans for cases still `open` (every `open` case
requires review in v1, so this scan is what triggers `request_review`), SLA-due and
TTL-expired cases, and new `human_reviews` rows, enqueuing idempotent transition jobs.
`complaint_cases.state` is the sole source of truth (never River job state). Ambiguity in
the holiday calendar resolves toward the earlier deadline (a doubtful day counts as a
business day, so the SLA expires sooner, never later).

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `internal/orchestrator/` | New | River jobs + `ComplaintCase` state machine, SLA/escalate/resume, business-day calendar; poll scans `open` cases, due SLAs, expired TTLs, and unprocessed reviews |
| `internal/ledger/ledger.go` | Modified | Extend `Body` with a trailing `omitempty` `ComplaintTransition` sub-struct so complaint content is bound into the hash (see design.md) |
| `internal/postgres/adapters.go` | Modified | `CreateEvaluation` sets `evidence_records.record_kind='evaluation'` explicitly; `evidenceRowToRecord` read path reconstructs `Body.ComplaintTransition`; new `ComplaintCaseStore` reuses `WithTenantTx` + evidence-append-in-same-tx |
| `db/migrations/00009_complaint_workflow.sql` | New | `complaint_cases` (with `idempotency_key`), `human_reviews`, `business_day_holidays` (tenant_id + RLS); ALTER on `evidence_records` (`record_kind`, nullable FKs, exactly-one-of CHECK, partial unique indexes) |
| `db/queries/*.sql` + sqlc | New | Case CRUD/transition queries (idempotent create, CAS with temporal guard) |
| `cmd/worker/main.go` | Modified | Register real workers with `UniqueOpts` |
| `internal/httpapi/httpapi.go` | Modified | `POST /v1/complaints` (create) + `POST /v1/complaints/{id}/reviews` (human-review) endpoints |
| `docs/technical-design.md` | Modified (doc-sync, at apply time) | Reconcile canonical `ComplaintCase`/`HumanReview` structs with this change's superseding shapes (see design.md Canonical-Model Note) |
| `go.mod` | Modified | Promote River from `// indirect` to direct (`go mod tidy`) |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Holiday table legally incomplete | Med | Static versioned table + fail-closed ambiguity + counsel-confirmation TODO |
| `UniqueOpts` exactly-once unproven (River v0.39.0) | Med | Verify semantics with targeted test before relying on it |
| Greenfield package exceeds 400-line PR budget | High | Chained-PR slicing (final call at sdd-tasks) |
| Poll vs notify testability confusion | Low | Poll = correctness contract; notify explicitly deferred |
| Production pools bypass RLS (owner role) | Med | Enforcement is explicit `tenant_id = $N` predicates in every query, not RLS; RLS + `vigia_app` grants remain as defense-in-depth, verified by an `rls_isolation_test.go`-style test |
| Nullable-FK evidence-ledger ALTER breaks hash binding for complaint records | High | Extend `ledger.Body` with a trailing `omitempty` `ComplaintTransition` sub-struct so complaint content is bound into the hash (see design.md) |

## Rollback Plan

Migration `00009` is additive (new tables + a nullable-column ALTER on
`evidence_records`); roll back via goose down of `00009` and revert the
`internal/orchestrator` package, worker registration, and the `CreateEvaluation`
`record_kind` write. No existing table or behavior is altered beyond the additive ALTER,
so revert is isolated and low-risk.

## Dependencies

- River v0.39.0 (promote to direct dependency).
- Existing `tenantdb.WithTenantTx` + evidence-ledger append pattern (issue #3, merged).

## Success Criteria

- [ ] Opening a complaint starts a durable case with a 10-business-day SLA timer and escalation.
- [ ] `POST /v1/complaints` is idempotent: a repeated request with the same idempotency key returns the existing case, no duplicate SLA timer or evidence row.
- [ ] Case pauses at `awaiting_review` and resumes after console approve/override without lost/duplicated state.
- [ ] Job retries are idempotent: no double evidence writes; evidence append + state transition are atomic.
- [ ] Approval TTL bounds the wait; expiry escalates (fail-closed), never auto-approves — enforced mechanically by the CAS temporal guard, not job ordering.
- [ ] Tenant isolation is enforced by explicit `tenant_id` predicates on every CAS/UPDATE/INSERT, verified additionally by an `rls_isolation_test.go`-style test.
- [ ] Every newly created case reaches `awaiting_review` via the poll job's `open`-case scan, without requiring SLA breach first.
- [ ] Complaint-transition evidence is hash-bound: tampering `complaint_case_id`/`transition_kind`/`human_review_id` on a complaint evidence row is detected by chain verification.

## Proposal question round

Product decisions in this proposal were pre-resolved by the orchestrator (poll-based
approach, static holiday table with counsel-confirmation TODO, fail-closed ambiguity/TTL,
chained-PR delivery). If desired, the open item worth user confirmation before spec is:
whether v1 should ship weekends-only and defer the statutory-holiday table to a follow-up,
versus seeding the full LFT Art. 74 table now (current proposal assumes the latter).
