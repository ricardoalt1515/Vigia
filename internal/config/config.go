package config

import (
	"fmt"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
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

// validJudgeModes is the enum of accepted JUDGE_MODE values. Load fails fast
// (MissingKeysError naming JUDGE_MODE) when JUDGE_MODE resolves to anything
// else, so an unrecognized value (e.g. a typo like "Anthropic") cannot
// silently fall back to FakeJudge in cmd/seed's buildJudge.
var validJudgeModes = map[string]bool{
	"fake":      true,
	"anthropic": true,
}

var validTranscriberModes = map[string]bool{
	"fake":                        true,
	"aws-bedrock-data-automation": true,
	"aws-transcribe":              true,
}

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

	Transcriber TranscriberConfig
}

type TranscriberConfig struct {
	Mode            string
	LanguageCode    string
	AWSRegion       string
	AWSInputBucket  string
	AWSOutputBucket string
	AWSPollInterval time.Duration
	AWSTimeout      time.Duration
	KeepProviderRaw bool
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
		Transcriber: TranscriberConfig{
			Mode:            optional(lookup, "TRANSCRIBER_MODE"),
			LanguageCode:    optional(lookup, "TRANSCRIBER_LANGUAGE_CODE"),
			AWSRegion:       optional(lookup, "TRANSCRIBER_AWS_REGION"),
			AWSInputBucket:  optional(lookup, "TRANSCRIBER_AWS_INPUT_BUCKET"),
			AWSOutputBucket: optional(lookup, "TRANSCRIBER_AWS_OUTPUT_BUCKET"),
			KeepProviderRaw: boolOptional(lookup, "TRANSCRIBER_KEEP_PROVIDER_RAW"),
		},
	}

	cfg.ObjectStore.UseSSL = strings.HasPrefix(strings.ToLower(cfg.ObjectStore.Endpoint), "https://")

	if cfg.JudgeMode == "" {
		cfg.JudgeMode = "fake"
	}
	if cfg.JudgeModelID == "" {
		cfg.JudgeModelID = DefaultJudgeModelID
	}
	if cfg.Transcriber.Mode == "" {
		cfg.Transcriber.Mode = "fake"
	}
	if cfg.Transcriber.LanguageCode == "" {
		cfg.Transcriber.LanguageCode = "es-MX"
	}
	if cfg.Transcriber.AWSRegion == "" {
		cfg.Transcriber.AWSRegion = optional(lookup, "AWS_REGION")
	}

	transcriberDurationMissing := []string{}
	var err error
	cfg.Transcriber.AWSPollInterval, err = durationOptional(lookup, "TRANSCRIBER_AWS_POLL_INTERVAL", 5*time.Second)
	if err != nil {
		transcriberDurationMissing = append(transcriberDurationMissing, "TRANSCRIBER_AWS_POLL_INTERVAL")
	}
	cfg.Transcriber.AWSTimeout, err = durationOptional(lookup, "TRANSCRIBER_AWS_TIMEOUT", 10*time.Minute)
	if err != nil {
		transcriberDurationMissing = append(transcriberDurationMissing, "TRANSCRIBER_AWS_TIMEOUT")
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
	if !validJudgeModes[cfg.JudgeMode] {
		missing = append(missing, "JUDGE_MODE")
	}
	if !validTranscriberModes[cfg.Transcriber.Mode] {
		missing = append(missing, "TRANSCRIBER_MODE")
	}
	if strings.HasPrefix(cfg.Transcriber.Mode, "aws-") && cfg.Transcriber.AWSRegion == "" {
		missing = append(missing, "TRANSCRIBER_AWS_REGION")
	}
	if cfg.Transcriber.Mode == "aws-transcribe" && cfg.Transcriber.AWSOutputBucket == "" {
		missing = append(missing, "TRANSCRIBER_AWS_OUTPUT_BUCKET")
	}
	missing = append(missing, transcriberDurationMissing...)
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

func boolOptional(lookup LookupFunc, key string) bool {
	value := optional(lookup, key)
	parsed, err := strconv.ParseBool(value)
	return err == nil && parsed
}

func durationOptional(lookup LookupFunc, key string, defaultValue time.Duration) (time.Duration, error) {
	value := optional(lookup, key)
	if value == "" {
		return defaultValue, nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return 0, err
	}
	return parsed, nil
}

func isURL(value string) bool {
	parsed, err := url.Parse(value)
	return err == nil && parsed.Scheme != "" && parsed.Host != ""
}
