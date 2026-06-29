# Vigía — Product Requirements Document (PRD)

Status: Draft v0.1 · Owner: founder · Last updated: 2026-06-28
Companion docs: [regulatory-ruleset.md](regulatory-ruleset.md) · [architecture.md](architecture.md) · [roadmap.md](roadmap.md)

---

## 1. Summary

Vigía is a **Collections Compliance Control Plane** for the Mexican market: a software layer that
sits on top of any collection operation (in-house human agents, external despachos, or AI bots),
evaluates every debtor interaction against an executable REDECO/CONDUSEF ruleset, blocks or
escalates risky actions, and produces an immutable, regulator-ready evidence ledger.

It does **not** start as another autonomous collector. It starts as a **supervision / evidence
layer** ("Shadow Mode") that listens, scores, and documents — sidestepping channel-policy risk
and proving value before any outbound automation is built.

## 2. Problem

Collection in Mexico is mostly manual (phone-first; ~8 of 10 debts collected by phone) and
reputationally toxic. The 2026 regulatory shift turned a soft reputational problem into a hard,
quantified financial-liability problem:

- **Volume of harm is large and rising.** REDECO complaints Jan–Jun 2025: 27,985 (+52.8% YoY);
  the single most frequent cause is *amenazar, ofender o intimidar* (36.3%).
- **Liability moved to the lender.** After SCJN Amparo en Revisión 323/2025 (Jan 15, 2026), the
  financial entity — not the despacho — is responsible for registering, supervising, responding to
  complaints, and reporting monthly. **If it fails, the fine is the lender's.**
- **The fine is now material.** 200–2,000 días de salario mínimo ≈ MXN $63,008–$630,080.
- **Lenders cannot currently prove compliance per interaction.** QA is manual and incomplete.
  They cannot answer in minutes: *How many out-of-hours contacts did we make? Which conversations
  crossed into threats/intimidation? Which despachos repeat-offend on REDECO causes? Which
  promise-to-pay came from a risky flow?*

INFERENCE: Compliance functions today as a **veto/selection criterion**, not necessarily as the
primary budget line. The product must therefore be sold as *"recover without regulatory risk +
traceability you don't have"*, not as *"compliance for its own sake."* This is the #1 open risk
(see §11) and the thing Shadow Mode is designed to test cheaply.

## 3. Market & competition

The category is **validated** (outcome/automation), so this is a deliberate fast-follower, not blue ocean.

| Player | What it does | Compliance depth (public) | Gap we exploit |
|--------|--------------|---------------------------|----------------|
| Altur (YC S25) | Autonomous voice/text collection bots, dialer | Claims "compliance in every interaction"; no public REDECO catalog/export | No public audit-grade evidence layer |
| Colektia (~$9M) | "AI collections infrastructure", segmentation, recovery | Content on MX collection law; product sells ROI/automation | No pre-action guardrails / regulator export |
| Moonflow ($249–$999+/mo) | Collections SaaS, multichannel | Customer-experience focus, not REDECO/audit | No MX regulatory evidence engine |
| ContactShip / Dapta / Recaudo | Generalist voice AI adapted to collections | Analytics/monitoring only | Not collections-native, no MX compliance core |
| Acendes/Presti, SIAC, Sysde SAF | SOFOM core/LMS (origination, portfolio, PLD, reports) | Strong on core/regulatory reporting, weak on omnichannel AI | Integration targets, not front-end competitors |

**The gap (VERIFIED + INFERENCE):** none of the leaders publicly ship an explicit, executable
regulatory + auditable layer for Mexico — operational mapping to the REDECO catalog, immutable
evidence, campaign preflight, and CONDUSEF-ready export. They sell recovery, cost, and scale.
The gap is **governance + evidence**, not "omnichannel collector".

## 4. Positioning & wedge

- **Category:** Collections Compliance Control Plane (supervision + evidence + guardrails).
- **Wedge:** "Collect without getting fined" on the fresh CONDUSEF/SCJN tailwind.
- **Sales line:** *"The fine is now yours, not the despacho's. Vigía proves, per interaction, that
  you stayed within REDECO — and hands you the evidence package to answer CONDUSEF."*
- **Entry mode:** Shadow Mode (score existing recordings; no sending) → Copilot → Partial autonomy.
- **Channel stance:** Voice-first. SMS as commodity reminders. **WhatsApp is gated** behind a
  Channel Risk Engine because Meta's policy prohibits debt collection irrespective of licenses.

## 5. ICP & buyer

- **Primary ICP (INFERENCE):** mid-market **SOFOM E.N.R.** or **fintech lender / BNPL** with a
  consumer portfolio, hybrid in-house + despacho operation, 10–100 agents/gestores equivalent, real
  reputational pressure, a small compliance/legal team, and enough technical flexibility to pilot.
- **Why not banks first:** highest absolute pain but long sales, heavy security, slow procurement.
- **Why not despachos first:** they feel operational pain, but the REDECO obligation and fine fall on
  the financial entity → despacho is a better channel/partner or second segment, not the first buyer.
- **Economic buyer:** Head of Collections / Credit Risk / Compliance.
- **Where to find them:** ASOFOM (200+ members, directory of 245+ lenders), AMFE (50+), FinTech
  México (180+), ASOFOM National Convention 2026 (900+ leaders).

## 6. Product overview — modules

Vigía v1 is a control plane with these modules (detail in §7):

1. **Policy Compiler** — compiles REDECO + lender-internal + channel policies into a versioned,
   executable rule bundle (the moat seed). See [regulatory-ruleset.md](regulatory-ruleset.md).
2. **Evaluation & Guardrail Engine** — hybrid deterministic + LLM-judge detection; real-time
   pre-action blocking (Copilot+) and batch scoring (Shadow).
3. **Evidence Ledger** — immutable, hash-chained record per interaction with audio/transcript,
   applied policy version, model/prompt version, detector outputs, score, human override.
4. **REDECO Ops** — complaint intake → catalog classification → SLA engine (10 business days) →
   despacho penalization registry → monthly export/API (first 5 business days).
5. **Universal Supervisor** — scorecards over internal agents, despachos, and bots under one ruleset;
   dashboards for out-of-hours, threats/intimidation, repeat-offending despachos, risky PTP flows.
6. **Payment Resolution Layer** — generates payment instructions to the **creditor only** (never the
   despacho), with reconciliation hooks. Enforces MX-REDECO-10.

## 7. Feature backlog (so nothing is forgotten)

Legend: `[P1]` Shadow Mode · `[P2]` Copilot · `[P3]` Partial autonomy · `[P4]` Expansion.

### Policy Compiler
- `[P1]` REDECO ruleset MX-REDECO-01..16 as executable rules with effective dates and legal basis.
- `[P1]` Rule types: hard-block / warn / process-control; detector kind: deterministic vs LLM-judge.
- `[P2]` Lender-internal policy overlay (per-tenant custom rules).
- `[P2]` Channel policy rules (voice/SMS allowed; WhatsApp high-risk gate).
- `[P4]` Multi-country policy packs (extend beyond MX).

### Evaluation & Guardrail Engine
- `[P1]` Deterministic detectors: contact hours (08:00–21:00 debtor TZ), third-party contact,
  protected population, authorized channel/source, payment-routing (creditor-only), disclosure presence.
- `[P1]` LLM-judge detectors with rubrics: tone/threat/intimidation, disclosure completeness.
- `[P1]` Confidence-threshold routing to human review.
- `[P1]` Golden-set regression: labeled interactions per REDECO cause; CI gate before policy/model deploy.
- `[P2]` Real-time streaming guardrails: pre-flight an agent utterance, hard-block/reroute before it goes out.
- `[P2]` Campaign preflight simulator: run scripts/templates/sequences against the ruleset before launch.

### Evidence Ledger
- `[P1]` Append-only, hash-chained records (tamper-evident); WORM storage for audio/transcripts.
- `[P1]` Per-record provenance: policy version, model id/version, prompt version, inputs, decision, override.
- `[P1]` Evidence package export (PDF/JSON) to answer a specific complaint.
- `[P3]` Optional external anchoring of hashes.

### REDECO Ops
- `[P1]` Complaint intake + classification to the official cause catalog.
- `[P2]` SLA engine: 10-business-day response timers + escalation.
- `[P2]` Despacho penalization registry (per MX-REDECO-13).
- `[P2]` Monthly REDECO export/report generation (first 5 business days; channel, cause, status, resolution).
- `[P4]` Direct REDECO API submission integration.

### Universal Supervisor
- `[P1]` Read-only dashboards: out-of-hours contacts, threat/intimidation conversations, score by despacho.
- `[P1]` Per-interaction drill-down with evidence.
- `[P2]` Scorecards across humans / despachos / bots; sampling and alerting.
- `[P2]` Repeat-offense detection by REDECO cause.

### Payment Resolution Layer
- `[P2]` Generate SPEI / reference / payment-link instructions to creditor accounts only.
- `[P2]` Reconciliation hooks + CRM/core update.
- `[P3]` CoDi / convenience-store (cash) rails for sub-banked portfolios.

### Connectors (cross-cutting)
- `[P1]` Batch ingestion of existing call recordings (audio + metadata) → canonical `InteractionEvent`.
- `[P2]` Live voice stream ingestion (Telnyx/Twilio) for real-time path.
- `[P2]` CRM / loan-core connectors (SIAC, Sysde, Mambu, Temenos, internal).
- `[P2]` SMS channel.
- `[P3]` WhatsApp behind Channel Risk Engine (legal-reviewed flows only).

## 8. Channel strategy

- **Voice = foundational.** ~8/10 debts collected by phone; no Meta policy risk; richest signal for QA.
- **SMS = commodity** reminders / payment links; low policy complexity.
- **WhatsApp = high policy risk.** Meta prohibits debt collection irrespective of licenses. Treat as a
  conditional, legal-reviewed channel — never the commercial foundation of v1.

## 9. Out of scope (v1)

- A full eval platform competing with Braintrust/Langfuse (build only what the ruleset needs).
- Generic LLM observability product.
- "CNBV + PLD + privacy + everything" GRC suite — v1 scope = REDECO/CONDUSEF + channel policy +
  operational consent + creditor internal rules. Broader regulation is expansion, not focus.
- Fully autonomous omnichannel collector (that is Phase 3+, not the wedge).

## 10. Success metrics

- **Validation (pre-scale):** ≥2 design partners; ≥1 paid pilot (target MXN 30k–80k/mo); a
  margin/risk-recovery case study ("we found N out-of-hours/threat interactions you couldn't see").
- **Product (per tenant):** % interactions auto-scored; violations detected vs manual QA baseline;
  complaint response time reduction; out-of-hours contact rate trend; evidence-package generation time.
- **Business:** pilot→paid conversion; expansion to multi-despacho supervision; logo references in ASOFOM/AMFE.

## 11. Open questions & risks

| Risk | Why it matters | Kill signal |
|------|----------------|-------------|
| Compliance ≠ budget driver | May be a veto, not a line item | After 10 interviews, nobody pays for evidence/guardrails without a recovery promise |
| Incumbents copy fast | Altur/Colektia have distribution | A design partner says "I'll just ask my current vendor for this" |
| No access to real data | No conversations/complaints/overrides = no flywheel | No partner shares anonymizable audio/transcripts |
| Regulatory scope creep | REDECO+CNBV+PLD+privacy from day 1 kills velocity | Roadmap starts looking like a GRC system |
| Small buyers lack budget | Mid-market is accessible but not all pay for new SaaS | Everyone wants consulting/ad-hoc, nobody a recurring subscription |
| Pricing model unproven | Public evidence does not yet show a "compliance-only" premium | WTP test only converts to free or "feature of another vendor" |

**The single killer question to ask design partners:**
> "If tomorrow I showed you, per conversation, which contacts violated hours, tone, third-party,
> disclosure, or payment routing — and handed you the evidence package to answer REDECO — would you
> pay for it this quarter even if it does not increase recovery in the pilot?"

If the dominant answer is *no* → the compliance-first wedge is overstated; pivot toward QA/recovery
framing or to the plan-B vertical (Mexican accounting/CFDI ops).
