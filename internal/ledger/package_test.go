package ledger_test

import (
	"strings"
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

// buildTestJudgedPackage builds a package from a judged record's
// Body.Judge, mirroring buildTestPackage but with the judge sub-object set.
func buildTestJudgedPackage(t *testing.T) ledger.Package {
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
		Judge: &ledger.JudgeEvidence{
			RubricVersion: "mx-redeco-05.tone-threat.v1",
			JudgeModelID:  "claude-haiku-4-5-20251001",
			Confidence:    "0.9500",
		},
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

// TestBuildPackageIncludesJudgeSubObject covers *A new judged record's
// evidence body carries rubric_version and judge_model_id* at the package
// export layer: BuildPackage copies Body.Judge into the exported record.
func TestBuildPackageIncludesJudgeSubObject(t *testing.T) {
	pkg := buildTestJudgedPackage(t)

	if pkg.Record.Judge == nil {
		t.Fatal("pkg.Record.Judge is nil, want the judge sub-object copied from Body.Judge")
	}
	if pkg.Record.Judge.RubricVersion != "mx-redeco-05.tone-threat.v1" {
		t.Fatalf("Judge.RubricVersion = %q, want mx-redeco-05.tone-threat.v1", pkg.Record.Judge.RubricVersion)
	}
	if pkg.Record.Judge.JudgeModelID != "claude-haiku-4-5-20251001" {
		t.Fatalf("Judge.JudgeModelID = %q, want claude-haiku-4-5-20251001", pkg.Record.Judge.JudgeModelID)
	}
	if pkg.Record.Judge.Confidence != "0.9500" {
		t.Fatalf("Judge.Confidence = %q, want 0.9500", pkg.Record.Judge.Confidence)
	}
}

// TestVerifyPackageOldPackagesWithNoJudgeKeyStillVerify covers backward
// compatibility: a package with no judge key (the pre-#4 shape) still
// verifies byte-identically.
func TestVerifyPackageOldPackagesWithNoJudgeKeyStillVerify(t *testing.T) {
	pkg := buildTestPackage(t)
	if pkg.Record.Judge != nil {
		t.Fatal("buildTestPackage's pkg.Record.Judge is non-nil, want nil for the judge-less fixture")
	}

	result := ledger.VerifyPackage(pkg)
	if !result.OK {
		t.Fatalf("VerifyPackage() OK = false, reason %q, want intact for a judge-less package", result.BreakReason)
	}
}

// TestVerifyPackageJudgedRecordRoundTrips covers *A new judged record's
// evidence body carries rubric_version and judge_model_id* end-to-end: a
// package built from a judged record's Body.Judge round-trips through
// VerifyPackage.
func TestVerifyPackageJudgedRecordRoundTrips(t *testing.T) {
	pkg := buildTestJudgedPackage(t)

	result := ledger.VerifyPackage(pkg)
	if !result.OK {
		t.Fatalf("VerifyPackage() OK = false, reason %q, want intact for a judged package", result.BreakReason)
	}
}

func TestVerifyPackageDetectsTamperedJudgeConfidence(t *testing.T) {
	pkg := buildTestJudgedPackage(t)
	pkg.Record.Judge.Confidence = "0.8000"

	result := ledger.VerifyPackage(pkg)
	if result.OK {
		t.Fatal("VerifyPackage() OK = true, want tampering of Judge.Confidence detected")
	}
	if result.BreakReason != "hash mismatch" {
		t.Fatalf("BreakReason = %q, want hash mismatch", result.BreakReason)
	}
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
		{
			name:       "garbage record prev_hash",
			tamper:     func(p *ledger.Package) { p.Record.PrevHash = "not-hex" },
			wantReason: "invalid prev_hash format",
		},
		{
			name: "valid-format but wrong record prev_hash",
			tamper: func(p *ledger.Package) {
				p.Record.PrevHash = strings.Repeat("a", 64)
			},
			wantReason: "hash mismatch",
		},
		{
			name:       "tampered record created_at",
			tamper:     func(p *ledger.Package) { p.Record.CreatedAt = "2026-06-21T10:00:00.000000Z" },
			wantReason: "hash mismatch",
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
