package orchestrator

import "time"

type ComplaintCaseState string

const (
	ComplaintStateOpen           ComplaintCaseState = "open"
	ComplaintStateAwaitingReview ComplaintCaseState = "awaiting_review"
	ComplaintStateEscalated      ComplaintCaseState = "escalated"
	ComplaintStateResolved       ComplaintCaseState = "resolved"
)

type ComplaintTransitionKind string

const (
	TransitionOpen          ComplaintTransitionKind = "open"
	TransitionRequestReview ComplaintTransitionKind = "request_review"
	TransitionApprove       ComplaintTransitionKind = "approve"
	TransitionOverride      ComplaintTransitionKind = "override"
	TransitionTTLExpired    ComplaintTransitionKind = "ttl_expired"
	TransitionSLABreach     ComplaintTransitionKind = "sla_breach"
)

func (kind ComplaintTransitionKind) AllowsHumanReviewID() bool {
	return kind == TransitionApprove || kind == TransitionOverride
}

type ComplaintTransitionPlan struct {
	From ComplaintCaseState
	Kind ComplaintTransitionKind
	To   ComplaintCaseState
}

func PlanComplaintTransition(from ComplaintCaseState, kind ComplaintTransitionKind) (ComplaintTransitionPlan, bool) {
	to, ok := nextComplaintState(from, kind)
	if !ok {
		return ComplaintTransitionPlan{}, false
	}
	return ComplaintTransitionPlan{From: from, Kind: kind, To: to}, true
}

func nextComplaintState(from ComplaintCaseState, kind ComplaintTransitionKind) (ComplaintCaseState, bool) {
	switch kind {
	case TransitionRequestReview:
		if from == ComplaintStateOpen {
			return ComplaintStateAwaitingReview, true
		}
	case TransitionApprove, TransitionOverride:
		if from == ComplaintStateAwaitingReview {
			return ComplaintStateResolved, true
		}
	case TransitionTTLExpired:
		if from == ComplaintStateAwaitingReview {
			return ComplaintStateEscalated, true
		}
	case TransitionSLABreach:
		if from == ComplaintStateOpen || from == ComplaintStateAwaitingReview {
			return ComplaintStateEscalated, true
		}
	}
	return "", false
}

type ComplaintCase struct {
	ID              string
	TenantID        string
	InteractionID   string
	RedecoCause     string
	State           string
	OpenedAt        time.Time
	SLADueAt        time.Time
	CalendarVersion string
	ReviewExpiresAt *time.Time
	ResolvedAt      *time.Time
	IdempotencyKey  string
	CreatedAt       time.Time
	UpdatedAt       time.Time
	Created         bool
}

type CreateComplaintCaseInput struct {
	TenantID        string
	InteractionID   string
	RedecoCause     string
	OpenedAt        time.Time
	SLADueAt        time.Time
	CalendarVersion string
	IdempotencyKey  string
}

type ApplyComplaintTransitionInput struct {
	TenantID        string
	ComplaintCaseID string
	Kind            ComplaintTransitionKind
	Now             time.Time
	ReviewExpiresAt time.Time
	HumanReviewID   *string
}

type ApplyComplaintTransitionResult struct {
	Applied bool
	Case    ComplaintCase
}

type CreateHumanReviewInput struct {
	TenantID        string
	ComplaintCaseID string
	Decision        string
	Reviewer        string
	Notes           string
}
