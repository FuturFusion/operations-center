package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/image"
	imageMock "github.com/FuturFusion/operations-center/internal/image/mock"
	provisioningMock "github.com/FuturFusion/operations-center/internal/provisioning/mock"
	"github.com/FuturFusion/operations-center/internal/security/authn"
	"github.com/FuturFusion/operations-center/internal/security/authz"
	"github.com/FuturFusion/operations-center/internal/warning"
	warningMock "github.com/FuturFusion/operations-center/internal/warning/mock"
	"github.com/FuturFusion/operations-center/shared/api"
)

func Test_memberRefresh_warnings(t *testing.T) {
	t.Parallel()

	id := uuid.MustParse("b32d0079-c48b-4957-b1cb-bef54125c861")
	boom := context.DeadlineExceeded

	tests := []struct {
		name        string
		register    func(router Router, authorizer *authz.Authorizer, warnings memberRefreshWarnings, refreshErr error)
		target      string
		requestBody string
		refreshErr  error

		wantScope api.WarningScope
	}{
		{
			name: "update refresh fails",
			register: func(router Router, authorizer *authz.Authorizer, warnings memberRefreshWarnings, refreshErr error) {
				registerInternalUpdateFilesHandler(router, authorizer, &provisioningMock.UpdateServiceMock{
					RefreshLocalFilesFunc: func(ctx context.Context, id uuid.UUID) error {
						return refreshErr
					},
				}, warnings)
			},
			target:     "/" + id.String() + "/:refresh",
			refreshErr: boom,

			wantScope: api.WarningScope{Scope: "member_refresh", EntityType: "update", Entity: id.String()},
		},
		{
			name: "update refresh succeeds",
			register: func(router Router, authorizer *authz.Authorizer, warnings memberRefreshWarnings, refreshErr error) {
				registerInternalUpdateFilesHandler(router, authorizer, &provisioningMock.UpdateServiceMock{
					RefreshLocalFilesFunc: func(ctx context.Context, id uuid.UUID) error {
						return refreshErr
					},
				}, warnings)
			},
			target: "/" + id.String() + "/:refresh",

			wantScope: api.WarningScope{Scope: "member_refresh", EntityType: "update", Entity: id.String()},
		},
		{
			name: "image refresh fails",
			register: func(router Router, authorizer *authz.Authorizer, warnings memberRefreshWarnings, refreshErr error) {
				registerInternalImageFilesHandler(router, authorizer, &imageMock.ImageIncusServiceMock{
					SyncLocalImageFunc: func(ctx context.Context, source image.IncusImageSource, img image.IncusImage) error {
						return refreshErr
					},
				}, &imageMock.IncusImageSourceServiceMock{
					GetByNameFunc: func(ctx context.Context, name string) (*image.IncusImageSource, error) {
						return &image.IncusImageSource{Name: name}, nil
					},
				}, warnings)
			},
			target:      "/debian:13:amd64:default/:refresh?source=images",
			requestBody: `{"name": "debian:13:amd64:default"}`,
			refreshErr:  boom,

			wantScope: api.WarningScope{Scope: "member_refresh", EntityType: "image", Entity: "debian:13:amd64:default"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			emitted := make(chan warning.Warning, 1)
			removed := make(chan api.WarningScope, 1)
			warnings := memberRefreshWarnings{
				emitter: &warningMock.WarningServiceMock{
					EmitFunc: func(ctx context.Context, w warning.Warning) {
						emitted <- w
					},
					RemoveStaleFunc: func(ctx context.Context, scope api.WarningScope, newWarnings warning.Warnings) {
						removed <- scope
					},
				},
				memberName: func(ctx context.Context) string { return "member2" },
			}

			authenticator := authn.New([]authn.Auther{dummyAuthenticator{}})

			serveMux := http.NewServeMux()
			router := newRouter(serveMux).AddMiddlewares(authenticator.Middleware())

			var authorizer authz.Authorizer = noopAuthorizer{}
			tc.register(router, &authorizer, warnings, tc.refreshErr)

			rec := httptest.NewRecorder()

			// Run test
			req := httptest.NewRequest(http.MethodPost, tc.target, strings.NewReader(tc.requestBody))
			req.Header.Set("Content-Type", "application/json")
			serveMux.ServeHTTP(rec, req)

			// Assert
			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

			if tc.refreshErr == nil {
				require.Equal(t, tc.wantScope, <-removed, "a successful refresh removes the warning of the member")
				return
			}

			w := <-emitted
			require.Equal(t, api.WarningTypeUpdateRefreshFailed, w.Type)
			require.Equal(t, tc.wantScope, api.WarningScope{Scope: w.Scope, EntityType: w.EntityType, Entity: w.Entity})
			require.Contains(t, strings.Join(w.Messages, " "), `cluster member "member2"`)
		})
	}
}
