package main

import (
	"context"
	"strings"
	"testing"

	"github.com/ricardoalt1515/vigia/internal/auth"
)

func TestIssueTenantAPIKey(t *testing.T) {
	ctx := context.Background()
	tenantID := "11111111-1111-1111-1111-111111111111"
	store := &recordingAPIKeyStore{}

	issued, err := IssueTenantAPIKey(ctx, store, IssueTenantAPIKeyParams{
		TenantID: tenantID,
		Label:    "local-dev",
	})
	if err != nil {
		t.Fatalf("IssueTenantAPIKey() error = %v", err)
	}

	if issued.PlaintextKey == "" {
		t.Fatal("plaintext key was empty")
	}
	if !strings.HasPrefix(issued.PlaintextKey, tenantAPIKeyPrefix) {
		t.Fatalf("plaintext key prefix missing, want %q", tenantAPIKeyPrefix)
	}
	if issued.PlaintextKey == store.created.KeyHash {
		t.Fatal("plaintext key was persisted as the key hash")
	}
	if store.created.KeyHash != auth.HashAPIKey(issued.PlaintextKey) {
		t.Fatal("stored key hash does not match plaintext key hash")
	}
	if strings.Contains(store.created.KeyHash, issued.PlaintextKey) {
		t.Fatal("stored key hash contains plaintext key material")
	}
	if store.created.TenantID != tenantID {
		t.Fatalf("tenant ID = %q, want %q", store.created.TenantID, tenantID)
	}
	if store.created.Label != "local-dev" {
		t.Fatalf("label = %q, want local-dev", store.created.Label)
	}
	if store.created.Status != auth.StatusActive {
		t.Fatalf("status = %q, want %q", store.created.Status, auth.StatusActive)
	}
}

type recordingAPIKeyStore struct {
	created CreateTenantAPIKeyParams
}

func (s *recordingAPIKeyStore) CreateTenantAPIKey(_ context.Context, params CreateTenantAPIKeyParams) error {
	s.created = params
	return nil
}
