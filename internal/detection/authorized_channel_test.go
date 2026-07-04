package detection_test

import (
	"strings"
	"testing"
	"time"

	"github.com/ricardoalt1515/vigia/internal/core"
	"github.com/ricardoalt1515/vigia/internal/detection"
)

func TestAuthorizedChannelDetector(t *testing.T) {
	detector := detection.AuthorizedChannelDetector{}
	occurredAt := time.Date(2026, 6, 15, 14, 30, 0, 0, time.UTC)

	tests := []struct {
		name              string
		channel           core.InteractionChannel
		authorizedChannel []string
		wantOutcome       detection.Outcome
		rationaleWant     string
	}{
		{
			name:              "channel is in the authorized list passes",
			channel:           core.InteractionChannelCall,
			authorizedChannel: []string{"call", "email"},
			wantOutcome:       detection.OutcomePass,
		},
		{
			name:              "channel is not in the authorized list blocks",
			channel:           core.InteractionChannelMessage,
			authorizedChannel: []string{"call", "email"},
			wantOutcome:       detection.OutcomeBlock,
			rationaleWant:     "message",
		},
		{
			name:              "nil authorized-channel list fails closed",
			channel:           core.InteractionChannelCall,
			authorizedChannel: nil,
			wantOutcome:       detection.OutcomeBlock,
			rationaleWant:     "cannot be verified",
		},
		{
			name:              "empty authorized-channel list fails closed",
			channel:           core.InteractionChannelCall,
			authorizedChannel: []string{},
			wantOutcome:       detection.OutcomeBlock,
			rationaleWant:     "cannot be verified",
		},
		{
			name:              "authorized-channel list comparison is case-sensitive: differently-cased entry blocks",
			channel:           core.InteractionChannelCall,
			authorizedChannel: []string{"CALL"},
			wantOutcome:       detection.OutcomeBlock,
			rationaleWant:     "call",
		},
		{
			name:              "single-entry exact match passes",
			channel:           core.InteractionChannelEmail,
			authorizedChannel: []string{"email"},
			wantOutcome:       detection.OutcomePass,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detector.Evaluate(detection.Interaction{
				OccurredAt:         occurredAt,
				Channel:            tt.channel,
				AuthorizedChannels: tt.authorizedChannel,
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

// TestAuthorizedChannelDetectorNoIO documents and asserts the pure-function
// contract at the signature level, per spec.md "Each new detector performs
// no I/O".
func TestAuthorizedChannelDetectorNoIO(t *testing.T) {
	var d detection.Detector = detection.AuthorizedChannelDetector{}
	in := detection.Interaction{
		OccurredAt:         time.Date(2026, 6, 15, 14, 30, 0, 0, time.UTC),
		Channel:            core.InteractionChannelCall,
		AuthorizedChannels: []string{"call"},
	}
	r1 := d.Evaluate(in)
	r2 := d.Evaluate(in)
	if r1 != r2 {
		t.Fatalf("Evaluate is not deterministic for identical input: %+v vs %+v", r1, r2)
	}
}
