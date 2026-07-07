package ledger

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"
)

func TestMerkleCheckpointUsesRFC6962DomainSeparatedHashes(t *testing.T) {
	records := []EvidenceRecord{
		{Body: Body{Seq: 1}, Hash: repeatHex("11")},
		{Body: Body{Seq: 2}, Hash: repeatHex("22")},
		{Body: Body{Seq: 3}, Hash: repeatHex("33")},
	}

	got, err := BuildMerkleCheckpoint(records, time.Date(2026, 7, 7, 12, 0, 0, 123456000, time.UTC))
	if err != nil {
		t.Fatalf("BuildMerkleCheckpoint: %v", err)
	}

	leaf1 := merkleLeafHash(mustDecodeHex(t, repeatHex("11")))
	leaf2 := merkleLeafHash(mustDecodeHex(t, repeatHex("22")))
	leaf3 := merkleLeafHash(mustDecodeHex(t, repeatHex("33")))
	left := merkleNodeHash(leaf1, leaf2)
	wantRoot := hex.EncodeToString(merkleNodeHash(left, leaf3))

	if got.RootHash != wantRoot {
		t.Fatalf("root = %s, want %s", got.RootHash, wantRoot)
	}
	if got.FirstSeq != 1 || got.LastSeq != 3 || got.RecordCount != 3 || got.ChainHeadHash != repeatHex("33") {
		t.Fatalf("checkpoint metadata = %+v", got)
	}
}

func TestMerkleCheckpointCanonicalBytesAreStable(t *testing.T) {
	createdAt := time.Date(2026, 7, 7, 12, 0, 0, 123456789, time.FixedZone("offset", -6*60*60))
	cp := MerkleCheckpoint{
		TenantID:      "tenant-1",
		FirstSeq:      10,
		LastSeq:       12,
		RecordCount:   3,
		RootHash:      repeatHex("aa"),
		ChainHeadHash: repeatHex("bb"),
		CreatedAt:     createdAt,
	}

	got := string(cp.CanonicalBytes())
	want := `{"tenant_id":"tenant-1","first_seq":10,"last_seq":12,"record_count":3,"root_hash":"` + repeatHex("aa") + `","chain_head_hash":"` + repeatHex("bb") + `","created_at":"2026-07-07T18:00:00.123456Z"}`
	if got != want {
		t.Fatalf("canonical bytes = %s, want %s", got, want)
	}
}

func TestBuildMerkleCheckpointRejectsEmptyAndNonContiguousRecords(t *testing.T) {
	_, err := BuildMerkleCheckpoint(nil, time.Now())
	if err == nil {
		t.Fatal("empty records error = nil")
	}

	records := []EvidenceRecord{
		{Body: Body{Seq: 1}, Hash: repeatHex("11")},
		{Body: Body{Seq: 3}, Hash: repeatHex("33")},
	}
	_, err = BuildMerkleCheckpoint(records, time.Now())
	if err == nil {
		t.Fatal("non-contiguous records error = nil")
	}
}

func TestVerifyTimestampDigestChecksTokenHashBinding(t *testing.T) {
	cp := MerkleCheckpoint{TenantID: "tenant-1", FirstSeq: 1, LastSeq: 1, RecordCount: 1, RootHash: repeatHex("aa"), ChainHeadHash: repeatHex("bb"), CreatedAt: time.Date(2026, 7, 7, 0, 0, 0, 0, time.UTC)}
	sum := sha256.Sum256(cp.CanonicalBytes())

	if err := VerifyTimestampDigest(cp, sum[:]); err != nil {
		t.Fatalf("VerifyTimestampDigest valid: %v", err)
	}
	bad := sha256.Sum256([]byte("different"))
	if err := VerifyTimestampDigest(cp, bad[:]); err == nil {
		t.Fatal("VerifyTimestampDigest mismatch error = nil")
	}
}

func repeatHex(byteHex string) string {
	out := ""
	for i := 0; i < 32; i++ {
		out += byteHex
	}
	return out
}

func mustDecodeHex(t *testing.T, value string) []byte {
	t.Helper()
	out, err := hex.DecodeString(value)
	if err != nil {
		t.Fatalf("DecodeString: %v", err)
	}
	return out
}
