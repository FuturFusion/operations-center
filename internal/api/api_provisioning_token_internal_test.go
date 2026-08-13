package api

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
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
				GetTokenImageFromTokenSeedFunc: func(ctx context.Context, id uuid.UUID, name string, imageType api.ImageType, architecture images.UpdateFileArchitecture, channel string) (io.ReadCloser, int, error) {
					serviceCalled = true
					gotImageType = imageType
					gotArchitecture = architecture
					gotChannel = channel

					return io.NopCloser(bytes.NewBufferString("image-data")), len("image-data"), nil
				},
			}

			body, resp := doTokenRequestFull(t, tokenService, http.MethodGet, tc.path, "", nil)

			require.Equal(t, tc.wantStatus, resp.statusCode)
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

func Test_tokenHandler_tokenSeedImageGet_wireFormat(t *testing.T) {
	const (
		path      = "/" + tokenUUID + "/seeds/test/architecture/x86_64/type/iso/file.iso"
		imageData = "image-data"
	)

	newTokenService := func(size int) *provisioningMock.TokenServiceMock {
		return &provisioningMock.TokenServiceMock{
			GetTokenSeedByNameFunc: func(ctx context.Context, id uuid.UUID, name string) (*provisioning.TokenSeed, error) {
				return &provisioning.TokenSeed{Public: true}, nil
			},
			GetTokenImageFromTokenSeedFunc: func(ctx context.Context, id uuid.UUID, name string, imageType api.ImageType, architecture images.UpdateFileArchitecture, channel string) (io.ReadCloser, int, error) {
				return io.NopCloser(bytes.NewBufferString(imageData)), size, nil
			},
		}
	}

	t.Run("client without accept-encoding gets the image uncompressed", func(t *testing.T) {
		body, resp := doTokenRequestFull(t, newTokenService(len(imageData)), http.MethodGet, path, "", nil)

		require.Equal(t, http.StatusOK, resp.statusCode)
		require.Equal(t, imageData, body)
		require.Equal(t, "application/octet-stream", resp.header.Get("Content-Type"))
		require.Empty(t, resp.header.Get("Content-Encoding"))
		require.Equal(t, `attachment; filename="pre-seed-test.iso"`, resp.header.Get("Content-Disposition"))
		require.Equal(t, int64(len(imageData)), resp.contentLength)
		require.Equal(t, "none", resp.header.Get("Accept-Ranges"))
	})

	t.Run("client accepting gzip gets the image compressed in transit", func(t *testing.T) {
		body, resp := doTokenRequestFull(t, newTokenService(len(imageData)), http.MethodGet, path, "", http.Header{"Accept-Encoding": []string{"gzip"}})

		require.Equal(t, http.StatusOK, resp.statusCode)
		require.Equal(t, "gzip", resp.header.Get("Content-Encoding"))

		require.Equal(t, "application/octet-stream", resp.header.Get("Content-Type"))
		require.Equal(t, `attachment; filename="pre-seed-test.iso"`, resp.header.Get("Content-Disposition"))
		require.Equal(t, "none", resp.header.Get("Accept-Ranges"))

		require.Equal(t, int64(len(body)), resp.contentLength)

		gzipReader, err := gzip.NewReader(strings.NewReader(body))
		require.NoError(t, err)

		decompressed, err := io.ReadAll(gzipReader)
		require.NoError(t, err)
		require.Equal(t, imageData, string(decompressed))
	})

	t.Run("head reports the metadata without a body", func(t *testing.T) {
		body, resp := doTokenRequestFull(t, newTokenService(len(imageData)), http.MethodHead, path, "", nil)

		require.Equal(t, http.StatusOK, resp.statusCode)
		require.Empty(t, body)
		require.Equal(t, "application/octet-stream", resp.header.Get("Content-Type"))
		require.Equal(t, int64(len(imageData)), resp.contentLength)
	})
}

func doTokenRequest(t *testing.T, tokenService provisioning.TokenService, method string, target string, requestBody string) (string, int) {
	t.Helper()

	body, resp := doTokenRequestFull(t, tokenService, method, target, requestBody, nil)

	return body, resp.statusCode
}

type tokenResponse struct {
	statusCode    int
	header        http.Header
	contentLength int64
}

func doTokenRequestFull(t *testing.T, tokenService provisioning.TokenService, method string, target string, requestBody string, requestHeader http.Header) (string, tokenResponse) {
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

	maps.Copy(req.Header, requestHeader)

	client := &http.Client{Transport: &http.Transport{DisableCompression: true}}
	t.Cleanup(client.CloseIdleConnections)

	resp, err := client.Do(req)
	require.NoError(t, err)

	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	return string(body), tokenResponse{
		statusCode:    resp.StatusCode,
		header:        resp.Header,
		contentLength: resp.ContentLength,
	}
}
