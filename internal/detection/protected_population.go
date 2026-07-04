package detection

import (
	"fmt"
	"time"
)

// legalMajorityAge and elderlyAge are the fixed legal-age thresholds MX-
// REDECO-07 (protected population) evaluates against. Both are plain Go
// constants, not tenant/bundle configuration: REDECO and the cited statute
// (Ley de los Derechos de las Personas Adultas Mayores) define them as fixed
// legal ages, not tenant policy choices. These values MUST be confirmed by
// legal counsel before production use (see design.md's open item).
const (
	legalMajorityAge = 18
	elderlyAge       = 60
)

// ProtectedPopulationDetector evaluates whether an interaction's contacted
// party is a protected minor or elderly person (MX-REDECO-07). A minor is
// protected regardless of ContactPartyRelationship — the debtor exemption
// applies only to the elderly case. Age is always computed relative to
// Interaction.OccurredAt, never time.Now(), so the outcome is stable across
// re-evaluation.
type ProtectedPopulationDetector struct{}

// Evaluate implements the decision table from design.md's "debtor-
// relationship precedence in protected-population detection" decision:
//  1. Age below legalMajorityAge as of OccurredAt -> BLOCK, always (no
//     debtor exemption for minors).
//  2. Age at/above elderlyAge as of OccurredAt -> BLOCK, unless
//     ContactPartyRelationship == "debtor" (elderly-debtor exemption) -> PASS.
//  3. Age between the two thresholds -> PASS, regardless of relationship.
//  4. ContactedPartyDOB missing (nil) and relationship == "debtor" -> PASS
//     (accepted residual risk; DOB is a sparsely-populated new field).
//  5. ContactedPartyDOB missing (nil) and relationship is anything else
//     (including unset) -> BLOCK, fail-closed: age cannot be verified for a
//     non-debtor contact.
func (d ProtectedPopulationDetector) Evaluate(in Interaction) Result {
	isDebtor := in.ContactPartyRelationship == "debtor"

	if in.ContactedPartyDOB == nil {
		if isDebtor {
			return Result{
				Outcome:   OutcomePass,
				Rationale: "contacted party's date of birth is missing but relationship is debtor; a minor debtor with no DOB on file is an accepted residual risk",
			}
		}
		return Result{
			Outcome:   OutcomeBlock,
			Rationale: "contacted party's age cannot be verified: date of birth is missing and relationship is not debtor",
		}
	}

	age := ageAt(*in.ContactedPartyDOB, in.OccurredAt)

	if age < legalMajorityAge {
		return Result{
			Outcome:   OutcomeBlock,
			Rationale: fmt.Sprintf("contacted party is below the protected age of majority (age %d as of the interaction) — the debtor exemption never applies to minors", age),
		}
	}

	if age >= elderlyAge {
		if isDebtor {
			return Result{
				Outcome:   OutcomePass,
				Rationale: fmt.Sprintf("contacted party is elderly (age %d as of the interaction) but is the debtor; the elderly-debtor exemption applies", age),
			}
		}
		return Result{
			Outcome:   OutcomeBlock,
			Rationale: fmt.Sprintf("contacted party is a protected elderly person (age %d as of the interaction) and is not the debtor", age),
		}
	}

	return Result{
		Outcome:   OutcomePass,
		Rationale: fmt.Sprintf("contacted party's age (%d as of the interaction) is between the protected thresholds", age),
	}
}

// ageAt computes dob's age in whole years as of instant, never time.Now().
// It is a pure calendar calculation: an age of N means N birthdays have
// occurred at or before instant.
func ageAt(dob, instant time.Time) int {
	age := instant.Year() - dob.Year()
	// Adjust down by one if instant falls before dob's birthday in the
	// current year (birthday hasn't occurred yet this year).
	anniversary := time.Date(instant.Year(), dob.Month(), dob.Day(), dob.Hour(), dob.Minute(), dob.Second(), dob.Nanosecond(), dob.Location())
	if instant.Before(anniversary) {
		age--
	}
	return age
}

var _ Detector = ProtectedPopulationDetector{}
