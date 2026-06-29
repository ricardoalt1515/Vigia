package main

import (
	"context"
	"testing"

	"github.com/riverqueue/river"
)

// TestNoopJobKind asserts that the job kind constant is stable and equals "noop".
func TestNoopJobKind(t *testing.T) {
	if got := (NoopJob{}).Kind(); got != "noop" {
		t.Fatalf("NoopJob.Kind() = %q, want %q", got, "noop")
	}
}

// TestNoopWorkerWork asserts that Work always returns nil and performs no side effects.
func TestNoopWorkerWork(t *testing.T) {
	w := &NoopWorker{}
	err := w.Work(context.Background(), &river.Job[NoopJob]{})
	if err != nil {
		t.Fatalf("NoopWorker.Work() = %v, want nil", err)
	}
}
