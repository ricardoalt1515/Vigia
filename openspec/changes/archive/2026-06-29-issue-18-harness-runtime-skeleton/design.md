# Design: Issue #18 Harness Runtime Skeleton

## Objective

Create a small, test-driven `internal/harness` package that proves the core safety invariants for sandboxed Domain Agent execution before the project adds real providers, Case orchestration, demo output files, MCP, or Bedrock.

This design optimizes for boring, explicit primitives over a general-purpose agent framework.

## Non-Goals

- No Bedrock adapter.
- No MCP integration.
- No database persistence.
- No HTTP/API integration.
- No tenant auth/RLS work.
- No four Domain Agents.
- No Case orchestrator.
- No CLI or Case Brief output files.
- No evidence ledger behavior.
- No tests that assert exact model prose.

## Package Boundary

All production code for this slice should live under:

```text
internal/harness
```

Tests should live beside the package:

```text
internal/harness/*_test.go
```

No other package should need to import `internal/harness` yet. This keeps #18 as an isolated runtime-invariant slice.

## Core Types

Use one small package with explicit types. Split files only by concept if it improves readability.

Suggested files:

```text
internal/harness/runtime.go
internal/harness/model.go
internal/harness/tools.go
internal/harness/permissions.go
internal/harness/events.go
internal/harness/validation.go
internal/harness/budget.go
internal/harness/fake_model_test.go
internal/harness/runtime_test.go
```

Do not create subpackages yet. Subpackages would be premature until #19-#22 prove real seams.

### Runtime

`Runtime` coordinates one synthetic step:

```go
type Runtime struct {
    Model       ModelProvider
    Tools       ToolRegistry
    Permissions PermissionGate
    Validator   Validator
    Budget      Budget
}
```

A simple execution API is enough:

```go
func (r Runtime) RunStep(ctx context.Context, input StepInput) (StepResult, error)
```

`StepResult` should include:

- final status;
- final output when valid;
- tool result when a tool was proposed;
- structured events;
- terminal error classification for validation/budget/permission failures where useful.

Prefer returning structured failure state over hiding all failures in `error`. Use `error` for programmer/configuration errors or context cancellation.

### Model Provider

Keep the provider boundary narrow and Harness-specific:

```go
type ModelProvider interface {
    Generate(ctx context.Context, request ModelRequest) (ModelOutput, error)
}
```

`ModelOutput` should be typed enough for tests:

- optional visible plan;
- optional tool call;
- optional final output;
- raw metadata only if needed for diagnostics.

No prompts, Bedrock model IDs, streaming, token accounting, or provider SDKs in #18.

### Fake Model Provider

Implement the fake provider in tests unless production code needs a tiny test helper. The fake should return a queued sequence of `ModelOutput` values or errors so tests can prove:

- invalid then valid output;
- repeated invalid output;
- tool proposal;
- budget exhaustion.

Do not put fake behavior behind sleeps, randomness, or external files.

### Permission Model

Permission decisions should be typed:

```go
type PermissionDecisionKind string

const (
    PermissionAllowed          PermissionDecisionKind = "allowed"
    PermissionDenied           PermissionDecisionKind = "denied"
    PermissionApprovalRequired PermissionDecisionKind = "approval_required"
)
```

A decision should include a machine-readable reason where useful:

```go
type PermissionDecision struct {
    Kind   PermissionDecisionKind
    Reason string
}
```

The gate stays simple:

```go
type PermissionGate interface {
    Decide(ctx context.Context, call ToolCall) PermissionDecision
}
```

No human approval flow is implemented in #18. `approval_required` is a terminal tool result shape for later slices.

### Tool Registry and Tool Results

Use a minimal tool abstraction:

```go
type ToolCall struct {
    Name  string
    Input map[string]any
}

type Tool interface {
    Execute(ctx context.Context, call ToolCall) (ToolResult, error)
}

type ToolRegistry map[string]Tool
```

`ToolResult` should support statuses:

- `success`
- `denied`
- `approval_required`
- `not_found` if a proposed tool is missing

Denied and approval-required paths must not call the tool executor.

Do not design rich schemas or real synthetic Case tools here; #19 owns tool contracts and fixtures.

### Validation

Validation should happen immediately after model output and before accepting plan/tool/final fields:

```go
type Validator interface {
    Validate(ModelOutput) error
}
```

Runtime behavior:

1. Request model output.
2. Validate it.
3. If invalid, record `validation_failure`.
4. Retry only while retry budget/attempts allow it.
5. Never repair, coerce, or invent missing fields.

The validator can be a function adapter in production code to keep tests concise:

```go
type ValidatorFunc func(ModelOutput) error
```

### Budget

Use explicit counters, not implicit loops:

```go
type Budget struct {
    MaxModelAttempts int
    MaxToolCalls     int
}
```

Rules:

- A model attempt is consumed before each provider call.
- A tool call budget is consumed only when the runtime is about to execute an allowed tool.
- Denied and approval-required tools do not execute, so they should not consume execution budget unless implementation finds a clearer reason to count proposals separately. If proposal budgeting is needed later, add it in a later issue.
- When a budget is exceeded, record `budget_exceeded` before returning.

### Events

Event log should be in-memory for #18:

```go
type EventType string

type Event struct {
    Type EventType
    Data map[string]any
}
```

Required event constants:

- `agent_started`
- `plan_created`
- `tool_proposed`
- `permission_decision`
- `tool_result`
- `validation_failure`
- `budget_exceeded`
- `agent_completed`

Do not persist events to disk in #18. #21 owns output files.

Event data should be operational metadata only. Do not include hidden chain-of-thought. Visible plans from `ModelOutput` are acceptable because they are explicit model output, not hidden reasoning.

## Runtime Flow

Recommended `RunStep` flow:

1. Initialize event slice and record `agent_started`.
2. Check model attempt budget.
3. Call `ModelProvider.Generate`.
4. Validate output.
5. On validation failure:
   - record `validation_failure`;
   - retry if attempts remain;
   - otherwise return failed result.
6. If valid output includes a plan, record `plan_created`.
7. If valid output includes a tool call:
   - record `tool_proposed`;
   - ask `PermissionGate.Decide`;
   - record `permission_decision`;
   - if denied, return denied `ToolResult` and record `tool_result`;
   - if approval-required, return approval-required `ToolResult` and record `tool_result`;
   - if allowed, check tool-call budget, find the tool, execute it, and record `tool_result`.
8. Record `agent_completed` only for successful completion paths.
9. Return `StepResult` with all events.

Failure paths must return the event log accumulated up to the decisive failure.

## Tests

Write tests before production code where meaningful. Suggested tests:

1. `TestRunStepAllowedReadToolRecordsEvents`
   - Fake model proposes `read_case`.
   - Permission gate allows it.
   - Tool executes.
   - Result includes event sequence through `agent_completed`.

2. `TestRunStepDeniedToolDoesNotExecute`
   - Fake model proposes authority-bearing tool.
   - Permission gate denies it.
   - A spy tool proves `Execute` was not called.
   - Result has denied tool status and permission/tool events.

3. `TestRunStepApprovalRequiredToolDoesNotExecute`
   - Permission gate returns `approval_required`.
   - Tool is not executed.
   - Result preserves typed approval-required status.

4. `TestRunStepInvalidOutputFailsWithoutRepair`
   - Validator rejects output.
   - No retry or exhausted retry.
   - Event log includes `validation_failure`.
   - Runtime does not synthesize a replacement output.

5. `TestRunStepValidationRetryOnce`
   - Fake returns invalid then valid output.
   - Runtime records one validation failure.
   - Runtime calls model exactly twice.
   - Runtime completes after valid response.

6. `TestRunStepModelBudgetExceeded`
   - Invalid outputs require another attempt but model budget is exhausted.
   - Runtime records `budget_exceeded` and stops.

7. `TestRunStepToolBudgetExceeded`
   - Model proposes an allowed tool but tool budget is zero.
   - Runtime records `budget_exceeded`.
   - Tool is not executed.

Keep assertions behavioral. Event order assertions are acceptable because event order is part of the runtime contract. Avoid exact prose assertions.

## Review Workload Forecast

Expected implementation size: small to medium.

Estimated changed files:

- 5-8 files under `internal/harness`
- 1-2 test files
- no docs beyond SDD artifacts

Estimated changed lines: likely under 400 if implementation stays compact. If generated code, provider SDKs, CLI code, or DB changes appear, the scope has drifted and implementation must stop.

Chained PRs recommended: No.

Decision needed before apply: No, unless implementation discovers a need to touch paths outside `internal/harness`.

## Verification

Focused verification:

```bash
go test ./internal/harness
```

Final verification:

```bash
go test ./...
```

If unrelated packages fail because of concurrent #14 work, report exact failing packages and evidence instead of masking the failure.

## Stop Conditions

Stop and ask before continuing if:

- implementation needs to edit DB/auth/config/API/CLI paths;
- runtime design starts requiring provider-specific concepts;
- tests would only restate struct fields instead of proving behavior;
- event persistence to files becomes necessary;
- model/provider abstractions become broad enough to look like a framework rather than this #18 runtime skeleton.
