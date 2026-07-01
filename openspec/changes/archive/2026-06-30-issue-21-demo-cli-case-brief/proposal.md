# Proposal: Issue #21 Harness Demo CLI and Case Brief Outputs

## Why

#18 (runtime), #19 (lab tools + embedded synthetic Case `CASE-SYN-001`), and #20 (the deterministic
`caseflow` orchestrator + four Domain Agents) compose an end-to-end Case workflow, but it produces only an
IN-MEMORY `CaseBrief`. There is no way to RUN the lab or SEE its outputs without writing Go test code, and
the orchestrator's per-step operational events are discarded. The Agent Harness Lab is therefore not
portfolio-visible and not runnable as a demo.

#21 adds a local Demo CLI that drives the #20 orchestrator with a deterministic Fake provider and writes
three reviewable artifacts per run: an operational event log, a validatable Case Brief JSON, and a
human Case Brief in Spanish. Doing this NOW — before Bedrock (#22) — proves the workflow-first lab is
runnable with NO cloud credentials and makes its compliance outputs inspectable, while keeping the live
model surface out of scope.

This proposal covers GitHub issue #21 only.

## Goal

Add `go run ./cmd/harness-demo --case <path>` that runs the synthetic Case through the #20 deterministic
orchestrator using a default Fake Model Provider (no AWS, no network) and writes, under
`data/synthetic/harness-runs/`:

- `<case_id>.jsonl` — Harness Event Log: structured OPERATIONAL events (not hidden chain-of-thought).
- `<case_id>.brief.json` — schema-valid Case Brief contract.
- `<case_id>.brief.md` — human Case Brief in NEUTRAL PROFESSIONAL SPANISH, including a disclaimer that
  outputs are DRAFTS requiring Compliance Supervisor review.

Reuse #18/#19/#20 unchanged except for ONE additive seam on the orchestrator to surface events.

## What Changes

- Add a new `cmd/harness-demo` main package: flag parsing, default Fake provider, run via `caseflow`,
  serialize + render the three artifacts, deterministic exit codes. Sibling to existing `cmd/{api,seed,worker}`.
- Add a demo-only deterministic Fake `ProviderFactory` inside `cmd/harness-demo`, scripted per agent for
  `CASE-SYN-001` (mirrors the e2e `perAgentProvider` pattern). It requires no real model.
- Add a CLI-local serialization DTO + forward marshaler that flattens the non-serializable `CaseBrief`
  (interface-typed `Stages[].Handoff`) into the validatable `.brief.json`, plus a committed JSON Schema.
- Add a CLI-local Spanish Case Brief renderer (`.brief.md`) with the draft / Compliance-Supervisor-review
  disclaimer. This is the ONLY Spanish artifact; all code, keys, JSONL, and JSON stay English.
- Add ONE additive seam to `internal/harness/caseflow`: an optional event observer (backward-compatible
  functional option on `NewOrchestrator`) so per-agent `harness.Event`s reach the CLI for the JSONL log.
- Add a portfolio-visible copy of the synthetic Case at `data/synthetic/cases/CASE-SYN-001.json` so the
  `--case` input is reviewable outside `internal/.../fixtures`.

## Scope

### In Scope
- `cmd/harness-demo` CLI driving the #20 orchestrator with a default deterministic Fake provider.
- Demo-only scripted provider for `CASE-SYN-001`; embedded store + real `LabPermissionGate` + real tools.
- Three output artifacts (`.jsonl`, `.brief.json`, `.brief.md`) written to `data/synthetic/harness-runs/`.
- Additive event-observer seam on `caseflow` orchestrator (optional; default behavior unchanged).
- Committed JSON Schema for the Case Brief; Spanish `.brief.md` with draft/review disclaimer.
- Behavior-focused tests proving the three files are generated, the JSON is schema-valid, the markdown is
  Spanish + carries the disclaimer, and the run is deterministic. Network/DB/Bedrock-free.

### Out of Scope (Non-Goals)
- NO Bedrock or any live/real model provider (#22).
- NO Remote MCP server, NO HTTP API, NO console/UI.
- NO real evidence-ledger writes, EvidenceRecord, hash chain, RLS, or crypto-integrity claims.
- NO changes to #18 runtime interfaces, #19 tool contracts, or #20 agent definitions / handoff types.
- NO generic arbitrary-case loader: only the scripted synthetic Case is runnable in this slice.

## Outputs

| Artifact | Language | Content |
|----------|----------|---------|
| `<case_id>.jsonl` | English | Per-step operational `harness.Event`s, annotated with `agent_name` + sequence |
| `<case_id>.brief.json` | English | Flattened `CaseBrief` (status, ordered stages + concrete handoffs, failure fields), schema-valid |
| `<case_id>.brief.md` | Neutral professional Spanish | Human Case Brief + DRAFT / Compliance-Supervisor-review disclaimer |

## Capabilities

### New Capabilities
- `harness-demo-cli`: the `cmd/harness-demo` CLI — `--case` flag, default Fake provider, deterministic run
  via the #20 orchestrator, exit codes, and writing the three artifacts. No cloud/network.
- `harness-case-brief-output`: the Case Brief OUTPUT contract — the serialization DTO/marshaler producing
  schema-valid `.brief.json`, the JSON Schema, and the Spanish `.brief.md` renderer with the draft/review
  disclaimer.

### Modified Capabilities
- `harness-case-orchestrator`: additive optional event-observer seam to surface per-agent operational
  events. Existing four-arg construction, fixed-order execution, and `CaseBrief` semantics are unchanged.

## Open Questions Resolved

1. **Case input source** — `--case` is a PATH to a synthetic Case JSON, defaulting to
   `data/synthetic/cases/CASE-SYN-001.json`. The CLI reads it only to obtain `case_id`, then runs that id
   through the orchestrator against the EMBEDDED store (`labtools.Load()` is the source of truth the tools
   read). Only `CASE-SYN-001` is scripted; any other id exits non-zero with a clear "unsupported synthetic
   case" message. The portfolio copy is a verbatim mirror of the embedded fixture. This honors the issue's
   `--case <path>` UX, stays deterministic and cloud-free, and avoids building a generic case-injection seam.
2. **Deterministic Fake provider** — A demo-only scripted `ProviderFactory` lives in `cmd/harness-demo`
   (not in `caseflow`), keyed per agent for `CASE-SYN-001`, mirroring the e2e `perAgentProvider`. Keeping the
   canned script in the CLI keeps the production `caseflow` package free of demo data.
3. **CaseBrief JSON serialization** — A CLI-local serialization DTO + forward marshaler type-switches on
   each handoff `Kind()` and marshals the concrete struct (forward-only; round-trip into the interface is not
   needed for an output). This honors #20's decision boundary ("#21 owns Case Brief file rendering"), keeps
   #20 types intact, and is validated against a committed JSON Schema.
4. **Event log surfacing** — Add ONE additive seam: an optional event observer on the orchestrator
   (backward-compatible functional option). `Run`/`runAgent` invoke it with `(agentName, result.Events)` per
   `RunStep`; the CLI accumulates and writes JSONL. This is the smallest additive change to #20 (the existing
   four-arg `NewOrchestrator` and all current tests keep compiling) and avoids wrapping the runtime. Events are
   the runtime's existing STRUCTURED operational set (`agent_started`, `plan_created`, `tool_proposed`,
   `permission_decision`, `tool_result`, `validation_failure`, `budget_exceeded`, `agent_completed`) — never
   hidden chain-of-thought (visible plans are explicit model output per `events.go`).
5. **brief.md language** — `.brief.md` is NEUTRAL PROFESSIONAL SPANISH and is the ONLY Spanish artifact; it
   MUST carry the draft / Compliance-Supervisor-review disclaimer. All code, identifiers, JSON keys, the JSONL,
   and `.brief.json` stay ENGLISH. Transcript/debtor text rendered into the brief is treated as untrusted DATA.

## Impact

| Area | Impact | Description |
|------|--------|-------------|
| `cmd/harness-demo` | New | CLI driver, demo Fake provider, serialization DTO, JSON Schema, Spanish renderer, file IO |
| `data/synthetic/cases/CASE-SYN-001.json` | New | Portfolio-visible copy of the embedded synthetic Case |
| `data/synthetic/harness-runs/*` | New (runtime output) | Generated `.jsonl` / `.brief.json` / `.brief.md` |
| `internal/harness/caseflow` (orchestrator) | Modified (additive) | Optional event-observer seam; existing behavior unchanged |
| `internal/harness` (runtime), `internal/harness/labtools` | Reused | No interface or contract changes |

## Constraints

- Workflow-first: the CLI only DRIVES the #20 deterministic orchestrator + Fake provider. No autonomous
  loop, no model routing.
- Default run requires NO AWS, NO network, NO database. Deterministic for the synthetic fixture.
- Reuse #18/#19/#20; the only #20 change is the additive event seam. Smallest viable change; no future-proofing.
- Treat transcript/debtor/collector text as untrusted DATA when rendered into any artifact.
- Authority boundary preserved: outputs are DRAFTS; the Spanish brief states Compliance Supervisor review is
  required. No approval/campaign-block/ledger/override claims.
- Keep Judge and Harness model ports separate; MCP external-only; Bedrock opt-in (all out of scope here).
- Strict TDD active: behavior-focused tests before production code.

## Rollback Plan

The change is additive and isolated. Revert by removing `cmd/harness-demo`, the
`data/synthetic/cases/CASE-SYN-001.json` copy, and the additive orchestrator event-observer seam. No
migrations, no schema, no changes to #18/#19/#20 contracts to undo. Generated files under
`data/synthetic/harness-runs/` are disposable.

## Dependencies

- #18 runtime (`Runtime`, `RunStep`, `StepResult.Events`, event contract) — landed.
- #19 lab tools, `LabPermissionGate`, embedded `CASE-SYN-001` — landed.
- #20 `caseflow` orchestrator, four agents, `CaseBrief`/`HandoffArtifact` — landed (in this stacked branch).

## Decision Boundaries

- #21 owns the demo CLI, the three output artifacts, their serialization/rendering, and the additive event
  seam. It produces FILES from the #20 in-memory `CaseBrief`.
- #22 owns the Bedrock opt-in provider and hardening the input builder against real-model injection. #21
  uses only the deterministic Fake provider.
- Remote MCP, HTTP API, console, real evidence-ledger persistence, and crypto integrity remain in later slices.

## Review Workload

MEDIUM. The slice spans a new `cmd/harness-demo` (CLI + demo provider + serialization DTO + JSON Schema +
Spanish renderer + file IO), one additive orchestrator seam, a portfolio case copy, and tests. It may
approach the 400-line budget. `sdd-tasks` MUST forecast; if over budget, recommend two stacked slices:
(1) the additive `caseflow` event-observer seam + the serialization DTO/schema + tests; (2) the
`cmd/harness-demo` CLI + Spanish renderer + portfolio case file + end-to-end CLI test. Grounding flag for
design: `runAgent` currently discards `result.Events`; the observer must be threaded through `Run`→`runAgent`
without changing the existing `NewOrchestrator` four-arg signature.

## Success Criteria

- [ ] `go run ./cmd/harness-demo --case data/synthetic/cases/CASE-SYN-001.json` runs the synthetic Case
      through the #20 orchestrator with the Fake provider and exits 0 — no AWS/network/DB.
- [ ] It writes `<case_id>.jsonl`, `<case_id>.brief.json`, and `<case_id>.brief.md` under
      `data/synthetic/harness-runs/`.
- [ ] The JSONL contains only structured operational events (no chain-of-thought), annotated per agent.
- [ ] `<case_id>.brief.json` validates against the committed Case Brief JSON Schema.
- [ ] `<case_id>.brief.md` is neutral professional Spanish and includes the DRAFT / Compliance-Supervisor
      review disclaimer.
- [ ] Tests prove the three files are generated and valid for the synthetic fixture and that the run is
      deterministic; `go test ./...` passes or any failure is explicitly unrelated and evidenced.

## Recommended Next Phase

Proceed to SDD spec for `issue-21-demo-cli-case-brief`, translating `harness-demo-cli` and
`harness-case-brief-output` into requirements and scenarios, plus the `harness-case-orchestrator` event-seam
delta. Spec and design may run in parallel.
