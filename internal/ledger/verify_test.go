package ledger_test

import (
	"testing"
	"time"

	"github.com/ricardoalt1515/vigia/internal/ledger"
)

// buildChain constructs an intact, honestly-produced chain of n records for
// one tenant, following the real append algorithm: seq starts at 1,
// prev_hash chains to the previous record's hash, genesis prev_hash is "".
func buildChain(n int) []ledger.EvidenceRecord {
	records := make([]ledger.EvidenceRecord, 0, n)
	prevHash := ledger.GenesisPrevHash
	for i := 1; i <= n; i++ {
		body := ledger.Body{
			TenantID:            "tenant-1",
			InteractionEventID:  "interaction-1",
			EvaluationID:        "eval-" + string(rune('0'+i)),
			Seq:                 int64(i),
			OverallOutcome:      "pass",
			PolicyBundleVersion: "",
			InputsDigest:        "digest",
			CreatedAt:           time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC).Add(time.Duration(i) * time.Minute),
		}
		hash := ledger.Hash(prevHash, body)
		records = append(records, ledger.EvidenceRecord{
			ID:       "record-" + string(rune('0'+i)),
			Body:     body,
			PrevHash: prevHash,
			Hash:     hash,
		})
		prevHash = hash
	}
	return records
}

func TestVerifyChainIntact(t *testing.T) {
	records := buildChain(3)

	result := ledger.VerifyChain(records)

	if !result.OK {
		t.Fatalf("VerifyChain() OK = false, reason %q at seq %d, want intact", result.BreakReason, result.BreakAtSeq)
	}
	if result.Count != 3 {
		t.Fatalf("Count = %d, want 3", result.Count)
	}
}

func TestVerifyChainEmpty(t *testing.T) {
	result := ledger.VerifyChain(nil)

	if !result.OK {
		t.Fatalf("VerifyChain(nil) OK = false, want true")
	}
	if result.Count != 0 {
		t.Fatalf("Count = %d, want 0", result.Count)
	}
}

func TestVerifyChainSingleRecord(t *testing.T) {
	records := buildChain(1)

	result := ledger.VerifyChain(records)

	if !result.OK {
		t.Fatalf("VerifyChain() OK = false, reason %q, want intact", result.BreakReason)
	}
	if result.Count != 1 {
		t.Fatalf("Count = %d, want 1", result.Count)
	}
}

func TestVerifyChainDetectsTampering(t *testing.T) {
	tests := []struct {
		name        string
		tamper      func([]ledger.EvidenceRecord) []ledger.EvidenceRecord
		wantBreakAt int64
		wantReason  string
	}{
		{
			name: "flipped overall_outcome causes hash mismatch",
			tamper: func(rs []ledger.EvidenceRecord) []ledger.EvidenceRecord {
				rs[1].Body.OverallOutcome = "fail"
				return rs
			},
			wantBreakAt: 2,
			wantReason:  "hash mismatch",
		},
		{
			name: "dropped record causes seq gap",
			tamper: func(rs []ledger.EvidenceRecord) []ledger.EvidenceRecord {
				return append(rs[:1], rs[2:]...)
			},
			wantBreakAt: 3,
			wantReason:  "seq gap",
		},
		{
			name: "rewritten prev_hash causes linkage break",
			tamper: func(rs []ledger.EvidenceRecord) []ledger.EvidenceRecord {
				rs[2].PrevHash = "rewritten-hash"
				return rs
			},
			wantBreakAt: 3,
			wantReason:  "prev_hash linkage",
		},
		{
			name: "bad genesis prev_hash on record 0",
			tamper: func(rs []ledger.EvidenceRecord) []ledger.EvidenceRecord {
				rs[0].PrevHash = "not-genesis"
				return rs
			},
			wantBreakAt: 1,
			wantReason:  "genesis prev_hash",
		},
		{
			name: "tampered seq on record 0 breaks genesis seq invariant",
			tamper: func(rs []ledger.EvidenceRecord) []ledger.EvidenceRecord {
				rs[0].Body.Seq = 2
				return rs
			},
			wantBreakAt: 2,
			wantReason:  "seq gap",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			records := buildChain(3)
			tampered := tt.tamper(records)

			result := ledger.VerifyChain(tampered)

			if result.OK {
				t.Fatal("VerifyChain() OK = true, want tampering detected")
			}
			if result.BreakAtSeq != tt.wantBreakAt {
				t.Fatalf("BreakAtSeq = %d, want %d", result.BreakAtSeq, tt.wantBreakAt)
			}
			if result.BreakReason != tt.wantReason {
				t.Fatalf("BreakReason = %q, want %q", result.BreakReason, tt.wantReason)
			}
		})
	}
}

// TestVerifyChainExpectedSeqOnFirstRecordTamperedSeq covers the misleading
// BreakAtSeq case: if the first record's stored Seq is tampered (e.g. to 2),
// BreakAtSeq alone reports the tampered value with no indication of what was
// actually expected. ExpectedSeq must report 1 (the genesis seq) regardless
// of what value was found.
func TestVerifyChainExpectedSeqOnFirstRecordTamperedSeq(t *testing.T) {
	records := buildChain(3)
	records[0].Body.Seq = 2

	result := ledger.VerifyChain(records)

	if result.OK {
		t.Fatal("VerifyChain() OK = true, want tampering detected")
	}
	if result.BreakAtSeq != 2 {
		t.Fatalf("BreakAtSeq = %d, want 2 (the tampered value found)", result.BreakAtSeq)
	}
	if result.ExpectedSeq != 1 {
		t.Fatalf("ExpectedSeq = %d, want 1 (the genesis seq that should have been there)", result.ExpectedSeq)
	}
}

// TestVerifyChainExpectedSeqOnMidChainSeqGap covers ExpectedSeq for a
// non-genesis seq gap: it must report the previous record's seq + 1, not the
// tampered/found value.
func TestVerifyChainExpectedSeqOnMidChainSeqGap(t *testing.T) {
	records := buildChain(3)
	tampered := append(records[:1], records[2:]...)

	result := ledger.VerifyChain(tampered)

	if result.OK {
		t.Fatal("VerifyChain() OK = true, want tampering detected")
	}
	if result.BreakAtSeq != 3 {
		t.Fatalf("BreakAtSeq = %d, want 3 (the value found)", result.BreakAtSeq)
	}
	if result.ExpectedSeq != 2 {
		t.Fatalf("ExpectedSeq = %d, want 2 (previous record's seq + 1)", result.ExpectedSeq)
	}
}
