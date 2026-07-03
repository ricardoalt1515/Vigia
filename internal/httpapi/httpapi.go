package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/ricardoalt1515/vigia/internal/auth"
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

type Server struct {
	authenticator *auth.Authenticator
	interactions  InteractionReader
	summary       SummaryReader
	evidence      EvidenceReader
	mux           *http.ServeMux
}

func NewServer(authenticator *auth.Authenticator, interactions InteractionReader, summary SummaryReader, evidence EvidenceReader) *Server {
	s := &Server{
		authenticator: authenticator,
		interactions:  interactions,
		summary:       summary,
		evidence:      evidence,
		mux:           http.NewServeMux(),
	}
	s.mux.HandleFunc("GET /v1/interactions", s.handleGetInteractions)
	s.mux.HandleFunc("GET /v1/summary", s.handleGetSummary)
	s.mux.HandleFunc("GET /v1/interactions/{id}/evidence", s.handleGetEvidence)
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

func writeError(w http.ResponseWriter, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": http.StatusText(status)})
}
