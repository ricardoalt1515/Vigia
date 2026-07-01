package bedrock

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/ricardoalt1515/vigia/internal/harness"
)

// UsageReporter receives token usage after a successful Provider.Generate call. It is wired via
// NewFactory's WithUsageReporter option, never on the harness.ModelProvider port itself.
type UsageReporter func(agentName string, usage Usage)

// Provider implements harness.ModelProvider against Claude via Amazon Bedrock Runtime for one
// named Domain Agent. NewFactory constructs a fresh Provider per agent, sharing one invoker.
type Provider struct {
	client      invoker
	modelID     string
	maxTokens   int
	agentName   string
	reportUsage UsageReporter // may be nil
}

// Generate builds a Bedrock Claude Messages request from req, invokes the configured invoker, and
// normalizes the result into harness.ModelOutput. On success with a configured reportUsage, the
// reporter is invoked with the parsed Usage before returning.
func (p *Provider) Generate(ctx context.Context, req harness.ModelRequest) (harness.ModelOutput, error) {
	body, err := buildRequestBody(req.Input, p.maxTokens)
	if err != nil {
		return harness.ModelOutput{}, normalizeError(p.agentName, err)
	}

	contentType := "application/json"
	out, err := p.client.InvokeModel(ctx, &bedrockruntime.InvokeModelInput{
		ModelId:     &p.modelID,
		ContentType: &contentType,
		Accept:      &contentType,
		Body:        body,
	})
	if err != nil {
		return harness.ModelOutput{}, normalizeError(p.agentName, err)
	}

	modelOutput, usage, err := parseResponse(out.Body)
	if err != nil {
		return harness.ModelOutput{}, normalizeError(p.agentName, err)
	}

	if p.reportUsage != nil {
		p.reportUsage(p.agentName, usage)
	}

	return modelOutput, nil
}
