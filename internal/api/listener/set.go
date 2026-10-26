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

type ListenerSet struct {
	sync.Mutex
	ServerListener  *FancyTLSListener
	ClusterListener *FancyTLSListener
}

// Active reports whether any network listener is set up.
func (l *ListenerSet) Active() bool {
	l.Lock()
	defer l.Unlock()

	return l.ServerListener != nil || l.ClusterListener != nil
}

func (l *ListenerSet) UpdateCertificate(cert tls.Certificate) {
	l.Lock()
	defer l.Unlock()

	if l.ServerListener != nil {
		l.ServerListener.Config(cert)
	}

	if l.ClusterListener != nil {
		l.ClusterListener.Config(cert)
	}
}

func (l *ListenerSet) UpdateTrustedProxies(ctx context.Context, trustedHTTPSProxies []string) {
	l.Lock()
	defer l.Unlock()

	if l.ServerListener != nil {
		err := l.ServerListener.TrustedProxy(trustedHTTPSProxies)
		if err != nil {
			slog.WarnContext(ctx, "Failed to set trusted HTTPS proxies", logger.Err(err))
		}
	}

	if l.ClusterListener != nil {
		err := l.ClusterListener.TrustedProxy(trustedHTTPSProxies)
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

	grp := errgroup.Group{}
	grp.Go(serve(l.ServerListener))
	grp.Go(serve(l.ClusterListener))

	return grp.Wait()
}
