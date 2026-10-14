package provisioning

import (
	"context"
	"testing"
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
			oldSignalLifecycleEventDelay := signalLifecycleEventDelay
			signalLifecycleEventDelay = 0
			defer func() {
				signalLifecycleEventDelay = oldSignalLifecycleEventDelay
			}()

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

			select {
			case <-signaled:
			case <-time.After(time.Second):
				t.Fatal("Lifecycle signal not emitted")
			}

			// Assert
			tc.assertLog(t, logBuf)
		})
	}
}
