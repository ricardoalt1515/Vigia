# Design: Issue #21 Harness Demo CLI and Case Brief Outputs

## Technical Approach

Add a self-contained `cmd/harness-demo` main package that DRIVES the existing #20
deterministic `caseflow` orchestrator with a demo-only Fake provider and writes three
files. The only production-code change is ONE additive, backward-compatible functional
option on `caseflow.NewOrchestrator` that surfaces per-agent `harness.Event`s. Everything
else (DTO flattening, JSON Schema, Spanish rendering, Fake provider, file IO) lives in
`cmd/harness-demo` so `caseflow`, the #18 runtime, and #19 lab tools stay untouched. No
network, AWS, DB, or autonomous loop is introduced. This maps directly to the proposal's
five resolved questions.

## Architecture Decisions

### Decision: Event seam is a variadic functional option, not a signature change

**Choice**: Append `opts ...OrchestratorOption` to `NewOrchestrator`; add an unexported
`observer EventObserver` field (nil = current behavior). `runAgent` gains an `observer`
parameter and calls it after every `RunStep`.
**Alternatives considered**: (a) fifth positional param; (b) wrapping the `Runtime`;
(c) returning events from `Run`.
**Rationale**: Variadic options keep the existing four-arg call sites and all #20 tests
compiling verbatim. Positional param breaks callers; wrapping the runtime touches #18;
returning events changes the `Run`/`CaseBrief` contract. Option is the smallest additive seam.

### Decision: Forward-only DTO via concrete type-switch (no interface round-trip)

**Choice**: A CLI-local `briefDTO` flattens `CaseBrief`; `marshalHandoff` type-switches on
the concrete `HandoffArtifact` implementations and marshals each via its existing json tags.
**Alternatives considered**: custom `MarshalJSON` on `CaseBrief` in `caseflow` (rejected —
mutates #20 types); reflection/`map[string]any` (rejected — loses schema fidelity).
**Rationale**: Output is one-directional; no unmarshal into the interface is needed. Keeps
#20 types intact and honors the #20/#21 boundary ("#21 owns Case Brief file rendering").

### Decision: Case-id guard gates the run, not the Fake provider

**Choice**: The CLI reads `--case` JSON only to extract `case_id`; if it is not
`CASE-SYN-001` it exits code 2 BEFORE constructing the orchestrator. The Fake provider is
scripted per agent for that one case.
**Rationale**: Deterministic, cloud-free, avoids a generic case-injection seam (proposal Q1).

### Decision: Schema validated offline with `santhosh-tekuri/jsonschema/v6`

**Choice**: Commit a Draft 2020-12 schema at `cmd/harness-demo/schema/case_brief.schema.json`;
tests validate the emitted `.brief.json` against it with this pure-Go, offline library.
**Alternatives considered**: hand-rolled structural assertions (rejected — never exercises
the committed schema, the actual contract artifact).
**Rationale**: Real validation of the shipped contract with no network at test time.

## Data Flow

    --case file ──read case_id──> guard(CASE-SYN-001?) ──no──> exit 2
         │ yes
         ▼
    labtools.Load() ─> Registry+Gate ─┐
    demoProviderFactory ──────────────┤
    caseflow.AllAgentDefinitions() ───┼─> NewOrchestrator(..., WithEventObserver(sink))
                                       ▼
                            Orchestrator.Run(ctx, "CASE-SYN-001")
                    observer(agentName, events)│   returns CaseBrief
                                 ▼             ▼
                       eventlog(JSONL)   briefDTO ─> .brief.json (schema-valid)
                                                └──> render ─> .brief.md (Spanish + disclaimer)

## Interfaces / Contracts

Additive seam in `internal/harness/caseflow/orchestrator.go`:

```go
type EventObserver func(agentName string, events []harness.Event)
type OrchestratorOption func(*Orchestrator)

func WithEventObserver(obs EventObserver) OrchestratorOption {
    return func(o *Orchestrator) { o.observer = obs }
}

// signature: first four params UNCHANGED; variadic appended.
func NewOrchestrator(factory ProviderFactory, registry harness.ToolRegistry,
    gate harness.PermissionGate, defs []AgentDefinition,
    opts ...OrchestratorOption) (*Orchestrator, error)
```

`Orchestrator` gains `observer EventObserver`. `Run` passes `o.observer` into `runAgent`;
inside `runAgent`, immediately after each `result, err := rt.RunStep(...)` returns without a
transport error, call `if observer != nil { observer(def.Name, result.Events) }` — so every
event kind (agent_started … agent_completed, validation_failure, permission_decision,
budget_exceeded) is surfaced per step, then the existing switch runs unchanged.

CLI DTO + marshaler (`cmd/harness-demo/brief.go`):

```go
type briefDTO struct {
    CaseID        string     `json:"case_id"`
    Status        string     `json:"status"`
    Stages        []stageDTO `json:"stages"`
    FailedAgent   string     `json:"failed_agent,omitempty"`
    FailureReason string     `json:"failure_reason,omitempty"`
}
type stageDTO struct {
    AgentName string          `json:"agent_name"`
    Kind      string          `json:"kind"`
    Handoff   json.RawMessage `json:"handoff"`
}
func marshalHandoff(h caseflow.HandoffArtifact) (string, json.RawMessage, error) {
    switch v := h.(type) {
    case *caseflow.PolicyExplanation:     return kindJSON(v)
    case *caseflow.CaseInvestigation:     return kindJSON(v)
    case *caseflow.EvidenceManifestDraft: return kindJSON(v)
    case *caseflow.SupervisorNoteDraft:   return kindJSON(v)
    default: return "", nil, fmt.Errorf("unknown handoff kind %q", h.Kind())
    }
}
```

`kindJSON` returns `h.Kind()` plus `json.Marshal(v)`. Incomplete runs marshal fine: `Stages`
is partial and `failed_agent`/`failure_reason` are populated; both shapes are one schema.

JSON Schema (`schema/case_brief.schema.json`, Draft 2020-12): `status` enum
`["complete","incomplete"]`; `stages[].handoff` is a `oneOf` of the four handoff shapes
selected by the sibling `kind` discriminator via `if/then`; `failed_agent`/`failure_reason`
optional so the incomplete shape validates.

Exit codes: `0` = the three artifacts were written (a schema-valid INCOMPLETE brief is a
legitimate success — evidence is still produced); `2` = usage/unsupported-case error (bad
flag, unreadable `--case`, `case_id != CASE-SYN-001`), detected before running; `1` =
infrastructure failure (store load, marshal, schema-render, or file write). Distinct `2`
lets scripts separate operator error from run/IO failure.

### Decision: All-or-nothing artifact write via temp-dir-then-rename

**Choice**: `main.go` renders all three payloads (JSONL bytes, `.brief.json` bytes,
`.brief.md` bytes) fully in memory FIRST. Only after all three render without error does it
write them — each via `os.CreateTemp` in the target directory (`data/synthetic/harness-runs/`)
followed by `os.Rename` to the final `<case_id>.{jsonl,brief.json,brief.md}` path. If any
render step fails, nothing is written and the CLI exits 1. If a write/rename step fails after
some files already landed, the CLI removes any of the three final paths it already created for
this run before exiting 1.
**Alternatives considered**: sequential direct writes (rejected — a mid-sequence failure, e.g.
JSONL succeeds then schema-render fails, leaves a partial `.jsonl` on disk, violating the
harness-demo-cli spec's "MUST NOT leave partially written output files"); writing to a
temp subdirectory then moving all three as one step (rejected — added directory-management
complexity with no benefit over per-file temp-then-rename, since `os.Rename` within the same
volume is already atomic per file).
**Rationale**: Rendering before writing means a render-time error (e.g. marshal failure)
never touches disk. Per-file temp-then-rename makes each individual file's appearance atomic;
the best-effort cleanup on a late write failure satisfies the spec's no-partial-output
requirement in the target directory even though the three files are not written under one
filesystem transaction (Go's stdlib has no multi-file transaction primitive, and this project
does not depend on one for the demo CLI).

## Spanish Brief Renderer (`render.go`)

Neutral professional Spanish, single template. Sections: opening DISCLAIMER block
("BORRADOR — requiere revisión del Supervisor de Cumplimiento"); `Resumen del caso`
(case_id, estado); one section per stage keyed by handoff kind (Política aplicable,
Investigación, Manifiesto de evidencia (borrador), Nota para el supervisor (borrador)); a
`Fallo` section when incomplete; closing DISCLAIMER repeated. Untrusted transcript/debtor/
collector free-text fields (`Evidence`, `Analysis`, `Findings`, `NoteBody`, rule
`PlainLanguage`) pass through `renderUntrusted`: escape markdown control chars, never
interpolate into headings, and place multi-line values inside fenced code blocks with any
internal ``` sequence neutralized — so untrusted data cannot inject markdown or be read as
instructions.

## Demo Fake ProviderFactory (`provider.go`)

`demoProviderFactory(name string) harness.ModelProvider` returns a `*scriptedProvider` (a
queued `ModelOutput` list) per the four agent names, mirroring the #20 e2e
`perAgentProvider` but living in `package main` so demo data never enters `caseflow`. Each
agent gets a tool-call step then a synthesis step scripted for CASE-SYN-001. Unknown agent
name panics defensively; unknown case ids never reach here (guarded at exit 2).

## File Changes

| File | Action | Description |
|------|--------|-------------|
| `internal/harness/caseflow/orchestrator.go` | Modify (additive) | `EventObserver`, `OrchestratorOption`, `WithEventObserver`, `observer` field, variadic `NewOrchestrator`, observer call in `runAgent` |
| `cmd/harness-demo/main.go` | Create | Flag parsing, case-id guard, wiring, exit codes, render-then-temp-write-then-rename all-or-nothing artifact commit |
| `cmd/harness-demo/provider.go` | Create | Demo scripted Fake `ProviderFactory` for CASE-SYN-001 |
| `cmd/harness-demo/brief.go` | Create | `briefDTO`, `marshalHandoff`, `.brief.json` writer |
| `cmd/harness-demo/render.go` | Create | Spanish `.brief.md` renderer + `renderUntrusted` |
| `cmd/harness-demo/eventlog.go` | Create | JSONL sink accumulating `(agent_name, seq, event)` |
| `cmd/harness-demo/schema/case_brief.schema.json` | Create | Committed Draft 2020-12 Case Brief schema |
| `data/synthetic/cases/CASE-SYN-001.json` | Create | Portfolio mirror of the embedded fixture |
| `cmd/harness-demo/*_test.go` | Create | Behavior tests (see Testing Strategy) |
| `go.mod` / `go.sum` | Modify | Add test-only `santhosh-tekuri/jsonschema/v6` |

## Testing Strategy

| Layer | What to Test | Approach |
|-------|-------------|----------|
| Unit | `marshalHandoff` covers all 4 kinds + unknown error; DTO shape | Table-driven per handoff kind |
| Unit | `renderUntrusted` neutralizes markdown/fence injection; disclaimer present; Spanish | Table-driven with adversarial inputs |
| Unit | Seam: `WithEventObserver` receives per-agent events; nil-observer path unchanged | `caseflow` test with a recording observer |
| Integration | `.brief.json` validates against committed schema (complete + incomplete) | `santhosh-tekuri/jsonschema/v6`, offline |
| E2E | `run()` on CASE-SYN-001 writes 3 files to `t.TempDir()`, exit 0, deterministic on repeat; bad case id exits 2 | In-process `run(args, dir)`, no network/DB/Bedrock |
| E2E | A forced render/write failure after a successful render leaves zero files in the output dir (no partial `.jsonl` etc.) | Inject a failing writer/render step in `run()`, assert `t.TempDir()` is empty |

All tests are network/DB/Bedrock-free per project constraints; filesystem tests use
`t.TempDir()`.

## Migration / Rollout

No migration. Additive and revertable: drop `cmd/harness-demo`,
`data/synthetic/cases/CASE-SYN-001.json`, the go.mod test dep, and the orchestrator seam.
Generated `data/synthetic/harness-runs/*` are disposable. No schema/DB/#18/#19/#20 contract
changes to undo.

## Review Workload Forecast

`400-line budget risk: High`. `Chained PRs recommended: Yes`. `Decision needed before apply:
Yes`. I AGREE with the proposal's two-slice split; `sdd-tasks` should finalize it:

- Slice 1: additive `caseflow` event-observer seam + `briefDTO`/`marshalHandoff` +
  committed JSON Schema + their unit/integration tests. Self-contained, independently
  verifiable, low blast radius on #20.
- Slice 2: `cmd/harness-demo` CLI (`main`, `provider`, `render`, `eventlog`) + Spanish
  renderer + `data/synthetic/cases/CASE-SYN-001.json` + e2e CLI test.

Slice 1 must land first because Slice 2's DTO/schema and event JSONL depend on it.

## Open Questions

None. `santhosh-tekuri/jsonschema/v6` is CONFIRMED as the offline schema-validation
dependency: it is pure-Go, requires no network at test time, is test-scoped only (not
imported by any non-test production file), and validates the actual committed schema
artifact rather than a hand-rolled proxy for it — consistent with this project's existing
tolerance for test-only dependencies.
