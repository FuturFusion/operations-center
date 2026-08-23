package server

import (
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_availableVersionGreaterThan(t *testing.T) {
	tests := []struct {
		name             string
		currentVersion   string
		availableVersion string

		want bool
	}{
		{
			name:             "available version greater",
			currentVersion:   "202601172317",
			availableVersion: "202601210123",

			want: true,
		},
		{
			name:             "available version equal",
			currentVersion:   "202601210123",
			availableVersion: "202601210123",

			want: false,
		},
		{
			name:             "available version smaller",
			currentVersion:   "202601210123",
			availableVersion: "202601172317",

			want: false,
		},
		{
			name:             "current invalid",
			currentVersion:   "invalid",
			availableVersion: "202601210123",

			want: true,
		},
		{
			name:             "available invalid",
			currentVersion:   "202601210123",
			availableVersion: "invalid",

			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := availableVersionGreaterThan(tc.currentVersion, tc.availableVersion)

			require.Equal(t, tc.want, got)
		})
	}
}

func Test_mediaURLWarnings(t *testing.T) {
	tests := []struct {
		name     string
		mediaURL string

		want []string
	}{
		{
			name:     "plain host name",
			mediaURL: "https://oc.example.com:8443/1.0/provisioning/tokens/x/seeds/y/architecture/x86_64/type/iso/file.iso",

			want: nil,
		},
		{
			name:     "IPv4 address with a non-default port",
			mediaURL: "https://192.0.2.10:8443/1.0/provisioning/tokens/x/seeds/y/architecture/x86_64/type/iso/file.iso",

			want: nil,
		},
		{
			name:     "IPv6 address on the default port",
			mediaURL: "https://[2001:db8::1]/1.0/provisioning/tokens/x/seeds/y/architecture/x86_64/type/iso/file.iso",

			want: nil,
		},
		{
			name:     "IPv6 address with a non-default port",
			mediaURL: "https://[2001:db8::1]:8443/1.0/provisioning/tokens/x/seeds/y/architecture/x86_64/type/iso/file.iso",

			want: []string{"it combines an IPv6 address with a non-default port, which BMC firmware is known to parse incorrectly"},
		},
		{
			name:     "overly long URL",
			mediaURL: "https://oc.example.com:8443/1.0/provisioning/tokens/00000000-0000-0000-0000-000000000000/seeds/" + strings.Repeat("s", 200) + "/architecture/x86_64/type/iso/file.iso",

			want: []string{"it is longer than the 255 characters some BMCs accept"},
		},
		{
			name:     "both",
			mediaURL: "https://[2001:db8::1]:8443/1.0/provisioning/tokens/00000000-0000-0000-0000-000000000000/seeds/" + strings.Repeat("s", 200) + "/architecture/x86_64/type/iso/file.iso",

			want: []string{
				"it combines an IPv6 address with a non-default port, which BMC firmware is known to parse incorrectly",
				"it is longer than the 255 characters some BMCs accept",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mediaURL, err := url.Parse(tc.mediaURL)
			require.NoError(t, err)

			require.Equal(t, tc.want, mediaURLWarnings(mediaURL))
		})
	}
}
