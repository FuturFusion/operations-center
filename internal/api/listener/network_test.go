package listener_test

import (
	"net"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/api/listener"
	config "github.com/FuturFusion/operations-center/internal/config/daemon"
)

func TestCanonicalNetworkAddress(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		address string

		want string
	}{
		{
			name:    "IPv4 address without port",
			address: "192.0.2.1",

			want: "192.0.2.1:8443",
		},
		{
			name:    "IPv4 address with port",
			address: "192.0.2.1:9000",

			want: "192.0.2.1:9000",
		},
		{
			name:    "IPv4 address with trailing colon",
			address: "192.0.2.1:",

			want: "192.0.2.1:8443",
		},
		{
			name:    "bare IPv6 address",
			address: "2001:db8::1",

			want: "[2001:db8::1]:8443",
		},
		{
			name:    "IPv6 address with port",
			address: "[2001:db8::1]:9000",

			want: "[2001:db8::1]:9000",
		},
		{
			name:    "bare IPv6 wildcard",
			address: "::",

			want: "[::]:8443",
		},
		{
			name:    "IPv6 wildcard in brackets without port",
			address: "[::]",

			want: "[::]:8443",
		},
		{
			name:    "host name without port",
			address: "example.com",

			want: "example.com:8443",
		},
		{
			name:    "host name with port",
			address: "example.com:9000",

			want: "example.com:9000",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := listener.CanonicalNetworkAddress(tc.address, "8443")

			require.Equal(t, tc.want, got, "the address must be in the canonical form host:port")
		})
	}
}

func TestIsAddressCovered(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		address1 string
		address2 string

		want bool
	}{
		{
			name:     "same address",
			address1: "192.0.2.1:8443",
			address2: "192.0.2.1:8443",

			want: true,
		},
		{
			name:     "same address, one with the default port",
			address1: "192.0.2.1",
			address2: net.JoinHostPort("192.0.2.1", config.DefaultRestServerPort),

			want: true,
		},
		{
			name:     "same address, different port",
			address1: "192.0.2.1:8443",
			address2: "192.0.2.1:9000",

			want: false,
		},
		{
			name:     "different address, same port",
			address1: "192.0.2.1:8443",
			address2: "192.0.2.2:8443",

			want: false,
		},
		{
			name:     "same IPv6 address in another notation",
			address1: "[2001:db8::1]:8443",
			address2: "[2001:db8:0:0::1]:8443",

			want: true,
		},
		{
			name:     "invalid second address",
			address1: "192.0.2.1:8443",
			address2: "[::",

			want: false,
		},
		{
			name:     "IPv4 wildcard covers an IPv4 address",
			address1: "192.0.2.1:8443",
			address2: "0.0.0.0:8443",

			want: true,
		},
		{
			name:     "IPv4 wildcard with another port",
			address1: "192.0.2.1:8443",
			address2: "0.0.0.0:9000",

			want: false,
		},
		{
			name:     "IPv4 wildcard does not cover an IPv6 address",
			address1: "[2001:db8::1]:8443",
			address2: "0.0.0.0:8443",

			want: false,
		},
		{
			name:     "IPv6 wildcard covers an IPv4 address",
			address1: "192.0.2.1:8443",
			address2: "[::]:8443",

			want: true,
		},
		{
			name:     "IPv6 wildcard covers an IPv6 address",
			address1: "[2001:db8::1]:8443",
			address2: "[::]:8443",

			want: true,
		},
		{
			name:     "empty host covers any address",
			address1: "192.0.2.1:8443",
			address2: ":8443",

			want: true,
		},
		{
			name:     "a specific address does not cover the wildcard",
			address1: "[::]:8443",
			address2: "192.0.2.1:8443",

			want: false,
		},
		{
			name:     "invalid address",
			address1: "[2001:db8::1",
			address2: "[::]:8443",

			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := listener.IsAddressCovered(tc.address1, tc.address2)

			require.Equal(t, tc.want, got, "address1 %q covered by address2 %q", tc.address1, tc.address2)
		})
	}
}
