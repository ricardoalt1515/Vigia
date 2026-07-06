package orchestrator

import (
	"context"
	"encoding/csv"
	"strings"
	"testing"
	"time"

	"github.com/riverqueue/river"
)

func TestExportRedecoMonthlyReportCSVIncludesRequiredFields(t *testing.T) {
	despachoID := "9d830d02-7ccd-42be-8f10-63cbecb4048e"
	report := RedecoMonthlyReport{
		TenantID: "tenant-a",
		Period:   RedecoReportPeriod{Year: 2026, Month: time.June},
		Entries: []RedecoMonthlyReportEntry{{
			ComplaintCaseID: "case-1",
			InteractionID:   "interaction-1",
			DespachoID:      &despachoID,
			DespachoName:    "Despacho Norte",
			Channel:         "phone",
			Cause:           "MX-REDECO-05",
			Status:          "escalated",
			Resolution:      "escalated",
			Penalization:    PenalizationPenalized,
			OccurredAt:      time.Date(2026, 6, 2, 15, 4, 0, 0, time.UTC),
			ClosedAt:        time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC),
		}},
	}

	out, err := ExportRedecoMonthlyReportCSV(report)
	if err != nil {
		t.Fatalf("ExportRedecoMonthlyReportCSV() error = %v", err)
	}
	rows, err := csv.NewReader(strings.NewReader(string(out))).ReadAll()
	if err != nil {
		t.Fatalf("CSV is not parseable: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want header plus one entry", len(rows))
	}
	wantHeader := []string{"tenant_id", "period_year", "period_month", "complaint_case_id", "interaction_id", "despacho_id", "despacho_name", "channel", "cause", "status", "resolution", "penalization", "occurred_at", "closed_at"}
	for i, want := range wantHeader {
		if rows[0][i] != want {
			t.Fatalf("header[%d] = %q, want %q", i, rows[0][i], want)
		}
	}
	entry := rows[1]
	assertCSVField(t, entry, 7, "phone")
	assertCSVField(t, entry, 8, "MX-REDECO-05")
	assertCSVField(t, entry, 9, "escalated")
	assertCSVField(t, entry, 10, "escalated")
	assertCSVField(t, entry, 11, "penalized")
}

func TestPreviousMonthUsesClosedMonthlyPeriod(t *testing.T) {
	period := PreviousMonth(time.Date(2026, time.January, 7, 12, 0, 0, 0, time.UTC))
	if period.Year != 2025 || period.Month != time.December {
		t.Fatalf("PreviousMonth() = %04d-%02d, want 2025-12", period.Year, period.Month)
	}
	start, end := period.Bounds()
	if start != time.Date(2025, time.December, 1, 0, 0, 0, 0, time.UTC) {
		t.Fatalf("start = %s", start)
	}
	if end != time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC) {
		t.Fatalf("end = %s", end)
	}
}

func TestRedecoMonthlyReportWorkerGeneratesPreviousMonthForEachTenant(t *testing.T) {
	store := &fakeRedecoReportStore{tenants: []string{"tenant-a", "tenant-b"}}
	worker := NewRedecoMonthlyReportWorker(store, ComplaintJobSettings{Now: func() time.Time {
		return time.Date(2026, time.July, 6, 12, 0, 0, 0, time.UTC)
	}})

	if err := worker.Work(context.Background(), &river.Job[RedecoMonthlyReportArgs]{Args: RedecoMonthlyReportArgs{}}); err != nil {
		t.Fatalf("Work() error = %v", err)
	}

	if len(store.calls) != 2 {
		t.Fatalf("GenerateRedecoMonthlyReport calls = %d, want 2", len(store.calls))
	}
	for _, call := range store.calls {
		if call.period.Year != 2026 || call.period.Month != time.June {
			t.Fatalf("period = %04d-%02d, want 2026-06", call.period.Year, call.period.Month)
		}
	}
}

func TestRedecoMonthlyReportWorkerUsesExplicitTenantAndPeriod(t *testing.T) {
	store := &fakeRedecoReportStore{tenants: []string{"tenant-a", "tenant-b"}}
	worker := NewRedecoMonthlyReportWorker(store, ComplaintJobSettings{})

	args := RedecoMonthlyReportArgs{TenantID: "tenant-a", Year: 2026, Month: 5}
	if err := worker.Work(context.Background(), &river.Job[RedecoMonthlyReportArgs]{Args: args}); err != nil {
		t.Fatalf("Work() error = %v", err)
	}

	if store.listCalled {
		t.Fatal("ListRedecoReportTenants called for explicit tenant, want direct generation")
	}
	if len(store.calls) != 1 || store.calls[0].tenantID != "tenant-a" {
		t.Fatalf("calls = %#v, want one tenant-a generation", store.calls)
	}
	if store.calls[0].period.Year != 2026 || store.calls[0].period.Month != time.May {
		t.Fatalf("period = %04d-%02d, want 2026-05", store.calls[0].period.Year, store.calls[0].period.Month)
	}
}

type fakeRedecoReportStore struct {
	tenants    []string
	listCalled bool
	calls      []struct {
		tenantID string
		period   RedecoReportPeriod
	}
}

func (f *fakeRedecoReportStore) ListRedecoReportTenants(ctx context.Context) ([]string, error) {
	f.listCalled = true
	return f.tenants, nil
}

func (f *fakeRedecoReportStore) GenerateRedecoMonthlyReport(ctx context.Context, tenantID string, period RedecoReportPeriod) (RedecoMonthlyReport, error) {
	f.calls = append(f.calls, struct {
		tenantID string
		period   RedecoReportPeriod
	}{tenantID: tenantID, period: period})
	return RedecoMonthlyReport{TenantID: tenantID, Period: period}, nil
}

func assertCSVField(t *testing.T, row []string, index int, want string) {
	t.Helper()
	if row[index] != want {
		t.Fatalf("row[%d] = %q, want %q", index, row[index], want)
	}
}
