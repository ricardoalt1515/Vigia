package detection

import "fmt"

// PaymentRoutingDetector evaluates whether an interaction's payment-
// recipient designation identifies the creditor as recipient (MX-REDECO-10).
// Only "creditor" passes; any other value, including an unset/empty
// designation, fails closed to BLOCK: the detector never assumes an
// unverified recipient is the creditor.
type PaymentRoutingDetector struct{}

// Evaluate inspects Interaction.PaymentRecipient. "creditor" passes;
// anything else, including the empty string, blocks.
func (d PaymentRoutingDetector) Evaluate(in Interaction) Result {
	switch in.PaymentRecipient {
	case "":
		return Result{
			Outcome:   OutcomeBlock,
			Rationale: "payment-recipient designation is missing; recipient cannot be verified as the creditor",
		}
	case "creditor":
		return Result{
			Outcome:   OutcomePass,
			Rationale: "payment is routed to the creditor",
		}
	default:
		return Result{
			Outcome:   OutcomeBlock,
			Rationale: fmt.Sprintf("payment-recipient designation %q does not identify the creditor as recipient", in.PaymentRecipient),
		}
	}
}

var _ Detector = PaymentRoutingDetector{}
