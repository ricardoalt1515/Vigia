# Harness Bedrock Provider Specification

## Purpose

`internal/harness/bedrock` is an opt-in infrastructure package implementing `harness.ModelProvider` for
Claude via Amazon Bedrock. It is the first real runtime model provider for the Agent Harness Lab,
confined behind an injectable client seam so no test or default demo run ever requires live AWS
credentials or network access. It normalizes Bedrock Claude Messages request/response, errors, and
usage metadata into Harness types without widening the `ModelProvider` port, and exposes an env-driven
`caseflow.ProviderFactory` constructor that fails fast on missing configuration.

## ADDED Requirements

### Requirement: Generate implements harness.ModelProvider via an injectable invoker seam

The adapter SHALL implement `harness.ModelProvider.Generate(ctx, ModelRequest) (ModelOutput, error)`
against a minimal injectable invoker interface (e.g. an `InvokeModel(ctx, ...)`-shaped seam) that the
real Bedrock Runtime client satisfies. Tests SHALL inject a fake implementation of this seam and MUST
NOT call live AWS.

#### Scenario: Generate succeeds against a fake invoker

- **GIVEN** the adapter is constructed with a fake invoker that returns a valid Bedrock Claude Messages
  response
- **WHEN** `Generate` is called with a `harness.ModelRequest`
- **THEN** it returns a `harness.ModelOutput` and a nil error
- **AND** no live AWS SDK client or network call is used

#### Scenario: All adapter tests run with zero live AWS

- **GIVEN** a clean local test environment without AWS credentials or network access
- **WHEN** `go test ./internal/harness/bedrock/...` runs
- **THEN** all tests pass using fake/mock invoker implementations only
- **AND** no test requires live AWS credentials, live network access, or a real Bedrock model call

### Requirement: Request normalization maps ModelRequest into a Bedrock Claude Messages request

The adapter SHALL map `harness.ModelRequest.Input` into a valid Bedrock Claude Messages API request body.
AWS SDK request types and Bedrock-specific request shapes MUST NOT cross the adapter's exported boundary;
`caseflow` and Domain Agent code interact only with `harness.ModelRequest`.

#### Scenario: ModelRequest.Input is mapped into the Bedrock Claude Messages request

- **GIVEN** a `harness.ModelRequest` with a non-empty `Input`
- **WHEN** the adapter builds the outbound Bedrock invocation
- **THEN** the request body sent to the injected invoker is a valid Bedrock Claude Messages request
  derived from `Input`
- **AND** the adapter's exported `Generate` signature exposes only `harness` types, never AWS SDK or
  Bedrock request/response types

### Requirement: Response normalization maps a Bedrock Claude response back into ModelOutput

The adapter SHALL map a Bedrock Claude Messages API response into `harness.ModelOutput`, populating
`Plan`, `ToolCall`, and/or `FinalOutput` as appropriate to the response content. AWS SDK response types
MUST NOT be returned from the adapter's exported surface.

#### Scenario: A Bedrock tool-use response maps to ModelOutput.ToolCall

- **GIVEN** a fake invoker returns a Bedrock Claude response representing a tool-use step
- **WHEN** `Generate` processes that response
- **THEN** the returned `ModelOutput.ToolCall` is populated with the tool name and input
- **AND** `ModelOutput.FinalOutput` is empty

#### Scenario: A Bedrock final-answer response maps to ModelOutput.FinalOutput

- **GIVEN** a fake invoker returns a Bedrock Claude response representing a completed final answer
- **WHEN** `Generate` processes that response
- **THEN** the returned `ModelOutput.FinalOutput` contains the finished answer text
- **AND** `ModelOutput.ToolCall` is nil

### Requirement: Bedrock errors normalize into a small stable adapter error set

The adapter SHALL normalize distinct Bedrock failure classes — throttling, authentication/authorization
failure, model-not-found, and network/timeout failures — into a small, stable set of adapter errors.
Each normalized error SHALL be surfaced only as a Go `error` with a clear message, reaching the existing
`runAgent` failure-reason path in `internal/harness/caseflow/orchestrator.go` without any raw AWS SDK
error type crossing the adapter boundary.

#### Scenario: Throttling error normalizes to a clear adapter error

- **GIVEN** a fake invoker returns a Bedrock throttling error
- **WHEN** `Generate` is called
- **THEN** it returns a non-nil error from the adapter's stable error set
- **AND** the error message clearly identifies a throttling failure
- **AND** the error is not, and does not wrap an exported check for, an AWS SDK-specific error type

#### Scenario: Authentication failure normalizes to a clear adapter error

- **GIVEN** a fake invoker returns a Bedrock authentication/authorization failure
- **WHEN** `Generate` is called
- **THEN** it returns a non-nil error from the adapter's stable error set
- **AND** the error message clearly identifies an authentication/authorization failure

#### Scenario: Model-not-found normalizes to a clear adapter error

- **GIVEN** a fake invoker returns a Bedrock model-not-found error for the configured model id
- **WHEN** `Generate` is called
- **THEN** it returns a non-nil error from the adapter's stable error set
- **AND** the error message clearly identifies a model-not-found failure

#### Scenario: Network/timeout failure normalizes to a clear adapter error

- **GIVEN** a fake invoker returns a network or timeout failure
- **WHEN** `Generate` is called
- **THEN** it returns a non-nil error from the adapter's stable error set
- **AND** the error message clearly identifies a network/timeout failure

#### Scenario: Normalized adapter errors reach the orchestrator failure-reason path

- **GIVEN** a `caseflow.Orchestrator` running an agent whose `harness.Runtime.Model` is the Bedrock
  adapter, backed by a fake invoker configured to return a normalized adapter error
- **WHEN** the orchestrator runs that agent
- **THEN** the resulting `CaseBrief.FailureReason` is derived from the normalized adapter error's message
- **AND** no raw AWS SDK error type or Bedrock-specific error shape is observable from `caseflow` or
  Domain Agent code

### Requirement: Usage metadata is captured via an optional reporter hook without widening the ModelProvider port

The adapter SHALL capture provider-agnostic usage metadata (e.g. token counts, as actually returned by
Bedrock) and expose it only through an OPTIONAL reporter hook that the CLI may wire to its event sink.
This reporter mechanism MUST NOT add fields to `harness.ModelOutput` and MUST NOT change the
`harness.ModelProvider` interface signature.

#### Scenario: Usage metadata reaches a configured reporter

- **GIVEN** the adapter is constructed with an optional usage reporter
- **AND** a fake invoker returns a response including usage metadata
- **WHEN** `Generate` completes successfully
- **THEN** the configured reporter is invoked with the normalized usage metadata for that call

#### Scenario: Adapter functions correctly with no reporter configured

- **GIVEN** the adapter is constructed without a usage reporter
- **WHEN** `Generate` completes successfully
- **THEN** it returns the same `ModelOutput` and error as it would with a reporter configured
- **AND** no panic or error occurs due to the absent reporter

#### Scenario: The ModelProvider port and ModelOutput struct are unchanged

- **GIVEN** the #22 Bedrock adapter is added to the codebase
- **WHEN** `internal/harness/model.go` is inspected
- **THEN** `harness.ModelProvider.Generate`'s signature is unchanged from #18/#20
- **AND** `harness.ModelOutput`'s fields are unchanged from #18/#20

### Requirement: An env-driven ProviderFactory constructor fails fast on missing configuration or credentials

The package SHALL expose a constructor that builds a `caseflow.ProviderFactory` from `internal/config`
values (`AWSRegion`, `BedrockModelID`) and AWS credential resolution. When `AWS_REGION`,
`BEDROCK_MODEL_ID`, or resolvable AWS credentials are missing, the constructor SHALL return a clear,
descriptive error and MUST NOT return a usable factory or attempt any live Bedrock call.

#### Scenario: Valid configuration and credentials construct a usable factory

- **GIVEN** `AWS_REGION` and `BEDROCK_MODEL_ID` are set and AWS credentials are resolvable
- **WHEN** the constructor is called
- **THEN** it returns a non-nil `caseflow.ProviderFactory` and a nil error

#### Scenario: Missing AWS_REGION fails the constructor with a clear error

- **GIVEN** `AWS_REGION` is empty or unset
- **WHEN** the constructor is called
- **THEN** it returns a nil factory and a non-nil error
- **AND** the error message clearly identifies the missing `AWS_REGION` value
- **AND** no live AWS call is attempted

#### Scenario: Missing BEDROCK_MODEL_ID fails the constructor with a clear error

- **GIVEN** `BEDROCK_MODEL_ID` is empty or unset
- **WHEN** the constructor is called
- **THEN** it returns a nil factory and a non-nil error
- **AND** the error message clearly identifies the missing `BEDROCK_MODEL_ID` value
- **AND** no live AWS call is attempted

#### Scenario: Missing resolvable AWS credentials fails the constructor with a clear error

- **GIVEN** `AWS_REGION` and `BEDROCK_MODEL_ID` are set but no AWS credentials can be resolved
- **WHEN** the constructor is called
- **THEN** it returns a nil factory and a non-nil error
- **AND** the error message clearly identifies the credential resolution failure

### Requirement: No AWS SDK dependency leaks into core Harness packages

`caseflow`, Domain Agent code, and the #18 runtime, #19 tools, and #20 orchestrator packages MUST NOT
import AWS SDK packages or reference Bedrock-specific types. The AWS SDK dependency is confined to
`internal/harness/bedrock` (and its CLI wiring point in `cmd/harness-demo`).

#### Scenario: Core Harness packages have zero AWS SDK imports

- **GIVEN** the #22 Bedrock adapter is added to the codebase
- **WHEN** the import graphs of `internal/harness`, `internal/harness/caseflow`, and
  `internal/harness/labtools` are inspected
- **THEN** none of them import any `github.com/aws/aws-sdk-go-v2/...` package
- **AND** none of them reference Bedrock-specific request/response/error types

### Requirement: A live Bedrock round trip is not automated, and this gap is documented

Because a genuine end-to-end Bedrock call requires live AWS credentials and network access, the #22
implementation SHALL NOT automate a live-Bedrock round-trip test. This gap SHALL be explicitly documented
rather than silently skipped or omitted.

#### Scenario: The accepted testing gap is documented

- **GIVEN** the #22 Bedrock adapter test suite
- **WHEN** the test suite and its accompanying documentation are reviewed
- **THEN** they explicitly state that a live-Bedrock round trip is not automated
- **AND** they state that live-Bedrock behavior must be exercised manually, outside `go test ./...`
