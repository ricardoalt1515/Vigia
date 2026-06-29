package auth

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestTenantAPIKeyAuth(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	validKey := "vigia_test_valid_key"
	validHash := HashAPIKey(validKey)
	activeRecord := TenantAPIKey{
		ID:       "key-1",
		TenantID: "tenant-1",
		KeyHash:  validHash,
		Status:   StatusActive,
	}

	tests := []struct {
		name          string
		authorization string
		records       map[string]TenantAPIKey
		wantTenantID  string
		wantErr       error
		wantLookup    bool
		wantHash      string
	}{
		{
			name:          "missing authorization",
			authorization: "",
			wantErr:       ErrUnauthorized,
		},
		{
			name:          "malformed authorization scheme",
			authorization: "Basic " + validKey,
			wantErr:       ErrUnauthorized,
		},
		{
			name:          "malformed bearer without key",
			authorization: "Bearer",
			wantErr:       ErrUnauthorized,
		},
		{
			name:          "invalid key",
			authorization: "Bearer missing-key",
			records:       map[string]TenantAPIKey{},
			wantErr:       ErrUnauthorized,
			wantLookup:    true,
			wantHash:      HashAPIKey("missing-key"),
		},
		{
			name:          "revoked key",
			authorization: "Bearer " + validKey,
			records: map[string]TenantAPIKey{
				validHash: withStatus(activeRecord, "revoked"),
			},
			wantErr:    ErrUnauthorized,
			wantLookup: true,
			wantHash:   validHash,
		},
		{
			name:          "expired key",
			authorization: "Bearer " + validKey,
			records: map[string]TenantAPIKey{
				validHash: withExpiry(activeRecord, now.Add(-time.Minute)),
			},
			wantErr:    ErrUnauthorized,
			wantLookup: true,
			wantHash:   validHash,
		},
		{
			name:          "valid key resolves tenant",
			authorization: "Bearer " + validKey,
			records: map[string]TenantAPIKey{
				validHash: activeRecord,
			},
			wantTenantID: "tenant-1",
			wantLookup:   true,
			wantHash:     validHash,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &memoryKeyStore{records: tt.records}
			authenticator := NewAuthenticator(store, func() time.Time { return now })

			tenant, err := authenticator.Authenticate(ctx, tt.authorization)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Authenticate() error = %v, want %v", err, tt.wantErr)
			}
			if tenant.TenantID != tt.wantTenantID {
				t.Fatalf("Authenticate() tenant = %q, want %q", tenant.TenantID, tt.wantTenantID)
			}
			if tt.wantLookup != (len(store.lookupHashes) == 1) {
				t.Fatalf("lookup count = %d, want lookup %v", len(store.lookupHashes), tt.wantLookup)
			}
			if tt.wantHash != "" && store.lookupHashes[0] != tt.wantHash {
				t.Fatalf("lookup hash = %q, want %q", store.lookupHashes[0], tt.wantHash)
			}
			if tt.wantHash != "" && store.lookupHashes[0] == validKey {
				t.Fatal("store lookup used plaintext key instead of hash")
			}
		})
	}
}

func withStatus(record TenantAPIKey, status string) TenantAPIKey {
	record.Status = status
	return record
}

func withExpiry(record TenantAPIKey, expiresAt time.Time) TenantAPIKey {
	record.ExpiresAt = &expiresAt
	return record
}

type memoryKeyStore struct {
	records      map[string]TenantAPIKey
	lookupHashes []string
}

func (s *memoryKeyStore) LookupTenantAPIKeyByHash(_ context.Context, hash string) (TenantAPIKey, error) {
	s.lookupHashes = append(s.lookupHashes, hash)
	if s.records == nil {
		return TenantAPIKey{}, ErrAPIKeyNotFound
	}
	record, ok := s.records[hash]
	if !ok {
		return TenantAPIKey{}, ErrAPIKeyNotFound
	}
	return record, nil
}
