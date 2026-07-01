# Harness Demo CLI Specification

## Purpose

`cmd/harness-demo` is a local, cloud-free CLI that drives the #20 `caseflow.Orchestrator` against a
deterministic Fake Model Provider, so the Agent Harness Lab is runnable and its outputs are inspectable
without writing Go test code.

## ADDED Requirements

### Requirement: CLI accepts a `--case` path and defaults to the synthetic case

The CLI SHALL accept a `--case <path>` flag identifying a synthetic Case JSON file. When the flag is
omitted, the CLI SHALL default to `data/synthetic/cases/CASE-SYN-001.json`. The CLI SHALL read the file
only to obtain `case_id`, then run that id against the embedded lab store (`labtools.Load()`); it MUST NOT
use the file's other contents as an alternate data source.

#### Scenario: Default run resolves to the embedded synthetic case

- **GIVEN** the CLI is invoked with no `--case` flag
- **WHEN** it starts
- **THEN** it resolves `case_id` from `data/synthetic/cases/CASE-SYN-001.json`
- **AND** it runs that `case_id` against the embedded `labtools` store

#### Scenario: Explicit `--case` path is honored

- **GIVEN** the CLI is invoked with `--case /some/path/case.json`
- **WHEN** the file contains a valid `case_id`
- **THEN** the CLI resolves and runs that `case_id`

### Requirement: Only `CASE-SYN-001` is runnable; unsupported case ids fail fast

The CLI SHALL run only the scripted synthetic case `CASE-SYN-001`. When the resolved `case_id` has no
matching scripted Fake provider script, the CLI MUST exit with a non-zero status, print a clear
"unsupported synthetic case" message to stderr, and MUST NOT write any of the three output files.

#### Scenario: Unsupported case id exits non-zero and writes nothing

- **GIVEN** a `--case` file whose `case_id` is not `CASE-SYN-001`
- **WHEN** the CLI runs
- **THEN** it exits with a non-zero status code
- **AND** it prints a clear message identifying the case as an unsupported synthetic case
- **AND** no `.jsonl`, `.brief.json`, or `.brief.md` file is written under `data/synthetic/harness-runs/`

### Requirement: Default run uses a demo-only deterministic Fake provider with no cloud dependencies

The CLI SHALL construct a demo-only `ProviderFactory` returning scripted, deterministic responses per
agent for `CASE-SYN-001`, mirroring the #20 test `perAgentProvider` pattern. The default run MUST NOT
require AWS credentials, network access, or a database connection. The CLI SHALL use the real
`LabPermissionGate` and real lab tools from #19 when executing the orchestrator.

#### Scenario: Default run requires no external services

- **GIVEN** a clean environment with no AWS credentials, no network access, and no database connection
- **WHEN** `go run ./cmd/harness-demo` is executed with default flags
- **THEN** the run completes and exits without any external-service error

### Requirement: Successful runs exit 0 and write all three output artifacts

When the orchestrator run completes (whether the resulting `CaseBrief.Status` is `complete` or
`incomplete`) without an unsupported-case or CLI-level failure, the CLI SHALL exit 0 and write all three
artifacts — `<case_id>.jsonl`, `<case_id>.brief.json`, `<case_id>.brief.md` — under
`data/synthetic/harness-runs/`.

#### Scenario: Default run against CASE-SYN-001 exits 0 and writes all three files

- **GIVEN** the default Fake provider script produces valid handoffs for all four agents
- **WHEN** `go run ./cmd/harness-demo` is executed with default flags
- **THEN** the process exits with status code 0
- **AND** `data/synthetic/harness-runs/CASE-SYN-001.jsonl` exists
- **AND** `data/synthetic/harness-runs/CASE-SYN-001.brief.json` exists
- **AND** `data/synthetic/harness-runs/CASE-SYN-001.brief.md` exists

### Requirement: CLI-level run failures exit non-zero without partial output

If the CLI cannot construct the orchestrator, read the case file, or an unexpected (non-orchestrator)
error occurs before artifacts are fully written, the CLI SHALL exit with a non-zero status and MUST NOT
leave partially written output files under `data/synthetic/harness-runs/`.

#### Scenario: Malformed case file exits non-zero and writes nothing

- **GIVEN** a `--case` path pointing to a file that is not valid JSON
- **WHEN** the CLI runs
- **THEN** it exits with a non-zero status code
- **AND** no output files are written under `data/synthetic/harness-runs/`

### Requirement: The CLI is workflow-first and performs no autonomous routing

The CLI SHALL only drive the deterministic #20 orchestrator with the Fake provider; it MUST NOT implement
an autonomous agent loop, model-based routing, or any mechanism that lets model output select which
agents run or in what order.

#### Scenario: Agent invocation order is fixed regardless of Fake provider content

- **GIVEN** the Fake provider script for `CASE-SYN-001`
- **WHEN** the CLI runs the orchestrator
- **THEN** the four Domain Agents execute in the fixed order defined by #20, unaffected by scripted model
  content
