package orchestrator

import (
	"context"
	"errors"
	"reflect"
	"strings"
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

func TestComplaintPollWorkerContinuesAfterTenantErrorAndReturnsAggregate(t *testing.T) {
	now := time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC)
	store := &fakeComplaintPollStore{
		tenants: []string{"tenant-fails", "tenant-continues"},
		openCasesByTenant: map[string][]ComplaintCase{
			"tenant-continues": {{ID: "case-continues", TenantID: "tenant-continues"}},
		},
		openErrByTenant: map[string]error{"tenant-fails": errors.New("open scan failed")},
	}
	enqueuer := &recordingTransitionEnqueuer{}
	worker := NewComplaintPollWorker(store, enqueuer, ComplaintJobSettings{Now: func() time.Time { return now }})

	err := worker.Work(context.Background(), &river.Job[ComplaintPollArgs]{})
	if err == nil || !strings.Contains(err.Error(), "tenant-fails") || !strings.Contains(err.Error(), "open scan failed") {
		t.Fatalf("poll work error = %v, want aggregate with failing tenant", err)
	}

	want := []ComplaintTransitionArgs{{TenantID: "tenant-continues", ComplaintCaseID: "case-continues", TransitionKind: string(TransitionRequestReview)}}
	if !reflect.DeepEqual(enqueuer.args, want) {
		t.Fatalf("enqueued transitions mismatch after tenant error\ngot:  %#v\nwant: %#v", enqueuer.args, want)
	}
}

func TestComplaintPollWorkerContinuesAfterScanAndEnqueueErrors(t *testing.T) {
	now := time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC)
	store := &fakeComplaintPollStore{
		tenants: []string{"tenant-a"},
		openCases: []ComplaintCase{
			{ID: "case-enqueue-fails", TenantID: "tenant-a"},
			{ID: "case-open-continues", TenantID: "tenant-a"},
		},
		slaDueErrByTenant: map[string]error{"tenant-a": errors.New("sla scan failed")},
		expiredReviewCases: []ComplaintCase{
			{ID: "case-ttl-continues", TenantID: "tenant-a"},
		},
		unprocessedReviews: []HumanReview{{ID: "review-approve", TenantID: "tenant-a", ComplaintCaseID: "case-review-continues", Decision: string(TransitionApprove)}},
	}
	enqueuer := &recordingTransitionEnqueuer{errByCaseID: map[string]error{"case-enqueue-fails": errors.New("enqueue failed")}}
	worker := NewComplaintPollWorker(store, enqueuer, ComplaintJobSettings{Now: func() time.Time { return now }})

	err := worker.Work(context.Background(), &river.Job[ComplaintPollArgs]{})
	if err == nil || !strings.Contains(err.Error(), "enqueue failed") || !strings.Contains(err.Error(), "sla scan failed") {
		t.Fatalf("poll work error = %v, want aggregate enqueue and scan errors", err)
	}

	want := []ComplaintTransitionArgs{
		{TenantID: "tenant-a", ComplaintCaseID: "case-enqueue-fails", TransitionKind: string(TransitionRequestReview)},
		{TenantID: "tenant-a", ComplaintCaseID: "case-open-continues", TransitionKind: string(TransitionRequestReview)},
		{TenantID: "tenant-a", ComplaintCaseID: "case-ttl-continues", TransitionKind: string(TransitionTTLExpired)},
		{TenantID: "tenant-a", ComplaintCaseID: "case-review-continues", TransitionKind: string(TransitionApprove), HumanReviewID: ptr("review-approve")},
	}
	if !reflect.DeepEqual(enqueuer.args, want) {
		t.Fatalf("enqueued transitions mismatch after scan/enqueue errors\ngot:  %#v\nwant: %#v", enqueuer.args, want)
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

	openCasesByTenant          map[string][]ComplaintCase
	slaDueCasesByTenant        map[string][]ComplaintCase
	expiredReviewCasesByTenant map[string][]ComplaintCase
	unprocessedReviewsByTenant map[string][]HumanReview

	openErrByTenant          map[string]error
	slaDueErrByTenant        map[string]error
	expiredReviewErrByTenant map[string]error
	reviewsErrByTenant       map[string]error
}

func (s *fakeComplaintPollStore) ListComplaintPollTenants(context.Context) ([]string, error) {
	return s.tenants, nil
}
func (s *fakeComplaintPollStore) ListOpenComplaintCases(_ context.Context, tenantID string, _ int32) ([]ComplaintCase, error) {
	if err := s.openErrByTenant[tenantID]; err != nil {
		return nil, err
	}
	if rows, ok := s.openCasesByTenant[tenantID]; ok {
		return rows, nil
	}
	return s.openCases, nil
}
func (s *fakeComplaintPollStore) ListSLADueComplaintCases(_ context.Context, tenantID string, _ time.Time, _ int32) ([]ComplaintCase, error) {
	if err := s.slaDueErrByTenant[tenantID]; err != nil {
		return nil, err
	}
	if rows, ok := s.slaDueCasesByTenant[tenantID]; ok {
		return rows, nil
	}
	return s.slaDueCases, nil
}
func (s *fakeComplaintPollStore) ListExpiredReviewComplaintCases(_ context.Context, tenantID string, _ time.Time, _ int32) ([]ComplaintCase, error) {
	if err := s.expiredReviewErrByTenant[tenantID]; err != nil {
		return nil, err
	}
	if rows, ok := s.expiredReviewCasesByTenant[tenantID]; ok {
		return rows, nil
	}
	return s.expiredReviewCases, nil
}
func (s *fakeComplaintPollStore) ListUnprocessedHumanReviews(_ context.Context, tenantID string, _ int32) ([]HumanReview, error) {
	if err := s.reviewsErrByTenant[tenantID]; err != nil {
		return nil, err
	}
	if rows, ok := s.unprocessedReviewsByTenant[tenantID]; ok {
		return rows, nil
	}
	return s.unprocessedReviews, nil
}

type recordingTransitionEnqueuer struct {
	args        []ComplaintTransitionArgs
	errByCaseID map[string]error
}

func (e *recordingTransitionEnqueuer) EnqueueComplaintTransition(_ context.Context, args ComplaintTransitionArgs) error {
	e.args = append(e.args, args)
	return e.errByCaseID[args.ComplaintCaseID]
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
