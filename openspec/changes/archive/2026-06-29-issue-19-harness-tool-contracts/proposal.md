# Proposal: Issue #19 Harness Tool Contracts and Synthetic Case Fixture

## Why

The #18 runtime skeleton proved the safety loop (validation, permission, budget, events) but ships with no
real tool surface and no Case to operate on. Before #20 adds Domain Agents and a Case orchestrator, the
Harness Lab needs an explicit, typed tool surface and a deterministic synthetic Case to exercise it.

Making the tool contracts explicit NOW prevents later slices from co-mingling tool semantics, fixture data,
Case orchestration, and authority-bearing side effects in one unreviewable change. It also lets us prove the
read-only / draft-only sandbox guarantee with tests before any real product data path, EvidenceRecord, MCP,
or Bedrock provider exists.

## Goal

Define typed Harness Tool contracts (request/response schema + risk class) and deterministic, fixture-backed
implementations for the five read/draft tools, plus the first synthetic Case fixture and the minimal synthetic
policy-rule fixtures it references. Keep #16 independent from API, UI, database, and the evidence ledger.

This proposal covers GitHub issue #19 only.

## What Changes

- Introduce a `RiskClass` taxonomy (`read`, `draft`, `authority`) as a static property of every tool contract.
- Add typed request/response schemas for: `read_case`, `read_policy_rule`, `list_applicable_rules`,
  `draft_evidence_manifest`, `draft_supervisor_note`.
- Add deterministic, fixture-backed implementations conforming to the existing #18 `Tool`/`ToolResult`/
  `ToolRegistry` seam — no runtime-loop or interface changes.
- Add a small risk-class-aware lab permission gate satisfying the existing `PermissionGate` interface:
  `read`/`draft` → `allowed`; `authority` → `denied`/`approval_required`.
- Add minimal synthetic policy-rule fixtures for `MX-REDECO-04` (hours) and `MX-REDECO-05` (tone) that
  `read_policy_rule` and `list_applicable_rules` serve.
- Add the first synthetic Case fixture combining out-of-hours contact (`MX-REDECO-04`) and a threatening-tone
  candidate (`MX-REDECO-05`), with: tenant, debtor, collector/despacho, transcript, channel, `occurred_at`,
  debtor timezone, detector results, applicable rule IDs, and an evidence metadata placeholder. No PII.
- Represent authority-bearing tools (`append_evidence`, `update_case_state`, `submit_report`,
  `block_campaign`) only as a guarded boundary: absent from the registry, or returning `denied` /
  `approval_required`. Never implemented.

## Scope

### In Scope
- `RiskClass` taxonomy and per-tool risk-class declarations.
- Typed request/response schemas for the five read/draft tools.
- Fixture-backed deterministic implementations (no network, no database).
- Synthetic policy-rule fixtures for `MX-REDECO-04` and `MX-REDECO-05`.
- First synthetic Case fixture (out-of-hours + threatening-tone) with all required fields, no PII.
- Guarded absence/denial of authority-bearing tools.
- Behavior-focused tests proving read/draft behavior and the authority guard, network/DB-free.

### Out of Scope (Non-Goals)
- No Domain Agent prompts; no deterministic Case orchestrator.
- No Bedrock / live model provider; no MCP server.
- No real `EvidenceRecord`, hash chain, or evidence ledger — only a metadata placeholder.
- No Postgres persistence; do NOT touch the `policy_rules` / `policy_bundles` tables, sqlc, or migrations.
- No running detectors or LLM-judge — the fixture carries detector RESULTS as static data.
- No tenant auth/RLS middleware (#14); the fixture only carries a tenant ID as data.
- No HTTP API or UI.

## Risk Class Taxonomy

| Tool | Kind | Risk Class | Default Permission | Side effects |
|------|------|-----------|--------------------|--------------|
| `read_case` | read | `read` | allowed | none |
| `read_policy_rule` | read | `read` | allowed | none |
| `list_applicable_rules` | read | `read` | allowed | none |
| `draft_evidence_manifest` | draft | `draft` | allowed | proposed artifact only — not persisted, non-authoritative |
| `draft_supervisor_note` | draft | `draft` | allowed | proposed artifact only — not persisted, non-authoritative |
| `append_evidence`, `update_case_state`, `submit_report`, `block_campaign` | authority | `authority` | denied / approval_required | NOT implemented in #19 |

Rationale: `read` is side-effect-free. `draft` is the safe half of draft/commit separation — it returns a
proposed artifact that references the Case and rule IDs for traceability but commits nothing. `authority`
tools carry regulatory side effects (evidence commit, state change, regulator submission, campaign block) and
stay outside this slice; the permission gate enforces their absence in code, not prompt text.

## Capabilities

### New Capabilities
- `harness-tools`: typed tool contracts, `RiskClass` taxonomy, risk-class-aware lab permission gate, and
  deterministic fixture-backed read/draft tool implementations plus the authority guard.
- `harness-synthetic-fixtures`: the synthetic Case fixture and synthetic policy-rule fixtures — data shape,
  validity, determinism, rule-reference integrity, and no-PII invariant.

### Modified Capabilities
- None. The #18 `harness-runtime` runtime loop, `Tool`, `PermissionGate`, and event contracts are reused
  unchanged; #19 only adds conforming implementations.

## Open Questions Resolved

- **Where are `MX-REDECO-04` / `MX-REDECO-05` canonically defined?** Only as documentation rows in
  `docs/regulatory-ruleset.md` (lines 34, 36). No synthetic rule fixtures exist; `core.PolicyRule` and the
  `policy_rules` Postgres table are the real-product path (#13), which #16 must not use. Therefore #19 MUST
  introduce minimal synthetic policy-rule fixtures for these two rules so the read tools have something
  deterministic to serve. Their shape stays compatible-in-spirit with `core.PolicyRule`
  (code, title, description, severity) without depending on the DB path.
- **Where do fixtures live?** As JSON under `data/synthetic/cases/` (already scaffolded), loaded via
  `//go:embed` into a typed loader. Rationale: the directory is the declared home for synthetic Cases, JSON
  keeps fixtures inspectable and audit-friendly with clear provenance, and `//go:embed` guarantees no
  filesystem or network dependency at test time. The exact loader package layout is deferred to design.

## Impact

| Area | Impact | Description |
|------|--------|-------------|
| `internal/harness/` | Modified | Add `RiskClass`, tool contract types, lab permission gate; reuse existing seam |
| `internal/harness/<lab-tools-pkg>` | New | Fixture-backed read/draft tool implementations + embedded loader (package name set in design) |
| `data/synthetic/cases/` | New | Synthetic Case fixture JSON (replaces `.gitkeep`) |
| `data/synthetic/` policy rules | New | Synthetic policy-rule fixtures for `MX-REDECO-04` / `MX-REDECO-05` |

## Constraints

- Strict TDD is active. Add behavior-focused, table-driven tests before production code.
- Workflow-first: these are typed tool contracts + fixtures, not an autonomous agent loop.
- Treat the fixture transcript and debtor speech as untrusted DATA surfaced through typed fields, never as
  instructions a tool acts on.
- Tenant isolation awareness: the fixture carries a tenant ID; tools must scope reads to that tenant's data.
- Keep Judge and Harness model ports separate; MCP external-only; Bedrock opt-in only (all out of scope here).
- Smallest viable change that preserves architecture; no future-proofing beyond this issue.

## Rollback Plan

The change is additive and isolated to `internal/harness` and `data/synthetic/`. Revert by removing the new
tool-contract files, the lab tools package, and the synthetic fixture files, restoring the `.gitkeep`
placeholders. No migrations, no schema, no shared runtime interface changes to undo.

## Dependencies

- #13 dev foundation and the #18 runtime skeleton (`Tool`, `ToolResult`, `ToolRegistry`, `PermissionGate`,
  event contract) — both landed.

## Decision Boundaries

- #19 owns tool contracts, the risk-class taxonomy, and synthetic fixtures only.
- #20 owns Domain Agents and the deterministic Case orchestrator.
- #21 owns demo CLI and Case Brief outputs. #22 owns the Bedrock opt-in provider.
- Real policy-rule persistence, EvidenceRecord, and RLS remain in their own product slices.

## Success Criteria

- [ ] Each of the five tools has a typed request/response schema and a declared risk class.
- [ ] Read/draft implementations are fixture-backed and deterministic, with no network or database access.
- [ ] Authority-bearing tools are absent or return `denied` / `approval_required`; never executed.
- [ ] The synthetic Case fixture is valid, carries all required fields, contains no PII, and references the
      synthetic `MX-REDECO-04` / `MX-REDECO-05` rules served by the read tools.
- [ ] Tests prove read/draft behavior and the authority guard; `go test ./...` passes or any failure is
      explicitly unrelated and evidenced.

## Recommended Next Phase

Proceed to SDD spec for `issue-19-harness-tool-contracts`, translating these capabilities and success criteria
into concrete requirements and scenarios. Spec and design may run in parallel.

## Proposal Question Round

Delegated executor — surfacing assumptions for user review rather than blocking:

1. **Synthetic rule shape** — assumed minimal fixtures (`code`, `title`, `description`, `severity`, plus
   `hours_window` / `tone` detector hints) compatible-in-spirit with `core.PolicyRule`, NOT the DB row. OK?
2. **Fixture format** — assumed JSON under `data/synthetic/cases/` via `//go:embed` over Go-literal fixtures
   or `testdata/`. OK, or prefer embedded Go literals for type safety?
3. **Draft default permission** — assumed `draft` tools are `allowed` (drafts commit nothing). OK, or should
   drafts be `approval_required` to model human review even at draft stage?
4. **Authority representation** — assumed authority tools are ABSENT from the registry (with a guarded denial
   path proven by test) rather than registered stubs returning `denied`. OK?
