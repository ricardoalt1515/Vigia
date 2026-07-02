package ledger

// VerifyResult reports whether a replayed chain (or package) is intact, and
// if not, the first point at which it broke.
type VerifyResult struct {
	OK          bool
	Count       int
	BreakAtSeq  int64  // seq found at the break point (see ExpectedSeq for the seq that should have been there); 0 when OK
	ExpectedSeq int64  // seq VerifyChain expected at the break point (previous record's seq + 1, or 1 for the first record); 0 when OK
	BreakReason string // "" | "genesis prev_hash" | "seq gap" | "prev_hash linkage" | "hash mismatch" | "inputs_digest mismatch" | "evaluation id mismatch" | "evaluation overall_outcome mismatch" | "evaluation policy_bundle_version mismatch" | "interaction id mismatch" | "interaction tenant_id mismatch" | "invalid prev_hash format"
}

// VerifyChain replays records that MUST already be ordered by Seq ascending.
// Empty and single-record chains are valid. Checks, in order, per record:
//
//	record[0].PrevHash  == GenesisPrevHash
//	record[0].Body.Seq  == 1                       (genesis seq)
//	record[i].Seq       == record[i-1].Seq + 1     (no gap / no fork)
//	record[i].PrevHash  == record[i-1].Hash        (linkage)
//	Hash(record[i].PrevHash, record[i].Body) == record[i].Hash  (integrity)
func VerifyChain(records []EvidenceRecord) VerifyResult {
	if len(records) == 0 {
		return VerifyResult{OK: true, Count: 0}
	}

	if records[0].PrevHash != GenesisPrevHash {
		return VerifyResult{OK: false, Count: len(records), BreakAtSeq: records[0].Body.Seq, ExpectedSeq: 1, BreakReason: "genesis prev_hash"}
	}
	if records[0].Body.Seq != 1 {
		return VerifyResult{OK: false, Count: len(records), BreakAtSeq: records[0].Body.Seq, ExpectedSeq: 1, BreakReason: "seq gap"}
	}
	if Hash(records[0].PrevHash, records[0].Body) != records[0].Hash {
		return VerifyResult{OK: false, Count: len(records), BreakAtSeq: records[0].Body.Seq, ExpectedSeq: 1, BreakReason: "hash mismatch"}
	}

	for i := 1; i < len(records); i++ {
		prev := records[i-1]
		curr := records[i]
		expectedSeq := prev.Body.Seq + 1

		if curr.Body.Seq != expectedSeq {
			return VerifyResult{OK: false, Count: len(records), BreakAtSeq: curr.Body.Seq, ExpectedSeq: expectedSeq, BreakReason: "seq gap"}
		}
		if curr.PrevHash != prev.Hash {
			return VerifyResult{OK: false, Count: len(records), BreakAtSeq: curr.Body.Seq, ExpectedSeq: expectedSeq, BreakReason: "prev_hash linkage"}
		}
		if Hash(curr.PrevHash, curr.Body) != curr.Hash {
			return VerifyResult{OK: false, Count: len(records), BreakAtSeq: curr.Body.Seq, ExpectedSeq: expectedSeq, BreakReason: "hash mismatch"}
		}
	}

	return VerifyResult{OK: true, Count: len(records)}
}
