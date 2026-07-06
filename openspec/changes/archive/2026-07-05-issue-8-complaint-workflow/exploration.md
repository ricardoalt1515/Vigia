# Exploration: Durable complaint workflow with SLA + HITL (River) — issue #8

## Current State

River is already a dependency (`go.mod`, listed as `// indirect` despite `cmd/worker/main.go` importing it directly — worth a `go mod tidy` check) with its schema already migrated (`db/migrations/00002_river_tables.sql`, goose-managed). The only consumer is a trivial `NoopJob`/`NoopWorker` in `cmd/worker/` — no domain jobs, no `UniqueOpts` usage yet. `internal/orchestrator` (named in the docs as the target home) does not exist — this is greenfield work.

The evidence-ledger append pattern from issue #3 is the template for "append + transition atomically": `internal/postgres/adapters.go`'s `EvaluationStore.CreateEvaluation` wraps the evaluation header insert, detector rows, evidence append (seq/prev_hash under a `ledger_chain_heads` lock), and chain-head update in one `tenantdb.WithTenantTx` call. Issue #8 must replicate this exact shape for `ComplaintCase` transitions.

Business-day SLA calculation (MX-REDECO-13, Art. 122, "10 business days") has no implementation anywhere, and `docs/regulatory-ruleset.md`'s "open legal items" section flags business-day/holiday nuances as unconfirmed with counsel (see MX-REDECO-04A precedent for how this repo documents such gaps).

## Affected Areas

- `internal/orchestrator/` (new) — River jobs + `ComplaintCase` state machine
- `internal/postgres/adapters.go` — new complaint-case store, reusing the `tenantdb.WithTenantTx` pattern
- `db/migrations/0000N_complaint_cases.sql` — `complaint_cases`, `human_reviews`, holiday/business-day reference
- `cmd/worker/` — real workers with `river.UniqueOpts` idempotency keys, replacing/extending `NoopWorker`
- `internal/httpapi/httpapi.go` — new write endpoint for human review insert (the "re-enqueue on insert" trigger)
- `go.mod` — verify River's indirect/direct status

## Approaches

1. **Poll-based SLA + poll-based HITL resume** — River periodic job polls due SLAs and new `human_reviews` rows. Pros: simplest, deterministic, easy idempotency testing. Cons: resume latency bounded by poll interval. Effort: Medium.
2. **LISTEN/NOTIFY-driven resume + periodic SLA job** — matches ADR-01's literal wording. Pros: near-instant resume. Cons: extra correctness surface (reconnect-on-drop), still needs a poll backstop for missed notifies. Effort: High.
3. **Hybrid: poll as correctness guarantee + NOTIFY as latency optimization only.** Effort: Medium-High.

## Recommendation

Approach 1 for the initial PR; approach 3's NOTIFY nudge as an explicit follow-up only if latency becomes a real requirement. Correctness contract regardless of choice: `complaint_cases.state` is the sole source of truth (never River job state); every job idempotency-keyed on `(complaint_case_id, transition_kind)`; evidence append happens inside the same `tenantdb.WithTenantTx` as the state UPDATE; approval TTL expiry escalates (fail-closed), never silently auto-approves.

## Risks

- Business-day/holiday calendar sourcing is unresolved and legally load-bearing — needs an explicit proposal-time decision (static versioned table vs. weekends-only v1 vs. external service).
- River `UniqueOpts` idempotency semantics are unproven in this codebase (zero real usage today).
- `go.mod` River packages marked indirect despite direct imports — check before adding more usage.
- Fully greenfield package (`internal/orchestrator`) — recommend chained-PR slicing (migration+model → case-open+SLA job → HITL pause/resume → HTTP endpoint) to stay under the 400-line review budget.
- Issue #7 (unmerged, other branches) has a superficially similar HITL concept (evaluation-time fold) — do not conflate with or depend on it; issue #8's HITL is workflow-time/durable, not evaluation-time.

## Ready for Proposal

Yes. Open decision carried into proposal: how to source the Mexican business-day/holiday calendar for the SLA calculation.
