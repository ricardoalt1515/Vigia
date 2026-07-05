package ledger_test

import (
	"testing"
	"time"

	"github.com/ricardoalt1515/vigia/internal/ledger"
)

// goldenBody is the fixed, hardcoded Body used to pin the canonical hash
// bytes. Any accidental change to field order, field set, or serialization
// format must make TestHashGoldenValue fail.
func goldenBody() ledger.Body {
	return ledger.Body{
		TenantID:            "11111111-1111-1111-1111-111111111111",
		InteractionEventID:  "22222222-2222-2222-2222-222222222222",
		EvaluationID:        "33333333-3333-3333-3333-333333333333",
		Seq:                 1,
		OverallOutcome:      "fail",
		PolicyBundleVersion: "",
		InputsDigest:        "44444444444444444444444444444444444444444444444444444444444444",
		CreatedAt:           time.Date(2026, 6, 15, 23, 0, 0, 123456000, time.UTC),
	}
}

// TestHashGoldenValue pins the exact canonical bytes for a fixed Body +
// genesis prev_hash. This value was computed once via the real
// implementation (Hash) and hardcoded here — it must NEVER be regenerated
// blindly. Any diff means canonicalization drifted.
//
// pinned via 2.3 — any diff means canonicalization drifted, do not silently
// update.
func TestHashGoldenValue(t *testing.T) {
	const wantHash = "4479342d9bbcc290750de7a01f1986d234884e256bcd30965aaa49f05810384d"

	got := ledger.Hash(ledger.GenesisPrevHash, goldenBody())
	if got != wantHash {
		t.Fatalf("Hash() = %q, want golden %q", got, wantHash)
	}
}

func TestHashIsDeterministic(t *testing.T) {
	body1 := goldenBody()
	body2 := goldenBody()
	const prevHash = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

	h1 := ledger.Hash(prevHash, body1)
	h2 := ledger.Hash(prevHash, body2)

	if h1 != h2 {
		t.Fatalf("Hash() not deterministic: %q != %q", h1, h2)
	}
}

// TestHashRejectsInvalidPrevHash pins the defensive validation documented on
// Hash: prevHash must be the empty genesis sentinel or exactly 64 lowercase
// hex characters, otherwise the prevHash||canonicalBody concatenation could
// become ambiguous.
func TestHashRejectsInvalidPrevHash(t *testing.T) {
	tests := []struct {
		name     string
		prevHash string
	}{
		{name: "not hex", prevHash: "not-genesis"},
		{name: "wrong length", prevHash: "abc123"},
		{name: "uppercase hex", prevHash: "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("Hash() did not panic on invalid prevHash")
				}
			}()
			ledger.Hash(tt.prevHash, goldenBody())
		})
	}
}

func TestHashChangesWhenBodyChanges(t *testing.T) {
	base := goldenBody()
	changed := goldenBody()
	changed.OverallOutcome = "pass"

	if ledger.Hash(ledger.GenesisPrevHash, base) == ledger.Hash(ledger.GenesisPrevHash, changed) {
		t.Fatal("Hash() did not change when OverallOutcome changed")
	}
}

func TestGenesisPrevHashIsEmptyString(t *testing.T) {
	if ledger.GenesisPrevHash != "" {
		t.Fatalf("GenesisPrevHash = %q, want empty string", ledger.GenesisPrevHash)
	}
}

// TestHashGoldenValueUnchangedWithJudgeFieldAdded covers *Golden-hash test
// pins the judge-absent body shape unchanged*: re-running the existing #3
// golden-hash test unmodified, after Body gains the trailing Judge
// *JudgeEvidence omitempty field, MUST still produce the identical pinned
// hex. This is a no-diff assertion proving omitempty is inert for
// judge-less bodies.
func TestHashGoldenValueUnchangedWithJudgeFieldAdded(t *testing.T) {
	const wantHash = "4479342d9bbcc290750de7a01f1986d234884e256bcd30965aaa49f05810384d"

	body := goldenBody()
	body.Judge = nil // explicit: this is the pre-#4, judge-absent shape.

	got := ledger.Hash(ledger.GenesisPrevHash, body)
	if got != wantHash {
		t.Fatalf("Hash() = %q, want golden %q (unchanged by the judge-absent Judge field)", got, wantHash)
	}
}

// TestHashGoldenValueUnchangedWithEmptyPolicyBundleVersion covers issue #6's
// load-bearing hazard: when no bundle is active, the stamped version stays
// the empty-string sentinel, so Body serializes byte-identically to today
// and the golden hash MUST remain untouched. This is a no-diff assertion:
// goldenBody() already sets PolicyBundleVersion to "" explicitly, proving
// the sentinel path never regresses the pinned hex.
func TestHashGoldenValueUnchangedWithEmptyPolicyBundleVersion(t *testing.T) {
	const wantHash = "4479342d9bbcc290750de7a01f1986d234884e256bcd30965aaa49f05810384d"

	body := goldenBody()
	body.PolicyBundleVersion = "" // explicit: the issue #6 no-active-bundle sentinel.

	got := ledger.Hash(ledger.GenesisPrevHash, body)
	if got != wantHash {
		t.Fatalf("Hash() = %q, want golden %q (unchanged by the empty-sentinel PolicyBundleVersion)", got, wantHash)
	}
}

// TestHashChangesWithNonEmptyPolicyBundleVersion proves stamping is
// genuinely hashed when present: a real (non-empty) bundle version must
// produce a DIFFERENT hash than the empty-sentinel golden case, or the
// evidence ledger would not actually be binding evaluations to the bundle
// version that judged them.
func TestHashChangesWithNonEmptyPolicyBundleVersion(t *testing.T) {
	emptyVersionHash := ledger.Hash(ledger.GenesisPrevHash, goldenBody())

	stamped := goldenBody()
	stamped.PolicyBundleVersion = "v2"
	stampedHash := ledger.Hash(ledger.GenesisPrevHash, stamped)

	if stampedHash == emptyVersionHash {
		t.Fatal("Hash() did not change when PolicyBundleVersion went from empty to a real stamped version")
	}
}

// goldenJudgeBody is the fixed, hardcoded judge-present Body used to pin the
// judge-present canonical hash bytes.
func goldenJudgeBody() ledger.Body {
	body := goldenBody()
	body.Judge = &ledger.JudgeEvidence{
		RubricVersion: "mx-redeco-05.tone-threat.v1",
		JudgeModelID:  "claude-haiku-4-5-20251001",
		Confidence:    "0.9500",
	}
	return body
}

// TestHashGoldenValueJudgePresent covers *Golden-hash test pins the
// judge-present body shape*: a fixed Body with a fixed JudgeEvidence must
// hash to an exact pinned hex. This value was computed once via the real
// implementation and hardcoded here (task 4.4) — any accidental change to
// the judge sub-object's presence, order, or serialization format must make
// this test fail.
func TestHashGoldenValueJudgePresent(t *testing.T) {
	const wantHash = "970ee863644efec78dc0502dbb8add843af48e4822d9463102a7cbc5a06e0455"

	got := ledger.Hash(ledger.GenesisPrevHash, goldenJudgeBody())
	if len(got) != 64 {
		t.Fatalf("Hash() = %q, want a 64-char hex digest", got)
	}
	if got != wantHash {
		t.Fatalf("Hash() = %q, want golden %q", got, wantHash)
	}
}

// TestChainVerifiesAcrossJudgeShapeChange covers linkage across the shape
// change: a chain with a judge-less record followed by a judged record
// VerifyChains OK.
func TestChainVerifiesAcrossJudgeShapeChange(t *testing.T) {
	first := goldenBody()
	first.Judge = nil
	first.Seq = 1
	firstHash := ledger.Hash(ledger.GenesisPrevHash, first)

	second := goldenJudgeBody()
	second.Seq = 2
	secondHash := ledger.Hash(firstHash, second)

	records := []ledger.EvidenceRecord{
		{ID: "r1", Body: first, PrevHash: ledger.GenesisPrevHash, Hash: firstHash},
		{ID: "r2", Body: second, PrevHash: firstHash, Hash: secondHash},
	}

	result := ledger.VerifyChain(records)
	if !result.OK {
		t.Fatalf("VerifyChain() = %+v, want OK across the judge-less -> judged shape change", result)
	}
	if result.Count != 2 {
		t.Fatalf("VerifyChain().Count = %d, want 2", result.Count)
	}
}

func TestHashGoldenValueUnchangedWithComplaintTransitionFieldAdded(t *testing.T) {
	const wantHash = "4479342d9bbcc290750de7a01f1986d234884e256bcd30965aaa49f05810384d"

	body := goldenBody()
	body.ComplaintTransition = nil

	got := ledger.Hash(ledger.GenesisPrevHash, body)
	if got != wantHash {
		t.Fatalf("Hash() = %q, want golden %q (unchanged by complaint-transition-absent field)", got, wantHash)
	}
}

func TestHashBindsComplaintTransitionFields(t *testing.T) {
	reviewID := "55555555-5555-5555-5555-555555555555"
	base := goldenBody()
	base.InteractionEventID = ""
	base.EvaluationID = ""
	base.OverallOutcome = "resolved"
	base.InputsDigest = ""
	base.ComplaintTransition = &ledger.ComplaintTransitionEvidence{
		ComplaintCaseID: "66666666-6666-6666-6666-666666666666",
		TransitionKind:  "approve",
		FromState:       "awaiting_review",
		ToState:         "resolved",
		HumanReviewID:   &reviewID,
	}
	baseHash := ledger.Hash(ledger.GenesisPrevHash, base)

	tests := []struct {
		name   string
		mutate func(*ledger.Body)
	}{
		{name: "case id", mutate: func(b *ledger.Body) { b.ComplaintTransition.ComplaintCaseID = "77777777-7777-7777-7777-777777777777" }},
		{name: "transition kind", mutate: func(b *ledger.Body) { b.ComplaintTransition.TransitionKind = "override" }},
		{name: "from state", mutate: func(b *ledger.Body) { b.ComplaintTransition.FromState = "open" }},
		{name: "to state", mutate: func(b *ledger.Body) { b.ComplaintTransition.ToState = "escalated" }},
		{name: "human review id", mutate: func(b *ledger.Body) {
			other := "88888888-8888-8888-8888-888888888888"
			b.ComplaintTransition.HumanReviewID = &other
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			changed := base
			transition := *base.ComplaintTransition
			changed.ComplaintTransition = &transition
			tt.mutate(&changed)
			if ledger.Hash(ledger.GenesisPrevHash, changed) == baseHash {
				t.Fatalf("Hash() did not change when complaint transition %s changed", tt.name)
			}
		})
	}
}
