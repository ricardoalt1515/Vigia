package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/ricardoalt1515/vigia/internal/core"
	vigiaDB "github.com/ricardoalt1515/vigia/internal/db"
	"github.com/ricardoalt1515/vigia/internal/detection"
	"github.com/ricardoalt1515/vigia/internal/evaluation"
	"github.com/ricardoalt1515/vigia/internal/judge"
)

// SeedQuerier is the minimal read/write port SeedDevData needs.
// It is a strict subset of internal/db.Querier so unit tests can supply an in-memory fake.
type SeedQuerier interface {
	GetTenantBySlug(ctx context.Context, slug string) (vigiaDB.Tenant, error)
	CreateTenant(ctx context.Context, arg vigiaDB.CreateTenantParams) (vigiaDB.Tenant, error)
	ListDebtorsByTenant(ctx context.Context, tenantID pgtype.UUID) ([]vigiaDB.ListDebtorsByTenantRow, error)
	CreateDebtor(ctx context.Context, arg vigiaDB.CreateDebtorParams) (vigiaDB.CreateDebtorRow, error)
	ListInteractionEventsByTenant(ctx context.Context, tenantID pgtype.UUID) ([]vigiaDB.ListInteractionEventsByTenantRow, error)
	CreateInteractionEvent(ctx context.Context, arg vigiaDB.CreateInteractionEventParams) (vigiaDB.CreateInteractionEventRow, error)
	GetEvaluationByInteractionEventID(ctx context.Context, arg vigiaDB.GetEvaluationByInteractionEventIDParams) (vigiaDB.Evaluation, error)
	CreateInteractionTranscript(ctx context.Context, arg vigiaDB.CreateInteractionTranscriptParams) (vigiaDB.InteractionTranscript, error)
	GetInteractionTranscriptByInteraction(ctx context.Context, arg vigiaDB.GetInteractionTranscriptByInteractionParams) (vigiaDB.InteractionTranscript, error)
}

// seedUtterance is the JSON shape stored in interaction_transcripts.utterances.
type seedUtterance struct {
	Speaker string `json:"speaker"`
	Text    string `json:"text"`
}

// KeyIssuer mints a tenant API key and returns the plaintext key once.
type KeyIssuer interface {
	IssueTenantAPIKey(ctx context.Context, params IssueTenantAPIKeyParams) (IssuedTenantAPIKey, error)
}

// Evaluator runs detectors over a seeded interaction and persists the
// resulting evaluation, so the API/console have evaluations to surface for
// seed data (spec "Seed Provides Timezone and an Out-of-Hours Demo
// Interaction").
type Evaluator interface {
	EvaluateInteraction(ctx context.Context, in evaluation.EvaluateInteractionInput) (core.Evaluation, error)
}

// DevDataParams controls the fixture data created by SeedDevData.
// All fields have sensible defaults so the binary works with zero flags.
type DevDataParams struct {
	Slug       string // tenant slug (default: demo)
	Name       string // tenant display name
	DebtorRef  string // debtor external_ref (default: debtor-001)
	DebtorName string // debtor display_name
	Label      string // API key label (default: local-dev)
	Timezone   string // debtor IANA timezone (default: America/Mexico_City)
}

// defaultDebtorTimezone is the demo debtor's IANA timezone when DevDataParams.Timezone
// is left unset. It intentionally has no silent code-level fallback beyond this single,
// explicit default (Decision 2 — no lingering timezone default at the DB layer).
const defaultDebtorTimezone = "America/Mexico_City"

// defaultDebtorAgeYears is the demo debtor's age (issue #7): an adult, well
// above both the legal-majority and elderly protected-population
// thresholds, so the seeded debtor's own date of birth never triggers
// MX-REDECO-07 on the compliant fixtures. Only the protected-population
// violation demo fixture overrides this via contactedPartyDOBOverride.
const defaultDebtorAgeYears = 35

// DevDataCounts records which entities were newly created vs. already present.
type DevDataCounts struct {
	TenantCreated       bool
	DebtorCreated       bool
	InteractionsCreated int
}

// DevDataResult holds the IDs of all upserted entities and the freshly minted plaintext key.
type DevDataResult struct {
	TenantID       string
	DebtorID       string
	InteractionIDs []string
	PlaintextKey   string
	Created        DevDataCounts
}

// interactionFixture describes one seeded interaction event. Utterances is
// nil for fixtures with no transcript content (the original #2/#3
// fixtures); when non-empty, SeedDevData persists it via
// interaction_transcripts so the judge has real stored content to read
// (spec "Seed Provides Threatening and Neutral Synthetic Transcripts").
type interactionFixture struct {
	channel       string
	direction     string
	transcriptRef string
	occurredAt    time.Time
	utterances    []seedUtterance

	// Detector-input snapshot fields (issue #7: MX-REDECO-06/07/10/11).
	// compliantFixture defaults these to values that PASS all four new
	// hard-block detectors; a fixture demonstrating one detector's
	// violation overrides only the field that detector reads, so its
	// BLOCK is attributable to exactly one new detector at a time.
	contactPartyRelationship string
	// contactedPartyDOBOverride, when non-nil, replaces the seeded
	// debtor's own date of birth for this one interaction (used only by
	// the protected-population violation demo fixture, to simulate a
	// protected minor contacted party without changing the debtor's own
	// record). nil means "snapshot the debtor's date of birth", mirroring
	// the real contacted_party_dob ingest-time snapshot.
	contactedPartyDOBOverride *time.Time
	authorizedChannels        []string
	paymentRecipient          string
	disclosureProvided        *bool
}

// compliantFixture builds an interactionFixture whose detector-input
// snapshot fields PASS all four new hard-block detectors (MX-REDECO-06:
// relationship debtor; MX-REDECO-07: debtor's own — adult — date of birth,
// via contactedPartyDOBOverride left nil; MX-REDECO-11: this fixture's own
// channel is authorized; MX-REDECO-10: payment routed to the creditor).
func compliantFixture(channel, direction, transcriptRef string, occurredAt time.Time, utterances []seedUtterance) interactionFixture {
	return interactionFixture{
		channel:                  channel,
		direction:                direction,
		transcriptRef:            transcriptRef,
		occurredAt:               occurredAt,
		utterances:               utterances,
		contactPartyRelationship: "debtor",
		authorizedChannels:       []string{channel},
		paymentRecipient:         "creditor",
		disclosureProvided:       boolPtr(true),
	}
}

func boolPtr(b bool) *bool { return &b }

// resolvedContactedPartyDOB returns f.contactedPartyDOBOverride when set,
// otherwise falls back to the debtor's own snapshotted date of birth —
// mirroring the real ingest-time contacted_party_dob snapshot, which
// normally mirrors the debtor's own record unless a specific interaction's
// contacted party differs (used here only by the protected-population
// violation demo fixture).
func (f interactionFixture) resolvedContactedPartyDOB(debtorDOB *time.Time) *time.Time {
	if f.contactedPartyDOBOverride != nil {
		return f.contactedPartyDOBOverride
	}
	return debtorDOB
}

// toDetectionInteraction builds the detection.Interaction the deterministic
// detectors evaluate, snapshotting this fixture's channel and
// detector-input fields (issue #7) alongside the pre-existing
// OccurredAt/DebtorTimezone fields (issue #2/#3).
func (f interactionFixture) toDetectionInteraction(debtorTimezone string, debtorDOB *time.Time) detection.Interaction {
	return detection.Interaction{
		OccurredAt:               f.occurredAt,
		DebtorTimezone:           debtorTimezone,
		Channel:                  core.InteractionChannel(f.channel),
		ContactPartyRelationship: f.contactPartyRelationship,
		ContactedPartyDOB:        f.resolvedContactedPartyDOB(debtorDOB),
		AuthorizedChannels:       f.authorizedChannels,
		PaymentRecipient:         f.paymentRecipient,
		DisclosureProvided:       f.disclosureProvided,
	}
}

// threateningTranscriptUtterances is Spanish, MX-REDECO-05-marker synthetic
// content: it contains a threat-keyword FakeJudge recognizes ("vamos a tu
// casa"), so the seeded interaction demonstrates a HARD BLOCK + requires_hitl
// end to end with the default (fake) judge.
func threateningTranscriptUtterances() []seedUtterance {
	return []seedUtterance{
		{Speaker: "agent", Text: "Buenos días, le hablamos de Vigía Cobranza por su adeudo pendiente."},
		{Speaker: "debtor", Text: "No tengo dinero en este momento, lo siento."},
		{Speaker: "agent", Text: "Si no pagas hoy mismo, vamos a tu casa y vas a tener problemas serios."},
	}
}

// neutralTranscriptUtterances is Spanish synthetic content with no
// threat-keyword markers, so the seeded interaction demonstrates a PASS
// judge outcome end to end.
func neutralTranscriptUtterances() []seedUtterance {
	return []seedUtterance{
		{Speaker: "agent", Text: "Buenos días, le hablamos de Vigía Cobranza para recordarle su pago."},
		{Speaker: "debtor", Text: "Sí, puedo pagar la próxima semana."},
		{Speaker: "agent", Text: "Perfecto, quedamos así. Que tenga buen día."},
	}
}

// devDataFixtures returns the canonical es-MX demo interaction fixtures,
// including one interaction whose debtor-local wall-clock time falls
// outside the contact-hours window [08:00:00, 21:00:00) so the
// out-of-hours outcome and console tile render with dev data (spec
// "Seed Provides Timezone and an Out-of-Hours Demo Interaction"), and one
// threatening + one neutral synthetic transcript so the tone/threat judge,
// the requires_hitl flag, and the console threat/HITL badge render with dev
// data (spec "Seed Provides Threatening and Neutral Synthetic Transcripts").
func devDataFixtures(now time.Time, debtorLoc *time.Location) []interactionFixture {
	afterHours := afterHoursInstant(now, debtorLoc)

	thirdPartyDemo := compliantFixture(
		string(core.InteractionChannelCall), string(core.InteractionDirectionOutbound),
		"seed/demo/call-05-third-party", now.Add(-3*time.Hour), nil,
	)
	// MX-REDECO-06 violation demo: contacted party is not the debtor and
	// not an authorized third party. Every other detector input stays
	// compliant except disclosureProvided (see below), so this
	// interaction's BLOCK is attributable to the third-party-contact
	// detector alone.
	thirdPartyDemo.contactPartyRelationship = "third_party"
	// Also emits a coexisting MX-REDECO-03 warn (disclosure not stated),
	// demonstrating that a warn row coexisting with a hard-block row still
	// yields overall fail, driven by the block (spec "A warn row coexisting
	// with a hard-block row yields overall fail").
	thirdPartyDemo.disclosureProvided = boolPtr(false)

	protectedMinorDemo := compliantFixture(
		string(core.InteractionChannelCall), string(core.InteractionDirectionOutbound),
		"seed/demo/call-06-protected-minor", now.Add(-2*time.Hour), nil,
	)
	// MX-REDECO-07 violation demo: the contacted party is a minor (age 15
	// as of OccurredAt), which BLOCKs regardless of contactPartyRelationship
	// (kept "debtor" here) and additionally requires HITL. Overriding only
	// the DOB — not the seeded debtor's own record — isolates the BLOCK to
	// the protected-population detector.
	minorDOB := now.Add(-2*time.Hour).AddDate(-15, 0, 0)
	protectedMinorDemo.contactedPartyDOBOverride = &minorDOB

	unauthorizedChannelDemo := compliantFixture(
		string(core.InteractionChannelCall), string(core.InteractionDirectionOutbound),
		"seed/demo/call-07-unauthorized-channel", now.Add(-1*time.Hour), nil,
	)
	// MX-REDECO-11 violation demo: the interaction's channel ("call") is
	// not present in the debtor's authorized-channel list (only "email"
	// is authorized), isolating the BLOCK to the authorized-channel
	// detector.
	unauthorizedChannelDemo.authorizedChannels = []string{string(core.InteractionChannelEmail)}

	paymentRoutingDemo := compliantFixture(
		string(core.InteractionChannelMessage), string(core.InteractionDirectionInbound),
		"seed/demo/message-08-payment-routing", now.Add(-30*time.Minute), nil,
	)
	// MX-REDECO-10 violation demo: payment is routed to a non-creditor
	// recipient, isolating the BLOCK to the payment-routing detector.
	paymentRoutingDemo.paymentRecipient = "collector"

	disclosureWarnDemo := compliantFixture(
		string(core.InteractionChannelCall), string(core.InteractionDirectionOutbound),
		"seed/demo/call-09-disclosure-warn", now.Add(-15*time.Minute), nil,
	)
	// MX-REDECO-03 warn-only demo: the required UNE/complaints-unit
	// disclosure was not stated, but every hard-block detector input stays
	// compliant, so this interaction's evaluation stays overall PASS with a
	// single warn/medium detector_result_rows entry (spec "A warn-only
	// interaction evaluation stays overall pass").
	disclosureWarnDemo.disclosureProvided = boolPtr(false)

	return []interactionFixture{
		compliantFixture(
			string(core.InteractionChannelCall), string(core.InteractionDirectionOutbound),
			"seed/demo/call-01", now.Add(-72*time.Hour), nil,
		),
		compliantFixture(
			string(core.InteractionChannelMessage), string(core.InteractionDirectionInbound),
			"seed/demo/message-01", now.Add(-48*time.Hour), nil,
		),
		compliantFixture(
			string(core.InteractionChannelEmail), string(core.InteractionDirectionOutbound),
			"seed/demo/email-01", now.Add(-24*time.Hour), nil,
		),
		compliantFixture(
			string(core.InteractionChannelCall), string(core.InteractionDirectionOutbound),
			"seed/demo/call-02-after-hours", afterHours, nil,
		),
		compliantFixture(
			string(core.InteractionChannelCall), string(core.InteractionDirectionOutbound),
			"seed/demo/call-03-threatening", now.Add(-12*time.Hour), threateningTranscriptUtterances(),
		),
		compliantFixture(
			string(core.InteractionChannelCall), string(core.InteractionDirectionOutbound),
			"seed/demo/call-04-neutral", now.Add(-6*time.Hour), neutralTranscriptUtterances(),
		),
		thirdPartyDemo,
		protectedMinorDemo,
		unauthorizedChannelDemo,
		paymentRoutingDemo,
		disclosureWarnDemo,
	}
}

// afterHoursInstant returns an instant, at or before now, whose debtor-local
// wall-clock time is 22:00:00 — outside the [08:00:00, 21:00:00) contact
// window regardless of the debtor's timezone.
func afterHoursInstant(now time.Time, debtorLoc *time.Location) time.Time {
	local := now.In(debtorLoc)
	candidate := time.Date(local.Year(), local.Month(), local.Day(), 22, 0, 0, 0, debtorLoc)
	if candidate.After(now) {
		candidate = candidate.AddDate(0, 0, -1)
	}
	return candidate
}

// SeedDevData creates a demo tenant, debtor, and three labeled es-MX interaction events,
// then issues a fresh tenant API key. All entity operations are idempotent: existing
// entities are detected and reused; only missing ones are created. The API key is always
// minted fresh (plaintext of a prior key cannot be recovered from the hash).
//
// Inserts run through the owner/migration role over cfg.DatabaseURL, which bypasses RLS
// because the table owner is never subject to RLS unless FORCE ROW LEVEL SECURITY is set.
// Do NOT wrap these inserts in tenantdb.WithTenantTx.
//
// FK ordering: tenant → debtor → interaction_events → API key.
//
// Every fixture ends up evaluated: newly created interactions are evaluated
// immediately, and pre-existing ones (from a prior seed run) are backfilled
// if they don't already have an evaluation row. Re-running the seed against
// already-seeded data must not create duplicate evaluations rows.
func SeedDevData(ctx context.Context, q SeedQuerier, issue KeyIssuer, evaluator Evaluator, p DevDataParams) (DevDataResult, error) {
	var result DevDataResult
	now := time.Now().UTC()

	// --- Tenant (idempotent by slug) ---
	tenant, err := q.GetTenantBySlug(ctx, p.Slug)
	if err != nil {
		if !isNotFound(err) {
			return DevDataResult{}, fmt.Errorf("get tenant by slug: %w", err)
		}
		tenant, err = q.CreateTenant(ctx, vigiaDB.CreateTenantParams{
			Slug:   p.Slug,
			Name:   p.Name,
			Status: string(core.TenantStatusActive),
		})
		if err != nil {
			return DevDataResult{}, fmt.Errorf("create tenant: %w", err)
		}
		result.Created.TenantCreated = true
	}
	result.TenantID = uuidToString(tenant.ID)

	// --- Debtor (idempotent by external_ref) ---
	existingDebtors, err := q.ListDebtorsByTenant(ctx, tenant.ID)
	if err != nil {
		return DevDataResult{}, fmt.Errorf("list debtors: %w", err)
	}
	var debtorID pgtype.UUID
	var debtorTimezone string
	// debtorDOB is the durable DOB source (issue #7), snapshotted onto each
	// created interaction_events row's contacted_party_dob column below,
	// unless a fixture supplies its own contactedPartyDOBOverride.
	var debtorDOB *time.Time
	debtorFound := false
	for _, d := range existingDebtors {
		if d.ExternalRef == p.DebtorRef {
			debtorID = d.ID
			debtorTimezone = d.Timezone
			debtorDOB = pgDateToTimePtr(d.DateOfBirth)
			debtorFound = true
			break
		}
	}
	if !debtorFound {
		timezone := p.Timezone
		if timezone == "" {
			timezone = defaultDebtorTimezone
		}
		if _, err := time.LoadLocation(timezone); err != nil {
			return DevDataResult{}, fmt.Errorf("invalid debtor timezone %q: %w", timezone, err)
		}
		dob := now.AddDate(-defaultDebtorAgeYears, 0, 0)
		created, err := q.CreateDebtor(ctx, vigiaDB.CreateDebtorParams{
			TenantID:    tenant.ID,
			ExternalRef: p.DebtorRef,
			DisplayName: p.DebtorName,
			Timezone:    timezone,
			DateOfBirth: timeToPgDate(dob),
		})
		if err != nil {
			return DevDataResult{}, fmt.Errorf("create debtor: %w", err)
		}
		debtorID = created.ID
		debtorTimezone = created.Timezone
		debtorDOB = pgDateToTimePtr(created.DateOfBirth)
		result.Created.DebtorCreated = true
	}
	result.DebtorID = uuidToString(debtorID)

	// debtorLoc resolves the debtor's snapshotted timezone so fixture
	// construction can compute a real out-of-hours local instant. This never
	// silently falls back to UTC: an unresolvable debtor timezone is a seed
	// configuration error, not a runtime default (Decision 2).
	debtorLoc, err := time.LoadLocation(debtorTimezone)
	if err != nil {
		return DevDataResult{}, fmt.Errorf("resolve debtor timezone %q: %w", debtorTimezone, err)
	}

	// --- Interaction events (idempotent by transcript_ref) ---
	existingEvents, err := q.ListInteractionEventsByTenant(ctx, tenant.ID)
	if err != nil {
		return DevDataResult{}, fmt.Errorf("list interaction events: %w", err)
	}

	existingByRef := make(map[string]pgtype.UUID, len(existingEvents)) // transcript_ref -> id
	for _, e := range existingEvents {
		if e.TranscriptRef != nil {
			existingByRef[*e.TranscriptRef] = e.ID
		}
	}

	fixtures := devDataFixtures(now, debtorLoc)
	interactionIDs := make([]string, 0, len(fixtures))
	for _, fix := range fixtures {
		if id, alreadyExists := existingByRef[fix.transcriptRef]; alreadyExists {
			interactionIDs = append(interactionIDs, uuidToString(id))

			if err := ensureTranscript(ctx, q, tenant.ID, id, fix.utterances); err != nil {
				return DevDataResult{}, fmt.Errorf("ensure transcript for interaction event %s: %w", fix.transcriptRef, err)
			}

			// Backfill: a pre-existing interaction (e.g. from a prior seed
			// run) may not have been evaluated yet. Evaluate it now instead
			// of leaving it permanently unevaluated; the (tenant_id,
			// interaction_event_id) UNIQUE constraint plus this existence
			// check keeps a re-run idempotent.
			if _, err := q.GetEvaluationByInteractionEventID(ctx, vigiaDB.GetEvaluationByInteractionEventIDParams{
				TenantID:           tenant.ID,
				InteractionEventID: id,
			}); err != nil {
				if !isNotFound(err) {
					return DevDataResult{}, fmt.Errorf("get evaluation for interaction event %s: %w", fix.transcriptRef, err)
				}
				utterances, err := resolveUtterances(ctx, q, tenant.ID, id)
				if err != nil {
					return DevDataResult{}, fmt.Errorf("resolve utterances for interaction event %s: %w", fix.transcriptRef, err)
				}
				if _, err := evaluator.EvaluateInteraction(ctx, evaluation.EvaluateInteractionInput{
					TenantID:           result.TenantID,
					InteractionEventID: uuidToString(id),
					Interaction:        fix.toDetectionInteraction(debtorTimezone, debtorDOB),
					Utterances:         utterances,
				}); err != nil {
					return DevDataResult{}, fmt.Errorf("backfill evaluate interaction event %s: %w", fix.transcriptRef, err)
				}
			}
			continue
		}
		ref := fix.transcriptRef
		created, err := q.CreateInteractionEvent(ctx, vigiaDB.CreateInteractionEventParams{
			TenantID:  tenant.ID,
			DebtorID:  debtorID,
			Channel:   fix.channel,
			Direction: fix.direction,
			Status:    "recorded",
			OccurredAt: pgtype.Timestamptz{
				Time:  fix.occurredAt,
				Valid: true,
			},
			TranscriptRef:            &ref,
			DebtorTimezone:           debtorTimezone,
			ContactPartyRelationship: optionalString(fix.contactPartyRelationship),
			ContactedPartyDob:        optionalPgDate(fix.resolvedContactedPartyDOB(debtorDOB)),
			AuthorizedChannels:       fix.authorizedChannels,
			PaymentRecipient:         optionalString(fix.paymentRecipient),
			DisclosureProvided:       fix.disclosureProvided,
		})
		if err != nil {
			return DevDataResult{}, fmt.Errorf("create interaction event %s: %w", fix.transcriptRef, err)
		}
		interactionID := uuidToString(created.ID)
		interactionIDs = append(interactionIDs, interactionID)
		result.Created.InteractionsCreated++

		if err := ensureTranscript(ctx, q, tenant.ID, created.ID, fix.utterances); err != nil {
			return DevDataResult{}, fmt.Errorf("create transcript for interaction event %s: %w", fix.transcriptRef, err)
		}
		utterances, err := resolveUtterances(ctx, q, tenant.ID, created.ID)
		if err != nil {
			return DevDataResult{}, fmt.Errorf("resolve utterances for interaction event %s: %w", fix.transcriptRef, err)
		}

		// Evaluate newly created interactions immediately.
		if _, err := evaluator.EvaluateInteraction(ctx, evaluation.EvaluateInteractionInput{
			TenantID:           result.TenantID,
			InteractionEventID: interactionID,
			Interaction:        fix.toDetectionInteraction(debtorTimezone, debtorDOB),
			Utterances:         utterances,
		}); err != nil {
			return DevDataResult{}, fmt.Errorf("evaluate interaction event %s: %w", fix.transcriptRef, err)
		}
	}
	result.InteractionIDs = interactionIDs

	// --- API key (always fresh — plaintext is only available at mint time) ---
	issued, err := issue.IssueTenantAPIKey(ctx, IssueTenantAPIKeyParams{
		TenantID: result.TenantID,
		Label:    p.Label,
	})
	if err != nil {
		return DevDataResult{}, fmt.Errorf("issue api key: %w", err)
	}
	result.PlaintextKey = issued.PlaintextKey

	return result, nil
}

// ensureTranscript persists utterances as the interaction's transcript
// content, idempotently: a transcript is created only when the interaction
// has none yet (mirrors the transcript_ref existence-check pattern). A
// no-op when utterances is empty (the original #2/#3 fixtures carry no
// transcript content).
func ensureTranscript(ctx context.Context, q SeedQuerier, tenantID, interactionEventID pgtype.UUID, utterances []seedUtterance) error {
	if len(utterances) == 0 {
		return nil
	}
	_, err := q.GetInteractionTranscriptByInteraction(ctx, vigiaDB.GetInteractionTranscriptByInteractionParams{
		TenantID:           tenantID,
		InteractionEventID: interactionEventID,
	})
	if err == nil {
		return nil // already exists — idempotent no-op.
	}
	if !isNotFound(err) {
		return fmt.Errorf("get interaction transcript: %w", err)
	}

	payload, err := json.Marshal(utterances)
	if err != nil {
		return fmt.Errorf("marshal utterances: %w", err)
	}
	if _, err := q.CreateInteractionTranscript(ctx, vigiaDB.CreateInteractionTranscriptParams{
		TenantID:           tenantID,
		InteractionEventID: interactionEventID,
		Utterances:         payload,
	}); err != nil {
		return fmt.Errorf("create interaction transcript: %w", err)
	}
	return nil
}

// resolveUtterances reads the interaction's transcript content back from
// the store (never from the in-memory fixture) and maps it to
// judge.Utterance, so the judge always reads the persisted content — the
// same path a real (non-seed) evaluation would take. Returns nil (no
// utterances) when the interaction has no transcript row.
func resolveUtterances(ctx context.Context, q SeedQuerier, tenantID, interactionEventID pgtype.UUID) ([]judge.Utterance, error) {
	row, err := q.GetInteractionTranscriptByInteraction(ctx, vigiaDB.GetInteractionTranscriptByInteractionParams{
		TenantID:           tenantID,
		InteractionEventID: interactionEventID,
	})
	if err != nil {
		if isNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("get interaction transcript: %w", err)
	}

	var stored []seedUtterance
	if err := json.Unmarshal(row.Utterances, &stored); err != nil {
		return nil, fmt.Errorf("unmarshal stored utterances: %w", err)
	}
	utterances := make([]judge.Utterance, 0, len(stored))
	for _, u := range stored {
		utterances = append(utterances, judge.Utterance{Speaker: u.Speaker, Text: u.Text})
	}
	return utterances, nil
}

// isNotFound returns true when err represents "no rows returned" from pgx.
func isNotFound(err error) bool {
	return errors.Is(err, pgx.ErrNoRows)
}

// uuidToString converts a pgtype.UUID to its canonical hyphenated hex string.
func uuidToString(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}
	b := u.Bytes
	return fmt.Sprintf(
		"%02x%02x%02x%02x-%02x%02x-%02x%02x-%02x%02x-%02x%02x%02x%02x%02x%02x",
		b[0], b[1], b[2], b[3],
		b[4], b[5],
		b[6], b[7],
		b[8], b[9],
		b[10], b[11], b[12], b[13], b[14], b[15],
	)
}

// optionalString returns nil for an empty string, otherwise a pointer to s.
// Mirrors the "empty/unset means unresolved, detector fails closed" contract
// (issue #7) for the nullable *string detector-input columns.
func optionalString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// timeToPgDate converts a time.Time to a valid pgtype.Date, truncating any
// time-of-day/location component to the UTC calendar date — dates have no
// time-of-day component in Postgres.
func timeToPgDate(t time.Time) pgtype.Date {
	return pgtype.Date{
		Time:  time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC),
		Valid: true,
	}
}

// optionalPgDate converts a *time.Time to a pgtype.Date, returning an
// invalid (NULL) pgtype.Date for a nil input.
func optionalPgDate(t *time.Time) pgtype.Date {
	if t == nil {
		return pgtype.Date{}
	}
	return timeToPgDate(*t)
}

// pgDateToTimePtr converts a pgtype.Date to *time.Time, returning nil for an
// invalid (NULL) date.
func pgDateToTimePtr(d pgtype.Date) *time.Time {
	if !d.Valid {
		return nil
	}
	t := d.Time
	return &t
}
