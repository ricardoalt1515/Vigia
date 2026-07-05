package orchestrator

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/golang-sql/civil"
	"github.com/riverqueue/river"
)

func TestComplaintPollWorkerEnqueuesAllFindings(t *testing.T) {
	now := time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC)
	store := &fakeComplaintPollStore{
		tenants:            []string{"tenant-a"},
		openCases:          []ComplaintCase{{ID: "case-open", TenantID: "tenant-a"}},
		slaDueCases:        []ComplaintCase{{ID: "case-sla", TenantID: "tenant-a"}},
		expiredReviewCases: []ComplaintCase{{ID: "case-ttl", TenantID: "tenant-a"}},
		unprocessedReviews: []HumanReview{{ID: "review-approve", TenantID: "tenant-a", ComplaintCaseID: "case-review", Decision: string(TransitionApprove)}},
	}
	enqueuer := &recordingTransitionEnqueuer{}
	worker := NewComplaintPollWorker(store, enqueuer, ComplaintJobSettings{Now: func() time.Time { return now }})

	if err := worker.Work(context.Background(), &river.Job[ComplaintPollArgs]{}); err != nil {
		t.Fatalf("poll work: %v", err)
	}

	got := enqueuer.args
	want := []ComplaintTransitionArgs{
		{TenantID: "tenant-a", ComplaintCaseID: "case-open", TransitionKind: string(TransitionRequestReview)},
		{TenantID: "tenant-a", ComplaintCaseID: "case-sla", TransitionKind: string(TransitionSLABreach)},
		{TenantID: "tenant-a", ComplaintCaseID: "case-ttl", TransitionKind: string(TransitionTTLExpired)},
		{TenantID: "tenant-a", ComplaintCaseID: "case-review", TransitionKind: string(TransitionApprove), HumanReviewID: ptr("review-approve")},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("enqueued transitions mismatch\ngot:  %#v\nwant: %#v", got, want)
	}
}

func TestComplaintTransitionArgsUniqueOptsDedupeByCaseAndKindOnly(t *testing.T) {
	first := ComplaintTransitionArgs{TenantID: "tenant-a", ComplaintCaseID: "case-1", TransitionKind: string(TransitionApprove), HumanReviewID: ptr("review-1")}
	second := ComplaintTransitionArgs{TenantID: "tenant-a", ComplaintCaseID: "case-1", TransitionKind: string(TransitionApprove), HumanReviewID: ptr("review-2")}

	opts := first.InsertOpts()
	if !opts.UniqueOpts.ByArgs {
		t.Fatalf("ComplaintTransitionArgs.InsertOpts().UniqueOpts.ByArgs = false, want true")
	}
	if first.UniqueKey() != second.UniqueKey() {
		t.Fatalf("duplicate reviews for the same case/kind must share a unique key: %q != %q", first.UniqueKey(), second.UniqueKey())
	}

	differentKind := first
	differentKind.TransitionKind = string(TransitionOverride)
	if first.UniqueKey() == differentKind.UniqueKey() {
		t.Fatalf("different transition kinds must not dedupe together")
	}
}

func TestComplaintTransitionArgsRiverUniqueTagsAreCaseAndKindOnly(t *testing.T) {
	argsType := reflect.TypeOf(ComplaintTransitionArgs{})
	got := make(map[string]string)
	for i := 0; i < argsType.NumField(); i++ {
		field := argsType.Field(i)
		if field.Tag.Get("river") == "unique" {
			got[field.Name] = field.Tag.Get("json")
		}
	}

	want := map[string]string{
		"ComplaintCaseID": "complaint_case_id",
		"TransitionKind":  "transition_kind",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("river unique-tag fields mismatch\ngot:  %#v\nwant: %#v", got, want)
	}
}

func TestComplaintTransitionWorkerAppliesHumanReviewTransitionsIdempotently(t *testing.T) {
	store := &fakeTransitionStore{}
	worker := NewComplaintTransitionWorker(store, ComplaintJobSettings{Now: func() time.Time { return time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC) }})
	args := ComplaintTransitionArgs{TenantID: "tenant-a", ComplaintCaseID: "case-1", TransitionKind: string(TransitionApprove), HumanReviewID: ptr("review-winning")}

	if err := worker.Work(context.Background(), &river.Job[ComplaintTransitionArgs]{Args: args}); err != nil {
		t.Fatalf("first transition work: %v", err)
	}
	if err := worker.Work(context.Background(), &river.Job[ComplaintTransitionArgs]{Args: args}); err != nil {
		t.Fatalf("duplicate transition work should no-op successfully: %v", err)
	}

	if got, want := len(store.inputs), 2; got != want {
		t.Fatalf("ApplyTransition calls = %d, want %d", got, want)
	}
	if got := *store.inputs[0].HumanReviewID; got != "review-winning" {
		t.Fatalf("HumanReviewID passed to store = %q, want review-winning", got)
	}
	if store.inputs[0].Kind != TransitionApprove {
		t.Fatalf("Kind passed to store = %q, want %q", store.inputs[0].Kind, TransitionApprove)
	}
}

func TestComplaintTransitionWorkerComputesRequestReviewTTLWithDefaultBusinessDays(t *testing.T) {
	now := time.Date(2026, 7, 3, 12, 0, 0, 0, time.UTC) // Friday
	store := &fakeCalendarTransitionStore{
		caseRow: ComplaintCase{ID: "case-1", TenantID: "tenant-a", CalendarVersion: "mx-test"},
		holidays: []HolidayRow{
			{Version: "mx-test", Date: civil.Date{Year: 2026, Month: time.July, Day: 6}}, // Monday holiday extends default 3-business-day TTL.
			{Version: "other", Date: civil.Date{Year: 2026, Month: time.July, Day: 7}},
		},
	}
	worker := NewComplaintTransitionWorker(store, ComplaintJobSettings{Now: func() time.Time { return now }})
	args := ComplaintTransitionArgs{TenantID: "tenant-a", ComplaintCaseID: "case-1", TransitionKind: string(TransitionRequestReview)}

	if err := worker.Work(context.Background(), &river.Job[ComplaintTransitionArgs]{Args: args}); err != nil {
		t.Fatalf("request_review transition work: %v", err)
	}

	if got, want := len(store.inputs), 1; got != want {
		t.Fatalf("ApplyTransition calls = %d, want %d", got, want)
	}
	wantExpiresAt := time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC) // Tue/Wed/Thu after weekend + Monday holiday.
	if !store.inputs[0].ReviewExpiresAt.Equal(wantExpiresAt) {
		t.Fatalf("ReviewExpiresAt = %s, want %s", store.inputs[0].ReviewExpiresAt, wantExpiresAt)
	}
}

type fakeComplaintPollStore struct {
	tenants            []string
	openCases          []ComplaintCase
	slaDueCases        []ComplaintCase
	expiredReviewCases []ComplaintCase
	unprocessedReviews []HumanReview
}

func (s *fakeComplaintPollStore) ListComplaintPollTenants(context.Context) ([]string, error) {
	return s.tenants, nil
}
func (s *fakeComplaintPollStore) ListOpenComplaintCases(context.Context, string, int32) ([]ComplaintCase, error) {
	return s.openCases, nil
}
func (s *fakeComplaintPollStore) ListSLADueComplaintCases(context.Context, string, time.Time, int32) ([]ComplaintCase, error) {
	return s.slaDueCases, nil
}
func (s *fakeComplaintPollStore) ListExpiredReviewComplaintCases(context.Context, string, time.Time, int32) ([]ComplaintCase, error) {
	return s.expiredReviewCases, nil
}
func (s *fakeComplaintPollStore) ListUnprocessedHumanReviews(context.Context, string, int32) ([]HumanReview, error) {
	return s.unprocessedReviews, nil
}

type recordingTransitionEnqueuer struct{ args []ComplaintTransitionArgs }

func (e *recordingTransitionEnqueuer) EnqueueComplaintTransition(_ context.Context, args ComplaintTransitionArgs) error {
	e.args = append(e.args, args)
	return nil
}

type fakeTransitionStore struct {
	inputs []ApplyComplaintTransitionInput
}

func (s *fakeTransitionStore) ApplyTransition(_ context.Context, in ApplyComplaintTransitionInput) (ApplyComplaintTransitionResult, error) {
	s.inputs = append(s.inputs, in)
	if len(s.inputs) > 1 {
		return ApplyComplaintTransitionResult{Applied: false}, nil
	}
	return ApplyComplaintTransitionResult{Applied: true}, nil
}

type fakeCalendarTransitionStore struct {
	fakeTransitionStore
	caseRow  ComplaintCase
	holidays []HolidayRow
}

func (s *fakeCalendarTransitionStore) GetComplaintCase(_ context.Context, tenantID string, complaintCaseID string) (ComplaintCase, error) {
	if s.caseRow.TenantID != tenantID || s.caseRow.ID != complaintCaseID {
		return ComplaintCase{}, nil
	}
	return s.caseRow, nil
}

func (s *fakeCalendarTransitionStore) ListBusinessDayHolidays(_ context.Context, version string) ([]HolidayRow, error) {
	return s.holidays, nil
}

func ptr(s string) *string { return &s }
