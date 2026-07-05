# Complaint Workflow Specification

## Purpose

Durable, tenant-isolated complaint case lifecycle for REDECO (MX-REDECO-13, Art. 122).
A case tracks a 10-business-day SLA, escalates on breach or approval-TTL expiry, and
pauses for human review without losing or duplicating state across process restarts
or job retries. `complaint_cases.state` is the sole source of truth; River job/queue
state is never authoritative. The periodic poll job is what actually drives every
`open` case into human review — nothing else triggers that transition.

## Requirements

### Requirement: Case State Machine

The system MUST model each complaint as a durable `complaint_cases` row with state
`open → awaiting_review → escalated → resolved`. Transitions MUST be persisted before
any dependent action is considered complete.

#### Scenario: Case opens with SLA timer

- GIVEN a new complaint is registered for a tenant
- WHEN the case is created
- THEN a `complaint_cases` row is inserted in state `open` with an `sla_due_at` set to
  10 business days from creation
- AND the row is scoped to the tenant via the `tenant_id` predicate on the insert
  (RLS is additionally enabled as defense-in-depth)

### Requirement: Idempotent Case Creation

The system MUST accept a client-supplied idempotency key on case creation and enforce
`UNIQUE (tenant_id, idempotency_key)` so a repeated creation request never produces a
duplicate case, SLA timer, or evidence row.

#### Scenario: Repeated creation request returns the existing case

- GIVEN a complaint case was already created for a tenant with idempotency key `K`
- WHEN a new `POST /v1/complaints` request arrives for the same tenant with the same
  idempotency key `K`
- THEN no new `complaint_cases` row is inserted
- AND the endpoint returns the existing case
- AND no duplicate SLA computation or evidence record is produced

#### Scenario: Case pauses for human review

- GIVEN a case in state `open` (every `open` case requires review in v1 — there is no
  policy gate that skips it)
- WHEN the periodic poll job's `open`-case scan finds the case and enqueues
  `request_review`, and that transition commits
- THEN the case state and a `review_expires_at` approval TTL of **3 business days**
  from the transition (the configured default, computed via the same business-day
  calculator as the SLA) are persisted atomically
- AND no auto-approval logic exists that can move the case forward without a
  `human_reviews` row

#### Scenario: Process restart mid-transition loses nothing

- GIVEN a transition job crashes or the worker process restarts after writing the
  state update but before acknowledging the job
- WHEN the worker restarts and the poll job runs again
- THEN the case state reflects the last successfully committed transaction exactly
  once, with no partial or lost writes

### Requirement: Poll-Triggered Review Request

The system MUST have the periodic poll job scan for cases in state `open` and enqueue
a `request_review` transition for each one found. This is the only trigger for the
`open → awaiting_review` transition: case creation MUST NOT enqueue `request_review`
directly, and no other code path may do so.

#### Scenario: Open case is picked up by the poll and reaches awaiting_review

- GIVEN a case was created and is still in state `open`
- WHEN the next poll cycle runs
- THEN the poll job enqueues a `request_review` transition for that case
- AND once that transition commits, the case is in state `awaiting_review` with a
  `review_expires_at` TTL set
- AND if the poll job had not scanned `open` cases, the case would remain in `open`
  indefinitely (until SLA breach), never reaching human review

### Requirement: SLA Poll and Escalation

The system MUST run a River periodic poll job that scans for cases whose
`sla_due_at` or `review_expires_at` has passed and enqueues idempotent transition jobs.

#### Scenario: SLA breach escalates

- GIVEN a case in state `open` or `awaiting_review` with `sla_due_at` in the past
- WHEN the poll job runs
- THEN the case transitions to `escalated`
- AND the escalation is recorded with a reference to the breached deadline

#### Scenario: Approval TTL expiry escalates, never auto-approves

- GIVEN a case in state `awaiting_review` with `review_expires_at` in the past and no
  `human_reviews` row inserted before expiry
- WHEN the poll job runs
- THEN the case transitions to `escalated`
- AND the case MUST NOT transition to `resolved` as a result of TTL expiry alone

#### Scenario: Approval submitted after TTL expiry is rejected

- GIVEN a case has already transitioned to `escalated` due to TTL expiry
- WHEN a `human_reviews` row is subsequently inserted approving the case
- THEN the poll job MUST NOT move the case back to `awaiting_review` or `resolved`
  from the late approval alone, because the resulting `approve` transition's CAS
  predicate (`state='awaiting_review' AND review_expires_at > now()`) matches 0 rows
  and no-ops
- AND the late review is recorded for audit but does not override the escalation
- AND if the client submits the review via the HTTP endpoint after escalation has
  already happened, the endpoint returns `409 Conflict` instead of accepting the review

### Requirement: HITL Resume via Poll

The system MUST resume a paused case by polling for new `human_reviews` rows rather
than relying on LISTEN/NOTIFY for correctness.

#### Scenario: Approve resumes the case

- GIVEN a case in state `awaiting_review` with `review_expires_at` in the future
- WHEN a `human_reviews` row with an approve decision is inserted before expiry
- THEN the next poll cycle transitions the case toward `resolved`

#### Scenario: Duplicate human_reviews inserts resolve deterministically

- GIVEN a case in state `awaiting_review`
- WHEN two `human_reviews` rows are inserted for the same case before the poll job
  next runs (e.g., a double-submit or retried client request)
- THEN the poll job applies exactly one resulting state transition
- AND the committing transaction marks **all** unprocessed `human_reviews` rows for
  the case as processed/superseded, with the evidence record referencing the single
  winning row
- AND processing the second row MUST NOT re-append evidence or re-trigger a second
  transition, and the poll job MUST NOT re-discover it as unprocessed on a later cycle

#### Scenario: Approve and override enqueued for the same case race deterministically per-attempt

- GIVEN a case in state `awaiting_review` has both an `approve` and an `override`
  transition enqueued (e.g. from two different `human_reviews` rows)
- WHEN both transition jobs run
- THEN exactly one of them commits (whichever CAS matches first), and the other's CAS
  matches 0 rows and no-ops
- AND which decision wins is accepted as race-dependent, not deterministic on
  submission order — `UniqueOpts` on `(complaint_case_id, transition_kind)` does not
  dedupe across the two different transition kinds

#### Scenario: A late human_reviews row stays unprocessed when TTL wins

- GIVEN a case escalates due to `ttl_expired` before a pending `approve`/`override`
  transition's CAS commits
- WHEN the late transition's CAS no-ops
- THEN the `human_reviews` row backing that late transition is never bulk-marked
  processed/superseded (only a committing transition performs that bookkeeping)
- AND this is an accepted limitation: the row remains `unprocessed` permanently unless
  a future follow-up adds a sweep to close out unprocessed reviews on cases that
  reached a terminal state via a different transition

### Requirement: Idempotent Transitions

The system MUST key every transition job with River `UniqueOpts` on
`(complaint_case_id, transition_kind)` so retried or duplicate jobs cannot apply the
same transition twice.

#### Scenario: Retried job does not double-append evidence

- GIVEN a transition job has already committed its state change and evidence append
- WHEN the same job is retried (e.g., after a transient worker crash before
  acknowledgment)
- THEN the retry MUST NOT insert a duplicate evidence record
- AND the case state MUST NOT change again as a result of the retry

#### Scenario: Concurrent poll runs do not double-transition

- GIVEN two poll job executions overlap due to a scheduling race
- WHEN both attempt to enqueue a transition for the same
  `(complaint_case_id, transition_kind)`
- THEN `UniqueOpts` MUST prevent the second job from being enqueued or executed
  redundantly

### Requirement: Atomic Evidence Append with State Transition

The system MUST append the evidence record and update `complaint_cases.state` inside
a single `tenantdb.WithTenantTx` call. Partial application (evidence written without
the state change, or vice versa) MUST NOT be observable. Complaint-transition evidence
MUST extend the same per-tenant hash chain used by evaluation evidence
(`evidence_records`), discriminated by a `record_kind` column, rather than a separate
chain or table.

#### Scenario: Evidence append and transition commit together

- GIVEN a case transition is triggered (escalation, resume, or resolution)
- WHEN the transaction commits
- THEN both the evidence row and the `complaint_cases.state` update are visible
  together
- AND if the transaction fails, neither the evidence row nor the state change is
  persisted

#### Scenario: Complaint and evaluation evidence share one hash chain

- GIVEN a tenant has both evaluation evidence records and complaint-transition
  evidence records
- WHEN either kind of record is appended
- THEN it is inserted into the same `evidence_records` table with `record_kind` set
  to `'evaluation'` or `'complaint_transition'` respectively, continuing the same
  per-tenant `seq`/`prev_hash` chain
- AND a CHECK constraint enforces exactly one of `(evaluation_id, interaction_event_id)`
  or `complaint_case_id` is set, consistent with the record's `record_kind` — an
  `evaluation` row requires both `evaluation_id` and `interaction_event_id` non-NULL and
  `complaint_case_id` NULL; a `complaint_transition` row requires the reverse
- AND at most one evaluation record exists per `(tenant_id, evaluation_id)` and at
  most one complaint-transition record exists per
  `(tenant_id, complaint_case_id, transition_kind)`, each enforced by a partial
  unique index scoped to its `record_kind`

#### Scenario: Complaint evidence hash binds the transition content

- GIVEN a `complaint_transition` evidence record was appended with a given
  `complaint_case_id`, `transition_kind`, `from_state`, `to_state`, and (when
  applicable) `human_review_id`
- WHEN any of those values is modified after the fact (directly in storage, bypassing
  the append-only trigger) and chain verification is re-run
- THEN the record's stored hash no longer matches the recomputed hash of its (possibly
  tampered) content, because those fields are part of the hashed `ledger.Body` via the
  `ComplaintTransition` sub-object
- AND verification reports the tamper (hash mismatch), exactly as it does today for a
  tampered evaluation field such as `overall_outcome`

### Requirement: Tenant Isolation

The system MUST enforce tenant isolation on `complaint_cases` and `human_reviews`
so no tenant can read or write another tenant's case data. Because the production
`cmd/api`/`cmd/worker` pools connect via the owner role (which bypasses row-level
security), the primary enforcement mechanism is an explicit `tenant_id = $N` predicate
in every CAS/UPDATE/INSERT/SELECT against these tables. Row-level security policies
and `vigia_app` grants remain enabled on both tables as defense-in-depth, and are
verified independently by a restricted-role test in the style of
`rls_isolation_test.go`.

#### Scenario: Cross-tenant read is blocked by app-level predicate

- GIVEN complaint cases exist for tenant A and tenant B
- WHEN a query for tenant A executes with an explicit `tenant_id = A` predicate
  (the enforced path, run on the owner pool)
- THEN only tenant A's `complaint_cases` and `human_reviews` rows are returned

#### Scenario: Cross-tenant write is blocked by app-level predicate

- GIVEN a case/transition targets tenant A
- WHEN an insert, update, or CAS statement includes `tenant_id = A` in its predicate
  and the target row belongs to tenant B
- THEN the statement affects 0 rows and the write is rejected

#### Scenario: Restricted role is blocked as defense-in-depth

- GIVEN complaint cases exist for tenant A and tenant B
- WHEN a query executes under the restricted `vigia_app` role scoped to tenant A's
  session context (`app.tenant_id`)
- THEN only tenant A's `complaint_cases` and `human_reviews` rows are visible, and
  this is verified by a `rls_isolation_test.go`-style test on `vigia_app`
