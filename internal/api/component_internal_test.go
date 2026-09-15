package api

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/util/logger"
	apisystem "github.com/FuturFusion/operations-center/shared/api/system"
)

func TestComponentMiddleware(t *testing.T) {
	logBuf := &bytes.Buffer{}

	require.NoError(t, logger.InitLogger(logBuf, "", true, false, true))

	t.Cleanup(func() {
		require.NoError(t, logger.InitLogger(logBuf, "", true, false, true), "the log levels must not leak into other tests")
	})

	handler := componentMiddleware(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		slog.InfoContext(r.Context(), "handling")
	}))

	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/1.0", nil))

	require.Contains(t, logBuf.String(), "component=api", "a request is attributed to the api component")
}

func TestComponentDaemonTask(t *testing.T) {
	logBuf := &bytes.Buffer{}

	require.NoError(t, logger.InitLogger(logBuf, "", true, false, true))

	t.Cleanup(func() {
		require.NoError(t, logger.InitLogger(logBuf, "", true, false, true), "the log levels must not leak into other tests")
	})

	componentTask(componentTaskRefreshInventory, func(ctx context.Context) {
		slog.InfoContext(ctx, "task body")
	})(t.Context())

	require.Contains(t, logBuf.String(), "component=daemon.task.refresh_inventory", "a background task is attributed to its own component")
}

func TestValidateComponentLevels(t *testing.T) {
	require.NoError(t, validateComponentLevels(t.Context(), apisystem.Settings{
		SettingsPut: apisystem.SettingsPut{
			LogLevels: map[string]string{"daemon.task": "DEBUG"},
		},
	}), "a parent of a registered component is accepted")

	err := validateComponentLevels(t.Context(), apisystem.Settings{
		SettingsPut: apisystem.SettingsPut{
			LogLevels: map[string]string{"daemon.tsak": "DEBUG"},
		},
	})

	var validationErr domain.ErrValidation

	require.ErrorAs(t, err, &validationErr, "an unknown component is a validation error, so the API answers with 400")
	require.ErrorContains(t, err, `"daemon.tsak"`, "the rejection names the unknown component")
}
