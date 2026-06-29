# Vigía — Session Handoff

> Read this first to resume work in a new session without losing context.
> Last updated: 2026-06-28 · Status: pre-code; planning + pre-build review complete; issues published.

## How to resume

1. Read this file.
2. Read docs in order: [README.md](README.md) → [docs/PRD.md](docs/PRD.md) →
   [docs/technical-design.md](docs/technical-design.md) → [docs/build-plan.md](docs/build-plan.md) →
   [docs/regulatory-ruleset.md](docs/regulatory-ruleset.md) → [docs/architecture.md](docs/architecture.md).
3. `gh issue list -R ricardoalt1515/Vigia`. Start with **#13 (dev env)**, then **#14 (auth)**, then **#1**.
4. Next concrete action: build **issue #13 (dev environment & bootstrap)** — first task of the build.

## What Vigía is (one paragraph)

A **Collections Compliance Control Plane** for Mexico: software on top of any collection operation
(in-house agents, despachos, bots) that evaluates every debtor interaction against an executable
REDECO/CONDUSEF ruleset, blocks/escalates risky actions, and produces an immutable, regulator-ready
**evidence ledger**. Starts in **Shadow Mode** (listen, score, document; no sending). Wedge:
*"recover without regulatory risk + traceability you don't have"* — riding the 2026 CONDUSEF/SCJN
crackdown where **the fine is now the lender's, not the despacho's.** Voice-first (WhatsApp prohibits
debt collection).

## Objective function (IMPORTANT — it evolved)

- **Portfolio-first.** Building a big, complex flagship is the goal. Selling is **secondary/deferred**.
  Do NOT push "sell/validate first".
- **Learn Go + build the agent harness himself** (realized in the Copilot phase — see reframe below).
- **Mexico/LATAM-first**, internationalize later.
- **Implement the full agentic distributed-systems stack** on a real regulated domain.

## Pre-build review outcomes (2026-06-28)

Three adversarial subagents reviewed the plan/stack/architecture vs June-2026 best practice (one was
rate-limited; its scope was covered by the others). Verdict: the **compliance spine is modern and a
strength** (deterministic + LLM-judge + evidence ledger + golden-set CI = 2026 best practice). Applied
fixes:

- **Reframe (ADR-09): workflow-first, not "agent".** The Shadow-Mode core is a deterministic workflow +
  LLM-judge, not an autonomous loop. The genuine **agent harness is built in the Copilot phase (M4 / #10).**
- **River, hardened (ADR-01).** Kept River for the learning goal, but the durable state machine is now a
  first-class design: explicit `ComplaintCase.State` enum, idempotency keys, exactly-once evidence writes
  in-transaction, HITL pause via `awaiting_review` + re-enqueue. DBOS is the documented off-ramp.
- **Added (ADR-10/11/12/13):** prompt caching on the judge prefix; LLM-judge injection boundary +
  output-shape validation; per-tenant API-key auth → RLS; MCP only as a learning artifact.
- **Data model fixes:** added `Debtor` (needed by REDECO 01/07/11/16), `TenantAPIKey`, `PolicyBundleRule`
  join, normalized `DetectorResultRow` (was JSONB), `ComplaintCase` state machine; UUID v7; created/updated_at.
- **Build-readiness:** `go.mod` renamed to `github.com/ricardoalt1515/vigia`; `.gitkeep` so the scaffold
  survives clone; `docker-compose.yml` (Postgres + MinIO/WORM) + `Makefile` + `.env.example`; **goose**
  migrations; `internal/config` + `internal/auth`; CI pipeline issue.
- **Tooling locked:** goose (migrations), MinIO/S3 Object-Lock (WORM audio), digitorus/timestamp + free
  RFC 3161 TSA, OTel GenAI semantic conventions, transcript chunking, golden-set-first (TDD).

## Key decisions (and why)

| Decision | Why |
|---|---|
| Pivot → compliance/QA/evidence control plane | Category validated/crowded (Altur, Colektia, Moonflow); gap is governance + evidence |
| **Workflow-first; agent harness at Copilot (M4)** (ADR-09) | 2026 best practice: workflows for fixed/auditable subtasks; don't over-build agent machinery Shadow Mode never uses |
| **Go + River hardened** (ADR-01) | Learn Go + build the durable machine; River = transparent Postgres substrate; DBOS = off-ramp |
| **Deterministic-first + LLM-judge** (ADR-03) | 2026 "Hybrid Norm"; temp 0, analytic rubrics, κ-validated, versioned |
| **Evidence ledger hash-chain + Merkle + RFC3161** (ADR-02) | Audit-grade must be defensible |
| **Prompt caching (10) + injection boundary (11)** | Cost + safety; non-optional for volume LLM + adversarial transcripts |
| sqlc/pgx/RLS, goose, MinIO WORM, UUID v7, tenant API-key auth | Build-readiness + control |

Full ADRs with cited sources: [docs/technical-design.md §1](docs/technical-design.md).

## The open business risk (validate when selling becomes primary)

Compliance may be a **veto criterion, not a budget driver** (INFERENCE — unvalidated). Killer question in
[docs/roadmap.md](docs/roadmap.md): *"Would you pay this quarter for the evidence/guardrail layer even if
it doesn't raise recovery?"*

## Repo state

- Remote: `git@github.com:ricardoalt1515/Vigia.git` (gh authed as `ricardoalt1515`).
- Module path now `github.com/ricardoalt1515/vigia`.
- Files present: docs, `go.mod`, `sqlc.yaml`, `docker-compose.yml`, `Makefile`, `.env.example`,
  `.gitignore`, `.gitkeep` in each dir. **No application Go code yet** (by design).
- **Committed (local, not pushed):** `1f54484 chore: bootstrap Go scaffold and planning docs`.
  Push only when asked. No application Go code committed yet (by design).

## Issues (GitHub, dependency-ordered)

Label `ready` = triaged for an AFK agent. Start at **#13**.

```
#13 dev env / bootstrap        (UNBLOCKED — start here)
#14 tenant auth (API key→RLS)  ← #13
#1  thin walking skeleton      ← #13, #14   (re-scoped: one interaction → API → console)
#2  hours detector             ← #1
#3  evidence ledger            ← #2
#4  LLM-judge tone/threat       ← #2, #3   (+ prompt caching + injection boundary)
#15 CI pipeline                ← #13
#5  golden-set + CI gate       ← #4, #15   (golden cases authored first — TDD)
#6  policy bundle versioning    ← #4
#7  remaining detectors + dash  ← #6
#8  durable complaint + HITL    ← #3        (hardened River state machine)
#9  monthly REDECO report       ← #8, #7
#10 realtime guardrails + agent harness ← #6, #4
#11 voice pipeline              ← #7
#12 observability + Merkle anchor ← #3
```

## Working preferences

- Reply in **Spanish**; mentor/senior-architect tone, warm-direct, concise by default.
- **Verify load-bearing claims** before asserting (web search); separate VERIFIED vs INFERENCE.
- The **user drives decisions**; offer tradeoffs, recommend, don't railroad.
- Artifacts default to **English**; regulatory terms stay in Spanish.
- Don't write application code until asked. Don't commit/push unless asked.

## Immediate next step

Build **#13 (dev env/bootstrap)**: docker-compose (Postgres+MinIO), Makefile, .env.example, goose wired,
`internal/config`, sqlc generating, core types incl. `Debtor`. Then **#14 (auth)**, then **#1**.
