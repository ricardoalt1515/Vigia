package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/ricardoalt1515/vigia/internal/auth"
	"github.com/ricardoalt1515/vigia/internal/core"
	"github.com/ricardoalt1515/vigia/internal/evaluation"
	"github.com/ricardoalt1515/vigia/internal/harness"
	"github.com/ricardoalt1515/vigia/internal/harness/outboundgate"
	"github.com/ricardoalt1515/vigia/internal/ledger"
	"github.com/ricardoalt1515/vigia/internal/orchestrator"
	"github.com/ricardoalt1515/vigia/internal/outbound"
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

type ComplaintWorkflow interface {
	CreateComplaintCase(ctx context.Context, in orchestrator.CreateComplaintCaseInput) (orchestrator.ComplaintCase, error)
	CreateHumanReview(ctx context.Context, in CreateHumanReviewInput) (HumanReview, error)
	ListBusinessDayHolidays(ctx context.Context, version string) ([]orchestrator.HolidayRow, error)
}

type RedecoMonthlyReporter interface {
	GenerateRedecoMonthlyReport(ctx context.Context, tenantID string, period orchestrator.RedecoReportPeriod) (orchestrator.RedecoMonthlyReport, error)
}

type CampaignPreflightRunner interface {
	Run(ctx context.Context, campaign outbound.CampaignArtifact) (outbound.PreflightBrief, error)
}

type OutboundDecisionRunner interface {
	Decide(ctx context.Context, req outbound.DecisionRequest) (outbound.Decision, error)
}

type CreateHumanReviewInput = orchestrator.CreateHumanReviewInput

type HumanReview = orchestrator.HumanReview

var ErrComplaintReviewConflict = errors.New("httpapi: complaint review conflict")

const (
	defaultComplaintCalendarVersion = "mx-lft-art-74-2026a"
	complaintSLABusinessDays        = 10
)

type Server struct {
	authenticator *auth.Authenticator
	interactions  InteractionReader
	summary       SummaryReader
	evidence      EvidenceReader
	reevaluator   ReEvaluator
	dashboards    DashboardReader
	complaints    ComplaintWorkflow
	reports       RedecoMonthlyReporter
	preflight     CampaignPreflightRunner
	outbound      OutboundDecisionRunner
	outboundRec   outbound.DecisionRecorder
	now           func() time.Time
	mux           *http.ServeMux
}

func NewServer(authenticator *auth.Authenticator, interactions InteractionReader, summary SummaryReader, evidence EvidenceReader, reevaluator ReEvaluator, dashboards DashboardReader, complaints ComplaintWorkflow, reports ...RedecoMonthlyReporter) *Server {
	var reportService RedecoMonthlyReporter
	if len(reports) > 0 {
		reportService = reports[0]
	}
	s := &Server{
		authenticator: authenticator,
		interactions:  interactions,
		summary:       summary,
		evidence:      evidence,
		reevaluator:   reevaluator,
		dashboards:    dashboards,
		complaints:    complaints,
		reports:       reportService,
		now:           time.Now,
		mux:           http.NewServeMux(),
	}
	s.mux.HandleFunc("GET /v1/interactions", s.handleGetInteractions)
	s.mux.HandleFunc("GET /v1/summary", s.handleGetSummary)
	s.mux.HandleFunc("GET /v1/interactions/{id}/evidence", s.handleGetEvidence)
	s.mux.HandleFunc("POST /v1/interactions/{id}/reevaluate", s.handleReEvaluate)
	s.mux.HandleFunc("GET /v1/dashboards/by-despacho", s.handleGetDashboardByDespacho)
	s.mux.HandleFunc("GET /v1/dashboards/by-cause", s.handleGetDashboardByCause)
	s.mux.HandleFunc("GET /v1/reports/redeco-monthly.csv", s.handleGetRedecoMonthlyReport)
	s.mux.HandleFunc("POST /v1/complaints", s.handleCreateComplaint)
	s.mux.HandleFunc("POST /v1/complaints/{id}/reviews", s.handleCreateComplaintReview)
	s.mux.HandleFunc("POST /v1/campaigns/preflight", s.handleCampaignPreflight)
	s.mux.HandleFunc("POST /v1/outbound/guardrails/decide", s.handleOutboundGuardrailDecision)
	return s
}

func (s *Server) SetCampaignPreflight(preflight CampaignPreflightRunner) {
	s.preflight = preflight
}

func (s *Server) SetOutboundGuardrails(decider OutboundDecisionRunner, recorder outbound.DecisionRecorder) {
	s.outbound = decider
	s.outboundRec = recorder
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

type outboundGuardrailDecisionRequest map[string]any

type outboundGuardrailDecisionResponse struct {
	Decision string         `json:"decision"`
	Reason   string         `json:"reason"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

func (s *Server) handleOutboundGuardrailDecision(w http.ResponseWriter, r *http.Request) {
	tenant, err := s.authenticator.Authenticate(r.Context(), r.Header.Get("Authorization"))
	if err != nil {
		if errors.Is(err, auth.ErrUnauthorized) {
			writeError(w, http.StatusUnauthorized)
			return
		}
		writeError(w, http.StatusInternalServerError)
		return
	}
	if s.outbound == nil {
		writeError(w, http.StatusInternalServerError)
		return
	}
	var input outboundGuardrailDecisionRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest)
		return
	}
	gate := outboundgate.NewGate(outboundgate.Config{
		TenantID:  tenant.TenantID,
		ActorID:   tenant.KeyID,
		Decider:   s.outbound,
		Recorder:  s.outboundRec,
		RequestID: stringValueFromMap(input, "proposal_id"),
	})
	decision := gate.Decide(r.Context(), harness.ToolCall{Name: "send_outbound_utterance", Input: map[string]any(input)})
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(outboundGuardrailDecisionResponse{Decision: string(decision.Kind), Reason: decision.Reason, Metadata: decision.Metadata}); err != nil {
		return
	}
}

func stringValueFromMap(input map[string]any, key string) string {
	value, _ := input[key].(string)
	return value
}

type campaignPreflightRequest struct {
	CampaignID string                     `json:"campaign_id"`
	Name       string                     `json:"name"`
	ActorID    string                     `json:"actor_id"`
	Audience   []campaignRecipientRequest `json:"audience"`
	Steps      []campaignStepRequest      `json:"steps"`
	Schedule   campaignScheduleRequest    `json:"schedule"`
}

type campaignRecipientRequest struct {
	RecipientRef string   `json:"recipient_ref"`
	DebtorID     string   `json:"debtor_id"`
	Relationship string   `json:"relationship"`
	ChannelRefs  []string `json:"channel_refs"`
	Timezone     string   `json:"timezone"`
}

type campaignStepRequest struct {
	StepID            string `json:"step_id"`
	TemplateID        string `json:"template_id"`
	Channel           string `json:"channel"`
	TextTemplate      string `json:"text_template"`
	SendOffsetMinutes int64  `json:"send_offset_minutes"`
	PaymentTarget     string `json:"payment_target"`
}

type campaignScheduleRequest struct {
	StartsAt string `json:"starts_at"`
	Timezone string `json:"timezone"`
}

func (s *Server) handleCampaignPreflight(w http.ResponseWriter, r *http.Request) {
	tenant, err := s.authenticator.Authenticate(r.Context(), r.Header.Get("Authorization"))
	if err != nil {
		if errors.Is(err, auth.ErrUnauthorized) {
			writeError(w, http.StatusUnauthorized)
			return
		}
		writeError(w, http.StatusInternalServerError)
		return
	}
	if s.preflight == nil {
		writeError(w, http.StatusInternalServerError)
		return
	}
	var req campaignPreflightRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest)
		return
	}
	campaign, err := req.toDomain(tenant)
	if err != nil {
		writeError(w, http.StatusBadRequest)
		return
	}
	brief, err := s.preflight.Run(r.Context(), campaign)
	if err != nil {
		writeError(w, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(brief); err != nil {
		return
	}
}

func (r campaignPreflightRequest) toDomain(tenant auth.TenantContext) (outbound.CampaignArtifact, error) {
	startsAt, err := time.Parse(time.RFC3339, r.Schedule.StartsAt)
	if err != nil {
		return outbound.CampaignArtifact{}, err
	}
	campaign := outbound.CampaignArtifact{
		CampaignID: r.CampaignID,
		Name:       r.Name,
		TenantID:   tenant.TenantID,
		ActorID:    tenant.KeyID,
		Audience:   make([]outbound.CampaignRecipient, 0, len(r.Audience)),
		Steps:      make([]outbound.CampaignStep, 0, len(r.Steps)),
		Schedule:   outbound.CampaignSchedule{StartsAt: startsAt, Timezone: r.Schedule.Timezone},
	}
	for _, recipient := range r.Audience {
		campaign.Audience = append(campaign.Audience, outbound.CampaignRecipient{
			RecipientRef: recipient.RecipientRef,
			DebtorID:     recipient.DebtorID,
			Relationship: recipient.Relationship,
			ChannelRefs:  recipient.ChannelRefs,
			Timezone:     recipient.Timezone,
		})
	}
	for _, step := range r.Steps {
		campaign.Steps = append(campaign.Steps, outbound.CampaignStep{
			StepID:        step.StepID,
			TemplateID:    step.TemplateID,
			Channel:       core.InteractionChannel(step.Channel),
			TextTemplate:  step.TextTemplate,
			SendOffset:    time.Duration(step.SendOffsetMinutes) * time.Minute,
			PaymentTarget: step.PaymentTarget,
		})
	}
	return campaign, nil
}

func (s *Server) handleGetRedecoMonthlyReport(w http.ResponseWriter, r *http.Request) {
	tenant, err := s.authenticator.Authenticate(r.Context(), r.Header.Get("Authorization"))
	if err != nil {
		if errors.Is(err, auth.ErrUnauthorized) {
			writeError(w, http.StatusUnauthorized)
			return
		}
		writeError(w, http.StatusInternalServerError)
		return
	}
	if s.reports == nil {
		writeError(w, http.StatusInternalServerError)
		return
	}
	year, err := strconv.Atoi(r.URL.Query().Get("year"))
	if err != nil {
		writeError(w, http.StatusBadRequest)
		return
	}
	month, err := strconv.Atoi(r.URL.Query().Get("month"))
	if err != nil {
		writeError(w, http.StatusBadRequest)
		return
	}
	period, err := orchestrator.NewRedecoReportPeriod(year, time.Month(month))
	if err != nil {
		writeError(w, http.StatusBadRequest)
		return
	}
	report, err := s.reports.GenerateRedecoMonthlyReport(r.Context(), tenant.TenantID, period)
	if err != nil {
		writeError(w, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=redeco-monthly-"+strconv.Itoa(year)+"-"+strconv.Itoa(month)+".csv")
	http.ServeContent(w, r, "redeco-monthly.csv", time.Time{}, bytes.NewReader(report.CSV))
}

type createComplaintRequest struct {
	IdempotencyKey string `json:"idempotency_key"`
	InteractionID  string `json:"interaction_id"`
	RedecoCause    string `json:"redeco_cause"`
}

type complaintCaseResponse struct {
	ID              string     `json:"id"`
	TenantID        string     `json:"tenant_id"`
	InteractionID   string     `json:"interaction_id"`
	RedecoCause     string     `json:"redeco_cause"`
	State           string     `json:"state"`
	OpenedAt        time.Time  `json:"opened_at"`
	SLADueAt        time.Time  `json:"sla_due_at"`
	CalendarVersion string     `json:"calendar_version"`
	ReviewExpiresAt *time.Time `json:"review_expires_at,omitempty"`
	ResolvedAt      *time.Time `json:"resolved_at,omitempty"`
	IdempotencyKey  string     `json:"idempotency_key"`
}

type createComplaintReviewRequest struct {
	Decision string `json:"decision"`
	Reviewer string `json:"reviewer"`
	Notes    string `json:"notes"`
}

type complaintReviewResponse struct {
	Review HumanReview `json:"review"`
}

func (s *Server) handleCreateComplaint(w http.ResponseWriter, r *http.Request) {
	tenant, err := s.authenticator.Authenticate(r.Context(), r.Header.Get("Authorization"))
	if err != nil {
		if errors.Is(err, auth.ErrUnauthorized) {
			writeError(w, http.StatusUnauthorized)
			return
		}
		writeError(w, http.StatusInternalServerError)
		return
	}
	if s.complaints == nil {
		writeError(w, http.StatusInternalServerError)
		return
	}

	var req createComplaintRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest)
		return
	}
	idempotencyKey := strings.TrimSpace(req.IdempotencyKey)
	headerIdempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if idempotencyKey != "" && headerIdempotencyKey != "" && idempotencyKey != headerIdempotencyKey {
		writeError(w, http.StatusBadRequest)
		return
	}
	if idempotencyKey == "" {
		idempotencyKey = headerIdempotencyKey
	}
	if idempotencyKey == "" || req.InteractionID == "" || req.RedecoCause == "" {
		writeError(w, http.StatusBadRequest)
		return
	}

	openedAt := s.now().UTC().Truncate(time.Microsecond)
	holidays, err := s.complaints.ListBusinessDayHolidays(r.Context(), defaultComplaintCalendarVersion)
	if err != nil {
		writeError(w, http.StatusInternalServerError)
		return
	}
	caseRow, err := s.complaints.CreateComplaintCase(r.Context(), orchestrator.CreateComplaintCaseInput{
		TenantID:        tenant.TenantID,
		InteractionID:   req.InteractionID,
		RedecoCause:     req.RedecoCause,
		OpenedAt:        openedAt,
		SLADueAt:        orchestrator.AddBusinessDays(openedAt, complaintSLABusinessDays, orchestrator.LoadCalendar(defaultComplaintCalendarVersion, holidays)),
		CalendarVersion: defaultComplaintCalendarVersion,
		IdempotencyKey:  idempotencyKey,
	})
	if err != nil {
		if errors.Is(err, orchestrator.ErrComplaintIdempotencyConflict) {
			writeError(w, http.StatusConflict)
			return
		}
		writeError(w, http.StatusInternalServerError)
		return
	}

	status := http.StatusOK
	if caseRow.Created {
		status = http.StatusCreated
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(complaintCaseFromDomain(caseRow))
}

func (s *Server) handleCreateComplaintReview(w http.ResponseWriter, r *http.Request) {
	tenant, err := s.authenticator.Authenticate(r.Context(), r.Header.Get("Authorization"))
	if err != nil {
		if errors.Is(err, auth.ErrUnauthorized) {
			writeError(w, http.StatusUnauthorized)
			return
		}
		writeError(w, http.StatusInternalServerError)
		return
	}
	if s.complaints == nil {
		writeError(w, http.StatusInternalServerError)
		return
	}

	var req createComplaintReviewRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest)
		return
	}
	if (req.Decision != string(orchestrator.TransitionApprove) && req.Decision != string(orchestrator.TransitionOverride)) || req.Reviewer == "" {
		writeError(w, http.StatusBadRequest)
		return
	}

	review, err := s.complaints.CreateHumanReview(r.Context(), CreateHumanReviewInput{
		TenantID:        tenant.TenantID,
		ComplaintCaseID: r.PathValue("id"),
		Decision:        req.Decision,
		Reviewer:        req.Reviewer,
		Notes:           req.Notes,
	})
	if err != nil {
		if errors.Is(err, ErrComplaintReviewConflict) || errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusConflict)
			return
		}
		writeError(w, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(complaintReviewResponse{Review: review})
}

func complaintCaseFromDomain(item orchestrator.ComplaintCase) complaintCaseResponse {
	return complaintCaseResponse{ID: item.ID, TenantID: item.TenantID, InteractionID: item.InteractionID, RedecoCause: item.RedecoCause, State: item.State, OpenedAt: item.OpenedAt, SLADueAt: item.SLADueAt, CalendarVersion: item.CalendarVersion, ReviewExpiresAt: item.ReviewExpiresAt, ResolvedAt: item.ResolvedAt, IdempotencyKey: item.IdempotencyKey}
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
