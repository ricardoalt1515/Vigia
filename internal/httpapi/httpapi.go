package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/ricardoalt1515/vigia/internal/auth"
)

type Interaction struct {
	ID         string    `json:"id"`
	OccurredAt time.Time `json:"occurred_at"`
	Channel    string    `json:"channel"`
	Direction  string    `json:"direction"`
}

type InteractionReader interface {
	ListInteractions(ctx context.Context, tenantID string) ([]Interaction, error)
}

type Server struct {
	authenticator *auth.Authenticator
	interactions  InteractionReader
	mux           *http.ServeMux
}

func NewServer(authenticator *auth.Authenticator, interactions InteractionReader) *Server {
	s := &Server{
		authenticator: authenticator,
		interactions:  interactions,
		mux:           http.NewServeMux(),
	}
	s.mux.HandleFunc("GET /v1/interactions", s.handleGetInteractions)
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

func writeError(w http.ResponseWriter, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": http.StatusText(status)})
}
