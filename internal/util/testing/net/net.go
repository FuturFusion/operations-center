package net

import (
	"net"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func LocalhostIP(t *testing.T) string {
	t.Helper()

	ln, err := net.Listen("tcp", "[::1]:0")
	if err != nil {
		// IPv6 unavailable
		return "127.0.0.1"
	}

	err = ln.Close()
	require.NoError(t, err)

	return "[::1]"
}

// FreePort returns the number of a TCP port without a listener on host.
// The port is not reserved. Another process can take it before the caller
// uses it.
func FreePort(t *testing.T, host string) string {
	t.Helper()

	listener, err := net.Listen("tcp", net.JoinHostPort(host, "0"))
	require.NoError(t, err, "Failed to listen on a free port")

	tcpAddr, ok := listener.Addr().(*net.TCPAddr)
	require.True(t, ok, "Failed to get the port of the listener")

	err = listener.Close()
	require.NoError(t, err, "Failed to close the listener")

	return strconv.Itoa(tcpAddr.Port)
}
