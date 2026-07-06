package orchestrator

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
)

const (
	RedecoMonthlyReportJobKind = "redeco_monthly_report"

	PenalizationPenalized  = "penalized"
	PenalizationCleared    = "cleared"
	PenalizationOverridden = "overridden"
)

type RedecoReportPeriod struct {
	Year  int
	Month time.Month
}

func NewRedecoReportPeriod(year int, month time.Month) (RedecoReportPeriod, error) {
	if year <= 0 {
		return RedecoReportPeriod{}, fmt.Errorf("invalid report year %d", year)
	}
	if month < time.January || month > time.December {
		return RedecoReportPeriod{}, fmt.Errorf("invalid report month %d", month)
	}
	return RedecoReportPeriod{Year: year, Month: month}, nil
}

func PreviousMonth(now time.Time) RedecoReportPeriod {
	firstOfMonth := time.Date(now.UTC().Year(), now.UTC().Month(), 1, 0, 0, 0, 0, time.UTC)
	previous := firstOfMonth.AddDate(0, -1, 0)
	return RedecoReportPeriod{Year: previous.Year(), Month: previous.Month()}
}

func (p RedecoReportPeriod) Bounds() (time.Time, time.Time) {
	start := time.Date(p.Year, p.Month, 1, 0, 0, 0, 0, time.UTC)
	return start, start.AddDate(0, 1, 0)
}

type RedecoMonthlyReportEntry struct {
	ComplaintCaseID string
	InteractionID   string
	DespachoID      *string
	DespachoName    string
	Channel         string
	Cause           string
	Status          string
	Resolution      string
	Penalization    string
	OccurredAt      time.Time
	ClosedAt        time.Time
}

type RedecoMonthlyReport struct {
	TenantID string
	Period   RedecoReportPeriod
	Entries  []RedecoMonthlyReportEntry
	CSV      []byte
}

type RedecoMonthlyReportStore interface {
	ListRedecoReportTenants(ctx context.Context) ([]string, error)
	GenerateRedecoMonthlyReport(ctx context.Context, tenantID string, period RedecoReportPeriod) (RedecoMonthlyReport, error)
}

type RedecoMonthlyReportArgs struct {
	TenantID string `json:"tenant_id,omitempty" river:"unique"`
	Year     int    `json:"year" river:"unique"`
	Month    int    `json:"month" river:"unique"`
}

func (RedecoMonthlyReportArgs) Kind() string { return RedecoMonthlyReportJobKind }

func (a RedecoMonthlyReportArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{UniqueOpts: river.UniqueOpts{ByArgs: true}}
}

type RedecoMonthlyReportWorker struct {
	river.WorkerDefaults[RedecoMonthlyReportArgs]
	store    RedecoMonthlyReportStore
	settings ComplaintJobSettings
}

func NewRedecoMonthlyReportWorker(store RedecoMonthlyReportStore, settings ComplaintJobSettings) *RedecoMonthlyReportWorker {
	return &RedecoMonthlyReportWorker{store: store, settings: settings}
}

func (w *RedecoMonthlyReportWorker) Work(ctx context.Context, job *river.Job[RedecoMonthlyReportArgs]) error {
	period, err := reportPeriodFromArgs(job.Args, w.settings.now())
	if err != nil {
		return err
	}
	if job.Args.TenantID != "" {
		_, err := w.store.GenerateRedecoMonthlyReport(ctx, job.Args.TenantID, period)
		return err
	}
	tenants, err := w.store.ListRedecoReportTenants(ctx)
	if err != nil {
		return err
	}
	var errs []error
	for _, tenantID := range tenants {
		if _, err := w.store.GenerateRedecoMonthlyReport(ctx, tenantID, period); err != nil {
			errs = append(errs, fmt.Errorf("generate tenant %s: %w", tenantID, err))
		}
	}
	return errors.Join(errs...)
}

func reportPeriodFromArgs(args RedecoMonthlyReportArgs, now time.Time) (RedecoReportPeriod, error) {
	if args.Year == 0 && args.Month == 0 {
		return PreviousMonth(now), nil
	}
	return NewRedecoReportPeriod(args.Year, time.Month(args.Month))
}

func NewRedecoMonthlyReportPeriodicJob() *river.PeriodicJob {
	return river.NewPeriodicJob(
		river.PeriodicInterval(24*time.Hour),
		func() (river.JobArgs, *river.InsertOpts) {
			period := PreviousMonth(time.Now())
			args := RedecoMonthlyReportArgs{Year: period.Year, Month: int(period.Month)}
			opts := args.InsertOpts()
			return args, &opts
		},
		&river.PeriodicJobOpts{RunOnStart: true},
	)
}

func ExportRedecoMonthlyReportCSV(report RedecoMonthlyReport) ([]byte, error) {
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	if err := writer.Write([]string{
		"tenant_id",
		"period_year",
		"period_month",
		"complaint_case_id",
		"interaction_id",
		"despacho_id",
		"despacho_name",
		"channel",
		"cause",
		"status",
		"resolution",
		"penalization",
		"occurred_at",
		"closed_at",
	}); err != nil {
		return nil, err
	}
	for _, entry := range report.Entries {
		despachoID := ""
		if entry.DespachoID != nil {
			despachoID = *entry.DespachoID
		}
		if err := writer.Write([]string{
			report.TenantID,
			strconv.Itoa(report.Period.Year),
			strconv.Itoa(int(report.Period.Month)),
			entry.ComplaintCaseID,
			entry.InteractionID,
			despachoID,
			entry.DespachoName,
			entry.Channel,
			entry.Cause,
			entry.Status,
			entry.Resolution,
			entry.Penalization,
			entry.OccurredAt.UTC().Format(time.RFC3339),
			entry.ClosedAt.UTC().Format(time.RFC3339),
		}); err != nil {
			return nil, err
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func EnqueueRedecoMonthlyReport(ctx context.Context, args RedecoMonthlyReportArgs) error {
	client := river.ClientFromContext[pgx.Tx](ctx)
	if client == nil {
		return fmt.Errorf("river client not found in worker context")
	}
	_, err := client.Insert(ctx, args, nil)
	return err
}
