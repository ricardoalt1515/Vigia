// Package evaluation orchestrates running detectors over an interaction and
// persisting the resulting evaluation header + detector result child rows.
// The orchestrator resolves no I/O itself beyond calling the injected Store;
// detectors stay pure (internal/detection).
package evaluation

import (
	"context"
	"errors"

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

	overallOutcome := "pass"
	results := make([]DetectorResultInput, 0, len(s.Detectors)+len(s.Judges))

	for _, nd := range s.Detectors {
		res := nd.Detector.Evaluate(in.Interaction)

		var outcome core.DetectorOutcome
		var severity core.Severity
		if res.Outcome == detection.OutcomeBlock {
			outcome = core.DetectorOutcomeFail
			severity = core.SeverityHigh
			overallOutcome = "fail"
		} else {
			outcome = core.DetectorOutcomePass
			severity = core.SeverityLow
		}

		results = append(results, DetectorResultInput{
			DetectorCode: nd.Code,
			Outcome:      outcome,
			Severity:     severity,
			Rationale:    res.Rationale,
		})
	}

	requiresHITL := false
	var judgeModelID, rubricVersion string
	var judgeConfidence *float64

	for _, nj := range s.Judges {
		res, err := nj.Judge.Evaluate(ctx, judge.JudgeInput{
			Utterances: in.Utterances,
			Rubric:     s.Rubric,
		})
		// judgeModelID/rubricVersion are recorded from every attempt — success
		// or failure — so a fail-closed HITL row still carries evidence of
		// which model/rubric MX-REDECO-05 attempted (issue #4 judgment-day
		// finding: provenance must not go blank just because the judge
		// failed).
		judgeModelID = res.JudgeModelID
		rubricVersion = res.RubricVersion
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

	return s.Store.CreateEvaluation(ctx, CreateEvaluationInput{
		TenantID:           in.TenantID,
		InteractionEventID: in.InteractionEventID,
		OverallOutcome:     overallOutcome,
		DetectorResults:    results,
		RequiresHITL:       requiresHITL,
		JudgeModelID:       judgeModelID,
		RubricVersion:      rubricVersion,
		JudgeConfidence:    judgeConfidence,
	})
}
