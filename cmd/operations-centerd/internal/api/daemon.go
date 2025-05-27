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
	"strings"
	"time"

	ghClient "github.com/google/go-github/v69/github"
	incusTLS "github.com/lxc/incus/v6/shared/tls"
	"golang.org/x/sync/errgroup"

	"github.com/FuturFusion/operations-center/cmd/operations-centerd/internal/config"
	"github.com/FuturFusion/operations-center/internal/authn"
	authnoidc "github.com/FuturFusion/operations-center/internal/authn/oidc"
	authntls "github.com/FuturFusion/operations-center/internal/authn/tls"
	authnunixsocket "github.com/FuturFusion/operations-center/internal/authn/unixsocket"
	"github.com/FuturFusion/operations-center/internal/authz"
	authzchain "github.com/FuturFusion/operations-center/internal/authz/chain"
	authzopenfga "github.com/FuturFusion/operations-center/internal/authz/openfga"
	authztlz "github.com/FuturFusion/operations-center/internal/authz/tls"
	"github.com/FuturFusion/operations-center/internal/authz/unixsocket"
	"github.com/FuturFusion/operations-center/internal/dbschema"
	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/file"
	incusAdapter "github.com/FuturFusion/operations-center/internal/inventory/server/incus"
	serverMiddleware "github.com/FuturFusion/operations-center/internal/inventory/server/middleware"
	"github.com/FuturFusion/operations-center/internal/logger"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/provisioning/adapter/filecache"
	"github.com/FuturFusion/operations-center/internal/provisioning/adapter/github"
	"github.com/FuturFusion/operations-center/internal/provisioning/adapter/local"
	provisioningAdapterMiddleware "github.com/FuturFusion/operations-center/internal/provisioning/adapter/middleware"
	provisioningServiceMiddleware "github.com/FuturFusion/operations-center/internal/provisioning/middleware"
	provisioningRepoMiddleware "github.com/FuturFusion/operations-center/internal/provisioning/repo/middleware"
	provisioningSqlite "github.com/FuturFusion/operations-center/internal/provisioning/repo/sqlite"
	"github.com/FuturFusion/operations-center/internal/provisioning/repo/sqlite/entities"
	dbdriver "github.com/FuturFusion/operations-center/internal/sqlite"
	"github.com/FuturFusion/operations-center/internal/task"
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

	shutdownFuncs []func(context.Context) error
	errgroup      *errgroup.Group
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

	// UnixSocket authenticator is always available.
	authers := []authn.Auther{
		authnunixsocket.UnixSocket{},
	}

	// Setup OIDC authentication.
	var oidcVerifier *authnoidc.Verifier
	if d.config.OidcIssuer != "" && d.config.OidcClientID != "" {
		oidcVerifier, err = authnoidc.NewVerifier(context.TODO(), d.config.OidcIssuer, d.config.OidcClientID, d.config.OidcScope, d.config.OidcAudience, d.config.OidcClaim)
		if err != nil {
			return err
		}

		authers = append(authers, authnoidc.New(oidcVerifier))
	}

	// Setup client cert fingerprint authentication.
	if len(d.config.TrustedTLSClientCertFingerprints) > 0 {
		authers = append(authers, authntls.New(d.config.TrustedTLSClientCertFingerprints))
	}

	// Create authenticator
	authenticator := authn.New(authers)

	// TODO: setup authorizer
	authorizers := []authz.Authorizer{
		unixsocket.New(),
		authztlz.New(ctx, d.config.TrustedTLSClientCertFingerprints),
	}

	if d.config.OpenfgaAPIURL != "" && d.config.OpenfgaAPIToken != "" && d.config.OpenfgaStoreID != "" {
		openfgaAuthorizer, err := authzopenfga.New(ctx, d.config.OpenfgaAPIURL, d.config.OpenfgaAPIToken, d.config.OpenfgaStoreID)
		if err != nil {
			// TODO: cloud also be a warning
			return err
		}

		authorizers = append(authorizers, openfgaAuthorizer)
		d.shutdownFuncs = append(d.shutdownFuncs, openfgaAuthorizer.Shutdown)
	}

	authorizer := authzchain.New(authorizers...)

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
			tokenSvc,
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

	githubSourceWithCache, err := filecache.New(
		github.New(gh),
		filepath.Join(d.env.VarDir(), "updates_cache"),
	)
	if err != nil {
		return err
	}

	localSource, err := local.New(
		filepath.Join(d.env.VarDir(), "updates_local"),
	)
	if err != nil {
		return err
	}

	updateSvc := provisioningServiceMiddleware.NewUpdateServiceWithSlog(
		provisioning.NewUpdateService(
			provisioningRepoMiddleware.NewUpdateRepoWithSlog(
				provisioningSqlite.NewUpdate(dbWithTransaction),
				slog.Default(),
				provisioningRepoMiddleware.UpdateRepoWithSlogWithInformativeErrFunc(
					func(err error) bool {
						return errors.Is(err, domain.ErrNotFound)
					},
				),
			),
			provisioning.UpdateServiceWithSource(
				"github.com/lxc/incus-os",
				provisioningAdapterMiddleware.NewUpdateSourcePortWithSlog(
					githubSourceWithCache,
					slog.Default(),
				),
			),
			provisioning.UpdateServiceWithSource(
				"local",
				provisioningAdapterMiddleware.NewUpdateSourceWithAddPortWithSlog(
					localSource,
					slog.Default(),
				),
			),
			provisioning.UpdateServiceWithLatestLimit(3),
		),
		slog.Default(),
	)

	// Setup Routes
	serveMux := http.NewServeMux()
	// TODO: Move access log and request ID middlewares here
	router := newRouter(serveMux)

	registerUIHandlers(router, d.env.VarDir())

	if oidcVerifier != nil {
		registerOIDCHandlers(router, oidcVerifier)
	}

	isNoAuthenticationRequired := func(r *http.Request) bool {
		// POST /1.0/provisioning/servers is authenticated using a token.
		if r.Method == http.MethodPost && r.URL.Path == "/1.0/provisioning/servers" {
			return true
		}

		// GET /1.0/provisioning/updates no authentication required to get updates.
		if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/1.0/provisioning/updates") {
			return true
		}

		return false
	}

	api10router := router.SubGroup("/1.0").AddMiddlewares(
		// Authentication middleware is skipped if isNoAuthenticationRequired applies.
		unless(
			authenticator.Middleware,
			isNoAuthenticationRequired,
		),
	)
	registerAPI10Handler(api10router)

	provisioningRouter := api10router.SubGroup("/provisioning")

	provisioningTokenRouter := provisioningRouter.SubGroup("/tokens")
	registerProvisioningTokenHandler(provisioningTokenRouter, authorizer, tokenSvc)

	provisioningClusterRouter := provisioningRouter.SubGroup("/clusters")
	registerProvisioningClusterHandler(provisioningClusterRouter, authorizer, clusterSvcWrapped)

	provisioningServerRouter := provisioningRouter.SubGroup("/servers")
	registerProvisioningServerHandler(provisioningServerRouter, authorizer, serverSvc)

	updateRouter := provisioningRouter.SubGroup("/updates")
	registerUpdateHandler(updateRouter, authorizer, updateSvc)

	inventoryRouter := api10router.SubGroup("/inventory")

	inventorySyncers := registerInventoryRoutes(dbWithTransaction, clusterSvcWrapped, serverClientProvider, authorizer, inventoryRouter)

	clusterSvc.SetInventorySyncers(inventorySyncers)

	errorLogger := &log.Logger{}
	errorLogger.SetOutput(httpErrorLogger{})

	// Setup web server
	server := &http.Server{
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

	d.shutdownFuncs = append(d.shutdownFuncs, server.Shutdown)

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

		err = server.Serve(unixListener)
		if errors.Is(err, http.ErrServerClosed) {
			// Ignore error from graceful shutdown.
			return nil
		}

		return err
	})

	group.Go(func() error {
		slog.InfoContext(ctx, "Start https listener", slog.Any("addr", server.Addr))

		certFile := filepath.Join(d.env.VarDir(), "server.crt")
		keyFile := filepath.Join(d.env.VarDir(), "server.key")

		// Ensure that the certificate exists, or create a new one if it does not.
		err := incusTLS.FindOrGenCert(certFile, keyFile, false, true)
		if err != nil {
			return err
		}

		err = server.ListenAndServeTLS(certFile, keyFile)
		if errors.Is(err, http.ErrServerClosed) {
			// Ignore error from graceful shutdown.
			return nil
		}

		return err
	})

	refreshTask := func(ctx context.Context) {
		slog.InfoContext(ctx, "Refresh updates triggered")
		err := updateSvc.Refresh(ctx)
		if err != nil {
			slog.ErrorContext(ctx, "Refresh updates failed", logger.Err(err))
		} else {
			slog.InfoContext(ctx, "Refresh updates completed")
		}
	}

	var updateSourceOptions []task.EveryOption
	if d.config.UpdatesSourcePollSkipFirst {
		updateSourceOptions = append(updateSourceOptions, task.SkipFirst)
	}

	updateSourceStop, _ := task.Start(ctx, refreshTask, task.Every(d.config.UpdatesSourcePollInterval, updateSourceOptions...))
	d.shutdownFuncs = append(d.shutdownFuncs, func(ctx context.Context) error {
		// Default grace period for tasks to finish during shutdown, if no deadline
		// is defined on the context.
		d := 60 * time.Second

		deadline, ok := ctx.Deadline()
		if ok {
			deadlineDuration := time.Until(deadline)
			if deadlineDuration > 0 {
				d = deadlineDuration
			}
		}

		return updateSourceStop(d)
	})

	select {
	case <-errgroupCtx.Done():
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer shutdownCancel()
		return d.Stop(shutdownCtx)
	case <-time.After(50 * time.Millisecond):
		// Grace period we wait for potential immediate errors from serving the http server.
		// TODO: More clean way would be to check if the listeners are reachable (http, unix socket).
	}

	return nil
}

func (d *Daemon) Stop(ctx context.Context) error {
	errs := make([]error, len(d.shutdownFuncs)+1)

	for _, shutdown := range d.shutdownFuncs {
		err := shutdown(ctx)
		errs = append(errs, err)
	}

	errgroupWaitErr := d.errgroup.Wait()
	errs = append(errs, errgroupWaitErr)

	return errors.Join(errs...)
}

type httpErrorLogger struct{}

func (httpErrorLogger) Write(p []byte) (n int, err error) {
	slog.ErrorContext(context.Background(), string(p)) //nolint:sloglint // error message coming from the http server is the message.
	return len(p), nil
}
