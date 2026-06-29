# Harness Runtime Specification

## Purpose

Define the verified issue #18 Harness runtime skeleton: a small, deterministic, sandboxed runtime for one synthetic agent step using Fake Model Provider tests. The specification preserves the boundary that Harness Lab remains infrastructure for later reviewable slices and does not implement Bedrock, MCP, Case orchestration, database persistence, evidence ledger behavior, or tenant auth/RLS.

## Requirements

### Requirement: Minimal deterministic runtime step

The Harness runtime SHALL run one synthetic agent step using a deterministic Fake Model Provider.

#### Scenario: Runs one synthetic step

- **Given** an agent config with a Fake Model Provider response, an allowed read tool, a permission gate, a validator, and a budget
- **When** the runtime executes the step
- **Then** it records `agent_started`
- **And** it records `plan_created` when the model output includes a valid plan
- **And** it records `tool_proposed` for the proposed tool call
- **And** it records `permission_decision`
- **And** it records `tool_result` when the allowed tool executes
- **And** it records `agent_completed` when the step completes successfully

### Requirement: Permission decisions

The Harness runtime SHALL require an explicit typed permission decision before executing a proposed tool.

#### Scenario: Approved read tool executes

- **Given** a proposed read-only tool call that is allowed by the permission gate
- **When** the runtime evaluates the tool call
- **Then** the runtime executes the tool
- **And** the tool result status is successful
- **And** the event log includes the permission decision and tool result

#### Scenario: Denied authority-bearing tool does not execute

- **Given** a proposed authority-bearing tool call
- **And** the permission gate returns `denied`
- **When** the runtime evaluates the tool call
- **Then** the runtime does not execute the tool implementation
- **And** the tool result status is `denied`
- **And** the event log includes the permission decision and denied tool result

#### Scenario: Approval-required tool does not execute

- **Given** a proposed authority-bearing tool call
- **And** the permission gate returns `approval_required`
- **When** the runtime evaluates the tool call
- **Then** the runtime does not execute the tool implementation
- **And** the tool result status is `approval_required`
- **And** the result preserves enough typed information for a future human approval flow

### Requirement: Output validation

The Harness runtime SHALL validate model output before accepting plans, tool calls, or final output.

#### Scenario: Invalid model output fails validation

- **Given** a Fake Model Provider response that violates the configured output validator
- **When** the runtime processes the response
- **Then** it records `validation_failure`
- **And** it does not silently repair, coerce, or invent missing output
- **And** the step fails unless a configured retry succeeds

#### Scenario: Validation retry happens once

- **Given** the runtime is configured for one validation retry
- **And** the Fake Model Provider returns invalid output first and valid output second
- **When** the runtime executes the step
- **Then** it records the first `validation_failure`
- **And** it requests exactly one additional model response
- **And** it completes only if the second response validates

#### Scenario: Validation retry is not unbounded

- **Given** the runtime is configured for one validation retry
- **And** the Fake Model Provider returns invalid output repeatedly
- **When** the runtime executes the step
- **Then** it stops after the allowed attempts
- **And** it returns a validation failure result
- **And** it does not continue retrying

### Requirement: Budget enforcement

The Harness runtime SHALL enforce configured budgets for model attempts and tool calls.

#### Scenario: Model-attempt budget stops execution

- **Given** a model-attempt budget that is exhausted before a valid response is accepted
- **When** the runtime would request another model response
- **Then** it stops execution
- **And** it records `budget_exceeded`
- **And** it returns an incomplete or failed runtime result without continuing

#### Scenario: Tool-call budget stops execution

- **Given** a tool-call budget of zero or an already exhausted tool-call budget
- **And** the model proposes a tool call
- **When** the runtime evaluates the proposed tool
- **Then** it does not execute the tool
- **And** it records `budget_exceeded`
- **And** it returns an incomplete or failed runtime result without continuing

### Requirement: Structured event log contract

The Harness runtime SHALL expose structured operational events rather than hidden reasoning.

#### Scenario: Event records are typed and inspectable

- **Given** any runtime execution path
- **When** the runtime records an event
- **Then** the event includes a typed event name
- **And** the event includes enough structured metadata to understand what operational decision occurred
- **And** the event does not expose hidden chain-of-thought

#### Scenario: Failure paths record the decisive event

- **Given** validation failure, denied permission, approval-required permission, or budget exhaustion
- **When** the runtime stops or skips execution because of that condition
- **Then** the event log includes the decisive event before returning the result

### Requirement: Fake provider only for tests

The #18 implementation SHALL use Fake Model Provider for deterministic invariant tests and SHALL NOT require live model credentials.

#### Scenario: Tests run without external AI provider

- **Given** a clean local test environment without AWS or Bedrock credentials
- **When** `go test ./...` runs
- **Then** Harness invariant tests execute using Fake Model Provider only
- **And** no live model provider is called

### Requirement: Scope isolation

The #18 implementation SHALL remain isolated from future Agent Harness Lab slices and concurrent tenant-auth work.

#### Scenario: No full #16 behavior is implemented

- **Given** this change targets only #18
- **When** implementation is complete
- **Then** it does not include Bedrock integration, four Domain Agents, Case orchestration, demo CLI, Case Brief output files, MCP, database persistence, or evidence ledger behavior

#### Scenario: No #14 auth/RLS behavior is implemented

- **Given** issue #14 is handled by another agent
- **When** implementation is complete
- **Then** it does not add tenant API-key auth, request middleware, RLS tenant context setting, or cross-tenant API behavior
