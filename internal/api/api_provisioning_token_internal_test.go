package api

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/lxc/incus-os/incus-osd/api/images"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/provisioning"
	provisioningMock "github.com/FuturFusion/operations-center/internal/provisioning/mock"
	"github.com/FuturFusion/operations-center/internal/security/authn"
	"github.com/FuturFusion/operations-center/internal/security/authz"
	"github.com/FuturFusion/operations-center/shared/api"
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

func Test_tokenHandler_tokenSeedImageGet(t *testing.T) {
	tests := []struct {
		name string
		path string

		wantStatus        int
		wantServiceCalled bool
		wantImageType     api.ImageType
		wantArchitecture  images.UpdateFileArchitecture
		wantChannel       string
	}{
		{
			name: "success - iso, no channel",
			path: "/" + tokenUUID + "/seeds/test/architecture/x86_64/type/iso/file.iso",

			wantStatus:        http.StatusOK,
			wantServiceCalled: true,
			wantImageType:     api.ImageTypeISO,
			wantArchitecture:  images.UpdateFileArchitecture64BitX86,
		},
		{
			name: "success - raw, with channel, keys reordered",
			path: "/" + tokenUUID + "/seeds/test/type/raw/channel/stable/architecture/aarch64/whatever-not-inspected",

			wantStatus:        http.StatusOK,
			wantServiceCalled: true,
			wantImageType:     api.ImageTypeRaw,
			wantArchitecture:  images.UpdateFileArchitecture64BitARM,
			wantChannel:       "stable",
		},
		{
			name: "success - trailing filename with no extension is accepted and ignored",
			path: "/" + tokenUUID + "/seeds/test/architecture/x86_64/type/iso/noextension",

			wantStatus:        http.StatusOK,
			wantServiceCalled: true,
			wantImageType:     api.ImageTypeISO,
			wantArchitecture:  images.UpdateFileArchitecture64BitX86,
		},
		{
			name: "error - missing type",
			path: "/" + tokenUUID + "/seeds/test/architecture/x86_64/file.iso",

			wantStatus: http.StatusBadRequest,
		},
		{
			name: "error - missing architecture",
			path: "/" + tokenUUID + "/seeds/test/type/iso/file.iso",

			wantStatus: http.StatusBadRequest,
		},
		{
			name: "error - invalid type",
			path: "/" + tokenUUID + "/seeds/test/architecture/x86_64/type/exe/file.exe",

			wantStatus: http.StatusBadRequest,
		},
		{
			name: "error - invalid architecture",
			path: "/" + tokenUUID + "/seeds/test/architecture/mips/type/iso/file.iso",

			wantStatus: http.StatusBadRequest,
		},
		{
			name: "error - unknown key",
			path: "/" + tokenUUID + "/seeds/test/architecture/x86_64/type/iso/bogus/value/file.iso",

			wantStatus: http.StatusBadRequest,
		},
		{
			name: "error - odd number of key/value segments",
			path: "/" + tokenUUID + "/seeds/test/architecture/x86_64/type/file.iso",

			wantStatus: http.StatusBadRequest,
		},
		{
			name: "error - missing filename segment entirely",
			path: "/" + tokenUUID + "/seeds/test/",

			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var serviceCalled bool

			var gotImageType api.ImageType

			var gotArchitecture images.UpdateFileArchitecture

			var gotChannel string

			tokenService := &provisioningMock.TokenServiceMock{
				GetTokenSeedByNameFunc: func(ctx context.Context, id uuid.UUID, name string) (*provisioning.TokenSeed, error) {
					return &provisioning.TokenSeed{Public: true}, nil
				},
				GetTokenImageFromTokenSeedFunc: func(ctx context.Context, id uuid.UUID, name string, imageType api.ImageType, architecture images.UpdateFileArchitecture, channel string) (io.ReadCloser, error) {
					serviceCalled = true
					gotImageType = imageType
					gotArchitecture = architecture
					gotChannel = channel

					return io.NopCloser(bytes.NewBufferString("image-data")), nil
				},
			}

			body, status := doTokenRequest(t, tokenService, http.MethodGet, tc.path, "")

			require.Equal(t, tc.wantStatus, status)
			require.Equal(t, tc.wantServiceCalled, serviceCalled)

			if tc.wantServiceCalled {
				require.Equal(t, tc.wantImageType, gotImageType)
				require.Equal(t, tc.wantArchitecture, gotArchitecture)
				require.Equal(t, tc.wantChannel, gotChannel)
				require.Equal(t, "image-data", body)
			}
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
