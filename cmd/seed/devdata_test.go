package main

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/ricardoalt1515/vigia/internal/core"
	vigiaDB "github.com/ricardoalt1515/vigia/internal/db"
	"github.com/ricardoalt1515/vigia/internal/evaluation"
)

// --- fake SeedQuerier ---

type fakeCall struct {
	method string
	arg    any
}

type fakeSeedQuerier struct {
	calls        []fakeCall
	nextEventSeq byte

	// canned responses
	tenantBySlug map[string]vigiaDB.Tenant
	debtorRows   []vigiaDB.ListDebtorsByTenantRow
	eventRows    []vigiaDB.ListInteractionEventsByTenantRow

	// existingEvaluationsByInteractionID marks which interaction_event_ids
	// (by uuidToString) already have an evaluation row, so
	// GetEvaluationByInteractionEventID returns a found row instead of
	// pgx.ErrNoRows.
	existingEvaluationsByInteractionID map[string]bool

	// existingTranscriptsByInteractionID marks which interaction_event_ids
	// (by uuidToString) already have a transcript row, so
	// GetInteractionTranscriptByInteraction returns a found row instead of
	// pgx.ErrNoRows.
	existingTranscriptsByInteractionID map[string]vigiaDB.InteractionTranscript

	// errors
	getTenantErr     error
	createTenantErr  error
	listDebtorsErr   error
	createDebtorErr  error
	listEventsErr    error
	createEventErr   error
	getEvaluationErr error
	createTranscript error
	getTranscriptErr error
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
	f.nextEventSeq++
	id := pgtype.UUID{Bytes: [16]byte{3, f.nextEventSeq}, Valid: true}
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

func (f *fakeSeedQuerier) GetEvaluationByInteractionEventID(ctx context.Context, arg vigiaDB.GetEvaluationByInteractionEventIDParams) (vigiaDB.Evaluation, error) {
	f.calls = append(f.calls, fakeCall{method: "GetEvaluationByInteractionEventID", arg: arg})
	if f.getEvaluationErr != nil {
		return vigiaDB.Evaluation{}, f.getEvaluationErr
	}
	if f.existingEvaluationsByInteractionID[uuidToString(arg.InteractionEventID)] {
		return vigiaDB.Evaluation{
			ID:                 arg.InteractionEventID,
			TenantID:           arg.TenantID,
			InteractionEventID: arg.InteractionEventID,
			OverallOutcome:     "pass",
		}, nil
	}
	return vigiaDB.Evaluation{}, pgx.ErrNoRows
}

func (f *fakeSeedQuerier) CreateInteractionTranscript(ctx context.Context, arg vigiaDB.CreateInteractionTranscriptParams) (vigiaDB.InteractionTranscript, error) {
	f.calls = append(f.calls, fakeCall{method: "CreateInteractionTranscript", arg: arg})
	if f.createTranscript != nil {
		return vigiaDB.InteractionTranscript{}, f.createTranscript
	}
	rec := vigiaDB.InteractionTranscript{
		TenantID:           arg.TenantID,
		InteractionEventID: arg.InteractionEventID,
		Utterances:         arg.Utterances,
	}
	if f.existingTranscriptsByInteractionID == nil {
		f.existingTranscriptsByInteractionID = map[string]vigiaDB.InteractionTranscript{}
	}
	f.existingTranscriptsByInteractionID[uuidToString(arg.InteractionEventID)] = rec
	return rec, nil
}

func (f *fakeSeedQuerier) GetInteractionTranscriptByInteraction(ctx context.Context, arg vigiaDB.GetInteractionTranscriptByInteractionParams) (vigiaDB.InteractionTranscript, error) {
	f.calls = append(f.calls, fakeCall{method: "GetInteractionTranscriptByInteraction", arg: arg})
	if f.getTranscriptErr != nil {
		return vigiaDB.InteractionTranscript{}, f.getTranscriptErr
	}
	if rec, ok := f.existingTranscriptsByInteractionID[uuidToString(arg.InteractionEventID)]; ok {
		return rec, nil
	}
	return vigiaDB.InteractionTranscript{}, pgx.ErrNoRows
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

// --- fake Evaluator ---

type fakeEvaluatorCall struct {
	tenantID           string
	interactionEventID string
	debtorTimezone     string
}

type fakeEvaluator struct {
	calls []fakeEvaluatorCall
	err   error
}

func (f *fakeEvaluator) EvaluateInteraction(ctx context.Context, in evaluation.EvaluateInteractionInput) (core.Evaluation, error) {
	f.calls = append(f.calls, fakeEvaluatorCall{
		tenantID:           in.TenantID,
		interactionEventID: in.InteractionEventID,
		debtorTimezone:     in.Interaction.DebtorTimezone,
	})
	if f.err != nil {
		return core.Evaluation{}, f.err
	}
	return core.Evaluation{ID: "evaluation-fake", OverallOutcome: "fail"}, nil
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

		evaluator := &fakeEvaluator{}
		result, err := SeedDevData(ctx, q, issuer, evaluator, defaultParams())
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
		// Exactly six CreateInteractionEvent calls (three original fixtures,
		// the out-of-hours demo fixture, and the threatening + neutral
		// transcript fixtures, issue #4)
		if got := q.countMethodCalls("CreateInteractionEvent"); got != 6 {
			t.Errorf("CreateInteractionEvent calls = %d, want 6", got)
		}
		// Exactly two CreateInteractionTranscript calls (threatening + neutral)
		if got := q.countMethodCalls("CreateInteractionTranscript"); got != 2 {
			t.Errorf("CreateInteractionTranscript calls = %d, want 2", got)
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
		if len(result.InteractionIDs) != 6 {
			t.Errorf("InteractionIDs = %d, want 6", len(result.InteractionIDs))
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
		if result.Created.InteractionsCreated != 6 {
			t.Errorf("Created.InteractionsCreated = %d, want 6", result.Created.InteractionsCreated)
		}
		// Every newly created interaction is evaluated exactly once, with the
		// debtor's snapshotted timezone (never empty/UTC-defaulted).
		if len(evaluator.calls) != 6 {
			t.Fatalf("Evaluator calls = %d, want 6", len(evaluator.calls))
		}
		for _, c := range evaluator.calls {
			if c.tenantID != result.TenantID {
				t.Errorf("evaluator call tenantID = %q, want %q", c.tenantID, result.TenantID)
			}
			if c.debtorTimezone == "" {
				t.Error("evaluator call debtorTimezone should not be empty")
			}
		}
	})

	t.Run("idempotent_rerun", func(t *testing.T) {
		existingTenantID := pgtype.UUID{Bytes: [16]byte{10}, Valid: true}
		existingDebtorID := pgtype.UUID{Bytes: [16]byte{20}, Valid: true}
		ref0 := "seed/demo/call-01"
		ref1 := "seed/demo/message-01"
		ref2 := "seed/demo/email-01"
		ref3 := "seed/demo/call-02-after-hours"
		ref4 := "seed/demo/call-03-threatening"
		ref5 := "seed/demo/call-04-neutral"
		id4 := pgtype.UUID{Bytes: [16]byte{35}, Valid: true}
		id5 := pgtype.UUID{Bytes: [16]byte{36}, Valid: true}

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
				{ID: pgtype.UUID{Bytes: [16]byte{34}, Valid: true}, TranscriptRef: &ref3},
				{ID: id4, TranscriptRef: &ref4},
				{ID: id5, TranscriptRef: &ref5},
			},
			existingEvaluationsByInteractionID: map[string]bool{
				uuidToString(pgtype.UUID{Bytes: [16]byte{31}, Valid: true}): true,
				uuidToString(pgtype.UUID{Bytes: [16]byte{32}, Valid: true}): true,
				uuidToString(pgtype.UUID{Bytes: [16]byte{33}, Valid: true}): true,
				uuidToString(pgtype.UUID{Bytes: [16]byte{34}, Valid: true}): true,
				uuidToString(id4): true,
				uuidToString(id5): true,
			},
			existingTranscriptsByInteractionID: map[string]vigiaDB.InteractionTranscript{
				uuidToString(id4): {InteractionEventID: id4, Utterances: []byte(`[]`)},
				uuidToString(id5): {InteractionEventID: id5, Utterances: []byte(`[]`)},
			},
		}
		issuer := &fakeKeyIssuer{}

		evaluator := &fakeEvaluator{}
		result, err := SeedDevData(ctx, q, issuer, evaluator, defaultParams())
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
		// Zero new transcripts — both already exist
		if got := q.countMethodCalls("CreateInteractionTranscript"); got != 0 {
			t.Errorf("CreateInteractionTranscript calls = %d, want 0 (idempotent)", got)
		}
		// Key is always issued fresh
		if issuer.calls != 1 {
			t.Errorf("KeyIssuer calls = %d, want 1", issuer.calls)
		}
		// No re-evaluation of already-seeded interactions
		if len(evaluator.calls) != 0 {
			t.Errorf("Evaluator calls = %d, want 0 (idempotent — no re-evaluation)", len(evaluator.calls))
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
		// Only the first interaction exists; five are missing.
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
			// The one pre-existing interaction already has an evaluation, so
			// this test's "only newly created interactions are evaluated"
			// assertion below stays about creation, not backfill (backfill
			// itself is covered by its own test case).
			existingEvaluationsByInteractionID: map[string]bool{
				uuidToString(pgtype.UUID{Bytes: [16]byte{31}, Valid: true}): true,
			},
		}
		issuer := &fakeKeyIssuer{}

		evaluator := &fakeEvaluator{}
		result, err := SeedDevData(ctx, q, issuer, evaluator, defaultParams())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if got := q.countMethodCalls("CreateTenant"); got != 0 {
			t.Errorf("CreateTenant calls = %d, want 0", got)
		}
		if got := q.countMethodCalls("CreateDebtor"); got != 0 {
			t.Errorf("CreateDebtor calls = %d, want 0", got)
		}
		// Only the five missing interactions should be created
		if got := q.countMethodCalls("CreateInteractionEvent"); got != 5 {
			t.Errorf("CreateInteractionEvent calls = %d, want 5 (missing ones only)", got)
		}
		if issuer.calls != 1 {
			t.Errorf("KeyIssuer calls = %d, want 1", issuer.calls)
		}
		if result.Created.InteractionsCreated != 5 {
			t.Errorf("Created.InteractionsCreated = %d, want 5", result.Created.InteractionsCreated)
		}
		if len(evaluator.calls) != 5 {
			t.Errorf("Evaluator calls = %d, want 5 (only newly created interactions)", len(evaluator.calls))
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
		wantRefs := []string{
			"seed/demo/message-01", "seed/demo/email-01", "seed/demo/call-02-after-hours",
			"seed/demo/call-03-threatening", "seed/demo/call-04-neutral",
		}
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

	t.Run("backfill_evaluates_previously_unevaluated_existing_interactions", func(t *testing.T) {
		existingTenantID := pgtype.UUID{Bytes: [16]byte{10}, Valid: true}
		existingDebtorID := pgtype.UUID{Bytes: [16]byte{20}, Valid: true}
		ref0 := "seed/demo/call-01"
		ref1 := "seed/demo/message-01"
		ref2 := "seed/demo/email-01"
		ref3 := "seed/demo/call-02-after-hours"
		ref4 := "seed/demo/call-03-threatening"
		ref5 := "seed/demo/call-04-neutral"
		id0 := pgtype.UUID{Bytes: [16]byte{31}, Valid: true}
		id1 := pgtype.UUID{Bytes: [16]byte{32}, Valid: true}
		id2 := pgtype.UUID{Bytes: [16]byte{33}, Valid: true}
		id3 := pgtype.UUID{Bytes: [16]byte{34}, Valid: true}
		id4 := pgtype.UUID{Bytes: [16]byte{35}, Valid: true}
		id5 := pgtype.UUID{Bytes: [16]byte{36}, Valid: true}

		q := &fakeSeedQuerier{
			tenantBySlug: map[string]vigiaDB.Tenant{
				"demo": {ID: existingTenantID, Slug: "demo"},
			},
			debtorRows: []vigiaDB.ListDebtorsByTenantRow{
				{ID: existingDebtorID, TenantID: existingTenantID, ExternalRef: "debtor-001", Timezone: "America/Mexico_City"},
			},
			eventRows: []vigiaDB.ListInteractionEventsByTenantRow{
				{ID: id0, TranscriptRef: &ref0},
				{ID: id1, TranscriptRef: &ref1},
				{ID: id2, TranscriptRef: &ref2},
				{ID: id3, TranscriptRef: &ref3},
				{ID: id4, TranscriptRef: &ref4},
				{ID: id5, TranscriptRef: &ref5},
			},
			// Only two of the six pre-existing interactions already have an
			// evaluation row; the other two of the original four must be
			// backfilled on this run. The threatening/neutral fixtures already
			// have evaluations + transcripts, so this test stays scoped to the
			// original-four backfill scenario.
			existingEvaluationsByInteractionID: map[string]bool{
				uuidToString(id0): true,
				uuidToString(id1): true,
				uuidToString(id4): true,
				uuidToString(id5): true,
			},
			existingTranscriptsByInteractionID: map[string]vigiaDB.InteractionTranscript{
				uuidToString(id4): {InteractionEventID: id4, Utterances: []byte(`[]`)},
				uuidToString(id5): {InteractionEventID: id5, Utterances: []byte(`[]`)},
			},
		}
		issuer := &fakeKeyIssuer{}

		evaluator := &fakeEvaluator{}
		result, err := SeedDevData(ctx, q, issuer, evaluator, defaultParams())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Still no new interaction events created — all four already exist.
		if got := q.countMethodCalls("CreateInteractionEvent"); got != 0 {
			t.Errorf("CreateInteractionEvent calls = %d, want 0 (backfill only, no re-insert)", got)
		}
		if result.Created.InteractionsCreated != 0 {
			t.Errorf("Created.InteractionsCreated = %d, want 0 (backfill only)", result.Created.InteractionsCreated)
		}

		// Exactly the two unevaluated interactions get backfilled.
		if len(evaluator.calls) != 2 {
			t.Fatalf("Evaluator calls = %d, want 2 (backfill for unevaluated interactions only)", len(evaluator.calls))
		}
		var evaluatedIDs []string
		for _, c := range evaluator.calls {
			evaluatedIDs = append(evaluatedIDs, c.interactionEventID)
		}
		wantEvaluated := []string{uuidToString(id2), uuidToString(id3)}
		for _, want := range wantEvaluated {
			found := false
			for _, got := range evaluatedIDs {
				if got == want {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected backfill evaluation for interaction_event_id %q, not found in %v", want, evaluatedIDs)
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
