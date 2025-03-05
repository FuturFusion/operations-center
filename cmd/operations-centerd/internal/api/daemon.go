package api

import (
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	ghClient "github.com/google/go-github/v69/github"
	incusTLS "github.com/lxc/incus/v6/shared/tls"
	"golang.org/x/sync/errgroup"

	"github.com/FuturFusion/operations-center/cmd/operations-centerd/internal/config"
	"github.com/FuturFusion/operations-center/internal/dbschema"
	incusAdapter "github.com/FuturFusion/operations-center/internal/inventory/server/incus"
	serverMiddleware "github.com/FuturFusion/operations-center/internal/inventory/server/middleware"
	"github.com/FuturFusion/operations-center/internal/logger"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	provisioningServiceMiddleware "github.com/FuturFusion/operations-center/internal/provisioning/middleware"
	"github.com/FuturFusion/operations-center/internal/provisioning/repo/github"
	provisioningRepoMiddleware "github.com/FuturFusion/operations-center/internal/provisioning/repo/middleware"
	provisioningSqlite "github.com/FuturFusion/operations-center/internal/provisioning/repo/sqlite"
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

	config            *config.Config
	clientCertificate string
	clientKey         string

	server   *http.Server
	errgroup *errgroup.Group
}

func NewDaemon(ctx context.Context, env environment, cfg *config.Config) *Daemon {
	clientCert, err := os.ReadFile(cfg.ClientCertificateFilename)
	if err != nil {
		slog.WarnContext(ctx, "failed to read client certificate", slog.String("file", cfg.ClientCertificateFilename), logger.Err(err))
	}

	clientKey, err := os.ReadFile(cfg.ClientKeyFilename)
	if err != nil {
		slog.WarnContext(ctx, "failed to read client key", slog.String("file", cfg.ClientKeyFilename), logger.Err(err))
	}

	d := &Daemon{
		env:               env,
		config:            cfg,
		clientCertificate: string(clientCert),
		clientKey:         string(clientKey),
	}

	return d
}

func (d *Daemon) Start(ctx context.Context) error {
	slog.InfoContext(ctx, "Starting up", slog.String("version", version.Version))

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

	// TODO: Decide on the usage of the GITHUB_TOKEN. It is necessary to avoid
	// being hit by the Github rate limiting.
	gh := ghClient.NewClient(nil).WithAuthToken(os.Getenv("GITHUB_TOKEN"))

	serverClientProvider := serverMiddleware.NewServerClientWithSlog(
		incusAdapter.New(
			d.clientCertificate,
			d.clientKey,
		),
		slog.Default(),
	)

	// Setup Services
	tokenSvc := provisioningServiceMiddleware.NewTokenServiceWithSlog(
		provisioning.NewTokenService(
			provisioningRepoMiddleware.NewTokenRepoWithSlog(
				provisioningSqlite.NewToken(dbWithTransaction),
				slog.Default(),
			),
		),
		slog.Default(),
	)

	clusterSvc := provisioning.NewClusterService(
		provisioningRepoMiddleware.NewClusterRepoWithSlog(
			provisioningSqlite.NewCluster(dbWithTransaction),
			slog.Default(),
		),
		nil,
	)
	clusterSvcWrapped := provisioningServiceMiddleware.NewClusterServiceWithSlog(
		clusterSvc,
		slog.Default(),
	)

	serverSvc := provisioningServiceMiddleware.NewServerServiceWithSlog(
		provisioning.NewServerService(
			provisioningRepoMiddleware.NewServerRepoWithSlog(
				provisioningSqlite.NewServer(dbWithTransaction),
				slog.Default(),
			),
		),
		slog.Default(),
	)

	updateSvc := provisioningServiceMiddleware.NewUpdateServiceWithSlog(
		provisioning.NewUpdateService(
			provisioningRepoMiddleware.NewUpdateRepoWithSlog(
				github.NewUpdate(gh),
				slog.Default(),
			),
		),
		slog.Default(),
	)

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

	provisioningTokenRouter := newSubRouter(provisioningRouter, "/tokens")
	registerProvisioningTokenHandler(provisioningTokenRouter, tokenSvc)

	provisioningClusterRouter := newSubRouter(provisioningRouter, "/clusters")
	registerProvisioningClusterHandler(provisioningClusterRouter, clusterSvcWrapped)

	provisioningServerRouter := newSubRouter(provisioningRouter, "/servers")
	registerProvisioningServerHandler(provisioningServerRouter, serverSvc)

	updateRouter := newSubRouter(provisioningRouter, "/updates")
	registerUpdateHandler(updateRouter, updateSvc)

	inventoryRouter := newSubRouter(api10router, "/inventory")

	inventorySyncers := registerInventoryRoutes(dbWithTransaction, clusterSvcWrapped, serverSvc, serverClientProvider, inventoryRouter)

	clusterSvc.SetInventorySyncers(inventorySyncers)

	errorLogger := &log.Logger{}
	errorLogger.SetOutput(httpErrorLogger{})

	// Setup web server
	d.server = &http.Server{
		Handler: logger.RequestIDMiddleware(
			logger.AccessLogMiddleware(
				router,
			),
		),
		IdleTimeout: 30 * time.Second,
		Addr:        fmt.Sprintf("%s:%d", d.config.RestServerAddr, d.config.RestServerPort),
		ErrorLog:    errorLogger,
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

		slog.InfoContext(ctx, "Start unix socket listener", slog.Any("addr", unixListener.Addr()))

		err = d.server.Serve(unixListener)
		if errors.Is(err, http.ErrServerClosed) {
			// Ignore error from graceful shutdown.
			return nil
		}

		return err
	})

	group.Go(func() error {
		slog.InfoContext(ctx, "Start https listener", slog.Any("addr", d.server.Addr))

		certFile := filepath.Join(d.env.VarDir(), "server.crt")
		keyFile := filepath.Join(d.env.VarDir(), "server.key")

		// Ensure that the certificate exists, or create a new one if it does not.
		err := incusTLS.FindOrGenCert(certFile, keyFile, false, true)
		if err != nil {
			return err
		}

		err = d.server.ListenAndServeTLS(certFile, keyFile)
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

type httpErrorLogger struct{}

func (httpErrorLogger) Write(p []byte) (n int, err error) {
	slog.ErrorContext(context.Background(), string(p)) //nolint:sloglint // error message coming from the http server is the message.
	return len(p), nil
}
