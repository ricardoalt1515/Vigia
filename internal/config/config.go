package config

import (
	"fmt"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
)

type LookupFunc func(key string) (string, bool)

// DefaultJudgeModelID is the pinned Haiku-class snapshot used by the
// Anthropic judge when JUDGE_MODEL_ID is unset. Verified against
// anthropic-sdk-go's anthropic.ModelClaudeHaiku4_5_20251001 constant at
// apply time (issue #4 design decision 7 / flagged decision).
const DefaultJudgeModelID = "claude-haiku-4-5-20251001"

// defaultJudgeHITLConfidenceThreshold is the conservative default HITL
// routing threshold (issue #4 design): a judge verdict with confidence
// below this value fails closed to requires_hitl.
const defaultJudgeHITLConfidenceThreshold = 0.75

type Config struct {
	AppEnv         string
	DatabaseURL    string
	ObjectStore    ObjectStoreConfig
	AWSRegion      string
	BedrockModelID string

	// AnthropicAPIKey is required only when JudgeMode == "anthropic".
	AnthropicAPIKey string
	// JudgeMode selects the judge implementation: "fake" (default, no key
	// required) or "anthropic".
	JudgeMode string
	// JudgeModelID is the pinned model id used by the Anthropic judge.
	JudgeModelID string
	// JudgeHITLConfidenceThreshold routes a judge verdict to requires_hitl
	// when its confidence is below this value, in [0,1].
	JudgeHITLConfidenceThreshold float64
}

type ObjectStoreConfig struct {
	Endpoint  string
	AccessKey string
	SecretKey string
	Bucket    string
	Region    string
	UseSSL    bool
}

type MissingKeysError struct {
	Keys []string
}

func (e MissingKeysError) Error() string {
	keys := append([]string(nil), e.Keys...)
	sort.Strings(keys)
	return fmt.Sprintf("missing or invalid required configuration: %s", strings.Join(keys, ", "))
}

func LoadFromEnv() (Config, error) {
	return Load(os.LookupEnv)
}

func Load(lookup LookupFunc) (Config, error) {
	if lookup == nil {
		lookup = os.LookupEnv
	}

	cfg := Config{
		AppEnv:      required(lookup, "APP_ENV"),
		DatabaseURL: required(lookup, "DATABASE_URL"),
		ObjectStore: ObjectStoreConfig{
			Endpoint:  required(lookup, "OBJECT_STORE_ENDPOINT"),
			AccessKey: required(lookup, "OBJECT_STORE_ACCESS_KEY"),
			SecretKey: required(lookup, "OBJECT_STORE_SECRET_KEY"),
			Bucket:    required(lookup, "OBJECT_STORE_BUCKET"),
			Region:    required(lookup, "OBJECT_STORE_REGION"),
		},
		AWSRegion:      optional(lookup, "AWS_REGION"),
		BedrockModelID: optional(lookup, "BEDROCK_MODEL_ID"),

		AnthropicAPIKey: optional(lookup, "ANTHROPIC_API_KEY"),
		JudgeMode:       optional(lookup, "JUDGE_MODE"),
		JudgeModelID:    optional(lookup, "JUDGE_MODEL_ID"),
	}

	cfg.ObjectStore.UseSSL = strings.HasPrefix(strings.ToLower(cfg.ObjectStore.Endpoint), "https://")

	if cfg.JudgeMode == "" {
		cfg.JudgeMode = "fake"
	}
	if cfg.JudgeModelID == "" {
		cfg.JudgeModelID = DefaultJudgeModelID
	}

	thresholdMissing := false
	if raw, ok := lookup("JUDGE_HITL_CONFIDENCE_THRESHOLD"); ok && strings.TrimSpace(raw) != "" {
		threshold, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
		if err != nil || threshold < 0 || threshold > 1 {
			thresholdMissing = true
		} else {
			cfg.JudgeHITLConfidenceThreshold = threshold
		}
	} else {
		cfg.JudgeHITLConfidenceThreshold = defaultJudgeHITLConfidenceThreshold
	}

	missing := validate(cfg)
	if cfg.JudgeMode == "anthropic" && cfg.AnthropicAPIKey == "" {
		missing = append(missing, "ANTHROPIC_API_KEY")
	}
	if thresholdMissing {
		missing = append(missing, "JUDGE_HITL_CONFIDENCE_THRESHOLD")
	}
	if len(missing) > 0 {
		return Config{}, MissingKeysError{Keys: missing}
	}
	return cfg, nil
}

func FromMap(values map[string]string) LookupFunc {
	return func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	}
}

func validate(cfg Config) []string {
	var missing []string
	if cfg.AppEnv == "" {
		missing = append(missing, "APP_ENV")
	}
	if cfg.DatabaseURL == "" || !isURL(cfg.DatabaseURL) {
		missing = append(missing, "DATABASE_URL")
	}
	if cfg.ObjectStore.Endpoint == "" || !isURL(cfg.ObjectStore.Endpoint) {
		missing = append(missing, "OBJECT_STORE_ENDPOINT")
	}
	if cfg.ObjectStore.AccessKey == "" {
		missing = append(missing, "OBJECT_STORE_ACCESS_KEY")
	}
	if cfg.ObjectStore.SecretKey == "" {
		missing = append(missing, "OBJECT_STORE_SECRET_KEY")
	}
	if cfg.ObjectStore.Bucket == "" {
		missing = append(missing, "OBJECT_STORE_BUCKET")
	}
	if cfg.ObjectStore.Region == "" {
		missing = append(missing, "OBJECT_STORE_REGION")
	}
	return missing
}

func required(lookup LookupFunc, key string) string {
	value, _ := lookup(key)
	return strings.TrimSpace(value)
}

func optional(lookup LookupFunc, key string) string {
	value, ok := lookup(key)
	if !ok {
		return ""
	}
	return strings.TrimSpace(value)
}

func isURL(value string) bool {
	parsed, err := url.Parse(value)
	return err == nil && parsed.Scheme != "" && parsed.Host != ""
}
