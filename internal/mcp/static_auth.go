package mcp

import (
	"context"
	"crypto/subtle"
	"strings"

	"github.com/ricardoalt1515/vigia/internal/auth"
)

// StaticBearerAuthenticator is a local first-slice authenticator for the stdio
// MCP command. It yields the same auth.TenantContext shape as the production
// API-key authenticator without introducing a database dependency for synthetic fixtures.
type StaticBearerAuthenticator struct {
	Token    string
	TenantID string
	KeyID    string
}

func (a StaticBearerAuthenticator) Authenticate(ctx context.Context, authorization string) (auth.TenantContext, error) {
	if strings.TrimSpace(a.Token) == "" || strings.TrimSpace(a.TenantID) == "" || strings.TrimSpace(a.KeyID) == "" {
		return auth.TenantContext{}, auth.ErrUnauthorized
	}
	want := "Bearer " + strings.TrimSpace(a.Token)
	if subtle.ConstantTimeCompare([]byte(strings.TrimSpace(authorization)), []byte(want)) != 1 {
		return auth.TenantContext{}, auth.ErrUnauthorized
	}
	return auth.TenantContext{TenantID: strings.TrimSpace(a.TenantID), KeyID: strings.TrimSpace(a.KeyID)}, nil
}
