package judge

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateVerdictAcceptsValidVerdict(t *testing.T) {
	data := []byte(`{"outcome":"block","confidence":0.95,"rationale":"threatening language detected"}`)

	got, err := validateVerdict(data)
	if err != nil {
		t.Fatalf("validateVerdict returned unexpected error: %v", err)
	}
	if got.Outcome != "block" {
		t.Fatalf("Outcome = %q, want block", got.Outcome)
	}
	if got.Confidence != 0.95 {
		t.Fatalf("Confidence = %v, want 0.95", got.Confidence)
	}
	if got.Rationale == "" {
		t.Fatal("Rationale is empty")
	}
}

// TestValidateVerdictRejectsSchemaInvalidOutput covers *Schema-invalid
// output is rejected regardless of apparent verdict*.
func TestValidateVerdictRejectsSchemaInvalidOutput(t *testing.T) {
	tests := []struct {
		name string
		data string
	}{
		{
			name: "unexpected outcome enum value",
			data: `{"outcome":"maybe","confidence":0.9,"rationale":"ambiguous"}`,
		},
		{
			name: "missing required field",
			data: `{"outcome":"pass","confidence":0.9}`,
		},
		{
			name: "additionalProperties rejected",
			data: `{"outcome":"pass","confidence":0.9,"rationale":"fine","extra":"nope"}`,
		},
		{
			name: "confidence above range",
			data: `{"outcome":"pass","confidence":1.5,"rationale":"fine"}`,
		},
		{
			name: "confidence below range",
			data: `{"outcome":"pass","confidence":-0.1,"rationale":"fine"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := validateVerdict([]byte(tt.data))
			if err == nil {
				t.Fatal("validateVerdict returned nil error, want schema validation failure")
			}
			if !errors.Is(err, ErrSchemaInvalid) {
				t.Fatalf("error = %v, want wrapping ErrSchemaInvalid", err)
			}
		})
	}
}

// TestValidateVerdictRejectsSemanticallyInconsistentVerdict covers *Semantic
// sanity check rejects an internally inconsistent verdict*: a schema-valid
// payload whose rationale is empty after trimming whitespace (passes
// minLength:1 syntactically, but is semantically empty) must still be
// rejected.
func TestValidateVerdictRejectsSemanticallyInconsistentVerdict(t *testing.T) {
	data := []byte(`{"outcome":"block","confidence":0.95,"rationale":"   "}`)

	_, err := validateVerdict(data)
	if err == nil {
		t.Fatal("validateVerdict returned nil error, want semantic validation failure")
	}
	if !errors.Is(err, ErrSchemaInvalid) {
		t.Fatalf("error = %v, want wrapping ErrSchemaInvalid", err)
	}
	if !strings.Contains(err.Error(), "rationale") {
		t.Fatalf("error = %v, want it to mention the empty rationale", err)
	}
}

func TestValidateVerdictRejectsMalformedJSON(t *testing.T) {
	_, err := validateVerdict([]byte(`not json`))
	if err == nil {
		t.Fatal("validateVerdict returned nil error, want malformed-output failure")
	}
}
