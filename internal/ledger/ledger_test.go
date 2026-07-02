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

	h1 := ledger.Hash("some-prev-hash", body1)
	h2 := ledger.Hash("some-prev-hash", body2)

	if h1 != h2 {
		t.Fatalf("Hash() not deterministic: %q != %q", h1, h2)
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
