# Verify Report: Issue #4 LLM-Judge Tone/Threat Detector (MX-REDECO-05)

Branch: `feat/issue-4-llm-judge-tone-threat`. Verdict: **PASS**.

SDD verify gate after apply + judgment-day fix commit `2032fb6`, before archive.

## Test execution evidence

- `go test ./... -count=1` (no DB env): 20/20 packages `ok`, 0 failures.
- `go test ./... -count=1` (DATABASE_URL/APP_DATABASE_URL set, local docker-compose Postgres): 20/20 packages `ok`, 0 failures, 0 skips of note (integration tests ran for real, not short-mode-skipped).
- `make test-rls` equivalent (TestRestrictedAppRoleIsLeastPrivilege incl. `interaction_transcripts` subtest, TestRLSIsolationForCurrentTenantInteractions, TestEvaluationRLSIsolationAcrossTenants, TestEvidenceRLSIsolationAcrossTenants, TestInteractionTranscriptsRLSIsolationAcrossTenants): all PASS.
- `cmd/seed` integration + unit tests (TestSeedDevDataIntegration, TestSeedDevData subtests, TestSeedDispatch, TestIssueTenantAPIKey): all PASS.
- `apps/console`: `npx tsc --noEmit` clean, zero errors (manual-demo scenarios verified via typecheck evidence per spec's testing-mode note).

## Spec scenario → test mapping (all 9 requirements, ~30 scenarios)

1. **Judge Port Is a Narrow, Fallible, Context-Aware Seam** — `TestJudgeInterfaceSignatureAcceptsCtxAndReturnsError`, `TestServiceWiresJudgeAsDistinctTypedStep`, fake judge passed with no special-casing (same test).
2. **Judge Fails Closed to requires_hitl** (7 scenarios) — `TestServiceJudgeTimeoutSetsRequiresHITLNeverSilentPass`, `TestServiceJudgeTransportErrorSetsRequiresHITL`, `TestServiceMalformedJudgeOutputSetsRequiresHITLNeverPass`, `TestServiceLowConfidenceSetsRequiresHITL`, `TestServiceConfidentBlockIsHardBlockAndRequiresHITL`, `TestServiceConfidentPassDoesNotSetRequiresHITL`, `TestServiceHITLThresholdConfigurableWithoutCodeChange` (all in internal/evaluation/service_test.go).
3. **Transcript Is Untrusted Data; Injection Never Flips Verdict** — `TestFakeJudgeEvaluate/injection_attempt_inside_a_threatening_transcript_does_not_flip_the_verdict`, `TestBuildTranscriptBlockDelimitsAsData`, `TestValidateVerdictRejectsSchemaInvalidOutput` (5 subtests), `TestValidateVerdictRejectsSemanticallyInconsistentVerdict`.
4. **Anthropic Judge Temp 0/Pinned Model/Cached Prefix** — `TestAnthropicJudgeBuildsRequestAtTemperatureZeroWithPinnedModel`, `TestAnthropicJudgeCachesStablePrefixNotTranscript` (strengthened in 2032fb6 to assert exactly 2 system blocks, no `<transcript>` leakage into system, rubric text match), `TestAnthropicJudgeRetriesTransientFailureWithinBudget`, `TestAnthropicJudgeGivesUpAfterExhaustingRetryBudget`.
5. **Migration 00006** (4 scenarios) — `TestMigration00006AddsNullableJudgeColumns`, `TestMigration00006PreservesEvaluationsUniqueConstraint` (internal/db/migration_test.go), `TestTranscriptContentRoundTripsSpeakerAndText`, `TestEvaluationServiceJudgeBlockPersistsInOneTransaction`.
6. **Evidence Body Extension Additive/Byte-Identical** (4 scenarios) — `TestHashGoldenValueUnchangedWithJudgeFieldAdded` (pinned hex `4479342d...810384d`, judge-absent), `TestHashGoldenValueJudgePresent` (pinned hex `970ee863...a06e0455`, judge-present), `TestChainVerifiesAcrossJudgeShapeChange`, `TestEvidenceJudgeBodyRoundTripsThroughDBReconstruction` (DB round-trip gate-fix + tamper detection).
7. **Interactions Query Aggregates Across Detector Results** (3 scenarios) — `TestListInteractionsDoesNotFanOutAcrossDetectorResults`, `TestListInteractionsWorstSeverityWinsOnDisagreement`, `TestListInteractionsCarriesThreatHITLFlag`; DTO nil-safety also covered in `internal/httpapi/httpapi_test.go` `TestGetInteractions` (requires_hitl/threat_flagged fields, following Outcome/Reason nil-safe pattern).
8. **Console Surfaces Threat/HITL Badges** ([manual-demo], 2 scenarios) — code inspected (`apps/console/src/app/interactions/InteractionsTable.tsx`: red THREAT badge on `threat_flagged`, amber HITL badge on `requires_hitl`, "Show only flagged" toggle filtering `threat_flagged || requires_hitl`); TypeScript typecheck clean. Per spec's testing-mode note these are validated by a human running the dev environment — typecheck evidence accepted as verify-time proxy, full manual confirmation still pending a human running `cmd/seed dev-data` + opening the console.
9. **Anthropic API Key Fail-Fast; Fake Judge Needs No Key** (3 scenarios) — `TestLoadFailsFastWhenAnthropicJudgeEnabledWithoutKey`, `TestLoadFakeJudgeRequiresNoAPIKey`, `TestLoadAnthropicJudgeSucceedsWithKey`; "never calls live API" scenario satisfied structurally — every judge test in the suite uses `FakeJudge` or a fake `http.RoundTripper`, confirmed by code inspection of anthropic_test.go (injects `option.WithHTTPClient` with a canned transport, never a real network client).
10. **Seed Provides Threatening/Neutral Transcripts** (2 scenarios) — `TestSeedDevDataIntegration` (asserts threatening interaction → overall_outcome="fail" + requires_hitl=true; neutral interaction → requires_hitl=false; idempotent re-run creates no duplicates).

## Judgment-day fixes (commit 2032fb6) — all present and tested

1. `JUDGE_MODE` enum validation (`validJudgeModes` map in config.go) rejects unknown values instead of silently defaulting to fake — tested by `TestLoadRejectsUnknownJudgeMode`, `TestLoadAcceptsKnownJudgeModes`.
2. `judge_model_id`/`rubric_version` now recorded on every judge attempt including fail-closed paths (service.go moved the assignment above the error-check branch) — tested by new assertions appended to `TestServiceJudgeTimeoutSetsRequiresHITLNeverSilentPass`, `TestServiceJudgeTransportErrorSetsRequiresHITL`, `TestServiceMalformedJudgeOutputSetsRequiresHITLNeverPass`, `TestServiceLowConfidenceSetsRequiresHITL`, plus `TestAnthropicJudgeMissingToolBlockIsMalformedOutput`/`SchemaInvalid`/`BelowThresholdConfidence`/`GivesUpAfterExhaustingRetryBudget` (anthropic.go also records provenance on every attempt, not just success).
3. `ErrMultipleJudgesNotSupported` guard (>1 configured judge now fails fast instead of last-judge-wins clobbering header fields) — tested by `TestServiceRejectsMultipleJudges`.
4. Dead `BuildSystemPrompt` removed from prompt.go (never called by `AnthropicJudge.buildRequest`, which sends system+rubric as two separate cached blocks); the real request-path test `TestAnthropicJudgeCachesStablePrefixNotTranscript` strengthened to assert exactly 2 system blocks, no `<transcript>` leakage, and rubric text match — replacing the deleted unit test's coverage with a stronger integration-level assertion on the real code path.

## Design conformance — gate-fix invariants confirmed

- Golden-hash both shapes: `TestHashGoldenValueUnchangedWithJudgeFieldAdded` (absent, pinned unchanged) + `TestHashGoldenValueJudgePresent` (present, pinned) both pass.
- DB round-trip reconstruction of `Body.Judge`: `evidenceRowToRecord` (adapters.go) reconstructs from 3 nullable columns verbatim (no re-format); `TestEvidenceJudgeBodyRoundTripsThroughDBReconstruction` proves it via `ChainVerifier.VerifyChain` + `EvidenceReader.GetEvidencePackage`/`VerifyPackage`, plus tamper detection.
- `judge_confidence` TEXT verbatim: migration column is `text` not `numeric`; `adapters.go` writes `strconv.FormatFloat(*in.JudgeConfidence, 'f', 4, 64)` and reads back the string directly with no re-formatting.
- `package.go` downstream: `PackageRecord`/`BuildPackage`/`VerifyPackage` carry optional `judge` object (`TestBuildPackageIncludesJudgeSubObject`, `TestVerifyPackageOldPackagesWithNoJudgeKeyStillVerify`, `TestVerifyPackageJudgedRecordRoundTrips`, `TestVerifyPackageDetectsTamperedJudgeConfidence`).
- Injection boundary structural: `prompt.go` `BuildTranscriptBlock` XML-escapes all 5 special chars, wraps in `<transcript>` block; `FakeJudge` decides by keyword-scan not by injected text (structural, not prompt-wording-dependent).
- `cache_control` placement: `anthropic.go` uses `anthropic.NewCacheControlEphemeralParam()` (not zero-value struct — apply-phase learned this gotcha) on system+rubric+tool-schema blocks; transcript block carries none.
- Temp 0 + pinned model vs SDK constant: `anthropic.go` sets `Temperature: param.NewOpt(0.0)`, `Model: anthropic.Model(a.modelID)` where default is `DefaultJudgeModelID = "claude-haiku-4-5-20251001"`, verified during apply against `anthropic.ModelClaudeHaiku4_5_20251001` SDK constant (exact match, zero deviation).
- Config fail-fast: `internal/config/config.go` wires `AnthropicAPIKey`/`JudgeMode`/`JudgeModelID`/`JudgeHITLConfidenceThreshold` into the existing `MissingKeysError` path only when `JudgeMode == "anthropic"`; fake mode needs no key.

## Tasks.md truthfulness

52/52 checkboxes marked `[x]`, 0 unchecked. Spot-checked: all referenced files exist (`internal/judge/*`, `db/migrations/00006_llm_judge_tone_threat.sql`, `internal/postgres/evidence_judge_integration_test.go`, `apps/console/src/app/interactions/InteractionsTable.tsx`). Checkboxes accurately reflect implementation state.

## Design-doc discrepancy (fixed at archive)

design.md's "Config" section originally said `cmd/api wiring builds the judge from config`. Actual implementation wires the judge selector (`buildJudge`) into `cmd/seed/main.go`, because `cmd/api` has zero `evaluation.Service` touchpoints in this codebase (its HTTP endpoints are read-only). This was a documented, justified deviation from apply-progress — implementation was correct; design.md text just didn't match. **Corrected during archive**: the archived design.md now reads "`cmd/seed` wiring builds the judge from config" with an explanatory clause about `cmd/api` having no evaluation touchpoint. Purely cosmetic, was never a functional defect.

## Non-blocking ratified deviations (already accepted, re-confirmed at verify)

- `overall_outcome="fail"` folds on EVERY judge failure path (timeout/transport/malformed/low-confidence), not just malformed as design's literal control-flow table suggested — stricter/safer than design pseudocode, required by the malformed-output spec scenario ("overall_outcome MUST NOT be PASS").
- `detector_result_rows.confidence`/`score` remain display-only/unhashed (only `evidence_records.judge_confidence` TEXT column is the hash-bearing copy) — matches design's explicit distinction between "queryable copy" and "hash-bearing copy".

**Where**: All internal/judge, internal/evaluation, internal/ledger, internal/postgres, internal/httpapi, internal/config, internal/db, cmd/seed, apps/console files touched by issue #4; verify ran against local docker-compose Postgres (vigia-postgres-1).

**Learned**: No new findings beyond what apply-progress and the judgment-day review already surfaced. All 4 judgment-day findings are correctly fixed and covered by strengthened/new tests, which themselves pass.

## Verdict: PASS

0 CRITICAL, 0 WARNING (beyond the already-ratified/documented deviations above), 1 SUGGESTION (fix design.md's stale "cmd/api wiring" line to say cmd/seed, for future readers — purely cosmetic, did not block archive; applied during archive, see above).

**Next**: sdd-archive.
