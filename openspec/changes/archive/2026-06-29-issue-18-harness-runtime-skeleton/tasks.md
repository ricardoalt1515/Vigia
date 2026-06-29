# Tasks: Issue #18 Harness Runtime Skeleton

## Review Workload Forecast

- Estimated changed lines: under 400 if implementation stays inside `internal/harness`.
- Chained PRs recommended: No.
- 400-line budget risk: Medium. The runtime can stay small, but tests plus types may approach the limit.
- Decision needed before apply: No, unless implementation needs to touch paths outside `internal/harness`.
- Delivery strategy: single reviewable work unit is acceptable for #18.

## Implementation Tasks

- [x] Add failing runtime test for an allowed read tool executing through Fake Model Provider.
  - Prove event sequence includes `agent_started`, `plan_created`, `tool_proposed`, `permission_decision`, `tool_result`, and `agent_completed`.
  - Do not assert exact model prose.

- [x] Implement the minimal model, tool, permission, event, budget, validation, and runtime types needed to satisfy the allowed-tool test.
  - Keep code inside `internal/harness`.
  - Avoid subpackages and provider-specific concepts.

- [x] Add failing test for a denied authority-bearing tool.
  - Prove the permission decision is `denied`.
  - Prove the tool executor is not called.
  - Prove the returned tool result status is `denied`.

- [x] Implement denied tool-result handling.
  - Record `permission_decision` and `tool_result` events.
  - Do not execute the tool when denied.

- [x] Add failing test for an approval-required authority-bearing tool.
  - Prove the permission decision is `approval_required`.
  - Prove the tool executor is not called.
  - Prove the result preserves approval-required status and reason.

- [x] Implement approval-required tool-result handling.
  - Keep it terminal for #18; do not add a human approval flow.

- [x] Add failing test for invalid model output.
  - Validator rejects the output.
  - Runtime records `validation_failure`.
  - Runtime does not silently repair, coerce, or invent missing output.

- [x] Implement validation failure handling.
  - Validate before accepting plan, tool call, or final output.
  - Return structured failure state with accumulated events.

- [x] Add failing test for one validation retry.
  - Fake Model Provider returns invalid output first and valid output second.
  - Runtime calls the model exactly twice.
  - Runtime records exactly one `validation_failure` before completing.

- [x] Implement bounded validation retry.
  - Retry only while configured model-attempt budget allows it.
  - Do not create unbounded loops.

- [x] Add failing test for model-attempt budget exhaustion.
  - Runtime records `budget_exceeded`.
  - Runtime stops without another model call after budget is exhausted.

- [x] Implement model-attempt budget enforcement.
  - Consume attempts before provider calls.
  - Emit `budget_exceeded` before returning.

- [x] Add failing test for tool-call budget exhaustion.
  - Model proposes an allowed tool.
  - Tool-call budget is zero.
  - Runtime records `budget_exceeded`.
  - Tool executor is not called.

- [x] Implement tool-call budget enforcement.
  - Check budget before executing an allowed tool.
  - Do not count denied or approval-required paths as executed tool calls.

- [x] Run focused verification.
  - Command: `go test ./internal/harness`.
  - Fix failures inside #18 scope only.

- [x] Run full Go verification.
  - Command: `go test ./...`.
  - If unrelated concurrent work fails, report exact package/error evidence.

- [x] Review scope before completion.
  - Confirm changed production/test files are limited to `internal/harness` plus SDD artifacts.
  - Confirm no Bedrock, MCP, DB, CLI, Case orchestrator, evidence ledger, or #14 auth/RLS behavior was added.

## Apply Notes

- Strict TDD is active. Start each behavior with a failing test before production code where meaningful.
- Tests should prove runtime behavior and invariants, not field declarations.
- Stop before editing outside `internal/harness` unless the user explicitly approves a design change.
