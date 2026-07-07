# Voice Transcription Ingestion Specification

## Purpose

Define the additive voice-ingestion slice that converts collection-call audio into normalized transcript utterances, feeds the existing evaluation path, and preserves the existing evaluation and evidence semantics. This slice introduces a provider-neutral transcription seam, an AWS-first production adapter family, deterministic fake behavior for tests, and an STT evaluation harness for synthetic es-MX audio.

## Requirements

### Requirement: Transcriber seam returns normalized utterances and transcription metadata

The system MUST expose a provider-neutral `Transcriber` seam that accepts supported audio input and returns a normalized transcription result whose authoritative transcript payload is compatible with `[]judge.Utterance`. The result MUST preserve additive transcription metadata needed for auditability, such as provider identity and provider-specific transcription details, without changing the `judge.Utterance` handoff shape consumed by evaluation.

#### Scenario: Supported audio is normalized for evaluation

- GIVEN a supported collection-call audio input and a configured transcriber
- WHEN transcription succeeds
- THEN the transcriber result MUST include normalized utterances compatible with `[]judge.Utterance`
- AND those utterances MUST be suitable for `evaluation.EvaluateInteractionInput.Utterances`
- AND provider metadata MUST be available as additive transcription data.

#### Scenario: Transcription failure does not fabricate utterances

- GIVEN a supported audio input and a configured transcriber
- WHEN transcription fails
- THEN the transcriber MUST return a failure instead of fabricated utterances
- AND downstream evaluation MUST NOT receive invented transcript content.

### Requirement: AWS adapter family is the default production transcription family

The system MUST support an AWS adapter family as the default production transcription family for this slice. Production configuration MUST allow explicit selection of an AWS transcriber mode/provider, while test and CI defaults MUST remain deterministic and MUST NOT require AWS credentials. Transcriber configuration MUST be additive and narrowly scoped so unrelated configuration behavior remains unchanged.

#### Scenario: Production configuration selects an AWS transcriber

- GIVEN a non-test environment that enables voice transcription
- WHEN the transcriber is constructed from valid production configuration
- THEN the selected production transcriber MUST belong to the AWS adapter family
- AND the system MUST NOT require OpenAI, Deepgram, or AI SDK providers for this slice.

#### Scenario: Default test configuration does not require AWS credentials

- GIVEN default unit-test or CI execution for this repository
- WHEN no explicit live-provider override is supplied
- THEN transcription behavior MUST use deterministic non-paid test behavior
- AND the default path MUST NOT require AWS credentials or paid network access.

#### Scenario: Transcriber configuration remains additive

- GIVEN existing repository configuration behavior outside voice transcription
- WHEN transcriber-specific configuration is added
- THEN the new configuration MUST be limited to transcriber selection and transcriber-specific settings
- AND unrelated judge, evaluation, and other provider settings MUST remain unchanged by default.

### Requirement: FakeTranscriber is deterministic and CI-safe

The system MUST provide a `FakeTranscriber` adapter for tests, CI, and local deterministic workflows. `FakeTranscriber` MUST return stable transcript results for the same fixture/input and MUST require no external network call, paid provider, or cloud credential.

#### Scenario: FakeTranscriber returns repeatable utterances

- GIVEN the same fake transcription fixture or scripted input
- WHEN `FakeTranscriber` runs multiple times
- THEN it MUST return the same normalized utterances each time
- AND it MUST return the same provider metadata each time.

#### Scenario: FakeTranscriber is usable in CI without cloud access

- GIVEN CI or local development without AWS credentials
- WHEN tests exercise audio-transcription-dependent behavior through `FakeTranscriber`
- THEN the tests MUST complete without live cloud access
- AND transcription-dependent behavior MUST remain testable.

### Requirement: Audio ingestion feeds the existing evaluation and evidence path

The system MUST provide an audio ingestion path that accepts a supported collection-call audio input, obtains normalized utterances from the configured transcriber, invokes the existing evaluation service using `evaluation.EvaluateInteractionInput.Utterances`, and persists the resulting `Evaluation` and `EvidenceRecord` through the existing authoritative service/store path. This slice MUST NOT redesign verdict semantics, `evaluation.Service`, or evidence-ledger semantics.

#### Scenario: Audio input produces a normal evaluation and evidence record

- GIVEN a supported collection-call audio input and a successful transcription result
- WHEN the ingestion path is executed
- THEN the system MUST call the existing evaluation path with the normalized utterances
- AND the system MUST persist a normal `Evaluation`
- AND the system MUST persist the corresponding `EvidenceRecord` through the existing evidence path.

#### Scenario: Failed transcription prevents partial evaluation persistence

- GIVEN a supported collection-call audio input
- WHEN transcription fails before evaluation input is formed
- THEN the system MUST NOT persist a new evaluation derived from fabricated utterances
- AND the system MUST NOT append a misleading evidence record for that failed transcription attempt.

### Requirement: STT evaluation harness reports comparative accuracy on synthetic es-MX audio

The system MUST provide an STT evaluation harness for synthetic es-MX collection audio that compares transcriber adapters against reference transcripts and reports comparative accuracy metrics including, at minimum, WER and CER. The harness output MUST also identify the evaluated adapter/provider for each result so comparisons are auditable.

#### Scenario: Harness reports WER and CER for a synthetic collection set

- GIVEN a synthetic es-MX collection-audio fixture set with reference transcripts
- WHEN the STT evaluation harness runs against one or more configured adapters
- THEN the harness MUST compute and report WER for each evaluated adapter
- AND the harness MUST compute and report CER for each evaluated adapter
- AND each reported result MUST identify the adapter/provider that produced it.

#### Scenario: Default automated test execution avoids paid STT calls

- GIVEN the repository's default automated test or CI execution
- WHEN STT evaluation coverage is exercised
- THEN the default coverage MUST rely on synthetic fixtures and deterministic behavior
- AND it MUST NOT require paid external STT calls.

### Requirement: Transcription metadata is auditable without changing verdict semantics

The system MUST preserve additive provider/transcription metadata needed to explain how audio became transcript data, but that metadata MUST NOT change evaluation verdict semantics by itself. For the same normalized utterances, the downstream evaluation outcome MUST remain governed by the existing evaluation and evidence path rather than by provider identity.

#### Scenario: Audit trail identifies the transcription source

- GIVEN an evaluation produced from audio ingestion
- WHEN an operator or developer inspects the stored or emitted transcription-related context available through the existing path
- THEN the transcription source MUST identify the provider or adapter that produced the transcript
- AND the audit context MUST remain additive to the existing evaluation/evidence behavior.

#### Scenario: Provider identity alone does not change the verdict

- GIVEN two transcription results with the same normalized utterances but different provider metadata
- WHEN both are evaluated through the existing evaluation path
- THEN the verdict semantics MUST remain the same
- AND provider metadata MUST NOT become an independent verdict input.
