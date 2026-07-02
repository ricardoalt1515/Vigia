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
	blockOutcome := "BLOCK"
	blockReason := "outside window"
	reader := &fakeInteractionReader{
		itemsByTenant: map[string][]Interaction{
			"tenant-a": {
				{
					ID:         "interaction-a",
					OccurredAt: fixedTime,
					Channel:    "phone",
					Direction:  "outbound",
					Outcome:    &blockOutcome,
					Reason:     &blockReason,
				},
				{
					ID:         "interaction-b",
					OccurredAt: fixedTime,
					Channel:    "phone",
					Direction:  "outbound",
					Outcome:    nil,
					Reason:     nil,
				},
			},
		},
	}
	summary := &fakeSummaryReader{countByTenant: map[string]int64{"tenant-a": 3}}
	handler := NewServer(auth.NewAuthenticator(store, func() time.Time { return fixedTime }), reader, summary)

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
		if len(response.Interactions) != 2 {
			t.Fatalf("interactions len = %d, want 2", len(response.Interactions))
		}
		got := response.Interactions[0]
		if got.ID != "interaction-a" || got.Channel != "phone" || got.Direction != "outbound" {
			t.Fatalf("interaction = %#v", got)
		}
		if reader.lastTenantID != "tenant-a" {
			t.Fatalf("tenant id = %q, want tenant-a", reader.lastTenantID)
		}
	})

	t.Run("evaluated interaction includes non-null outcome and reason", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/interactions", nil)
		req.Header.Set("Authorization", "Bearer tenant-a-key")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		var response interactionsResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		got := response.Interactions[0]
		if got.Outcome == nil || *got.Outcome != "BLOCK" {
			t.Fatalf("Outcome = %v, want BLOCK", got.Outcome)
		}
		if got.Reason == nil || *got.Reason != "outside window" {
			t.Fatalf("Reason = %v, want non-empty", got.Reason)
		}
	})

	t.Run("unevaluated interaction does not fabricate an outcome", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/interactions", nil)
		req.Header.Set("Authorization", "Bearer tenant-a-key")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		var response interactionsResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		got := response.Interactions[1]
		if got.Outcome != nil {
			t.Fatalf("Outcome = %v, want nil (not fabricated PASS)", *got.Outcome)
		}
		if got.Reason != nil {
			t.Fatalf("Reason = %v, want nil", *got.Reason)
		}
	})
}

func TestGetSummary(t *testing.T) {
	fixedTime := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	store := &fakeKeyStore{
		records: map[string]auth.TenantAPIKey{
			auth.HashAPIKey("tenant-a-key"): {ID: "key-a", TenantID: "tenant-a", KeyHash: auth.HashAPIKey("tenant-a-key"), Status: auth.StatusActive},
			auth.HashAPIKey("tenant-b-key"): {ID: "key-b", TenantID: "tenant-b", KeyHash: auth.HashAPIKey("tenant-b-key"), Status: auth.StatusActive},
		},
	}
	reader := &fakeInteractionReader{itemsByTenant: map[string][]Interaction{}}
	summary := &fakeSummaryReader{countByTenant: map[string]int64{"tenant-a": 4, "tenant-b": 1}}
	handler := NewServer(auth.NewAuthenticator(store, func() time.Time { return fixedTime }), reader, summary)

	t.Run("returns the tenant's out-of-hours count", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/summary", nil)
		req.Header.Set("Authorization", "Bearer tenant-a-key")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body %q", rec.Code, http.StatusOK, rec.Body.String())
		}
		var response summaryResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if response.OutOfHoursCount != 4 {
			t.Fatalf("OutOfHoursCount = %d, want 4", response.OutOfHoursCount)
		}
	})

	t.Run("summary count is tenant-isolated", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/summary", nil)
		req.Header.Set("Authorization", "Bearer tenant-b-key")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		var response summaryResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if response.OutOfHoursCount != 1 {
			t.Fatalf("OutOfHoursCount = %d, want 1 (must not include tenant-a's count)", response.OutOfHoursCount)
		}
	})

	t.Run("rejects unauthorized credentials", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/summary", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
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

type fakeSummaryReader struct {
	countByTenant map[string]int64
	err           error
}

func (r *fakeSummaryReader) CountOutOfHours(ctx context.Context, tenantID string) (int64, error) {
	if r.err != nil {
		return 0, r.err
	}
	return r.countByTenant[tenantID], nil
}
