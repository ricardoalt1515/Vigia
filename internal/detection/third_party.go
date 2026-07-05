package detection

import "fmt"

// ThirdPartyContactDetector evaluates whether an interaction's contacted
// party is the debtor or a debtor-authorized third party (MX-REDECO-06).
// Any other relationship — including an unset/unknown one — fails closed to
// BLOCK: the detector never assumes an unproven relationship is authorized.
type ThirdPartyContactDetector struct{}

// Evaluate inspects Interaction.ContactPartyRelationship. "debtor" and
// "authorized" pass; anything else, including the empty string, blocks.
func (d ThirdPartyContactDetector) Evaluate(in Interaction) Result {
	switch in.ContactPartyRelationship {
	case "":
		return Result{
			Outcome:   OutcomeBlock,
			Rationale: "contact-party relationship is unknown and cannot be proven authorized",
		}
	case "debtor", "authorized":
		return Result{
			Outcome:   OutcomePass,
			Rationale: fmt.Sprintf("contact-party relationship %q is authorized", in.ContactPartyRelationship),
		}
	default:
		return Result{
			Outcome:   OutcomeBlock,
			Rationale: fmt.Sprintf("contact-party relationship %q is not the debtor or an authorized third party", in.ContactPartyRelationship),
		}
	}
}

var _ Detector = ThirdPartyContactDetector{}
