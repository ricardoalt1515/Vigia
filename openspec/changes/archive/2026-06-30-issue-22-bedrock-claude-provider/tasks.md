# Tasks: Issue #22 Bedrock Claude Opt-in Harness Model Provider

Two work units, split into two sequential slices per the design's Review Workload Forecast
(400-line budget risk: High). **Slice 1 (WU1) MUST be implemented, tested green, and committed
before Slice 2 (WU2) begins** — WU2's `--provider bedrock` wiring and config-missing exit-2 test
depend on `bedrock.NewFactory` and `ErrMissingConfig` existing in Slice 1. Strict TDD is active:
the failing test is written and confirmed failing before each production file is created or
extended.

---

## Slice 1 / Work Unit 1 — `internal/harness/bedrock` package (zero touch to `cmd/harness-demo`)

Sequential internally. Self-contained, independently `go test`-verifiable. Does not modify any
file under `cmd/harness-demo/`.

### Dependencies

- [x] `go get github.com/aws/aws-sdk-go-v2` (core module).
- [x] `go get github.com/aws/aws-sdk-go-v2/config` (default credential chain + region resolution).
- [x] `go get github.com/aws/aws-sdk-go-v2/service/bedrockruntime` (+ transitively pulls
  `.../types` and `.../credentials`).
- [x] Run `go mod tidy`; confirm only the expected AWS SDK v2 modules are added to `go.mod`/`go.sum`.

### RED: failing client seam reference test

- [x] **[TEST FIRST]** Create `internal/harness/bedrock/client_test.go` (`package bedrock`).
  Assert that a `*bedrockruntime.Client` satisfies the package's unexported `invoker` interface
  (compile-time assertion, e.g. `var _ invoker = (*bedrockruntime.Client)(nil)`), and that a
  hand-written `fakeInvoker{fn: func(...)}` also satisfies it. Must fail to compile before
  `client.go` exists.

### GREEN: client seam implementation

- [x] Create `internal/harness/bedrock/client.go` (`package bedrock`). Define:
  `type invoker interface { InvokeModel(ctx context.Context, params
  *bedrockruntime.InvokeModelInput, optFns ...func(*bedrockruntime.Options))
  (*bedrockruntime.InvokeModelOutput, error) }` — structural seam matching the real SDK client
  method verbatim, no wrapper type.

### Verify client seam

- [x] Run `go test ./internal/harness/bedrock/...` — client seam test green.

---

### RED: failing error normalization tests

- [x] **[TEST FIRST]** Create `internal/harness/bedrock/errors_test.go` (`package bedrock`).
  Table-driven over each Bedrock SDK exception type. Must fail before `errors.go` exists. Cover:
  - `types.ThrottlingException` → `normalizeError` returns an error satisfying
    `errors.Is(err, ErrThrottled)`.
    Satisfies: `harness-bedrock-provider/spec.md` § "Throttling error normalizes to a clear
    adapter error".
  - `types.AccessDeniedException` → `errors.Is(err, ErrUnauthorized)`.
    Satisfies: § "Authentication failure normalizes to a clear adapter error".
  - `types.ResourceNotFoundException` → `errors.Is(err, ErrModelNotFound)`.
    Satisfies: § "Model-not-found normalizes to a clear adapter error".
  - `types.ValidationException` and `types.ModelErrorException` → `errors.Is(err,
    ErrInvalidRequest)`.
  - `types.InternalServerException`, `types.ServiceUnavailableException`,
    `types.ModelTimeoutException`, and an unrecognized/default error type → `errors.Is(err,
    ErrTransient)`.
    Satisfies: § "Network/timeout failure normalizes to a clear adapter error".
  - Each normalized error's message includes the passed agent name (assert via
    `strings.Contains(err.Error(), agentName)`) and no raw AWS SDK exception type or Bedrock error
    shape is exposed on the returned error's exported surface.
    Satisfies: § "Bedrock errors normalize into a small stable adapter error set" and § "No AWS
    SDK dependency leaks into core Harness packages" (adapter boundary check, package-local).

### GREEN: error normalization implementation

- [x] Create `internal/harness/bedrock/errors.go` (`package bedrock`). Contents per design:
  sentinel errors `ErrMissingConfig`, `ErrUnauthorized`, `ErrThrottled`, `ErrModelNotFound`,
  `ErrInvalidRequest`, `ErrTransient` (all `errors.New("bedrock: ...")`); `func normalizeError(agent
  string, err error) error` using `errors.As` against each `types.*Exception` per the design's
  mapping table, returning `fmt.Errorf("bedrock generate for agent %q: %w", agent, sentinel)`.

### Verify errors

- [x] Run `go test ./internal/harness/bedrock/...` — error normalization tests green.

---

### RED: failing request/response normalization tests

- [x] **[TEST FIRST]** Create `internal/harness/bedrock/normalize_test.go` (`package bedrock`).
  Must fail before `normalize.go` exists. Cover, table-driven:
  - `buildRequestBody(input, maxTokens)`: marshaled JSON includes `anthropic_version`,
    `max_tokens` equal to the input value, a `system` field carrying the adapter-owned envelope
    instruction, and `messages` containing one `role: user` entry whose content text equals
    `input`.
    Satisfies: `harness-bedrock-provider/spec.md` § "ModelRequest.Input is mapped into the
    Bedrock Claude Messages request".
  - `parseResponse(body)` on a canned Claude response whose text is the strict JSON envelope
    `{"plan":"","tool_call":{"name":"","input":{}},"final_output":""}` with a populated
    `tool_call` → returns `harness.ModelOutput.ToolCall` populated with name/input and empty
    `FinalOutput`.
    Satisfies: § "A Bedrock tool-use response maps to ModelOutput.ToolCall".
  - `parseResponse(body)` on an envelope with a populated `final_output` and empty `tool_call` →
    `ModelOutput.FinalOutput` set, `ModelOutput.ToolCall` nil.
    Satisfies: § "A Bedrock final-answer response maps to ModelOutput.FinalOutput".
  - `parseResponse(body)` on non-envelope assistant text (plain prose, not matching the JSON
    envelope shape) → the whole concatenated text becomes `FinalOutput`, `Plan`/`ToolCall` remain
    zero-valued (fallback path).
    Satisfies: design.md § "Adapter-owned system envelope maps Claude text into ModelOutput" —
    fallback decision.
  - `parseResponse(body)` extracts `Usage{InputTokens, OutputTokens}` from the response's
    `usage.input_tokens` / `usage.output_tokens` fields.
    Satisfies: § "Usage metadata is captured via an optional reporter hook" (extraction half).

### GREEN: request/response normalization implementation

- [x] Create `internal/harness/bedrock/normalize.go` (`package bedrock`). Contents per design:
  `func buildRequestBody(input string, maxTokens int) ([]byte, error)` building the Claude
  Messages request (`anthropic_version`, `max_tokens`, adapter-owned `system` envelope
  instruction, single user text message from `input`); `func parseResponse(body []byte)
  (harness.ModelOutput, Usage, error)` decoding `content[].text`, attempting strict envelope
  unmarshal first, falling back to plain `FinalOutput` on failure, and extracting `Usage` from
  `usage.{input,output}_tokens`. Define `type Usage struct { InputTokens, OutputTokens int }` here
  or in `factory.go` per design's placement — keep it colocated with `parseResponse`.

### Verify normalize

- [x] Run `go test ./internal/harness/bedrock/...` — normalization tests green.

---

### RED: failing Provider.Generate tests

- [x] **[TEST FIRST]** Create `internal/harness/bedrock/provider_test.go` (`package bedrock`).
  Uses a `fakeInvoker{fn: func(...)}` implementing `invoker` — construction of
  `bedrockruntime.InvokeModelInput`/`InvokeModelOutput` values is for type construction only,
  never a network/client call (see Apply Notes). Must fail before `provider.go` exists. Cover:
  - `Provider.Generate` against a fake invoker returning a valid envelope response → returns the
    expected `harness.ModelOutput` and a nil error; no live AWS SDK client or network call is
    used.
    Satisfies: `harness-bedrock-provider/spec.md` § "Generate succeeds against a fake invoker".
  - `Provider.Generate` against a fake invoker returning an SDK exception type → returns the
    corresponding normalized `Err*` (via `errors.Is`), not a raw SDK error.
    Satisfies: § "Bedrock errors normalize into a small stable adapter error set" (integration
    through `Generate`).
  - `Provider.Generate` with a configured `UsageReporter` → the reporter is invoked with
    `(agentName, Usage)` matching the fake response's usage fields, on successful completion only.
    Satisfies: § "Usage metadata reaches a configured reporter".
  - `Provider.Generate` with no reporter configured (`reportUsage` nil) → same `ModelOutput`/error
    as with a reporter, no panic.
    Satisfies: § "Adapter functions correctly with no reporter configured".
  - Confirm `harness.ModelProvider.Generate`'s signature and `harness.ModelOutput`'s fields are
    unchanged (compile-time reference to `internal/harness/model.go` types, no new fields used).
    Satisfies: § "The ModelProvider port and ModelOutput struct are unchanged".

### GREEN: Provider.Generate implementation

- [x] Create `internal/harness/bedrock/provider.go` (`package bedrock`). Contents per design:
  `type UsageReporter func(agentName string, usage Usage)`; `type Provider struct { client
  invoker; modelID string; maxTokens int; agentName string; reportUsage UsageReporter }`; `func
  (p *Provider) Generate(ctx context.Context, req harness.ModelRequest) (harness.ModelOutput,
  error)` — calls `buildRequestBody`, invokes `p.client.InvokeModel`, on error returns
  `normalizeError(p.agentName, err)`, on success calls `parseResponse` and, if `p.reportUsage !=
  nil`, calls `p.reportUsage(p.agentName, usage)` before returning the `ModelOutput`.

### Verify provider

- [x] Run `go test ./internal/harness/bedrock/...` — `Provider.Generate` tests green.

---

### RED: failing NewFactory tests

- [x] **[TEST FIRST]** Create `internal/harness/bedrock/factory_test.go` (`package bedrock`). Must
  fail before `factory.go` exists. Cover:
  - Empty/unset `Options.Region` → `NewFactory` returns a nil factory and an error satisfying
    `errors.Is(err, ErrMissingConfig)`, identifying the missing `AWS_REGION` value by name, with no
    SDK call attempted.
    Satisfies: `harness-bedrock-provider/spec.md` § "Missing AWS_REGION fails the constructor with
    a clear error".
  - Empty/unset `Options.ModelID` → same shape, identifying `BEDROCK_MODEL_ID`.
    Satisfies: § "Missing BEDROCK_MODEL_ID fails the constructor with a clear error".
  - Valid `Region`/`ModelID` but no resolvable credentials (e.g. via a scoped/cleared credential
    environment in the test, no live network) → `errors.Is(err, ErrMissingConfig)`, message
    identifies credential resolution failure.
    Satisfies: § "Missing resolvable AWS credentials fails the constructor with a clear error".
  - Valid `Region`, `ModelID`, and resolvable credentials → `NewFactory` returns a non-nil
    `caseflow.ProviderFactory` and nil error; calling the returned factory with an agent name
    returns a non-nil `*Provider` (no live Bedrock call made during construction).
    Satisfies: § "Valid configuration and credentials construct a usable factory".
  - `WithUsageReporter` passed to `NewFactory` results in each `*Provider` produced by the
    returned factory closure carrying that reporter.

### GREEN: NewFactory implementation

- [x] Create `internal/harness/bedrock/factory.go` (`package bedrock`). Contents per design:
  `type Options struct { Region, ModelID string; MaxTokens int }`; `type Option func(*factoryConfig)`;
  `func WithUsageReporter(r UsageReporter) Option`; `func NewFactory(ctx context.Context, opts
  Options, fnOpts ...Option) (caseflow.ProviderFactory, error)` — validates `Region`/`ModelID`
  non-empty first (→ wrapped `ErrMissingConfig`, no SDK call), then calls
  `config.LoadDefaultConfig(ctx, awsconfig.WithRegion(opts.Region))` and eagerly
  `awsCfg.Credentials.Retrieve(ctx)` to fail fast on unresolvable credentials (→ wrapped
  `ErrMissingConfig`), then constructs one shared `*bedrockruntime.Client` and returns a closure
  `func(agentName string) harness.ModelProvider` producing a fresh `&Provider{agentName: name,
  client: client, modelID: opts.ModelID, maxTokens: opts.MaxTokens, reportUsage: cfg.reporter}`
  per call.

### Verify factory

- [x] Run `go test ./internal/harness/bedrock/...` — `NewFactory` tests green.

---

### Note: intentional AWS SDK import in tests (not a bug)

- [x] No code change — documentation/awareness task. `*_test.go` files in this package import
  `github.com/aws/aws-sdk-go-v2/service/bedrockruntime` (and `.../types`) to construct
  `*bedrockruntime.InvokeModelInput`/`InvokeModelOutput` values for `fakeInvoker`. This is
  intentional per design.md's "Structural client seam" decision: tests are AWS-network/credential-
  free, NOT AWS-SDK-import-free. Do not "fix" this by adding a translation-layer wrapper; state
  this explicitly in the Slice 1 PR description so it isn't mistaken for scope creep in review.

## Slice 1 Final Verification

- [x] Run `go test ./internal/harness/bedrock/...` — full package green, including a documented
  assertion (in a test comment or `doc.go`) that a live-Bedrock round trip is not automated and
  must be exercised manually outside `go test ./...`.
  Satisfies: `harness-bedrock-provider/spec.md` § "The accepted testing gap is documented".
- [x] Run `go vet ./internal/harness/bedrock/...` — no errors.
- [x] Run `go list -deps ./internal/harness/... ./internal/harness/labtools/...` (excluding
  `internal/harness/bedrock` itself) and confirm zero matches for
  `github.com/aws/aws-sdk-go-v2`.
  Satisfies: § "Core Harness packages have zero AWS SDK imports".
- [x] Confirm zero touch to any file under `cmd/harness-demo/`.
- [x] **STOP: commit Slice 1 as one work-unit commit before starting Slice 2**, using a
  conventional commit message (no AI attribution trailer). Slice 2 depends on `bedrock.NewFactory`
  and `ErrMissingConfig` existing and green.
- [x] Post-verify follow-up: added `internal/harness/bedrock/orchestrator_integration_test.go`
  (`TestOrchestrator_BedrockAdapterErrorReachesFailureReason`) wiring a real `bedrock.Provider`
  (fake invoker, throttling exception) into a real `caseflow.Orchestrator` run, closing the
  `sdd-verify` WARNING that no test proved the two compose end-to-end for
  `CaseBrief.FailureReason`.

---

## Slice 2 / Work Unit 2 — `cmd/harness-demo` `--provider {fake|bedrock}` wiring

Sequential internally. Requires Slice 1 committed and green. Adds/modifies only
`cmd/harness-demo/main.go`, `cmd/harness-demo/provider.go`, their `_test.go` files, and
`.env.example`.

### RED: failing provider-selection tests

- [x] **[TEST FIRST]** Extend `cmd/harness-demo/provider_test.go` (`package main`). Must fail
  before the selection logic exists. Cover:
  - A provider-selection helper (e.g. `selectProviderFactory(provider string, ...) (caseflow.ProviderFactory,
    error)`) called with `"fake"` (or empty string) returns `demoProviderFactory` unchanged, no
    Bedrock construction attempted.
  - Called with `"bedrock"` and valid `AWS_REGION`/`BEDROCK_MODEL_ID` env values (via a fake
    invoker injection seam for the test — no live AWS) delegates to `bedrock.NewFactory`.
  - Called with `"bedrock"` and missing `AWS_REGION` or `BEDROCK_MODEL_ID` returns an error
    satisfying `errors.Is(err, bedrock.ErrMissingConfig)`.
  - Called with any value other than `"fake"`/`"bedrock"`/empty returns a usage error.
    Satisfies: `harness-demo-cli/spec.md` § "Unknown --provider value fails as a usage error".

### GREEN: provider-selection implementation

- [x] Modify `cmd/harness-demo/provider.go` (`package main`), additive only: add the
  provider-selection helper switching between `demoProviderFactory` and a call to
  `bedrock.NewFactory(ctx, bedrock.Options{Region: ..., ModelID: ...}, ...)`, and optional usage
  accumulator wiring via `bedrock.WithUsageReporter` (e.g. feeding into the existing event sink or
  stderr summary — keep minimal, additive).

### Verify provider selection

- [x] Run `go test ./cmd/harness-demo/...` — provider-selection tests green.

---

### RED: failing CLI `--provider` flag / exit-code tests

- [x] **[TEST FIRST]** Extend `cmd/harness-demo/main_test.go` (`package main`). Must fail before
  `main.go`'s `--provider` flag exists. Cover:
  - `run([]string{"--provider", "bedrock"}, dir)` with `AWS_REGION`/`BEDROCK_MODEL_ID` unset (via
    `t.Setenv` clearing/omitting them) returns exit code `2`, prints a clear stderr message
    identifying the missing configuration, the `caseflow.Orchestrator` is never constructed (no
    side effect observable), and the target dir is empty afterward.
    Satisfies: `harness-demo-cli/spec.md` § "Missing AWS_REGION fails fast with no orchestrator and
    no partial output", § "Missing BEDROCK_MODEL_ID fails fast with no orchestrator and no partial
    output", § "Missing resolvable AWS credentials fails fast with no orchestrator and no partial
    output" (fail-fast-before-construction shape covered generically; the three missing-config
    variants share one assertion path keyed by which env var is unset).
  - `run(nil, dir)` (no `--provider` flag) and `run([]string{"--provider", "fake"}, dir)` produce
    byte-for-byte identical exit code, stderr/stdout, and written artifacts to the pre-#22 (#21)
    default behavior for the same inputs — regression test using the existing default-run fixture.
    Satisfies: § "Omitting --provider preserves #21 default behavior", § "Explicit --provider fake
    preserves #21 default behavior".
  - `run([]string{"--provider", "something-else"}, dir)` returns exit code `2`, prints a usage
    error naming the invalid value, attempts no Bedrock configuration load, constructs no
    orchestrator, and writes no output files.
    Satisfies: § "Unknown --provider value fails as a usage error".
  - `run([]string{"--provider", "bedrock"}, dir)` with valid env vars and a fake/mock Bedrock
    invoker seam injected for the test producing a scripted successful run → exits `0` and writes
    the same three artifacts (`<case_id>.jsonl`, `<case_id>.brief.json`, `<case_id>.brief.md`)
    under the target dir as the `fake` path, with the four Domain Agents executing in the same
    fixed order.
    Satisfies: § "--provider bedrock with valid config runs and writes artifacts", § "Agent
    invocation order is unaffected by --provider selection".

### GREEN: CLI `--provider` flag implementation

- [x] Modify `cmd/harness-demo/main.go` (`package main`), additive only: add a `--provider`
  string flag (default `"fake"`); validate its value is `"fake"` or `"bedrock"`, else print a
  usage error and return exit `2` before any other work. When `"bedrock"`, read `AWS_REGION` and
  `BEDROCK_MODEL_ID` via `os.LookupEnv` (intentionally bypassing `internal/config.Load` — see Apply
  Notes) and call the Slice 2 provider-selection helper / `bedrock.NewFactory` BEFORE
  `caseflow.NewOrchestrator` is constructed; on an error satisfying `errors.Is(err,
  bedrock.ErrMissingConfig)`, print the error to stderr and return exit `2` with no orchestrator
  constructed and no files written. When `"fake"` (or flag omitted), behavior is byte-for-byte
  unchanged from #21.

### Verify CLI flag

- [x] Run `go test ./cmd/harness-demo/...` — all `--provider` flag/exit-code tests green.

---

### `.env.example` guidance

- [x] Modify `.env.example`: fill Bedrock opt-in guidance into the existing `AWS_REGION` /
  `BEDROCK_MODEL_ID` scaffold (short comment noting these are optional, required only for
  `--provider bedrock`, and that Bedrock is opt-in / not used in default tests or demos per
  project conventions).

---

### Import-boundary audit

- [x] Run `go list -deps ./internal/harness/caseflow/...` and confirm zero matches for
  `github.com/aws/aws-sdk-go-v2`. Repeat for Domain Agent packages and the #18/#19/#20 packages
  (`internal/harness`, `internal/harness/labtools`) if not already covered by Slice 1's audit.
  Record the confirming command output in the Slice 2 PR description.
  Satisfies: `harness-bedrock-provider/spec.md` § "Core Harness packages have zero AWS SDK
  imports" (final confirmation after CLI wiring lands).
  Confirmed: `go list -deps ./internal/harness/caseflow/... ./internal/harness/labtools/... ./internal/harness/`
  produces zero matches for `aws-sdk-go-v2`.

## Slice 2 Final Verification

- [x] Run `go test ./...` from repo root — full suite green, including every Slice 1 and
  pre-existing #18/#19/#20/#21 test unmodified.
- [x] Run `go vet ./...` — no errors.
- [x] Confirm no network calls, live AWS credentials, or live Bedrock access occur during
  `go test ./...`.
- [x] Confirm modified/created files are limited to `cmd/harness-demo/{main.go,provider.go}` (both
  additive-only changes) plus their `_test.go` files, and `.env.example`. No file under
  `internal/harness/` is touched in this slice.
- [x] **STOP: commit Slice 2 as one work-unit commit**, using a conventional commit message (no AI
  attribution trailer). PR description must explicitly state: (a) the new production AWS SDK
  dependency landed in Slice 1, (b) tests in `internal/harness/bedrock` import `bedrockruntime` for
  type construction only, never network/client use, (c) `main.go` intentionally reads
  `AWS_REGION`/`BEDROCK_MODEL_ID` via raw `os.LookupEnv` instead of `internal/config.Load`, because
  that loader requires unrelated `DATABASE_URL`/`OBJECT_STORE_*` keys that would regress the
  DB-free demo path.

---

## Apply Notes

- Strict TDD is active throughout both slices: write and confirm the failing test before each
  production file.
- Slice 1 MUST be committed and fully green before any Slice 2 file is created — Slice 2's
  provider selection and config-missing CLI test depend on `bedrock.NewFactory` and
  `bedrock.ErrMissingConfig` from Slice 1.
- `internal/harness/bedrock/*_test.go` files import `bedrockruntime` (and `.../types`) for type
  construction of fake invoker inputs/outputs only — never for a real client or network call. This
  is a disclosed, intentional design tradeoff (design.md § "Structural client seam"), not a defect.
- `cmd/harness-demo/main.go` reads `AWS_REGION`/`BEDROCK_MODEL_ID` via raw `os.LookupEnv`,
  deliberately bypassing `internal/config.Load`/`LoadFromEnv`, because that loader unconditionally
  requires `APP_ENV`, `DATABASE_URL`, and `OBJECT_STORE_*` keys unrelated to this DB-free CLI demo.
  This is a disclosed deviation from the proposal's literal "reuse internal/config" language
  (design.md § "Fail-fast config validation") — reusing only the documented key NAMES, not the
  loader function. State this explicitly in the Slice 2 PR description.
- `caseflow`, Domain Agent code, and the #18 runtime, #19 tools, and #20 orchestrator packages
  MUST NOT import any `github.com/aws/aws-sdk-go-v2/...` package at any point in either slice.
- The AWS SDK production dependency is confined to `internal/harness/bedrock` and its CLI wiring
  point in `cmd/harness-demo`.
- No live-Bedrock round trip is automated in either slice; this gap is documented in Slice 1, not
  silently skipped.
- Use `t.TempDir()` and `t.Setenv()` for all filesystem- and env-touching tests in both slices.
- Bedrock stays opt-in only: never enabled in default tests or demos (only exercised via
  `--provider bedrock` with a fake/mock invoker seam in tests).

---

## Review Workload Forecast

Restating the design's Review Workload Forecast (design.md § "Review Workload Forecast") for
`sdd-apply`: **`400-line budget risk: High`. `Chained PRs recommended: Yes`. `Decision needed
before apply: Yes`.**

Two-slice / two-PR split, Slice 1 before Slice 2:

| Slice / PR | Work Unit | Files | Description |
|------------|-----------|-------|--------------|
| Slice 1 (PR1) | WU1 | `internal/harness/bedrock/{client.go,provider.go,normalize.go,errors.go,factory.go,*_test.go}`; `go.mod`/`go.sum` | Self-contained `internal/harness/bedrock` package implementing `harness.ModelProvider` via an injectable invoker seam, request/response normalization, error taxonomy, usage reporter hook, and fail-fast `NewFactory`. Zero touch to `cmd/harness-demo`. **Key reviewer flag: new production AWS SDK v2 dependency** (`aws-sdk-go-v2`, `.../config`, `.../service/bedrockruntime`). |
| Slice 2 (PR2) | WU2 | `cmd/harness-demo/{main.go,provider.go}` (additive) + their `_test.go` files; `.env.example` | `--provider {fake|bedrock}` flag, `os.LookupEnv`-based config read, `bedrock.NewFactory` wiring before orchestrator construction, exit-2 on `ErrMissingConfig`, and import-boundary audit. |

Slice 1 must land first because Slice 2's factory selection and config-missing CLI test depend on
`bedrock.NewFactory` and its `ErrMissingConfig`. Each slice ends at a fully green `go test ./...`
and is independently reviewable: Slice 1 is a pure new package behind an unchanged port, Slice 2
is the additive CLI wiring that consumes it.
