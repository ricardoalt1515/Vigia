# Design: Issue #22 Bedrock Claude Opt-in Harness Model Provider

## Technical Approach

Add a self-contained `internal/harness/bedrock` package that implements the EXISTING
`harness.ModelProvider` interface (`Generate(ctx, ModelRequest) (ModelOutput, error)`) by
invoking Claude through the AWS SDK v2 Bedrock Runtime. All AWS SDK types, JSON body shaping,
usage accounting, and error taxonomy stay behind the package boundary — `caseflow`, the #18
runtime, and Domain Agents keep seeing only `harness` types. `cmd/harness-demo` gains ONE
additive `--provider {fake|bedrock}` flag that selects between the existing `demoProviderFactory`
and a Bedrock-backed `caseflow.ProviderFactory`. `fake` is the default; every existing exit code
and the whole cloud-free test/demo path are byte-for-byte unchanged. This elaborates the HOW of
the proposal's six resolved questions with concrete Go signatures; it does not reopen them.

## Architecture Decisions

### Decision: Structural client seam — real `*bedrockruntime.Client` satisfies it, no wrapper

**Choice**: Define a one-method unexported interface matching the SDK method verbatim:

```go
// internal/harness/bedrock/client.go
type invoker interface {
    InvokeModel(ctx context.Context, params *bedrockruntime.InvokeModelInput,
        optFns ...func(*bedrockruntime.Options)) (*bedrockruntime.InvokeModelOutput, error)
}
```

**Alternatives considered**: (a) a hand-written `type Client struct` wrapper exposing a
harness-shaped method; (b) mocking the concrete `*bedrockruntime.Client`.
**Rationale**: The real `*bedrockruntime.Client` already has this exact signature, so it
satisfies `invoker` structurally with zero adapter code. Tests inject a `fakeInvoker` struct
with a func field — no SDK network calls, no wrapper to maintain. A wrapper would add a layer
that only re-types the same call; mocking the concrete client is impossible (it is a struct).
**Disclosed tradeoff**: because `invoker` is typed verbatim against the SDK method signature,
`*_test.go` files in this package MUST import `github.com/aws/aws-sdk-go-v2/service/bedrockruntime`
to construct `*bedrockruntime.InvokeModelInput`/`InvokeModelOutput` values for the fake — tests
are AWS-SDK-import-free is NOT true, only AWS-network/credential-free is true. Alternative (a),
a harness-shaped wrapper, would let tests use plain Go types with zero SDK import at the cost of
one extra translation layer to maintain. This design accepts that tradeoff; `sdd-tasks` should
state "tests import bedrockruntime for type construction only, never for a client/network call"
in the PR description so this isn't a surprise in review.

### Decision: Adapter-owned system envelope maps Claude text into `ModelOutput`

**Choice**: The Bedrock body carries an adapter-owned system prompt instructing Claude to reply
with a strict JSON envelope `{"plan":"","tool_call":{"name":"","input":{}},"final_output":""}`.
`parseResponse` unmarshals it into `harness.ModelOutput{Plan, ToolCall, FinalOutput}` (real
model.go fields; `ToolCall` → `*harness.ToolCall{Name, Input map[string]any}`). Fallback: if the
assistant text is not that envelope, the whole concatenated text becomes `FinalOutput` and
`Plan`/`ToolCall` stay zero.
**Alternatives considered**: Bedrock native tool-use blocks (heavier, pulls tool schemas across
the boundary); widening `ModelOutput` with raw text (violates "no port widening").
**Rationale**: The system prompt lives inside the adapter-built body — it touches NO caseflow
code and keeps `ModelProvider`/`ModelOutput` unchanged. The fallback guarantees a real model
never crashes the runtime; a non-envelope reply degrades to a plain final answer that
downstream validators/`DecodeHandoff` judge normally. Model text is treated as untrusted DATA.

### Decision: Small stable error set, mapped via `errors.As` on SDK exception types

**Choice**: Exported sentinels in `errors.go`, each returned wrapped with agent + cause context
so the message reaches `runAgent`'s failure-reason path unchanged:

```go
var (
    ErrMissingConfig  = errors.New("bedrock: missing required configuration")
    ErrUnauthorized   = errors.New("bedrock: request not authorized")
    ErrThrottled      = errors.New("bedrock: request throttled")
    ErrModelNotFound  = errors.New("bedrock: model not found or not accessible")
    ErrInvalidRequest = errors.New("bedrock: invalid request")
    ErrTransient      = errors.New("bedrock: transient upstream error")
)
func normalizeError(agent string, err error) error
```

`normalizeError` runs `errors.As` against `types.ThrottlingException` → `ErrThrottled`,
`types.AccessDeniedException` → `ErrUnauthorized`, `types.ResourceNotFoundException` →
`ErrModelNotFound`, `types.ValidationException`/`ModelErrorException` → `ErrInvalidRequest`,
and `InternalServerException`/`ServiceUnavailableException`/`ModelTimeoutException`/default →
`ErrTransient`, returning `fmt.Errorf("bedrock generate for agent %q: %w", agent, sentinel)`.
**Rationale**: All satisfy plain `error`, so they flow through the unchanged
`Generate → RunStep → runAgent` path (confirmed: `runtime.go:64-67` returns the Generate error
from `RunStep`; `orchestrator.go:142` returns it as the agent's failure reason from `runAgent`,
which `Orchestrator.Run` then assigns to `CaseBrief.FailureReason` at `orchestrator.go:253` via
`err.Error()`). Callers that later care can branch with `errors.Is`; today the clear message is
enough.

### Decision: Usage reporter is a constructor functional option, never on the port

**Choice**: `type UsageReporter func(agentName string, usage Usage)` where
`type Usage struct{ InputTokens, OutputTokens int }`, wired via
`func WithUsageReporter(r UsageReporter) Option` on `NewFactory` — NOT on `ModelProvider` or
`ModelOutput`. Each per-agent `Provider` captures its `agentName` and, after a successful parse,
calls `reportUsage(agentName, usage)` if non-nil (from Claude response `usage.input_tokens` /
`output_tokens`).
**Rationale**: Mirrors #21's `WithEventObserver` seam pattern exactly. Keeps usage additive and
adapter-owned; a nil reporter (the default) is a no-op, so the port stays narrow.

### Decision: Fail-fast config validation in the adapter constructor, exit 2 at the CLI

**Choice**: `NewFactory` validates `Region` and `ModelID` non-empty (→ `ErrMissingConfig`),
then `config.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))` and eagerly
`awsCfg.Credentials.Retrieve(ctx)` to fail before any orchestrator is built when credentials are
unresolvable (→ wrapped `ErrMissingConfig`). The CLI reads the two documented env keys
(`AWS_REGION`, `BEDROCK_MODEL_ID` — the same keys `internal/config` defines) via `os.LookupEnv`,
calls `NewFactory` BEFORE `caseflow.NewOrchestrator`, and on error prints to stderr and returns
exit `2`, mirroring `main.go:71-74`'s unsupported-case-id exit-2 usage convention.
**Alternatives considered**: full `config.LoadFromEnv()` in the CLI — rejected: it `required()`s
`DATABASE_URL`/`OBJECT_STORE_*`, which would drag DB/object-store config into a deliberately
DB-free demo and regress the `fake` path. Reading only the two Bedrock keys honors "Bedrock vars
stay optional; the adapter enforces presence" without a new global config requirement.
**Rationale**: Credentials resolve once, synchronously, with a clear message before work starts.
**Disclosed deviation from proposal Q2**: `internal/config.Load`/`LoadFromEnv` (confirmed at
`internal/config/config.go:40-69`) exposes no partial/scoped loader for just `AWSRegion`/
`BedrockModelID` — it always requires `APP_ENV`, `DATABASE_URL`, and the `OBJECT_STORE_*` keys.
Given that, this design does NOT call any `internal/config` function; the CLI reads
`AWS_REGION`/`BEDROCK_MODEL_ID` via raw `os.LookupEnv`, duplicating the two key-name string
literals that `internal/config` also defines, rather than referencing `config.Config` fields.
Proposal Q2 said "reuse `internal/config`" — this design reuses its documented KEY NAMES, not its
loader function, because the loader function is unusable without unrelated required config. This
is a real (if narrow) deviation from the resolved question's literal text, not pure elaboration;
`sdd-tasks`/the PR description must call it out explicitly rather than let it read as silent scope
drift.

## Data Flow

    --provider fake  ─────────────> demoProviderFactory (unchanged, default)
    --provider bedrock
         │  os.LookupEnv(AWS_REGION, BEDROCK_MODEL_ID)
         ▼
    bedrock.NewFactory(ctx, Options{Region,ModelID,MaxTokens}, WithUsageReporter)
         │  validate ─ missing/creds? ─> ErrMissingConfig ─> CLI exit 2
         ▼  ok: caseflow.ProviderFactory
    NewOrchestrator(factory, registry, gate, defs, WithEventObserver) ─> Run
         │  per agent: factory(agentName) -> *bedrock.Provider
         ▼
    Provider.Generate(ctx, ModelRequest{Input})
      buildRequestBody(system envelope + user Input) ─> invoker.InvokeModel
      parseResponse(body) ─> harness.ModelOutput  + Usage ─> reportUsage(agent,usage)
      SDK error ─> normalizeError ─> Err* ─> RunStep ─> runAgent FailureReason

## Interfaces / Contracts

```go
// factory.go
type Options struct { Region, ModelID string; MaxTokens int }
type Option func(*factoryConfig)
func WithUsageReporter(r UsageReporter) Option
func NewFactory(ctx context.Context, opts Options, fnOpts ...Option) (caseflow.ProviderFactory, error)

// provider.go
type Provider struct {
    client      invoker
    modelID     string
    maxTokens   int
    agentName   string
    reportUsage UsageReporter // may be nil
}
func (p *Provider) Generate(ctx context.Context, req harness.ModelRequest) (harness.ModelOutput, error)

// normalize.go
func buildRequestBody(input string, maxTokens int) ([]byte, error) // Claude Messages: anthropic_version, max_tokens, system, messages[{role:user,content:[{type:text,text:input}]}]
func parseResponse(body []byte) (harness.ModelOutput, Usage, error) // decode content[].text -> envelope, usage.{input,output}_tokens
```

`NewFactory`'s returned closure captures one shared `*bedrockruntime.Client` and returns a fresh
`&Provider{agentName: name, ...}` per call — matching `ProviderFactory func(agentName string) harness.ModelProvider`.

## File Changes

| File | Action | Description |
|------|--------|-------------|
| `internal/harness/bedrock/client.go` | Create | `invoker` seam interface (SDK-shaped) |
| `internal/harness/bedrock/provider.go` | Create | `Provider` implementing `harness.ModelProvider.Generate` + usage call |
| `internal/harness/bedrock/normalize.go` | Create | `buildRequestBody` / `parseResponse`, Claude Messages envelope mapping |
| `internal/harness/bedrock/errors.go` | Create | Exported sentinels + `normalizeError` (`errors.As` on SDK types) |
| `internal/harness/bedrock/factory.go` | Create | `Options`, `Option`, `WithUsageReporter`, `NewFactory` fail-fast + `ProviderFactory` closure |
| `internal/harness/bedrock/*_test.go` | Create | Fake-invoker tests (see Testing Strategy) |
| `cmd/harness-demo/main.go` | Modify (additive) | `--provider` flag, value guard, env read, `NewFactory` before orchestrator, exit-2 on `ErrMissingConfig` |
| `cmd/harness-demo/provider.go` | Modify (additive) | select `demoProviderFactory` vs bedrock factory; optional usage accumulator wiring |
| `cmd/harness-demo/*_test.go` | Modify (additive) | `--provider bedrock` config-missing exit-2 test (no live AWS) |
| `.env.example` | Modify | Fill Bedrock opt-in guidance into the existing empty `AWS_REGION`/`BEDROCK_MODEL_ID` scaffold |
| `go.mod` / `go.sum` | Modify | Add production AWS SDK v2 modules (below) |

**AWS SDK v2 modules (production):** `github.com/aws/aws-sdk-go-v2` (core),
`github.com/aws/aws-sdk-go-v2/config` (default chain + region), and
`github.com/aws/aws-sdk-go-v2/service/bedrockruntime` (+ its `/types`). Credentials come
transitively via `.../aws-sdk-go-v2/credentials`. Confirmed by import audit: only
`internal/harness/bedrock` and `cmd/harness-demo` wiring import these; `caseflow`, Domain Agents,
and the #18 runtime import none of them (they depend on `internal/harness` types only).

## Testing Strategy

| Layer | What to Test | Approach |
|-------|-------------|----------|
| Unit | `parseResponse`: envelope → `Plan`/`ToolCall`/`FinalOutput`; non-envelope fallback → `FinalOutput`; usage extracted | Table-driven with canned Claude JSON bodies |
| Unit | `buildRequestBody`: input becomes a single user text block; `max_tokens`/`anthropic_version` present | Marshal + assert JSON shape |
| Unit | `normalizeError`: each SDK exception type maps to the right `Err*` and preserves agent name via `errors.Is` | Table-driven over `types.*Exception` values |
| Unit | `Provider.Generate`: fake `invoker` returns a scripted body → correct `ModelOutput`; fake returns SDK error → normalized `Err*` | `fakeInvoker{fn: func(...)}` injected, zero network |
| Unit | Usage reporter called with `(agentName, Usage)`; nil reporter is a no-op | Recording reporter |
| Unit | `NewFactory`: empty region/model-id → `ErrMissingConfig` (no SDK call) | Assert `errors.Is`, no network |
| E2E (CLI) | `--provider bedrock` with unset env → exit 2, clear stderr; `--provider fake`/default unchanged | In-process `run(args, dir)`, no live AWS |

All tests are network/AWS-free: the `invoker` seam is faked, `NewFactory` config-missing paths
short-circuit before any SDK call, and the CLI test asserts fail-fast without credentials.
Filesystem assertions use `t.TempDir()`. No live-Bedrock round trip is automated (needs
credentials) — that gap is exercised manually and documented, not silently skipped.

## Migration / Rollout

No migration, no schema, no DB. Purely additive and revertable per the proposal's Rollback Plan:
delete `internal/harness/bedrock`, remove the `--provider` branch (restoring the direct
`demoProviderFactory` wiring), revert the `.env.example` guidance, and drop the AWS SDK entries
via `go mod tidy`. The Fake path and all #18/#19/#20/#21 contracts are untouched throughout.

## Review Workload Forecast

`400-line budget risk: High`. `Chained PRs recommended: Yes`. `Decision needed before apply: Yes`.
I AGREE with the proposal's two-slice split; `sdd-tasks` should finalize it:

- **Slice 1** — `internal/harness/bedrock` package (client seam, provider, normalize, errors,
  factory) + fake-invoker unit tests + the production AWS SDK v2 go.mod/go.sum bump. Self-contained,
  independently `go test`-verifiable, zero touch to `cmd/harness-demo`. The new PRODUCTION AWS SDK
  dependency is the key reviewer decision and belongs in this PR's description.
- **Slice 2** — `cmd/harness-demo` `--provider {fake|bedrock}` wiring + optional usage accumulator
  + config-missing exit-2 CLI test + `.env.example` guidance.

Slice 1 must land first because Slice 2's factory selection and config-missing test depend on
`bedrock.NewFactory` and its `ErrMissingConfig`.

## Open Questions

None. AWS SDK v2 `bedrockruntime.Client.InvokeModel` signature is confirmed as the seam target;
credentials fail-fast uses the SDK's own `aws.CredentialsProvider.Retrieve`.
