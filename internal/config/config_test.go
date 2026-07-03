package config

import (
	"errors"
	"strings"
	"testing"
)

func TestLoadReturnsValidatedBootstrapConfig(t *testing.T) {
	env := map[string]string{
		"APP_ENV":                 "development",
		"DATABASE_URL":            "postgres://vigia:vigia@localhost:5432/vigia?sslmode=disable",
		"OBJECT_STORE_ENDPOINT":   "http://localhost:9000",
		"OBJECT_STORE_ACCESS_KEY": "vigia",
		"OBJECT_STORE_SECRET_KEY": "vigia-secret",
		"OBJECT_STORE_BUCKET":     "vigia-foundation",
		"OBJECT_STORE_REGION":     "us-east-1",
	}

	cfg, err := Load(FromMap(env))
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}

	if cfg.AppEnv != "development" {
		t.Fatalf("AppEnv = %q, want development", cfg.AppEnv)
	}
	if cfg.DatabaseURL != env["DATABASE_URL"] {
		t.Fatalf("DatabaseURL = %q, want %q", cfg.DatabaseURL, env["DATABASE_URL"])
	}
	if cfg.ObjectStore.Endpoint != env["OBJECT_STORE_ENDPOINT"] {
		t.Fatalf("ObjectStore.Endpoint = %q, want %q", cfg.ObjectStore.Endpoint, env["OBJECT_STORE_ENDPOINT"])
	}
	if cfg.ObjectStore.UseSSL {
		t.Fatal("ObjectStore.UseSSL = true, want false for http endpoint")
	}
}

func TestLoadRejectsMissingRequiredConfigWithUsefulError(t *testing.T) {
	env := map[string]string{
		"APP_ENV":               "development",
		"OBJECT_STORE_ENDPOINT": "http://localhost:9000",
	}

	_, err := Load(FromMap(env))
	if err == nil {
		t.Fatal("Load returned nil error, want missing config error")
	}
	var missing MissingKeysError
	if !errors.As(err, &missing) {
		t.Fatalf("Load error type = %T, want MissingKeysError", err)
	}

	for _, key := range []string{"DATABASE_URL", "OBJECT_STORE_ACCESS_KEY", "OBJECT_STORE_SECRET_KEY", "OBJECT_STORE_BUCKET", "OBJECT_STORE_REGION"} {
		if !strings.Contains(err.Error(), key) {
			t.Fatalf("Load error %q does not name missing key %s", err.Error(), key)
		}
	}
}

func baseValidEnv() map[string]string {
	return map[string]string{
		"APP_ENV":                 "test",
		"DATABASE_URL":            "postgres://vigia:vigia@localhost:5432/vigia?sslmode=disable",
		"OBJECT_STORE_ENDPOINT":   "http://localhost:9000",
		"OBJECT_STORE_ACCESS_KEY": "vigia",
		"OBJECT_STORE_SECRET_KEY": "vigia-secret",
		"OBJECT_STORE_BUCKET":     "vigia-foundation",
		"OBJECT_STORE_REGION":     "us-east-1",
	}
}

func TestLoadFailsFastWhenAnthropicJudgeEnabledWithoutKey(t *testing.T) {
	env := baseValidEnv()
	env["JUDGE_MODE"] = "anthropic"
	// ANTHROPIC_API_KEY intentionally unset.

	_, err := Load(FromMap(env))
	if err == nil {
		t.Fatal("Load returned nil error, want MissingKeysError naming ANTHROPIC_API_KEY")
	}
	var missing MissingKeysError
	if !errors.As(err, &missing) {
		t.Fatalf("Load error type = %T, want MissingKeysError", err)
	}
	if !strings.Contains(err.Error(), "ANTHROPIC_API_KEY") {
		t.Fatalf("Load error %q does not name missing key ANTHROPIC_API_KEY", err.Error())
	}
}

func TestLoadFakeJudgeRequiresNoAPIKey(t *testing.T) {
	env := baseValidEnv()
	// JUDGE_MODE unset -> defaults to "fake"; ANTHROPIC_API_KEY unset.

	cfg, err := Load(FromMap(env))
	if err != nil {
		t.Fatalf("Load returned unexpected error for fake judge mode: %v", err)
	}
	if cfg.JudgeMode != "fake" {
		t.Fatalf("JudgeMode = %q, want default %q", cfg.JudgeMode, "fake")
	}
	if cfg.AnthropicAPIKey != "" {
		t.Fatalf("AnthropicAPIKey = %q, want empty when unset", cfg.AnthropicAPIKey)
	}
}

func TestLoadAnthropicJudgeSucceedsWithKey(t *testing.T) {
	env := baseValidEnv()
	env["JUDGE_MODE"] = "anthropic"
	env["ANTHROPIC_API_KEY"] = "sk-ant-test"

	cfg, err := Load(FromMap(env))
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if cfg.JudgeMode != "anthropic" {
		t.Fatalf("JudgeMode = %q, want anthropic", cfg.JudgeMode)
	}
	if cfg.AnthropicAPIKey != "sk-ant-test" {
		t.Fatalf("AnthropicAPIKey = %q, want sk-ant-test", cfg.AnthropicAPIKey)
	}
}

func TestLoadJudgeModelIDDefaultsToPinnedConstant(t *testing.T) {
	env := baseValidEnv()

	cfg, err := Load(FromMap(env))
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if cfg.JudgeModelID != DefaultJudgeModelID {
		t.Fatalf("JudgeModelID = %q, want default %q", cfg.JudgeModelID, DefaultJudgeModelID)
	}
}

func TestLoadJudgeModelIDOverride(t *testing.T) {
	env := baseValidEnv()
	env["JUDGE_MODEL_ID"] = "claude-haiku-4-5"

	cfg, err := Load(FromMap(env))
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if cfg.JudgeModelID != "claude-haiku-4-5" {
		t.Fatalf("JudgeModelID = %q, want override value", cfg.JudgeModelID)
	}
}

func TestLoadJudgeHITLConfidenceThresholdDefault(t *testing.T) {
	env := baseValidEnv()

	cfg, err := Load(FromMap(env))
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if cfg.JudgeHITLConfidenceThreshold != 0.75 {
		t.Fatalf("JudgeHITLConfidenceThreshold = %v, want default 0.75", cfg.JudgeHITLConfidenceThreshold)
	}
}

func TestLoadJudgeHITLConfidenceThresholdInvalidFailsFast(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{name: "not a number", value: "not-a-number"},
		{name: "below zero", value: "-0.1"},
		{name: "above one", value: "1.1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := baseValidEnv()
			env["JUDGE_HITL_CONFIDENCE_THRESHOLD"] = tt.value

			_, err := Load(FromMap(env))
			if err == nil {
				t.Fatal("Load returned nil error, want validation error for out-of-range threshold")
			}
			var missing MissingKeysError
			if !errors.As(err, &missing) {
				t.Fatalf("Load error type = %T, want MissingKeysError", err)
			}
			if !strings.Contains(err.Error(), "JUDGE_HITL_CONFIDENCE_THRESHOLD") {
				t.Fatalf("Load error %q does not name JUDGE_HITL_CONFIDENCE_THRESHOLD", err.Error())
			}
		})
	}
}

func TestLoadKeepsBedrockAndAWSOptional(t *testing.T) {
	env := map[string]string{
		"APP_ENV":                 "test",
		"DATABASE_URL":            "postgres://vigia:vigia@localhost:5432/vigia?sslmode=disable",
		"OBJECT_STORE_ENDPOINT":   "http://localhost:9000",
		"OBJECT_STORE_ACCESS_KEY": "vigia",
		"OBJECT_STORE_SECRET_KEY": "vigia-secret",
		"OBJECT_STORE_BUCKET":     "vigia-foundation",
		"OBJECT_STORE_REGION":     "us-east-1",
	}

	cfg, err := Load(FromMap(env))
	if err != nil {
		t.Fatalf("Load returned unexpected error without optional provider variables: %v", err)
	}
	if cfg.AWSRegion != "" {
		t.Fatalf("AWSRegion = %q, want empty when AWS_REGION is absent", cfg.AWSRegion)
	}
	if cfg.BedrockModelID != "" {
		t.Fatalf("BedrockModelID = %q, want empty when BEDROCK_MODEL_ID is absent", cfg.BedrockModelID)
	}
}
