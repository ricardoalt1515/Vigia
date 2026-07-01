package harness

import "testing"

func TestRiskClassConstants(t *testing.T) {
	cases := []struct {
		name  string
		class RiskClass
		want  RiskClass
	}{
		{"read", RiskClassRead, "read"},
		{"draft", RiskClassDraft, "draft"},
		{"authority", RiskClassAuthority, "authority"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var rc RiskClass = tc.class
			if rc != tc.want {
				t.Fatalf("RiskClass constant %q = %q, want %q", tc.name, rc, tc.want)
			}
		})
	}

	// Verify mutual distinctness
	if RiskClassRead == RiskClassDraft {
		t.Error("RiskClassRead and RiskClassDraft must be distinct")
	}
	if RiskClassRead == RiskClassAuthority {
		t.Error("RiskClassRead and RiskClassAuthority must be distinct")
	}
	if RiskClassDraft == RiskClassAuthority {
		t.Error("RiskClassDraft and RiskClassAuthority must be distinct")
	}
}
