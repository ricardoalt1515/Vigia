# Harness Demo CLI Specification

## Purpose

This delta adds an additive `--provider {fake|bedrock}` flag to `cmd/harness-demo` (defined in #21) so the demo CLI can optionally drive the #20 `caseflow.Orchestrator` with the #22 real Bedrock Claude `ModelProvider` instead of only the deterministic Fake provider. The default `fake` path's behavior, outputs, and exit codes are unchanged from #21. This delta does not restate #21's existing requirements (`--case` handling, output artifacts, failure/exit-code contract for the `fake` path) — see `openspec/specs/harness-demo-cli/spec.md` for those.

## MODIFIED Requirements

### Requirement: CLI accepts an optional `--provider` flag selecting the Model Provider, defaulting to `fake`

The CLI SHALL accept a `--provider {fake|bedrock}` flag. When the flag is omitted, or explicitly set to `fake`, the CLI's behavior, output artifacts, and exit codes SHALL be identical to the #21 CLI's default behavior. When set to `bedrock`, the CLI SHALL construct the run using the #22 Bedrock `caseflow.ProviderFactory` instead of the demo-only Fake `ProviderFactory`. Any value other than `fake` or `bedrock` SHALL be treated as a usage error.

#### Scenario: Omitting --provider preserves #21 default behavior

- **GIVEN** the CLI is invoked with no `--provider` flag and a valid `--case` (or its default)
- **WHEN** the run completes
- **THEN** the CLI's exit code, stderr/stdout messages, and written artifacts are byte-for-byte identical to the #21 default behavior for the same inputs

#### Scenario: Explicit --provider fake preserves #21 default behavior

- **GIVEN** the CLI is invoked with `--provider fake` and a valid `--case` (or its default)
- **WHEN** the run completes
- **THEN** the CLI's exit code, stderr/stdout messages, and written artifacts are byte-for-byte identical to the #21 default behavior for the same inputs

#### Scenario: Unknown --provider value fails as a usage error

- **GIVEN** the CLI is invoked with `--provider something-else`
- **WHEN** it starts
- **THEN** it exits with status code 2
- **AND** it prints a clear usage error naming the invalid `--provider` value
- **AND** it does not attempt to load Bedrock configuration, construct any orchestrator, or write any output files

### Requirement: `--provider bedrock` with valid configuration and credentials constructs the Bedrock adapter and runs

When `--provider bedrock` is selected and `AWS_REGION`, `BEDROCK_MODEL_ID`, and resolvable AWS credentials are present, the CLI SHALL load configuration via `internal/config`, construct the #22 Bedrock `caseflow.ProviderFactory`, and run the orchestrator exactly as it does for the `fake` path (same output artifacts, same success/failure exit-code contract), substituting the Bedrock-backed provider for the Fake one.

#### Scenario: --provider bedrock with valid config runs and writes artifacts

- **GIVEN** `AWS_REGION` and `BEDROCK_MODEL_ID` are set, AWS credentials are resolvable, and the CLI is invoked with `--provider bedrock` and a supported `--case`
- **AND** the underlying Bedrock client seam is a fake/mock invoker producing a scripted successful run (never live AWS)
- **WHEN** the CLI runs
- **THEN** it exits 0
- **AND** it writes `<case_id>.jsonl`, `<case_id>.brief.json`, and `<case_id>.brief.md` under `data/synthetic/harness-runs/`, exactly as the `fake` path does

### Requirement: `--provider bedrock` with missing configuration or credentials fails fast, before the orchestrator is constructed

When `--provider bedrock` is selected and `AWS_REGION`, `BEDROCK_MODEL_ID`, or resolvable AWS credentials are missing, the CLI SHALL exit 2 with a clear, usage-style error message identifying what is missing, MUST NOT construct the `caseflow.Orchestrator`, and MUST NOT write any output files. This failure MUST occur before any orchestrator construction, mirroring the #21 no-partial-output guarantee.

#### Scenario: Missing AWS_REGION fails fast with no orchestrator and no partial output

- **GIVEN** `AWS_REGION` is unset, `BEDROCK_MODEL_ID` is set, and the CLI is invoked with `--provider bedrock`
- **WHEN** the CLI runs
- **THEN** it exits with status code 2
- **AND** it prints a clear error identifying the missing `AWS_REGION` configuration
- **AND** the `caseflow.Orchestrator` is never constructed
- **AND** no `.jsonl`, `.brief.json`, or `.brief.md` file is written under `data/synthetic/harness-runs/`

#### Scenario: Missing BEDROCK_MODEL_ID fails fast with no orchestrator and no partial output

- **GIVEN** `BEDROCK_MODEL_ID` is unset, `AWS_REGION` is set, and the CLI is invoked with `--provider bedrock`
- **WHEN** the CLI runs
- **THEN** it exits with status code 2
- **AND** it prints a clear error identifying the missing `BEDROCK_MODEL_ID` configuration
- **AND** the `caseflow.Orchestrator` is never constructed
- **AND** no `.jsonl`, `.brief.json`, or `.brief.md` file is written under `data/synthetic/harness-runs/`

#### Scenario: Missing resolvable AWS credentials fails fast with no orchestrator and no partial output

- **GIVEN** `AWS_REGION` and `BEDROCK_MODEL_ID` are set, no AWS credentials are resolvable, and the CLI is invoked with `--provider bedrock`
- **WHEN** the CLI runs
- **THEN** it exits with status code 2
- **AND** it prints a clear error identifying the credential resolution failure
- **AND** the `caseflow.Orchestrator` is never constructed
- **AND** no `.jsonl`, `.brief.json`, or `.brief.md` file is written under `data/synthetic/harness-runs/`

### Requirement: The `--provider` flag does not change the CLI's workflow-first, non-autonomous execution model

Selecting `--provider bedrock` SHALL only substitute which `harness.ModelProvider` implementation backs each agent's `harness.Runtime`. It MUST NOT change the fixed, deterministic agent invocation order defined by #20, and MUST NOT introduce any model-based routing or autonomous agent-loop behavior.

#### Scenario: Agent invocation order is unaffected by --provider selection

- **GIVEN** the CLI is invoked with `--provider bedrock` against a fake/mock Bedrock invoker
- **WHEN** the CLI runs the orchestrator
- **THEN** the four Domain Agents execute in the same fixed order as the `fake` path
