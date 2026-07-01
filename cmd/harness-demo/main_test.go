package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ricardoalt1515/vigia/internal/harness/bedrock"
	"github.com/ricardoalt1515/vigia/internal/harness/caseflow"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

// clearAWSCredentialEnvForTest scopes the process environment so the AWS SDK default credential
// chain cannot resolve credentials from env vars, EC2 IMDS, ECS container credentials, or web
// identity tokens — and cannot reach the network doing so. Mirrors
// internal/harness/bedrock/factory_test.go's clearCredentialEnv (unexported there, so duplicated
// here at the CLI test boundary).
func clearAWSCredentialEnvForTest(t *testing.T) {
	t.Helper()
	t.Setenv("AWS_ACCESS_KEY_ID", "")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "")
	t.Setenv("AWS_SESSION_TOKEN", "")
	t.Setenv("AWS_PROFILE", "")
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", "/nonexistent/aws-credentials-file-for-tests")
	t.Setenv("AWS_CONFIG_FILE", "/nonexistent/aws-config-file-for-tests")
	t.Setenv("AWS_CONTAINER_CREDENTIALS_RELATIVE_URI", "")
	t.Setenv("AWS_CONTAINER_CREDENTIALS_FULL_URI", "")
	t.Setenv("AWS_WEB_IDENTITY_TOKEN_FILE", "")
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
}

// --- Portfolio case file (Slice 2) ------------------------------------------------------------

func TestPortfolioCaseFile_ExistsAndMatchesEmbeddedFixture(t *testing.T) {
	path := filepath.Join("..", "..", "data", "synthetic", "cases", "CASE-SYN-001.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	var decoded struct {
		CaseID string `json:"case_id"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("%s is not valid JSON: %v", path, err)
	}
	if decoded.CaseID != "CASE-SYN-001" {
		t.Errorf("case_id: want %q, got %q", "CASE-SYN-001", decoded.CaseID)
	}
}

// --- CLI wiring / exit-code / atomicity e2e tests ---------------------------------------------

func listFiles(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatalf("ReadDir %s: %v", dir, err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

// chdirToRepoRoot changes the test process's working directory to the repository root so that
// run()'s default --case path (relative to the repo root, matching `go run ./cmd/harness-demo`
// usage) resolves correctly under `go test`. t.Chdir restores the original directory on cleanup.
func chdirToRepoRoot(t *testing.T) {
	t.Helper()
	t.Chdir(filepath.Join("..", ".."))
}

func TestRun_DefaultCase_ExitsZeroAndWritesThreeFiles(t *testing.T) {
	chdirToRepoRoot(t)
	dir := t.TempDir()
	code := run(nil, dir)
	if code != 0 {
		t.Fatalf("run: want exit code 0, got %d", code)
	}

	files := listFiles(t, dir)
	want := map[string]bool{
		"CASE-SYN-001.jsonl":      true,
		"CASE-SYN-001.brief.json": true,
		"CASE-SYN-001.brief.md":   true,
	}
	if len(files) != 3 {
		t.Fatalf("expected exactly 3 files, got %d: %v", len(files), files)
	}
	for _, f := range files {
		if !want[f] {
			t.Errorf("unexpected file %q written", f)
		}
	}
}

func TestRun_ExplicitCasePath_BehavesLikeDefault(t *testing.T) {
	casePath := filepath.Join("..", "..", "data", "synthetic", "cases", "CASE-SYN-001.json")
	dir := t.TempDir()
	code := run([]string{"--case", casePath}, dir)
	if code != 0 {
		t.Fatalf("run: want exit code 0, got %d", code)
	}
	files := listFiles(t, dir)
	if len(files) != 3 {
		t.Fatalf("expected exactly 3 files, got %d: %v", len(files), files)
	}
}

func TestRun_UnsupportedCaseID_ExitsNonZeroAndWritesNothing(t *testing.T) {
	dir := t.TempDir()

	unsupported := filepath.Join(t.TempDir(), "other-case.json")
	if err := os.WriteFile(unsupported, []byte(`{"case_id":"CASE-SYN-999"}`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	code := run([]string{"--case", unsupported}, dir)
	if code == 0 {
		t.Fatalf("run: want non-zero exit code for unsupported case id, got 0")
	}
	files := listFiles(t, dir)
	if len(files) != 0 {
		t.Errorf("expected no files written for an unsupported case id, got: %v", files)
	}
}

func TestRun_MalformedCaseFile_ExitsNonZeroAndWritesNothing(t *testing.T) {
	dir := t.TempDir()

	malformed := filepath.Join(t.TempDir(), "malformed.json")
	if err := os.WriteFile(malformed, []byte(`{not valid json`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	code := run([]string{"--case", malformed}, dir)
	if code == 0 {
		t.Fatalf("run: want non-zero exit code for a malformed case file, got 0")
	}
	files := listFiles(t, dir)
	if len(files) != 0 {
		t.Errorf("expected no files written for a malformed case file, got: %v", files)
	}
}

func TestRun_ForcedWriteFailure_LeavesZeroPartialFiles(t *testing.T) {
	chdirToRepoRoot(t)
	dir := t.TempDir()

	// Force a failure while writing the last of the three artifacts (after the first two would
	// have already landed under a naive sequential-write implementation), proving the
	// render-all-first-then-temp-write-then-rename-with-cleanup mechanism leaves no partial
	// output.
	testForcedWriteFailureSuffix = ".brief.md"
	defer func() { testForcedWriteFailureSuffix = "" }()

	code := run(nil, dir)
	if code == 0 {
		t.Fatalf("run: want non-zero exit code when a write step is forced to fail, got 0")
	}
	files := listFiles(t, dir)
	if len(files) != 0 {
		t.Errorf("expected zero files after a forced write failure, got: %v", files)
	}
}

func TestRun_Determinism_TwoRunsProduceIdenticalOutput(t *testing.T) {
	chdirToRepoRoot(t)
	dir1 := t.TempDir()
	dir2 := t.TempDir()

	if code := run(nil, dir1); code != 0 {
		t.Fatalf("run (1): want exit code 0, got %d", code)
	}
	if code := run(nil, dir2); code != 0 {
		t.Fatalf("run (2): want exit code 0, got %d", code)
	}

	for _, name := range []string{"CASE-SYN-001.jsonl", "CASE-SYN-001.brief.json"} {
		b1, err := os.ReadFile(filepath.Join(dir1, name))
		if err != nil {
			t.Fatalf("ReadFile %s (run 1): %v", name, err)
		}
		b2, err := os.ReadFile(filepath.Join(dir2, name))
		if err != nil {
			t.Fatalf("ReadFile %s (run 2): %v", name, err)
		}
		if !bytes.Equal(b1, b2) {
			t.Errorf("%s differs between two consecutive runs:\nrun1: %s\nrun2: %s", name, b1, b2)
		}
	}
}

func TestRun_FixedAgentOrder(t *testing.T) {
	chdirToRepoRoot(t)
	dir := t.TempDir()
	if code := run(nil, dir); code != 0 {
		t.Fatalf("run: want exit code 0, got %d", code)
	}

	b, err := os.ReadFile(filepath.Join(dir, "CASE-SYN-001.brief.json"))
	if err != nil {
		t.Fatalf("ReadFile brief.json: %v", err)
	}
	var dto briefDTO
	if err := json.Unmarshal(b, &dto); err != nil {
		t.Fatalf("Unmarshal brief.json: %v", err)
	}

	wantOrder := []string{"PolicyExplainer", "CaseInvestigator", "EvidencePackager", "SupervisorNoteDrafter"}
	if len(dto.Stages) != len(wantOrder) {
		t.Fatalf("expected %d stages, got %d", len(wantOrder), len(dto.Stages))
	}
	for i, want := range wantOrder {
		if dto.Stages[i].AgentName != want {
			t.Errorf("Stages[%d].AgentName: want %q, got %q", i, want, dto.Stages[i].AgentName)
		}
	}
}

func TestRun_BriefJSON_ValidatesAgainstSchema(t *testing.T) {
	// Resolve the schema path relative to this package's directory before chdir'ing to the repo
	// root, so schema compilation is unaffected by run()'s default-case cwd requirement.
	schemaAbsPath, err := filepath.Abs(caseBriefSchemaPath)
	if err != nil {
		t.Fatalf("filepath.Abs: %v", err)
	}

	chdirToRepoRoot(t)
	dir := t.TempDir()
	if code := run(nil, dir); code != 0 {
		t.Fatalf("run: want exit code 0, got %d", code)
	}

	b, err := os.ReadFile(filepath.Join(dir, "CASE-SYN-001.brief.json"))
	if err != nil {
		t.Fatalf("ReadFile brief.json: %v", err)
	}

	c := jsonschema.NewCompiler()
	sch, err := c.Compile(schemaAbsPath)
	if err != nil {
		t.Fatalf("compile schema: %v", err)
	}

	var inst any
	if err := json.Unmarshal(b, &inst); err != nil {
		t.Fatalf("Unmarshal brief.json: %v", err)
	}
	if err := sch.Validate(inst); err != nil {
		t.Errorf("brief.json produced by a real run failed schema validation: %v", err)
	}
}

func TestRun_BriefMD_IsSpanishWithDisclaimer(t *testing.T) {
	chdirToRepoRoot(t)
	dir := t.TempDir()
	if code := run(nil, dir); code != 0 {
		t.Fatalf("run: want exit code 0, got %d", code)
	}

	b, err := os.ReadFile(filepath.Join(dir, "CASE-SYN-001.brief.md"))
	if err != nil {
		t.Fatalf("ReadFile brief.md: %v", err)
	}
	content := string(b)

	if !bytes.Contains(b, []byte("Resumen del caso")) {
		t.Errorf("brief.md missing Spanish section header, got:\n%s", content)
	}
	if !bytes.Contains(b, []byte("BORRADOR")) {
		t.Errorf("brief.md missing disclaimer, got:\n%s", content)
	}
}

func TestRun_JSONL_MonotonicSequenceAndKnownEventTypesOnly(t *testing.T) {
	chdirToRepoRoot(t)
	dir := t.TempDir()
	if code := run(nil, dir); code != 0 {
		t.Fatalf("run: want exit code 0, got %d", code)
	}

	b, err := os.ReadFile(filepath.Join(dir, "CASE-SYN-001.jsonl"))
	if err != nil {
		t.Fatalf("ReadFile jsonl: %v", err)
	}

	scanner := bufio.NewScanner(bytes.NewReader(b))
	lastSeq := -1.0
	lineCount := 0
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		lineCount++
		var decoded map[string]any
		if err := json.Unmarshal(line, &decoded); err != nil {
			t.Fatalf("line %d not valid JSON: %v", lineCount, err)
		}
		typ, _ := decoded["type"].(string)
		if !knownEventTypes[typ] {
			t.Errorf("line %d: unknown event type %v", lineCount, decoded["type"])
		}
		seq, ok := decoded["sequence"].(float64)
		if !ok || seq <= lastSeq {
			t.Errorf("line %d: sequence %v not monotonically increasing after %v", lineCount, decoded["sequence"], lastSeq)
		}
		lastSeq = seq
	}
	if lineCount == 0 {
		t.Fatal("expected at least one JSONL line from a real run")
	}
}

// --- --provider flag (Slice 2 / #22) --------------------------------------------------------

func TestRun_ProviderBedrock_MissingConfig_ExitsTwoWithNoOrchestratorOrOutput(t *testing.T) {
	tests := []struct {
		name       string
		region     string
		modelID    string
		clearCreds bool
	}{
		{name: "missing AWS_REGION", region: "", modelID: "anthropic.claude-3-sonnet"},
		{name: "missing BEDROCK_MODEL_ID", region: "us-east-1", modelID: ""},
		{name: "missing resolvable credentials", region: "us-east-1", modelID: "anthropic.claude-3-sonnet", clearCreds: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			chdirToRepoRoot(t)
			dir := t.TempDir()
			t.Setenv("AWS_REGION", tc.region)
			t.Setenv("BEDROCK_MODEL_ID", tc.modelID)
			if tc.clearCreds {
				clearAWSCredentialEnvForTest(t)
			}

			code := run([]string{"--provider", "bedrock"}, dir)
			if code != 2 {
				t.Fatalf("run: want exit code 2, got %d", code)
			}
			files := listFiles(t, dir)
			if len(files) != 0 {
				t.Errorf("expected zero files written, got: %v", files)
			}
		})
	}
}

func TestRun_UnknownProviderValue_ExitsTwoWithNoOutput(t *testing.T) {
	dir := t.TempDir()
	code := run([]string{"--provider", "something-else"}, dir)
	if code != 2 {
		t.Fatalf("run: want exit code 2 for unknown --provider value, got %d", code)
	}
	files := listFiles(t, dir)
	if len(files) != 0 {
		t.Errorf("expected zero files written for unknown --provider value, got: %v", files)
	}
}

func TestRun_ProviderFakeAndDefault_ProduceIdenticalOutput(t *testing.T) {
	chdirToRepoRoot(t)
	dirDefault := t.TempDir()
	dirFake := t.TempDir()

	if code := run(nil, dirDefault); code != 0 {
		t.Fatalf("run(nil): want exit code 0, got %d", code)
	}
	if code := run([]string{"--provider", "fake"}, dirFake); code != 0 {
		t.Fatalf("run(--provider fake): want exit code 0, got %d", code)
	}

	for _, name := range []string{"CASE-SYN-001.jsonl", "CASE-SYN-001.brief.json", "CASE-SYN-001.brief.md"} {
		b1, err := os.ReadFile(filepath.Join(dirDefault, name))
		if err != nil {
			t.Fatalf("ReadFile %s (default): %v", name, err)
		}
		b2, err := os.ReadFile(filepath.Join(dirFake, name))
		if err != nil {
			t.Fatalf("ReadFile %s (--provider fake): %v", name, err)
		}
		if !bytes.Equal(b1, b2) {
			t.Errorf("%s differs between default and --provider fake runs", name)
		}
	}
}

func TestRun_ProviderBedrock_ValidConfig_ExitsZeroAndWritesSameArtifactsAsFake(t *testing.T) {
	chdirToRepoRoot(t)
	dir := t.TempDir()
	t.Setenv("AWS_REGION", "us-east-1")
	t.Setenv("BEDROCK_MODEL_ID", "anthropic.claude-3-sonnet")

	restore := newBedrockFactory
	defer func() { newBedrockFactory = restore }()
	// No live AWS/network call: the fake seam below stands in for bedrock.NewFactory and returns
	// the same scripted demoProviderFactory the "fake" path uses.
	newBedrockFactory = func(_ context.Context, _ bedrock.Options, _ ...bedrock.Option) (caseflow.ProviderFactory, error) {
		return demoProviderFactory, nil
	}

	code := run([]string{"--provider", "bedrock"}, dir)
	if code != 0 {
		t.Fatalf("run: want exit code 0, got %d", code)
	}

	files := listFiles(t, dir)
	want := map[string]bool{
		"CASE-SYN-001.jsonl":      true,
		"CASE-SYN-001.brief.json": true,
		"CASE-SYN-001.brief.md":   true,
	}
	if len(files) != 3 {
		t.Fatalf("expected exactly 3 files, got %d: %v", len(files), files)
	}
	for _, f := range files {
		if !want[f] {
			t.Errorf("unexpected file %q written", f)
		}
	}

	b, err := os.ReadFile(filepath.Join(dir, "CASE-SYN-001.brief.json"))
	if err != nil {
		t.Fatalf("ReadFile brief.json: %v", err)
	}
	var dto briefDTO
	if err := json.Unmarshal(b, &dto); err != nil {
		t.Fatalf("Unmarshal brief.json: %v", err)
	}
	wantOrder := []string{"PolicyExplainer", "CaseInvestigator", "EvidencePackager", "SupervisorNoteDrafter"}
	if len(dto.Stages) != len(wantOrder) {
		t.Fatalf("expected %d stages, got %d", len(wantOrder), len(dto.Stages))
	}
	for i, want := range wantOrder {
		if dto.Stages[i].AgentName != want {
			t.Errorf("Stages[%d].AgentName: want %q, got %q", i, want, dto.Stages[i].AgentName)
		}
	}
}
