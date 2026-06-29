# Vigía — Build Plan (Portfolio-first)

Status: Draft v0.2 (pre-build review applied) · Last updated: 2026-06-28
Companion: [technical-design.md](technical-design.md) · [architecture.md](architecture.md)

> Goal: build a large, genuinely complex, end-to-end system showcasing the full agentic
> distributed-systems stack on a real regulated domain. Selling is deferred.
> Discipline: build in order so **every milestone is demoable on its own**.
>
> **Framing (ADR-09):** the Shadow-Mode core is a deterministic **workflow + LLM-judge**, not an
> autonomous agent. The genuine **agent harness is built in the Copilot phase (M4)**, where the model
> actually chooses actions — that's where the "build the harness" learning goal is realized.

## Cross-cutting requirements (apply across milestones)

- **Prompt caching** on the stable judge prefix (ADR-10) — from the first judge call, not later.
- **Injection boundary** on judge inputs + output-shape validation (ADR-11).
- **Durable correctness** on River: idempotency keys, exactly-once evidence writes in-transaction,
  explicit state enum (ADR-01).
- **OTel GenAI semconv** spans; **golden-set-first (TDD)** for any detector.

## Milestones

### M0 — Foundation & dev environment
- `docker-compose.yml` (Postgres + MinIO/WORM) + `Makefile` + `.env.example`; `internal/config` env load.
- Go module = `github.com/ricardoalt1515/vigia`; **goose** migrations wired; sqlc generating.
- Postgres + RLS + tenant context; **tenant API-key auth** (`internal/auth`); base `core` types (incl.
  `Debtor`, `TenantAPIKey`, normalized `DetectorResultRow`, `PolicyBundleRule`).
- River wired (Postgres queue) + a trivial job to prove the worker runtime.
- **Demoable:** `make dev` boots Postgres+MinIO; seed inserts synthetic interactions for 2 tenants; a
  `psql`/API call confirms cross-tenant reads are blocked by RLS.
- Maps to GitHub issues **#13 (dev env/bootstrap), #14 (auth)**; unblocks #1.

### M1 — Detection core (workflow brain)
- Policy Compiler → versioned immutable `PolicyBundle` (+ `PolicyBundleRule`).
- Deterministic detectors (hours, third-party, protected pop., channel, payment, disclosure presence).
- LLM-judge (tone/threat, disclosure completeness, impersonation): analytic rubric, temp 0, **prompt
  caching**, **injection boundary**, confidence→HITL. **Golden cases authored first (TDD).**
- Golden-set eval runner + CI gate (GitHub Actions, issue **#15**).
- **Demoable:** synthetic transcript → full `Evaluation` (normalized `DetectorResultRow`s); CI blocks a
  broken rubric. Maps to issues #1–#7.

### M2 — Evidence Ledger + Supervision Console (first strong demo)
- Append-only hash-chained `EvidenceRecord` (write-once; in-transaction with state changes); evidence
  export + tamper proof; per-record RFC 3161 timestamp captured.
- Next.js console: read-only dashboards (out-of-hours, threats, score by despacho) + drill-down.
- **Demoable:** dashboard, drill into a flagged call, export tamper-evident evidence; prove tampering is
  detected. Maps to issues #3, #7.

### M3 — Durable orchestration + REDECO Ops (River)
- **River** workflows (NOT Inngest): complaint case with 10-business-day SLA + escalation + HITL
  pause/resume (hardened state machine, ADR-01); monthly REDECO report; despacho penalization registry;
  scheduled ledger verification.
- **Demoable:** open a complaint, watch SLA + escalation, human override resumes durably; generate a
  monthly report. Maps to issues #8, #9.

### M4 — Realtime guardrails + HITL + the agent harness
- The genuine **agent harness** (`internal/harness`): model chooses next action, dispatches tools,
  gated by realtime pre-action guardrails (hard-block/reroute); campaign preflight simulator.
- **Demoable:** an outbound utterance blocked live; a low-confidence case routes to the HITL queue and
  resumes after approval. Maps to issue #10.

### M5 — Voice pipeline
- `Transcriber` adapters (Whisper default; Deepgram/AssemblyAI) + STT eval harness on es-MX audio;
  audio → transcript (chunked) → evaluation → evidence.
- **Demoable:** drop a (synthetic) audio call → full compliance analysis. Maps to issue #11.

### M6 — Observability + Merkle anchoring + polish
- OTel GenAI-semconv traces end-to-end + Langfuse; per-tenant cost/quality; Merkle checkpoints + RFC
  3161 anchoring + daily verify job; demo script + case-study page.
- **Demoable:** trace view ingestion→evidence; ledger checkpoint with timestamp. Maps to issue #12.

## Reaching "almost everything"

Every agentic primitive is covered: tool contracts + injection-safe judge (M1), evals-as-CI (M1),
immutable trace/evidence (M2), durable orchestration + HITL (M3), the real agent loop + guardrails (M4),
sandboxed preflight (M4), voice (M5), observability (M6).

## Deferred (archived until selling is primary)

GTM playbook, interview script, pricing, design-partner outreach, legal sign-off, prod hardening, real
integrations. See PRD §5 and roadmap.md.

## Next action

Build **#13 (dev env/bootstrap)** → **#14 (auth)** → **#1 (thin walking skeleton)**. Specs are in
technical-design.md. Keep each milestone demoable before starting the next.
