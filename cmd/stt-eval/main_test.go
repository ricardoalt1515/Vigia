package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ricardoalt1515/vigia/internal/judge"
	"github.com/ricardoalt1515/vigia/internal/stteval"
)

func TestRunDefaultsToFakeAndWritesJSONReport(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "manifest.json")
	manifest := stteval.Manifest{Fixtures: []stteval.Fixture{{ID: "mx-1", AudioURI: "fixture://mx-1", Language: "es-MX", Reference: []judge.Utterance{{Speaker: "agent", Text: "hola mundo"}}}}}
	data, _ := json.Marshal(manifest)
	if err := os.WriteFile(manifestPath, data, 0o600); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := run([]string{"-manifest", manifestPath}, &out); err != nil {
		t.Fatalf("run returned error: %v", err)
	}
	if !strings.Contains(out.String(), `"provider": "fake"`) || strings.Contains(out.String(), "AWS") {
		t.Fatalf("output = %s", out.String())
	}
}
