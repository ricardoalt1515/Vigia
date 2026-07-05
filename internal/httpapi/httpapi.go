package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/ricardoalt1515/vigia/internal/auth"
	"github.com/ricardoalt1515/vigia/internal/core"
	"github.com/ricardoalt1515/vigia/internal/evaluation"
	"github.com/ricardoalt1515/vigia/internal/ledger"
)

// Interaction is the API DTO for GET /v1/interactions. Outcome, Reason,
// RequiresHITL, and ThreatFlagged are all nil when the interaction has not
// yet been evaluated — the API never fabricates a PASS/BLOCK outcome or a
// false flag for an unevaluated interaction.
type Interaction struct {
	ID            string    `json:"id"`
	OccurredAt    time.Time `json:"occurred_at"`
	Channel       string    `json:"channel"`
	Direction     string    `json:"direction"`
	Outcome       *string   `json:"outcome"`
	Reason        *string   `json:"reason"`
	RequiresHITL  *bool     `json:"requires_hitl"`
	ThreatFlagged *bool     `json:"threat_flagged"`
	// PolicyBundleVersion is nil when the interaction has not been
	// evaluated (no evaluation row), and the stored (possibly empty-string
	// sentinel) value when it has — the empty string is a real, distinct
	// value from nil, never coerced to either extreme (issue #6).
	PolicyBundleVersion *string `json:"policy_bundle_version"`
}

type InteractionReader interface {
	ListInteractions(ctx context.Context, tenantID string) ([]Interaction, error)
}

// SummaryReader returns the tenant's current out-of-hours (BLOCK) evaluation
// count, computed by a SQL aggregate — never by counting the interactions
// list in application code.
type SummaryReader interface {
	CountOutOfHours(ctx context.Context, tenantID string) (int64, error)
}

// ErrEvidenceNotFound is returned by EvidenceReader when no exportable
// evidence package exists for the requested interaction: the interaction
// does not exist, belongs to another tenant, has not been evaluated, or
// predates the evidence ledger (issue #3 Decision 7: no backfill). All four
// cases collapse to the same generic 404 — the handler never distinguishes
// them in the response, so nothing about other tenants' data (or which case
// occurred) leaks.
var ErrEvidenceNotFound = errors.New("httpapi: evidence not found")

// EvidenceReader loads a self-contained, hash-verifiable evidence package
// for one interaction, scoped to the caller's tenant.
type EvidenceReader interface {
	GetEvidencePackage(ctx context.Context, tenantID, interactionID string) (ledger.Package, error)
}

// DespachoRate is one row of the by-despacho violation-rate ranking.
// DespachoID is nil for the synthetic "unattributed" bucket, which covers
// interactions with no despacho FK set rather than silently dropping them
// or folding them into a named despacho. ViolationRate is
// Violations/Total, guarded against a zero Total by DashboardReader.
type DespachoRate struct {
	DespachoID    *string `json:"despacho_id"`
	DespachoName  string  `json:"despacho_name"`
	Total         int64   `json:"total"`
	Violations    int64   `json:"violations"`
	ViolationRate float64 `json:"violation_rate"`
}

// CauseCount is one row of the by-REDECO-cause breakdown. Violations counts
// only outcome = 'fail' rows; Warnings is a separate count of outcome =
// 'warn' rows (non-zero in practice only for MX-REDECO-03) so warn-level
// activity is visible without inflating Violations.
type CauseCount struct {
	RuleCode   string `json:"rule_code"`
	Violations int64  `json:"violations"`
	Warnings   int64  `json:"warnings"`
}

// DashboardReader returns the two tenant-scoped compliance dashboards, both
// computed as SQL aggregates (never client-side), following the same
// tenantdb.WithTenantTx + RLS seam as SummaryReader.CountOutOfHours.
type DashboardReader interface {
	ByDespacho(ctx context.Context, tenantID string) ([]DespachoRate, error)
	ByCause(ctx context.Context, tenantID string) ([]CauseCount, error)
}

// ReEvaluator reruns the wired detectors/judge against a historical
// interaction and stamps a caller-supplied historical PolicyBundle version
// onto the (unpersisted) result (issue #6). Implementations resolve the
// interaction's owning tenant internally — the handler independently
// verifies the returned core.Evaluation.TenantID matches the authenticated
// caller before responding, so a result belonging to another tenant can
// never leak.
type ReEvaluator interface {
	ReEvaluateInteraction(ctx context.Context, tenantID, interactionID, policyBundleID string) (core.Evaluation, error)
}

type Server struct {
	authenticator *auth.Authenticator
	interactions  InteractionReader
	summary       SummaryReader
	evidence      EvidenceReader
	reevaluator   ReEvaluator
	dashboards    DashboardReader
	mux           *http.ServeMux
}

func NewServer(authenticator *auth.Authenticator, interactions InteractionReader, summary SummaryReader, evidence EvidenceReader, reevaluator ReEvaluator, dashboards DashboardReader) *Server {
	s := &Server{
		authenticator: authenticator,
		interactions:  interactions,
		summary:       summary,
		evidence:      evidence,
		reevaluator:   reevaluator,
		dashboards:    dashboards,
		mux:           http.NewServeMux(),
	}
	s.mux.HandleFunc("GET /v1/interactions", s.handleGetInteractions)
	s.mux.HandleFunc("GET /v1/summary", s.handleGetSummary)
	s.mux.HandleFunc("GET /v1/interactions/{id}/evidence", s.handleGetEvidence)
	s.mux.HandleFunc("POST /v1/interactions/{id}/reevaluate", s.handleReEvaluate)
	s.mux.HandleFunc("GET /v1/dashboards/by-despacho", s.handleGetDashboardByDespacho)
	s.mux.HandleFunc("GET /v1/dashboards/by-cause", s.handleGetDashboardByCause)
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

type interactionsResponse struct {
	Interactions []Interaction `json:"interactions"`
}

func (s *Server) handleGetInteractions(w http.ResponseWriter, r *http.Request) {
	tenant, err := s.authenticator.Authenticate(r.Context(), r.Header.Get("Authorization"))
	if err != nil {
		if errors.Is(err, auth.ErrUnauthorized) {
			writeError(w, http.StatusUnauthorized)
			return
		}
		writeError(w, http.StatusInternalServerError)
		return
	}

	items, err := s.interactions.ListInteractions(r.Context(), tenant.TenantID)
	if err != nil {
		writeError(w, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(interactionsResponse{Interactions: items}); err != nil {
		return
	}
}

type summaryResponse struct {
	OutOfHoursCount int64 `json:"out_of_hours_count"`
}

func (s *Server) handleGetSummary(w http.ResponseWriter, r *http.Request) {
	tenant, err := s.authenticator.Authenticate(r.Context(), r.Header.Get("Authorization"))
	if err != nil {
		if errors.Is(err, auth.ErrUnauthorized) {
			writeError(w, http.StatusUnauthorized)
			return
		}
		writeError(w, http.StatusInternalServerError)
		return
	}

	count, err := s.summary.CountOutOfHours(r.Context(), tenant.TenantID)
	if err != nil {
		writeError(w, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(summaryResponse{OutOfHoursCount: count}); err != nil {
		return
	}
}

func (s *Server) handleGetEvidence(w http.ResponseWriter, r *http.Request) {
	tenant, err := s.authenticator.Authenticate(r.Context(), r.Header.Get("Authorization"))
	if err != nil {
		if errors.Is(err, auth.ErrUnauthorized) {
			writeError(w, http.StatusUnauthorized)
			return
		}
		writeError(w, http.StatusInternalServerError)
		return
	}

	id := r.PathValue("id")
	pkg, err := s.evidence.GetEvidencePackage(r.Context(), tenant.TenantID, id)
	if err != nil {
		if errors.Is(err, ErrEvidenceNotFound) {
			writeError(w, http.StatusNotFound)
			return
		}
		writeError(w, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(pkg); err != nil {
		return
	}
}

// reevaluateRequest is the POST /v1/interactions/{id}/reevaluate request
// body: the historical bundle to rerun the evaluation against.
type reevaluateRequest struct {
	PolicyBundleID string `json:"policy_bundle_id"`
}

// reevaluateResponse is the stamped, unpersisted evaluation result.
// PolicyBundleID is nil only in the theoretical case a ReEvaluator returns
// no bundle id — in practice a successful ReEvaluateInteraction call always
// stamps the caller-supplied bundle id.
type reevaluateResponse struct {
	OverallOutcome      string  `json:"overall_outcome"`
	PolicyBundleVersion string  `json:"policy_bundle_version"`
	PolicyBundleID      *string `json:"policy_bundle_id"`
}

func (s *Server) handleReEvaluate(w http.ResponseWriter, r *http.Request) {
	tenant, err := s.authenticator.Authenticate(r.Context(), r.Header.Get("Authorization"))
	if err != nil {
		if errors.Is(err, auth.ErrUnauthorized) {
			writeError(w, http.StatusUnauthorized)
			return
		}
		writeError(w, http.StatusInternalServerError)
		return
	}

	var req reevaluateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest)
		return
	}

	id := r.PathValue("id")
	got, err := s.reevaluator.ReEvaluateInteraction(r.Context(), tenant.TenantID, id, req.PolicyBundleID)
	if err != nil {
		if errors.Is(err, evaluation.ErrInteractionNotFound) || errors.Is(err, evaluation.ErrPolicyBundleNotFound) {
			writeError(w, http.StatusNotFound)
			return
		}
		writeError(w, http.StatusInternalServerError)
		return
	}

	// The tenant scoping already happened before any bundle/detector/judge
	// work ran, inside ReEvaluateInteraction's tenant-scoped interaction
	// lookup (a foreign-tenant interactionID resolves to
	// ErrInteractionNotFound, handled above). This is a defense-in-depth
	// check on the returned result only, mirroring handleGetEvidence's
	// tenant-scoped lookup precedent — it should never trigger in practice.
	if string(got.TenantID) != tenant.TenantID {
		writeError(w, http.StatusNotFound)
		return
	}

	var policyBundleID *string
	if got.PolicyBundleID != nil {
		id := string(*got.PolicyBundleID)
		policyBundleID = &id
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(reevaluateResponse{
		OverallOutcome:      got.OverallOutcome,
		PolicyBundleVersion: got.PolicyBundleVersion,
		PolicyBundleID:      policyBundleID,
	}); err != nil {
		return
	}
}

// despachoRatesResponse wraps the by-despacho ranking in a named key,
// following the same array-wrapping convention as interactionsResponse.
type despachoRatesResponse struct {
	Despachos []DespachoRate `json:"despachos"`
}

func (s *Server) handleGetDashboardByDespacho(w http.ResponseWriter, r *http.Request) {
	tenant, err := s.authenticator.Authenticate(r.Context(), r.Header.Get("Authorization"))
	if err != nil {
		if errors.Is(err, auth.ErrUnauthorized) {
			writeError(w, http.StatusUnauthorized)
			return
		}
		writeError(w, http.StatusInternalServerError)
		return
	}

	rates, err := s.dashboards.ByDespacho(r.Context(), tenant.TenantID)
	if err != nil {
		writeError(w, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(despachoRatesResponse{Despachos: rates}); err != nil {
		return
	}
}

// causeCountsResponse wraps the by-cause breakdown in a named key.
type causeCountsResponse struct {
	Causes []CauseCount `json:"causes"`
}

func (s *Server) handleGetDashboardByCause(w http.ResponseWriter, r *http.Request) {
	tenant, err := s.authenticator.Authenticate(r.Context(), r.Header.Get("Authorization"))
	if err != nil {
		if errors.Is(err, auth.ErrUnauthorized) {
			writeError(w, http.StatusUnauthorized)
			return
		}
		writeError(w, http.StatusInternalServerError)
		return
	}

	counts, err := s.dashboards.ByCause(r.Context(), tenant.TenantID)
	if err != nil {
		writeError(w, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(causeCountsResponse{Causes: counts}); err != nil {
		return
	}
}

func writeError(w http.ResponseWriter, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": http.StatusText(status)})
}
