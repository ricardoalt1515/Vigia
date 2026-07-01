package main

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	vigiaDB "github.com/ricardoalt1515/vigia/internal/db"
)

// --- fake SeedQuerier ---

type fakeCall struct {
	method string
	arg    any
}

type fakeSeedQuerier struct {
	calls []fakeCall

	// canned responses
	tenantBySlug map[string]vigiaDB.Tenant
	debtorRows   []vigiaDB.ListDebtorsByTenantRow
	eventRows    []vigiaDB.ListInteractionEventsByTenantRow

	// errors
	getTenantErr    error
	createTenantErr error
	listDebtorsErr  error
	createDebtorErr error
	listEventsErr   error
	createEventErr  error
}

func (f *fakeSeedQuerier) GetTenantBySlug(ctx context.Context, slug string) (vigiaDB.Tenant, error) {
	f.calls = append(f.calls, fakeCall{method: "GetTenantBySlug", arg: slug})
	if f.getTenantErr != nil {
		return vigiaDB.Tenant{}, f.getTenantErr
	}
	t, ok := f.tenantBySlug[slug]
	if !ok {
		return vigiaDB.Tenant{}, pgx.ErrNoRows
	}
	return t, nil
}

func (f *fakeSeedQuerier) CreateTenant(ctx context.Context, arg vigiaDB.CreateTenantParams) (vigiaDB.Tenant, error) {
	f.calls = append(f.calls, fakeCall{method: "CreateTenant", arg: arg})
	if f.createTenantErr != nil {
		return vigiaDB.Tenant{}, f.createTenantErr
	}
	id := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	return vigiaDB.Tenant{ID: id, Slug: arg.Slug, Name: arg.Name, Status: arg.Status}, nil
}

func (f *fakeSeedQuerier) ListDebtorsByTenant(ctx context.Context, tenantID pgtype.UUID) ([]vigiaDB.ListDebtorsByTenantRow, error) {
	f.calls = append(f.calls, fakeCall{method: "ListDebtorsByTenant", arg: tenantID})
	if f.listDebtorsErr != nil {
		return nil, f.listDebtorsErr
	}
	return f.debtorRows, nil
}

func (f *fakeSeedQuerier) CreateDebtor(ctx context.Context, arg vigiaDB.CreateDebtorParams) (vigiaDB.CreateDebtorRow, error) {
	f.calls = append(f.calls, fakeCall{method: "CreateDebtor", arg: arg})
	if f.createDebtorErr != nil {
		return vigiaDB.CreateDebtorRow{}, f.createDebtorErr
	}
	id := pgtype.UUID{Bytes: [16]byte{2}, Valid: true}
	return vigiaDB.CreateDebtorRow{ID: id, TenantID: arg.TenantID, ExternalRef: arg.ExternalRef, DisplayName: arg.DisplayName, Timezone: arg.Timezone}, nil
}

func (f *fakeSeedQuerier) ListInteractionEventsByTenant(ctx context.Context, tenantID pgtype.UUID) ([]vigiaDB.ListInteractionEventsByTenantRow, error) {
	f.calls = append(f.calls, fakeCall{method: "ListInteractionEventsByTenant", arg: tenantID})
	if f.listEventsErr != nil {
		return nil, f.listEventsErr
	}
	return f.eventRows, nil
}

func (f *fakeSeedQuerier) CreateInteractionEvent(ctx context.Context, arg vigiaDB.CreateInteractionEventParams) (vigiaDB.CreateInteractionEventRow, error) {
	f.calls = append(f.calls, fakeCall{method: "CreateInteractionEvent", arg: arg})
	if f.createEventErr != nil {
		return vigiaDB.CreateInteractionEventRow{}, f.createEventErr
	}
	id := pgtype.UUID{Bytes: [16]byte{3}, Valid: true}
	return vigiaDB.CreateInteractionEventRow{
		ID:             id,
		TenantID:       arg.TenantID,
		DebtorID:       arg.DebtorID,
		Channel:        arg.Channel,
		Direction:      arg.Direction,
		Status:         arg.Status,
		OccurredAt:     arg.OccurredAt,
		TranscriptRef:  arg.TranscriptRef,
		DebtorTimezone: arg.DebtorTimezone,
	}, nil
}

func (f *fakeSeedQuerier) countMethodCalls(method string) int {
	n := 0
	for _, c := range f.calls {
		if c.method == method {
			n++
		}
	}
	return n
}

func (f *fakeSeedQuerier) callOrder() []string {
	out := make([]string, len(f.calls))
	for i, c := range f.calls {
		out[i] = c.method
	}
	return out
}

// --- fake KeyIssuer ---

type fakeKeyIssuer struct {
	calls     int
	returnKey string
	returnErr error
}

func (k *fakeKeyIssuer) IssueTenantAPIKey(ctx context.Context, params IssueTenantAPIKeyParams) (IssuedTenantAPIKey, error) {
	k.calls++
	if k.returnErr != nil {
		return IssuedTenantAPIKey{}, k.returnErr
	}
	key := k.returnKey
	if key == "" {
		key = "vigia_tenant_fake-plaintext-key"
	}
	return IssuedTenantAPIKey{PlaintextKey: key}, nil
}

// defaultParams returns a DevDataParams with standard dev fixture values.
func defaultParams() DevDataParams {
	return DevDataParams{
		Slug:       "demo",
		Name:       "Demo Tenant",
		DebtorRef:  "debtor-001",
		DebtorName: "Juana Pérez (demo)",
		Label:      "local-dev",
	}
}

// TestSeedDevData is the table-driven unit test for SeedDevData.
func TestSeedDevData(t *testing.T) {
	ctx := context.Background()

	t.Run("fresh_run_creates_all_entities", func(t *testing.T) {
		q := &fakeSeedQuerier{
			// no tenant by slug -> will create
			tenantBySlug: map[string]vigiaDB.Tenant{},
			// no debtors, no events
		}
		issuer := &fakeKeyIssuer{}

		result, err := SeedDevData(ctx, q, issuer, defaultParams())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Exactly one CreateTenant call
		if got := q.countMethodCalls("CreateTenant"); got != 1 {
			t.Errorf("CreateTenant calls = %d, want 1", got)
		}
		// Exactly one CreateDebtor call
		if got := q.countMethodCalls("CreateDebtor"); got != 1 {
			t.Errorf("CreateDebtor calls = %d, want 1", got)
		}
		// Exactly three CreateInteractionEvent calls
		if got := q.countMethodCalls("CreateInteractionEvent"); got != 3 {
			t.Errorf("CreateInteractionEvent calls = %d, want 3", got)
		}
		// Exactly one key issuance
		if issuer.calls != 1 {
			t.Errorf("KeyIssuer calls = %d, want 1", issuer.calls)
		}
		// FK order: tenant before debtor before interaction_events before key
		order := q.callOrder()
		assertCallBefore(t, order, "CreateTenant", "CreateDebtor")
		assertCallBefore(t, order, "CreateDebtor", "CreateInteractionEvent")
		// key issuance happens after all DB writes (issuer.calls checked separately)

		// Result carries IDs and plaintext key
		if result.TenantID == "" {
			t.Error("TenantID should be set in result")
		}
		if result.DebtorID == "" {
			t.Error("DebtorID should be set in result")
		}
		if len(result.InteractionIDs) != 3 {
			t.Errorf("InteractionIDs = %d, want 3", len(result.InteractionIDs))
		}
		if result.PlaintextKey == "" {
			t.Error("PlaintextKey should be set in result")
		}
		// Created counts
		if !result.Created.TenantCreated {
			t.Error("Created.TenantCreated should be true")
		}
		if !result.Created.DebtorCreated {
			t.Error("Created.DebtorCreated should be true")
		}
		if result.Created.InteractionsCreated != 3 {
			t.Errorf("Created.InteractionsCreated = %d, want 3", result.Created.InteractionsCreated)
		}
	})

	t.Run("idempotent_rerun", func(t *testing.T) {
		existingTenantID := pgtype.UUID{Bytes: [16]byte{10}, Valid: true}
		existingDebtorID := pgtype.UUID{Bytes: [16]byte{20}, Valid: true}
		ref0 := "seed/demo/call-01"
		ref1 := "seed/demo/message-01"
		ref2 := "seed/demo/email-01"

		q := &fakeSeedQuerier{
			tenantBySlug: map[string]vigiaDB.Tenant{
				"demo": {ID: existingTenantID, Slug: "demo"},
			},
			debtorRows: []vigiaDB.ListDebtorsByTenantRow{
				{ID: existingDebtorID, TenantID: existingTenantID, ExternalRef: "debtor-001", Timezone: "America/Mexico_City"},
			},
			eventRows: []vigiaDB.ListInteractionEventsByTenantRow{
				{ID: pgtype.UUID{Bytes: [16]byte{31}, Valid: true}, TranscriptRef: &ref0},
				{ID: pgtype.UUID{Bytes: [16]byte{32}, Valid: true}, TranscriptRef: &ref1},
				{ID: pgtype.UUID{Bytes: [16]byte{33}, Valid: true}, TranscriptRef: &ref2},
			},
		}
		issuer := &fakeKeyIssuer{}

		result, err := SeedDevData(ctx, q, issuer, defaultParams())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Zero creates for existing entities
		if got := q.countMethodCalls("CreateTenant"); got != 0 {
			t.Errorf("CreateTenant calls = %d, want 0 (idempotent)", got)
		}
		if got := q.countMethodCalls("CreateDebtor"); got != 0 {
			t.Errorf("CreateDebtor calls = %d, want 0 (idempotent)", got)
		}
		if got := q.countMethodCalls("CreateInteractionEvent"); got != 0 {
			t.Errorf("CreateInteractionEvent calls = %d, want 0 (idempotent)", got)
		}
		// Key is always issued fresh
		if issuer.calls != 1 {
			t.Errorf("KeyIssuer calls = %d, want 1", issuer.calls)
		}

		if result.Created.TenantCreated {
			t.Error("Created.TenantCreated should be false on re-run")
		}
		if result.Created.DebtorCreated {
			t.Error("Created.DebtorCreated should be false on re-run")
		}
		if result.Created.InteractionsCreated != 0 {
			t.Errorf("Created.InteractionsCreated = %d, want 0 on re-run", result.Created.InteractionsCreated)
		}
	})

	t.Run("partial_state_missing_interactions", func(t *testing.T) {
		existingTenantID := pgtype.UUID{Bytes: [16]byte{10}, Valid: true}
		existingDebtorID := pgtype.UUID{Bytes: [16]byte{20}, Valid: true}
		// Only the first interaction exists; two are missing.
		ref0 := "seed/demo/call-01"

		q := &fakeSeedQuerier{
			tenantBySlug: map[string]vigiaDB.Tenant{
				"demo": {ID: existingTenantID, Slug: "demo"},
			},
			debtorRows: []vigiaDB.ListDebtorsByTenantRow{
				{ID: existingDebtorID, TenantID: existingTenantID, ExternalRef: "debtor-001", Timezone: "America/Mexico_City"},
			},
			eventRows: []vigiaDB.ListInteractionEventsByTenantRow{
				{ID: pgtype.UUID{Bytes: [16]byte{31}, Valid: true}, TranscriptRef: &ref0},
			},
		}
		issuer := &fakeKeyIssuer{}

		result, err := SeedDevData(ctx, q, issuer, defaultParams())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if got := q.countMethodCalls("CreateTenant"); got != 0 {
			t.Errorf("CreateTenant calls = %d, want 0", got)
		}
		if got := q.countMethodCalls("CreateDebtor"); got != 0 {
			t.Errorf("CreateDebtor calls = %d, want 0", got)
		}
		// Only the two missing interactions should be created
		if got := q.countMethodCalls("CreateInteractionEvent"); got != 2 {
			t.Errorf("CreateInteractionEvent calls = %d, want 2 (missing ones only)", got)
		}
		if issuer.calls != 1 {
			t.Errorf("KeyIssuer calls = %d, want 1", issuer.calls)
		}
		if result.Created.InteractionsCreated != 2 {
			t.Errorf("Created.InteractionsCreated = %d, want 2", result.Created.InteractionsCreated)
		}

		// Verify the created interactions have the correct transcript_refs
		var createdRefs []string
		for _, c := range q.calls {
			if c.method == "CreateInteractionEvent" {
				p := c.arg.(vigiaDB.CreateInteractionEventParams)
				if p.TranscriptRef != nil {
					createdRefs = append(createdRefs, *p.TranscriptRef)
				}
			}
		}
		wantRefs := []string{"seed/demo/message-01", "seed/demo/email-01"}
		for _, want := range wantRefs {
			found := false
			for _, got := range createdRefs {
				if got == want {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected CreateInteractionEvent for transcript_ref %q, not found in %v", want, createdRefs)
			}
		}
	})
}

// assertCallBefore checks that the first occurrence of methodA appears before the first
// occurrence of methodB in the call log.
func assertCallBefore(t *testing.T, order []string, methodA, methodB string) {
	t.Helper()
	idxA, idxB := -1, -1
	for i, m := range order {
		if m == methodA && idxA == -1 {
			idxA = i
		}
		if m == methodB && idxB == -1 {
			idxB = i
		}
	}
	if idxA == -1 {
		t.Errorf("method %q never called", methodA)
		return
	}
	if idxB == -1 {
		t.Errorf("method %q never called", methodB)
		return
	}
	if idxA > idxB {
		t.Errorf("call order: %q (pos %d) must come before %q (pos %d)", methodA, idxA, methodB, idxB)
	}
}

// TestSeedDispatch verifies that "dev-data" in args routes to the seed path
// and that args without "dev-data" still use the legacy key-issuance path.
func TestSeedDispatch(t *testing.T) {
	// This test is structural: it verifies the dispatch logic of run() by checking that
	// the devdata path is selected when args[0] == "dev-data". Because run() requires a
	// real DB connection, we test the dispatch routing logic via a separate helper that
	// can be called without infrastructure. This test will be written after the dispatch
	// is wired in T1.7; for now it serves as the RED marker.
	//
	// The actual routing test is in TestRunDevDataDispatch below, which calls routeArgs().
	t.Run("dev-data_routes_to_seed", func(t *testing.T) {
		route := routeArgs([]string{"dev-data"})
		if route != "dev-data" {
			t.Errorf("routeArgs([dev-data]) = %q, want %q", route, "dev-data")
		}
	})

	t.Run("no_subcommand_routes_to_key_issuance", func(t *testing.T) {
		route := routeArgs([]string{"--tenant-id", "some-uuid", "--label", "foo"})
		if route != "key-issuance" {
			t.Errorf("routeArgs([--tenant-id ...]) = %q, want %q", route, "key-issuance")
		}
	})

	t.Run("empty_args_routes_to_key_issuance", func(t *testing.T) {
		route := routeArgs([]string{})
		if route != "key-issuance" {
			t.Errorf("routeArgs([]) = %q, want %q", route, "key-issuance")
		}
	})
}

// Compile-time check: fakeSeedQuerier must satisfy SeedQuerier.
var _ SeedQuerier = (*fakeSeedQuerier)(nil)
