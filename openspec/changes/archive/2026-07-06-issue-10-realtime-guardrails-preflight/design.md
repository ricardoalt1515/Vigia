# Design: Issue #10 Realtime Guardrails + Campaign Preflight Simulator

## Technical Approach

Add a deep outbound-guardrails module that evaluates typed outbound action proposals before any authority-bearing send. The model/Harness may propose an action, but the runtime remains the policy enforcement point:

1. Decode and validate a typed outbound proposal.
2. Resolve tenant-scoped authority context outside the model.
3. Resolve the active policy bundle; missing or failed resolution fails closed for authority sends.
4. Run deterministic checks first.
5. Invoke the existing `judge.Judge` seam only for semantic tone/threat.
6. Return a tri-state decision (`allowed`, `denied`, `approval_required`) through the existing Harness `PermissionGate` contract.
7. Emit bounded structured events and, for realtime blocked authority actions, append evidence without storing hidden reasoning.

The same outbound decision module is used in two modes:

- **Enforcement mode**: realtime send path. `allowed` is the only outcome that authorizes an external send. `denied` and `approval_required` stop before provider execution.
- **Dry-run mode**: complete-campaign preflight. It expands the campaign artifact into proposed outbound actions and evaluates them through the same policy logic with no provider side effects and no realtime enforcement ledger record.

This keeps the domain policy logic independent from infrastructure, while a Harness adapter reuses the #16 permission-gate/event-log contract.

## Current Code Constraints Read

- `internal/harness.Runtime.evaluateTool` already calls `PermissionGate.Decide` before tool lookup/execution and records `permission_decision` and `tool_result` events.
- `internal/harness.PermissionDecision` already supports `allowed`, `denied`, and `approval_required`, but currently carries only `Kind` and `Reason`.
- `internal/harness/labtools.LabPermissionGate` allows read/draft tools and denies authority tools fail-closed.
- Deterministic detectors in `internal/detection` are pure and already fail closed for missing timezone, unauthorized third-party, unauthorized channel, and missing/non-creditor payment recipient.
- `internal/judge.Judge` is already a separate network-bound seam from the Harness model provider.
- `evaluation.Service` currently resolves missing active bundles to an empty sentinel for shadow-mode compatibility. Authority-bearing outbound evaluation must not reuse that fail-open behavior; it must resolve a bundle explicitly and deny on gaps.
- The evidence ledger currently appends via `postgres.EvaluationStore.CreateEvaluation`, tied to an `interaction_events` row and an `evaluations` row.
- There is no existing campaign runtime or launch UI; preflight should therefore start as a domain/API/CLI seam over a complete campaign artifact, not a launch-management feature.
- `docs/remote-mcp.md` explicitly keeps MCP as an external read-only integration surface and out of internal Harness authority runtime.

## Architecture Decisions

### Decision: Put the enforcement seam at `PermissionGate.Decide`

**Choice**: Realtime outbound sends are authority tools whose Harness permission decision is produced by an outbound guardrail gate. The gate decodes the `send_outbound_utterance` proposal, delegates policy evaluation to the outbound module, and maps the result back to `harness.PermissionDecision`.

**Why**: `Runtime.evaluateTool` is already the last common point before tool execution. Placing enforcement here means a denied or approval-required send never reaches the external provider implementation. It also reuses existing event semantics (`tool_proposed`, `permission_decision`, `tool_result`).

**Rejected**: Putting guardrails inside a send provider adapter. That is too late and too easy for another send path to bypass. The provider adapter may still assert it received an allow token, but it must not be the primary policy seam.

### Decision: Add structured permission metadata without replacing the Harness contract

**Choice**: Extend `harness.PermissionDecision` additively with bounded metadata, for example:

```go
type PermissionDecision struct {
    Kind     PermissionDecisionKind
    Reason   string
    Metadata map[string]any // rule codes, policy bundle, decision id, evidence refs; no hidden reasoning
}
```

`Runtime.evaluateTool` should copy safe metadata into `permission_decision` and denied/approval tool-result events. Existing call sites using named struct fields remain compatible.

**Why**: The current contract can express `allowed`, `denied`, and `approval_required`, but `Reason` is too shallow for rule-aware audit. Metadata keeps the interface small while preserving the existing event-log flow.

**Constraint**: Metadata must be schema-bounded. It must never include hidden chain-of-thought, raw API keys, broad raw PII, or full unredacted transcript/content unless a later evidence policy explicitly allows a safe excerpt.

### Decision: Separate outbound domain logic from Harness adaptation

**Choice**: Introduce an outbound module with no dependency on `internal/harness` for core policy evaluation. Add a thin Harness adapter package for permission-gate integration.

Proposed seams:

```go
// internal/outbound
func (d Decider) Decide(ctx context.Context, in DecisionRequest) (Decision, error)
func (p PreflightService) Run(ctx context.Context, campaign CampaignArtifact) (PreflightBrief, error)

// internal/harness/outboundgate
func NewGate(decider outbound.Decider, fallback harness.PermissionGate) harness.PermissionGate
```

**Why**: Campaign preflight should call the same policy logic without pretending to be a Harness tool call. The Harness adapter is an integration seam, not the policy engine.

### Decision: Draft and send are distinct action kinds

**Choice**: Model output may propose either a draft action or a send action, but only send is authority-bearing.

```go
type ActionKind string

const (
    ActionDraftOutboundUtterance ActionKind = "draft_outbound_utterance"
    ActionSendOutboundUtterance  ActionKind = "send_outbound_utterance"
)

type OutboundActionProposal struct {
    Kind          ActionKind
    ProposalID    string
    CaseID        string
    CampaignID    string // optional for realtime, required when from campaign preflight
    StepID        string // optional except in preflight-expanded actions
    DebtorID      string
    Channel       core.InteractionChannel
    RecipientRef  string
    Text          string
    ProposedAt    time.Time
    PaymentTarget string // normalized designation, not a raw account secret
    DraftOf       string // optional provenance for rewritten drafts
}
```

Tenant identity comes from trusted runtime context or authenticated request context, not model input. The proposal may carry tenant-visible object references, but the context resolver must verify all references under tenant scope.

**Behavior**:

- Draft action: allowed by draft-tool policy; may be linted but cannot send.
- Send action: must pass outbound guardrails in enforcement mode before provider execution.
- Rewrite suggestions: always returned as draft suggestions and never converted to `send` inside the same decision.

### Decision: Use `denied` for fail-closed authority gaps; reserve `approval_required` for explicit policy/HITL states

**Choice**: The decision vocabulary is:

```go
type DecisionOutcome string
const (
    DecisionAllow            DecisionOutcome = "allow"
    DecisionDeny             DecisionOutcome = "deny"
    DecisionApprovalRequired DecisionOutcome = "approval_required"
)
```

Mapping to Harness:

| Outbound outcome | Harness kind | External send allowed? | Typical cause |
|---|---|---:|---|
| `allow` | `PermissionAllowed` | Yes | All required checks pass |
| `deny` | `PermissionDenied` | No | Rule violation, invalid schema, missing context, unknown bundle, unavailable required judge result |
| `approval_required` | `PermissionApprovalRequired` | No | A future policy action requires human approval before a send can be retried |

For this issue's required scenarios, missing context and unavailable/inconclusive judge results are `deny`, not `approval_required`, because the specs require fail-closed blocking when compliance cannot be proven.

### Decision: Resolve policy bundle explicitly for authority mode

**Choice**: Define an authority-specific bundle resolver port:

```go
type ActiveBundleResolver interface {
    ResolveActiveBundle(ctx context.Context, tenantID string) (ResolvedPolicyBundle, error)
}
```

A not-found bundle, stale bundle, resolver error, or empty version returns a deny decision with a fail-closed reason. This intentionally differs from `evaluation.Service.resolveActiveBundle`, which preserves shadow-mode historical behavior by stamping an empty sentinel.

### Decision: Deterministic-first policy flow short-circuits before judge calls

**Choice**: The evaluator runs checks in this order:

1. **Schema/action validation**: known action kind, non-empty proposal ID, channel, recipient, text for send, proposed time, and object references.
2. **Authority context resolution**: tenant, debtor/case/campaign, timezone, recipient relationship, authorized channels, payment target, active policy bundle, and audit context.
3. **Deterministic checks**:
   - channel/recipient ambiguity and authorization;
   - contact hours using debtor-local timezone;
   - third-party contact relationship;
   - payment-routing target;
   - any campaign schedule step constraints available in the artifact.
4. **Semantic tone/threat**: call `judge.Judge` only if deterministic checks pass and the content requires MX-REDECO-05 evaluation.
5. **Decision fold**: any hard-block result denies; explicit HITL policy may produce `approval_required`; all checks passing allows.
6. **Decision recording**: enforcement denied/approval outcomes produce audit/evidence records; dry-run outcomes produce dry-run references only.

The judge must not run if a deterministic hard block is already established. Tests should assert this with a spy judge.

### Decision: Keep Judge and Harness model providers separate

**Choice**: The outbound decider accepts a `judge.Judge` or `evaluation.NamedJudge`-style dependency for semantic tone/threat only. It never calls the Harness `ModelProvider` to judge content.

**Why**: The Harness model proposes actions; the Judge evaluates a bounded rubric. Combining them would make the model both proposer and regulator, violating the proposal and existing ADRs.

### Decision: Realtime blocked sends append enforcement evidence through existing ledger path first

**Choice**: The first implementation should reuse the existing `interaction_events -> evaluations -> evidence_records` chain for realtime blocked sends by creating an attempted outbound interaction snapshot with a status such as `blocked_before_send`, then persisting an evaluation/evidence record for the denial.

Required invariants:

- The external provider is not called before this decision completes.
- The event/evidence record identifies `decision_mode = "enforcement"`.
- The interaction status distinguishes attempted/blocked from actually sent, so dashboards and exports can filter correctly.
- The evidence body contains rule outcomes through detector-result rows and policy bundle version, but no hidden reasoning.
- Content stored for evidence must be bounded. Prefer content digest plus safe excerpt/redaction policy. If existing transcript persistence is reused, store only the proposed utterance as untrusted data and avoid secrets/PII beyond the minimum needed for audit.

**Why**: The existing ledger is append-only, hash-chained, tenant-scoped, and already integrated with detector result rows. A new ledger shape would be larger and riskier for the first slice.

**Caveat**: This is a pragmatic first-slice design. A later issue may introduce a dedicated `authority_decisions` ledger body type if blocked proposals should not live under `interaction_events` long-term.

### Decision: Dry-run preflight does not create realtime enforcement ledger records

**Choice**: Campaign preflight produces dry-run decision/event references, not realtime enforcement evidence records. The brief distinguishes:

```json
{"ref_type":"dry_run_decision","id":"...","mode":"dry_run"}
{"ref_type":"evidence_record","id":"...","mode":"enforcement"}
```

**Why**: Preflight evaluates hypothetical sends. Appending enforcement ledger records for every hypothetical campaign step would pollute the evidence ledger and dashboards. The spec only requires dry-run references and any available evidence refs, with a clear distinction.

## Proposed Interfaces / Contracts

### Outbound decision request and result

```go
type DecisionMode string
const (
    DecisionModeEnforcement DecisionMode = "enforcement"
    DecisionModeDryRun      DecisionMode = "dry_run"
)

type DecisionRequest struct {
    TenantID  string // trusted runtime/auth context
    ActorID   string // user/agent/system actor, trusted runtime/auth context
    Mode      DecisionMode
    Proposal  OutboundActionProposal
    RequestID string
}

type Decision struct {
    ID                  string
    Outcome             DecisionOutcome
    Reason              string
    Violations          []RuleViolation
    FailClosedReasons   []ContextGap
    PolicyBundleID      string
    PolicyBundleVersion string
    CheckedAt           time.Time
    EventRefs           []DecisionRef
    EvidenceRefs        []DecisionRef
    DraftSuggestion     *DraftSuggestion
}

type RuleViolation struct {
    RuleCode    string
    Severity    core.Severity
    Rationale   string // bounded, visible explanation; no hidden reasoning
    Remediation string
    ComponentID string // campaign step/template id when applicable
}

type ContextGap struct {
    Code        string // e.g. missing_debtor_timezone, ambiguous_recipient
    Field       string
    Remediation string
}

type DraftSuggestion struct {
    Text      string
    DraftOnly bool // always true
    BasedOn   string // denied proposal id
}
```

### Authority context resolver

```go
type AuthorityContextResolver interface {
    Resolve(ctx context.Context, tenantID string, p OutboundActionProposal) (AuthorityContext, error)
}

type AuthorityContext struct {
    TenantID                 string
    DebtorID                 string
    DebtorTimezone           string
    ContactPartyRelationship string
    AuthorizedChannels       []string
    PaymentRecipient         string
    Channel                  core.InteractionChannel
    ProposedAt               time.Time
    PolicyBundle             ResolvedPolicyBundle
    AuditContext             AuditContext
}
```

Resolver errors are not thrown through to allow accidentally; the decider converts them to `deny` with a fail-closed reason.

### Policy evaluator

```go
type PolicyEvaluator struct {
    Deterministic []NamedOutboundDetector
    ToneJudge     judge.Judge // optional only when MX-REDECO-05 is configured
    Rubric        judge.Rubric
}
```

`NamedOutboundDetector` can adapt existing `detection.Detector` implementations by mapping `AuthorityContext` + proposal text to `detection.Interaction`. The adapter should keep detectors pure and pass all time/timezone/channel/recipient/payment values explicitly.

### Decision recorder

```go
type DecisionRecorder interface {
    Record(ctx context.Context, req DecisionRequest, decision Decision) (RecordedDecision, error)
}

type RecordedDecision struct {
    EventRefs    []DecisionRef
    EvidenceRefs []DecisionRef
}
```

Implementations:

- `EnforcementRecorder`: persists realtime blocked/approval-required decisions and appends ledger evidence where required.
- `DryRunRecorder`: creates in-memory or lightweight dry-run refs for preflight output; no external provider calls and no enforcement ledger append.
- `NoopRecorder`: test helper only.

If the enforcement recorder fails while handling a would-be allowed decision, the decider should deny because required audit context is unavailable. If it fails after a rule violation is detected, the result remains denied and surfaces `decision_recording_failed`; integration tests must ensure normal blocked decisions do reach the ledger.

### Harness adapter

```go
type OutboundGate struct {
    Decider  outbound.Decider
    Fallback harness.PermissionGate
}

func (g OutboundGate) Decide(ctx context.Context, call harness.ToolCall) harness.PermissionDecision
```

Behavior:

- For `send_outbound_utterance`, decode input into `OutboundActionProposal`, call `Decider.Decide` in enforcement mode, and map outcome to `PermissionDecision`.
- For `draft_outbound_utterance`, either delegate to fallback draft policy or allow as a draft tool with no send authority.
- For all other tools, delegate to the existing gate (`LabPermissionGate` or future production gate).
- Unknown authority-like tools remain denied fail-closed by fallback behavior.

## Data Flow

### Realtime outbound enforcement

```text
Harness model/provider
  -> proposes tool call: send_outbound_utterance(input)
  -> Runtime.validate(model output)
  -> Runtime.evaluateTool adds tool_proposed event
  -> OutboundGate.Decide
      -> decode typed proposal
      -> resolve trusted tenant/actor context
      -> resolve debtor/recipient/channel/payment/policy bundle/audit context
      -> deterministic checks
      -> judge only for MX-REDECO-05 when needed
      -> record denied/approval evidence/events; allowed receives decision metadata
      -> return harness.PermissionDecision
  -> Runtime records permission_decision event with bounded metadata
  -> if denied/approval_required: Runtime returns denied/approval tool result; provider not called
  -> if allowed: Runtime executes send tool
      -> send tool asserts allow decision metadata/token if implemented
      -> external provider seam may be fake/narrow in first slice
```

### Complete-campaign preflight dry run

```text
Campaign artifact/request
  -> PreflightService.Run
      -> validate complete campaign shape
      -> resolve active bundle once per tenant/preflight run
      -> expand templates/scripts/sequences/schedules/recipients into proposed send actions
      -> for each step: Decider.Decide(mode=dry_run)
          -> same deterministic-first policy logic
          -> judge only when needed
          -> DryRunRecorder creates dry-run refs
      -> fold step decisions into campaign pass/fail
      -> return actionable PreflightBrief
```

Preflight never calls a send provider and never uses MCP as an internal runtime.

## Campaign Preflight Model

The first preflight unit is a complete campaign artifact:

```go
type CampaignArtifact struct {
    CampaignID string
    Name       string
    TenantID   string // trusted auth context when persisted/API-backed; not model-trusted
    Audience   []CampaignRecipient
    Steps      []CampaignStep
    Schedule   CampaignSchedule
}

type CampaignRecipient struct {
    RecipientRef string
    DebtorID     string
    Relationship string
    ChannelRefs  []string
    Timezone     string
}

type CampaignStep struct {
    StepID       string
    TemplateID   string
    Channel      core.InteractionChannel
    TextTemplate string
    SendOffset   time.Duration
    PaymentTarget string
}

type CampaignSchedule struct {
    StartsAt time.Time
    Timezone string
}

type PreflightBrief struct {
    CampaignID          string
    Status              string // passed | failed
    PolicyBundleVersion string
    Summary             string
    Findings            []PreflightFinding
    ContextGaps         []ContextGap
    EventRefs           []DecisionRef
    EvidenceRefs        []DecisionRef
}
```

`PreflightService` expands each recipient/step pair into a dry-run `OutboundActionProposal`. For large campaigns, implementation can cap evaluated recipients in a later optimization, but the first spec says complete campaign, so the first implementation should evaluate every supplied artifact element in deterministic tests.

## Evidence and Event Semantics

### Event metadata

Bounded `permission_decision` metadata should include:

- `decision_id`
- `decision_mode`: `enforcement` or `dry_run`
- `action_kind`
- `outcome`
- `policy_bundle_version`
- `violated_rule_codes`
- `fail_closed_codes`
- `event_refs`
- `evidence_refs`

It must not include hidden reasoning. It should avoid raw message text unless a safe excerpt policy is explicitly implemented.

### Realtime evidence

A realtime denied authority decision must append an evidence record. The first slice should reuse existing ledger behavior by persisting:

1. attempted outbound interaction snapshot (`status = blocked_before_send`);
2. evaluation header with `overall_outcome = fail` and resolved policy bundle version;
3. detector/judge result rows for violated rules or fail-closed context gaps;
4. evidence record created by the existing `EvaluationStore` transaction.

### Dry-run references

Dry-run preflight creates references that are useful for the brief and tests but visibly non-enforcement:

- `ref_type = dry_run_decision`
- `mode = dry_run`
- `campaign_id`, `step_id`, `template_id`, and recipient ref where applicable
- no external provider result
- no claim that a real blocked send occurred

## File Changes Forecast

| File / area | Action | Purpose |
|---|---|---|
| `internal/harness/permissions.go` | Modify | Add bounded `Metadata map[string]any` to `PermissionDecision` |
| `internal/harness/runtime.go` | Modify | Copy decision metadata into permission/tool-result events |
| `internal/outbound/*.go` | Create | Typed proposals, decisions, context resolver ports, policy evaluator, decider, remediation model |
| `internal/harness/outboundgate/*.go` | Create | Harness `PermissionGate` adapter for `send_outbound_utterance` |
| `internal/outbound/preflight/*.go` or `internal/outbound/campaign*.go` | Create | Campaign artifact model, dry-run expansion, actionable brief |
| `internal/postgres/*` and `db/queries/*` | Modify/Create | Adapter(s) to resolve authority context and persist realtime blocked evidence via existing interaction/evaluation ledger path |
| `cmd/*` or `internal/httpapi/*` | Possibly modify | Expose the narrowest existing seam for preflight; no broad launch UI |
| Tests under the same packages | Create/modify | Behavior-focused TDD coverage for every required scenario |

The exact preflight entrypoint should be finalized in tasks. Prefer the smallest non-UI seam already present in the codebase (HTTP API if tenant-auth wiring is needed; CLI if a local first slice is enough). Do not add campaign launch UI.

## Testing Strategy

| Layer | What to test | Notes |
|---|---|---|
| Unit | Decode `send_outbound_utterance` into typed proposal; invalid schema denies | No DB/network |
| Unit | Missing timezone, ambiguous recipient/channel, unknown bundle deny fail-closed | Fake resolver |
| Unit | Contact-hours violation denies without judge call | Spy judge must see zero calls |
| Unit | Third-party, payment-routing, channel checks deny deterministically | Existing detector adapters |
| Unit | Threat/tone path calls only `judge.Judge`, never Harness `ModelProvider` | Separate fakes for both ports |
| Unit | Judge unavailable/invalid/inconclusive denies | Mirrors spec fail-closed behavior |
| Unit | Denial result includes remediation and optional draft-only rewrite | Rewrite never changes outcome to allow |
| Harness unit | `OutboundGate` maps allow/deny/approval to existing `PermissionDecisionKind` values | Include metadata propagation |
| Integration | Realtime blocked action creates event metadata and evidence ledger record | Use existing postgres test harness |
| Preflight unit | Non-compliant complete campaign fails with step/template/rule details | No provider side effects |
| Preflight unit | Compliant complete campaign passes | Same decider, dry-run mode |
| Regression | Dry-run refs are not enforcement evidence refs | Explicit `mode` assertion |

Implementation phases must honor Strict TDD. Golden/fake judge cases for tone/threat should be authored before or with code changes.

## First Vertical Implementation Slice

The smallest useful vertical slice should prove the authority boundary end-to-end without live providers:

1. Add typed outbound proposal and decision model.
2. Add `PermissionDecision.Metadata` and runtime event propagation.
3. Add outbound decider with deterministic contact-hours and context-gap fail-closed checks.
4. Add `OutboundGate` for `send_outbound_utterance`.
5. Add a fake/narrow send tool that only executes after `allowed` in tests; no SMS/email/telephony integration.
6. Add tests proving out-of-hours and missing-timezone proposals are denied before tool execution, and compliant proposals reach the fake send tool.

Subsequent slices add remaining deterministic checks, judge integration, evidence persistence, then campaign preflight.

## Rollout / Rollback

- Roll out behind an internal feature/config flag for authority send tools.
- Keep draft tools usable even if authority send guardrails are disabled.
- Realtime send tool registration should require an outbound guardrail gate; no unguarded production send registration.
- Rollback disables/removes the outbound gate and preflight entrypoint. Existing append-only evidence records remain immutable audit history.
- MCP remains external/read-only and is not a runtime bypass.

## Review Workload Forecast

`400-line budget risk: High`. `Chained PRs recommended: Yes`. `Decision needed before apply: Yes`.

Recommended work-unit chain:

1. **Harness metadata + outbound decision core**: small interfaces, metadata propagation, deterministic contact-hours/context fail-closed tests.
2. **Realtime enforcement completeness**: third-party, payment routing, channel/recipient, judge seam, remediation/draft suggestions, and fake send proof.
3. **Evidence persistence**: postgres adapter creating attempted interaction/evaluation/evidence for blocked enforcement decisions.
4. **Campaign preflight**: complete campaign artifact expansion, dry-run refs, actionable brief, compliant/non-compliant campaign tests.
5. **Narrow entrypoint and fixtures**: HTTP/CLI seam and synthetic fixtures; no launch UI.

Each unit should stay reviewable independently. The evidence-persistence unit is the highest-risk slice because it touches ledger semantics and existing dashboards.

## Open Questions

None blocking design. The tasks phase should choose the narrowest preflight entrypoint (HTTP API vs CLI) based on existing implementation cost and review budget.
