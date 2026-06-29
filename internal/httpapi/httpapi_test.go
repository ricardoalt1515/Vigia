package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ricardoalt1515/vigia/internal/auth"
)

func TestGetInteractions(t *testing.T) {
	fixedTime := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	store := &fakeKeyStore{
		records: map[string]auth.TenantAPIKey{
			auth.HashAPIKey("tenant-a-key"): {
				ID:       "key-a",
				TenantID: "tenant-a",
				KeyHash:  auth.HashAPIKey("tenant-a-key"),
				Status:   auth.StatusActive,
			},
		},
	}
	reader := &fakeInteractionReader{
		itemsByTenant: map[string][]Interaction{
			"tenant-a": {
				{
					ID:         "interaction-a",
					OccurredAt: fixedTime,
					Channel:    "phone",
					Direction:  "outbound",
				},
			},
		},
	}
	handler := NewServer(auth.NewAuthenticator(store, func() time.Time { return fixedTime }), reader)

	t.Run("rejects unauthorized credentials before reading interactions", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/interactions", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
		}
		if reader.calls != 0 {
			t.Fatalf("reader calls = %d, want 0", reader.calls)
		}
	})

	t.Run("returns authenticated tenant interactions", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/interactions", nil)
		req.Header.Set("Authorization", "Bearer tenant-a-key")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body %q", rec.Code, http.StatusOK, rec.Body.String())
		}
		var response interactionsResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if len(response.Interactions) != 1 {
			t.Fatalf("interactions len = %d, want 1", len(response.Interactions))
		}
		got := response.Interactions[0]
		if got.ID != "interaction-a" || got.Channel != "phone" || got.Direction != "outbound" {
			t.Fatalf("interaction = %#v", got)
		}
		if reader.lastTenantID != "tenant-a" {
			t.Fatalf("tenant id = %q, want tenant-a", reader.lastTenantID)
		}
	})
}

type fakeKeyStore struct {
	records map[string]auth.TenantAPIKey
}

func (s *fakeKeyStore) LookupTenantAPIKeyByHash(ctx context.Context, hash string) (auth.TenantAPIKey, error) {
	record, ok := s.records[hash]
	if !ok {
		return auth.TenantAPIKey{}, auth.ErrAPIKeyNotFound
	}
	return record, nil
}

type fakeInteractionReader struct {
	itemsByTenant map[string][]Interaction
	calls         int
	lastTenantID  string
	err           error
}

func (r *fakeInteractionReader) ListInteractions(ctx context.Context, tenantID string) ([]Interaction, error) {
	r.calls++
	r.lastTenantID = tenantID
	if r.err != nil {
		return nil, r.err
	}
	items, ok := r.itemsByTenant[tenantID]
	if !ok {
		return nil, errors.New("unexpected tenant")
	}
	return items, nil
}
