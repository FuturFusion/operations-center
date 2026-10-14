package net_test

import (
	"net"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	testingnet "github.com/FuturFusion/operations-center/internal/util/testing/net"
)

func TestFreePort(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		host string
	}{
		{
			name: "all interfaces",
			host: "",
		},
		{
			name: "IPv4 loopback",
			host: "127.0.0.1",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Run test
			port := testingnet.FreePort(t, tc.host)

			// Assert
			portNumber, err := strconv.Atoi(port)
			require.NoError(t, err, "expect a port number")
			require.GreaterOrEqual(t, portNumber, 20000, "expect a port below the ephemeral port range")
			require.LessOrEqual(t, portNumber, 32767, "expect a port below the ephemeral port range")

			listener, err := net.Listen("tcp", net.JoinHostPort(tc.host, port))
			require.NoError(t, err, "expect the port to be free")
			require.NoError(t, listener.Close(), "expect the listener to close")
		})
	}
}

func TestFreePort_distinct(t *testing.T) {
	t.Parallel()

	// Setup
	const calls = 200

	ports := make(map[string]struct{}, calls)

	// Run test
	for range calls {
		ports[testingnet.FreePort(t, "")] = struct{}{}
	}

	// Assert
	require.Len(t, ports, calls, "expect FreePort to return each port at most once")
}
