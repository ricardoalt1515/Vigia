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
