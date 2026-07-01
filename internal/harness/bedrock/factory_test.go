package bedrock

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// clearCredentialEnv scopes the process environment so the AWS SDK default credential chain
// cannot resolve credentials from env vars, EC2 IMDS, ECS container credentials, or web identity
// tokens — and cannot reach the network doing so. AWS_SHARED_CREDENTIALS_FILE/AWS_CONFIG_FILE are
// pointed at nonexistent paths (rather than left unset) because the SDK's shared-config home
// directory lookup is resolved once at package init and does not observe a later t.Setenv("HOME",
// ...) override within the same test binary.
func clearCredentialEnv(t *testing.T) {
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

func TestNewFactory_MissingRegion(t *testing.T) {
	clearCredentialEnv(t)

	factory, err := NewFactory(context.Background(), Options{Region: "", ModelID: "anthropic.claude-3-sonnet"})
	if factory != nil {
		t.Error("factory != nil, want nil on missing region")
	}
	if !errors.Is(err, ErrMissingConfig) {
		t.Fatalf("err = %v, want errors.Is match for ErrMissingConfig", err)
	}
	if !strings.Contains(err.Error(), "AWS_REGION") {
		t.Errorf("err message %q does not identify AWS_REGION", err.Error())
	}
}

func TestNewFactory_MissingModelID(t *testing.T) {
	clearCredentialEnv(t)

	factory, err := NewFactory(context.Background(), Options{Region: "us-east-1", ModelID: ""})
	if factory != nil {
		t.Error("factory != nil, want nil on missing model id")
	}
	if !errors.Is(err, ErrMissingConfig) {
		t.Fatalf("err = %v, want errors.Is match for ErrMissingConfig", err)
	}
	if !strings.Contains(err.Error(), "BEDROCK_MODEL_ID") {
		t.Errorf("err message %q does not identify BEDROCK_MODEL_ID", err.Error())
	}
}

func TestNewFactory_MissingCredentials(t *testing.T) {
	clearCredentialEnv(t)

	factory, err := NewFactory(context.Background(), Options{Region: "us-east-1", ModelID: "anthropic.claude-3-sonnet"})
	if factory != nil {
		t.Error("factory != nil, want nil when credentials are unresolvable")
	}
	if !errors.Is(err, ErrMissingConfig) {
		t.Fatalf("err = %v, want errors.Is match for ErrMissingConfig", err)
	}
}

func TestNewFactory_ValidConfig(t *testing.T) {
	clearCredentialEnv(t)
	t.Setenv("AWS_ACCESS_KEY_ID", "fake-access-key-id")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "fake-secret-access-key")

	var recordedAgent string
	var recordedUsage Usage
	reporter := func(agentName string, usage Usage) {
		recordedAgent = agentName
		recordedUsage = usage
	}

	factory, err := NewFactory(context.Background(), Options{
		Region:    "us-east-1",
		ModelID:   "anthropic.claude-3-sonnet",
		MaxTokens: 512,
	}, WithUsageReporter(reporter))
	if err != nil {
		t.Fatalf("NewFactory() error = %v, want nil with valid config and credentials", err)
	}
	if factory == nil {
		t.Fatal("factory is nil, want non-nil caseflow.ProviderFactory")
	}

	provider := factory("triage_agent")
	if provider == nil {
		t.Fatal("factory(agentName) returned nil, want non-nil ModelProvider")
	}
	p, ok := provider.(*Provider)
	if !ok {
		t.Fatalf("factory(agentName) returned %T, want *Provider", provider)
	}
	if p.agentName != "triage_agent" {
		t.Errorf("p.agentName = %q, want %q", p.agentName, "triage_agent")
	}
	if p.reportUsage == nil {
		t.Fatal("p.reportUsage is nil, want the WithUsageReporter option carried through")
	}

	p.reportUsage("triage_agent", Usage{InputTokens: 1, OutputTokens: 2})
	if recordedAgent != "triage_agent" || recordedUsage != (Usage{InputTokens: 1, OutputTokens: 2}) {
		t.Errorf("reporter recorded (%q, %+v), want (%q, {1 2})", recordedAgent, recordedUsage, "triage_agent")
	}
}
