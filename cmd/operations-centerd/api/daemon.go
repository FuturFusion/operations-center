package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/FuturFusion/operations-center/cmd/operations-centerd/config"
	"github.com/FuturFusion/operations-center/internal/response"
	"github.com/FuturFusion/operations-center/internal/version"
)

type environment interface {
	GetUnixSocket() string
}

type Daemon struct {
	env environment

	config *config.Config

	server   *http.Server
	errgroup *errgroup.Group
}

func NewDaemon(env environment, cfg *config.Config) *Daemon {
	d := &Daemon{
		env:    env,
		config: cfg,
	}

	return d
}

func (d *Daemon) Start() error {
	slog.Info("Starting up", slog.String("version", version.Version))

	// TODO: setup open sqlite DB

	// TODO: setup certificates

	// TODO: setup authorizer

	// TODO: setup OIDC

	// Setup Services

	// Setup Routes
	router := http.NewServeMux()
	router.HandleFunc("GET /",
		response.With(
			rootHandler,
		),
	)

	api10router := http.NewServeMux()
	router.Handle("GET /1.0", api10router)

	api10router.HandleFunc("GET /",
		response.With(
			api10Get,
		),
	)

	// Setup web server
	d.server = &http.Server{
		Handler:     router,
		IdleTimeout: 30 * time.Second,
		Addr:        fmt.Sprintf("%s:%d", d.config.RestServerAddr, d.config.RestServerPort),
	}

	group, errgroupCtx := errgroup.WithContext(context.Background())
	d.errgroup = group

	group.Go(func() error {
		// TODO: Check if the socket file already exists. If it does, return an error,
		// because this indicates, that an other instance of the operations-center
		// is already running.
		unixListener, err := net.Listen("unix", d.env.GetUnixSocket())
		if err != nil {
			return err
		}

		slog.Info("Start unix socket listener", slog.Any("addr", unixListener.Addr()))

		err = d.server.Serve(unixListener)
		if errors.Is(err, http.ErrServerClosed) {
			// Ignore error from graceful shutdown.
			return nil
		}

		return err
	})

	group.Go(func() error {
		slog.Info("Start http listener", slog.Any("addr", d.server.Addr))

		err := d.server.ListenAndServe()
		if errors.Is(err, http.ErrServerClosed) {
			// Ignore error from graceful shutdown.
			return nil
		}

		return err
	})

	select {
	case <-errgroupCtx.Done():
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer shutdownCancel()
		return d.Stop(shutdownCtx)
	case <-time.After(500 * time.Millisecond):
		// Grace period we wait for potential immediate errors from serving the http server.
		// TODO: More clean way would be to check if the listeners are reachable (http, unix socket).
	}

	return nil
}

func (d *Daemon) Stop(ctx context.Context) error {
	shutdownErr := d.server.Shutdown(ctx)

	errgroupWaitErr := d.errgroup.Wait()

	return errors.Join(shutdownErr, errgroupWaitErr)
}
