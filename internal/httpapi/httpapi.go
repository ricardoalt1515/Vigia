package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/ricardoalt1515/vigia/internal/auth"
)

// Interaction is the API DTO for GET /v1/interactions. Outcome and Reason
// are nil when the interaction has not yet been evaluated — the API never
// fabricates a PASS/BLOCK outcome for an unevaluated interaction.
type Interaction struct {
	ID         string    `json:"id"`
	OccurredAt time.Time `json:"occurred_at"`
	Channel    string    `json:"channel"`
	Direction  string    `json:"direction"`
	Outcome    *string   `json:"outcome"`
	Reason     *string   `json:"reason"`
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

type Server struct {
	authenticator *auth.Authenticator
	interactions  InteractionReader
	summary       SummaryReader
	mux           *http.ServeMux
}

func NewServer(authenticator *auth.Authenticator, interactions InteractionReader, summary SummaryReader) *Server {
	s := &Server{
		authenticator: authenticator,
		interactions:  interactions,
		summary:       summary,
		mux:           http.NewServeMux(),
	}
	s.mux.HandleFunc("GET /v1/interactions", s.handleGetInteractions)
	s.mux.HandleFunc("GET /v1/summary", s.handleGetSummary)
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

func writeError(w http.ResponseWriter, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": http.StatusText(status)})
}
