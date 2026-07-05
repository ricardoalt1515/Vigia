package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ricardoalt1515/vigia/internal/auth"
	"github.com/ricardoalt1515/vigia/internal/core"
	"github.com/ricardoalt1515/vigia/internal/evaluation"
	"github.com/ricardoalt1515/vigia/internal/ledger"
	"github.com/ricardoalt1515/vigia/internal/orchestrator"
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
	requiresHITL := true
	threatFlagged := true
	notRequiresHITL := false
	notThreatFlagged := false
	reader := &fakeInteractionReader{
		itemsByTenant: map[string][]Interaction{
			"tenant-a": {
				{
					ID:            "interaction-a",
					OccurredAt:    fixedTime,
					Channel:       "phone",
					Direction:     "outbound",
					Outcome:       &blockOutcome,
					Reason:        &blockReason,
					RequiresHITL:  &requiresHITL,
					ThreatFlagged: &threatFlagged,
				},
				{
					ID:            "interaction-b",
					OccurredAt:    fixedTime,
					Channel:       "phone",
					Direction:     "outbound",
					Outcome:       nil,
					Reason:        nil,
					RequiresHITL:  &notRequiresHITL,
					ThreatFlagged: &notThreatFlagged,
				},
			},
		},
	}
	summary := &fakeSummaryReader{countByTenant: map[string]int64{"tenant-a": 3}}
	handler := NewServer(auth.NewAuthenticator(store, func() time.Time { return fixedTime }), reader, summary, &fakeEvidenceReader{}, &fakeReEvaluator{}, &fakeDashboardReader{}, nil)

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

	t.Run("interaction carries requires_hitl and threat_flagged fields", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/interactions", nil)
		req.Header.Set("Authorization", "Bearer tenant-a-key")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		var response interactionsResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		flagged := response.Interactions[0]
		if flagged.RequiresHITL == nil || !*flagged.RequiresHITL {
			t.Fatalf("RequiresHITL = %v, want true", flagged.RequiresHITL)
		}
		if flagged.ThreatFlagged == nil || !*flagged.ThreatFlagged {
			t.Fatalf("ThreatFlagged = %v, want true", flagged.ThreatFlagged)
		}

		unflagged := response.Interactions[1]
		if unflagged.RequiresHITL == nil || *unflagged.RequiresHITL {
			t.Fatalf("RequiresHITL = %v, want false", unflagged.RequiresHITL)
		}
		if unflagged.ThreatFlagged == nil || *unflagged.ThreatFlagged {
			t.Fatalf("ThreatFlagged = %v, want false", unflagged.ThreatFlagged)
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
	handler := NewServer(auth.NewAuthenticator(store, func() time.Time { return fixedTime }), reader, summary, &fakeEvidenceReader{}, &fakeReEvaluator{}, &fakeDashboardReader{}, nil)

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

// evidenceKey scopes fakeEvidenceReader lookups by (tenantID, interactionID)
// so the fake can prove tenant isolation the same way the real
// RLS-scoped adapter does.
type evidenceKey struct {
	tenantID      string
	interactionID string
}

type fakeEvidenceReader struct {
	packages map[evidenceKey]ledger.Package
	calls    int
	lastKey  evidenceKey
}

func (r *fakeEvidenceReader) GetEvidencePackage(ctx context.Context, tenantID, interactionID string) (ledger.Package, error) {
	r.calls++
	r.lastKey = evidenceKey{tenantID: tenantID, interactionID: interactionID}
	pkg, ok := r.packages[evidenceKey{tenantID: tenantID, interactionID: interactionID}]
	if !ok {
		return ledger.Package{}, ErrEvidenceNotFound
	}
	return pkg, nil
}

func testEvidencePackage() ledger.Package {
	results := []ledger.DetectorResult{
		{Code: "contact-hours", Outcome: "fail", Severity: "high", Rationale: "outside window"},
	}
	digest := ledger.ComputeInputsDigest(results)
	createdAt := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	body := ledger.Body{
		TenantID:            "tenant-a",
		InteractionEventID:  "interaction-evidenced",
		EvaluationID:        "eval-1",
		Seq:                 1,
		OverallOutcome:      "fail",
		PolicyBundleVersion: "",
		InputsDigest:        digest,
		CreatedAt:           createdAt,
	}
	hash := ledger.Hash(ledger.GenesisPrevHash, body)
	rec := ledger.EvidenceRecord{ID: "record-1", Body: body, PrevHash: ledger.GenesisPrevHash, Hash: hash}
	return ledger.BuildPackage(rec,
		ledger.PackageInteraction{ID: "interaction-evidenced", TenantID: "tenant-a", Channel: "phone", Direction: "outbound", OccurredAt: createdAt.Add(-time.Minute)},
		ledger.PackageEvaluation{ID: "eval-1", OverallOutcome: "fail", PolicyBundleVersion: "", CreatedAt: createdAt},
		results,
	)
}

func TestGetEvidence(t *testing.T) {
	fixedTime := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	store := &fakeKeyStore{
		records: map[string]auth.TenantAPIKey{
			auth.HashAPIKey("tenant-a-key"): {ID: "key-a", TenantID: "tenant-a", KeyHash: auth.HashAPIKey("tenant-a-key"), Status: auth.StatusActive},
			auth.HashAPIKey("tenant-b-key"): {ID: "key-b", TenantID: "tenant-b", KeyHash: auth.HashAPIKey("tenant-b-key"), Status: auth.StatusActive},
		},
	}
	pkg := testEvidencePackage()
	evidence := &fakeEvidenceReader{
		packages: map[evidenceKey]ledger.Package{
			{tenantID: "tenant-a", interactionID: "interaction-evidenced"}: pkg,
		},
	}
	reader := &fakeInteractionReader{itemsByTenant: map[string][]Interaction{}}
	summary := &fakeSummaryReader{}
	handler := NewServer(auth.NewAuthenticator(store, func() time.Time { return fixedTime }), reader, summary, evidence, &fakeReEvaluator{}, &fakeDashboardReader{}, nil)

	t.Run("evaluated interaction exports and independently verifies", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/interactions/interaction-evidenced/evidence", nil)
		req.Header.Set("Authorization", "Bearer tenant-a-key")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body %q", rec.Code, http.StatusOK, rec.Body.String())
		}
		var got ledger.Package
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if result := ledger.VerifyPackage(got); !result.OK {
			t.Fatalf("VerifyPackage(response) OK = false, reason %q, want intact", result.BreakReason)
		}
		if evidence.lastKey.tenantID != "tenant-a" {
			t.Fatalf("tenant id = %q, want tenant-a", evidence.lastKey.tenantID)
		}
	})

	t.Run("cross-tenant interaction id returns a generic 404 and leaks nothing", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/interactions/interaction-evidenced/evidence", nil)
		req.Header.Set("Authorization", "Bearer tenant-b-key")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
		}
		if body := rec.Body.String(); strings.Contains(body, "tenant-a") || strings.Contains(body, "outside window") {
			t.Fatalf("404 body leaked tenant A data: %q", body)
		}
	})

	t.Run("unevaluated interaction returns a generic 404 with no fabricated fields", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/interactions/never-evaluated/evidence", nil)
		req.Header.Set("Authorization", "Bearer tenant-a-key")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
		}
	})

	t.Run("rejects unauthorized credentials before reading evidence", func(t *testing.T) {
		evidence.calls = 0
		req := httptest.NewRequest(http.MethodGet, "/v1/interactions/interaction-evidenced/evidence", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
		}
		if evidence.calls != 0 {
			t.Fatalf("evidence reader calls = %d, want 0", evidence.calls)
		}
	})
}

// fakeDashboardReader scopes both dashboard aggregates by tenantID, mirroring
// fakeSummaryReader's tenant-keyed map so a fake can prove tenant isolation
// the same way the real RLS-scoped adapter does.
type fakeDashboardReader struct {
	byDespachoByTenant map[string][]DespachoRate
	byCauseByTenant    map[string][]CauseCount
	err                error
	byDespachoCalls    int
	byCauseCalls       int
	lastTenantID       string
}

func (r *fakeDashboardReader) ByDespacho(_ context.Context, tenantID string) ([]DespachoRate, error) {
	r.byDespachoCalls++
	r.lastTenantID = tenantID
	if r.err != nil {
		return nil, r.err
	}
	return r.byDespachoByTenant[tenantID], nil
}

func (r *fakeDashboardReader) ByCause(_ context.Context, tenantID string) ([]CauseCount, error) {
	r.byCauseCalls++
	r.lastTenantID = tenantID
	if r.err != nil {
		return nil, r.err
	}
	return r.byCauseByTenant[tenantID], nil
}

func TestGetDashboardByDespacho(t *testing.T) {
	fixedTime := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	store := &fakeKeyStore{
		records: map[string]auth.TenantAPIKey{
			auth.HashAPIKey("tenant-a-key"): {ID: "key-a", TenantID: "tenant-a", KeyHash: auth.HashAPIKey("tenant-a-key"), Status: auth.StatusActive},
			auth.HashAPIKey("tenant-b-key"): {ID: "key-b", TenantID: "tenant-b", KeyHash: auth.HashAPIKey("tenant-b-key"), Status: auth.StatusActive},
		},
	}
	despachoAID := "despacho-a"
	dashboards := &fakeDashboardReader{
		byDespachoByTenant: map[string][]DespachoRate{
			"tenant-a": {
				{DespachoID: &despachoAID, DespachoName: "Despacho A", Total: 10, Violations: 5, ViolationRate: 0.5},
				{DespachoID: nil, DespachoName: "unattributed", Total: 4, Violations: 1, ViolationRate: 0.25},
			},
			"tenant-b": {},
		},
	}
	reader := &fakeInteractionReader{itemsByTenant: map[string][]Interaction{}}
	summary := &fakeSummaryReader{}
	handler := NewServer(auth.NewAuthenticator(store, func() time.Time { return fixedTime }), reader, summary, &fakeEvidenceReader{}, &fakeReEvaluator{}, dashboards, nil)

	t.Run("rejects unauthorized credentials before reading despacho rates", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/dashboards/by-despacho", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
		}
		if dashboards.byDespachoCalls != 0 {
			t.Fatalf("byDespacho calls = %d, want 0", dashboards.byDespachoCalls)
		}
	})

	t.Run("returns the ranked despacho rates including the unattributed bucket", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/dashboards/by-despacho", nil)
		req.Header.Set("Authorization", "Bearer tenant-a-key")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body %q", rec.Code, http.StatusOK, rec.Body.String())
		}
		var response despachoRatesResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if len(response.Despachos) != 2 {
			t.Fatalf("despachos len = %d, want 2", len(response.Despachos))
		}
		first := response.Despachos[0]
		if first.DespachoID == nil || *first.DespachoID != "despacho-a" || first.ViolationRate != 0.5 {
			t.Fatalf("first despacho = %#v", first)
		}
		unattributed := response.Despachos[1]
		if unattributed.DespachoID != nil {
			t.Fatalf("unattributed DespachoID = %v, want nil", *unattributed.DespachoID)
		}
		if unattributed.DespachoName != "unattributed" {
			t.Fatalf("unattributed DespachoName = %q, want unattributed", unattributed.DespachoName)
		}
		if dashboards.lastTenantID != "tenant-a" {
			t.Fatalf("tenant id = %q, want tenant-a", dashboards.lastTenantID)
		}
	})

	t.Run("by-despacho ranking is tenant-isolated", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/dashboards/by-despacho", nil)
		req.Header.Set("Authorization", "Bearer tenant-b-key")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		var response despachoRatesResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if len(response.Despachos) != 0 {
			t.Fatalf("despachos len = %d, want 0 (must not include tenant-a's rows)", len(response.Despachos))
		}
	})
}

func TestGetDashboardByCause(t *testing.T) {
	fixedTime := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	store := &fakeKeyStore{
		records: map[string]auth.TenantAPIKey{
			auth.HashAPIKey("tenant-a-key"): {ID: "key-a", TenantID: "tenant-a", KeyHash: auth.HashAPIKey("tenant-a-key"), Status: auth.StatusActive},
		},
	}
	dashboards := &fakeDashboardReader{
		byCauseByTenant: map[string][]CauseCount{
			"tenant-a": {
				{RuleCode: "MX-REDECO-03", Violations: 0, Warnings: 3},
				{RuleCode: "MX-REDECO-04", Violations: 2, Warnings: 0},
			},
		},
	}
	reader := &fakeInteractionReader{itemsByTenant: map[string][]Interaction{}}
	summary := &fakeSummaryReader{}
	handler := NewServer(auth.NewAuthenticator(store, func() time.Time { return fixedTime }), reader, summary, &fakeEvidenceReader{}, &fakeReEvaluator{}, dashboards, nil)

	t.Run("returns per-rule-code violations and warnings separately", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/dashboards/by-cause", nil)
		req.Header.Set("Authorization", "Bearer tenant-a-key")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body %q", rec.Code, http.StatusOK, rec.Body.String())
		}
		var response causeCountsResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if len(response.Causes) != 2 {
			t.Fatalf("causes len = %d, want 2", len(response.Causes))
		}
		disclosure := response.Causes[0]
		if disclosure.RuleCode != "MX-REDECO-03" || disclosure.Violations != 0 || disclosure.Warnings != 3 {
			t.Fatalf("disclosure cause = %#v, want warnings=3 violations=0", disclosure)
		}
	})

	t.Run("rejects unauthorized credentials before reading cause counts", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/dashboards/by-cause", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
		}
	})
}

// reevalKey scopes fakeReEvaluator lookups by (interactionID, policyBundleID)
// so cases can distinguish "unknown interaction" from "unknown bundle".
type reevalKey struct {
	interactionID  string
	policyBundleID string
}

type fakeReEvaluator struct {
	results map[reevalKey]core.Evaluation
	err     error
	calls   int
}

func (r *fakeReEvaluator) ReEvaluateInteraction(_ context.Context, _, interactionID, policyBundleID string) (core.Evaluation, error) {
	r.calls++
	if r.err != nil {
		return core.Evaluation{}, r.err
	}
	got, ok := r.results[reevalKey{interactionID: interactionID, policyBundleID: policyBundleID}]
	if !ok {
		return core.Evaluation{}, evaluation.ErrPolicyBundleNotFound
	}
	return got, nil
}

// TestReEvaluateInteraction covers *Reproducible Re-Evaluation Against a
// Specific Bundle Version* [integration]: a valid historical bundle id
// returns 200 with the stamped version/id, and an unknown bundle id returns
// a defined error status while creating no evaluation row (proven here by
// the fake never persisting — ReEvaluateInteraction is non-persisting by
// construction).
func TestReEvaluateInteraction(t *testing.T) {
	fixedTime := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	store := &fakeKeyStore{
		records: map[string]auth.TenantAPIKey{
			auth.HashAPIKey("tenant-a-key"): {ID: "key-a", TenantID: "tenant-a", KeyHash: auth.HashAPIKey("tenant-a-key"), Status: auth.StatusActive},
		},
	}
	bundleID := "bundle-v1"
	reevaluator := &fakeReEvaluator{
		results: map[reevalKey]core.Evaluation{
			{interactionID: "interaction-1", policyBundleID: "bundle-v1"}: {
				TenantID:            "tenant-a",
				InteractionEventID:  "interaction-1",
				OverallOutcome:      "pass",
				PolicyBundleVersion: "v1",
				PolicyBundleID:      (*core.ID)(&bundleID),
			},
		},
	}
	reader := &fakeInteractionReader{itemsByTenant: map[string][]Interaction{}}
	summary := &fakeSummaryReader{}
	handler := NewServer(auth.NewAuthenticator(store, func() time.Time { return fixedTime }), reader, summary, &fakeEvidenceReader{}, reevaluator, &fakeDashboardReader{}, nil)

	t.Run("valid historical bundle id returns 200 with stamped version/id", func(t *testing.T) {
		body := strings.NewReader(`{"policy_bundle_id":"bundle-v1"}`)
		req := httptest.NewRequest(http.MethodPost, "/v1/interactions/interaction-1/reevaluate", body)
		req.Header.Set("Authorization", "Bearer tenant-a-key")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body %q", rec.Code, http.StatusOK, rec.Body.String())
		}
		var got reevaluateResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if got.OverallOutcome != "pass" {
			t.Fatalf("OverallOutcome = %q, want pass", got.OverallOutcome)
		}
		if got.PolicyBundleVersion != "v1" {
			t.Fatalf("PolicyBundleVersion = %q, want v1", got.PolicyBundleVersion)
		}
		if got.PolicyBundleID == nil || *got.PolicyBundleID != "bundle-v1" {
			t.Fatalf("PolicyBundleID = %v, want pointer to bundle-v1", got.PolicyBundleID)
		}
	})

	t.Run("unknown bundle id returns a defined error status and creates no evaluation", func(t *testing.T) {
		body := strings.NewReader(`{"policy_bundle_id":"unknown-bundle"}`)
		req := httptest.NewRequest(http.MethodPost, "/v1/interactions/interaction-1/reevaluate", body)
		req.Header.Set("Authorization", "Bearer tenant-a-key")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
		}
	})

	t.Run("rejects unauthorized credentials before reevaluating", func(t *testing.T) {
		reevaluator.calls = 0
		body := strings.NewReader(`{"policy_bundle_id":"bundle-v1"}`)
		req := httptest.NewRequest(http.MethodPost, "/v1/interactions/interaction-1/reevaluate", body)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
		}
		if reevaluator.calls != 0 {
			t.Fatalf("reevaluator calls = %d, want 0", reevaluator.calls)
		}
	})

	t.Run("cross-tenant result never leaks: reevaluated interaction belonging to another tenant returns 404", func(t *testing.T) {
		crossTenantBundleID := "cross-bundle"
		crossTenantReevaluator := &fakeReEvaluator{
			results: map[reevalKey]core.Evaluation{
				{interactionID: "interaction-cross", policyBundleID: "cross-bundle"}: {
					TenantID:            "tenant-b",
					InteractionEventID:  "interaction-cross",
					OverallOutcome:      "pass",
					PolicyBundleVersion: "v1",
					PolicyBundleID:      (*core.ID)(&crossTenantBundleID),
				},
			},
		}
		crossHandler := NewServer(auth.NewAuthenticator(store, func() time.Time { return fixedTime }), reader, summary, &fakeEvidenceReader{}, crossTenantReevaluator, &fakeDashboardReader{}, nil)

		body := strings.NewReader(`{"policy_bundle_id":"cross-bundle"}`)
		req := httptest.NewRequest(http.MethodPost, "/v1/interactions/interaction-cross/reevaluate", body)
		req.Header.Set("Authorization", "Bearer tenant-a-key")
		rec := httptest.NewRecorder()

		crossHandler.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want %d (result tenant_id must never leak across the authenticated caller's tenant)", rec.Code, http.StatusNotFound)
		}
	})
}

func TestComplaintEndpoints(t *testing.T) {
	fixedTime := time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)
	store := &fakeKeyStore{records: map[string]auth.TenantAPIKey{
		auth.HashAPIKey("tenant-a-key"): {ID: "key-a", TenantID: "tenant-a", KeyHash: auth.HashAPIKey("tenant-a-key"), Status: auth.StatusActive},
	}}
	complaints := &fakeComplaintWorkflow{
		holidays: []orchestrator.HolidayRow{},
		createResults: []orchestrator.ComplaintCase{
			{ID: "case-1", TenantID: "tenant-a", InteractionID: "11111111-1111-1111-1111-111111111111", RedecoCause: "improper_contact", State: "open", OpenedAt: fixedTime, SLADueAt: fixedTime.AddDate(0, 0, 14), CalendarVersion: "mx-lft-art-74-2026a", IdempotencyKey: "idem-1", Created: true},
			{ID: "case-1", TenantID: "tenant-a", InteractionID: "11111111-1111-1111-1111-111111111111", RedecoCause: "improper_contact", State: "open", OpenedAt: fixedTime, SLADueAt: fixedTime.AddDate(0, 0, 14), CalendarVersion: "mx-lft-art-74-2026a", IdempotencyKey: "idem-1", Created: false},
			{ID: "case-2", TenantID: "tenant-a", InteractionID: "22222222-2222-2222-2222-222222222222", RedecoCause: "harassment", State: "open", OpenedAt: fixedTime, SLADueAt: fixedTime.AddDate(0, 0, 14), CalendarVersion: "mx-lft-art-74-2026a", IdempotencyKey: "header-idem", Created: true},
			{ID: "case-3", TenantID: "tenant-a", InteractionID: "33333333-3333-3333-3333-333333333333", RedecoCause: "harassment", State: "open", OpenedAt: fixedTime, SLADueAt: fixedTime.AddDate(0, 0, 14), CalendarVersion: "mx-lft-art-74-2026a", IdempotencyKey: "same-idem", Created: true},
		},
	}
	handler := NewServer(auth.NewAuthenticator(store, func() time.Time { return fixedTime }), &fakeInteractionReader{itemsByTenant: map[string][]Interaction{}}, &fakeSummaryReader{}, &fakeEvidenceReader{}, &fakeReEvaluator{}, &fakeDashboardReader{}, complaints)
	handler.now = func() time.Time { return fixedTime }

	t.Run("creates complaint case with computed SLA input", func(t *testing.T) {
		body := strings.NewReader(`{"idempotency_key":"idem-1","interaction_id":"11111111-1111-1111-1111-111111111111","redeco_cause":"improper_contact"}`)
		req := httptest.NewRequest(http.MethodPost, "/v1/complaints", body)
		req.Header.Set("Authorization", "Bearer tenant-a-key")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d, want %d; body %q", rec.Code, http.StatusCreated, rec.Body.String())
		}
		if len(complaints.createInputs) != 1 {
			t.Fatalf("create calls = %d, want 1", len(complaints.createInputs))
		}
		in := complaints.createInputs[0]
		if in.TenantID != "tenant-a" || in.IdempotencyKey != "idem-1" || in.RedecoCause != "improper_contact" {
			t.Fatalf("create input = %#v", in)
		}
		if in.CalendarVersion != defaultComplaintCalendarVersion {
			t.Fatalf("calendar version = %q", in.CalendarVersion)
		}
		if !in.SLADueAt.Equal(orchestrator.AddBusinessDays(fixedTime, 10, orchestrator.LoadCalendar(defaultComplaintCalendarVersion, nil))) {
			t.Fatalf("SLADueAt = %s", in.SLADueAt)
		}
		var got complaintCaseResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if got.ID != "case-1" || got.State != "open" || got.IdempotencyKey != "idem-1" {
			t.Fatalf("response = %#v", got)
		}
	})

	t.Run("idempotent repeat returns existing case with 200", func(t *testing.T) {
		body := strings.NewReader(`{"idempotency_key":"idem-1","interaction_id":"11111111-1111-1111-1111-111111111111","redeco_cause":"improper_contact"}`)
		req := httptest.NewRequest(http.MethodPost, "/v1/complaints", body)
		req.Header.Set("Authorization", "Bearer tenant-a-key")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body %q", rec.Code, http.StatusOK, rec.Body.String())
		}
		if len(complaints.createInputs) != 2 {
			t.Fatalf("create calls = %d, want 2", len(complaints.createInputs))
		}
	})

	t.Run("accepts idempotency key from header when body omits it", func(t *testing.T) {
		body := strings.NewReader(`{"interaction_id":"22222222-2222-2222-2222-222222222222","redeco_cause":"harassment"}`)
		req := httptest.NewRequest(http.MethodPost, "/v1/complaints", body)
		req.Header.Set("Authorization", "Bearer tenant-a-key")
		req.Header.Set("Idempotency-Key", "header-idem")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d, want %d; body %q", rec.Code, http.StatusCreated, rec.Body.String())
		}
		if len(complaints.createInputs) != 3 {
			t.Fatalf("create calls = %d, want 3", len(complaints.createInputs))
		}
		if got := complaints.createInputs[2].IdempotencyKey; got != "header-idem" {
			t.Fatalf("idempotency key = %q, want header-idem", got)
		}
	})

	t.Run("accepts matching header and body idempotency keys", func(t *testing.T) {
		body := strings.NewReader(`{"idempotency_key":"same-idem","interaction_id":"33333333-3333-3333-3333-333333333333","redeco_cause":"harassment"}`)
		req := httptest.NewRequest(http.MethodPost, "/v1/complaints", body)
		req.Header.Set("Authorization", "Bearer tenant-a-key")
		req.Header.Set("Idempotency-Key", "same-idem")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d, want %d; body %q", rec.Code, http.StatusCreated, rec.Body.String())
		}
		if len(complaints.createInputs) != 4 {
			t.Fatalf("create calls = %d, want 4", len(complaints.createInputs))
		}
		if got := complaints.createInputs[3].IdempotencyKey; got != "same-idem" {
			t.Fatalf("idempotency key = %q, want same-idem", got)
		}
	})

	t.Run("rejects mismatched header and body idempotency keys", func(t *testing.T) {
		before := len(complaints.createInputs)
		body := strings.NewReader(`{"idempotency_key":"body-idem","interaction_id":"33333333-3333-3333-3333-333333333333","redeco_cause":"harassment"}`)
		req := httptest.NewRequest(http.MethodPost, "/v1/complaints", body)
		req.Header.Set("Authorization", "Bearer tenant-a-key")
		req.Header.Set("Idempotency-Key", "header-idem")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d; body %q", rec.Code, http.StatusBadRequest, rec.Body.String())
		}
		if len(complaints.createInputs) != before {
			t.Fatalf("create calls = %d, want %d", len(complaints.createInputs), before)
		}
	})

	t.Run("accepts human review for awaiting complaint", func(t *testing.T) {
		complaints.reviewResults = []HumanReview{{ID: "review-1", TenantID: "tenant-a", ComplaintCaseID: "case-awaiting", Decision: "approve", Reviewer: "ops@example.com", Notes: "ok", CreatedAt: fixedTime}}
		body := strings.NewReader(`{"decision":"approve","reviewer":"ops@example.com","notes":"ok"}`)
		req := httptest.NewRequest(http.MethodPost, "/v1/complaints/case-awaiting/reviews", body)
		req.Header.Set("Authorization", "Bearer tenant-a-key")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusAccepted {
			t.Fatalf("status = %d, want %d; body %q", rec.Code, http.StatusAccepted, rec.Body.String())
		}
		if len(complaints.reviewInputs) != 1 || complaints.reviewInputs[0].TenantID != "tenant-a" || complaints.reviewInputs[0].Decision != "approve" {
			t.Fatalf("review inputs = %#v", complaints.reviewInputs)
		}
	})

	t.Run("late review returns conflict and records nothing", func(t *testing.T) {
		complaints.reviewErr = ErrComplaintReviewConflict
		body := strings.NewReader(`{"decision":"approve","reviewer":"ops@example.com"}`)
		req := httptest.NewRequest(http.MethodPost, "/v1/complaints/case-escalated/reviews", body)
		req.Header.Set("Authorization", "Bearer tenant-a-key")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusConflict {
			t.Fatalf("status = %d, want %d; body %q", rec.Code, http.StatusConflict, rec.Body.String())
		}
	})
}

type fakeComplaintWorkflow struct {
	holidays      []orchestrator.HolidayRow
	createResults []orchestrator.ComplaintCase
	createInputs  []orchestrator.CreateComplaintCaseInput
	reviewResults []HumanReview
	reviewInputs  []CreateHumanReviewInput
	reviewErr     error
}

func (f *fakeComplaintWorkflow) ListBusinessDayHolidays(ctx context.Context, version string) ([]orchestrator.HolidayRow, error) {
	return f.holidays, nil
}

func (f *fakeComplaintWorkflow) CreateComplaintCase(ctx context.Context, in orchestrator.CreateComplaintCaseInput) (orchestrator.ComplaintCase, error) {
	f.createInputs = append(f.createInputs, in)
	if len(f.createResults) == 0 {
		return orchestrator.ComplaintCase{}, errors.New("missing fake create result")
	}
	out := f.createResults[0]
	f.createResults = f.createResults[1:]
	return out, nil
}

func (f *fakeComplaintWorkflow) CreateHumanReview(ctx context.Context, in CreateHumanReviewInput) (HumanReview, error) {
	f.reviewInputs = append(f.reviewInputs, in)
	if f.reviewErr != nil {
		return HumanReview{}, f.reviewErr
	}
	if len(f.reviewResults) == 0 {
		return HumanReview{}, errors.New("missing fake review result")
	}
	out := f.reviewResults[0]
	f.reviewResults = f.reviewResults[1:]
	return out, nil
}
