package api

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/provisioning/mock"
)

func TestLifecycleEventsMonitorTask(t *testing.T) {
	oldRetryMin, oldRetryMax := lifecycleMonitorRetryMin, lifecycleMonitorRetryMax
	lifecycleMonitorRetryMin, lifecycleMonitorRetryMax = time.Millisecond, time.Millisecond
	t.Cleanup(func() {
		lifecycleMonitorRetryMin, lifecycleMonitorRetryMax = oldRetryMin, oldRetryMax
	})

	tests := []struct {
		name     string
		startErr func(call int64) error

		waitForCalls int64
		assertCalls  func(t *testing.T, calls int64)
	}{
		{
			name: "success after two failures",
			startErr: func(call int64) error {
				if call <= 2 {
					return errors.New("boom")
				}

				return nil
			},

			waitForCalls: 3,
			assertCalls: func(t *testing.T, calls int64) {
				t.Helper()
				require.Equal(t, int64(3), calls, "expect no further start attempts after the first success")
			},
		},
		{
			name: "always failing",
			startErr: func(call int64) error {
				return errors.New("boom")
			},

			waitForCalls: 3,
			assertCalls: func(t *testing.T, calls int64) {
				t.Helper()
				require.GreaterOrEqual(t, calls, int64(3), "expect the start to be retried")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			var calls atomic.Int64
			clusterSvc := &mock.ClusterServiceMock{
				StartLifecycleEventsMonitorFunc: func(ctx context.Context) error {
					return tc.startErr(calls.Add(1))
				},
			}

			// Run test
			stop := lifecycleEventsMonitorTask(clusterSvc).Start(t.Context())
			require.Eventually(t, func() bool {
				return calls.Load() >= tc.waitForCalls
			}, time.Second, time.Millisecond, "expect the start to be retried until the wanted number of calls")

			stopCtx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()

			err := stop(stopCtx)
			callsAfterStop := calls.Load()
			time.Sleep(10 * time.Millisecond)

			// Assert
			require.NoError(t, err, "expect stop to return before the deadline")
			require.Equal(t, callsAfterStop, calls.Load(), "expect no start attempts after stop")
			tc.assertCalls(t, calls.Load())
		})
	}
}
