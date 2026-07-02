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
