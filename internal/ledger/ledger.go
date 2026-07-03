// Package ledger implements the append-only, hash-chained evidence ledger
// core: canonical hashing, chain verification, and self-contained package
// export/verification. This package is pure — zero I/O, zero database
// access. Persistence lives in internal/postgres.
package ledger

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"time"
)

// GenesisPrevHash is the prev_hash of a tenant's first EvidenceRecord: the
// empty string "". Pinned — the golden-hash test depends on it.
const GenesisPrevHash = ""

// canonicalTimeLayout renders created_at deterministically: always UTC,
// always 6-digit microseconds, always trailing 'Z'. This is the ONLY
// acceptable timestamp format for hashing — encoding/json's default
// RFC3339Nano format has trailing-zero variance that would break
// re-verification across a DB round-trip.
const canonicalTimeLayout = "2006-01-02T15:04:05.000000Z07:00"

// DetectorResult is one detector's contribution to inputs_digest.
type DetectorResult struct {
	Code      string
	Outcome   string // core.DetectorOutcome value, e.g. "pass" | "fail"
	Severity  string
	Rationale string
}

// JudgeEvidence is the LLM-judge sub-object recorded on Body ONLY when a
// judge produced a verdict for that evaluation (issue #4). Confidence is a
// fixed 4-decimal string (e.g. "0.9500"), never a float — a raw float64
// would risk the same numeric-round-trip drift hazard as CreatedAt (see
// canonicalTimeLayout's doc comment), so the already-canonical string is
// what gets hashed and what must be stored/read back verbatim.
type JudgeEvidence struct {
	RubricVersion string `json:"rubric_version"`
	JudgeModelID  string `json:"judge_model_id"`
	Confidence    string `json:"confidence"`
}

// Body is the hashed content of a record. FIELD ORDER IS LOAD-BEARING:
// encoding/json emits struct fields in declaration order, and that order is
// baked into every stored hash. Do not reorder, add, or remove fields
// without a migration + a new golden hash. CreatedAt is serialized by the
// fixed microsecond UTC formatter (see canonicalBody), NOT time.Time's
// default JSON marshaling. Judge is the sole issue-#4 addition: a trailing,
// conditional (omitempty) pointer field so judge-less bodies (the pre-#4
// shape, and any future judge-less evaluation) serialize byte-identically —
// omitempty on a nil pointer omits the key entirely, never emitting
// "judge":null.
type Body struct {
	TenantID            string         `json:"tenant_id"`
	InteractionEventID  string         `json:"interaction_event_id"`
	EvaluationID        string         `json:"evaluation_id"`
	Seq                 int64          `json:"seq"`
	OverallOutcome      string         `json:"overall_outcome"`
	PolicyBundleVersion string         `json:"policy_bundle_version"`
	InputsDigest        string         `json:"inputs_digest"`
	CreatedAt           time.Time      `json:"created_at"`
	Judge               *JudgeEvidence `json:"judge,omitempty"`
}

// EvidenceRecord is a persisted, hashed ledger entry.
type EvidenceRecord struct {
	ID       string
	Body     Body
	PrevHash string
	Hash     string
}

// canonicalBodyDTO mirrors Body but renders CreatedAt through the fixed
// canonicalTimeLayout string instead of time.Time's default marshaling, so
// the hashed bytes never drift with Go's default JSON time format. Judge
// mirrors Body.Judge's omitempty pointer directly: encoding/json omits a
// nil *JudgeEvidence entirely, so judge-less bodies serialize identically
// to their pre-#4 shape.
type canonicalBodyDTO struct {
	TenantID            string         `json:"tenant_id"`
	InteractionEventID  string         `json:"interaction_event_id"`
	EvaluationID        string         `json:"evaluation_id"`
	Seq                 int64          `json:"seq"`
	OverallOutcome      string         `json:"overall_outcome"`
	PolicyBundleVersion string         `json:"policy_bundle_version"`
	InputsDigest        string         `json:"inputs_digest"`
	CreatedAt           string         `json:"created_at"`
	Judge               *JudgeEvidence `json:"judge,omitempty"`
}

// canonicalBody marshals Body deterministically. created_at is rendered by a
// FIXED formatter so DB round-trips re-verify: always UTC, always 6-digit
// microseconds, always trailing 'Z'.
func canonicalBody(b Body) []byte {
	dto := canonicalBodyDTO{
		TenantID:            b.TenantID,
		InteractionEventID:  b.InteractionEventID,
		EvaluationID:        b.EvaluationID,
		Seq:                 b.Seq,
		OverallOutcome:      b.OverallOutcome,
		PolicyBundleVersion: b.PolicyBundleVersion,
		InputsDigest:        b.InputsDigest,
		CreatedAt:           b.CreatedAt.UTC().Format(canonicalTimeLayout),
		Judge:               b.Judge,
	}
	// encoding/json marshals struct fields in declaration order; the error
	// path here is unreachable for this DTO (no channels/funcs/cyclic types).
	out, err := json.Marshal(dto)
	if err != nil {
		panic("ledger: canonicalBody: unexpected marshal error: " + err.Error())
	}
	return out
}

// Hash = hex(sha256(prevHash-ASCII bytes || canonicalBody(body))). Pure.
//
// Concatenating prevHash and canonicalBody(body) directly (with no
// separator or length prefix) is unambiguous ONLY because of two invariants
// enforced below: prevHash is always either "" (GenesisPrevHash) or exactly
// 64 lowercase hex characters, and canonicalBody(body) always starts with
// '{' (a JSON object). Since neither '{' nor any hex digit can be produced
// by the other, no two distinct (prevHash, body) pairs can ever concatenate
// to the same byte string. If prevHash were allowed to be arbitrary bytes,
// this concatenation would become ambiguous (e.g. prevHash="ab" + body="c"
// == prevHash="a" + body="bc"), silently weakening the hash chain.
func Hash(prevHash string, body Body) string {
	if !isValidPrevHash(prevHash) {
		panic("ledger: Hash: prevHash must be \"\" (genesis) or 64 lowercase hex characters, got: " + prevHash)
	}
	sum := sha256.New()
	sum.Write([]byte(prevHash))
	sum.Write(canonicalBody(body))
	return hex.EncodeToString(sum.Sum(nil))
}

// isValidPrevHash reports whether prevHash is a value that preserves the
// concatenation invariant documented on Hash: the empty genesis sentinel, or
// exactly 64 lowercase hex characters (a sha256 hex digest).
func isValidPrevHash(prevHash string) bool {
	if prevHash == GenesisPrevHash {
		return true
	}
	if len(prevHash) != 64 {
		return false
	}
	for _, c := range prevHash {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// canonicalDetectorResult mirrors DetectorResult for deterministic
// marshaling (declaration order is already fixed by the struct itself).
type canonicalDetectorResult struct {
	Code      string `json:"code"`
	Outcome   string `json:"outcome"`
	Severity  string `json:"severity"`
	Rationale string `json:"rationale"`
}

// ComputeInputsDigest = hex(sha256(canonical(sorted results))). Results are
// sorted by Code first (the ordering invariant); each entry contributes
// code+outcome+severity+rationale.
func ComputeInputsDigest(results []DetectorResult) string {
	sorted := make([]DetectorResult, len(results))
	copy(sorted, results)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Code < sorted[j].Code })

	dtos := make([]canonicalDetectorResult, 0, len(sorted))
	for _, r := range sorted {
		dtos = append(dtos, canonicalDetectorResult{
			Code:      r.Code,
			Outcome:   r.Outcome,
			Severity:  r.Severity,
			Rationale: r.Rationale,
		})
	}

	out, err := json.Marshal(dtos)
	if err != nil {
		panic("ledger: ComputeInputsDigest: unexpected marshal error: " + err.Error())
	}
	sum := sha256.Sum256(out)
	return hex.EncodeToString(sum[:])
}
