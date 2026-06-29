# Proposal: Issue #18 Harness Runtime Skeleton

## Problem

Vigía needs to prove its Agent Harness safety model before adding real providers, Domain Agents, CLI outputs, MCP, database persistence, or authority-bearing workflows. The project already decided that compliance authority remains workflow-first and deterministic; the early Harness Lab exists to demonstrate sandboxed agent execution with visible constraints, not to make autonomous compliance decisions.

Without a small runtime skeleton first, later #16 slices would risk mixing provider integration, Case orchestration, output rendering, and safety invariants in one large change. That would make the harness harder to test, review, and trust.

## Goal

Implement the smallest shared `internal/harness` runtime skeleton that can run one deterministic synthetic agent step through a Fake Model Provider while enforcing and recording core safety invariants:

- model output validation;
- permission decisions;
- tool result shapes for allowed, denied, and approval-required outcomes;
- budget enforcement;
- structured operational event logging.

This proposal covers GitHub issue #18 only.

## Users and Outcomes

### Compliance Supervisor outcome

No direct product-facing workflow changes yet. This slice is infrastructure for later Case Brief work, ensuring future sandboxed Domain Agents are auditable before their outputs are shown to supervisors.

### Technical reviewer outcome

A reviewer can inspect behavior-focused Go tests and see that the Harness runtime:

- executes only approved read-like tools;
- blocks or approval-gates authority-bearing tools;
- emits structured events for important runtime decisions;
- stops when validation or budget rules fail;
- uses deterministic fake model responses in tests.

## Scope

Create initial `internal/harness` primitives for:

- minimal runtime loop for one synthetic agent step;
- `ModelProvider` interface and deterministic `FakeModelProvider` for tests;
- typed model output shape with plan/tool/final-output concepts as needed by #18;
- typed permission decision model;
- small tool registry/executor seam;
- event contract for:
  - `agent_started`
  - `plan_created`
  - `tool_proposed`
  - `permission_decision`
  - `tool_result`
  - `validation_failure`
  - `budget_exceeded`
  - `agent_completed`
- budget limits for model attempts and tool calls;
- output validation hook shape;
- denied and approval-required tool result shapes;
- invariant tests using Fake Model Provider only.

## Non-Goals

- No Bedrock integration.
- No four Domain Agents.
- No deterministic Case orchestrator.
- No demo CLI.
- No generated Case Brief files.
- No MCP server.
- No database persistence.
- No evidence ledger.
- No tenant auth/RLS work from issue #14.
- No exact model prose assertions.
- No product UI or HTTP API.

## Acceptance Summary

The change is complete when:

- `internal/harness` contains a minimal runtime loop that can run one synthetic agent step with Fake Model Provider.
- Runtime records structured events for applicable #18 event types.
- Permission gate allows an approved read tool and denies or approval-gates an authority-bearing tool.
- Budget limits stop execution and emit `budget_exceeded`.
- Invalid model output fails validation without silent repair.
- Tests prove permission, validation, retry/budget primitives using Fake Model Provider only.
- `go test ./...` passes, or any failure is explicitly unrelated and evidenced.

## Constraints

- Strict TDD is active. Add behavior-focused tests before production code at meaningful seams.
- Keep implementation isolated to `internal/harness` unless later SDD phases prove a minimal adjacent change is required.
- Do not touch #14 tenant auth/RLS scope.
- Do not broaden this into the full #16 epic.
- Keep technical artifacts in English.
- Preserve current uncommitted work from other slices.

## Decision Boundaries

This proposal intentionally decides only the runtime-invariant foundation. Later issues own later decisions:

- #19 owns richer tool contracts and synthetic Case fixture.
- #20 owns deterministic Case orchestrator and Domain Agent instances.
- #21 owns demo CLI and Case Brief outputs.
- #22 owns Bedrock Claude opt-in provider.
- #14 owns tenant auth/RLS and must remain separate.

## Risks

- Parent #16 has broader scope; implementation must avoid absorbing future slices.
- A premature abstraction-heavy runtime would create framework debt. The design should stay minimal and test-driven.
- The active branch contains substantial unrelated uncommitted work. Implementation must avoid DB/auth/config paths and report changed files clearly.
- Validation retry behavior must be explicit: retry once only where configured by the runtime, never silent repair.

## Recommended Next Phase

Proceed to SDD spec for `issue-18-harness-runtime-skeleton`, translating the acceptance summary into concrete requirements and scenarios.
