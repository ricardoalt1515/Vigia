// Package evaluation orchestrates running detectors over an interaction and
// persisting the resulting evaluation header + detector result child rows.
// The orchestrator resolves no I/O itself beyond calling the injected Store;
// detectors stay pure (internal/detection).
package evaluation

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/ricardoalt1515/vigia/internal/core"
	"github.com/ricardoalt1515/vigia/internal/detection"
	"github.com/ricardoalt1515/vigia/internal/judge"
)

// ErrNoDetectors is returned by EvaluateInteraction when the Service has no
// configured detectors. An evaluation with zero detector result rows would
// fabricate a "pass" outcome with no supporting evidence, so this case is
// rejected instead of persisted.
var ErrNoDetectors = errors.New("evaluation: no detectors configured")

// ErrMultipleJudgesNotSupported is returned by EvaluateInteraction when more
// than one Judge is configured. The header fields judgeModelID/rubricVersion/
// judgeConfidence below are last-judge-wins across s.Judges, so today more
// than one configured judge would silently clobber provenance for every
// judge but the last one to run. Real multi-judge support (merging or
// per-judge rows) is tracked by issue #7; until then this fails fast instead
// of persisting misleading provenance.
var ErrMultipleJudgesNotSupported = errors.New("evaluation: multiple judges configured is not supported yet (see issue #7); configure at most one judge")

// NamedDetector pairs a Detector with the stable detector_code persisted
// alongside its result row.
type NamedDetector struct {
	Code     string
	Detector detection.Detector

	// RequiresHITL marks a detector whose catalog action mandates human
	// review on a block (e.g. MX-REDECO-07's "HARD BLOCK + HITL" per
	// docs/regulatory-ruleset.md:38). true is OR'd into the evaluation's
	// requires_hitl only when THIS detector blocks; it is never set by any
	// other detector's outcome and is never unset by this detector passing.
	// The zero value (false) reproduces today's behavior for every other
	// detector: a hard block alone does not require human review.
	RequiresHITL bool
}

// DetectorResultInput is one detector's persisted result, already mapped
// from the detection-seam vocabulary (pass/block) to the shared
// core.DetectorOutcome enum (pass/fail).
type DetectorResultInput struct {
	DetectorCode string
	Outcome      core.DetectorOutcome
	Severity     core.Severity
	Rationale    string

	// Confidence and Score are issue #4 additions for the judge's result
	// row: judge rows set Confidence; Score is reserved (nil for now).
	// Detector rows leave both nil.
	Confidence *float64
	Score      *float64
}

// CreateEvaluationInput is everything an EvaluationStore needs to persist an
// evaluations header row and its detector_result_rows children in a single
// tenant-scoped transaction.
type CreateEvaluationInput struct {
	TenantID           string
	InteractionEventID string
	OverallOutcome     string // "pass" | "fail"
	DetectorResults    []DetectorResultInput

	// RequiresHITL, JudgeModelID, RubricVersion, and JudgeConfidence are
	// issue #4 additions: set only when a judge step ran (JudgeModelID !=
	// "" is the sentinel EvaluationStore implementations use to decide
	// whether to populate the evidence body's judge sub-object). Zero
	// values (false, "", "", nil) reproduce today's judge-less behavior
	// exactly, keeping historical evaluations/evidence byte-identical.
	RequiresHITL    bool
	JudgeModelID    string
	RubricVersion   string
	JudgeConfidence *float64

	// PolicyBundleVersion and PolicyBundleID are issue #6 additions: the
	// resolved active bundle's version string + FK id, stamped by
	// EvaluateInteraction via Service.Resolver. Empty string/nil reproduce
	// today's no-active-bundle sentinel exactly (Design Decision 3).
	PolicyBundleVersion      string
	PolicyBundleID           *string
	JudgeInputTokens         int64
	JudgeOutputTokens        int64
	JudgeCacheReadTokens     int64
	JudgeCacheCreationTokens int64
}

// BundleResolver resolves the current active PolicyBundle for a tenant.
// found=false (no active bundle) and a nil Service.Resolver both keep the
// existing empty-string/nil sentinel path unchanged (Design Decision 3/4):
// evaluation must never hard-fail solely because no bundle is configured.
type BundleResolver interface {
	ActiveBundle(ctx context.Context, tenantID string) (version, id string, found bool, err error)
}

// EvaluationStore persists an evaluation. Implementations (internal/postgres)
// MUST write the header and every child row inside one tenantdb.WithTenantTx
// call.
type EvaluationStore interface {
	CreateEvaluation(ctx context.Context, in CreateEvaluationInput) (core.Evaluation, error)
}

// NamedJudge pairs a judge.Judge with the stable detector_code its result
// row carries (e.g. "MX-REDECO-05"). Re-exported alias shape of
// judge.NamedJudge so callers can construct either interchangeably.
type NamedJudge = judge.NamedJudge

// EvaluateInteractionInput carries the tenant/interaction identity alongside
// the pure detection.Interaction payload the detectors need, plus the
// transcript utterances a configured judge step reads (issue #4). Utterances
// is empty/unused when no Judges are configured.
type EvaluateInteractionInput struct {
	TenantID           string
	InteractionEventID string
	Interaction        detection.Interaction
	Utterances         []judge.Utterance
}

// Service runs every configured detector over an interaction, maps outcomes
// to the persisted vocabulary (detector "block" -> core "fail", severity
// high; "pass" -> "pass", severity low), then runs every configured judge as
// a distinct typed step (issue #4) with fail-closed folding, and persists
// the result via Store. The overall evaluation outcome is "fail" if any
// detector blocks OR if any judge step fails closed OR confidently blocks.
type Service struct {
	Detectors []NamedDetector
	Judges    []NamedJudge
	// Rubric is the resolved, versioned judge rubric, loaded once at
	// construction (not per call) and threaded into every judge's
	// JudgeInput. Zero value is safe when no Judges are configured.
	Rubric judge.Rubric
	Store  EvaluationStore

	// Resolver resolves the tenant's active PolicyBundle for stamping
	// (issue #6). Nil is safe: no stamping occurs, reproducing the exact
	// pre-#6 sentinel behavior so existing table-driven tests stay green
	// with no resolver configured (Design Decision 4).
	Resolver BundleResolver

	// Interactions and Bundles are used only by ReEvaluateInteraction
	// (issue #6): Interactions resolves the historical interaction's
	// tenant/payload by id, Bundles validates the caller-supplied
	// policyBundleID belongs to that tenant and resolves its version
	// string. Both are required for ReEvaluateInteraction; EvaluateInteraction
	// never touches them.
	Interactions InteractionLookup
	Bundles      BundleVersionLookup

	// Logger records resolveActiveBundle's error path (a real resolver/DB
	// error, as opposed to the expected "no active bundle configured" case)
	// distinctly, so an operator can tell the two apart in production even
	// though both fail open to the same ""/nil sentinel (Design Decision
	// 3). Nil is safe: slog.Default() is used instead, so existing callers
	// that never set Logger are unaffected.
	Logger *slog.Logger
}

// logger returns s.Logger, falling back to slog.Default() when unset.
func (s Service) logger() *slog.Logger {
	if s.Logger != nil {
		return s.Logger
	}
	return slog.Default()
}

// judgeFailureRationale renders a fail-closed rationale that names the
// specific failure mode, so a HITL reviewer (and this package's tests) can
// tell a timeout apart from a transport error, malformed output, or a
// below-threshold verdict.
func judgeFailureRationale(err error) string {
	switch {
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		return "fail-closed: judge timed out: " + err.Error()
	case errors.Is(err, judge.ErrTransport):
		return "fail-closed: judge transport error: " + err.Error()
	case errors.Is(err, judge.ErrLowConfidence):
		return "fail-closed: judge confidence below threshold: " + err.Error()
	case errors.Is(err, judge.ErrSchemaInvalid), errors.Is(err, judge.ErrMalformedOutput):
		return "fail-closed: judge output malformed or invalid: " + err.Error()
	default:
		return "fail-closed: judge error: " + err.Error()
	}
}

// EvaluateInteraction runs every detector over in.Interaction, then runs
// every configured judge over in.Utterances as a separate loop (never
// implemented via detection.Detector), and persists the resulting
// evaluation header + detector/judge result rows via s.Store.
//
// Fail-closed folding (spec: "Judge Fails Closed to requires_hitl on Every
// Uncertain Path"): ANY judge error — timeout, transport, malformed/schema-
// invalid output, or confidence below threshold — sets requires_hitl = true
// AND folds overall_outcome to "fail", so a judge failure is never a silent
// (or even quiet) pass. A confident BLOCK verdict is a HARD BLOCK that ALSO
// sets requires_hitl = true (MX-REDECO-05 mandates human review even on a
// clear block). A confident PASS verdict does not block and does not set
// requires_hitl from the judge step alone.
func (s Service) EvaluateInteraction(ctx context.Context, in EvaluateInteractionInput) (core.Evaluation, error) {
	if len(s.Detectors) == 0 {
		return core.Evaluation{}, ErrNoDetectors
	}
	if len(s.Judges) > 1 {
		return core.Evaluation{}, ErrMultipleJudgesNotSupported
	}

	run := s.runDetectorsAndJudges(ctx, in.Interaction, in.Utterances)

	policyBundleVersion, policyBundleID := s.resolveActiveBundle(ctx, in.TenantID)

	return s.Store.CreateEvaluation(ctx, CreateEvaluationInput{
		TenantID:                 in.TenantID,
		InteractionEventID:       in.InteractionEventID,
		OverallOutcome:           run.overallOutcome,
		DetectorResults:          run.results,
		RequiresHITL:             run.requiresHITL,
		JudgeModelID:             run.judgeModelID,
		RubricVersion:            run.rubricVersion,
		JudgeConfidence:          run.judgeConfidence,
		PolicyBundleVersion:      policyBundleVersion,
		PolicyBundleID:           policyBundleID,
		JudgeInputTokens:         run.judgeInputTokens,
		JudgeOutputTokens:        run.judgeOutputTokens,
		JudgeCacheReadTokens:     run.judgeCacheReadTokens,
		JudgeCacheCreationTokens: run.judgeCacheCreationTokens,
	})
}

// detectorJudgeRun is the shared, side-effect-free result of running every
// configured detector then every configured judge over one interaction. It
// is reused by both EvaluateInteraction (which persists it) and
// ReEvaluateInteraction (which does not), so the two entry points can never
// drift on outcome-folding logic.
type detectorJudgeRun struct {
	overallOutcome           string
	results                  []DetectorResultInput
	requiresHITL             bool
	judgeModelID             string
	rubricVersion            string
	judgeConfidence          *float64
	judgeInputTokens         int64
	judgeOutputTokens        int64
	judgeCacheReadTokens     int64
	judgeCacheCreationTokens int64
}

// runDetectorsAndJudges runs every s.Detectors entry over interaction, then
// every s.Judges entry over utterances as a distinct typed step, applying
// the same fail-closed folding EvaluateInteraction has always used. It has
// no side effects (no store call, no bundle resolution) so callers control
// persistence independently.
func (s Service) runDetectorsAndJudges(ctx context.Context, interaction detection.Interaction, utterances []judge.Utterance) detectorJudgeRun {
	overallOutcome := "pass"
	results := make([]DetectorResultInput, 0, len(s.Detectors)+len(s.Judges))
	requiresHITL := false

	for _, nd := range s.Detectors {
		res := nd.Detector.Evaluate(interaction)

		// Explicit branch on detection.Outcome (block/warn/pass), plus a
		// fail-closed default — see design.md's "MX-REDECO-03 is warn-level"
		// decision: a binary if(block)/else would silently collapse any
		// future non-block, non-pass outcome into pass/low. Each recognized
		// case below sets its own outcome/severity and its own effect (or
		// lack of effect) on overallOutcome/requiresHITL, so adding a new
		// detection.Outcome value in the future MUST be handled here
		// explicitly, not left to fall through. default is intentionally
		// fail-closed (never fail-open): an unrecognized outcome value can
		// only reach here via a bug or an unwired future detection.Outcome,
		// and silently treating that as a pass would hide the gap instead of
		// surfacing it.
		var outcome core.DetectorOutcome
		var severity core.Severity
		rationale := res.Rationale
		switch res.Outcome {
		case detection.OutcomeBlock:
			outcome = core.DetectorOutcomeFail
			severity = core.SeverityHigh
			overallOutcome = "fail"
			if nd.RequiresHITL {
				requiresHITL = true
			}
		case detection.OutcomeWarn:
			outcome = core.DetectorOutcomeWarn
			severity = core.SeverityMedium
			// A warn row alone never flips overallOutcome or requiresHITL.
		case detection.OutcomePass:
			outcome = core.DetectorOutcomePass
			severity = core.SeverityLow
		default:
			outcome = core.DetectorOutcomeFail
			severity = core.SeverityHigh
			overallOutcome = "fail"
			rationale = fmt.Sprintf("fail-closed: detector %q returned an unrecognized outcome %q: %s", nd.Code, res.Outcome, res.Rationale)
			s.logger().Error("evaluation.unrecognized_detector_outcome",
				"detector_code", nd.Code,
				"outcome", string(res.Outcome),
			)
		}

		results = append(results, DetectorResultInput{
			DetectorCode: nd.Code,
			Outcome:      outcome,
			Severity:     severity,
			Rationale:    rationale,
		})
	}

	var judgeModelID, rubricVersion string
	var judgeConfidence *float64
	var judgeInputTokens, judgeOutputTokens, judgeCacheReadTokens, judgeCacheCreationTokens int64

	for _, nj := range s.Judges {
		res, err := nj.Judge.Evaluate(ctx, judge.JudgeInput{
			Utterances: utterances,
			Rubric:     s.Rubric,
		})
		// judgeModelID/rubricVersion are recorded from every attempt — success
		// or failure — so a fail-closed HITL row still carries evidence of
		// which model/rubric MX-REDECO-05 attempted (issue #4 judgment-day
		// finding: provenance must not go blank just because the judge
		// failed).
		judgeModelID = res.JudgeModelID
		rubricVersion = res.RubricVersion
		judgeInputTokens = res.InputTokens
		judgeOutputTokens = res.OutputTokens
		judgeCacheReadTokens = res.CacheReadInputTokens
		judgeCacheCreationTokens = res.CacheCreationInputTokens
		if err != nil {
			requiresHITL = true
			overallOutcome = "fail"
			results = append(results, DetectorResultInput{
				DetectorCode: nj.Code,
				Outcome:      core.DetectorOutcomeReview,
				Severity:     core.SeverityHigh,
				Rationale:    judgeFailureRationale(err),
			})
			continue
		}

		confidence := res.Confidence
		var severity core.Severity
		var outcome core.DetectorOutcome
		if res.Outcome == judge.OutcomeBlock {
			outcome = core.DetectorOutcomeFail
			severity = core.SeverityCritical
			overallOutcome = "fail"
			requiresHITL = true
		} else {
			outcome = core.DetectorOutcomePass
			severity = core.SeverityLow
		}

		results = append(results, DetectorResultInput{
			DetectorCode: nj.Code,
			Outcome:      outcome,
			Severity:     severity,
			Rationale:    res.Rationale,
			Confidence:   &confidence,
		})
		judgeConfidence = &confidence
	}

	return detectorJudgeRun{
		overallOutcome:           overallOutcome,
		results:                  results,
		requiresHITL:             requiresHITL,
		judgeModelID:             judgeModelID,
		rubricVersion:            rubricVersion,
		judgeConfidence:          judgeConfidence,
		judgeInputTokens:         judgeInputTokens,
		judgeOutputTokens:        judgeOutputTokens,
		judgeCacheReadTokens:     judgeCacheReadTokens,
		judgeCacheCreationTokens: judgeCacheCreationTokens,
	}
}

// ErrInteractionNotFound is returned by ReEvaluateInteraction when
// s.Interactions has no record for the given interactionID.
var ErrInteractionNotFound = errors.New("evaluation: interaction not found")

// ErrPolicyBundleNotFound is returned by ReEvaluateInteraction when the
// requested policyBundleID does not resolve for the interaction's tenant.
// This covers both "no such bundle" and "bundle belongs to a different
// tenant" — both collapse to the same error so nothing about another
// tenant's bundle existence leaks (mirrors httpapi.ErrEvidenceNotFound's
// generic-404 precedent).
var ErrPolicyBundleNotFound = errors.New("evaluation: policy bundle not found")

// ReEvaluationInput is what ReEvaluateInteraction needs to rerun the wired
// detectors/judge for a historical interaction: the tenant it belongs to
// (resolved internally, not supplied by the caller) plus the pure
// detection.Interaction payload and any transcript utterances.
type ReEvaluationInput struct {
	TenantID    string
	Interaction detection.Interaction
	Utterances  []judge.Utterance
}

// InteractionLookup resolves a previously-recorded interaction by id,
// scoped to the authenticated tenant, for ReEvaluateInteraction.
// found=false means no such interaction exists for that tenant — this
// covers both "no such interaction" and "interaction belongs to a
// different tenant" so a foreign-tenant interaction id is
// indistinguishable from an unknown one (mirrors ErrPolicyBundleNotFound's
// same collapsing behavior).
type InteractionLookup interface {
	GetInteractionForReEvaluation(ctx context.Context, tenantID, interactionID string) (ReEvaluationInput, bool, error)
}

// BundleVersionLookup resolves a specific historical PolicyBundle's version
// string by id, scoped to a tenant. found=false covers both "no such
// bundle" and "bundle belongs to a different tenant".
type BundleVersionLookup interface {
	BundleVersionByID(ctx context.Context, tenantID, policyBundleID string) (version string, found bool, err error)
}

// ReEvaluateInteraction reruns the currently-wired detectors/judge against a
// previously-recorded interaction and stamps the caller-supplied historical
// policyBundleID/version onto the resulting evaluation (spec: "Reproducible
// Re-Evaluation Against a Specific Bundle Version"). It does NOT persist:
// s.Store.CreateEvaluation is never called here — the returned
// core.Evaluation exists only to prove the stamping mechanism is
// reproducible (Design Decision 5). It does not select detectors/rubric
// based on bundle rule content: the same wired pipeline always runs,
// independent of which historical bundle is requested.
//
// tenantID is the authenticated caller's tenant. It is checked FIRST, via
// the tenant-scoped s.Interactions lookup, before any bundle lookup or
// detector/judge execution runs — a foreign-tenant or unknown
// interactionID resolves to ErrInteractionNotFound without ever invoking a
// detector or the judge (judgment-day finding: sending another tenant's
// transcript to an external LLM judge before the tenant check is a data
// leak, not just an authorization gap).
func (s Service) ReEvaluateInteraction(ctx context.Context, tenantID, interactionID, policyBundleID string) (core.Evaluation, error) {
	if len(s.Detectors) == 0 {
		return core.Evaluation{}, ErrNoDetectors
	}
	if len(s.Judges) > 1 {
		return core.Evaluation{}, ErrMultipleJudgesNotSupported
	}

	reInput, found, err := s.Interactions.GetInteractionForReEvaluation(ctx, tenantID, interactionID)
	if err != nil {
		return core.Evaluation{}, err
	}
	if !found {
		return core.Evaluation{}, ErrInteractionNotFound
	}

	version, found, err := s.Bundles.BundleVersionByID(ctx, tenantID, policyBundleID)
	if err != nil {
		return core.Evaluation{}, err
	}
	if !found {
		return core.Evaluation{}, ErrPolicyBundleNotFound
	}

	run := s.runDetectorsAndJudges(ctx, reInput.Interaction, reInput.Utterances)

	bundleID := core.ID(policyBundleID)
	return core.Evaluation{
		TenantID:            core.ID(reInput.TenantID),
		InteractionEventID:  core.ID(interactionID),
		OverallOutcome:      run.overallOutcome,
		PolicyBundleVersion: version,
		PolicyBundleID:      &bundleID,
	}, nil
}

// resolveActiveBundle stamps the tenant's active bundle version + id via
// s.Resolver, degrading to the existing ""/nil sentinel on a nil resolver,
// not-found, or resolver error (Design Decision 3: a missing or erroring
// bundle resolution must never hard-fail EvaluateInteraction). A resolver
// error is logged distinctly from the expected not-found case (judgment-day
// finding: silently folding a real DB/resolver failure into "no active
// bundle" hides operational problems), but never changes the returned
// sentinel — ledger golden hashes must stay byte-identical either way.
func (s Service) resolveActiveBundle(ctx context.Context, tenantID string) (version string, id *string) {
	if s.Resolver == nil {
		return "", nil
	}
	v, bundleID, found, err := s.Resolver.ActiveBundle(ctx, tenantID)
	if err != nil {
		s.logger().Error("evaluation.resolve_active_bundle_failed",
			"tenant_id", tenantID,
			"err", err.Error(),
		)
		return "", nil
	}
	if !found {
		return "", nil
	}
	return v, &bundleID
}
