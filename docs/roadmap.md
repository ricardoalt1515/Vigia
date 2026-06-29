# Vigía — Roadmap & Validation Plan

Status: Draft v0.1 · Last updated: 2026-06-28
Companion: [PRD.md](PRD.md) · [architecture.md](architecture.md)

---

## Guiding principle

Build and sell in parallel. The founder's biggest risk is **avoiding sales**, not technical
execution. Validate willingness-to-pay with real buyers while building the Shadow Mode slice.
Do not hide inside a "perfect SaaS". Sell the diagnosis/pilot.

## Phased product roadmap

### Phase 1 — Shadow Mode (compliance & QA over existing operations)
You do not collect. You listen, score, and produce evidence.
- Batch ingestion of existing call recordings → `InteractionEvent`.
- Policy Compiler with REDECO ruleset (deterministic + LLM-judge detectors).
- Evidence Ledger (hash-chain + WORM audio) and per-complaint evidence export.
- Read-only Supervision Console: out-of-hours, threats/intimidation, score by despacho.
- Golden-set regression as a CI gate.
- **Goal:** prove Vigía catches violations manual QA misses; reduce manual review; get a paid pilot.

### Phase 2 — Copilot
- Real-time streaming guardrails: pre-flight utterances, hard-block/reroute risky actions.
- Campaign preflight simulator (scripts/templates/sequences vs ruleset before launch).
- REDECO Ops: complaint SLA engine, despacho penalization registry, monthly export.
- CRM/core connectors; SMS channel; payment resolution to creditor-only accounts.
- Human keeps final decision on sensitive cases.

### Phase 3 — Partial autonomy
- Outbound voice automation for early-stage delinquency and simple reminders, with hard rules,
  handoff, and severe case restriction.

### Phase 4 — Expansion
- REDECO API submission integration; processors; deeper core integrations.
- Per-despacho benchmarking dashboard; benchmarks by complaint cause, contact rate, safe-PTP rate.
- Multi-country policy packs (extend the ruleset beyond Mexico).

## Defensibility (what to accumulate)

The moat is **not** the LLM, voice, WhatsApp, or Twilio (all absorbable). It appears only when three
hard-to-copy assets exist together:
1. A living, MX-first ruleset with real operational depth.
2. A **data flywheel** of conversations, outcomes, complaints, overrides, and resolutions labeled to
   the REDECO catalog.
3. Sticky integration with Mexican cores, REDECO ops, evidence, and compliance workflows the legal
   team already uses.

Until that flywheel exists, absorption risk is high. Prioritize getting real, labeled data.

## The 5 cheap validation experiments (before scaling code)

| Experiment | What you do | Positive signal | Kill signal |
|------------|-------------|-----------------|-------------|
| Interview set | 10 interviews: 4 SOFOM E.N.R., 3 fintech/BNPL, 2 despachos, 1 lawyer/compliance | They can't easily prove per-interaction compliance; QA is manual/incomplete | "Compliance matters, but we wouldn't buy new software for it" |
| Shadow-score 100 calls | Take anonymized audio/transcripts; return REDECO score + risks + evidence | You surface real, actionable failures they recognize | Findings change nothing operationally |
| Dashboard smoke test | Mock panel: guardrails, hours calendar, score by despacho, evidence pack, monthly REDECO report | Buyer asks to pilot with real data | "Nice but not a priority" |
| Willingness-to-pay test | Offer 8 weeks of "observability + scoring + REDECO pack" without full autonomy | They accept paying to pilot the control layer | Only accept free or as a feature of another vendor |
| Channel stress test | Internal legal/policy review on WhatsApp + voice/SMS demo | Buyer accepts voice-first v1, doesn't require WhatsApp | "Without WhatsApp it's useless" → kills initial shape |

### The single killer question
> "If tomorrow I showed you, per conversation, which contacts violated hours, tone, third-party,
> disclosure, or payment routing — and handed you the evidence package to answer REDECO — would you
> pay for it this quarter even if it does not increase recovery in the pilot?"

Dominant **no** → compliance-first wedge is overstated; reframe toward QA/recovery, or move to the
plan-B project (Mexican accounting/CFDI ops).

## Where to find design partners

ASOFOM (200+ members; directory 245+ lenders), AMFE (50+), FinTech México (180+),
ASOFOM National Convention 2026 (900+ leaders). Lead with the diagnosis, not the platform.

## Immediate next steps

1. Lock the Phase 1 Shadow Mode MVP scope (in/out, `InteractionEvent` schema, initial deterministic detectors).
2. Draft the 10-interview script around the killer question.
3. Build the batch connector + deterministic detectors (hours, third-party, channel, payment routing) first.
4. Line up 1–2 design partners from ASOFOM/AMFE/FinTech México for a shadow-score on real recordings.
