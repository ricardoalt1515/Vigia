# Explore: Issue #18 Harness Runtime Skeleton

## Source

- GitHub issue: #18 — Agent Harness Lab slice: runtime skeleton and invariant tests
- Parent epic: #16 — Agent Harness Lab: sandboxed domain-specific agents
- Dependency: #13 foundation bootstrap, already archived under `openspec/changes/archive/2026-06-29-issue-13-foundation-bootstrap/`
- Explicitly excluded by user: #14 tenant auth/RLS, currently handled by another agent

## Objective

Build the smallest shared `internal/harness` runtime skeleton that proves Vigía can constrain, validate, budget, and audit sandboxed Domain Agent execution before real providers or product integration exist.

This is a runtime-invariant slice, not the full #16 demo.

## Existing Decisions

- The compliance authority path remains workflow-first and deterministic; the Harness Lab is sandboxed and non-authority-bearing.
- Judge and Harness model ports stay separate.
- MCP is an external AI-client integration surface, not the internal harness runtime.
- Bedrock is opt-in later; tests and this slice use a Fake Model Provider only.
- Harness event logs record operational events, not hidden chain-of-thought.
- Invalid model output must fail validation without silent repair. Parent #16 allows one validation retry; #18 should prove the retry/budget primitive with deterministic fake responses.
- Risky or authority-bearing tools must return `approval_required` or `denied`; they must not execute in this slice.

## Scope for #18

Implement initial `internal/harness` primitives:

- A minimal runtime loop that can run one synthetic agent step.
- A deterministic Fake Model Provider for tests.
- Typed model output shape sufficient for a plan, proposed tool call, and final output.
- A typed permission decision model.
- A tool registry/executor seam small enough to test allowed read tools and denied/approval-required authority tools.
- Event log contract for:
  - `agent_started`
  - `plan_created`
  - `tool_proposed`
  - `permission_decision`
  - `tool_result`
  - `validation_failure`
  - `budget_exceeded`
  - `agent_completed`
- Budget enforcement for model attempts/tool calls at minimum.
- Output validation hook shape.
- Denied and approval-required tool result shapes.

## Non-Goals

- No Bedrock integration.
- No four Domain Agents.
- No deterministic Case orchestrator.
- No demo CLI.
- No output files under `data/synthetic/harness-runs` beyond existing placeholders.
- No MCP server.
- No database persistence.
- No evidence ledger.
- No exact model prose tests.
- No auth/RLS work from #14.

## Likely Implementation Area

Primary package:

- `internal/harness`

Likely test package:

- `internal/harness/*_test.go`

Existing #13 left `internal/harness` as scaffold only, so #18 can stay isolated from DB/auth/config work.

## Suggested Runtime Concepts

Keep names boring and explicit:

- `Runtime`
- `ModelProvider`
- `FakeModelProvider`
- `AgentConfig`
- `ModelOutput`
- `Plan`
- `ToolCall`
- `ToolRegistry`
- `ToolExecutor`
- `PermissionGate`
- `PermissionDecision`
- `ToolResult`
- `Validator`
- `Budget`
- `Event`
- `EventType`

Avoid abstract framework names until later slices prove they are needed.

## Testing Seams

Strict TDD is active for this repository. Meaningful seams for RED/GREEN:

1. Allowed read tool executes and records the expected event chain.
2. Authority-bearing tool is denied or approval-gated and does not execute.
3. Invalid model output emits `validation_failure` and does not silently repair.
4. Validation retry happens once when configured fake responses provide invalid then valid output.
5. Budget exhaustion stops execution and emits `budget_exceeded`.

Use table-driven Go tests where it improves clarity. Do not test exact prose from fake model output.

## Risks

- The branch has substantial existing uncommitted work from #13/#14 context. #18 must avoid DB/auth/config paths unless SDD later explicitly proves a need.
- Parent #16 includes larger concepts such as Case orchestrator, four Domain Agents, CLI, Bedrock, and output files. #18 must not absorb those; they belong to #19-#22.
- Over-abstracting the harness now would create fake architecture. Keep primitives minimal and driven by invariant tests.

## Recommendation

Proceed to SDD proposal for `issue-18-harness-runtime-skeleton`, using #18 as the bounded PRD and #16 only as parent-context constraints.
