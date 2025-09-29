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

	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/provisioning/adapter/updateserver"
	"github.com/FuturFusion/operations-center/internal/signature/signaturetest"
	"github.com/FuturFusion/operations-center/shared/api"
)

func TestUpdateServer_GetLatest(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		updates    updateserver.UpdatesIndex

		assertErr   require.ErrorAssertionFunc
		wantUpdates provisioning.Updates
	}{
		{
			name:       "success - one update",
			statusCode: http.StatusOK,
			updates: updateserver.UpdatesIndex{
				Format: "1.0",
				Updates: []provisioning.Update{
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
					UUID:        uuid.MustParse(`1f82dc0a-0487-5532-a76a-74ed263b2280`),
					Version:     "1",
					Channels:    provisioning.UpdateChannels{"daily", "stable"},
					Severity:    images.UpdateSeverityNone,
					PublishedAt: time.Date(2025, 5, 22, 15, 21, 0, 0, time.UTC),
					Status:      api.UpdateStatusUnknown,
					Files:       provisioning.UpdateFiles{},
				},
			},
		},
		{
			name:       "success - two updates",
			statusCode: http.StatusOK,
			updates: updateserver.UpdatesIndex{
				Format: "1.0",
				Updates: []provisioning.Update{
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
					UUID:        uuid.MustParse(`25eacea3-d627-5c40-bfe5-52a9ea85e0ea`),
					Version:     "2",
					Severity:    images.UpdateSeverityNone,
					PublishedAt: time.Date(2025, 5, 22, 15, 21, 0, 0, time.UTC),
					Status:      api.UpdateStatusUnknown,
					Files: provisioning.UpdateFiles{
						provisioning.UpdateFile{
							Filename:     "undefined_architecture.iso",
							Architecture: images.UpdateFileArchitecture64BitX86,
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

			assertErr: require.Error,
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
			caCert, cert, key := signaturetest.GenerateCertChain(t)

			body, err := json.Marshal(tc.updates)
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

			s := updateserver.New(svr.URL, string(caCert))
			updates, err := s.GetLatest(context.Background(), 1)
			tc.assertErr(t, err)

			require.Len(t, updates, len(tc.wantUpdates))
			require.Equal(t, tc.wantUpdates, updates)
		})
	}
}

func TestUpdateServer_GetUpdateFileByFilename(t *testing.T) {
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

			s := updateserver.New(svr.URL, "")
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

func readAll(t *testing.T, r io.ReadCloser) []byte {
	t.Helper()

	if r == nil {
		return nil
	}

	body, err := io.ReadAll(r)
	require.NoError(t, err)

	return body
}
