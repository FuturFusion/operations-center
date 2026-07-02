package net

import (
	"net"
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
