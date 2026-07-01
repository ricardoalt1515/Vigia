# Delta for Harness Case Orchestrator

## ADDED Requirements

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
