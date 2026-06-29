package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"
)

const StatusActive = "active"

var (
	ErrUnauthorized   = errors.New("unauthorized")
	ErrAPIKeyNotFound = errors.New("tenant api key not found")
)

type TenantAPIKey struct {
	ID        string
	TenantID  string
	KeyHash   string
	Status    string
	ExpiresAt *time.Time
}

type TenantContext struct {
	TenantID string
	KeyID    string
}

type TenantAPIKeyStore interface {
	LookupTenantAPIKeyByHash(ctx context.Context, hash string) (TenantAPIKey, error)
}

type Authenticator struct {
	store TenantAPIKeyStore
	now   func() time.Time
}

func NewAuthenticator(store TenantAPIKeyStore, now func() time.Time) *Authenticator {
	if now == nil {
		now = time.Now
	}
	return &Authenticator{store: store, now: now}
}

func (a *Authenticator) Authenticate(ctx context.Context, authorization string) (TenantContext, error) {
	key, ok := bearerKey(authorization)
	if !ok {
		return TenantContext{}, ErrUnauthorized
	}

	record, err := a.store.LookupTenantAPIKeyByHash(ctx, HashAPIKey(key))
	if err != nil {
		if errors.Is(err, ErrAPIKeyNotFound) {
			return TenantContext{}, ErrUnauthorized
		}
		return TenantContext{}, err
	}
	if record.Status != StatusActive || record.TenantID == "" {
		return TenantContext{}, ErrUnauthorized
	}
	if record.ExpiresAt != nil && !record.ExpiresAt.After(a.now()) {
		return TenantContext{}, ErrUnauthorized
	}

	return TenantContext{TenantID: record.TenantID, KeyID: record.ID}, nil
}

func HashAPIKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

func bearerKey(authorization string) (string, bool) {
	scheme, key, ok := strings.Cut(strings.TrimSpace(authorization), " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return "", false
	}
	key = strings.TrimSpace(key)
	if key == "" || strings.Contains(key, " ") {
		return "", false
	}
	return key, true
}
