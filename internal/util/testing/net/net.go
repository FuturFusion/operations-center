package net

import (
	"math/rand/v2"
	"net"
	"strconv"
	"sync"
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
// The port is not reserved, so another process can take it before the caller
// uses it. FreePort makes this unlikely: it picks the port outside of the
// range for ephemeral ports. The kernel never assigns these ports to a
// listener on port 0 or to an outgoing connection. FreePort returns each port
// at most once per process. Only a caller in another process can pick the
// same port.
func FreePort(t *testing.T, host string) string {
	t.Helper()

	freePortsMu.Lock()
	defer freePortsMu.Unlock()

	for range freePortAttempts {
		port := freePortMin + rand.IntN(freePortMax-freePortMin+1)

		_, ok := freePorts[port]
		if ok {
			continue
		}

		listener, err := net.Listen("tcp", net.JoinHostPort(host, strconv.Itoa(port)))
		if err != nil {
			continue
		}

		err = listener.Close()
		require.NoError(t, err, "Failed to close the listener")

		freePorts[port] = struct{}{}

		return strconv.Itoa(port)
	}

	require.FailNow(t, "Failed to find a free port", "No free port on host %q after %d attempts", host, freePortAttempts)

	return ""
}

// The default range for ephemeral ports starts at 32768 on Linux and at 49152
// on other systems.
const (
	freePortMin      = 20000
	freePortMax      = 32767
	freePortAttempts = 100
)

var (
	freePortsMu sync.Mutex
	freePorts   = map[int]struct{}{}
)
