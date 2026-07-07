package stteval

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ricardoalt1515/vigia/internal/judge"
	"github.com/ricardoalt1515/vigia/internal/transcriber"
)

func TestWERAndCERKnownExamples(t *testing.T) {
	metrics := CompareText("hola mundo", "hola cruel mundo")
	if metrics.WER != 0.5 {
		t.Fatalf("WER = %v, want 0.5", metrics.WER)
	}
	if metrics.CER <= 0 {
		t.Fatalf("CER = %v, want non-zero", metrics.CER)
	}
}

func TestLoadManifestDeterministically(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.json")
	payload := Manifest{Fixtures: []Fixture{{ID: "mx-1", AudioURI: "fixture://mx-1", Language: "es-MX", Reference: []judge.Utterance{{Speaker: "agent", Text: "hola"}}}}}
	data, _ := json.Marshal(payload)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	manifest, err := LoadManifest(path)
	if err != nil {
		t.Fatalf("LoadManifest returned error: %v", err)
	}
	if len(manifest.Fixtures) != 1 || manifest.Fixtures[0].ID != "mx-1" {
		t.Fatalf("manifest = %#v", manifest)
	}
}

func TestHarnessUsesFakeWithoutCredentials(t *testing.T) {
	fixture := Fixture{ID: "mx-1", AudioURI: "fixture://mx-1", Language: "es-MX", Reference: []judge.Utterance{{Speaker: "agent", Text: "hola mundo"}}}
	fake := transcriber.NewFakeTranscriber(map[string]transcriber.Result{"fixture://mx-1": {Utterances: fixture.Reference}}, transcriber.Result{})

	report, err := Run(context.Background(), []Fixture{fixture}, []NamedTranscriber{{Name: "fake", Transcriber: fake}})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if len(report.Results) != 1 {
		t.Fatalf("results = %d, want 1", len(report.Results))
	}
	if report.Results[0].Provider != "fake" || report.Results[0].WER != 0 || report.Results[0].CER != 0 {
		t.Fatalf("result = %#v", report.Results[0])
	}
}
