package ledger

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

var (
	ErrNoCheckpointRecords     = errors.New("ledger: merkle checkpoint requires at least one record")
	ErrNoNewCheckpointRecords  = errors.New("ledger: no new evidence records to checkpoint")
	ErrNonContiguousCheckpoint = errors.New("ledger: merkle checkpoint records must be contiguous by seq")
)

// MerkleCheckpoint is the stable, timestamped summary of a contiguous range of
// evidence records. It follows the Certificate Transparency/RFC 6962 pattern:
// the tree is append-only, leaves and internal nodes are domain-separated, and
// the RFC 3161 token is bound to CanonicalBytes(), not to a database row shape.
type MerkleCheckpoint struct {
	TenantID      string
	FirstSeq      int64
	LastSeq       int64
	RecordCount   int64
	RootHash      string
	ChainHeadHash string
	CreatedAt     time.Time
}

type canonicalCheckpointDTO struct {
	TenantID      string `json:"tenant_id"`
	FirstSeq      int64  `json:"first_seq"`
	LastSeq       int64  `json:"last_seq"`
	RecordCount   int64  `json:"record_count"`
	RootHash      string `json:"root_hash"`
	ChainHeadHash string `json:"chain_head_hash"`
	CreatedAt     string `json:"created_at"`
}

// BuildMerkleCheckpoint creates a checkpoint for an already ordered,
// contiguous record range. The Merkle root commits to each record's existing
// hash-chain digest, while ChainHeadHash pins the range to the append-only
// chain head at LastSeq.
func BuildMerkleCheckpoint(records []EvidenceRecord, createdAt time.Time) (MerkleCheckpoint, error) {
	if len(records) == 0 {
		return MerkleCheckpoint{}, ErrNoCheckpointRecords
	}
	for i := 1; i < len(records); i++ {
		if records[i].Body.Seq != records[i-1].Body.Seq+1 {
			return MerkleCheckpoint{}, fmt.Errorf("%w: got seq %d after %d", ErrNonContiguousCheckpoint, records[i].Body.Seq, records[i-1].Body.Seq)
		}
	}

	leaves := make([][]byte, 0, len(records))
	for _, record := range records {
		hashBytes, err := hex.DecodeString(record.Hash)
		if err != nil || len(hashBytes) != sha256.Size {
			return MerkleCheckpoint{}, fmt.Errorf("ledger: record seq %d hash must be a sha256 hex digest", record.Body.Seq)
		}
		leaves = append(leaves, merkleLeafHash(hashBytes))
	}

	first := records[0]
	last := records[len(records)-1]
	return MerkleCheckpoint{
		TenantID:      first.Body.TenantID,
		FirstSeq:      first.Body.Seq,
		LastSeq:       last.Body.Seq,
		RecordCount:   int64(len(records)),
		RootHash:      hex.EncodeToString(merkleRoot(leaves)),
		ChainHeadHash: last.Hash,
		CreatedAt:     createdAt.UTC().Truncate(time.Microsecond),
	}, nil
}

// CanonicalBytes is the exact payload sent to RFC 3161 timestamp authorities.
// It intentionally excludes the returned token and TSA URL so the token binds
// only to the checkpoint fact: tenant, range, Merkle root, chain head, time.
func (c MerkleCheckpoint) CanonicalBytes() []byte {
	dto := canonicalCheckpointDTO{
		TenantID:      c.TenantID,
		FirstSeq:      c.FirstSeq,
		LastSeq:       c.LastSeq,
		RecordCount:   c.RecordCount,
		RootHash:      c.RootHash,
		ChainHeadHash: c.ChainHeadHash,
		CreatedAt:     c.CreatedAt.UTC().Truncate(time.Microsecond).Format(canonicalTimeLayout),
	}
	out, err := json.Marshal(dto)
	if err != nil {
		panic("ledger: MerkleCheckpoint.CanonicalBytes: unexpected marshal error: " + err.Error())
	}
	return out
}

// VerifyTimestampDigest checks that a parsed RFC 3161 token's hashed message is
// bound to this checkpoint's canonical bytes. Full certificate-chain trust is a
// caller concern because deployments choose their TSA roots and policies.
func VerifyTimestampDigest(c MerkleCheckpoint, hashedMessage []byte) error {
	want := sha256.Sum256(c.CanonicalBytes())
	if !bytes.Equal(hashedMessage, want[:]) {
		return errors.New("ledger: timestamp token digest does not match checkpoint canonical bytes")
	}
	return nil
}

func merkleRoot(nodes [][]byte) []byte {
	if len(nodes) == 1 {
		return nodes[0]
	}
	k := largestPowerOfTwoLessThan(len(nodes))
	return merkleNodeHash(merkleRoot(nodes[:k]), merkleRoot(nodes[k:]))
}

func largestPowerOfTwoLessThan(n int) int {
	k := 1
	for k<<1 < n {
		k <<= 1
	}
	return k
}

func merkleLeafHash(hash []byte) []byte {
	sum := sha256.Sum256(append([]byte{0x00}, hash...))
	return sum[:]
}

func merkleNodeHash(left, right []byte) []byte {
	buf := make([]byte, 0, 1+len(left)+len(right))
	buf = append(buf, 0x01)
	buf = append(buf, left...)
	buf = append(buf, right...)
	sum := sha256.Sum256(buf)
	return sum[:]
}
