package ledger_test

import (
	"testing"
	"time"

	"github.com/ricardoalt1515/vigia/internal/ledger"
)

func buildTestPackage(t *testing.T) ledger.Package {
	t.Helper()

	results := baselineDetectorResults()
	inputsDigest := ledger.ComputeInputsDigest(results)
	createdAt := time.Date(2026, 6, 20, 10, 0, 0, 0, time.UTC)

	body := ledger.Body{
		TenantID:            "tenant-1",
		InteractionEventID:  "interaction-1",
		EvaluationID:        "eval-1",
		Seq:                 1,
		OverallOutcome:      "fail",
		PolicyBundleVersion: "",
		InputsDigest:        inputsDigest,
		CreatedAt:           createdAt,
	}
	hash := ledger.Hash(ledger.GenesisPrevHash, body)

	rec := ledger.EvidenceRecord{
		ID:       "record-1",
		Body:     body,
		PrevHash: ledger.GenesisPrevHash,
		Hash:     hash,
	}

	return ledger.BuildPackage(rec, ledger.PackageInteraction{
		ID:         "interaction-1",
		TenantID:   "tenant-1",
		Channel:    "phone",
		Direction:  "outbound",
		OccurredAt: createdAt.Add(-time.Minute),
	}, ledger.PackageEvaluation{
		ID:                  "eval-1",
		OverallOutcome:      "fail",
		PolicyBundleVersion: "",
		CreatedAt:           createdAt,
	}, results)
}

func TestBuildAndVerifyPackageIntact(t *testing.T) {
	pkg := buildTestPackage(t)

	result := ledger.VerifyPackage(pkg)

	if !result.OK {
		t.Fatalf("VerifyPackage() OK = false, reason %q, want intact", result.BreakReason)
	}
}

func TestVerifyPackageDetectsTamperedHash(t *testing.T) {
	pkg := buildTestPackage(t)
	pkg.Record.Hash = "tampered-hash"

	result := ledger.VerifyPackage(pkg)

	if result.OK {
		t.Fatal("VerifyPackage() OK = true, want tampering detected")
	}
}

func TestVerifyPackageDetectsTamperedDetectorResultWithoutDigestUpdate(t *testing.T) {
	pkg := buildTestPackage(t)
	pkg.DetectorResults[0].Rationale = "tampered rationale"

	result := ledger.VerifyPackage(pkg)

	if result.OK {
		t.Fatal("VerifyPackage() OK = true, want tampering detected")
	}
	if result.BreakReason != "inputs_digest mismatch" {
		t.Fatalf("BreakReason = %q, want %q", result.BreakReason, "inputs_digest mismatch")
	}
}

// TestVerifyPackageDetectsTampering covers every cross-checked display-block
// field (Evaluation/Interaction vs. Record) plus every hash-contributing
// Record field, mirroring TestVerifyChainDetectsTampering. Evaluation and
// Interaction carry independently mutable copies that are NOT covered by the
// inputs_digest or hash recomputation, so a tampered display block must
// still be caught by VerifyPackage's cross-validation.
func TestVerifyPackageDetectsTampering(t *testing.T) {
	tests := []struct {
		name       string
		tamper     func(*ledger.Package)
		wantReason string
	}{
		// Cross-checked display-block fields (caught before hash/digest
		// recomputation runs).
		{
			name:       "tampered evaluation id",
			tamper:     func(p *ledger.Package) { p.Evaluation.ID = "eval-2" },
			wantReason: "evaluation id mismatch",
		},
		{
			name:       "tampered evaluation overall_outcome",
			tamper:     func(p *ledger.Package) { p.Evaluation.OverallOutcome = "pass" },
			wantReason: "evaluation overall_outcome mismatch",
		},
		{
			name:       "tampered evaluation policy_bundle_version",
			tamper:     func(p *ledger.Package) { p.Evaluation.PolicyBundleVersion = "v2" },
			wantReason: "evaluation policy_bundle_version mismatch",
		},
		{
			name:       "tampered interaction id",
			tamper:     func(p *ledger.Package) { p.Interaction.ID = "interaction-2" },
			wantReason: "interaction id mismatch",
		},
		{
			name:       "tampered interaction tenant_id",
			tamper:     func(p *ledger.Package) { p.Interaction.TenantID = "tenant-2" },
			wantReason: "interaction tenant_id mismatch",
		},
		// Hash-contributing Record fields. OverallOutcome, PolicyBundleVersion,
		// InteractionEventID, TenantID, and EvaluationID are also caught by
		// the cross-checks above (since only the Record side changed, it now
		// disagrees with the still-original Evaluation/Interaction block).
		// Seq is not cross-checked, so it falls through to the hash
		// recomputation, which then reports "hash mismatch".
		{
			name:       "tampered record overall_outcome",
			tamper:     func(p *ledger.Package) { p.Record.OverallOutcome = "pass" },
			wantReason: "evaluation overall_outcome mismatch",
		},
		{
			name:       "tampered record policy_bundle_version",
			tamper:     func(p *ledger.Package) { p.Record.PolicyBundleVersion = "v2" },
			wantReason: "evaluation policy_bundle_version mismatch",
		},
		{
			name:       "tampered record seq",
			tamper:     func(p *ledger.Package) { p.Record.Seq = 2 },
			wantReason: "hash mismatch",
		},
		{
			name:       "tampered record interaction_event_id",
			tamper:     func(p *ledger.Package) { p.Record.InteractionEventID = "interaction-2" },
			wantReason: "interaction id mismatch",
		},
		{
			name:       "tampered record tenant_id",
			tamper:     func(p *ledger.Package) { p.Record.TenantID = "tenant-2" },
			wantReason: "interaction tenant_id mismatch",
		},
		{
			name:       "tampered record evaluation_id",
			tamper:     func(p *ledger.Package) { p.Record.EvaluationID = "eval-2" },
			wantReason: "evaluation id mismatch",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pkg := buildTestPackage(t)
			tt.tamper(&pkg)

			result := ledger.VerifyPackage(pkg)

			if result.OK {
				t.Fatal("VerifyPackage() OK = true, want tampering detected")
			}
			if result.BreakReason != tt.wantReason {
				t.Fatalf("BreakReason = %q, want %q", result.BreakReason, tt.wantReason)
			}
		})
	}
}
