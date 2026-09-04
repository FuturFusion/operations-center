package redfish

import (
	"bytes"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/util/testing/testcert"
	"github.com/FuturFusion/operations-center/shared/api"
)

func Test_newBMCHTTPClient_boundsEveryRequest(t *testing.T) {
	tests := []struct {
		name        string
		certificate string
	}{
		{
			name: "without a pinned certificate",
		},
		{
			name:        "with a pinned certificate",
			certificate: testcert.ClientCertificate,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := provisioning.Server{
				BMCConfig: api.BMCConfig{
					Endpoint:    "https://bmc.local:8443",
					Certificate: tc.certificate,
				},
			}

			httpClient, err := newBMCHTTPClient(server)
			require.NoError(t, err)

			transport, ok := httpClient.Transport.(*http.Transport)
			require.True(t, ok, "the client carries a transport of ours instead of falling back to the default one")

			require.NotNil(t, transport.DialContext)
			require.Positive(t, transport.ResponseHeaderTimeout)
			require.Positive(t, transport.TLSHandshakeTimeout)
			require.Equal(t, tc.certificate != "", transport.TLSClientConfig != nil, "the TLS configuration is only pinned down for a known certificate")
		})
	}
}

func Test_parseTimestamp(t *testing.T) {
	tests := []struct {
		name       string
		timestamps []string

		want        time.Time
		wantWarning bool
	}{
		{
			name:       "RFC3339 timestamp",
			timestamps: []string{"2026-08-26T09:00:00Z"},

			want: time.Date(2026, 8, 26, 9, 0, 0, 0, time.UTC),
		},
		{
			name:       "RFC3339Nano timestamp",
			timestamps: []string{"2026-08-26T09:00:00.123456789Z"},

			want: time.Date(2026, 8, 26, 9, 0, 0, 123456789, time.UTC),
		},
		{
			name:       "property not reported at all",
			timestamps: []string{""},

			want: time.Time{},
		},
		{
			name:       "all zero timestamp with offset",
			timestamps: []string{"0000-00-00T00:00:00+00:00"},

			want: time.Time{},
		},
		{
			name:       "all zero timestamp in UTC",
			timestamps: []string{"0000-00-00T00:00:00Z"},

			want: time.Time{},
		},
		{
			name:       "all zero timestamp without offset",
			timestamps: []string{"0000-00-00T00:00:00"},

			want: time.Time{},
		},
		{
			name:       "all zero date only",
			timestamps: []string{"0000-00-00"},

			want: time.Time{},
		},
		{
			name:       "malformed timestamp",
			timestamps: []string{"26.08.2026 09:05:30"},

			want:        time.Time{},
			wantWarning: true,
		},
		{
			name:       "all zero timestamp followed by a valid one",
			timestamps: []string{"0000-00-00T00:00:00+00:00", "2026-08-26T09:00:00Z"},

			want: time.Date(2026, 8, 26, 9, 0, 0, 0, time.UTC),
		},
		{
			name:       "all zero timestamp followed by a malformed one",
			timestamps: []string{"0000-00-00T00:00:00+00:00", "not a timestamp"},

			want:        time.Time{},
			wantWarning: true,
		},
		{
			name:       "malformed timestamp followed by a valid one",
			timestamps: []string{"not a timestamp", "2026-08-26T09:00:00Z"},

			want: time.Date(2026, 8, 26, 9, 0, 0, 0, time.UTC),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			log := slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelWarn}))

			got := parseTimestamp(t.Context(), log, "Property", tc.timestamps...)

			require.Equal(t, tc.want, got)

			if tc.wantWarning {
				require.Contains(t, buf.String(), "Failed to parse timestamp reported by BMC")
			} else {
				require.Empty(t, buf.String())
			}
		})
	}
}
