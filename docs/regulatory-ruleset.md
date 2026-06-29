# Vigía — REDECO / CONDUSEF Executable Ruleset

Status: Draft v0.1 · Last updated: 2026-06-28
This is the **product core and moat seed**: the regulatory catalog turned into executable rules.
Each rule maps to a detector (deterministic or LLM-judge), an action, and a legal basis.

> Legal note: this is an engineering working spec, not legal advice. Every rule and its legal basis
> MUST be reviewed and signed off by a Mexican financial-services lawyer before production use.
> Primary normative source: *Disposición en Materia de Registros ante la CONDUSEF* (DOF 14-oct-2022)
> and REDECO operation; LTOSF for sanctions; SCJN Amparo en Revisión 323/2025 (Jan 15, 2026) for
> the entity's supervision/reporting duty and CONDUSEF's sanction power.

## Detector kinds

- **Deterministic** — exact, cheap, no hallucination. Use for hours, third-party, channel source,
  payment routing, disclosure presence. These are hard gates.
- **LLM-judge** — rubric-scored, for fuzzy semantics (tone, threat, intimidation, disclosure
  completeness). Always routes to human review below a confidence threshold.

## Action types

- **HARD BLOCK** — stop/reroute the action before it happens (Copilot+); flag + severe score (Shadow).
- **WARN** — allow but score negatively and surface to supervisor.
- **PROCESS CONTROL** — obligation on the financial entity/workflow, not the individual call script.
- **HITL** — mandatory human-in-the-loop review/override.

## Ruleset

| Rule ID | Type | Detector | Implementable rule | System action | Legal basis |
|---------|------|----------|--------------------|---------------|-------------|
| MX-REDECO-01 | Identity | Deterministic | Before first contact, validate identity of debtor / aval / obligado solidario and locator data. | BLOCK if insufficient match. | Art. 127.I |
| MX-REDECO-02 | Disclosure | LLM-judge | On first contact, disclose: despacho name, collector name, financial entity name, debt amount, calculation date, terms/conditions to settle, and contractual basis/reason for the debt. | WARN/BLOCK if mandatory disclosure missing. | Art. 127.II + REDECO cause catalog |
| MX-REDECO-03 | Disclosure | Deterministic | Provide the UNE / complaints-unit contact (address/email/phone) and that a complaint can be filed in REDECO. | WARN + negative score; BLOCK new campaigns whose template omits it. | REDECO cause catalog |
| MX-REDECO-04 | Hours | Deterministic | Contact only on business days, **08:00–21:00** in the debtor's timezone. | HARD BLOCK outside window. | REDECO / current disposition |
| MX-REDECO-04A | Doc conflict | Deterministic | If legacy 07:00–22:00 material is in use, flag documentary conflict and require current official criterion. | ESCALATE to legal. | Old CONDUSEF pages say 07:00–22:00; current REDECO/DOF say 08:00–21:00 |
| MX-REDECO-05 | Tone | LLM-judge | Prohibit threats, offense, intimidation, harassment. | HARD BLOCK + severe score + mandatory HITL. | REDECO causes; CONDUSEF material |
| MX-REDECO-06 | Third party | Deterministic | Prohibit collection management with persons who are not the user/debtor/co-debtor/aval/obligado solidario. | HARD BLOCK + suppress contact. | REDECO causes / disposition |
| MX-REDECO-07 | Protected pop. | Deterministic | Prohibit management with minors or elderly persons, unless the elderly person is the debtor. | HARD BLOCK + HITL. | Disposition / REDECO causes |
| MX-REDECO-08 | Impersonation | LLM-judge | Prohibit names resembling public institutions or documents simulating judicial/authority writs. | HARD BLOCK. | CONDUSEF / disposition |
| MX-REDECO-09 | Public shaming | Deterministic | Prohibit blacklists, posters, or public announcements about refusal to pay. | HARD BLOCK. | Disposition |
| MX-REDECO-10 | Payment | Deterministic | The despacho may not directly receive payment; payments/agreements must be made/received by the financial entity. | BLOCK any pay-to-despacho instruction; only generate creditor links/accounts. | Arts. 121, 126, 132 |
| MX-REDECO-11 | Auth. channel | Deterministic | Prohibit contact at address/phone/email other than the one provided by the entity or debtor/aval. | HARD BLOCK if channel not from authorized source. | Disposition |
| MX-REDECO-12 | Evidence | Process control | Every agreement/promise-to-pay must be documented and traceable; the entity must subscribe/receive it through established means. | REQUIRE evidence artifact (summary, audio, timestamp, channel, identity). | Arts. 126, 132; REDECO evidence to close a cause |
| MX-REDECO-13 | Complaints | Process control | The entity must respond to complaints within 10 business days via REDECO and separately register despacho penalizations. | SLA engine + escalation. | Art. 122 |
| MX-REDECO-14 | Reporting | Process control | Monthly REDECO report within the first 5 business days of the following month (channel, cause, status, resolution, penalization). Pending items closed before next month-end. | Monthly export/API + validation. | Art. 124 |
| MX-REDECO-15 | Despacho registry | Process control | Register/update the contracted despacho and its data; upload contract and RFC; update on change within legal term. | Master-data control. | Arts. 118–120, 131 |
| MX-REDECO-16 | Debtor data | Process control | Keep debtor identification data accurate/updated and available to CONDUSEF while collection actions are ongoing. | Data-quality control. | Art. 125 |

## Sanctions (for the risk/ROI narrative)

- Range: **200–2,000 días de salario mínimo general**.
- 2026 salario mínimo general: **$315.04 MXN/day** → ≈ **MXN $63,008 – $630,080** per sanction.
- The older circulating figure (MXN $13,458–$134,580) is **obsolete** — do not use.
- **Liability shifted:** post-SCJN 323/2025, the entity is responsible for supervising/reporting;
  failure makes the fine the entity's, not the despacho's. This is the core sales argument.

## National vs entity-specific

- REDECO regime + despacho obligations + LTOSF + monthly reporting → **national** for entities under CONDUSEF.
- CNBV enters via prudential/entity-type supervision (out of v1 scope).
- v1 mandatory layer = **REDECO/CONDUSEF + channel policy + operational consent/privacy + creditor
  internal rules**. Modeling the whole Mexican regulatory universe in v1 is scope creep.

## Open legal items to confirm with counsel

- Exact current contact-hours text and any business-day/holiday nuances (MX-REDECO-04).
- Precise disclosure script minimum for MX-REDECO-02 (what counts as complete).
- Current sanction computation basis (días de salario mínimo vs UMA) per latest LTOSF text.
- REDECO API submission spec and authentication for MX-REDECO-14 automation.
