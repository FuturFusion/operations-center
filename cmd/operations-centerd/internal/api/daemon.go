package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/FuturFusion/operations-center/cmd/operations-centerd/internal/config"
	"github.com/FuturFusion/operations-center/internal/dbschema"
	"github.com/FuturFusion/operations-center/internal/operations"
	"github.com/FuturFusion/operations-center/internal/operations/repo/sqlite"
	"github.com/FuturFusion/operations-center/internal/response"
	dbdriver "github.com/FuturFusion/operations-center/internal/sqlite"
	"github.com/FuturFusion/operations-center/internal/transaction"
	"github.com/FuturFusion/operations-center/internal/version"
)

type environment interface {
	GetUnixSocket() string
	VarDir() string
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

	db, err := dbdriver.Open(d.env.VarDir())
	if err != nil {
		return fmt.Errorf("Failed to open sqlite database: %w", err)
	}

	_, err = dbschema.Ensure(context.TODO(), db, d.env.VarDir())
	if err != nil {
		return err
	}

	dbWithTransaction := transaction.Enable(db)

	// TODO: setup certificates

	// TODO: setup authorizer

	// TODO: setup OIDC

	// Setup Services
	tokenSvc := operations.NewTokenService(sqlite.NewToken(dbWithTransaction))

	// Setup Routes
	router := http.NewServeMux()
	router.HandleFunc("GET /{$}",
		response.With(
			rootHandler,
		),
	)

	api10router := newSubRouter(router, "/1.0")
	registerAPI10Handler(api10router)

	provisioningRouter := newSubRouter(api10router, "/provisioning")

	tokenRouter := newSubRouter(provisioningRouter, "/tokens")
	registerTokenHandler(tokenRouter, tokenSvc)

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

// newSubRouter returns a derived http.ServeMux for the given prefix.
// The derived router is configured such that handlers can be defined as they
// would on a regular root ServeMux.
func newSubRouter(router *http.ServeMux, prefix string) *http.ServeMux {
	if !strings.HasPrefix(prefix, "/") {
		prefix = "/" + prefix
	}

	subrouter := http.NewServeMux()
	router.HandleFunc(prefix, func(w http.ResponseWriter, r *http.Request) {
		http.StripPrefix(prefix, handleRootPath(subrouter, false)).ServeHTTP(w, r)
	})
	router.HandleFunc(prefix+"/", func(w http.ResponseWriter, r *http.Request) {
		http.StripPrefix(prefix, handleRootPath(subrouter, false)).ServeHTTP(w, r)
	})

	return subrouter
}

// handleRootPath compensates for the handling of the root resource for a derived
// sub router.
// This allows to define routes on the sub router without knowledge of the prefix
// from where these routes will be served.
// If ignoreTrailingSlash is true, the root resource will be served for both,
// "/prefix" and "/prefix/". If ignoreTrailingSlash is false, the root resource
// is only served if the resource is requested without trailing slash ()"/prefix").
func handleRootPath(h http.Handler, ignoreTrailingSlash bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "":
			r2 := new(http.Request)
			*r2 = *r
			r2.URL = new(url.URL)
			*r2.URL = *r.URL
			r2.URL.Path = "/"
			h.ServeHTTP(w, r2)
			return

		case "/":
			if ignoreTrailingSlash {
				h.ServeHTTP(w, r)
				return
			}

			http.NotFound(w, r)
			return

		default:
			h.ServeHTTP(w, r)
			return
		}
	})
}
