package task

import (
	"context"
	"time"
)

// Start a single task executing the given function with the given schedule.
//
// This is a convenience around Group and it returns two functions that can be
// used to control the task. The first is a "stop" function trying to terminate
// the task gracefully within the given timeout and the second is a "reset"
// function to reset the task's state. See Group.Stop() and Group.Reset() for
// more details.
func Start(ctx context.Context, f Func, schedule Schedule) (func(time.Duration) error, func()) {
	group := Group{}
	task := group.Add(f, schedule)
	group.Start(ctx)

	stop := group.Stop
	reset := task.Reset

	return stop, reset
}

type TaskFunc func(context.Context) func(context.Context) error

func New(f Func, schedule Schedule, stopDeadline time.Duration) TaskFunc {
	group := Group{}
	group.Add(f, schedule)
	return func(ctx context.Context) func(ctx context.Context) error {
		group.Start(ctx)
		stop := group.Stop

		return func(ctx context.Context) error {
			return stop(deadlineFrom(ctx, stopDeadline))
		}
	}
}

// deadlineFrom extracts the deadline from the provided context if present and not yet expired.
// Otherwise the defaultDeadline is returned.
func deadlineFrom(ctx context.Context, defaultDeadline time.Duration) time.Duration {
	deadline, ok := ctx.Deadline()
	if ok {
		deadlineDuration := time.Until(deadline)
		if deadlineDuration > 0 {
			return deadlineDuration
		}
	}

	return defaultDeadline
}
