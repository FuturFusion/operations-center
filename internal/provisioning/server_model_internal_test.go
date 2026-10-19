package provisioning

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/lifecycle"
	"github.com/FuturFusion/operations-center/internal/util/logger"
	"github.com/FuturFusion/operations-center/internal/util/testing/log"
)

func TestServer_SignalLifecycleEvent(t *testing.T) {
	tests := []struct {
		name string

		assertLog log.MatcherFunc
	}{
		{
			name: "success",

			assertLog: log.Empty,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				signaled := make(chan struct{})
				lifecycle.ServerLifecycleSignal.AddListener(func(_ context.Context, _ lifecycle.ServerLifecycleMessage) {
					close(signaled)
				}, "TestServer_SignalLifecycleEvent")
				t.Cleanup(func() {
					lifecycle.ServerLifecycleSignal.RemoveListener("TestServer_SignalLifecycleEvent")
				})

				logBuf := &log.SyncBuffer{}
				err := logger.InitLogger(logBuf, "", false, false, false)
				require.NoError(t, err)

				server := Server{}

				// Run test
				server.SignalLifecycleEvent()

				time.Sleep(signalLifecycleEventDelay - time.Nanosecond)
				synctest.Wait()
				require.False(t, isClosed(signaled), "Lifecycle signal emitted before the delay")

				time.Sleep(time.Nanosecond)
				synctest.Wait()

				// Assert
				require.True(t, isClosed(signaled), "Lifecycle signal not emitted after the delay")
				tc.assertLog(t, logBuf)
			})
		})
	}
}

func isClosed(ch chan struct{}) bool {
	select {
	case <-ch:
		return true

	default:
		return false
	}
}
