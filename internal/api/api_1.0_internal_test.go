package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/security/authn"
	"github.com/FuturFusion/operations-center/shared/api"
)

func Test_api10Get(t *testing.T) {
	tests := []struct {
		name    string
		request func() *http.Request

		wantAuth string
	}{
		{
			name: "untrusted - no authentication context values",
			request: func() *http.Request {
				return httptest.NewRequest(http.MethodGet, "/1.0", http.NoBody)
			},

			wantAuth: api.AuthenticationUntrusted,
		},
		{
			name: "untrusted - not authenticated with empty protocol",
			request: func() *http.Request {
				return newRequestWithAuthContext(false, "")
			},

			wantAuth: api.AuthenticationUntrusted,
		},
		{
			name: "untrusted - not authenticated with protocol present",
			request: func() *http.Request {
				return newRequestWithAuthContext(false, api.AuthenticationMethodTLS)
			},

			wantAuth: api.AuthenticationUntrusted,
		},
		{
			name: "untrusted - authenticated without protocol",
			request: func() *http.Request {
				return newRequestWithAuthContext(true, "")
			},

			wantAuth: api.AuthenticationUntrusted,
		},
		{
			name: "trusted - tls",
			request: func() *http.Request {
				return newRequestWithAuthContext(true, api.AuthenticationMethodTLS)
			},

			wantAuth: api.AuthenticationMethodTLS,
		},
		{
			name: "trusted - oidc",
			request: func() *http.Request {
				return newRequestWithAuthContext(true, api.AuthenticationMethodOIDC)
			},

			wantAuth: api.AuthenticationMethodOIDC,
		},
		{
			name: "trusted - unix",
			request: func() *http.Request {
				return newRequestWithAuthContext(true, api.AuthenticationMethodUnix)
			},

			wantAuth: api.AuthenticationMethodUnix,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()

			err := api10Get(tc.request()).Render(recorder)
			require.NoError(t, err)

			var apiResponse api.Response
			err = json.Unmarshal(recorder.Body.Bytes(), &apiResponse)
			require.NoError(t, err)

			var serverInfo api.ServerUntrusted
			err = apiResponse.MetadataAsStruct(&serverInfo)
			require.NoError(t, err)

			require.Equal(t, tc.wantAuth, serverInfo.Auth)
		})
	}
}

func newRequestWithAuthContext(authenticated bool, protocol string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/1.0", http.NoBody)

	ctx := context.WithValue(req.Context(), authn.CtxAuthenticated, authenticated)
	ctx = context.WithValue(ctx, authn.CtxProtocol, protocol)

	return req.WithContext(ctx)
}
