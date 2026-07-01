# Proposal: Issue #22 Bedrock Claude Opt-in Harness Model Provider

## Why

#18–#21 make the Agent Harness Lab runnable end-to-end, but ONLY against the deterministic Fake
`ModelProvider`. There is no path to drive the same #20 orchestrator with a real Claude model, so the
lab cannot demonstrate live model behavior, real usage/error surfaces, or provider hardening. #22 adds
the FIRST real runtime `harness.ModelProvider` adapter — Claude via Amazon Bedrock — as an explicit
opt-in, without making cloud access part of any default test or demo. `internal/config` already scaffolds
`AWS_REGION`/`BEDROCK_MODEL_ID` (optional) and `.env.example` already lists them; #22 turns that scaffold
into a working, testable adapter behind a `--provider bedrock` flag.

This proposal covers GitHub issue #22 only.

## Goal

Provide `--provider bedrock` (default `fake`) on `cmd/harness-demo` that constructs a Bedrock-backed
`caseflow.ProviderFactory` from env config, invokes Claude through the Bedrock Runtime, normalizes
request/response/usage/errors into Harness types, and fails clearly and early when config or credentials
are missing. Fake stays the default everywhere; core Harness logic never hardcodes model IDs.

## What Changes

- New infrastructure package implementing `harness.ModelProvider` for Bedrock Claude.
- Additive `--provider {fake|bedrock}` flag on `cmd/harness-demo`; `fake` reproduces today's behavior.
- Bedrock construction path: read `AWS_REGION`/`BEDROCK_MODEL_ID` via `internal/config`, build the AWS
  SDK client, and return a `caseflow.ProviderFactory` — no change to `caseflow` or #18/#19/#20 contracts.
- Request/response/usage/error normalization confined to the adapter boundary.
- Documentation of Bedrock opt-in in `.env.example` (fill the existing empty scaffold with guidance).
- New PRODUCTION go.mod dependency: AWS SDK for Go v2 Bedrock Runtime + config/credentials.

## Scope

### In Scope
- `internal/harness/bedrock` adapter implementing `harness.ModelProvider.Generate`.
- Env-driven Bedrock factory + `--provider bedrock` wiring in `cmd/harness-demo`.
- Normalization of Bedrock Claude Messages request/response into `ModelRequest`/`ModelOutput`.
- Normalized, provider-agnostic error + usage reporting suitable for the Event Log / OTel later.
- Clear, early failure when `AWS_REGION`/`BEDROCK_MODEL_ID`/credentials are missing.
- Fake/mock-based tests via an injectable client seam; zero live AWS.

### Out of Scope (Non-Goals)
- NO migration of the compliance Judge to Bedrock; Judge and Harness model ports stay separate.
- NO dynamic provider fallback or routing.
- NO production AWS deployment; NO Remote MCP server; MCP stays external-only.
- NO changes to #18 runtime interfaces, #19 tools, #20 agent/handoff types, or the orchestrator loop.
- NO new required global config keys (Bedrock vars stay optional unless `--provider bedrock`).

## Capabilities

### New Capabilities
- `harness-bedrock-provider`: the opt-in Bedrock Claude adapter — client seam, request/response/usage/error
  normalization into Harness types, and its env-driven `caseflow.ProviderFactory` constructor.

### Modified Capabilities
- `harness-demo-cli`: additive `--provider {fake|bedrock}` flag selecting the factory; default `fake`
  keeps current behavior and exit codes unchanged.

## Open Questions Resolved

1. **Package/file layout** — New `internal/harness/bedrock` package (flat sibling to `caseflow`/`labtools`,
   consistent with existing layout). A `providers/` subdir is YAGNI for a single adapter; deferred.
2. **Config loading + wiring** — Reuse `internal/config` (already exposes `AWSRegion`/`BedrockModelID`,
   optional). `cmd/harness-demo` loads config only on `--provider bedrock`, then constructs the adapter;
   missing region/model-id/credentials fail fast with exit 2 (usage) before the orchestrator is built. The
   global config contract is unchanged — the two vars remain optional; the adapter constructor enforces
   their presence.
3. **Request/response normalization** — Adapter maps `ModelRequest.Input` into a Bedrock Claude Messages
   request, and maps the Claude response back to `ModelOutput` (`Plan` / optional `ToolCall` / `FinalOutput`).
   AWS SDK types never cross the adapter boundary; `caseflow`/Domain Agents see only `harness` types.
4. **Error + usage normalization** — Bedrock errors (throttling, auth, model-not-found, network) map to a
   small stable adapter error set surfaced only as `error` (its message reaches the existing failure-reason
   path in `runAgent`). Usage metadata is captured in a provider-agnostic struct exposed via an OPTIONAL
   adapter reporter the CLI can wire to its event sink — WITHOUT widening the `ModelProvider` port or
   `ModelOutput` with SDK/usage fields (design owns exact plumbing).
5. **Testing strategy** — Define a minimal invoker interface (e.g. `InvokeModel(ctx, ...)`) that the real
   Bedrock Runtime client satisfies; tests inject a fake implementation. All normalization, config-missing
   failure, and error mapping are unit-tested with fakes. Accepted gap: a real live-Bedrock round trip is
   NOT automated (needs credentials) — exercised manually and documented, not silently skipped.
6. **AWS SDK dependency** — Adds `github.com/aws/aws-sdk-go-v2/...` (Bedrock Runtime + config) as a NEW
   PRODUCTION dependency (unlike #21's test-only dep). Flagged as an explicit reviewable decision; it is
   compiled into the demo binary but only exercised on the opt-in path.

## Impact

| Area | Impact | Description |
|------|--------|-------------|
| `internal/harness/bedrock` | New | Adapter, client seam, normalization, error/usage mapping, factory |
| `cmd/harness-demo` | Modified (additive) | `--provider` flag + Bedrock factory selection; `fake` default |
| `internal/config` | Reused | Existing `AWSRegion`/`BedrockModelID`; adapter enforces at construction |
| `.env.example` | Modified | Fill Bedrock opt-in guidance into the existing empty scaffold |
| `go.mod` / `go.sum` | Modified | New production AWS SDK v2 Bedrock Runtime dependency |
| `internal/harness` (port), `caseflow`, `labtools` | Reused | No interface/contract changes |

## Constraints

- Bedrock is OPT-IN only; never used in default tests or demos. Fake remains the default.
- Judge and Harness model ports stay SEPARATE — this adapter serves the Harness `ModelProvider` port ONLY.
- MCP is out of scope entirely; MCP is never the internal Harness runtime.
- Domain logic stays infra-independent: Domain Agents / `caseflow` MUST NOT import AWS SDK types or see
  provider-specific shapes.
- Smallest viable change that preserves architecture; no port widening, no future-proofing.
- Treat any model output rendered downstream as untrusted DATA; core logic never hardcodes model IDs.
- Strict TDD active: behavior-focused tests (with fakes) before production code.

## Rollback Plan

Additive and isolated. Revert by removing `internal/harness/bedrock`, the `--provider` flag branch in
`cmd/harness-demo` (restoring the direct `demoProviderFactory` wiring), the `.env.example` guidance edit,
and the AWS SDK entries in `go.mod`/`go.sum` (`go mod tidy`). No migrations, no schema, no changes to
#18/#19/#20 contracts to undo. The Fake path is unaffected throughout.

## Dependencies

- #18 runtime (`ModelProvider`, `ModelRequest`, `ModelOutput`, Event contract) — landed.
- #20 `caseflow.ProviderFactory` seam (`func(agentName string) harness.ModelProvider`) — landed.
- #21 `cmd/harness-demo` CLI + `demoProviderFactory` pattern — landed (this stacked branch).
- `internal/config` `AWSRegion`/`BedrockModelID` scaffold — landed.
- External: valid AWS credentials + Bedrock Claude model access (only on the opt-in path, not for tests).

## Decision Boundaries

- #22 adds ONE new `ModelProvider` implementation plus CLI wiring. It does NOT touch caseflow orchestration
  logic, agent definitions, tool contracts, runtime interfaces, or `CaseBrief`/handoff types.
- #22 does NOT widen the `ModelProvider` port or `ModelOutput`; usage/error surfacing stays adapter-owned.
- Judge migration, provider fallback/routing, Remote MCP, and production AWS deployment remain out of scope.

## Review Workload

MEDIUM. One new adapter package (client seam + normalization + error/usage mapping + factory + tests),
one additive CLI flag, an `.env.example` edit, and a go.mod dependency bump. Likely near but under the
400-line budget. `sdd-tasks` MUST forecast; if over budget, recommend two stacked slices: (1) the
`internal/harness/bedrock` adapter + fakes/tests + go.mod dependency; (2) the `--provider bedrock` CLI
wiring + config-missing failure test + `.env.example`. The new PRODUCTION AWS SDK dependency is the key
reviewer decision — call it out explicitly in the PR.

## Success Criteria

- [ ] Fake Model Provider remains the default for tests and the demo CLI.
- [ ] The Bedrock provider runs only when `--provider bedrock` is explicitly requested.
- [ ] Core Harness logic (`caseflow`, Domain Agents, runtime) does not hardcode model IDs.
- [ ] Bedrock config is documented in `.env.example` and loaded via `internal/config`.
- [ ] The adapter normalizes provider errors and usage metadata without leaking AWS SDK/provider-specific
      shapes into Domain Agent or `caseflow` code.
- [ ] Missing `AWS_REGION`/`BEDROCK_MODEL_ID`/credentials fail clearly and early on the opt-in path.
- [ ] Tests use fakes/mocks and require no live AWS credentials; `go test ./...` passes.

## Recommended Next Phase

Proceed to SDD spec for `issue-22-bedrock-claude-provider`, translating `harness-bedrock-provider` into
requirements/scenarios plus the `harness-demo-cli` `--provider` delta. Spec and design may run in parallel;
design should resolve the exact usage/error reporting plumbing and the client-seam interface shape.
