package detection_test

import (
	"strings"
	"testing"
	"time"

	"github.com/ricardoalt1515/vigia/internal/detection"
)

func TestPaymentRoutingDetector(t *testing.T) {
	detector := detection.PaymentRoutingDetector{}
	occurredAt := time.Date(2026, 6, 15, 14, 30, 0, 0, time.UTC)

	tests := []struct {
		name             string
		paymentRecipient string
		wantOutcome      detection.Outcome
		rationaleWant    string
	}{
		{
			name:             "payment routed to the creditor passes",
			paymentRecipient: "creditor",
			wantOutcome:      detection.OutcomePass,
		},
		{
			name:             "payment routed to a non-creditor recipient blocks",
			paymentRecipient: "collector",
			wantOutcome:      detection.OutcomeBlock,
			rationaleWant:    "collector",
		},
		{
			name:             "missing recipient designation fails closed",
			paymentRecipient: "",
			wantOutcome:      detection.OutcomeBlock,
			rationaleWant:    "cannot be verified",
		},
		{
			name:             "unknown recipient value blocks",
			paymentRecipient: "third_party_agent",
			wantOutcome:      detection.OutcomeBlock,
			rationaleWant:    "third_party_agent",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detector.Evaluate(detection.Interaction{
				OccurredAt:       occurredAt,
				PaymentRecipient: tt.paymentRecipient,
			})
			if result.Outcome != tt.wantOutcome {
				t.Fatalf("outcome = %q, want %q (rationale: %s)", result.Outcome, tt.wantOutcome, result.Rationale)
			}
			if tt.rationaleWant != "" && !strings.Contains(result.Rationale, tt.rationaleWant) {
				t.Fatalf("rationale = %q, want it to contain %q", result.Rationale, tt.rationaleWant)
			}
		})
	}
}

// TestPaymentRoutingDetectorNoIO documents and asserts the pure-function
// contract at the signature level, per spec.md "Each new detector performs
// no I/O".
func TestPaymentRoutingDetectorNoIO(t *testing.T) {
	var d detection.Detector = detection.PaymentRoutingDetector{}
	in := detection.Interaction{
		OccurredAt:       time.Date(2026, 6, 15, 14, 30, 0, 0, time.UTC),
		PaymentRecipient: "creditor",
	}
	r1 := d.Evaluate(in)
	r2 := d.Evaluate(in)
	if r1 != r2 {
		t.Fatalf("Evaluate is not deterministic for identical input: %+v vs %+v", r1, r2)
	}
}
