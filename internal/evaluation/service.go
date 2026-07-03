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
)

// ErrNoDetectors is returned by EvaluateInteraction when the Service has no
// configured detectors. An evaluation with zero detector result rows would
// fabricate a "pass" outcome with no supporting evidence, so this case is
// rejected instead of persisted.
var ErrNoDetectors = errors.New("evaluation: no detectors configured")

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

// EvaluateInteractionInput carries the tenant/interaction identity alongside
// the pure detection.Interaction payload the detectors need.
type EvaluateInteractionInput struct {
	TenantID           string
	InteractionEventID string
	Interaction        detection.Interaction
}

// Service runs every configured detector over an interaction, maps outcomes
// to the persisted vocabulary (detector "block" -> core "fail", severity
// high; "pass" -> "pass", severity low), and persists the result via Store.
// The overall evaluation outcome is "fail" if any detector blocks.
type Service struct {
	Detectors []NamedDetector
	Store     EvaluationStore
}

// EvaluateInteraction runs every detector over in.Interaction and persists
// the resulting evaluation header + detector result rows via s.Store.
func (s Service) EvaluateInteraction(ctx context.Context, in EvaluateInteractionInput) (core.Evaluation, error) {
	if len(s.Detectors) == 0 {
		return core.Evaluation{}, ErrNoDetectors
	}

	overallOutcome := "pass"
	results := make([]DetectorResultInput, 0, len(s.Detectors))

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

	return s.Store.CreateEvaluation(ctx, CreateEvaluationInput{
		TenantID:           in.TenantID,
		InteractionEventID: in.InteractionEventID,
		OverallOutcome:     overallOutcome,
		DetectorResults:    results,
	})
}
