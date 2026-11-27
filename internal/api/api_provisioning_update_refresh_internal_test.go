package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	config "github.com/FuturFusion/operations-center/internal/config/daemon"
	envMock "github.com/FuturFusion/operations-center/internal/environment/mock"
	provisioningMock "github.com/FuturFusion/operations-center/internal/provisioning/mock"
	"github.com/FuturFusion/operations-center/internal/security/authn"
	"github.com/FuturFusion/operations-center/internal/security/authz"
)

// Not parallel, since it changes the global config.
func Test_updatesRefreshPost_leaderTerm(t *testing.T) {
	tests := []struct {
		name     string
		isLeader bool

		wantStatus int
	}{
		{
			name:     "the leader refreshes in the background",
			isLeader: true,

			wantStatus: http.StatusAccepted,
		},
		{
			name: "a member without a leadership term refuses the refresh",

			wantStatus: http.StatusServiceUnavailable,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			config.InitTest(t, &envMock.EnvironmentMock{
				IsIncusOSFunc: func() bool { return false },
				ClusteredFunc: func() bool { return false },
			}, nil)

			updatesCfg := config.GetUpdates().UpdatesPut
			updatesCfg.Source = "https://updates.example.com"
			err := config.UpdateUpdates(t.Context(), updatesCfg)
			require.NoError(t, err)

			term, endTerm := context.WithCancel(context.Background())
			defer endTerm()

			refreshing := make(chan context.Context, 1)
			done := make(chan struct{})
			service := &provisioningMock.UpdateServiceMock{
				RefreshFunc: func(ctx context.Context) error {
					refreshing <- ctx
					<-ctx.Done()
					close(done)

					return ctx.Err()
				},
			}

			leaderContext := func(ctx context.Context) (context.Context, context.CancelFunc, bool) {
				if !tc.isLeader {
					return nil, nil, false
				}

				leaderCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
				stop := context.AfterFunc(term, cancel)

				return leaderCtx, func() {
					stop()
					cancel()
				}, true
			}

			authenticator := authn.New([]authn.Auther{dummyAuthenticator{}})

			serveMux := http.NewServeMux()
			router := newRouter(serveMux).AddMiddlewares(authenticator.Middleware())

			var authorizer authz.Authorizer = noopAuthorizer{}
			registerUpdateHandler(router, &authorizer, service, forwarder{}, leaderContext)

			rec := httptest.NewRecorder()

			// Run test
			req := httptest.NewRequest(http.MethodPost, "/:refresh", nil)
			req.Header.Set("Content-Type", "application/json")
			serveMux.ServeHTTP(rec, req)

			// Assert
			require.Equal(t, tc.wantStatus, rec.Code, rec.Body.String())

			if !tc.isLeader {
				require.Empty(t, service.RefreshCalls())
				return
			}

			// The refresh outlives the request and ends with the leadership term.
			ctx := <-refreshing
			require.NoError(t, ctx.Err())

			endTerm()

			select {
			case <-done:
			case <-time.After(10 * time.Second):
				t.Fatal("The refresh does not end with the leadership term")
			}
		})
	}
}
