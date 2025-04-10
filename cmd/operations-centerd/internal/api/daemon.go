package api

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	ghClient "github.com/google/go-github/v69/github"
	incusTLS "github.com/lxc/incus/v6/shared/tls"
	"golang.org/x/sync/errgroup"

	"github.com/FuturFusion/operations-center/cmd/operations-centerd/internal/config"
	"github.com/FuturFusion/operations-center/internal/authn"
	"github.com/FuturFusion/operations-center/internal/dbschema"
	"github.com/FuturFusion/operations-center/internal/file"
	incusAdapter "github.com/FuturFusion/operations-center/internal/inventory/server/incus"
	serverMiddleware "github.com/FuturFusion/operations-center/internal/inventory/server/middleware"
	"github.com/FuturFusion/operations-center/internal/logger"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	provisioningServiceMiddleware "github.com/FuturFusion/operations-center/internal/provisioning/middleware"
	"github.com/FuturFusion/operations-center/internal/provisioning/repo/github"
	provisioningRepoMiddleware "github.com/FuturFusion/operations-center/internal/provisioning/repo/middleware"
	provisioningSqlite "github.com/FuturFusion/operations-center/internal/provisioning/repo/sqlite"
	"github.com/FuturFusion/operations-center/internal/provisioning/repo/sqlite/entities"
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
	clientCertFilename := filepath.Join(env.VarDir(), cfg.ClientCertificateFilename)
	clientCert, err := os.ReadFile(clientCertFilename)
	if err != nil {
		slog.WarnContext(ctx, "failed to read client certificate", slog.String("file", clientCertFilename), logger.Err(err))
	}

	clientKeyFilename := filepath.Join(env.VarDir(), cfg.ClientKeyFilename)
	clientKey, err := os.ReadFile(clientKeyFilename)
	if err != nil {
		slog.WarnContext(ctx, "failed to read client key", slog.String("file", clientKeyFilename), logger.Err(err))
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

	// TODO: should Ensure take the provided context? If not, document the reason.
	_, err = dbschema.Ensure(context.TODO(), db, d.env.VarDir())
	if err != nil {
		return err
	}

	dbWithTransaction := transaction.Enable(db)
	entities.PreparedStmts, err = entities.PrepareStmts(dbWithTransaction, false)
	if err != nil {
		return err
	}

	// TODO: setup certificates

	authenticator := authn.New(d.config.TrustedTLSClientCertFingerprints)

	// TODO: setup authorizer

	// TODO: setup OIDC

	gh := ghClient.NewClient(nil)
	if d.config.GithubToken != "" {
		gh = gh.WithAuthToken(d.config.GithubToken)
	}

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

	serverSvc := provisioningServiceMiddleware.NewServerServiceWithSlog(
		provisioning.NewServerService(
			provisioningRepoMiddleware.NewServerRepoWithSlog(
				provisioningSqlite.NewServer(dbWithTransaction),
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
		serverSvc,
		nil,
	)
	clusterSvcWrapped := provisioningServiceMiddleware.NewClusterServiceWithSlog(
		clusterSvc,
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
	serveMux := http.NewServeMux()
	// TODO: Move access log and request ID middlewares here
	router := newRouter(serveMux)

	registerUIHandlers(router, d.env.VarDir())

	api10router := router.SubGroup("/1.0").AddMiddlewares(authenticator.Middleware)
	registerAPI10Handler(api10router)

	provisioningRouter := api10router.SubGroup("/provisioning")

	provisioningTokenRouter := provisioningRouter.SubGroup("/tokens")
	registerProvisioningTokenHandler(provisioningTokenRouter, tokenSvc)

	provisioningClusterRouter := provisioningRouter.SubGroup("/clusters")
	registerProvisioningClusterHandler(provisioningClusterRouter, clusterSvcWrapped)

	provisioningServerRouter := provisioningRouter.SubGroup("/servers")
	registerProvisioningServerHandler(provisioningServerRouter, serverSvc)

	updateRouter := provisioningRouter.SubGroup("/updates")
	registerUpdateHandler(updateRouter, updateSvc)

	inventoryRouter := api10router.SubGroup("/inventory")

	inventorySyncers := registerInventoryRoutes(dbWithTransaction, clusterSvcWrapped, serverClientProvider, inventoryRouter)

	clusterSvc.SetInventorySyncers(inventorySyncers)

	errorLogger := &log.Logger{}
	errorLogger.SetOutput(httpErrorLogger{})

	// Setup web server
	d.server = &http.Server{
		Handler: logger.RequestIDMiddleware(
			logger.AccessLogMiddleware(
				serveMux,
			),
		),
		IdleTimeout: 30 * time.Second,
		Addr:        fmt.Sprintf("%s:%d", d.config.RestServerAddr, d.config.RestServerPort),
		ErrorLog:    errorLogger,
		TLSConfig: &tls.Config{
			NextProtos: []string{"h2", "http/1.1"},
			ClientAuth: tls.RequestClientCert,
		},
	}

	group, errgroupCtx := errgroup.WithContext(context.Background())
	d.errgroup = group

	group.Go(func() error {
		// TODO: if the socket file already exists, make a connection attempt. If
		// successful, another instance of operations-centerd is already running.
		// If not successful, it is save to delete the socket file.
		if file.PathExists(d.env.GetUnixSocket()) {
			err = os.Remove(d.env.GetUnixSocket())
			if err != nil {
				return err
			}
		}

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

type httpErrorLogger struct{}

func (httpErrorLogger) Write(p []byte) (n int, err error) {
	slog.ErrorContext(context.Background(), string(p)) //nolint:sloglint // error message coming from the http server is the message.
	return len(p), nil
}
