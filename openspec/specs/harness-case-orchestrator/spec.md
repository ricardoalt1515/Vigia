# Harness Case Orchestrator Specification

## ADDED Requirements

### Requirement: Deterministic fixed-order invocation of four Domain Agents

The Case orchestrator SHALL invoke the four Domain Agents in the following hardcoded order:
1. `PolicyExplainer`
2. `CaseInvestigator`
3. `EvidencePackager`
4. `SupervisorNoteDrafter`

This order MUST be determined by Go code, not by any model output, runtime configuration value, or external
input. The orchestrator MUST NOT use model-based routing, LLM planners, conditional dispatch, or any
dynamic mechanism to determine the agent sequence.

#### Scenario: All four agents are invoked in fixed order with Fake Model Provider

- **Given** a deterministic orchestrator and a Fake Model Provider configured to produce valid handoffs for
  each agent
- **When** the orchestrator executes for synthetic Case `CASE-SYN-001`
- **Then** the invocation sequence recorded in the CaseBrief stages is: `PolicyExplainer`,
  `CaseInvestigator`, `EvidencePackager`, `SupervisorNoteDrafter` — in that order
- **And** no model output influenced the ordering or selection of agents

---

### Requirement: Each agent receives only the approved Case input and prior Handoff Artifacts; no shared hidden state

When the orchestrator invokes an agent, the agent's input context SHALL contain exactly:
- The approved Case reference (`case_id`) from the orchestrator's original input
- The typed Handoff Artifacts produced by all previously completed agents, in fixed order

An agent MUST NOT receive raw conversation history of prior agents, intermediate tool events, model
chain-of-thought from prior runs, internal `StepResult` event lists, or any data not explicitly carried by
a typed Handoff Artifact.

#### Scenario: CaseInvestigator receives only case_id and PolicyExplanation; no prior-agent internal state

- **Given** a run where `PolicyExplainer` has completed and produced a `PolicyExplanation`
- **When** `CaseInvestigator` is invoked
- **Then** its input contains the approved `case_id` and the `PolicyExplanation` struct
- **And** no raw output, event list, intermediate step trace, or model chain-of-thought from
  `PolicyExplainer`'s run is present in the context

---

### Requirement: The CaseBrief terminal state carries complete or incomplete status with ordered handoffs and failure details

At the end of every orchestrator run the orchestrator SHALL produce a `CaseBrief` value with the following
fields:
- `CaseID` — echoed from the orchestrator's input
- `Status` — exactly `complete` or `incomplete`; no other values are valid
- `Stages` — an ordered list of all successfully produced Handoff Artifacts, each annotated with its
  producing agent name
- `FailedAgent` — name of the agent that was stopped (populated only when `Status == incomplete`)
- `FailureReason` — reason string derived from the terminal validation failure event (populated only when
  `Status == incomplete`)

#### Scenario: CaseBrief is complete when all four agents succeed

- **Given** a Fake Model Provider producing valid handoffs for all four agents
- **When** the orchestrator finishes the run
- **Then** `CaseBrief.Status` is `complete`
- **And** `Stages` contains exactly four entries in the fixed invocation order
- **And** `FailedAgent` and `FailureReason` are empty

#### Scenario: CaseBrief is incomplete when an agent fails both validation attempts

- **Given** a Fake Model Provider causing `EvidencePackager` to fail validation on both synthesis attempts
- **When** the orchestrator runs
- **Then** `CaseBrief.Status` is `incomplete`
- **And** `CaseBrief.FailedAgent` is `EvidencePackager`
- **And** `CaseBrief.FailureReason` is derived from the second validation failure event, not the first
- **And** `Stages` contains only the handoffs produced before `EvidencePackager` was invoked

---

### Requirement: Orchestrator stops after second validation failure; downstream agents are not invoked

After an agent's second consecutive validation failure, the orchestrator SHALL immediately stop the run and
produce an `incomplete` CaseBrief. The orchestrator MUST NOT invoke any agent that appears later in the
fixed order, regardless of whether those later agents could succeed independently.

#### Scenario: Downstream agents are not invoked after a failing agent

- **Given** a Fake Model Provider causing `PolicyExplainer` to fail validation on both attempts
- **When** the orchestrator runs
- **Then** `CaseInvestigator`, `EvidencePackager`, and `SupervisorNoteDrafter` are never invoked
- **And** `CaseBrief.FailedAgent` is `PolicyExplainer`
- **And** `CaseBrief.Status` is `incomplete`

---

### Requirement: All orchestrator and Domain Agent behavior is exercisable with the Fake Model Provider and no external services

The orchestrator and all Domain Agents MUST be fully testable using the injected Fake/deterministic
`ModelProvider` from the #18 runtime. No test SHALL require a network call, database connection, Bedrock
access, file system write, or any other external service. Tests MUST be deterministic: identical Fake
provider configuration and identical input always produce structurally identical `CaseBrief` output.

#### Scenario: A complete orchestrator run passes in a network-free test environment

- **Given** a clean test environment with no running database, no network access, and no Bedrock service
- **And** a Fake Model Provider configured to produce valid handoffs for all four agents
- **When** the orchestrator executes for `CASE-SYN-001`
- **Then** `CaseBrief.Status` is `complete`
- **And** repeated invocations with the same Fake provider configuration produce structurally identical
  output

---

### Requirement: Optional event-observer functional option surfaces per-agent operational events

`caseflow.NewOrchestrator` SHALL accept an optional, backward-compatible functional option that registers
an event observer. When an observer is supplied, the orchestrator SHALL invoke it with `(agentName,
events)` once per completed `RunStep` call inside `runAgent`, using the same `harness.Event` slice
currently produced by `StepResult.Events` and currently discarded. When no observer is supplied, the
orchestrator's behavior MUST be identical to today: `runAgent` continues to discard `result.Events`.

#### Scenario: Supplied observer receives events per RunStep, annotated by agent

- **GIVEN** an orchestrator constructed with an event-observer option
- **AND** a Fake Model Provider producing valid handoffs for all four agents
- **WHEN** the orchestrator runs `CASE-SYN-001`
- **THEN** the observer is invoked once per agent's `RunStep` call
- **AND** each invocation receives the agent's name and its `harness.Event` slice from that step
- **AND** the observed events cover all four agents in fixed invocation order

#### Scenario: Observer is invoked for the failing agent's step before the run stops

- **GIVEN** an orchestrator constructed with an event-observer option
- **AND** a Fake Model Provider causing one agent to fail validation on both attempts
- **WHEN** the orchestrator runs and stops after the second validation failure
- **THEN** the observer has been invoked for that agent's `RunStep` calls, including the step containing
  the terminal `validation_failure` event
- **AND** the observer is not invoked for any downstream, un-invoked agent

#### Scenario: No observer supplied preserves current discard behavior

- **GIVEN** an orchestrator constructed without the event-observer option (existing four-arg call)
- **WHEN** the orchestrator runs
- **THEN** `runAgent` behaves exactly as before the change: `result.Events` is not surfaced anywhere
- **AND** the run's `CaseBrief` output is unaffected by the presence or absence of an observer

---

### Requirement: The event-observer option is additive and does not change existing construction or execution semantics

Adding the event-observer option MUST NOT change the existing four-argument `NewOrchestrator(factory,
registry, gate, defs)` signature, the fixed agent invocation order, or `CaseBrief` field semantics
established by the current `harness-case-orchestrator` specification. All existing #20 tests MUST continue
to compile and pass unchanged.

#### Scenario: Existing four-arg NewOrchestrator calls compile and pass unchanged

- **GIVEN** the #20 test suite calling `NewOrchestrator(factory, registry, gate, defs)` with no options
- **WHEN** the codebase is built and tested after adding the event-observer option
- **THEN** all existing #20 tests compile without modification
- **AND** all existing #20 tests pass with unchanged results
