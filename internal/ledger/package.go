package ledger

import "time"

// PackageInteraction is the interaction slice embedded in an evidence
// export package.
type PackageInteraction struct {
	ID         string    `json:"id"`
	TenantID   string    `json:"tenant_id"`
	Channel    string    `json:"channel"`
	Direction  string    `json:"direction"`
	OccurredAt time.Time `json:"occurred_at"`
}

// PackageEvaluation is the evaluation slice embedded in an evidence export
// package.
type PackageEvaluation struct {
	ID                  string    `json:"id"`
	OverallOutcome      string    `json:"overall_outcome"`
	PolicyBundleVersion string    `json:"policy_bundle_version"`
	CreatedAt           time.Time `json:"created_at"`
}

// PackageRecord is exactly the hashed Body plus the chain proof
// (prev_hash, hash). CreatedAt is rendered as the canonical microsecond-UTC
// string so the package is byte-identical to what was hashed at insert time.
type PackageRecord struct {
	TenantID            string `json:"tenant_id"`
	InteractionEventID  string `json:"interaction_event_id"`
	EvaluationID        string `json:"evaluation_id"`
	Seq                 int64  `json:"seq"`
	OverallOutcome      string `json:"overall_outcome"`
	PolicyBundleVersion string `json:"policy_bundle_version"`
	InputsDigest        string `json:"inputs_digest"`
	CreatedAt           string `json:"created_at"`
	PrevHash            string `json:"prev_hash"`
	Hash                string `json:"hash"`
}

// Package is the self-contained evidence export DTO for one interaction.
// VerifyPackage re-verifies it with no database access.
type Package struct {
	SchemaVersion   string             `json:"schema_version"`
	Interaction     PackageInteraction `json:"interaction"`
	Evaluation      PackageEvaluation  `json:"evaluation"`
	DetectorResults []DetectorResult   `json:"detector_results"`
	Record          PackageRecord      `json:"record"`
}

// schemaVersion is the current Package schema identifier.
const schemaVersion = "vigia.evidence.v1"

// BuildPackage assembles the self-contained export DTO (pure).
func BuildPackage(rec EvidenceRecord, interaction PackageInteraction, eval PackageEvaluation, results []DetectorResult) Package {
	return Package{
		SchemaVersion:   schemaVersion,
		Interaction:     interaction,
		Evaluation:      eval,
		DetectorResults: results,
		Record: PackageRecord{
			TenantID:            rec.Body.TenantID,
			InteractionEventID:  rec.Body.InteractionEventID,
			EvaluationID:        rec.Body.EvaluationID,
			Seq:                 rec.Body.Seq,
			OverallOutcome:      rec.Body.OverallOutcome,
			PolicyBundleVersion: rec.Body.PolicyBundleVersion,
			InputsDigest:        rec.Body.InputsDigest,
			CreatedAt:           rec.Body.CreatedAt.UTC().Format(canonicalTimeLayout),
			PrevHash:            rec.PrevHash,
			Hash:                rec.Hash,
		},
	}
}

// VerifyPackage re-verifies an export with NO DB access. It cross-validates
// the display blocks (Evaluation, Interaction) against the verified Record
// before recomputing inputs_digest from detector_results and the record hash
// from prev_hash || canonical(body). Evaluation and Interaction carry
// independently mutable copies of identity/outcome fields that are NOT
// covered by the hash or inputs_digest recomputation, so a tampered display
// block must be caught here or VerifyPackage would report OK on a
// self-contradictory package.
func VerifyPackage(pkg Package) VerifyResult {
	if pkg.Evaluation.ID != pkg.Record.EvaluationID {
		return VerifyResult{OK: false, Count: 1, BreakAtSeq: pkg.Record.Seq, BreakReason: "evaluation id mismatch"}
	}
	if pkg.Evaluation.OverallOutcome != pkg.Record.OverallOutcome {
		return VerifyResult{OK: false, Count: 1, BreakAtSeq: pkg.Record.Seq, BreakReason: "evaluation overall_outcome mismatch"}
	}
	if pkg.Evaluation.PolicyBundleVersion != pkg.Record.PolicyBundleVersion {
		return VerifyResult{OK: false, Count: 1, BreakAtSeq: pkg.Record.Seq, BreakReason: "evaluation policy_bundle_version mismatch"}
	}
	if pkg.Interaction.ID != pkg.Record.InteractionEventID {
		return VerifyResult{OK: false, Count: 1, BreakAtSeq: pkg.Record.Seq, BreakReason: "interaction id mismatch"}
	}
	if pkg.Interaction.TenantID != pkg.Record.TenantID {
		return VerifyResult{OK: false, Count: 1, BreakAtSeq: pkg.Record.Seq, BreakReason: "interaction tenant_id mismatch"}
	}

	recomputedDigest := ComputeInputsDigest(pkg.DetectorResults)
	if recomputedDigest != pkg.Record.InputsDigest {
		return VerifyResult{OK: false, Count: 1, BreakAtSeq: pkg.Record.Seq, BreakReason: "inputs_digest mismatch"}
	}

	createdAt, err := time.Parse(canonicalTimeLayout, pkg.Record.CreatedAt)
	if err != nil {
		return VerifyResult{OK: false, Count: 1, BreakAtSeq: pkg.Record.Seq, BreakReason: "hash mismatch"}
	}

	body := Body{
		TenantID:            pkg.Record.TenantID,
		InteractionEventID:  pkg.Record.InteractionEventID,
		EvaluationID:        pkg.Record.EvaluationID,
		Seq:                 pkg.Record.Seq,
		OverallOutcome:      pkg.Record.OverallOutcome,
		PolicyBundleVersion: pkg.Record.PolicyBundleVersion,
		InputsDigest:        pkg.Record.InputsDigest,
		CreatedAt:           createdAt,
	}

	if Hash(pkg.Record.PrevHash, body) != pkg.Record.Hash {
		return VerifyResult{OK: false, Count: 1, BreakAtSeq: pkg.Record.Seq, BreakReason: "hash mismatch"}
	}

	return VerifyResult{OK: true, Count: 1}
}
