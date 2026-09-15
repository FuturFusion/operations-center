package middleware_test

import (
	"bytes"
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/provisioning/middleware"
	"github.com/FuturFusion/operations-center/internal/provisioning/mock"
	"github.com/FuturFusion/operations-center/internal/util/logger"
)

// TestComponentFiltering covers the component wiring of the generated slog
// decorators, which is identical for all of them.
func TestComponentFiltering(t *testing.T) {
	logBuf := &bytes.Buffer{}

	require.NoError(t, logger.InitLogger(logBuf, "", false, false, true))

	t.Cleanup(func() {
		require.NoError(t, logger.InitLogger(logBuf, "", false, false, true), "the log levels must not leak into other tests")
	})

	channelSvc := middleware.NewChannelServiceWithSlog(&mock.ChannelServiceMock{
		GetAllFunc: func(ctx context.Context) (provisioning.Channels, error) {
			slog.DebugContext(ctx, "inside the channel service")

			return nil, nil
		},
	})

	tokenSvc := middleware.NewTokenServiceWithSlog(&mock.TokenServiceMock{
		GetAllFunc: func(_ context.Context) (provisioning.Tokens, error) {
			return nil, nil
		},
	})

	_, err := channelSvc.GetAll(t.Context())
	require.NoError(t, err)

	require.Empty(t, logBuf.String(), "the default log level WARN suppresses the debug output of the decorators")

	require.NoError(t, logger.SetComponentLevels(map[string]slog.Level{"provisioning.channel_service": slog.LevelDebug}))

	_, err = channelSvc.GetAll(t.Context())
	require.NoError(t, err)

	_, err = tokenSvc.GetAll(t.Context())
	require.NoError(t, err)

	out := logBuf.String()

	require.Contains(t, out, "component=provisioning.channel_service", "the decorator attributes its records to its component")
	require.Contains(t, out, "inside the channel service", "the wrapped implementation inherits the component from the context")
	require.NotContains(t, out, "component=provisioning.token_service", "raising one component does not affect the others")
}
