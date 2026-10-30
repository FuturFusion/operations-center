package listener_test

import (
	"crypto/tls"
	"net"
	"net/http"
	"testing"
	"time"

	incustls "github.com/lxc/incus/v7/shared/tls"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/api/listener"
)

func newListener(t *testing.T) *listener.FancyTLSListener {
	t.Helper()

	inner, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	certPEM, keyPEM, err := incustls.GenerateMemCert(false, false)
	require.NoError(t, err)

	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	require.NoError(t, err)

	return listener.NewFancyTLSListener(inner, cert)
}

// requireServed fails unless l answers a request.
func requireServed(t *testing.T, l *listener.FancyTLSListener) {
	t.Helper()

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig:   &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
			DisableKeepAlives: true,
		},
		Timeout: 10 * time.Second,
	}

	resp, err := client.Get("https://" + l.Addr().String())
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
}

func TestListenerSet_replaceWhileServing(t *testing.T) {
	t.Parallel()

	first := newListener(t)

	set := &listener.ListenerSet{}
	set.SetServerListener(newListener(t))
	set.SetClusterListener(first)

	served := make(chan error)
	go func() {
		served <- set.Serve(&http.Server{})
	}()

	// Serve takes its listeners before they are replaced.
	requireServed(t, first)

	// A network change replaces the listeners while the previous ones are served.
	replacement := newListener(t)
	set.SetClusterListener(replacement)
	require.True(t, set.Active())

	server, cluster := set.Listeners()
	require.Equal(t, replacement, cluster)

	// Closing a served listener ends its Serve without an error.
	require.NoError(t, server.Close())
	require.NoError(t, replacement.Close())

	set.SetServerListener(nil)
	set.SetClusterListener(nil)
	require.False(t, set.Active())

	// The cluster listener served first is still served, until it is closed as well.
	select {
	case err := <-served:
		t.Fatalf("Serve returned while a listener is still served: %v", err)

	default:
	}

	require.NoError(t, first.Close())
	require.NoError(t, <-served)
}
