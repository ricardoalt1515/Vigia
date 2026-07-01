package bedrock

import (
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
)

// Exported sentinel errors form the adapter's small, stable error taxonomy. Callers reach these
// via errors.Is; no raw AWS SDK exception type crosses the adapter boundary.
var (
	ErrMissingConfig  = errors.New("bedrock: missing required configuration")
	ErrUnauthorized   = errors.New("bedrock: request not authorized")
	ErrThrottled      = errors.New("bedrock: request throttled")
	ErrModelNotFound  = errors.New("bedrock: model not found or not accessible")
	ErrInvalidRequest = errors.New("bedrock: invalid request")
	ErrTransient      = errors.New("bedrock: transient upstream error")
)

// normalizeError maps a Bedrock SDK error into the adapter's stable error set, wrapping the
// matched sentinel with the failing agent's name so the message reaches runAgent's failure-reason
// path unchanged.
func normalizeError(agent string, err error) error {
	sentinel := ErrTransient

	var throttling *types.ThrottlingException
	var accessDenied *types.AccessDeniedException
	var resourceNotFound *types.ResourceNotFoundException
	var validation *types.ValidationException
	var modelError *types.ModelErrorException

	switch {
	case errors.As(err, &throttling):
		sentinel = ErrThrottled
	case errors.As(err, &accessDenied):
		sentinel = ErrUnauthorized
	case errors.As(err, &resourceNotFound):
		sentinel = ErrModelNotFound
	case errors.As(err, &validation):
		sentinel = ErrInvalidRequest
	case errors.As(err, &modelError):
		sentinel = ErrInvalidRequest
	default:
		sentinel = ErrTransient
	}

	return fmt.Errorf("bedrock generate for agent %q: %w", agent, sentinel)
}
