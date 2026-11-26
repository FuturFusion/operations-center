package cluster

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/util/testing/boom"
	"github.com/FuturFusion/operations-center/internal/util/testing/errassert"
)

// errClusterBusy is the cowsql error while Raft runs another configuration change.
var errClusterBusy = errors.New("a configuration change is already in progress (5)")

func TestMembership_leave_retry(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// answer returns the result of the raft removal at the given attempt.
		answer func(attempt int) error
		// cancelAfter cancels the context of the removal after this duration.
		cancelAfter time.Duration

		wantAttempts int
		wantErr      require.ErrorAssertionFunc
	}{
		{
			name:        "another error is not retried",
			answer:      func(int) error { return boom.Error },
			cancelAfter: time.Hour,

			wantAttempts: 1,
			wantErr:      errassert.Is(boom.Error),
		},
		{
			name: "another error after a busy cluster ends the retries",
			answer: func(attempt int) error {
				if attempt < 3 {
					return errClusterBusy
				}

				return boom.Error
			},
			cancelAfter: time.Hour,

			wantAttempts: 3,
			wantErr:      errassert.Is(boom.Error),
		},
		{
			name:        "a cluster busy for all attempts is unavailable",
			answer:      func(int) error { return errClusterBusy },
			cancelAfter: time.Hour,

			wantAttempts: leaveAttempts,
			wantErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrUnavailable, a...)
				require.ErrorContains(tt, err, "Another change of the cluster members is in progress", a...)
			},
		},
		{
			name:        "a canceled removal stops retrying",
			answer:      func(int) error { return errClusterBusy },
			cancelAfter: leaveRetryWait + leaveRetryWait/2,

			wantAttempts: 2,
			wantErr:      errassert.Is(context.Canceled),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			synctest.Test(t, func(t *testing.T) {
				// Setup
				attempts := 0
				m := &membership{
					leaveRaft: func(string, bool, bool) (string, error) {
						attempts++
						return "", tc.answer(attempts)
					},
				}

				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()

				time.AfterFunc(tc.cancelAfter, cancel)

				// Run test
				address, err := m.leave(ctx, "member02", false, false)

				// Assert
				tc.wantErr(t, err, "the error of the removal")
				require.Empty(t, address, "a failed removal returns no address")
				require.Equal(t, tc.wantAttempts, attempts, "the number of removal attempts")
			})
		})
	}
}
