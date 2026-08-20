package api

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/provisioning"
	provisioningMock "github.com/FuturFusion/operations-center/internal/provisioning/mock"
	"github.com/FuturFusion/operations-center/internal/security/authn"
	"github.com/FuturFusion/operations-center/internal/security/authz"
)

const tokenUUID = "b32d0079-c48b-4957-b1cb-bef54125c861"

func Test_tokenHandler_tokenSeedsPost(t *testing.T) {
	tests := []struct {
		name        string
		requestBody string

		wantStatus              int
		wantResponseBodyContain string
		wantServiceCalled       bool
	}{
		{
			name: "success",
			requestBody: `{
  "name": "test",
  "description": "some description",
  "public": false,
  "seeds": {
    "network": {
      "version": "1"
    }
  }
}`,

			wantStatus:        http.StatusCreated,
			wantServiceCalled: true,
		},
		{
			name: "error - network seed nested in config block",
			requestBody: `{
  "name": "test",
  "seeds": {
    "network": {
      "config": {}
    }
  }
}`,

			wantStatus:              http.StatusBadRequest,
			wantResponseBodyContain: `unknown field \"config\"`,
		},
		{
			name: "error - fields only used by the UI form",
			requestBody: `{
  "name": "test",
  "seeds": {
    "application": "incus",
    "secondary_applications": []
  }
}`,

			wantStatus:              http.StatusBadRequest,
			wantResponseBodyContain: `unknown field \"application\"`,
		},
		{
			name: "error - incus seed is managed by operations center",
			requestBody: `{
  "name": "test",
  "seeds": {
    "incus": {
      "version": "1"
    }
  }
}`,

			wantStatus:              http.StatusBadRequest,
			wantResponseBodyContain: `unknown field \"incus\"`,
		},
		{
			name: "error - update seed is managed by operations center",
			requestBody: `{
  "name": "test",
  "seeds": {
    "update": {
      "version": "1"
    }
  }
}`,

			wantStatus:              http.StatusBadRequest,
			wantResponseBodyContain: `unknown field \"update\"`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var serviceCalled bool

			tokenService := &provisioningMock.TokenServiceMock{
				CreateTokenSeedFunc: func(ctx context.Context, tokenSeedConfig provisioning.TokenSeed) (provisioning.TokenSeed, error) {
					serviceCalled = true

					return tokenSeedConfig, nil
				},
			}

			body, status := doTokenRequest(t, tokenService, http.MethodPost, "/"+tokenUUID+"/seeds", tc.requestBody)

			require.Equal(t, tc.wantStatus, status)
			require.Equal(t, tc.wantServiceCalled, serviceCalled)

			if tc.wantResponseBodyContain != "" {
				require.Contains(t, body, tc.wantResponseBodyContain)
			}
		})
	}
}

func Test_tokenHandler_tokenSeedPut(t *testing.T) {
	tests := []struct {
		name        string
		requestBody string

		wantStatus              int
		wantResponseBodyContain string
	}{
		{
			name: "error - network seed nested in config block",
			requestBody: `{
  "description": "some description",
  "seeds": {
    "network": {
      "config": {}
    }
  }
}`,

			wantStatus:              http.StatusBadRequest,
			wantResponseBodyContain: `unknown field \"config\"`,
		},
		{
			name:        "error - name is not part of the update request",
			requestBody: `{"name": "test", "description": "some description"}`,

			wantStatus:              http.StatusBadRequest,
			wantResponseBodyContain: `unknown field \"name\"`,
		},
		{
			name: "error - incus seed is managed by operations center",
			requestBody: `{
  "seeds": {
    "incus": {
      "version": "1"
    }
  }
}`,

			wantStatus:              http.StatusBadRequest,
			wantResponseBodyContain: `unknown field \"incus\"`,
		},
		{
			name: "error - update seed is managed by operations center",
			requestBody: `{
  "seeds": {
    "update": {
      "version": "1"
    }
  }
}`,

			wantStatus:              http.StatusBadRequest,
			wantResponseBodyContain: `unknown field \"update\"`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var serviceCalled bool

			tokenService := &provisioningMock.TokenServiceMock{
				GetTokenSeedByNameFunc: func(ctx context.Context, id uuid.UUID, name string) (*provisioning.TokenSeed, error) {
					serviceCalled = true

					return &provisioning.TokenSeed{}, nil
				},
			}

			body, status := doTokenRequest(t, tokenService, http.MethodPut, "/"+tokenUUID+"/seeds/test", tc.requestBody)

			require.Equal(t, tc.wantStatus, status)
			require.False(t, serviceCalled)
			require.Contains(t, body, tc.wantResponseBodyContain)
		})
	}
}

func doTokenRequest(t *testing.T, tokenService provisioning.TokenService, method string, target string, requestBody string) (string, int) {
	t.Helper()

	authenticator := authn.New([]authn.Auther{dummyAuthenticator{}})

	serveMux := http.NewServeMux()
	router := newRouter(serveMux).AddMiddlewares(
		authenticator.Middleware(),
	)

	var authorizer authz.Authorizer = noopAuthorizer{}
	registerProvisioningTokenHandler(router, &authorizer, tokenService)

	server := httptest.NewServer(serveMux)
	t.Cleanup(server.Close)

	req, err := http.NewRequest(method, server.URL+target, bytes.NewBufferString(requestBody))
	require.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)

	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	return string(body), resp.StatusCode
}
