# Design: Issue #11 Voice Transcriber and STT Evaluation

## Status

Design complete. No code is implemented by this artifact.

## Goals and constraints

This change adds an audio-to-transcript seam that feeds the existing evaluation path without changing judge, detector, evaluation, or evidence-ledger verdict semantics.

Approved constraints:

- AWS is the production provider family for this slice.
- Do not use OpenAI, Whisper, Deepgram, or AI SDK in this slice.
- Prefer Bedrock Data Automation/audio processing if it supports the required workflow; use Amazon Transcribe as the mature AWS fallback/comparator.
- `FakeTranscriber` is required for deterministic tests, CI, and local workflows.
- Default tests and CI must not make paid network calls or require cloud credentials.
- Normalized transcript output must hand off as `[]judge.Utterance` into `evaluation.EvaluateInteractionInput.Utterances`.
- Existing `evaluation.Service`, detector/judge folding, and evidence ledger semantics remain authoritative.

## Current seams observed

- `internal/judge/judge.go` defines `judge.Utterance{Speaker, Text}` and the judge consumes `[]judge.Utterance` through `judge.JudgeInput`.
- `internal/evaluation/service.go` exposes `EvaluateInteraction(ctx, evaluation.EvaluateInteractionInput)` where `Utterances []judge.Utterance` is already the transcript handoff into judge-backed evaluation.
- `internal/postgres/adapters.go` persists normal evaluations and appends an evidence record in the same tenant-scoped transaction inside `EvaluationStore.CreateEvaluation`.
- `internal/postgres/adapters.go` already reads `interaction_transcripts.utterances` as an array of `{speaker,text}` for re-evaluation.
- `internal/config/config.go` currently defaults judge mode to fake and has AWS/Bedrock fields that must not be repurposed broadly for STT.

## Architecture overview

Add three new deep modules and keep existing evaluation as the sink:

```text
collection audio input
  -> internal/ingestion.AudioEvaluationService
       -> internal/transcriber.Transcriber
            -> fake adapter OR AWS adapter family
       -> transcript validation / metadata capture
       -> optional transcript persistence through TranscriptStore
       -> existing evaluation.Service.EvaluateInteraction(... Utterances: []judge.Utterance)
            -> existing EvaluationStore.CreateEvaluation
            -> existing evidence ledger append
```

The new modules are:

1. `internal/transcriber`: provider-neutral STT seam and normalized result types.
2. `internal/ingestion`: orchestration from audio input to existing evaluation.
3. `internal/stteval`: deterministic accuracy harness and metrics for comparing adapters.

The transcriber seam is the deep module interface. AWS job creation, polling, S3 staging, transcript JSON retrieval, diarization normalization, confidence parsing, and provider metadata extraction stay behind adapters. Callers only learn one small interface and receive normalized `judge.Utterance` values.

## Transcriber interface and normalized result types

Proposed package: `internal/transcriber`.

```go
type AudioInput struct {
    SourceURI    string        // local path, file://, s3://, or fixture id depending on adapter
    MediaType    string        // e.g. audio/wav, audio/mpeg
    LanguageCode string        // default es-MX when unset by caller/config
    DurationHint time.Duration // optional, zero when unknown
    Metadata     map[string]string
}

type Result struct {
    Utterances []judge.Utterance
    Segments   []Segment
    Metadata   Metadata
    Raw         json.RawMessage // provider response subset safe for audit/debug; may be nil in fake
}

type Segment struct {
    Speaker    string
    Text       string
    Start      time.Duration
    End        time.Duration
    Confidence *float64
}

type Metadata struct {
    Adapter        string // fake, aws-bedrock-data-automation, aws-transcribe
    Provider       string // fake, aws
    Service        string // bedrock-data-automation, transcribe, fake
    Region         string
    LanguageCode   string
    ModelOrJobType string
    RequestID      string
    JobName        string
    MediaFormat    string
    Diarized       bool
    SchemaVersion  string
}

type Transcriber interface {
    Transcribe(ctx context.Context, in AudioInput) (Result, error)
}
```

Interface rules:

- `Result.Utterances` is the authoritative evaluation payload.
- `Result.Segments` may carry timestamps/confidence and can be richer than `judge.Utterance`; it must not be required by evaluation.
- `Metadata.SchemaVersion` starts at `transcriber-result.v1` so fixture and audit formats can evolve deliberately.
- On provider failure, invalid output, unsupported media, no speech, timeout, credential failure, or empty transcript, return an error. Do not return fabricated utterances.
- The normalizer must reject blank-only utterances and preserve transcript order.
- Speaker labels may be provider labels (`spk_0`, `spk_1`) when real identities are unavailable; never infer debtor/agent identity unless supplied by a verified source.

Expected errors:

- `ErrUnsupportedAudio`
- `ErrNoSpeech`
- `ErrProviderUnavailable`
- `ErrProviderUnauthorized`
- `ErrProviderInvalidOutput`
- `ErrTranscriptionTimeout`

These are for caller decisions and tests; errors must wrap provider details without exposing credentials.

## AWS adapter strategy

### Shared AWS adapter boundary

Keep AWS-specific code below `internal/transcriber/aws...` adapters. The rest of the app depends only on `transcriber.Transcriber`.

The adapter family needs a small constructor/factory, not a broad runtime plugin system:

```go
type AWSConfig struct {
    Region           string
    Service          string // bedrock-data-automation | transcribe
    LanguageCode     string // default es-MX
    InputBucket      string // only if the selected service requires S3 input
    OutputBucket     string // only if async transcript output requires S3
    PollInterval     time.Duration
    Timeout          time.Duration
    KeepProviderRaw  bool
}
```

### Bedrock Data Automation / audio processing adapter boundary

Proposed adapter: `internal/transcriber/awsbda`.

Responsibilities behind the adapter:

- Prepare accepted audio input for Bedrock Data Automation if the API requires S3/object input.
- Start the provider job/invocation.
- Poll or wait within the caller's context/deadline.
- Retrieve provider output.
- Normalize text, speaker turns, timestamps, confidence, and provider identifiers into `transcriber.Result`.
- Return typed errors on unsupported media, unavailable region/service, bad credentials, timeout, or malformed provider output.

Use this as the preferred production default only if apply-time verification proves the service supports this slice's required STT workflow.

### Amazon Transcribe adapter boundary

Proposed adapter: `internal/transcriber/awstranscribe`.

Responsibilities behind the adapter:

- Start an Amazon Transcribe job with configured language and media format.
- Poll `GetTranscriptionJob` until completed/failed/canceled or context deadline.
- Retrieve the transcript JSON from the provider output location.
- Normalize transcript items and speaker labels into ordered `Segment` and `[]judge.Utterance` values.
- Preserve job name, service, region, language, media format, speaker-label flag, and request/job identifiers in metadata.

Amazon Transcribe is the mature AWS fallback/comparator. It is also the safer first implementation target if Bedrock Data Automation cannot be verified quickly for plain STT, diarization, timestamps, supported regions, and es-MX behavior.

### Provider selection

Use a narrow config enum:

- `TRANSCRIBER_MODE=fake|aws-bedrock-data-automation|aws-transcribe`
- `TRANSCRIBER_LANGUAGE_CODE=es-MX` by default
- AWS-specific settings only for live AWS modes:
  - `TRANSCRIBER_AWS_REGION` or fallback to existing `AWS_REGION`
  - `TRANSCRIBER_AWS_INPUT_BUCKET`
  - `TRANSCRIBER_AWS_OUTPUT_BUCKET`
  - `TRANSCRIBER_AWS_POLL_INTERVAL`
  - `TRANSCRIBER_AWS_TIMEOUT`
  - optional `TRANSCRIBER_KEEP_PROVIDER_RAW`

Default behavior:

- Unit tests, CI, and local deterministic workflows use `fake` unless explicitly overridden.
- Production wiring should select an AWS mode when voice transcription is enabled. The preferred AWS mode is `aws-bedrock-data-automation` only after apply-time verification confirms suitability; otherwise use `aws-transcribe` and keep BDA as a comparator behind the same seam.
- Unknown `TRANSCRIBER_MODE` values fail configuration validation instead of falling back to fake.

### Apply-time unknowns to verify with official docs and SDK code

Bedrock Data Automation/audio:

- Whether the available AWS SDK for Go v2 service supports audio transcription for this workflow in the target regions.
- Exact API names, request/response shapes, job lifecycle, and output format.
- Whether es-MX or Spanish locale handling is supported directly.
- Whether speaker diarization, timestamps, confidence, and raw transcript output are available.
- Required S3 input/output permissions, project configuration, quotas, latency, and pricing.
- Whether the service is generally available in the operator's region or still preview/limited.

Amazon Transcribe:

- es-MX language-code support and whether automatic language identification is preferable or harmful for this slice.
- Speaker labels/diarization limits for call audio and any interaction with channel identification.
- Supported media formats and sample rates for the synthetic fixtures.
- Job-name uniqueness constraints and cleanup behavior.
- Output JSON fields for word-level timing/confidence and speaker labels in the AWS SDK for Go v2.
- Cost and region constraints for live eval runs.

Do not guess these details in implementation. The apply phase must verify docs/SDK before selecting the concrete AWS default.

## FakeTranscriber deterministic design

Proposed adapter: `internal/transcriber/fake` or `internal/transcriber` if small.

`FakeTranscriber` should be scriptable and fixture-driven:

```go
type FakeTranscriber struct {
    Scripts map[string]transcriber.Result // key: SourceURI or fixture id
    Default transcriber.Result
    Err     error
}
```

Rules:

- Same input returns the same utterances, segments, and metadata every time.
- No audio decoding, network access, wall-clock reads, randomness, or credentials.
- Metadata uses stable values: `Provider: "fake"`, `Adapter: "fake"`, `Service: "fake"`, `SchemaVersion: "transcriber-result.v1"`.
- Tests can configure explicit failures to prove ingestion fails closed.
- Fixture matching should be deterministic by exact `SourceURI`/fixture id, not by fuzzy filename behavior.

This adapter is the default for tests/CI and the default STT eval adapter in automated coverage.

## Ingestion orchestration boundary

Proposed package: `internal/ingestion`.

```go
type EvaluationRunner interface {
    EvaluateInteraction(ctx context.Context, in evaluation.EvaluateInteractionInput) (core.Evaluation, error)
}

type TranscriptStore interface {
    SaveTranscript(ctx context.Context, in SaveTranscriptInput) error
}

type AudioEvaluationService struct {
    Transcriber transcriber.Transcriber
    Evaluator   EvaluationRunner
    Transcripts TranscriptStore // optional in pure unit tests; required in production wiring
}

type EvaluateAudioInput struct {
    TenantID            string
    InteractionEventID  string
    Interaction         detection.Interaction
    Audio               transcriber.AudioInput
}

type EvaluateAudioResult struct {
    Evaluation    core.Evaluation
    Transcription transcriber.Result
}
```

Flow:

1. Validate tenant id, interaction event id, and required audio fields.
2. Call `Transcriber.Transcribe(ctx, input.Audio)`.
3. If transcription returns an error or zero valid utterances, return an ingestion error and stop.
4. Persist transcript utterances and additive provider metadata through `TranscriptStore` when configured.
5. Call existing `evaluation.Service.EvaluateInteraction` with the original interaction payload and `Utterances: transcription.Utterances`.
6. Return the existing `core.Evaluation` plus transcription metadata to the caller.

`AudioEvaluationService` must not call detector or judge internals directly. It is a coordinator, not a replacement for `evaluation.Service`.

### Transcript persistence

Use the existing `interaction_transcripts` path because re-evaluation already reads it and maps `utterances` back to `[]judge.Utterance`.

Recommended schema additions are additive nullable columns on `interaction_transcripts`:

- `provider text`
- `adapter text`
- `service text`
- `language_code text`
- `provider_job_id text`
- `provider_request_id text`
- `metadata jsonb`

Keep the existing `utterances` JSON shape compatible with `[{"speaker":"...","text":"..."}]`; extra per-utterance fields are acceptable only if existing readers continue to ignore them safely.

Apply-time migration safety check: verify whether multiple transcript rows can already exist for a single `(tenant_id, interaction_event_id)`. If safe, add a uniqueness constraint or implement insert-once behavior so retrying ingestion cannot create ambiguous transcript rows for `GetInteractionTranscriptByInteraction`.

### Evidence behavior

Do not add STT metadata to `ledger.Body` in this slice. The ledger hash format is intentionally stable and already appends evidence only from `EvaluationStore.CreateEvaluation`. Transcription metadata is audit context for how utterances were produced; provider identity must not become an independent verdict input.

Fail-closed rules:

- Transcription failure: do not call evaluation, do not persist a new evaluation, do not append evidence.
- Transcript persistence failure before evaluation: do not call evaluation, because the transcript would not be auditable/replayable through the existing transcript path.
- Evaluation failure after successful transcription: return the evaluation error. The existing evaluation store decides whether anything was persisted; do not fabricate evidence.
- Never fall back from AWS to fake in production after a provider failure. Fake is only selected by explicit test/local configuration.

## STT evaluation harness design

Proposed package: `internal/stteval`; optional CLI: `cmd/stt-eval`.

Core types:

```go
type Fixture struct {
    ID          string
    AudioURI    string
    Language    string
    Reference   []judge.Utterance
    Description string
}

type AdapterResult struct {
    FixtureID string
    Adapter   string
    Provider  string
    Service   string
    WER       float64
    CER       float64
    Metadata  transcriber.Metadata
    Error     string
}
```

Harness flow:

1. Load synthetic es-MX fixture manifests and reference transcripts.
2. Run one or more configured `transcriber.Transcriber` adapters.
3. Flatten normalized utterances to comparable transcript text using a deterministic normalizer.
4. Compute WER and CER for each adapter/fixture pair.
5. Emit JSON and/or table output containing adapter/provider/service metadata, WER, CER, fixture id, and error status.

Metric rules:

- WER uses Levenshtein distance over normalized word tokens.
- CER uses Levenshtein distance over normalized runes.
- The normalizer should lowercase, trim whitespace, collapse repeated whitespace, and remove punctuation consistently. Record a `normalizer_version` in reports.
- Spanish accents should be handled deliberately. Default to preserving accents for CER and using the same normalized text for WER unless fixtures prove accent-insensitive scoring is needed; if accent folding is added, report it in the normalizer version.

Fixtures:

- Synthetic only; no real customer audio.
- Use `data/synthetic/audio/...` for checked-in tiny fixtures only if licensing and repo size are acceptable.
- If audio generation is used instead of checked-in binaries, generation must be deterministic and documented.
- Each fixture has a reference transcript with speaker labels and expected utterance order.

No-paid-default behavior:

- Unit tests use `FakeTranscriber` and small reference transcripts.
- Live AWS adapter evals require an explicit opt-in such as `STT_EVAL_LIVE=1` and valid AWS config.
- Tests that exercise live adapters must skip in `testing.Short()` and when credentials/config are absent.
- `cmd/stt-eval` must default to fake or dry-run fixture validation unless the live flag/provider is explicitly supplied.

## Config and issue #10 merge-risk containment

Keep configuration changes additive and localized in `internal/config/config.go`:

- Add a `TranscriberConfig` nested struct to `Config` instead of scattering top-level fields.
- Add transcriber-specific env names prefixed with `TRANSCRIBER_`.
- Do not change existing `JudgeMode`, `JudgeModelID`, `AWSRegion`, or `BedrockModelID` semantics.
- Reuse `AWS_REGION` only as fallback for `TRANSCRIBER_AWS_REGION`; do not make STT depend on generic Bedrock judge config.
- Validate only transcriber-specific enum/range errors with transcriber-specific missing keys.
- Keep default `fake` for normal tests/CI. Production command wiring can enforce AWS mode when voice ingestion is enabled.

Proposed shape:

```go
type Config struct {
    // existing fields unchanged
    Transcriber TranscriberConfig
}

type TranscriberConfig struct {
    Mode            string
    LanguageCode    string
    AWSRegion       string
    AWSInputBucket  string
    AWSOutputBucket string
    AWSPollInterval time.Duration
    AWSTimeout      time.Duration
    KeepProviderRaw bool
}
```

This should merge cleanly with issue #10 because it is a narrow, prefixed, nested addition and avoids reworking existing provider config.

## Error handling and observability

- All live transcriber calls accept `context.Context` and must honor deadlines/cancellation.
- Provider errors are typed/wrapped and scrubbed of secrets.
- No transcript means no evaluation call.
- No fabricated transcript is ever persisted or evaluated.
- Log provider family, adapter, service, language, duration, and error class; never log credentials or raw customer audio.
- If raw provider output is retained, gate it by config and store only safe JSON subsets needed for audit/debug.
- Provider identity is audit metadata only; it must not affect detector/judge verdict semantics for identical utterances.

## Testing strategy under strict TDD

Write tests before implementation. Default test commands must not require AWS credentials or network access.

Suggested test layers:

1. `internal/transcriber` unit tests
   - `FakeTranscriber` returns stable utterances and metadata.
   - Normalizer rejects empty/blank transcripts.
   - Provider-invalid-output paths return errors, not empty success.

2. AWS adapter unit tests
   - Use fake AWS clients/interfaces and static provider JSON fixtures.
   - Verify polling success, provider failure, timeout/cancel, malformed output, and speaker-label normalization.
   - No real AWS calls in unit tests.

3. `internal/ingestion` unit tests
   - Successful fake transcription calls `EvaluateInteraction` with exact `[]judge.Utterance`.
   - Transcription error does not call evaluator or transcript store.
   - Transcript-store failure does not call evaluator.
   - Evaluation error returns without fabricated success.

4. Postgres integration tests, skipped without DB env
   - Transcript persistence stores utterances and metadata under tenant RLS.
   - Existing evaluation/evidence path still appends exactly one evidence record on successful evaluation.
   - Failed transcription path does not create evaluation/evidence rows.

5. `internal/stteval` unit tests
   - WER/CER known examples.
   - Synthetic es-MX fixture manifest loads deterministically.
   - Default harness uses fake and does not require credentials.

6. Live AWS eval tests
   - Build-tagged or env-gated, skipped in `testing.Short()`.
   - Explicitly require `STT_EVAL_LIVE=1` plus AWS config.
   - Report adapter/provider metadata and cost-risk warning.

## Rollout

1. Land the transcriber seam, fake adapter, normalizer, and deterministic tests.
2. Add ingestion orchestration and transcript persistence with fake-driven tests.
3. Add STT eval harness and synthetic fixtures.
4. Add the first live AWS adapter after verifying the provider docs/SDK details above.
5. Add the comparator AWS adapter if both services are suitable and the review budget allows it; otherwise keep the second adapter boundary designed but unimplemented until a follow-up slice.

## Work-unit and review-budget forecast

Estimated implementation size if completed in one PR: 700-1,100 changed lines, depending on AWS SDK adapter breadth and fixture strategy.

Chained PRs are recommended if the project enforces a 400-line review budget:

1. Transcriber seam + fake adapter + config + unit tests.
2. Ingestion orchestration + transcript persistence + integration tests.
3. STT eval harness + synthetic fixtures + CLI.
4. Live AWS adapter(s), starting with the provider verified as suitable at apply time.

Review workload forecast:

- Chained PRs recommended: Yes.
- 400-line budget risk: High.
- Decision needed before apply: Yes, if the next phase must choose whether to implement both AWS adapters in this slice or only the verified default plus comparator boundary.

## Decisions

- The transcriber seam lives before evaluation and returns normalized `[]judge.Utterance`; evaluation remains unchanged.
- `FakeTranscriber` is the default deterministic adapter for tests/CI/local automation.
- AWS adapter choice is configuration-driven inside the AWS family; Bedrock Data Automation is preferred only after docs/SDK verification confirms suitability.
- Amazon Transcribe is the mature fallback/comparator.
- Transcription failures fail closed before evaluation and evidence persistence.
- STT metadata is additive audit context and must not alter verdict semantics.
