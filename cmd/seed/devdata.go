package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/ricardoalt1515/vigia/internal/core"
	vigiaDB "github.com/ricardoalt1515/vigia/internal/db"
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
}

// KeyIssuer mints a tenant API key and returns the plaintext key once.
type KeyIssuer interface {
	IssueTenantAPIKey(ctx context.Context, params IssueTenantAPIKeyParams) (IssuedTenantAPIKey, error)
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

// interactionFixture describes one seeded interaction event.
type interactionFixture struct {
	channel       string
	direction     string
	transcriptRef string
	occurredAt    time.Time
}

// devDataFixtures returns the three canonical es-MX demo interaction fixtures.
func devDataFixtures(now time.Time) []interactionFixture {
	return []interactionFixture{
		{
			channel:       string(core.InteractionChannelCall),
			direction:     string(core.InteractionDirectionOutbound),
			transcriptRef: "seed/demo/call-01",
			occurredAt:    now.Add(-72 * time.Hour),
		},
		{
			channel:       string(core.InteractionChannelMessage),
			direction:     string(core.InteractionDirectionInbound),
			transcriptRef: "seed/demo/message-01",
			occurredAt:    now.Add(-48 * time.Hour),
		},
		{
			channel:       string(core.InteractionChannelEmail),
			direction:     string(core.InteractionDirectionOutbound),
			transcriptRef: "seed/demo/email-01",
			occurredAt:    now.Add(-24 * time.Hour),
		},
	}
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
func SeedDevData(ctx context.Context, q SeedQuerier, issue KeyIssuer, p DevDataParams) (DevDataResult, error) {
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
	debtorFound := false
	for _, d := range existingDebtors {
		if d.ExternalRef == p.DebtorRef {
			debtorID = d.ID
			debtorTimezone = d.Timezone
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
		created, err := q.CreateDebtor(ctx, vigiaDB.CreateDebtorParams{
			TenantID:    tenant.ID,
			ExternalRef: p.DebtorRef,
			DisplayName: p.DebtorName,
			Timezone:    timezone,
		})
		if err != nil {
			return DevDataResult{}, fmt.Errorf("create debtor: %w", err)
		}
		debtorID = created.ID
		debtorTimezone = created.Timezone
		result.Created.DebtorCreated = true
	}
	result.DebtorID = uuidToString(debtorID)

	// --- Interaction events (idempotent by transcript_ref) ---
	existingEvents, err := q.ListInteractionEventsByTenant(ctx, tenant.ID)
	if err != nil {
		return DevDataResult{}, fmt.Errorf("list interaction events: %w", err)
	}

	existingByRef := make(map[string]string, len(existingEvents)) // transcript_ref -> id string
	for _, e := range existingEvents {
		if e.TranscriptRef != nil {
			existingByRef[*e.TranscriptRef] = uuidToString(e.ID)
		}
	}

	interactionIDs := make([]string, 0, 3)
	for _, fix := range devDataFixtures(now) {
		if id, alreadyExists := existingByRef[fix.transcriptRef]; alreadyExists {
			interactionIDs = append(interactionIDs, id)
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
			TranscriptRef:  &ref,
			DebtorTimezone: debtorTimezone,
		})
		if err != nil {
			return DevDataResult{}, fmt.Errorf("create interaction event %s: %w", fix.transcriptRef, err)
		}
		interactionIDs = append(interactionIDs, uuidToString(created.ID))
		result.Created.InteractionsCreated++
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
