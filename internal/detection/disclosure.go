package detection

// DisclosureDetector evaluates whether the required UNE/complaints-unit
// disclosure was stated during an interaction (MX-REDECO-03,
// docs/regulatory-ruleset.md:33). Unlike the hard-block detectors,
// MX-REDECO-03's catalog action is WARN-level: this detector never returns
// OutcomeBlock. When the disclosure was not stated, or its presence cannot
// be verified at all (DisclosureProvided == nil), the outcome fails closed
// to OutcomeWarn — fail-closed behavior for a warn-level rule cannot escalate
// to a hard block.
type DisclosureDetector struct{}

// Evaluate inspects Interaction.DisclosureProvided. true passes; false warns
// (disclosure confirmed not stated); nil warns (disclosure presence unknown).
func (d DisclosureDetector) Evaluate(in Interaction) Result {
	if in.DisclosureProvided == nil {
		return Result{
			Outcome:   OutcomeWarn,
			Rationale: "disclosure presence cannot be verified; failing closed to warn",
		}
	}

	if *in.DisclosureProvided {
		return Result{
			Outcome:   OutcomePass,
			Rationale: "required UNE/complaints-unit disclosure was stated",
		}
	}

	return Result{
		Outcome:   OutcomeWarn,
		Rationale: "required UNE/complaints-unit disclosure was not stated",
	}
}

var _ Detector = DisclosureDetector{}
