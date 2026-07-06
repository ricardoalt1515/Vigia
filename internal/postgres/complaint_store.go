package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/golang-sql/civil"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	vigiaDB "github.com/ricardoalt1515/vigia/internal/db"
	"github.com/ricardoalt1515/vigia/internal/ledger"
	"github.com/ricardoalt1515/vigia/internal/orchestrator"
	"github.com/ricardoalt1515/vigia/internal/tenantdb"
)

type ComplaintCaseStore struct {
	db tenantdb.Beginner
}

func NewComplaintCaseStore(db tenantdb.Beginner) *ComplaintCaseStore {
	return &ComplaintCaseStore{db: db}
}

func NewComplaintCaseStoreFromPool(pool *pgxpool.Pool) *ComplaintCaseStore {
	return NewComplaintCaseStore(poolBeginner{pool: pool})
}

func (s *ComplaintCaseStore) CreateComplaintCase(ctx context.Context, in orchestrator.CreateComplaintCaseInput) (orchestrator.ComplaintCase, error) {
	tenantUUID, err := parseUUID(in.TenantID)
	if err != nil {
		return orchestrator.ComplaintCase{}, err
	}
	interactionUUID, err := parseUUID(in.InteractionID)
	if err != nil {
		return orchestrator.ComplaintCase{}, err
	}

	var result orchestrator.ComplaintCase
	err = tenantdb.WithTenantTx(ctx, s.db, in.TenantID, func(ctx context.Context, tx tenantdb.Tx) error {
		q := vigiaDB.New(tx)
		row, err := q.CreateComplaintCase(ctx, vigiaDB.CreateComplaintCaseParams{
			TenantID:        tenantUUID,
			InteractionID:   interactionUUID,
			RedecoCause:     in.RedecoCause,
			OpenedAt:        pgtype.Timestamptz{Time: in.OpenedAt.UTC().Truncate(time.Microsecond), Valid: true},
			SlaDueAt:        pgtype.Timestamptz{Time: in.SLADueAt.UTC().Truncate(time.Microsecond), Valid: true},
			CalendarVersion: in.CalendarVersion,
			IdempotencyKey:  in.IdempotencyKey,
		})
		if err != nil {
			if !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
			existing, err := q.GetComplaintCaseByIdempotencyKey(ctx, vigiaDB.GetComplaintCaseByIdempotencyKeyParams{
				TenantID:       tenantUUID,
				IdempotencyKey: in.IdempotencyKey,
			})
			if err != nil {
				return err
			}
			if !complaintCaseMatchesCreateInput(existing, interactionUUID, in) {
				return orchestrator.ErrComplaintIdempotencyConflict
			}
			result = complaintCaseFromRow(existing, false)
			return nil
		}

		created := complaintCaseFromRow(row, true)
		if err := appendComplaintEvidence(ctx, q, tenantUUID, created.ID, orchestrator.TransitionOpen, "", orchestrator.ComplaintStateOpen, nil, row.OpenedAt.Time); err != nil {
			return err
		}
		result = created
		return nil
	})
	if err != nil {
		return orchestrator.ComplaintCase{}, err
	}
	return result, nil
}

func (s *ComplaintCaseStore) ListComplaintPollTenants(ctx context.Context) ([]string, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(ctx)
		}
	}()
	rows, err := tx.Query(ctx, `SELECT id FROM tenants ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tenantIDs []string
	for rows.Next() {
		var id pgtype.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		tenantIDs = append(tenantIDs, uuidString(id))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	committed = true
	return tenantIDs, nil
}

func (s *ComplaintCaseStore) GetComplaintCase(ctx context.Context, tenantID string, complaintCaseID string) (orchestrator.ComplaintCase, error) {
	tenantUUID, err := parseUUID(tenantID)
	if err != nil {
		return orchestrator.ComplaintCase{}, err
	}
	caseUUID, err := parseUUID(complaintCaseID)
	if err != nil {
		return orchestrator.ComplaintCase{}, err
	}
	var result orchestrator.ComplaintCase
	err = tenantdb.WithTenantTx(ctx, s.db, tenantID, func(ctx context.Context, tx tenantdb.Tx) error {
		row, err := vigiaDB.New(tx).GetComplaintCase(ctx, vigiaDB.GetComplaintCaseParams{TenantID: tenantUUID, ID: caseUUID})
		if err != nil {
			return err
		}
		result = complaintCaseFromRow(row, false)
		return nil
	})
	return result, err
}

func (s *ComplaintCaseStore) ListOpenComplaintCases(ctx context.Context, tenantID string, limit int32) ([]orchestrator.ComplaintCase, error) {
	tenantUUID, err := parseUUID(tenantID)
	if err != nil {
		return nil, err
	}
	var result []orchestrator.ComplaintCase
	err = tenantdb.WithTenantTx(ctx, s.db, tenantID, func(ctx context.Context, tx tenantdb.Tx) error {
		rows, err := vigiaDB.New(tx).ListOpenComplaintCases(ctx, vigiaDB.ListOpenComplaintCasesParams{TenantID: tenantUUID, Limit: limit})
		if err != nil {
			return err
		}
		result = complaintCasesFromRows(rows)
		return nil
	})
	return result, err
}

func (s *ComplaintCaseStore) ListSLADueComplaintCases(ctx context.Context, tenantID string, dueAt time.Time, limit int32) ([]orchestrator.ComplaintCase, error) {
	tenantUUID, err := parseUUID(tenantID)
	if err != nil {
		return nil, err
	}
	var result []orchestrator.ComplaintCase
	err = tenantdb.WithTenantTx(ctx, s.db, tenantID, func(ctx context.Context, tx tenantdb.Tx) error {
		rows, err := vigiaDB.New(tx).ListSLADueComplaintCases(ctx, vigiaDB.ListSLADueComplaintCasesParams{TenantID: tenantUUID, SlaDueAt: pgtype.Timestamptz{Time: dueAt.UTC().Truncate(time.Microsecond), Valid: true}, Limit: limit})
		if err != nil {
			return err
		}
		result = complaintCasesFromRows(rows)
		return nil
	})
	return result, err
}

func (s *ComplaintCaseStore) ListExpiredReviewComplaintCases(ctx context.Context, tenantID string, expiresAt time.Time, limit int32) ([]orchestrator.ComplaintCase, error) {
	tenantUUID, err := parseUUID(tenantID)
	if err != nil {
		return nil, err
	}
	var result []orchestrator.ComplaintCase
	err = tenantdb.WithTenantTx(ctx, s.db, tenantID, func(ctx context.Context, tx tenantdb.Tx) error {
		rows, err := vigiaDB.New(tx).ListExpiredReviewComplaintCases(ctx, vigiaDB.ListExpiredReviewComplaintCasesParams{TenantID: tenantUUID, ReviewExpiresAt: pgtype.Timestamptz{Time: expiresAt.UTC().Truncate(time.Microsecond), Valid: true}, Limit: limit})
		if err != nil {
			return err
		}
		result = complaintCasesFromRows(rows)
		return nil
	})
	return result, err
}

func (s *ComplaintCaseStore) ListUnprocessedHumanReviews(ctx context.Context, tenantID string, limit int32) ([]orchestrator.HumanReview, error) {
	tenantUUID, err := parseUUID(tenantID)
	if err != nil {
		return nil, err
	}
	var result []orchestrator.HumanReview
	err = tenantdb.WithTenantTx(ctx, s.db, tenantID, func(ctx context.Context, tx tenantdb.Tx) error {
		rows, err := vigiaDB.New(tx).ListUnprocessedHumanReviews(ctx, vigiaDB.ListUnprocessedHumanReviewsParams{TenantID: tenantUUID, Limit: limit})
		if err != nil {
			return err
		}
		result = humanReviewsFromRows(rows)
		return nil
	})
	return result, err
}

func (s *ComplaintCaseStore) CreateHumanReview(ctx context.Context, in orchestrator.CreateHumanReviewInput) (orchestrator.HumanReview, error) {
	tenantUUID, err := parseUUID(in.TenantID)
	if err != nil {
		return orchestrator.HumanReview{}, err
	}
	caseUUID, err := parseUUID(in.ComplaintCaseID)
	if err != nil {
		return orchestrator.HumanReview{}, err
	}
	var result orchestrator.HumanReview
	err = tenantdb.WithTenantTx(ctx, s.db, in.TenantID, func(ctx context.Context, tx tenantdb.Tx) error {
		row, err := vigiaDB.New(tx).InsertHumanReview(ctx, vigiaDB.InsertHumanReviewParams{
			TenantID:        tenantUUID,
			ComplaintCaseID: caseUUID,
			Decision:        in.Decision,
			Reviewer:        in.Reviewer,
			Notes:           in.Notes,
		})
		if err != nil {
			return err
		}
		result = humanReviewFromRow(row)
		return nil
	})
	return result, err
}

func (s *ComplaintCaseStore) ListBusinessDayHolidays(ctx context.Context, version string) ([]orchestrator.HolidayRow, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(ctx)
		}
	}()
	rows, err := vigiaDB.New(tx).ListBusinessDayHolidaysByVersion(ctx, version)
	if err != nil {
		return nil, err
	}
	result := make([]orchestrator.HolidayRow, 0, len(rows))
	for _, row := range rows {
		if !row.HolidayDate.Valid {
			continue
		}
		result = append(result, orchestrator.HolidayRow{Version: row.CalendarVersion, Date: civil.DateOf(row.HolidayDate.Time)})
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	committed = true
	return result, nil
}

func (s *ComplaintCaseStore) ApplyTransition(ctx context.Context, in orchestrator.ApplyComplaintTransitionInput) (orchestrator.ApplyComplaintTransitionResult, error) {
	if in.HumanReviewID != nil && !in.Kind.AllowsHumanReviewID() {
		return orchestrator.ApplyComplaintTransitionResult{}, errors.New("human review id is only valid for approve or override complaint transitions")
	}

	tenantUUID, err := parseUUID(in.TenantID)
	if err != nil {
		return orchestrator.ApplyComplaintTransitionResult{}, err
	}
	caseUUID, err := parseUUID(in.ComplaintCaseID)
	if err != nil {
		return orchestrator.ApplyComplaintTransitionResult{}, err
	}

	var result orchestrator.ApplyComplaintTransitionResult
	err = tenantdb.WithTenantTx(ctx, s.db, in.TenantID, func(ctx context.Context, tx tenantdb.Tx) error {
		q := vigiaDB.New(tx)
		current, err := q.GetComplaintCase(ctx, vigiaDB.GetComplaintCaseParams{TenantID: tenantUUID, ID: caseUUID})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil
			}
			return err
		}

		plan, ok := orchestrator.PlanComplaintTransition(orchestrator.ComplaintCaseState(current.State), in.Kind)
		if !ok {
			return nil
		}

		humanReviewUUID, hasHumanReview, err := validateHumanReviewForResolution(ctx, q, tenantUUID, caseUUID, in)
		if err != nil {
			return err
		}
		if isHumanResolutionTransition(in.Kind) && !hasHumanReview {
			return nil
		}

		updated, err := applyComplaintCaseCAS(ctx, q, tenantUUID, caseUUID, plan, in)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil
			}
			return err
		}

		if hasHumanReview {
			processedAt := pgtype.Timestamptz{Time: effectiveNow(in.Now), Valid: true}
			if _, err := q.MarkWinningHumanReviewProcessed(ctx, vigiaDB.MarkWinningHumanReviewProcessedParams{TenantID: tenantUUID, ComplaintCaseID: caseUUID, ID: humanReviewUUID, ProcessedAt: processedAt}); err != nil {
				return err
			}
			if _, err := q.MarkOtherHumanReviewsSuperseded(ctx, vigiaDB.MarkOtherHumanReviewsSupersededParams{TenantID: tenantUUID, ComplaintCaseID: caseUUID, ID: humanReviewUUID, SupersededAt: processedAt}); err != nil {
				return err
			}
		}

		var evidenceHumanReviewID *string
		if in.Kind.AllowsHumanReviewID() {
			evidenceHumanReviewID = in.HumanReviewID
		}
		if err := appendComplaintEvidence(ctx, q, tenantUUID, in.ComplaintCaseID, in.Kind, plan.From, plan.To, evidenceHumanReviewID, effectiveNow(in.Now)); err != nil {
			return err
		}
		result = orchestrator.ApplyComplaintTransitionResult{Applied: true, Case: complaintCaseFromRow(updated, false)}
		return nil
	})
	if err != nil {
		return orchestrator.ApplyComplaintTransitionResult{}, err
	}
	return result, nil
}

func validateHumanReviewForResolution(ctx context.Context, q *vigiaDB.Queries, tenantUUID, caseUUID pgtype.UUID, in orchestrator.ApplyComplaintTransitionInput) (pgtype.UUID, bool, error) {
	if !isHumanResolutionTransition(in.Kind) {
		return pgtype.UUID{}, false, nil
	}
	if in.HumanReviewID == nil {
		return pgtype.UUID{}, false, nil
	}
	humanReviewUUID, err := parseUUID(*in.HumanReviewID)
	if err != nil {
		return pgtype.UUID{}, false, err
	}
	if _, err := q.GetUnprocessedHumanReviewForCase(ctx, vigiaDB.GetUnprocessedHumanReviewForCaseParams{TenantID: tenantUUID, ComplaintCaseID: caseUUID, ID: humanReviewUUID, Decision: string(in.Kind)}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return pgtype.UUID{}, false, nil
		}
		return pgtype.UUID{}, false, err
	}
	return humanReviewUUID, true, nil
}

func isHumanResolutionTransition(kind orchestrator.ComplaintTransitionKind) bool {
	return kind.AllowsHumanReviewID()
}

func applyComplaintCaseCAS(ctx context.Context, q *vigiaDB.Queries, tenantUUID, caseUUID pgtype.UUID, plan orchestrator.ComplaintTransitionPlan, in orchestrator.ApplyComplaintTransitionInput) (vigiaDB.ComplaintCase, error) {
	switch in.Kind {
	case orchestrator.TransitionRequestReview:
		return q.TransitionComplaintCaseToAwaitingReview(ctx, vigiaDB.TransitionComplaintCaseToAwaitingReviewParams{
			TenantID: tenantUUID, ID: caseUUID, ReviewExpiresAt: pgtype.Timestamptz{Time: in.ReviewExpiresAt.UTC().Truncate(time.Microsecond), Valid: true},
		})
	case orchestrator.TransitionApprove, orchestrator.TransitionOverride:
		return q.TransitionComplaintCaseToResolved(ctx, vigiaDB.TransitionComplaintCaseToResolvedParams{
			TenantID: tenantUUID, ID: caseUUID, ResolvedAt: pgtype.Timestamptz{Time: effectiveNow(in.Now), Valid: true},
		})
	case orchestrator.TransitionTTLExpired, orchestrator.TransitionSLABreach:
		return q.TransitionComplaintCaseToEscalated(ctx, vigiaDB.TransitionComplaintCaseToEscalatedParams{
			TenantID: tenantUUID, ID: caseUUID, FromStates: []string{string(plan.From)},
		})
	default:
		return vigiaDB.ComplaintCase{}, pgx.ErrNoRows
	}
}

func appendComplaintEvidence(ctx context.Context, q *vigiaDB.Queries, tenantUUID pgtype.UUID, complaintCaseID string, kind orchestrator.ComplaintTransitionKind, from orchestrator.ComplaintCaseState, to orchestrator.ComplaintCaseState, humanReviewID *string, at time.Time) error {
	caseUUID, err := parseUUID(complaintCaseID)
	if err != nil {
		return err
	}
	var humanReviewUUID pgtype.UUID
	if humanReviewID != nil {
		humanReviewUUID, err = parseUUID(*humanReviewID)
		if err != nil {
			return err
		}
	}

	head, err := q.LockChainHead(ctx, vigiaDB.LockChainHeadParams{TenantID: tenantUUID, LastHash: ledger.GenesisPrevHash})
	if err != nil {
		return err
	}
	seq := head.LastSeq + 1
	prevHash := head.LastHash
	createdAt := at.UTC().Truncate(time.Microsecond)
	kindString := string(kind)
	fromString := string(from)
	toString := string(to)
	body := ledger.Body{
		TenantID:       uuidString(tenantUUID),
		Seq:            seq,
		OverallOutcome: toString,
		InputsDigest:   "",
		CreatedAt:      createdAt,
		ComplaintTransition: &ledger.ComplaintTransitionEvidence{
			ComplaintCaseID: complaintCaseID,
			TransitionKind:  kindString,
			FromState:       fromString,
			ToState:         toString,
			HumanReviewID:   humanReviewID,
		},
	}
	hash := ledger.Hash(prevHash, body)
	if _, err := q.InsertComplaintTransitionEvidenceRecord(ctx, vigiaDB.InsertComplaintTransitionEvidenceRecordParams{
		TenantID:            tenantUUID,
		Seq:                 seq,
		PrevHash:            prevHash,
		Hash:                hash,
		OverallOutcome:      body.OverallOutcome,
		InputsDigest:        body.InputsDigest,
		CreatedAt:           pgtype.Timestamptz{Time: createdAt, Valid: true},
		ComplaintCaseID:     caseUUID,
		TransitionKind:      &kindString,
		TransitionFromState: &fromString,
		TransitionToState:   &toString,
		HumanReviewID:       humanReviewUUID,
	}); err != nil {
		return err
	}
	return q.UpdateChainHead(ctx, vigiaDB.UpdateChainHeadParams{TenantID: tenantUUID, LastSeq: seq, LastHash: hash})
}

func complaintCaseMatchesCreateInput(row vigiaDB.ComplaintCase, interactionUUID pgtype.UUID, in orchestrator.CreateComplaintCaseInput) bool {
	return row.InteractionID == interactionUUID && row.RedecoCause == in.RedecoCause
}

func complaintCasesFromRows(rows []vigiaDB.ComplaintCase) []orchestrator.ComplaintCase {
	result := make([]orchestrator.ComplaintCase, 0, len(rows))
	for _, row := range rows {
		result = append(result, complaintCaseFromRow(row, false))
	}
	return result
}

func humanReviewsFromRows(rows []vigiaDB.HumanReview) []orchestrator.HumanReview {
	result := make([]orchestrator.HumanReview, 0, len(rows))
	for _, row := range rows {
		result = append(result, humanReviewFromRow(row))
	}
	return result
}

func humanReviewFromRow(row vigiaDB.HumanReview) orchestrator.HumanReview {
	return orchestrator.HumanReview{
		ID:              uuidString(row.ID),
		TenantID:        uuidString(row.TenantID),
		ComplaintCaseID: uuidString(row.ComplaintCaseID),
		Decision:        row.Decision,
		Reviewer:        row.Reviewer,
		Notes:           row.Notes,
		CreatedAt:       row.CreatedAt.Time,
	}
}

func complaintCaseFromRow(row vigiaDB.ComplaintCase, created bool) orchestrator.ComplaintCase {
	return orchestrator.ComplaintCase{
		ID:              uuidString(row.ID),
		TenantID:        uuidString(row.TenantID),
		InteractionID:   uuidString(row.InteractionID),
		RedecoCause:     row.RedecoCause,
		State:           row.State,
		OpenedAt:        row.OpenedAt.Time,
		SLADueAt:        row.SlaDueAt.Time,
		CalendarVersion: row.CalendarVersion,
		ReviewExpiresAt: timePtr(row.ReviewExpiresAt),
		ResolvedAt:      timePtr(row.ResolvedAt),
		IdempotencyKey:  row.IdempotencyKey,
		CreatedAt:       row.CreatedAt.Time,
		UpdatedAt:       row.UpdatedAt.Time,
		Created:         created,
	}
}

func timePtr(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	v := value.Time
	return &v
}

func effectiveNow(value time.Time) time.Time {
	if value.IsZero() {
		return time.Now().UTC().Truncate(time.Microsecond)
	}
	return value.UTC().Truncate(time.Microsecond)
}
