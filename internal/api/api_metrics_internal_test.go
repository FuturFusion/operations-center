package api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	config "github.com/FuturFusion/operations-center/internal/config/daemon"
	"github.com/FuturFusion/operations-center/internal/environment/mock"
	"github.com/FuturFusion/operations-center/internal/security/authn"
	"github.com/FuturFusion/operations-center/internal/security/authz"
	"github.com/FuturFusion/operations-center/internal/util/metrics"
	"github.com/FuturFusion/operations-center/shared/api"
	apisystem "github.com/FuturFusion/operations-center/shared/api/system"
)

type forbiddenAuthorizer struct{}

func (f forbiddenAuthorizer) CheckPermission(ctx context.Context, details *authz.RequestDetails, object authz.Object, entitlement authz.Entitlement) error {
	return api.StatusErrorf(http.StatusForbidden, "User does not have entitlement %q on object %q", entitlement, object)
}

func Test_metricsGet(t *testing.T) {
	const openMetricsAccept = "application/openmetrics-text; version=1.0.0"

	tests := []struct {
		name              string
		metricsEnabled    bool
		metricsComponents []string
		authorizer        authz.Authorizer
		accept            string

		wantStatus      int
		wantContentType string
		assertBody      func(t *testing.T, body string)
	}{
		{
			name:              "success - text format",
			metricsEnabled:    true,
			metricsComponents: []string{"runtime"},
			authorizer:        noopAuthorizer{},

			wantStatus:      http.StatusOK,
			wantContentType: "text/plain",
			assertBody: func(t *testing.T, body string) {
				t.Helper()

				require.Contains(t, body, "go_goroutines")
				require.Contains(t, body, "promhttp_metric_handler_requests_total", "the metrics of the handler belong to the runtime component")
				require.NotContains(t, body, "# EOF", "the text format has no end marker")
			},
		},
		{
			name:              "success - OpenMetrics format",
			metricsEnabled:    true,
			metricsComponents: []string{"runtime"},
			authorizer:        noopAuthorizer{},
			accept:            openMetricsAccept,

			wantStatus:      http.StatusOK,
			wantContentType: "application/openmetrics-text",
			assertBody: func(t *testing.T, body string) {
				t.Helper()

				require.Contains(t, body, "operations_center_build_info")
				require.True(t, strings.HasSuffix(body, "# EOF\n"), "the OpenMetrics format ends with the end marker")
			},
		},
		{
			name:              "success - runtime component not enabled",
			metricsEnabled:    true,
			metricsComponents: nil,
			authorizer:        noopAuthorizer{},

			wantStatus:      http.StatusOK,
			wantContentType: "text/plain",
			assertBody: func(t *testing.T, body string) {
				t.Helper()

				require.Empty(t, body, "no component is enabled, so no metrics are exposed")
			},
		},
		{
			name:              "error - metrics disabled",
			metricsEnabled:    false,
			metricsComponents: []string{"runtime"},
			authorizer:        noopAuthorizer{},

			wantStatus:      http.StatusBadRequest,
			wantContentType: "application/json",
			assertBody: func(t *testing.T, body string) {
				t.Helper()

				require.Contains(t, body, "metrics_enabled", "the error hints at the setting")
				require.NotContains(t, body, "go_goroutines")
			},
		},
		{
			name:              "error - forbidden",
			metricsEnabled:    true,
			metricsComponents: []string{"runtime"},
			authorizer:        forbiddenAuthorizer{},

			wantStatus:      http.StatusForbidden,
			wantContentType: "application/json",
			assertBody: func(t *testing.T, body string) {
				t.Helper()

				require.Contains(t, body, "can_view_metrics", "the route asks for the metrics entitlement")
				require.NotContains(t, body, "go_goroutines")
			},
		},
		{
			name:              "error - forbidden while disabled",
			metricsEnabled:    false,
			metricsComponents: []string{"runtime"},
			authorizer:        forbiddenAuthorizer{},

			wantStatus:      http.StatusForbidden,
			wantContentType: "application/json",
			assertBody: func(t *testing.T, body string) {
				t.Helper()

				require.Contains(t, body, "can_view_metrics")
				require.NotContains(t, body, "metrics_enabled", "the permission check runs first, so the setting is not disclosed")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			config.InitTest(t, &mock.EnvironmentMock{
				IsIncusOSFunc: func() bool {
					return false
				},
			}, nil)
			err := config.UpdateSettings(t.Context(), apisystem.SettingsPut{
				LogLevel:          "WARN",
				MetricsEnabled:    tc.metricsEnabled,
				MetricsComponents: tc.metricsComponents,
			})
			require.NoError(t, err)

			// InitTest does not apply the default settings, so an update to the
			// same values is not seen as a change.
			metrics.Apply(tc.metricsEnabled, tc.metricsComponents)
			t.Cleanup(func() { metrics.Apply(false, nil) })

			authenticator := authn.New([]authn.Auther{dummyAuthenticator{}})

			serveMux := http.NewServeMux()
			router := newRouter(serveMux).AddMiddlewares(
				authenticator.Middleware(),
			)

			registerMetricsHandler(router.SubGroup("/metrics"), &tc.authorizer, metrics.Gatherer())

			server := httptest.NewServer(serveMux)
			t.Cleanup(server.Close)

			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL+"/metrics", http.NoBody)
			require.NoError(t, err)
			req.Header.Set("Accept", tc.accept)

			// Execute http request
			resp, err := server.Client().Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			// Assert
			require.Equal(t, tc.wantStatus, resp.StatusCode, string(body))
			require.Contains(t, resp.Header.Get("Content-Type"), tc.wantContentType)
			tc.assertBody(t, string(body))
		})
	}
}
