package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	vigiaDB "github.com/ricardoalt1515/vigia/internal/db"
	"github.com/ricardoalt1515/vigia/internal/orchestrator"
	"github.com/ricardoalt1515/vigia/internal/tenantdb"
)

type RedecoReportStore struct {
	db tenantdb.Beginner
}

func NewRedecoReportStore(db tenantdb.Beginner) *RedecoReportStore {
	return &RedecoReportStore{db: db}
}

func NewRedecoReportStoreFromPool(pool *pgxpool.Pool) *RedecoReportStore {
	return NewRedecoReportStore(poolBeginner{pool: pool})
}

func (s *RedecoReportStore) ListRedecoReportTenants(ctx context.Context) ([]string, error) {
	caseStore := NewComplaintCaseStore(s.db)
	return caseStore.ListComplaintPollTenants(ctx)
}

func (s *RedecoReportStore) GenerateRedecoMonthlyReport(ctx context.Context, tenantID string, period orchestrator.RedecoReportPeriod) (orchestrator.RedecoMonthlyReport, error) {
	tenantUUID, err := parseUUID(tenantID)
	if err != nil {
		return orchestrator.RedecoMonthlyReport{}, err
	}
	periodStart, periodEnd := period.Bounds()
	report := orchestrator.RedecoMonthlyReport{TenantID: tenantID, Period: period}
	err = tenantdb.WithTenantTx(ctx, s.db, tenantID, func(ctx context.Context, tx tenantdb.Tx) error {
		q := vigiaDB.New(tx)
		rows, err := q.ListRedecoMonthlyReportEntries(ctx, vigiaDB.ListRedecoMonthlyReportEntriesParams{
			TenantID:     tenantUUID,
			ResolvedAt:   timestamptz(periodStart),
			ResolvedAt_2: timestamptz(periodEnd),
		})
		if err != nil {
			return err
		}
		report.Entries = redecoEntriesFromRows(rows)
		if err := q.DeleteDespachoPenalizationsForPeriod(ctx, vigiaDB.DeleteDespachoPenalizationsForPeriodParams{
			TenantID:    tenantUUID,
			PeriodYear:  int32(period.Year),
			PeriodMonth: int32(period.Month),
		}); err != nil {
			return err
		}
		for _, row := range rows {
			if !row.DespachoID.Valid {
				continue
			}
			if _, err := q.UpsertDespachoPenalization(ctx, vigiaDB.UpsertDespachoPenalizationParams{
				TenantID:        tenantUUID,
				DespachoID:      row.DespachoID,
				ComplaintCaseID: row.ComplaintCaseID,
				PeriodYear:      int32(period.Year),
				PeriodMonth:     int32(period.Month),
				Penalization:    row.Penalization,
				Resolution:      row.Resolution,
				SourceState:     row.Status,
			}); err != nil {
				return err
			}
		}
		csvBytes, err := orchestrator.ExportRedecoMonthlyReportCSV(report)
		if err != nil {
			return err
		}
		report.CSV = csvBytes
		return nil
	})
	if err != nil {
		return orchestrator.RedecoMonthlyReport{}, err
	}
	return report, nil
}

func redecoEntriesFromRows(rows []vigiaDB.ListRedecoMonthlyReportEntriesRow) []orchestrator.RedecoMonthlyReportEntry {
	entries := make([]orchestrator.RedecoMonthlyReportEntry, 0, len(rows))
	for _, row := range rows {
		entries = append(entries, orchestrator.RedecoMonthlyReportEntry{
			ComplaintCaseID: uuidString(row.ComplaintCaseID),
			InteractionID:   uuidString(row.InteractionID),
			DespachoID:      uuidStringPtr(row.DespachoID),
			DespachoName:    row.DespachoName,
			Channel:         row.Channel,
			Cause:           row.Cause,
			Status:          row.Status,
			Resolution:      row.Resolution,
			Penalization:    row.Penalization,
			OccurredAt:      timeFromTimestamptz(row.OccurredAt),
			ClosedAt:        timeFromTimestamptz(row.ClosedAt),
		})
	}
	return entries
}

func timestamptz(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value.UTC(), Valid: true}
}

func timeFromTimestamptz(value pgtype.Timestamptz) time.Time {
	if !value.Valid {
		return time.Time{}
	}
	return value.Time
}
