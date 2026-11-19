package listener

import (
	"context"
	"crypto/tls"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"sync"

	"golang.org/x/sync/errgroup"

	"github.com/FuturFusion/operations-center/internal/util/logger"
)

// ListenerSet holds the https listener and the cluster listener. They are replaced on a network change while the previous ones are still served.
type ListenerSet struct {
	sync.Mutex
	serverListener  *FancyTLSListener
	clusterListener *FancyTLSListener
}

// Listeners returns the https listener and the cluster listener, nil if they aren't set up.
func (l *ListenerSet) Listeners() (server *FancyTLSListener, cluster *FancyTLSListener) {
	l.Lock()
	defer l.Unlock()

	return l.serverListener, l.clusterListener
}

// SetServerListener replaces the https listener.
func (l *ListenerSet) SetServerListener(server *FancyTLSListener) {
	l.Lock()
	defer l.Unlock()

	l.serverListener = server
}

// SetClusterListener replaces the cluster listener.
func (l *ListenerSet) SetClusterListener(cluster *FancyTLSListener) {
	l.Lock()
	defer l.Unlock()

	l.clusterListener = cluster
}

// Active reports whether any network listener is set up.
func (l *ListenerSet) Active() bool {
	l.Lock()
	defer l.Unlock()

	return l.serverListener != nil || l.clusterListener != nil
}

func (l *ListenerSet) UpdateCertificate(cert tls.Certificate) {
	l.Lock()
	defer l.Unlock()

	if l.serverListener != nil {
		l.serverListener.Config(cert)
	}

	if l.clusterListener != nil {
		l.clusterListener.Config(cert)
	}
}

func (l *ListenerSet) UpdateTrustedProxies(ctx context.Context, trustedHTTPSProxies []string) {
	l.Lock()
	defer l.Unlock()

	if l.serverListener != nil {
		err := l.serverListener.TrustedProxy(trustedHTTPSProxies)
		if err != nil {
			slog.WarnContext(ctx, "Failed to set trusted HTTPS proxies", logger.Err(err))
		}
	}

	if l.clusterListener != nil {
		err := l.clusterListener.TrustedProxy(trustedHTTPSProxies)
		if err != nil {
			slog.WarnContext(ctx, "Failed to set trusted HTTPS proxies for cluster listener", logger.Err(err))
		}
	}
}

func (l *ListenerSet) Serve(server *http.Server) error {
	serve := func(l *FancyTLSListener) func() error {
		return func() error {
			if l == nil {
				return nil
			}

			err := server.Serve(l)
			if errors.Is(err, http.ErrServerClosed) {
				// Ignore error from graceful shutdown.
				return nil
			}

			if errors.Is(err, net.ErrClosed) {
				// Ignore error of used closed connection, it is likely caused after a

				// change of the network configuration.
				return nil
			}

			return err
		}
	}

	serverListener, clusterListener := l.Listeners()

	grp := errgroup.Group{}
	grp.Go(serve(serverListener))
	grp.Go(serve(clusterListener))

	return grp.Wait()
}
