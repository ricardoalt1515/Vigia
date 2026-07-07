## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | 700-1,100 |
| 400-line budget risk | High |
| Chained PRs recommended | Yes |
| Suggested split | Slice 1 → Slice 2 → Slice 3 → Slice 4 |
| Delivery strategy | single-pr exception approved |
| Chain strategy | not applicable |

Decision needed before apply: No
Chained PRs recommended: Yes
Chain strategy: not applicable
400-line budget risk: High

## Implementation Tasks

- [x] Slice 1 — `internal/transcriber` seam, `FakeTranscriber`, and additive config wiring in `internal/config/config.go`.
  - Scope: define normalized result types, provider metadata, error contract, and fake/scripted behavior; keep `judge.Utterance` as the evaluation handoff shape.
  - Files to touch: `internal/transcriber/**`, `internal/config/config.go`, and the narrow wiring/discovery points that construct the transcriber.
  - RED: add focused tests for stable fake output, blank/empty rejection, and config validation defaults.
  - GREEN: implement the seam and fake adapter until tests pass.
  - TRIANGULATE: run `go test ./internal/transcriber/... ./internal/config/...` plus the narrow caller tests that instantiate the seam.
  - REFACTOR: reduce duplicated normalization helpers; keep AWS-specific code out of this slice.

- [x] Slice 2 — `internal/ingestion` orchestration and transcript persistence through the existing transcript path.
  - Scope: audio input validation, transcribe-then-evaluate flow, fail-closed behavior, and additive transcript metadata persistence without changing evaluation/evidence semantics.
  - Files to touch: `internal/ingestion/**`, `internal/postgres/**` only where transcript persistence is wired, and any migration/discovery target needed for additive transcript columns.
  - RED: add tests that prove transcription failure stops evaluation, transcript-store failure blocks evaluation, and successful fake transcription calls `evaluation.Service.EvaluateInteraction` with exact `[]judge.Utterance`.
  - GREEN: implement the orchestrator and persistence wiring until tests pass.
  - TRIANGULATE: run `go test ./internal/ingestion/... ./internal/postgres/...` with the relevant DB-gated integration test subset when available.
  - REFACTOR: keep the orchestrator thin; do not touch judge/evaluation semantics beyond wiring.

- [x] Slice 3 — `internal/stteval` harness, synthetic fixtures, and `cmd/stt-eval` reporting surface.
  - Scope: deterministic WER/CER computation, fixture manifest loading, adapter/provider metadata reporting, and fake-default execution.
  - Files to touch: `internal/stteval/**`, `cmd/stt-eval/**`, and `data/synthetic/audio/**` for checked-in or generated fixtures.
  - RED: add tests for known WER/CER examples, manifest loading, and fake-only default behavior without credentials.
  - GREEN: implement the harness and CLI reporting until tests pass.
  - TRIANGULATE: run `go test ./internal/stteval/... ./cmd/stt-eval/...` and confirm no live AWS dependency in default mode.
  - REFACTOR: keep metric normalization explicit and documented; preserve auditable adapter metadata in output.

- [x] Slice 4 — live AWS adapter(s) under `internal/transcriber/aws...`, starting with the provider verified at apply time.
  - Scope: one AWS adapter boundary at a time, with Bedrock Data Automation/audio processing only if docs/SDK verification proves suitability; otherwise start with Amazon Transcribe as the mature fallback/comparator.
  - Files to touch: `internal/transcriber/awsbda/**`, `internal/transcriber/awstranscribe/**`, and any provider-specific fixtures/tests.
  - RED: add adapter tests around polling success, failure, timeout/cancel, malformed output, and speaker-label normalization using fakes/stubs only.
  - GREEN: implement the verified adapter until tests pass; do not add OpenAI/Deepgram/AI SDK dependencies.
  - TRIANGULATE: run provider-specific unit tests plus any env-gated live checks only when explicitly configured.
  - REFACTOR: keep provider selection behind the same seam; do not widen evaluation or evidence semantics.

## Slice Verification Notes

- Strict TDD is active: write the failing tests first for each slice, then implement, then rerun the narrow package tests before widening.
- Keep each slice reviewable on its own; do not mix AWS live adapter work into the seam/fake or ingestion slices.
- Preserve the existing `evaluation.EvaluateInteractionInput.Utterances` handoff and `EvidenceRecord` semantics exactly as-is.
- Treat issue #10 config overlap as a merge-risk constraint: only additive, prefixed transcriber settings in `internal/config/config.go`.
- Do not run paid/cloud-dependent tests by default; all default verification must succeed with fake/deterministic paths.

## Verify Remediation Tasks

- [x] Remediate verify blocker — add concrete AWS SDK-backed Amazon Transcribe client and production constructor behind `internal/transcriber/awstranscribe.Client`, with credential-free default tests.
  - RED: `go test ./internal/transcriber/awstranscribe` failed for missing `NewFromConfig` / `NewFromTranscriberConfig` and `SDKClient`.
  - GREEN: implemented `SDKClient`, `NewFromConfig`, and `NewFromTranscriberConfig`; focused package tests pass without AWS credentials.
  - TRIANGULATE: ran focused package group, full `go test ./...`, `make test`, and forbidden-provider dependency grep.
