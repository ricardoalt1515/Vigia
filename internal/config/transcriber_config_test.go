package config

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestLoadTranscriberDefaultsAreDeterministicAndAdditive(t *testing.T) {
	env := baseValidEnv()

	cfg, err := Load(FromMap(env))
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}

	if cfg.Transcriber.Mode != "fake" {
		t.Fatalf("Transcriber.Mode = %q, want fake", cfg.Transcriber.Mode)
	}
	if cfg.Transcriber.LanguageCode != "es-MX" {
		t.Fatalf("Transcriber.LanguageCode = %q, want es-MX", cfg.Transcriber.LanguageCode)
	}
	if cfg.Transcriber.AWSTimeout != 10*time.Minute {
		t.Fatalf("Transcriber.AWSTimeout = %v, want 10m", cfg.Transcriber.AWSTimeout)
	}
	if cfg.JudgeMode != "fake" || cfg.AWSRegion != "" {
		t.Fatalf("unrelated config changed: JudgeMode=%q AWSRegion=%q", cfg.JudgeMode, cfg.AWSRegion)
	}
}

func TestLoadTranscriberAWSRegionFallsBackToAWSRegion(t *testing.T) {
	env := baseValidEnv()
	env["TRANSCRIBER_MODE"] = "aws-transcribe"
	env["AWS_REGION"] = "us-west-2"
	env["TRANSCRIBER_AWS_OUTPUT_BUCKET"] = "transcripts"

	cfg, err := Load(FromMap(env))
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if cfg.Transcriber.AWSRegion != "us-west-2" {
		t.Fatalf("Transcriber.AWSRegion = %q, want fallback us-west-2", cfg.Transcriber.AWSRegion)
	}
}

func TestLoadRejectsUnknownTranscriberMode(t *testing.T) {
	env := baseValidEnv()
	env["TRANSCRIBER_MODE"] = "whisper"

	_, err := Load(FromMap(env))
	if err == nil {
		t.Fatal("Load returned nil error, want TRANSCRIBER_MODE validation error")
	}
	var missing MissingKeysError
	if !errors.As(err, &missing) || !strings.Contains(err.Error(), "TRANSCRIBER_MODE") {
		t.Fatalf("Load error = %v, want MissingKeysError naming TRANSCRIBER_MODE", err)
	}
}

func TestLoadRejectsInvalidTranscriberDuration(t *testing.T) {
	env := baseValidEnv()
	env["TRANSCRIBER_AWS_TIMEOUT"] = "nope"

	_, err := Load(FromMap(env))
	if err == nil || !strings.Contains(err.Error(), "TRANSCRIBER_AWS_TIMEOUT") {
		t.Fatalf("Load error = %v, want TRANSCRIBER_AWS_TIMEOUT validation error", err)
	}
}
