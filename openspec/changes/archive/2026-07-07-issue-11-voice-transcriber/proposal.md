# Proposal: Issue #11 Voice Pipeline Transcriber + STT Evaluation

## Intent

Close Vigía's audio ingestion gap by adding a provider-neutral speech-to-text seam that turns a collection-call audio file into transcript utterances, then feeds the existing evaluation and evidence path. The first product slice should prove that audio can produce a normal `Evaluation` and `EvidenceRecord` without hard-coding a single STT vendor.

## Problem

Vigía can evaluate structured interactions and transcript utterances, but collection-call audio still requires an ad hoc transcript step outside the system. That weakens traceability: the evidence trail can explain the compliance verdict, but not how the source audio became the transcript that the judge evaluated. Provider choice is also risky if assumed up front, especially for es-MX collection audio with speaker turns and domain-specific phrasing.

## Scope

- Add a `Transcriber` interface as the deep module seam for audio-to-transcript conversion.
- Normalize STT output into the existing `judge.Utterance` transcript shape so `evaluation.EvaluateInteractionInput.Utterances` can remain the handoff into the current evaluation path.
- Provide at least two adapters:
  - AWS-first adapter: Bedrock Data Automation/audio processing where suitable, with Amazon Transcribe as fallback/comparator when it better fits STT requirements.
  - `FakeTranscriber` for deterministic CI/tests and local development.
- Add an ingestion path that accepts a collection-call audio file, obtains transcript utterances, invokes the existing evaluation service, and persists the normal evaluation/evidence records through the existing store.
- Add an STT evaluation harness for synthetic es-MX collection audio that reports comparative accuracy (at minimum WER/CER and adapter metadata) without making paid network calls in normal tests.
- Add configuration for selecting the transcriber mode/provider while keeping fake/default test behavior deterministic.

## Out of scope

- No OpenAI, Whisper, Deepgram, or AI SDK integration in this slice.
- No production-scale async job orchestration unless required by the chosen AWS service contract; the first slice may wrap provider polling behind the adapter.
- No UI workflow for uploading or reviewing audio.
- No manual transcript correction workflow.
- No broad redesign of `evaluation.Service`, the judge seam, detector persistence, or evidence ledger semantics.
- No paid external AWS calls during default unit tests or CI.

## Provider decision

The original issue expected Whisper as the default. That acceptance expectation is replaced by the approved AWS-first decision:

- AWS is the default STT provider family because the user has access to Amazon Bedrock/AWS.
- Bedrock audio/Data Automation is preferred where it supports the required audio transcription workflow.
- Amazon Transcribe is the mature AWS STT fallback/comparator and should be evaluated for es-MX support, speaker labeling/diarization, timestamps, and call-analysis constraints during design.
- `FakeTranscriber` is required for deterministic tests.

## Affected areas

- `internal/transcriber`: provider-neutral interface, normalized transcript/result types, fake adapter, AWS adapters.
- `internal/ingestion`: audio-file-to-evaluation orchestration that calls the transcriber and then `evaluation.Service.EvaluateInteraction`.
- `internal/stteval` and `cmd/stt-eval`: synthetic audio evaluation harness and CLI/reporting surface.
- `data/synthetic/audio`: small checked-in or generated fixtures for es-MX collection-call scenarios, with expected reference transcripts.
- `internal/config/config.go`: transcriber mode/provider, AWS region/service settings, and optional eval-only provider settings.
- Existing `internal/judge/judge.go` and `internal/evaluation/service.go`: used as integration seams; avoid unnecessary changes beyond wiring normalized utterances into existing inputs.

## Acceptance criteria

- Given a supported collection-call audio file and configured transcriber, the system produces transcript utterances and completes the existing evaluation/evidence persistence path.
- STT is pluggable through a `Transcriber` interface with at least an AWS adapter and a fake adapter in this slice.
- The STT eval harness can compare adapters on synthetic es-MX collection audio and report comparative accuracy metrics.
- AWS STT is the default production adapter family; fake remains the default for tests/CI unless explicitly overridden.
- Default tests are deterministic and do not require AWS credentials or network access.
- Provider metadata needed for auditability is captured with the transcript/evaluation path where existing evidence structures allow it, without changing verdict semantics.

## Risks and unknowns

- Bedrock audio transcription availability may be through Bedrock Data Automation rather than ordinary model inference; design must verify the exact API, region availability, supported audio formats, latency, and cost profile.
- Amazon Transcribe es-MX, diarization/speaker-label, and Call Analytics support must be verified before choosing default adapter behavior.
- STT output may not map cleanly to speaker turns; the normalized transcript type should preserve raw/provider metadata so future improvements do not require a schema reset.
- Audio fixture licensing and reproducibility matter; synthetic fixtures should avoid real customer data and be stable enough for regression metrics.
- `internal/config/config.go` is the likely overlap with concurrent issue #10; keep config changes narrow and easy to merge.

## Compatibility with issue #10

This change must stay isolated in the issue #11 worktree and avoid touching the main checkout. The only expected overlap with issue #10 is configuration. To reduce merge risk, the spec/design should keep new transcriber configuration additive, grouped, and tested separately from unrelated provider or judge settings.

## Rollback

Rollback should be straightforward: disable the transcriber ingestion entry point and leave existing direct transcript/evaluation flows unchanged. Because this slice should add a new seam and adapters rather than replace `evaluation.Service`, existing deterministic detector/judge behavior and stored evaluations should continue to work without migration rollback.

## Success criteria

- A developer can run deterministic tests and see the fake transcriber drive audio-like input into the existing evaluation path.
- An operator can configure AWS STT explicitly for non-CI use without enabling OpenAI/AI SDK dependencies.
- The eval harness gives a data-driven comparison baseline for es-MX synthetic audio instead of treating the first provider choice as final.
- Existing evaluation, judge, and evidence tests continue to pass.

## Proposal question round

No live question round was run in this delegated phase; the orchestrator supplied the key provider decision. These product questions are recorded for interactive review before spec finalization:

1. Which audio workflow is the first production slice: single uploaded call recording, batch fixture evaluation, or an internal ingestion command?
2. Is speaker attribution required for the first compliance verdict, or is a single-speaker/unknown-speaker transcript acceptable when diarization is unavailable?
3. What minimum STT metadata must be preserved for auditability: provider, model/service, timestamps, confidence, language, diarization labels, raw transcript, or all of these?
4. Should AWS provider failures block evaluation fail-closed, queue for retry, or fall back to fake/manual transcript only in non-production environments?
5. What accuracy threshold or comparison rule should make one AWS adapter preferred over another for es-MX collection audio?
