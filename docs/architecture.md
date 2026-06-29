# Vigía — System Architecture

Status: Draft v0.3 (workflow-first reframe) · Last updated: 2026-06-28
Companion: [PRD.md](PRD.md) · [regulatory-ruleset.md](regulatory-ruleset.md) · [technical-design.md](technical-design.md) · [build-plan.md](build-plan.md)

> Authoritative stack, data model, ADRs, and module specs: [technical-design.md](technical-design.md).
> This is the conceptual view.

---

## 1. Design principles

1. **Workflow-first (ADR-09).** The Shadow-Mode core is a deterministic workflow + an LLM-judge step,
   not an autonomous agent. The real agent loop is built in the Copilot phase, where it's justified.
2. **Deterministic-first detection.** Exact rules (hours, third-party, channel, payment) are deterministic
   gates; the LLM-judge handles only fuzzy semantics (tone, disclosure completeness).
3. **Evidence is the product.** Every decision recorded with full provenance, tamper-evident, write-once.
4. **Untrusted transcripts.** Debtor/despacho speech is data, never instructions (injection boundary).
5. **Overlay, not replacement.** Adds value on top of existing operations (agents, despachos, dialers, bots).
6. **Voice-first; channel-risk-aware.** WhatsApp is gated (Meta policy).
7. **Multi-tenant + data residency MX** from day one.
8. **Durable correctness over a transparent substrate.** River (Postgres) + an explicit, hardened state
   machine — durability is understood and verifiable, not hidden behind a framework.

## 2. Layered view

```
┌─────────────────────────────────────────────────────────────────┐
│  SUPERVISION CONSOLE (Next.js / TS)  — scorecards, HITL, evidence │
└───────────────▲─────────────────────────────────────▲────────────┘
                │  (HTTP API, Go; tenant API-key auth → RLS)         │
┌───────────────┴───────────┐         ┌─────────────────┴───────────┐
│  REALTIME GUARDRAIL PATH   │         │   ASYNC / DURABLE PATH       │
│  (Copilot phase, M4)       │         │   (River workers, Go)        │
│  utterance pre-flight,     │         │   hardened state machine:    │
│  hard-block / reroute      │         │   cases · SLA · HITL · report│
└───────────────▲───────────┘         └─────────────────▲───────────┘
                │                                         │
        ┌───────┴─────────────────────────────────────────┴───────┐
        │        COMPLIANCE WORKFLOW ENGINE (Go)                    │
        │  Deterministic detectors  +  LLM-judge (temp 0, cached,   │
        │  injection-bounded)  ·  golden-set CI gate  ·  conf→HITL   │
        └───────────────────────▲──────────────────────────────────┘
                                 │
        ┌────────────────────────┴──────────────────────────────────┐
        │   POLICY COMPILER  —  REDECO + overlays → versioned bundle  │
        └────────────────────────▲──────────────────────────────────┘
                                 │
┌──────────────┐  ┌──────────────┴───────────┐  ┌─────────────────────┐
│ EVIDENCE     │  │  AGENT HARNESS (Go)       │  │ INGESTION /          │
│ LEDGER (Go)  │  │  DEFERRED → Copilot (M4): │  │ CONNECTORS           │
│ hash-chain,  │◄─┤  agent loop · tool dispatch│ │ voice (Telnyx/Twilio │
│ Merkle, WORM,│  │  (plain Go iface; MCP as   │ │ + batch recordings)· │
│ in-tx writes │  │  learning artifact) · HITL │  │ CRM/core · SMS · WA* │
└──────────────┘  └────────────────────────────┘  └─────────────────────┘
                                 │
                  ┌──────────────┴───────────────┐
                  │ OBSERVABILITY (OTel GenAI semconv / Langfuse) │
                  │ traces · cost · per-tenant                    │
                  └───────────────────────────────────────────────┘
   * WhatsApp only behind the Channel Risk Engine — never foundational
```

## 3. Core data model

See [technical-design.md §4](technical-design.md) for canonical Go structs (now incl. `Debtor`,
`TenantAPIKey`, `PolicyBundleRule`, normalized `DetectorResultRow`, `ComplaintCase` state machine). All
tables carry `tenant_id` (RLS); IDs are UUID v7.

## 4. Mapping to the agentic distributed-systems stack

| Capability | Where it lives | Stack primitive exercised |
|------------|----------------|---------------------------|
| Tool calling + contracts | Harness (M4): identity, timezone, channel-auth, payment-link, CRM | Plain Go tool interface (prod) + Go MCP SDK (learning) |
| Orchestration (queue+state+workers) | Async path: hardened durable state machine on River | River + idempotency + exactly-once |
| Evals (golden, regression, CI) | Eval runner gating every policy/model/rubric deploy | Evals-as-tests, CI gate |
| Sandbox | Campaign preflight simulator (M4) | Isolated eval runs |
| Human-in-the-loop | Console + state machine: pause `awaiting_review` → resume; approval TTL | Durable HITL pause/resume |
| Traceability | Evidence ledger: hash-chain + Merkle + in-tx writes | Immutable audit trail |
| Observability | OTel GenAI semconv + Langfuse; per-tenant cost/quality | End-to-end observability |
| Guardrails | Realtime path (M4) + judge injection boundary | Pre-action block + input safety |
| LLM efficiency | Judge with prompt caching on stable prefix | Cost control |
| Voice | Telnyx/Twilio + Transcriber (Whisper/Deepgram) + chunking | Voice agent stack |

## 5. Tech stack (summary)

Authoritative table in [technical-design.md §2](technical-design.md). In short: **Go 1.26** workflow
engine + River (hardened) + **Postgres + sqlc + pgx + RLS** + **goose** + per-tenant API-key auth +
**anthropic-sdk-go** (cached, injection-bounded `Judge`) + `Transcriber` STT + **MinIO/S3 WORM** +
**OTel GenAI semconv + Langfuse** + **Next.js (TS)** console. Agent harness deferred to M4.

## 6. Phase 1 (Shadow Mode) minimal architecture

```
Batch recording connector → Policy Compiler → Detection (deterministic + judge)
   → Evidence Ledger → Supervision Console (read-only dashboards + evidence export)
```

No agent loop, no real-time path, no telephony, no WhatsApp, no payment layer. Goal: prove Vigía detects
violations manual QA misses and produces a usable REDECO evidence package.
