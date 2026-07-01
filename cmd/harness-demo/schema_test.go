package main

import (
	"encoding/json"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

const caseBriefSchemaPath = "schema/case_brief.schema.json"

// compileCaseBriefSchema compiles the committed schema, failing the test with a clear message on
// any compile error.
func compileCaseBriefSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()
	c := jsonschema.NewCompiler()
	sch, err := c.Compile(caseBriefSchemaPath)
	if err != nil {
		t.Fatalf("compile %s: %v", caseBriefSchemaPath, err)
	}
	return sch
}

// validateDTO marshals dto to JSON, decodes it into a generic interface{}, and validates it
// against sch.
func validateDTO(t *testing.T, sch *jsonschema.Schema, dto briefDTO) error {
	t.Helper()
	b, err := json.Marshal(dto)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var inst any
	if err := json.Unmarshal(b, &inst); err != nil {
		t.Fatalf("json.Unmarshal into interface{}: %v", err)
	}
	return sch.Validate(inst)
}

func TestCaseBriefSchema_CompilesSuccessfully(t *testing.T) {
	compileCaseBriefSchema(t)
}

func TestCaseBriefSchema_ValidatesCompleteBrief(t *testing.T) {
	sch := compileCaseBriefSchema(t)
	dto, err := toBriefDTO(completeCaseBrief())
	if err != nil {
		t.Fatalf("toBriefDTO: %v", err)
	}
	if err := validateDTO(t, sch, dto); err != nil {
		t.Errorf("expected complete brief to validate, got error: %v", err)
	}
}

func TestCaseBriefSchema_ValidatesIncompleteBrief(t *testing.T) {
	sch := compileCaseBriefSchema(t)
	dto, err := toBriefDTO(incompleteCaseBrief())
	if err != nil {
		t.Fatalf("toBriefDTO: %v", err)
	}
	if err := validateDTO(t, sch, dto); err != nil {
		t.Errorf("expected incomplete brief to validate, got error: %v", err)
	}
}

func TestCaseBriefSchema_RejectsInvalidStatus(t *testing.T) {
	sch := compileCaseBriefSchema(t)

	raw := `{
		"case_id": "CASE-SYN-001",
		"status": "not-a-real-status",
		"stages": []
	}`
	var inst any
	if err := json.Unmarshal([]byte(raw), &inst); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if err := sch.Validate(inst); err == nil {
		t.Error("expected validation error for out-of-enum status, got nil")
	}
}
