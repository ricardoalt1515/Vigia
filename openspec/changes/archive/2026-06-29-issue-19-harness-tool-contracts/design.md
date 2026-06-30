# Design: Issue #19 Harness Tool Contracts and Synthetic Case Fixture

## Technical Approach

Add a `RiskClass` taxonomy to the existing harness vocabulary and an isolated sandbox
sub-package, `internal/harness/labtools`, that provides five deterministic, fixture-backed
`harness.Tool` adapters plus a risk-class-aware `harness.PermissionGate`. The #18 runtime seam
(`Tool`, `PermissionGate`, `ToolResult`, `ToolStatus`, runtime loop) is reused unchanged — `labtools`
only ADDS conforming implementations. Fixtures are JSON under `data/synthetic/`, embedded via
`//go:embed` and validated at load. This is the ports/adapters pattern: `harness` owns the ports and
the `RiskClass` vocabulary; `labtools` is the driven adapter. No DB, no API, no evidence ledger.

## Architecture Decisions

| # | Decision | Choice | Rejected / Rationale |
|---|----------|--------|----------------------|
| 1 | Package layout | `RiskClass` + constants in `internal/harness` (next to `ToolStatus`); the five typed DTOs, tool impls, loader, name→class catalog, and `LabPermissionGate` in new `internal/harness/labtools` | Putting DTOs/gate in `harness` would drag JSON/embed/fixture concerns into the pure seam. `RiskClass` stays in `harness` because it is shared safety vocabulary the gate contract classifies against, not implementation. Gate implements the `harness.PermissionGate` port from the adapter side — clean hexagonal split. |
| 2 | Synthetic rule shape | Minimal `labtools.SyntheticRule{Code,Title,Description,Severity,Hours,Tone}` — NOT `core.PolicyRule`, NOT the `policy_rules` DB path | `core.PolicyRule` carries DB identity/timestamps and is owned by #13/RLS; #16 must stay independent of API/UI/DB. Deliberate duplication isolates the sandbox from a schema that evolves under product/regulatory pressure. Field names are compatible-in-spirit so a future real adapter is easy without a compile-time `core`/DB dependency now. |
| 3 | Fixture format + loading | JSON under `data/synthetic/{cases,rules}/`, embedded by an embed-only `package synthfixtures` at `data/synthetic/embed.go`; parsed + validated in `labtools` | Go-literals couple data to code and make no-PII/shape review harder; `testdata/` is test-only but the read tools serve fixtures at RUNTIME. `//go:embed` cannot reach `../`, so the embed directive must live at `data/synthetic/` (the proposal's declared home, audit-friendly provenance). The embed package holds zero logic; all typing/validation stays in `labtools`. |
| 4 | Draft tools | `draft_*` return `ToolResult{Status: ToolStatusSuccess}` with a typed artifact in `Output` referencing Case + rule codes; `persisted:false`, `non_authoritative:true`; class `draft`; permission `allowed` | Draft is the safe half of draft/commit separation — proposes, commits nothing. `allowed` (not `approval_required`) because drafts have no side effect; modelling human review belongs to the authority/commit step, which #19 does not build. |
| 5 | Authority guard | Authority tools ABSENT from registry; `LabPermissionGate` classifies the four known authority names → `authority` and returns `PermissionDenied` BEFORE registry lookup; unknown names → denied (fail-closed) | Enforcement lives in CODE (the gate), not in tool absence — absence is defense-in-depth. The runtime calls `Decide` before `r.Tools[name]`, so denial short-circuits execution. `denied` is the locked default; `approval_required` is a one-line decision swap deferred to #20 (no HITL/resume path exists yet). |

## Data Flow

```
ModelOutput.ToolCall{Name, Input map[string]any}
   -> Runtime.evaluateTool
       -> LabPermissionGate.Decide(name)  [riskClassFor: read/draft->allowed, authority/unknown->denied]
            denied -> StepStatusPermissionDenied            (tool never executed)
            allowed -> ToolRegistry[name].Execute
                         decode Input map --(json round-trip)--> typed Request -> validate
                         query embedded CaseStore / RuleStore (in-memory, no IO)
                         build typed Response --(encode)--> ToolResult{Success, Output map}

load (once): synthfixtures.FS --embed--> loader.Load() -> validate -> CaseStore + RuleStore
```

## Typed Contracts (decision #6)

Each tool decodes `ToolCall.Input` into a typed Request and encodes its typed Response into
`ToolResult.Output` via a stdlib JSON round-trip codec (`decode[T](map)`, `encode(v)`) — no new deps,
deterministic, validated at the tool boundary. Transcript/debtor speech is a typed DATA field, never
interpreted as instructions.

| Tool | Class | Request | Response (`Output`) |
|------|-------|---------|----------|
| `read_case` | read | `{CaseID}` | `{Case: SyntheticCaseView}` — tenant, debtor ref, collector/despacho, channel, occurred_at, debtor_tz, transcript (untrusted), detector_results, applicable_rule_codes, evidence_placeholder |
| `read_policy_rule` | read | `{RuleCode}` | `{Rule: SyntheticRuleView}` — code, title, description, severity, hours/tone hints |
| `list_applicable_rules` | read | `{CaseID}` | `{RuleCodes:[], Rules:[SyntheticRuleView]}` scoped to the Case's tenant |
| `draft_evidence_manifest` | draft | `{CaseID}` | `{Manifest}` — case_id, tenant_id, rule_codes, detector summary, evidence placeholder, `persisted:false`, `non_authoritative:true` |
| `draft_supervisor_note` | draft | `{CaseID}` | `{Note}` — case_id, advisory summary citing rule codes + severity, recommended_action, `non_authoritative:true` |

Reads are scoped to the loaded synthetic tenant (no cross-tenant fixture exists — guarantee is
structural). Unknown CaseID/RuleCode or malformed input → `Execute` returns a non-nil `error`
(deterministic caller error in a fixed sandbox); richer error-as-observation is deferred until a
non-deterministic data path exists. Drafts carry no wall-clock timestamp (provenance = ids +
fixture_version) to stay deterministic.

## Loader Validation Responsibilities (decision #3)

- **Required fields**: non-empty Case id, tenant id, channel, occurred_at, debtor_tz, transcript; each rule has code/title/severity.
- **Rule-reference integrity**: every `applicable_rule_code` and detector-referenced code resolves to a loaded `SyntheticRule`; dangling ref → fail load.
- **No-PII shape**: synthetic ids use a `SYN-` prefix, parties are role labels (`DEBTOR_A`, `COLLECTOR_1`, `DESPACHO_X`), reject any field matching email/phone regex. Structural assertion (shape, not semantics).
- **Determinism**: fixed ids + timestamps; no `time.Now` at load. Fail-closed: any error aborts construction.

## File Changes

| File | Action | Description |
|------|--------|-------------|
| `internal/harness/risk.go` | Create | `RiskClass` type + `read`/`draft`/`authority` constants |
| `internal/harness/labtools/contracts.go` | Create | Five typed Request/Response DTOs + JSON-roundtrip map codec |
| `internal/harness/labtools/fixtures.go` | Create | `SyntheticCase`, `SyntheticRule`, hint structs, `CaseStore`/`RuleStore` |
| `internal/harness/labtools/loader.go` | Create | Parse + validate embedded JSON into stores |
| `internal/harness/labtools/catalog.go` | Create | Name→`RiskClass` catalog + authority name set |
| `internal/harness/labtools/gate.go` | Create | `LabPermissionGate` implementing `harness.PermissionGate` |
| `internal/harness/labtools/tools.go` | Create | Five `harness.Tool` impls + `Registry()` builder |
| `internal/harness/labtools/*_test.go` | Create | Table-driven loader/tool/gate/runtime-integration tests |
| `data/synthetic/embed.go` | Create | `package synthfixtures`; `//go:embed cases/*.json rules/*.json`; `var FS embed.FS` |
| `data/synthetic/cases/case-001.json` | Create | Synthetic Case (out-of-hours + threatening tone), no PII |
| `data/synthetic/rules/rule-mx-redeco-04.json`, `rule-mx-redeco-05.json` | Create | Synthetic hours + tone rules |
| `data/synthetic/cases/.gitkeep` | Delete | Replaced by real fixture |

## Testing Strategy (decision #7 — strict TDD, no network/DB)

| Layer | What to Test | Approach |
|-------|--------------|----------|
| Loader | valid load; tamper variants (missing field, dangling rule ref, PII-shaped value) fail | Table-driven; embedded + inline-mutated bytes |
| Read tools | correct typed Output; deterministic (run twice, deep-equal) | Table-driven over CaseID/RuleCode |
| Draft tools | success + non-authoritative artifact cites case+rule ids; `persisted:false`; no IO | Table-driven; assert package touches no disk |
| Gate | four authority names → denied; read/draft → allowed; unknown → denied | Table-driven `Decide` matrix, no registry needed |
| Integration | real `harness.Runtime` + labtools registry + gate + stub `ModelProvider`: authority proposal → `StepStatusPermissionDenied` (no execution); read → `StepStatusCompleted` | Reuse #18 loop unchanged; stub provider queues outputs |

TDD order per slice: RED (failing behavior test) → GREEN (minimal impl) → TRIANGULATE (add cases) →
REFACTOR. No `time.Now`, no `t.TempDir`, no network; `//go:embed` removes filesystem reads.

## Migration / Rollout

No migration. Additive and isolated to `internal/harness` + `data/synthetic`. Rollback = delete new
files, restore `.gitkeep`.

## Open Questions

- [ ] None blocking. `authority` default is `denied`; flip to `approval_required` deferred to #20.
