package updateserver_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lxc/incus-os/incus-osd/api/images"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/environment/mock"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/provisioning/adapter/updateserver"
	"github.com/FuturFusion/operations-center/internal/security/signature/signaturetest"
	"github.com/FuturFusion/operations-center/shared/api"
)

func TestUpdateServer_GetLatest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                         string
		queryParameterAuthentication bool
		statusCode                   int
		updates                      updateserver.UpdatesIndex

		assertErr   require.ErrorAssertionFunc
		wantUpdates provisioning.Updates
	}{
		{
			name:       "success - one update",
			statusCode: http.StatusOK,
			updates: updateserver.UpdatesIndex{
				Format: "1.0",
				Updates: []updateserver.Update{
					{
						Version:     "1",
						Channels:    []string{"stable", "daily"},
						Severity:    images.UpdateSeverityNone,
						PublishedAt: time.Date(2025, 5, 22, 15, 21, 0, 0, time.UTC),
					},
				},
			},

			assertErr: require.NoError,
			wantUpdates: provisioning.Updates{
				{
					UUID:             uuid.MustParse(`84f9f9b4-8bff-50ca-9c9f-020bb266e5cf`),
					Version:          "1",
					UpstreamChannels: provisioning.UpdateUpstreamChannels{"daily", "stable"},
					Severity:         images.UpdateSeverityNone,
					PublishedAt:      time.Date(2025, 5, 22, 15, 21, 0, 0, time.UTC),
					Status:           api.UpdateStatusUnknown,
					Files:            provisioning.UpdateFiles{},
				},
			},
		},
		{
			name:                         "success - one update - with query parameter authentication",
			queryParameterAuthentication: true,
			statusCode:                   http.StatusOK,
			updates: updateserver.UpdatesIndex{
				Format: "1.0",
				Updates: []updateserver.Update{
					{
						Version:     "1",
						Channels:    []string{"stable", "daily"},
						Severity:    images.UpdateSeverityNone,
						PublishedAt: time.Date(2025, 5, 22, 15, 21, 0, 0, time.UTC),
					},
				},
			},

			assertErr: require.NoError,
			wantUpdates: provisioning.Updates{
				{
					UUID:             uuid.MustParse(`84f9f9b4-8bff-50ca-9c9f-020bb266e5cf`),
					Version:          "1",
					UpstreamChannels: provisioning.UpdateUpstreamChannels{"daily", "stable"},
					Severity:         images.UpdateSeverityNone,
					PublishedAt:      time.Date(2025, 5, 22, 15, 21, 0, 0, time.UTC),
					Status:           api.UpdateStatusUnknown,
					Files:            provisioning.UpdateFiles{},
				},
			},
		},
		{
			name:       "success - two updates",
			statusCode: http.StatusOK,
			updates: updateserver.UpdatesIndex{
				Format: "1.0",
				Updates: []updateserver.Update{
					{
						Version:     "1",
						Severity:    images.UpdateSeverityNone,
						PublishedAt: time.Date(2024, 5, 22, 15, 21, 0, 0, time.UTC), // older update, will be filtered out
					},
					{
						Version:     "2",
						Severity:    images.UpdateSeverityNone,
						PublishedAt: time.Date(2025, 5, 22, 15, 21, 0, 0, time.UTC),
						Files: provisioning.UpdateFiles{
							provisioning.UpdateFile{
								Filename:  "undefined_architecture.iso",
								Component: images.UpdateFileComponentIncus,
							},
							provisioning.UpdateFile{
								Filename:     "undefined_file_component.iso", // filtered out, since the file component is unknown.
								Architecture: images.UpdateFileArchitecture64BitX86,
							},
						},
					},
				},
			},

			assertErr: require.NoError,
			wantUpdates: provisioning.Updates{
				{
					UUID:        uuid.MustParse(`69869f43-e46c-5094-8e04-c94c88b65d0d`),
					Version:     "2",
					Severity:    images.UpdateSeverityNone,
					PublishedAt: time.Date(2025, 5, 22, 15, 21, 0, 0, time.UTC),
					Status:      api.UpdateStatusUnknown,
					Files: provisioning.UpdateFiles{
						provisioning.UpdateFile{
							Filename:     "undefined_architecture.iso",
							Architecture: images.UpdateFileArchitectureUndefined,
							Component:    images.UpdateFileComponentIncus,
						},
					},
				},
			},
		},
		{
			name:       "error - wrong status code",
			statusCode: http.StatusInternalServerError,
			updates:    updateserver.UpdatesIndex{},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, `Failed to query "/index.sjson": unexpected status code 500`, a...)
			},
		},
		{
			name:       "error - invalid format",
			statusCode: http.StatusOK,
			updates: updateserver.UpdatesIndex{
				Format: "invalid", // invalid format
			},

			assertErr: require.Error,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			envMock := &mock.EnvironmentMock{
				GetTokenFunc: func(ctx context.Context) (string, error) {
					return "token", nil
				},
				IsIncusOSFunc: func() bool { return true },
			}

			caCert, cert, key := signaturetest.GenerateCertChain(t)

			body, err := json.Marshal(tc.updates)
			require.NoError(t, err)

			signedBody := signaturetest.SignContent(t, cert, key, body)

			var gotToken string
			svr := httptest.NewServer(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if !strings.HasSuffix(r.URL.Path, "/index.sjson") {
						http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
						return
					}

					if tc.queryParameterAuthentication {
						gotToken = r.URL.Query().Get("token")
					} else {
						gotToken = r.Header.Get("X-IncusOS-Authentication")
					}

					w.WriteHeader(tc.statusCode)
					_, _ = w.Write(signedBody)
				}),
			)
			defer svr.Close()

			s := updateserver.New(svr.URL, string(caCert), tc.queryParameterAuthentication, envMock)
			updates, err := s.GetLatest(context.Background(), 1)
			tc.assertErr(t, err)

			require.Len(t, updates, len(tc.wantUpdates))
			require.Equal(t, tc.wantUpdates, updates)
			require.Equal(t, "token", gotToken)
		})
	}
}

func TestUpdateServer_GetUpdateFileByFilename(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		statusCode   int
		responseBody []byte

		assertErr          require.ErrorAssertionFunc
		wantResponseLength int
		wantResponseBody   []byte
	}{
		{
			name:         "success - no files",
			statusCode:   http.StatusOK,
			responseBody: []byte(`some text`),

			assertErr:          require.NoError,
			wantResponseLength: 9,
			wantResponseBody:   []byte(`some text`),
		},
		{
			name:       "error - wrong status code",
			statusCode: http.StatusInternalServerError,

			assertErr: require.Error,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			errChan := make(chan error, 1)

			svr := httptest.NewServer(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if !strings.HasSuffix(r.URL.Path, "/1/one.txt") {
						http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
						return
					}

					w.WriteHeader(tc.statusCode)
					_, _ = w.Write(tc.responseBody)
				}),
			)
			defer svr.Close()

			envMock := &mock.EnvironmentMock{
				GetTokenFunc: func(ctx context.Context) (string, error) {
					return "", nil
				},
			}

			s := updateserver.New(svr.URL, "", false, envMock)
			stream, n, err := s.GetUpdateFileByFilenameUnverified(context.Background(), provisioning.Update{
				URL: "/1",
			}, "one.txt")
			tc.assertErr(t, err)

			var serverErr error
			select {
			case serverErr = <-errChan:
			default:
			}

			require.NoError(t, serverErr)

			responseBody := readAll(t, stream)

			require.Equal(t, tc.wantResponseLength, n)
			require.Equal(t, tc.wantResponseBody, responseBody)
		})
	}
}

func TestUpdateServer_GetUpdateManifest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		statusCode     int
		manifestBody   []byte
		verificationCA func(caCert []byte, otherCACert []byte) []byte

		assertErr    require.ErrorAssertionFunc
		wantManifest func(signedBody []byte) []byte
	}{
		{
			name:           "success",
			statusCode:     http.StatusOK,
			manifestBody:   []byte(`{"version":"1"}`),
			verificationCA: signedByCA,

			assertErr: require.NoError,
			wantManifest: func(signedBody []byte) []byte {
				return signedBody
			},
		},
		{
			name:           "error - wrong status code",
			statusCode:     http.StatusNotFound,
			manifestBody:   []byte(`{"version":"1"}`),
			verificationCA: signedByCA,

			assertErr: require.Error,
			wantManifest: func(signedBody []byte) []byte {
				return nil
			},
		},
		{
			name:           "error - invalid signature",
			statusCode:     http.StatusOK,
			manifestBody:   []byte(`{"version":"1"}`),
			verificationCA: signedByOtherCA,

			assertErr: require.Error,
			wantManifest: func(signedBody []byte) []byte {
				return nil
			},
		},
		{
			name:           "error - invalid manifest",
			statusCode:     http.StatusOK,
			manifestBody:   []byte(`{`),
			verificationCA: signedByCA,

			assertErr: require.Error,
			wantManifest: func(signedBody []byte) []byte {
				return nil
			},
		},
		{
			name:           "error - version mismatch",
			statusCode:     http.StatusOK,
			manifestBody:   []byte(`{"version":"2"}`),
			verificationCA: signedByCA,

			assertErr: require.Error,
			wantManifest: func(signedBody []byte) []byte {
				return nil
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			envMock := &mock.EnvironmentMock{
				GetTokenFunc: func(ctx context.Context) (string, error) {
					return "token", nil
				},
			}

			caCert, cert, key := signaturetest.GenerateCertChain(t)
			otherCACert, _, _ := signaturetest.GenerateCertChain(t)

			signedBody := signaturetest.SignContent(t, cert, key, tc.manifestBody)

			svr := httptest.NewServer(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path != "/1/update.sjson" {
						http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
						return
					}

					w.WriteHeader(tc.statusCode)
					_, _ = w.Write(signedBody)
				}),
			)
			defer svr.Close()

			s := updateserver.New(svr.URL, string(tc.verificationCA(caCert, otherCACert)), false, envMock)

			// Run test
			manifest, err := s.GetUpdateManifest(t.Context(), provisioning.Update{
				URL:     "/1",
				Version: "1",
			})

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.wantManifest(signedBody), manifest, "unexpected manifest")
		})
	}
}

func signedByCA(caCert []byte, _ []byte) []byte {
	return caCert
}

func signedByOtherCA(_ []byte, otherCACert []byte) []byte {
	return otherCACert
}

func TestUpdateServer_SourceConnectionTest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		newSource  func(svrURL string) string
		statusCode int

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "success - empty source",
			newSource: func(svrURL string) string {
				return ""
			},
			statusCode: http.StatusInternalServerError,

			assertErr: require.NoError,
		},
		{
			name: "success - unchanged source",
			newSource: func(svrURL string) string {
				return svrURL + "/"
			},
			statusCode: http.StatusInternalServerError,

			assertErr: require.NoError,
		},
		{
			name: "success - new source reachable",
			newSource: func(svrURL string) string {
				return svrURL + "/new"
			},
			statusCode: http.StatusOK,

			assertErr: require.NoError,
		},
		{
			name: "error - new source returns wrong status code",
			newSource: func(svrURL string) string {
				return svrURL + "/new"
			},
			statusCode: http.StatusInternalServerError,

			assertErr: require.Error,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			envMock := &mock.EnvironmentMock{
				GetTokenFunc: func(ctx context.Context) (string, error) {
					return "token", nil
				},
			}

			caCert, cert, key := signaturetest.GenerateCertChain(t)

			body, err := json.Marshal(updateserver.UpdatesIndex{Format: "1.0"})
			require.NoError(t, err)

			signedBody := signaturetest.SignContent(t, cert, key, body)

			svr := httptest.NewServer(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if !strings.HasSuffix(r.URL.Path, "/index.sjson") {
						http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
						return
					}

					w.WriteHeader(tc.statusCode)
					_, _ = w.Write(signedBody)
				}),
			)
			defer svr.Close()

			s := updateserver.New(svr.URL, string(caCert), false, envMock)

			// Run test
			err = s.SourceConnectionTest(context.Background(), tc.newSource(svr.URL), string(caCert), false)

			// Assert
			tc.assertErr(t, err, "unexpected result of the source connection test")
		})
	}
}

func readAll(t *testing.T, r io.ReadCloser) []byte {
	t.Helper()

	if r == nil {
		return nil
	}

	body, err := io.ReadAll(r)
	require.NoError(t, err)

	return body
}
