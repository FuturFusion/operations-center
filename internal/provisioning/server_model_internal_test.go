package provisioning

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/lifecycle"
	"github.com/FuturFusion/operations-center/internal/util/logger"
	"github.com/FuturFusion/operations-center/internal/util/testing/boom"
	"github.com/FuturFusion/operations-center/internal/util/testing/log"
)

func TestServer_signalLifecycleEvent(t *testing.T) {
	tests := []struct {
		name                              string
		lifecycleServerLifecycleSignalErr error

		assertLog func(t *testing.T, logBuf *bytes.Buffer)
	}{
		{
			name: "success",

			assertLog: log.Empty,
		},
		{
			name:                              "error - lifecycle.ServerLifecycleSignal",
			lifecycleServerLifecycleSignalErr: boom.Error,

			assertLog: log.Contains("boom!"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			lifecycle.ServerLifecycleSignal.AddListenerWithErr(func(_ context.Context, _ lifecycle.ServerLifecycleMessage) error {
				return tc.lifecycleServerLifecycleSignalErr
			}, "TestServer_signalLifecycleEvent")
			t.Cleanup(func() {
				lifecycle.ServerLifecycleSignal.RemoveListener("TestServer_signalLifecycleEvent")
			})

			logBuf := &bytes.Buffer{}
			err := logger.InitLogger(logBuf, "", false, false, false)
			require.NoError(t, err)

			server := Server{}

			server.signalLifecycleEvent(t.Context())

			tc.assertLog(t, logBuf)
		})
	}
}
