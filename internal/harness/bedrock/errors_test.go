package bedrock

import (
	"errors"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
)

func TestNormalizeError(t *testing.T) {
	tests := []struct {
		name   string
		sdkErr error
		want   error
	}{
		{
			name:   "throttling",
			sdkErr: &types.ThrottlingException{Message: aws("rate exceeded")},
			want:   ErrThrottled,
		},
		{
			name:   "access denied",
			sdkErr: &types.AccessDeniedException{Message: aws("not authorized")},
			want:   ErrUnauthorized,
		},
		{
			name:   "resource not found",
			sdkErr: &types.ResourceNotFoundException{Message: aws("model missing")},
			want:   ErrModelNotFound,
		},
		{
			name:   "validation exception",
			sdkErr: &types.ValidationException{Message: aws("bad request")},
			want:   ErrInvalidRequest,
		},
		{
			name:   "model error exception",
			sdkErr: &types.ModelErrorException{Message: aws("model error")},
			want:   ErrInvalidRequest,
		},
		{
			name:   "internal server exception",
			sdkErr: &types.InternalServerException{Message: aws("internal")},
			want:   ErrTransient,
		},
		{
			name:   "service unavailable exception",
			sdkErr: &types.ServiceUnavailableException{Message: aws("unavailable")},
			want:   ErrTransient,
		},
		{
			name:   "model timeout exception",
			sdkErr: &types.ModelTimeoutException{Message: aws("timeout")},
			want:   ErrTransient,
		},
		{
			name:   "unrecognized default",
			sdkErr: errors.New("some other transport error"),
			want:   ErrTransient,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeError("triage_agent", tt.sdkErr)
			if got == nil {
				t.Fatalf("normalizeError() returned nil, want non-nil wrapping %v", tt.want)
			}
			if !errors.Is(got, tt.want) {
				t.Errorf("normalizeError() = %v, want errors.Is match for %v", got, tt.want)
			}
			if !strings.Contains(got.Error(), "triage_agent") {
				t.Errorf("normalizeError() message %q does not contain agent name", got.Error())
			}
		})
	}
}

// aws is a small local helper to build *string message fields for SDK exception values.
func aws(s string) *string { return &s }
