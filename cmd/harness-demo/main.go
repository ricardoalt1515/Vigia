// Command harness-demo is a local, cloud-free CLI that drives the #20 caseflow.Orchestrator
// against a deterministic Fake Model Provider, so the Agent Harness Lab is runnable and its
// outputs are inspectable without writing Go test code. See
// openspec/changes/issue-21-demo-cli-case-brief for the full design and spec.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ricardoalt1515/vigia/internal/harness/caseflow"
	"github.com/ricardoalt1515/vigia/internal/harness/labtools"
)

// defaultCasePath is resolved relative to the current working directory, matching
// `go run ./cmd/harness-demo` invoked from the repository root.
const defaultCasePath = "data/synthetic/cases/CASE-SYN-001.json"

// defaultOutDir is where run() writes artifacts when main() invokes it.
const defaultOutDir = "data/synthetic/harness-runs"

// supportedCaseID is the only case id this demo CLI can run; its Fake provider script only
// covers this case.
const supportedCaseID = "CASE-SYN-001"

// testForcedWriteFailureSuffix, when non-empty, causes writeArtifactsAtomically to fail before
// completing the write step for the artifact whose filename has this suffix (e.g. ".brief.md").
// It exists solely as a test seam for main_test.go's zero-partial-files assertion; production
// code never sets it.
var testForcedWriteFailureSuffix string

func main() {
	os.Exit(run(os.Args[1:], defaultOutDir))
}

// caseIDFile is the minimal shape read from --case: only case_id is extracted. The CLI MUST NOT
// use any other field of the --case file as an alternate data source (per spec) — the run itself
// resolves case content from the embedded labtools store.
type caseIDFile struct {
	CaseID string `json:"case_id"`
}

// run is the in-process entry point behind main(), so tests can invoke it without os.Exit.
// Exit codes: 0 = all three artifacts were written; 2 = usage/unsupported-case error (bad flag,
// unreadable/invalid --case file, unsupported case_id), detected before the orchestrator is
// constructed; 1 = infrastructure failure (store load, orchestrator run, render, or file write).
func run(args []string, outDir string) int {
	fs := flag.NewFlagSet("harness-demo", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	casePath := fs.String("case", defaultCasePath, "path to a synthetic Case JSON file")
	provider := fs.String("provider", "fake", `model provider to use: "fake" (default, deterministic, no network) or "bedrock" (opt-in, reads AWS_REGION/BEDROCK_MODEL_ID)`)
	if err := fs.Parse(args); err != nil {
		return 2
	}

	data, err := os.ReadFile(*casePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "harness-demo: cannot read --case file %s: %v\n", *casePath, err)
		return 2
	}

	var cf caseIDFile
	if err := json.Unmarshal(data, &cf); err != nil {
		fmt.Fprintf(os.Stderr, "harness-demo: --case file %s is not valid JSON: %v\n", *casePath, err)
		return 2
	}

	if cf.CaseID != supportedCaseID {
		fmt.Fprintf(os.Stderr, "harness-demo: unsupported synthetic case %q — this demo CLI only runs the scripted synthetic case %q\n", cf.CaseID, supportedCaseID)
		return 2
	}

	// Resolve the Model Provider factory before any orchestrator construction or infrastructure
	// load, so a missing/invalid --provider selection (including missing Bedrock config or
	// credentials) fails fast with no partial side effects.
	providerFactory, err := selectProviderFactory(context.Background(), *provider)
	if err != nil {
		fmt.Fprintf(os.Stderr, "harness-demo: %v\n", err)
		return 2
	}

	caseStore, ruleStore, err := labtools.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "harness-demo: labtools.Load: %v\n", err)
		return 1
	}
	registry := labtools.Registry(caseStore, ruleStore)
	gate := labtools.NewLabPermissionGate()
	sink := newEventSink()

	orch, err := caseflow.NewOrchestrator(
		providerFactory,
		registry,
		gate,
		caseflow.AllAgentDefinitions(),
		caseflow.WithEventObserver(sink.observe),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "harness-demo: NewOrchestrator: %v\n", err)
		return 1
	}

	brief, err := orch.Run(context.Background(), cf.CaseID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "harness-demo: orchestrator run failed unexpectedly: %v\n", err)
		return 1
	}

	// Render all three payloads fully in memory FIRST. A schema-valid INCOMPLETE brief is a
	// legitimate successful run — evidence is still produced.
	jsonlBytes, err := sink.jsonl()
	if err != nil {
		fmt.Fprintf(os.Stderr, "harness-demo: render jsonl: %v\n", err)
		return 1
	}

	dto, err := toBriefDTO(brief)
	if err != nil {
		fmt.Fprintf(os.Stderr, "harness-demo: render brief.json: %v\n", err)
		return 1
	}
	briefJSONBytes, err := json.MarshalIndent(dto, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "harness-demo: marshal brief.json: %v\n", err)
		return 1
	}

	briefMDBytes := []byte(renderBriefMarkdown(dto))

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "harness-demo: create output directory %s: %v\n", outDir, err)
		return 1
	}

	artifacts := []namedArtifact{
		{suffix: ".jsonl", content: jsonlBytes},
		{suffix: ".brief.json", content: briefJSONBytes},
		{suffix: ".brief.md", content: briefMDBytes},
	}
	if err := writeArtifactsAtomically(outDir, brief.CaseID, artifacts); err != nil {
		fmt.Fprintf(os.Stderr, "harness-demo: %v\n", err)
		return 1
	}

	return 0
}

// namedArtifact pairs one rendered artifact's filename suffix with its already-rendered bytes.
type namedArtifact struct {
	suffix  string
	content []byte
}

// writeArtifactsAtomically writes each artifact via os.CreateTemp in dir followed by os.Rename to
// its final <caseID><suffix> path. All artifacts have already been rendered in memory by the
// caller; this function performs only the write step. If any write/rename step fails after some
// files already landed for this run (or testForcedWriteFailureSuffix forces a failure), it
// removes every final path already created for this run before returning the error — leaving
// zero partial output.
func writeArtifactsAtomically(dir, caseID string, artifacts []namedArtifact) error {
	var writtenFinalPaths []string

	cleanup := func() {
		for _, p := range writtenFinalPaths {
			os.Remove(p)
		}
	}

	for _, a := range artifacts {
		filename := caseID + a.suffix

		if testForcedWriteFailureSuffix != "" && a.suffix == testForcedWriteFailureSuffix {
			cleanup()
			return fmt.Errorf("forced write failure for %s (test seam)", filename)
		}

		tmp, err := os.CreateTemp(dir, "."+filename+".tmp-*")
		if err != nil {
			cleanup()
			return fmt.Errorf("create temp file for %s: %w", filename, err)
		}
		if _, err := tmp.Write(a.content); err != nil {
			tmp.Close()
			os.Remove(tmp.Name())
			cleanup()
			return fmt.Errorf("write temp file for %s: %w", filename, err)
		}
		if err := tmp.Close(); err != nil {
			os.Remove(tmp.Name())
			cleanup()
			return fmt.Errorf("close temp file for %s: %w", filename, err)
		}

		finalPath := filepath.Join(dir, filename)
		if err := os.Rename(tmp.Name(), finalPath); err != nil {
			os.Remove(tmp.Name())
			cleanup()
			return fmt.Errorf("rename temp file to %s: %w", filename, err)
		}
		writtenFinalPaths = append(writtenFinalPaths, finalPath)
	}

	return nil
}
