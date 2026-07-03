# Vigía — Session Handoff

> Read this first to resume work in a new session without losing context.
> Last updated: 2026-06-29 · Status: Issue #1 walking skeleton complete (seed + River worker + Next.js console).

## Walking skeleton demo — end-to-end run order (Issue #1)

Run these in order to see the three seeded interactions in the browser:

```bash
# 1. Start Postgres + MinIO
make dev

# 2. Apply all goose migrations (including River tables from 00002_river_tables.sql)
make migrate-up

# 3. Seed demo tenant, debtor, and six es-MX interactions (three original
#    fixtures, one out-of-hours demo, and — issue #4 — one threatening and
#    one neutral synthetic transcript that exercise the MX-REDECO-05
#    tone/threat judge).
#    Copy the printed tenant_api_key=<plaintext> value — you need it in step 7.
#
#    Judge env vars (issue #4), all optional — defaults work with zero setup:
#      JUDGE_MODE=fake (default, no key needed) | anthropic
#      ANTHROPIC_API_KEY=<key>            (required only when JUDGE_MODE=anthropic)
#      JUDGE_MODEL_ID=<model id>          (default: pinned claude-haiku-4-5-20251001)
#      JUDGE_HITL_CONFIDENCE_THRESHOLD=<0..1>  (default: 0.75)
make seed-dev

# 4. Start the Go API server (set DATABASE_URL + APP_DATABASE_URL if not in shell)
go run ./cmd/api

# 5. (Optional) Start the River worker; it enqueues and drains one no-op job then idles.
#    Ctrl-C when done.
make worker

# 5b. (Issue #3) Every seeded evaluation now appends a hash-chained evidence_records
#     row. Export a seeded interaction's self-contained evidence package (requires the
#     API server from step 4 running and the tenant_api_key from step 3):
curl -H "Authorization: Bearer <tenant_api_key>" \
  http://localhost:8080/v1/interactions/<interaction-id>/evidence

#     Verify the demo tenant's whole chain is intact from the command line:
go run ./cmd/ledger-verify -tenant-id <demo-tenant-id>

# 6. Install Next.js console dependencies (first time only)
make console-install

# 7. Start the console dev server.
#    Create apps/console/.env.local with the key from step 3:
#      VIGIA_API_KEY=<plaintext key>
#      VIGIA_API_BASE_URL=http://localhost:8080
make console-dev

# 8. Open http://localhost:3000
#    The interactions list page renders the seeded rows.
#    Wrong / missing key → API returns 401 → page shows no rows (RLS proof).
#    (Issue #4) The threatening seed transcript's row shows a red THREAT
#    badge and an amber HITL badge; the neutral seed transcript's row shows
#    neither. Use the "Show only flagged" toggle to filter to those rows.
```

**Cleanup:** `make down` stops Postgres + MinIO. `.next` and `node_modules` are gitignored.


## How to resume

1. Read this file.
2. Read docs in order: [README.md](README.md) → [docs/PRD.md](docs/PRD.md) →
   [docs/technical-design.md](docs/technical-design.md) → [docs/build-plan.md](docs/build-plan.md) →
   [docs/regulatory-ruleset.md](docs/regulatory-ruleset.md) → [docs/architecture.md](docs/architecture.md) →
   [docs/frontend-design.md](docs/frontend-design.md).
3. `gh issue list -R ricardoalt1515/Vigia`. Start with **#13 (dev env)**, then **#16 (Agent Harness Lab)**,
   then **#14 (auth)**, then **#1**.
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
- **Learn Go + build the agent harness himself** (sandboxed early in #16; authority-bearing in Copilot/M4).
- **Mexico/LATAM-first**, internationalize later.
- **Implement the full agentic distributed-systems stack** on a real regulated domain.

## Pre-build review outcomes (2026-06-28)

Three adversarial subagents reviewed the plan/stack/architecture vs June-2026 best practice (one was
rate-limited; its scope was covered by the others). Verdict: the **compliance spine is modern and a
strength** (deterministic + LLM-judge + evidence ledger + golden-set CI = 2026 best practice). Applied
fixes:

- **Reframe (ADR-09): workflow-first for compliance authority; agent lab early.** The Shadow-Mode core is a
  deterministic workflow + LLM-judge, not an autonomous loop. A sandboxed **Agent Harness Lab** is built early
  (#16) for portfolio/learning; authority-bearing agent behavior remains in Copilot (M4 / #10).
- **River, hardened (ADR-01).** Kept River for the learning goal, but the durable state machine is now a
  first-class design: explicit `ComplaintCase.State` enum, idempotency keys, exactly-once evidence writes
  in-transaction, HITL pause via `awaiting_review` + re-enqueue. DBOS is the documented off-ramp.
- **Added (ADR-10/11/12/13):** prompt caching on the judge prefix; LLM-judge injection boundary +
  output-shape validation; per-tenant API-key auth → RLS; MCP as an external AI-client integration surface,
  not the internal harness runtime.
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
| **Workflow-first authority + early Agent Harness Lab** (ADR-09) | Keep compliance decisions auditable while showing domain-specific agent composition, tool permissions, and event logs early |
| **Remote MCP as external integration** | June 2026 pattern: expose selected tenant-scoped Vigía capabilities to AI clients without making MCP the internal harness runtime |
| **Separate Judge and Harness model ports** | Judge needs deterministic, rubric-versioned evaluation; Harness needs sandboxed agent steps, tools, budgets, validation, and event logs |
| **Go + River hardened** (ADR-01) | Learn Go + build the durable machine; River = transparent Postgres substrate; DBOS = off-ramp |
| **Deterministic-first + LLM-judge** (ADR-03) | 2026 "Hybrid Norm"; temp 0, analytic rubrics, κ-validated, versioned |
| **Evidence ledger hash-chain + Merkle + RFC3161** (ADR-02) | Audit-grade must be defensible |
| **Prompt caching (10) + injection boundary (11)** | Cost + safety; non-optional for volume LLM + adversarial transcripts |
| sqlc/pgx/RLS, goose, MinIO WORM, UUID v7, tenant API-key auth | Build-readiness + control |

Full ADR-style decisions with cited sources: [docs/technical-design.md §1](docs/technical-design.md). Dedicated ADR files live in [docs/adr/](docs/adr/), including Remote MCP and separate Judge/Harness model ports.

## The open business risk (validate when selling becomes primary)

Compliance may be a **veto criterion, not a budget driver** (INFERENCE — unvalidated). Killer question in
[docs/roadmap.md](docs/roadmap.md): *"Would you pay this quarter for the evidence/guardrail layer even if
it doesn't raise recovery?"*

## Repo state

- Remote: `git@github.com:ricardoalt1515/Vigia.git` (gh authed as `ricardoalt1515`).
- Module path now `github.com/ricardoalt1515/vigia`.
- Files present: docs incl. `docs/frontend-design.md`, `go.mod`, `sqlc.yaml`, `docker-compose.yml`,
  `Makefile`, `.env.example`, `.gitignore`, `.gitkeep` in each dir. **No application Go code yet** (by design).
- **Committed (local, not pushed):** `29d91d4 chore: bootstrap Go scaffold and planning docs`.
  Push only when asked. No application Go code committed yet (by design).

## Issues (GitHub, dependency-ordered)

Label `ready` = triaged for an AFK agent. Start at **#13**.

```
#13 dev env / bootstrap        (UNBLOCKED — start here)
#16 agent harness lab epic     ← #13        (sandboxed/read-only/draft-only; uses Synthetic Tenant Context only)
  #18 runtime skeleton + invariant tests
  #19 tool contracts + synthetic Case fixture
  #20 deterministic Case orchestrator + Domain Agents
  #21 demo CLI + Case Brief outputs
  #22 Bedrock Claude opt-in provider
#14 tenant auth (API key→RLS)  ← #13        (required before #17 external MCP)
#17 remote MCP server          ← #16/#18-#22, #14 (tenant-scoped external integration; not internal runtime)
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
#10 realtime guardrails + authority-bearing harness ← #16, #6, #4
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
`internal/config`, sqlc generating, core types incl. `Debtor`. Then implement the **#16 Agent Harness Lab** via
slices **#18 → #19 → #20 → #21 → #22** using Synthetic Tenant Context only, then **#14 (auth)**, then **#1**.
Issue **#17 (Remote MCP Server)** is ready after the #16 slices and #14; its first slice must read synthetic Case
Brief artifacts through a tenant-aware index filtered by #14 tenant context, never through arbitrary file paths.
