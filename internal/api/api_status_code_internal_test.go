package api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/image"
	imageMock "github.com/FuturFusion/operations-center/internal/image/mock"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	provisioningMock "github.com/FuturFusion/operations-center/internal/provisioning/mock"
	"github.com/FuturFusion/operations-center/internal/security/authn"
	"github.com/FuturFusion/operations-center/internal/security/authz"
	"github.com/FuturFusion/operations-center/internal/warning"
	warningMock "github.com/FuturFusion/operations-center/internal/warning/mock"
	"github.com/FuturFusion/operations-center/shared/api"
)

func Test_updateAndActionHandlers_statusOK(t *testing.T) {
	t.Parallel()

	clusterService := &provisioningMock.ClusterServiceMock{
		GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Cluster, error) {
			return &provisioning.Cluster{Name: name}, nil
		},
		UpdateFunc: func(ctx context.Context, cluster provisioning.Cluster, updateServers bool) error {
			return nil
		},
		AddServersFunc: func(ctx context.Context, name string, serverNames []string, skipPostJoinOperations bool, copyServicesConfig bool) error {
			return nil
		},
		RemoveServerFunc: func(ctx context.Context, name string, removedServerNames []string, force bool) error {
			return nil
		},
	}

	registerCluster := func(router Router, authorizer *authz.Authorizer) {
		registerProvisioningClusterHandler(router, authorizer, clusterService, nil)
	}

	tests := []struct {
		name        string
		register    func(router Router, authorizer *authz.Authorizer)
		method      string
		target      string
		requestBody string
	}{
		{
			name: "incus image put",
			register: func(router Router, authorizer *authz.Authorizer) {
				registerImageIncusHandler(router, router, authorizer, &imageMock.ImageIncusServiceMock{
					GetByNameFunc: func(ctx context.Context, name string) (*image.IncusImage, error) {
						return &image.IncusImage{Name: name}, nil
					},
					UpdateFunc: func(ctx context.Context, incusImage image.IncusImage) error {
						return nil
					},
				}, forwarder{clustered: func() bool { return false }})
			},
			method:      http.MethodPut,
			target:      "/one",
			requestBody: `{"description": "some description"}`,
		},
		{
			name: "image source put",
			register: func(router Router, authorizer *authz.Authorizer) {
				registerImageSourceHandler(router, authorizer, &imageMock.IncusImageSourceServiceMock{
					GetByNameFunc: func(ctx context.Context, name string) (*image.IncusImageSource, error) {
						return &image.IncusImageSource{Name: name}, nil
					},
					UpdateFunc: func(ctx context.Context, source image.IncusImageSource) error {
						return nil
					},
				})
			},
			method:      http.MethodPut,
			target:      "/one",
			requestBody: `{"url": "https://images.example.com"}`,
		},
		{
			name: "channel put",
			register: func(router Router, authorizer *authz.Authorizer) {
				registerChannelsHandler(router, authorizer, &provisioningMock.ChannelServiceMock{
					GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Channel, error) {
						return &provisioning.Channel{Name: name}, nil
					},
					UpdateFunc: func(ctx context.Context, newChannel provisioning.Channel) error {
						return nil
					},
				})
			},
			method:      http.MethodPut,
			target:      "/one",
			requestBody: `{"description": "some description"}`,
		},
		{
			name:        "cluster put",
			register:    registerCluster,
			method:      http.MethodPut,
			target:      "/one",
			requestBody: `{"description": "some description"}`,
		},
		{
			name:        "cluster add servers post",
			register:    registerCluster,
			method:      http.MethodPost,
			target:      "/one/:add-servers",
			requestBody: `{"server_names": ["server1"]}`,
		},
		{
			name:        "cluster remove servers post",
			register:    registerCluster,
			method:      http.MethodPost,
			target:      "/one/:remove-servers",
			requestBody: `{"server_names": ["server1"]}`,
		},
		{
			name: "cluster template put",
			register: func(router Router, authorizer *authz.Authorizer) {
				registerProvisioningClusterTemplateHandler(router, authorizer, &provisioningMock.ClusterTemplateServiceMock{
					GetByNameFunc: func(ctx context.Context, name string) (*provisioning.ClusterTemplate, error) {
						return &provisioning.ClusterTemplate{Name: name}, nil
					},
					UpdateFunc: func(ctx context.Context, clusterTemplate provisioning.ClusterTemplate) error {
						return nil
					},
				})
			},
			method:      http.MethodPut,
			target:      "/one",
			requestBody: `{"description": "some description"}`,
		},
		{
			name: "warning put",
			register: func(router Router, authorizer *authz.Authorizer) {
				registerWarningHandler(router, authorizer, &warningMock.WarningServiceMock{
					GetByUUIDFunc: func(ctx context.Context, id uuid.UUID) (*warning.Warning, error) {
						return &warning.Warning{UUID: id}, nil
					},
					UpdateStatusByUUIDFunc: func(ctx context.Context, id uuid.UUID, status api.WarningStatus) (*warning.Warning, error) {
						return &warning.Warning{UUID: id}, nil
					},
				})
			},
			method:      http.MethodPut,
			target:      "/b32d0079-c48b-4957-b1cb-bef54125c861",
			requestBody: `{"status": "acknowledged"}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			authenticator := authn.New([]authn.Auther{dummyAuthenticator{}})

			serveMux := http.NewServeMux()
			router := newRouter(serveMux).AddMiddlewares(
				authenticator.Middleware(),
			)

			var authorizer authz.Authorizer = noopAuthorizer{}
			tc.register(router, &authorizer)

			server := httptest.NewServer(serveMux)
			t.Cleanup(server.Close)

			req, err := http.NewRequest(tc.method, server.URL+tc.target, strings.NewReader(tc.requestBody))
			require.NoError(t, err)

			// Run test
			resp, err := server.Client().Do(req)
			require.NoError(t, err)

			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			// Assert
			require.Equal(t, http.StatusOK, resp.StatusCode, "an update or an action does not create a resource")
			require.Empty(t, resp.Header.Get("Location"), "only a response for a created resource has a location")
			require.Contains(t, string(body), `"status_code":200`)
		})
	}
}
