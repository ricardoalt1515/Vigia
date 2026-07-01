package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestRunHandlesInitializeAndToolsList(t *testing.T) {
	t.Setenv("VIGIA_MCP_API_KEY", "local-secret")
	t.Setenv("VIGIA_MCP_TENANT_ID", "SYN-TENANT-001")
	t.Setenv("VIGIA_MCP_KEY_ID", "local-key")

	input := strings.NewReader(strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`,
		"",
	}, "\n"))
	var output bytes.Buffer
	if err := run(context.Background(), input, &output); err != nil {
		t.Fatalf("run: %v", err)
	}
	got := output.String()
	for _, want := range []string{"\"protocolVersion\"", "\"read_case_brief\"", "\"read_evidence_manifest\""} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %s: %s", want, got)
		}
	}
	if strings.Contains(got, "draft_supervisor_note") {
		t.Fatalf("output exposed draft tool: %s", got)
	}
}
