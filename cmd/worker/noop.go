package main

import (
	"context"

	"github.com/riverqueue/river"
)

// NoopJob is a trivial River job used to prove the worker pipeline end-to-end.
// It carries no arguments and performs no domain work.
type NoopJob struct{}

// Kind implements river.JobArgs. The kind string is the stable discriminator
// River uses to route jobs to their worker.
func (NoopJob) Kind() string { return "noop" }

// NoopWorker handles NoopJob tasks by doing nothing and returning nil.
// It intentionally imports no domain packages (no httpapi, auth, harness).
type NoopWorker struct{ river.WorkerDefaults[NoopJob] }

// Work implements river.Worker[NoopJob]. Returns nil immediately.
func (NoopWorker) Work(_ context.Context, _ *river.Job[NoopJob]) error { return nil }
