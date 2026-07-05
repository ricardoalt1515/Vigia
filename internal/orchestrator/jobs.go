package orchestrator

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
)

const (
	ComplaintPollJobKind       = "complaint_poll"
	ComplaintTransitionJobKind = "complaint_transition"

	DefaultComplaintPollLimit             int32 = 500
	DefaultComplaintReviewTTLBusinessDays int   = 3
)

type HumanReview struct {
	ID              string    `json:"id"`
	TenantID        string    `json:"tenant_id"`
	ComplaintCaseID string    `json:"complaint_case_id"`
	Decision        string    `json:"decision"`
	Reviewer        string    `json:"reviewer"`
	Notes           string    `json:"notes"`
	CreatedAt       time.Time `json:"created_at"`
}

type ComplaintJobSettings struct {
	PollLimit                      int32
	ComplaintReviewTTLBusinessDays int
	Now                            func() time.Time
}

func (s ComplaintJobSettings) pollLimit() int32 {
	if s.PollLimit > 0 {
		return s.PollLimit
	}
	return DefaultComplaintPollLimit
}

func (s ComplaintJobSettings) reviewTTLBusinessDays() int {
	if s.ComplaintReviewTTLBusinessDays > 0 {
		return s.ComplaintReviewTTLBusinessDays
	}
	return DefaultComplaintReviewTTLBusinessDays
}

func (s ComplaintJobSettings) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC().Truncate(time.Microsecond)
	}
	return time.Now().UTC().Truncate(time.Microsecond)
}

type ComplaintPollArgs struct{}

func (ComplaintPollArgs) Kind() string { return ComplaintPollJobKind }

type ComplaintTransitionArgs struct {
	TenantID        string  `json:"tenant_id"`
	ComplaintCaseID string  `json:"complaint_case_id" river:"unique"`
	TransitionKind  string  `json:"transition_kind" river:"unique"`
	HumanReviewID   *string `json:"human_review_id,omitempty"`
}

func (ComplaintTransitionArgs) Kind() string { return ComplaintTransitionJobKind }

func (a ComplaintTransitionArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{UniqueOpts: river.UniqueOpts{ByArgs: true}}
}

func (a ComplaintTransitionArgs) UniqueKey() string {
	return a.ComplaintCaseID + ":" + a.TransitionKind
}

type ComplaintPollStore interface {
	ListComplaintPollTenants(ctx context.Context) ([]string, error)
	ListOpenComplaintCases(ctx context.Context, tenantID string, limit int32) ([]ComplaintCase, error)
	ListSLADueComplaintCases(ctx context.Context, tenantID string, dueAt time.Time, limit int32) ([]ComplaintCase, error)
	ListExpiredReviewComplaintCases(ctx context.Context, tenantID string, expiresAt time.Time, limit int32) ([]ComplaintCase, error)
	ListUnprocessedHumanReviews(ctx context.Context, tenantID string, limit int32) ([]HumanReview, error)
}

type ComplaintTransitionStore interface {
	ApplyTransition(ctx context.Context, in ApplyComplaintTransitionInput) (ApplyComplaintTransitionResult, error)
}

type ComplaintTransitionCalendarStore interface {
	GetComplaintCase(ctx context.Context, tenantID string, complaintCaseID string) (ComplaintCase, error)
	ListBusinessDayHolidays(ctx context.Context, version string) ([]HolidayRow, error)
}

type ComplaintTransitionEnqueuer interface {
	EnqueueComplaintTransition(ctx context.Context, args ComplaintTransitionArgs) error
}

type ComplaintPollWorker struct {
	river.WorkerDefaults[ComplaintPollArgs]
	store    ComplaintPollStore
	enqueuer ComplaintTransitionEnqueuer
	settings ComplaintJobSettings
}

func NewComplaintPollWorker(store ComplaintPollStore, enqueuer ComplaintTransitionEnqueuer, settings ComplaintJobSettings) *ComplaintPollWorker {
	return &ComplaintPollWorker{store: store, enqueuer: enqueuer, settings: settings}
}

func (w *ComplaintPollWorker) Work(ctx context.Context, _ *river.Job[ComplaintPollArgs]) error {
	now := w.settings.now()
	limit := w.settings.pollLimit()
	tenants, err := w.store.ListComplaintPollTenants(ctx)
	if err != nil {
		return err
	}
	for _, tenantID := range tenants {
		if err := w.enqueueTenantFindings(ctx, tenantID, now, limit); err != nil {
			return err
		}
	}
	return nil
}

func (w *ComplaintPollWorker) enqueueTenantFindings(ctx context.Context, tenantID string, now time.Time, limit int32) error {
	openCases, err := w.store.ListOpenComplaintCases(ctx, tenantID, limit)
	if err != nil {
		return err
	}
	for _, item := range openCases {
		if err := w.enqueuer.EnqueueComplaintTransition(ctx, transitionArgs(item, TransitionRequestReview, nil)); err != nil {
			return err
		}
	}

	slaDueCases, err := w.store.ListSLADueComplaintCases(ctx, tenantID, now, limit)
	if err != nil {
		return err
	}
	for _, item := range slaDueCases {
		if err := w.enqueuer.EnqueueComplaintTransition(ctx, transitionArgs(item, TransitionSLABreach, nil)); err != nil {
			return err
		}
	}

	expiredCases, err := w.store.ListExpiredReviewComplaintCases(ctx, tenantID, now, limit)
	if err != nil {
		return err
	}
	for _, item := range expiredCases {
		if err := w.enqueuer.EnqueueComplaintTransition(ctx, transitionArgs(item, TransitionTTLExpired, nil)); err != nil {
			return err
		}
	}

	reviews, err := w.store.ListUnprocessedHumanReviews(ctx, tenantID, limit)
	if err != nil {
		return err
	}
	for _, review := range reviews {
		kind := ComplaintTransitionKind(review.Decision)
		if !kind.AllowsHumanReviewID() {
			continue
		}
		reviewID := review.ID
		args := ComplaintTransitionArgs{TenantID: review.TenantID, ComplaintCaseID: review.ComplaintCaseID, TransitionKind: string(kind), HumanReviewID: &reviewID}
		if err := w.enqueuer.EnqueueComplaintTransition(ctx, args); err != nil {
			return err
		}
	}
	return nil
}

func transitionArgs(item ComplaintCase, kind ComplaintTransitionKind, humanReviewID *string) ComplaintTransitionArgs {
	return ComplaintTransitionArgs{TenantID: item.TenantID, ComplaintCaseID: item.ID, TransitionKind: string(kind), HumanReviewID: humanReviewID}
}

type ComplaintTransitionWorker struct {
	river.WorkerDefaults[ComplaintTransitionArgs]
	store    ComplaintTransitionStore
	settings ComplaintJobSettings
}

func NewComplaintTransitionWorker(store ComplaintTransitionStore, settings ComplaintJobSettings) *ComplaintTransitionWorker {
	return &ComplaintTransitionWorker{store: store, settings: settings}
}

func (w *ComplaintTransitionWorker) Work(ctx context.Context, job *river.Job[ComplaintTransitionArgs]) error {
	kind := ComplaintTransitionKind(job.Args.TransitionKind)
	in := ApplyComplaintTransitionInput{
		TenantID:        job.Args.TenantID,
		ComplaintCaseID: job.Args.ComplaintCaseID,
		Kind:            kind,
		Now:             w.settings.now(),
		HumanReviewID:   job.Args.HumanReviewID,
	}
	if kind == TransitionRequestReview {
		calendarStore, ok := w.store.(ComplaintTransitionCalendarStore)
		if !ok {
			return fmt.Errorf("complaint transition store cannot load case calendar")
		}
		complaintCase, err := calendarStore.GetComplaintCase(ctx, job.Args.TenantID, job.Args.ComplaintCaseID)
		if err != nil {
			return err
		}
		holidayRows, err := calendarStore.ListBusinessDayHolidays(ctx, complaintCase.CalendarVersion)
		if err != nil {
			return err
		}
		in.ReviewExpiresAt = AddBusinessDays(in.Now, w.settings.reviewTTLBusinessDays(), LoadCalendar(complaintCase.CalendarVersion, holidayRows))
	}
	_, err := w.store.ApplyTransition(ctx, in)
	return err
}

type RiverContextTransitionEnqueuer struct{}

func (RiverContextTransitionEnqueuer) EnqueueComplaintTransition(ctx context.Context, args ComplaintTransitionArgs) error {
	client := river.ClientFromContext[pgx.Tx](ctx)
	if client == nil {
		return fmt.Errorf("river client not found in worker context")
	}
	_, err := client.Insert(ctx, args, nil)
	return err
}

func NewComplaintPeriodicJob() *river.PeriodicJob {
	return river.NewPeriodicJob(
		river.PeriodicInterval(60*time.Second),
		func() (river.JobArgs, *river.InsertOpts) { return ComplaintPollArgs{}, nil },
		&river.PeriodicJobOpts{RunOnStart: true},
	)
}
