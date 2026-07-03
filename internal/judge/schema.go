package judge

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

//go:embed schema/verdict.v1.json
var verdictSchemaJSON []byte

// verdictSchemaURL is a synthetic identifier for the embedded schema
// resource; it is never fetched over the network (jsonschema.Compiler
// resolves it purely from the in-memory resource added below).
const verdictSchemaURL = "vigia://judge/verdict.v1.json"

// verdictSchema is compiled once at package init from the embedded
// verdict.v1.json artifact. This is the same JSON document used as both the
// Anthropic tool's input_schema and the re-validation schema — one artifact,
// owned by the app (never trusting the model's own claims).
var verdictSchema = compileVerdictSchema()

func compileVerdictSchema() *jsonschema.Schema {
	var doc any
	if err := json.Unmarshal(verdictSchemaJSON, &doc); err != nil {
		panic("judge: embedded verdict schema is not valid JSON: " + err.Error())
	}

	c := jsonschema.NewCompiler()
	if err := c.AddResource(verdictSchemaURL, doc); err != nil {
		panic("judge: failed to add embedded verdict schema resource: " + err.Error())
	}
	schema, err := c.Compile(verdictSchemaURL)
	if err != nil {
		panic("judge: failed to compile embedded verdict schema: " + err.Error())
	}
	return schema
}

// VerdictInputSchemaMap decodes the embedded verdict.v1.json artifact into a
// generic map for use as the Anthropic tool's input_schema. Returns a fresh
// decode each call so callers cannot mutate the shared compiled schema.
func VerdictInputSchemaMap() map[string]any {
	var doc map[string]any
	if err := json.Unmarshal(verdictSchemaJSON, &doc); err != nil {
		panic("judge: embedded verdict schema is not valid JSON: " + err.Error())
	}
	return doc
}

// rawVerdict is the schema-validated, semantically-checked verdict decoded
// from the model's tool input.
type rawVerdict struct {
	Outcome    string  `json:"outcome"`
	Confidence float64 `json:"confidence"`
	Rationale  string  `json:"rationale"`
}

// validateVerdict parses data, validates it against the embedded
// verdict.v1.json schema, then runs a semantic sanity pass (non-schema
// checks the JSON Schema syntax cannot express, e.g. a rationale that is
// only whitespace). The model's own claims are never trusted: both layers
// must pass before a verdict is usable.
func validateVerdict(data []byte) (rawVerdict, error) {
	var inst any
	if err := json.Unmarshal(data, &inst); err != nil {
		return rawVerdict{}, fmt.Errorf("%w: invalid JSON: %v", ErrMalformedOutput, err)
	}

	if err := verdictSchema.Validate(inst); err != nil {
		return rawVerdict{}, fmt.Errorf("%w: %v", ErrSchemaInvalid, err)
	}

	var v rawVerdict
	if err := json.Unmarshal(data, &v); err != nil {
		return rawVerdict{}, fmt.Errorf("%w: %v", ErrMalformedOutput, err)
	}

	if err := semanticCheck(v); err != nil {
		return rawVerdict{}, err
	}

	return v, nil
}

// semanticCheck re-verifies fields the JSON Schema layer cannot fully
// express: a rationale that is only whitespace (schema's minLength counts
// characters, not trimmed content), and defense-in-depth re-checks of the
// outcome enum and confidence bounds.
func semanticCheck(v rawVerdict) error {
	if strings.TrimSpace(v.Rationale) == "" {
		return fmt.Errorf("%w: rationale is empty after trimming whitespace", ErrSchemaInvalid)
	}
	if v.Outcome != string(OutcomePass) && v.Outcome != string(OutcomeBlock) {
		return fmt.Errorf("%w: outcome %q is not one of pass|block", ErrSchemaInvalid, v.Outcome)
	}
	if v.Confidence < 0 || v.Confidence > 1 {
		return fmt.Errorf("%w: confidence %v is outside [0,1]", ErrSchemaInvalid, v.Confidence)
	}
	return nil
}
