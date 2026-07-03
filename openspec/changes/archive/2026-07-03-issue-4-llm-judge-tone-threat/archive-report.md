# Archive Report: Issue #4 LLM-Judge Tone/Threat Detector (MX-REDECO-05)

## Executive summary

Issue #4 — the first production LLM-judge (tone/threat, MX-REDECO-05) — is
fully planned, implemented, verified (PASS), merged to `main` via PR #43
(squash commit `c851a5b`), and issue #4 is closed. This change is archived.

## Change

`issue-4-llm-judge-tone-threat`

## Commits (branch `feat/issue-4-llm-judge-tone-threat`, squash-merged as `c851a5b`)

9 work-unit commits (Work Units 1–8 per `tasks.md`, `size:exception`
pre-approved single-PR delivery) plus 1 judgment-day fix commit:

- Work Unit 1 — dependency, config, migration 00006 + sqlc regen
- Work Unit 2 — `internal/judge` pure core (test-first, no I/O)
- Work Unit 3 — Anthropic judge client (test-first, fake transport only)
- Work Unit 4 — evidence body extension + golden-hash tests (both shapes)
- Work Unit 5 — `evaluation.Service` wiring + fail-closed paths
- Work Unit 6 — interactions query rewrite + httpapi DTO
- Work Unit 7 — console badge/filter
- Work Unit 8 — seed transcripts + docs
- (9th work-unit-adjacent commit covering final integration/doc polish per
  the work-unit-commits convention)
- `2032fb6` — `fix(issue-4): address judgment-day findings`

Squash-merged to `main` as `c851a5b` via PR #43.

## Verify verdict

**PASS** — 0 CRITICAL, 0 WARNING, 1 cosmetic SUGGESTION (design.md's stale
"cmd/api wiring" line, corrected during this archive pass — see below).
`go test ./... -count=1` green with and without DB env (20/20 packages),
RLS suite green, `cmd/seed` integration green, console `tsc --noEmit` clean.
Full detail: `openspec/changes/archive/2026-07-03-issue-4-llm-judge-tone-threat/verify-report.md`.

## Judgment-day outcome

A fresh-context adversarial review after apply found **2 WARNING + 2
SUGGESTION** findings, all fixed in commit `2032fb6`:

1. (WARNING) `JUDGE_MODE` had no enum validation — an unknown value silently
   defaulted to the fake judge instead of failing fast. Fixed with a
   `validJudgeModes` map rejecting unknown values.
2. (WARNING) `judge_model_id`/`rubric_version` provenance was not recorded on
   fail-closed paths (only on success), losing "what judge/version produced
   this HITL flag" for the most safety-critical rows. Fixed by moving the
   assignment above the error-check branch in `service.go` and `anthropic.go`.
3. (SUGGESTION) Multiple configured judges would silently clobber each
   other's header fields (last-judge-wins). Fixed with an explicit
   `ErrMultipleJudgesNotSupported` fail-fast guard.
4. (SUGGESTION) Dead `BuildSystemPrompt` function in `prompt.go`, never
   called by the real request path. Removed; the real-path test
   (`TestAnthropicJudgeCachesStablePrefixNotTranscript`) was strengthened to
   assert the actual cache/prefix invariants instead.

**2 non-blocking deviations were ratified** (confirmed correct, not bugs):

- `overall_outcome="fail"` folds on every judge failure path (timeout,
  transport, malformed, low-confidence), not only malformed as design's
  literal control-flow table implied — stricter/safer, required by the
  spec's malformed-output scenario ("overall_outcome MUST NOT be PASS").
- `detector_result_rows.confidence`/`score` remain display-only/unhashed;
  only `evidence_records.judge_confidence` (TEXT) is the hash-bearing copy —
  matches design's explicit queryable-vs-hash-bearing distinction.

A separate documented deviation — the judge selector is wired into
`cmd/seed/main.go`, not `cmd/api/main.go` (design's task 5.4 literal text),
because `cmd/api`'s HTTP endpoints are read-only with zero
`evaluation.Service` touchpoints in this codebase — was confirmed correct at
both apply and verify. **Fixed during this archive pass**: design.md's
"Config" section previously said `cmd/api` wiring builds the judge; it now
correctly says `cmd/seed` wiring builds the judge, with an explanatory
clause. Purely a documentation correction, no functional change.

## Specs merged

`openspec/changes/issue-4-llm-judge-tone-threat/specs/llm-judge/spec.md` was
a full new spec (no prior `openspec/specs/llm-judge/spec.md` existed), so it
was copied directly (not delta-merged) to:

- `openspec/specs/llm-judge/spec.md` — 9 requirements, ~30 scenarios covering
  the judge port contract, fail-closed HITL semantics, the injection
  boundary, the Anthropic client (temp 0/pinned model/cache_control),
  migration 00006, the evidence body extension (byte-identical + golden
  hash), the interactions query aggregation rewrite, console badges, config
  fail-fast, and seed transcripts.

## Archive contents

- `proposal.md` ✅
- `design.md` ✅ (cosmetic `cmd/api` → `cmd/seed` fix applied)
- `tasks.md` ✅ (52/52 tasks complete, 0 unchecked)
- `specs/llm-judge/spec.md` ✅
- `verify-report.md` ✅ (PASS verdict, full test/scenario mapping)
- `archive-report.md` ✅ (this file)

Archived to: `openspec/changes/archive/2026-07-03-issue-4-llm-judge-tone-threat/`

## Filesystem limitation (risk — needs follow-up)

This archive pass was executed by an agent with **no shell/Bash tool
available** — only Read/Edit/Write/Glob and Engram memory tools. As a
result:

- The archive folder contents above were **written as new files** at the
  archive path (copies of the source content, with the design.md fix
  applied), NOT moved via `git mv`/`mv`.
- The original `openspec/changes/issue-4-llm-judge-tone-threat/` directory
  **could not be deleted** and still exists on disk alongside the new
  archive copy. It must be removed (`git rm -r
  openspec/changes/issue-4-llm-judge-tone-threat/`) by an agent or human
  with shell access before this is committed, to avoid a duplicate change
  folder in the repo.
- No `git add`/`git commit`/`git push` was performed for this archive work.
  A shell-capable agent (or the user) must run:
  ```
  git rm -r openspec/changes/issue-4-llm-judge-tone-threat/
  git add openspec/specs/llm-judge/spec.md \
          openspec/changes/archive/2026-07-03-issue-4-llm-judge-tone-threat/
  git commit -m "docs(issue-4): archive llm-judge change and merge spec"
  git push origin main
  ```

## Follow-ups

- **Issue #5** (golden-set CI gate, eval-runner, Cohen's κ tracking,
  judge-drift gating) is the natural next slice on top of this judge seam.
  **It is blocked by issue #15 and MUST NOT be started** until #15 is
  resolved.
- The console badge/filter scenarios are `[manual-demo]`; a human should run
  `cmd/seed dev-data` and open the console to give final manual confirmation
  (verify accepted TypeScript typecheck evidence as a proxy, per the spec's
  testing-mode note).

## SDD cycle status

Planned (proposal/spec/design/tasks) → Implemented (apply, 9 commits) →
Verified (PASS) → **Archived** (this report), pending the filesystem
cleanup + git commit/push noted above by a shell-capable agent.
