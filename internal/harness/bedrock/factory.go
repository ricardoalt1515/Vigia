package bedrock

import (
	"context"
	"fmt"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/ricardoalt1515/vigia/internal/harness"
	"github.com/ricardoalt1515/vigia/internal/harness/caseflow"
)

// Options configures NewFactory. Region and ModelID mirror the AWS_REGION and BEDROCK_MODEL_ID
// env keys internal/config also documents; NewFactory does not read env vars itself — callers
// (e.g. the CLI) resolve those and pass them in explicitly.
type Options struct {
	Region    string
	ModelID   string
	MaxTokens int
}

// factoryConfig accumulates functional-option state applied to every Provider a factory produces.
type factoryConfig struct {
	reporter UsageReporter
}

// Option configures factory-wide behavior for NewFactory.
type Option func(*factoryConfig)

// WithUsageReporter registers a UsageReporter that every Provider produced by the returned
// caseflow.ProviderFactory carries.
func WithUsageReporter(r UsageReporter) Option {
	return func(cfg *factoryConfig) { cfg.reporter = r }
}

// NewFactory validates opts, resolves AWS credentials eagerly, and returns a caseflow.ProviderFactory
// that produces one fresh *Provider per agent name, sharing a single Bedrock Runtime client.
//
// It fails fast (before any orchestrator is built) on missing Region, missing ModelID, or
// unresolvable AWS credentials — all wrapped in ErrMissingConfig — with no Bedrock SDK call
// attempted in the failure paths.
func NewFactory(ctx context.Context, opts Options, fnOpts ...Option) (caseflow.ProviderFactory, error) {
	if opts.Region == "" {
		return nil, fmt.Errorf("%w: AWS_REGION is required", ErrMissingConfig)
	}
	if opts.ModelID == "" {
		return nil, fmt.Errorf("%w: BEDROCK_MODEL_ID is required", ErrMissingConfig)
	}

	cfg := &factoryConfig{}
	for _, opt := range fnOpts {
		opt(cfg)
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(opts.Region))
	if err != nil {
		return nil, fmt.Errorf("%w: failed to load AWS configuration: %v", ErrMissingConfig, err)
	}

	// Eagerly resolve credentials so an unresolvable credential chain fails fast here, before any
	// orchestrator or agent work starts, instead of surfacing on the first Generate call.
	if _, err := awsCfg.Credentials.Retrieve(ctx); err != nil {
		return nil, fmt.Errorf("%w: failed to resolve AWS credentials: %v", ErrMissingConfig, err)
	}

	client := bedrockruntime.NewFromConfig(awsCfg)

	factory := func(agentName string) harness.ModelProvider {
		return &Provider{
			client:      client,
			modelID:     opts.ModelID,
			maxTokens:   opts.MaxTokens,
			agentName:   agentName,
			reportUsage: cfg.reporter,
		}
	}

	return factory, nil
}
