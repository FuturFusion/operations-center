package api

import (
	"context"
	"testing"
	"testing/synctest"
)

type waitUpgradeNotificationFunc func()

func (f waitUpgradeNotificationFunc) WaitUpgradeNotification() {
	f()
}

func Test_waitUpgradeNotification(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		ctx  func(t *testing.T) context.Context
		wait func(release <-chan struct{})
	}{
		{
			name: "notification received",
			ctx: func(t *testing.T) context.Context {
				t.Helper()
				return t.Context()
			},
			wait: func(release <-chan struct{}) {},
		},
		{
			name: "context canceled",
			ctx: func(t *testing.T) context.Context {
				t.Helper()
				ctx, cancel := context.WithCancel(t.Context())
				cancel()

				return ctx
			},
			wait: func(release <-chan struct{}) {
				<-release
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			synctest.Test(t, func(t *testing.T) {
				// Setup
				release := make(chan struct{})
				gateway := waitUpgradeNotificationFunc(func() {
					tc.wait(release)
				})

				// Run test
				waitUpgradeNotification(tc.ctx(t), gateway)

				// Assert
				// A wait that does not return makes synctest fail with a deadlock.
				close(release)
				synctest.Wait()
			})
		})
	}
}
