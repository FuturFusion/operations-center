package server

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestBackgroundTasks_Stop(t *testing.T) {
	t.Parallel()

	s := &serverService{background: NewBackgroundTasks()}

	ended := make(chan struct{})
	s.runInBackground(t.Context(), func(ctx context.Context) {
		<-ctx.Done()
		close(ended)
	})

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	// A running task is cancelled and awaited.
	require.NoError(t, s.background.Stop(ctx))

	select {
	case <-ended:
	default:
		t.Fatal("Stop returned before the task ended")
	}

	// No task starts once the background tasks are stopped.
	s.runInBackground(t.Context(), func(ctx context.Context) {
		t.Error("A task started after the background tasks were stopped")
	})
}

func TestBackgroundTasks_Stop_deadline(t *testing.T) {
	t.Parallel()

	s := &serverService{background: NewBackgroundTasks()}

	release := make(chan struct{})
	defer close(release)

	// A task ignoring the cancellation holds the stop up until its deadline.
	s.runInBackground(t.Context(), func(ctx context.Context) {
		<-release
	})

	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()

	require.ErrorIs(t, s.background.Stop(ctx), context.DeadlineExceeded)
}
