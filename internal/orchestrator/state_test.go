package orchestrator

import "testing"

func TestPlanComplaintTransitionValidTransitions(t *testing.T) {
	tests := []struct {
		name string
		from ComplaintCaseState
		kind ComplaintTransitionKind
		want ComplaintCaseState
	}{
		{name: "open requests review", from: ComplaintStateOpen, kind: TransitionRequestReview, want: ComplaintStateAwaitingReview},
		{name: "awaiting review approves", from: ComplaintStateAwaitingReview, kind: TransitionApprove, want: ComplaintStateResolved},
		{name: "awaiting review overrides", from: ComplaintStateAwaitingReview, kind: TransitionOverride, want: ComplaintStateResolved},
		{name: "review ttl expires", from: ComplaintStateAwaitingReview, kind: TransitionTTLExpired, want: ComplaintStateEscalated},
		{name: "open sla breach escalates", from: ComplaintStateOpen, kind: TransitionSLABreach, want: ComplaintStateEscalated},
		{name: "awaiting review sla breach escalates", from: ComplaintStateAwaitingReview, kind: TransitionSLABreach, want: ComplaintStateEscalated},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan, ok := PlanComplaintTransition(tt.from, tt.kind)
			if !ok {
				t.Fatalf("PlanComplaintTransition(%q, %q) rejected valid transition", tt.from, tt.kind)
			}
			if plan.From != tt.from || plan.To != tt.want || plan.Kind != tt.kind {
				t.Fatalf("plan = %+v, want from=%q kind=%q to=%q", plan, tt.from, tt.kind, tt.want)
			}
		})
	}
}

func TestComplaintTransitionKindAllowsHumanReviewIDOnlyForHumanResolutions(t *testing.T) {
	tests := []struct {
		kind ComplaintTransitionKind
		want bool
	}{
		{kind: TransitionApprove, want: true},
		{kind: TransitionOverride, want: true},
		{kind: TransitionRequestReview, want: false},
		{kind: TransitionTTLExpired, want: false},
		{kind: TransitionSLABreach, want: false},
		{kind: TransitionOpen, want: false},
	}

	for _, tt := range tests {
		if got := tt.kind.AllowsHumanReviewID(); got != tt.want {
			t.Fatalf("%q AllowsHumanReviewID() = %v, want %v", tt.kind, got, tt.want)
		}
	}
}

func TestPlanComplaintTransitionRejectsInvalidAndTerminalTransitions(t *testing.T) {
	tests := []struct {
		from ComplaintCaseState
		kind ComplaintTransitionKind
	}{
		{from: ComplaintStateOpen, kind: TransitionApprove},
		{from: ComplaintStateOpen, kind: TransitionOverride},
		{from: ComplaintStateResolved, kind: TransitionSLABreach},
		{from: ComplaintStateEscalated, kind: TransitionRequestReview},
		{from: ComplaintCaseState("unknown"), kind: TransitionRequestReview},
		{from: ComplaintStateOpen, kind: ComplaintTransitionKind("bogus")},
	}

	for _, tt := range tests {
		if plan, ok := PlanComplaintTransition(tt.from, tt.kind); ok {
			t.Fatalf("PlanComplaintTransition(%q, %q) = %+v, want rejected", tt.from, tt.kind, plan)
		}
	}
}
