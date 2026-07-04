package detection

import "fmt"

// AuthorizedChannelDetector evaluates whether an interaction's Channel is
// present in the debtor's authorized-channel list (MX-REDECO-11). Channel
// itself is always populated on every interaction; only the authorized list
// can be absent. A nil or empty list fails closed to BLOCK: the detector
// never assumes an unverifiable channel is authorized. The comparison is
// exact-string and case-sensitive, matching the exact-match style already
// used by ThirdPartyContactDetector's relationship comparison.
type AuthorizedChannelDetector struct{}

// Evaluate compares in.Channel against in.AuthorizedChannels. If the list is
// nil/empty, the outcome fails closed to BLOCK. Otherwise, the channel MUST
// appear verbatim in the list to PASS; any other value, including a
// differently-cased match, BLOCKs.
func (d AuthorizedChannelDetector) Evaluate(in Interaction) Result {
	if len(in.AuthorizedChannels) == 0 {
		return Result{
			Outcome:   OutcomeBlock,
			Rationale: "authorized-channel list is missing; channel cannot be verified as authorized",
		}
	}

	channel := string(in.Channel)
	for _, authorized := range in.AuthorizedChannels {
		if authorized == channel {
			return Result{
				Outcome:   OutcomePass,
				Rationale: fmt.Sprintf("channel %q is in the authorized-channel list", channel),
			}
		}
	}

	return Result{
		Outcome:   OutcomeBlock,
		Rationale: fmt.Sprintf("channel %q is not in the authorized-channel list", channel),
	}
}

var _ Detector = AuthorizedChannelDetector{}
