# Vigía — Collections Compliance Control Plane

> Working codename. A compliance / QA / evidence control plane for debt collection (cobranza)
> in Mexico. Voice-first. REDECO-native. Supervises humans, external collection agencies
> (despachos), and bots against an executable regulatory ruleset; blocks risky actions and
> produces a regulator-ready evidence ledger for CONDUSEF.

**Status:** Pre-build. Direction validated through 3 research rounds; pivoted from "AI collector"
to "compliance control plane". Next step: ship the Phase 1 *Shadow Mode* slice and validate
willingness-to-pay with 2–3 design partners.

## The one-line thesis

> "Recover without opening regulatory risk, with traceability you don't have today."
> The wedge is **not** cheaper collection (already commoditized by Altur, Colektia, Moonflow).
> It is provable, audit-grade compliance on a fresh regulatory tailwind: after the SCJN ruling
> of Jan 15, 2026, **the fine is the financial entity's, not the despacho's.**

## Why this project

It is the convergence of the founder's three interests pointed at a buyer with budget and a
regulatory gun to their head:

- **Agents as distributed systems** → it is a guardrail / control plane.
- **Voice** → voice-first by design (WhatsApp prohibits debt collection).
- **Financial services / portfolios** → it lives in lending / cobranza.

It also forces the full agentic stack (orchestration, tool contracts, evals/CI, HITL,
guardrails, evidence, observability), making it a genuine flagship portfolio project.

## Documentation index

| Doc | Purpose |
|-----|---------|
| [HANDOFF.md](HANDOFF.md) | **Resume here** — session handoff: state, decisions, issues, next step |
| [docs/PRD.md](docs/PRD.md) | Product Requirements — problem, market, ICP, modules, features, scope |
| [docs/regulatory-ruleset.md](docs/regulatory-ruleset.md) | The REDECO/CONDUSEF executable ruleset (product core / moat seed) |
| [docs/architecture.md](docs/architecture.md) | System architecture, agentic-stack mapping, tech stack |
| [docs/technical-design.md](docs/technical-design.md) | Build-ready HOW — verified ADR decisions, data model, module specs |
| [docs/build-plan.md](docs/build-plan.md) | Milestones M0–M6 (portfolio-first), each independently demoable |
| [docs/roadmap.md](docs/roadmap.md) | GTM / validation experiments (deferred — selling is secondary) |

## Local database-backed tests

The local Postgres stack uses the migration owner URL for setup and a restricted
application role URL for RLS isolation checks:

```bash
make dev
make migrate-up
make test-rls
```

Defaults are defined in the Makefile:

- `DATABASE_URL=postgres://vigia:vigia@localhost:5432/vigia?sslmode=disable`
- `APP_DATABASE_URL=postgres://vigia_app:vigia_app@localhost:5432/vigia?sslmode=disable`

`make test-db` exports both URLs and runs the full Go suite against the migrated
local database. The `vigia_app` role is provisioned by the SQL migrations with
no superuser, table ownership, or `BYPASSRLS` privileges.

## Key facts (verified, mid-2026)

- Mexican consumer past-due portfolio (cartera vencida): ~MXN $58,380M in 2025 (record high).
- REDECO complaints Jan–Jun 2025: 27,985 (+52.8% YoY). Top cause: threats/offense/intimidation (36.3%).
- Most-complained sectors: banca múltiple (16,377), SOFOM E.N.R. (6,801), SOFIPO (2,502), SOFOM E.R. (1,776).
- SCJN Amparo en Revisión 323/2025 (resolved Jan 15, 2026): confirmed CONDUSEF's power to sanction
  and the financial entity's duty to register, supervise, respond to complaints, and report monthly.
- Sanctions: 200–2,000 días de salario mínimo ≈ MXN $63,008–$630,080 (at $315.04/day, 2026).
- Allowed contact window: **08:00–21:00** in the debtor's timezone (current; old 07:00–22:00 is obsolete).
- WhatsApp Business policy prohibits debt collection "irrespective of licenses" → voice-first.

> Sources are cited inline in the docs. Items marked INFERENCE are analytical synthesis, not direct quotes.
