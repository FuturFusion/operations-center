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
	"sync"
	"time"

	incusTLS "github.com/lxc/incus/v6/shared/tls"
	"github.com/maniartech/signals"
	"golang.org/x/sync/errgroup"

	"github.com/FuturFusion/operations-center/internal/api/listener"
	"github.com/FuturFusion/operations-center/internal/authn"
	authnoidc "github.com/FuturFusion/operations-center/internal/authn/oidc"
	authntls "github.com/FuturFusion/operations-center/internal/authn/tls"
	authnunixsocket "github.com/FuturFusion/operations-center/internal/authn/unixsocket"
	"github.com/FuturFusion/operations-center/internal/authz"
	authzchain "github.com/FuturFusion/operations-center/internal/authz/chain"
	oidcAuthorizer "github.com/FuturFusion/operations-center/internal/authz/oidc"
	authzopenfga "github.com/FuturFusion/operations-center/internal/authz/openfga"
	authztls "github.com/FuturFusion/operations-center/internal/authz/tls"
	"github.com/FuturFusion/operations-center/internal/authz/unixsocket"
	config "github.com/FuturFusion/operations-center/internal/config/daemon"
	"github.com/FuturFusion/operations-center/internal/dbschema"
	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/file"
	inventoryIncusAdapter "github.com/FuturFusion/operations-center/internal/inventory/server/incus"
	serverMiddleware "github.com/FuturFusion/operations-center/internal/inventory/server/middleware"
	"github.com/FuturFusion/operations-center/internal/logger"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/provisioning/adapter/flasher"
	provisioningIncusAdapter "github.com/FuturFusion/operations-center/internal/provisioning/adapter/incus"
	provisioningAdapterMiddleware "github.com/FuturFusion/operations-center/internal/provisioning/adapter/middleware"
	"github.com/FuturFusion/operations-center/internal/provisioning/adapter/terraform"
	"github.com/FuturFusion/operations-center/internal/provisioning/adapter/updateserver"
	provisioningServiceMiddleware "github.com/FuturFusion/operations-center/internal/provisioning/middleware"
	"github.com/FuturFusion/operations-center/internal/provisioning/repo/localfs"
	provisioningRepoMiddleware "github.com/FuturFusion/operations-center/internal/provisioning/repo/middleware"
	provisioningSqlite "github.com/FuturFusion/operations-center/internal/provisioning/repo/sqlite"
	"github.com/FuturFusion/operations-center/internal/provisioning/repo/sqlite/entities"
	dbdriver "github.com/FuturFusion/operations-center/internal/sqlite"
	"github.com/FuturFusion/operations-center/internal/system"
	systemServiceMiddleware "github.com/FuturFusion/operations-center/internal/system/middleware"
	"github.com/FuturFusion/operations-center/internal/task"
	"github.com/FuturFusion/operations-center/internal/transaction"
	"github.com/FuturFusion/operations-center/internal/version"
	"github.com/FuturFusion/operations-center/shared/api"
)

type environment interface {
	GetUnixSocket() string
	VarDir() string
	UsrShareDir() string
}

type Daemon struct {
	env environment

	// Global mutex for any changes to the daemon config including authenticator
	// oidcVerifier, authorizer and the likes.
	configReloadMu *sync.Mutex

	clientCertificate string
	clientKey         string

	authenticator *authn.Authenticator
	oidcVerifier  *authnoidc.Verifier
	authorizer    *authz.Authorizer

	server   *http.Server
	listener *listener.FancyTLSListener

	serverCertificateUpdate signals.Signal[tls.Certificate]
	serverCertificate       tls.Certificate

	shutdownFuncs []func(context.Context) error
	errgroup      *errgroup.Group
}

func NewDaemon(ctx context.Context, env environment) *Daemon {
	clientCertFilename := filepath.Join(env.VarDir(), config.ClientCertificateFilename)
	clientCert, err := os.ReadFile(clientCertFilename)
	if err != nil {
		slog.WarnContext(ctx, "failed to read client certificate", slog.String("file", clientCertFilename), logger.Err(err))
	}

	clientKeyFilename := filepath.Join(env.VarDir(), config.ClientKeyFilename)
	clientKey, err := os.ReadFile(clientKeyFilename)
	if err != nil {
		slog.WarnContext(ctx, "failed to read client key", slog.String("file", clientKeyFilename), logger.Err(err))
	}

	d := &Daemon{
		env:               env,
		configReloadMu:    &sync.Mutex{},
		clientCertificate: string(clientCert),
		clientKey:         string(clientKey),

		authenticator: &authn.Authenticator{},
		oidcVerifier:  &authnoidc.Verifier{},
		authorizer: func() *authz.Authorizer {
			var authorizer authz.Authorizer = authzchain.New()
			return &authorizer
		}(),

		serverCertificateUpdate: signals.NewSync[tls.Certificate](),
	}

	return d
}

func (d *Daemon) Start(ctx context.Context) error {
	slog.InfoContext(ctx, "Starting up", slog.String("version", version.Version))

	dbWithTransaction, err := d.initDB(ctx)
	if err != nil {
		return err
	}

	err = d.initAndLoadServerCert()
	if err != nil {
		return err
	}

	// Initialize security related infrastructure like authenticators and
	// authorizers on the daemon.
	err = d.securityConfigReload(ctx, config.GetSecurity())
	if err != nil {
		return err
	}

	// On update of the security configuration, perform reload of the security
	// related infrastructure.
	config.SecurityUpdateSignal.AddListener(func(ctx context.Context, cfg api.SystemSecurity) {
		err := d.securityConfigReload(ctx, cfg)
		if err != nil {
			slog.ErrorContext(ctx, "failed to reload security config", logger.Err(err))
		}
	})

	// Setup Services
	updateSvc, err := d.setupUpdatesService(dbWithTransaction)
	if err != nil {
		return err
	}

	tokenSvc := d.setupTokenService(dbWithTransaction, updateSvc)
	serverSvc := d.setupServerService(dbWithTransaction, tokenSvc)
	clusterSvc, err := d.setupClusterService(dbWithTransaction, serverSvc)
	if err != nil {
		return err
	}

	systemSvc := d.setupSystemService()

	// Setup API routes
	serveMux, inventorySyncers := d.setupAPIRoutes(updateSvc, tokenSvc, serverSvc, clusterSvc, systemSvc, dbWithTransaction)

	clusterSvc.SetInventorySyncers(inventorySyncers)

	// Setup API server
	errorLogger := &log.Logger{}
	errorLogger.SetOutput(httpErrorLogger{})

	d.server = &http.Server{
		Handler: logger.RequestIDMiddleware(
			logger.AccessLogMiddleware(
				serveMux,
			),
		),
		IdleTimeout: 30 * time.Second,
		ErrorLog:    errorLogger,
	}

	d.shutdownFuncs = append(d.shutdownFuncs, d.server.Shutdown)

	group, errgroupCtx := errgroup.WithContext(context.Background())
	d.errgroup = group

	// API server on unix socket
	d.setupSocketListener(ctx)

	// API server on TCP
	err = d.setupTCPListener(ctx, config.GetNetwork())
	if err != nil {
		return err
	}

	// If the network configuration changes, we need to reload the API server on TCP.
	config.NetworkUpdateSignal.AddListener(func(ctx context.Context, sn api.SystemNetwork) {
		err := d.setupTCPListener(ctx, sn)
		if err != nil {
			slog.ErrorContext(ctx, "Failed to reload network config", logger.Err(err))
		}
	})

	// Background tasks
	d.setupBackgroundTasks(ctx, updateSvc, serverSvc, clusterSvc)

	// Finalize daemon start
	// Wait for immediate errors during startup.
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

func (d *Daemon) initDB(_ context.Context) (dbdriver.DBTX, error) {
	db, err := dbdriver.Open(d.env.VarDir())
	if err != nil {
		return nil, fmt.Errorf("Failed to open sqlite database: %w", err)
	}

	// TODO: should Ensure take the provided context? If not, document the reason.
	_, err = dbschema.Ensure(context.TODO(), db, d.env.VarDir())
	if err != nil {
		return nil, err
	}

	dbWithTransaction := transaction.Enable(db)
	entities.PreparedStmts, err = entities.PrepareStmts(dbWithTransaction, false)
	if err != nil {
		return nil, err
	}

	return dbWithTransaction, nil
}

func (d *Daemon) initAndLoadServerCert() error {
	certFile := filepath.Join(d.env.VarDir(), "server.crt")
	keyFile := filepath.Join(d.env.VarDir(), "server.key")

	// Ensure that the certificate exists, or create a new one if it does not.
	err := incusTLS.FindOrGenCert(certFile, keyFile, false, true)
	if err != nil {
		return err
	}

	serverCertificatePEM, err := os.ReadFile(certFile)
	if err != nil {
		return fmt.Errorf("Failed to read server certificate from %q: %w", certFile, err)
	}

	serverKeyPEM, err := os.ReadFile(keyFile)
	if err != nil {
		return fmt.Errorf("Failed to read server key from %q: %w", keyFile, err)
	}

	d.serverCertificate, err = tls.X509KeyPair(serverCertificatePEM, serverKeyPEM)
	if err != nil {
		return fmt.Errorf("Failed to validate server certificate key pair: %w", err)
	}

	return nil
}

func (d *Daemon) securityConfigReload(ctx context.Context, cfg api.SystemSecurity) error {
	d.configReloadMu.Lock()
	defer d.configReloadMu.Unlock()

	var errs []error

	// UnixSocket authenticator is always available.
	authers := []authn.Auther{
		authnunixsocket.UnixSocket{},
	}

	// Setup OIDC authentication.
	if cfg.OIDC.Issuer != "" && cfg.OIDC.ClientID != "" {
		var err error
		newOIDCVerifier, err := authnoidc.NewVerifier(context.TODO(), cfg.OIDC.Issuer, cfg.OIDC.ClientID, cfg.OIDC.Scope, cfg.OIDC.Audience, cfg.OIDC.Claim)
		if err != nil {
			errs = append(errs, err)
		} else {
			*d.oidcVerifier = *newOIDCVerifier

			authers = append(authers, authnoidc.New(newOIDCVerifier))
		}
	}

	// Setup client cert fingerprint authentication.
	if len(cfg.TrustedTLSClientCertFingerprints) > 0 {
		authers = append(authers, authntls.New(cfg.TrustedTLSClientCertFingerprints))
	}

	// Create authenticator
	*d.authenticator = authn.New(authers)

	authorizers := []authz.Authorizer{
		unixsocket.New(),
		authztls.New(ctx, cfg.TrustedTLSClientCertFingerprints),
	}

	if cfg.OpenFGA.APIURL != "" && cfg.OpenFGA.APIToken != "" && cfg.OpenFGA.StoreID != "" {
		openfgaAuthorizer, err := authzopenfga.New(ctx, cfg.OpenFGA.APIURL, cfg.OpenFGA.APIToken, cfg.OpenFGA.StoreID)
		if err != nil {
			errs = append(errs, err)
		} else {
			authorizers = append(authorizers, openfgaAuthorizer)
		}
	}

	// If OIDC is configured and OpenFGA is explicitly not configured, grant
	// unrestricted access to all authenticated OIDC users.
	if cfg.OIDC.Issuer != "" && cfg.OIDC.ClientID != "" && cfg.OpenFGA.APIURL == "" && cfg.OpenFGA.APIToken == "" && cfg.OpenFGA.StoreID == "" {
		authorizers = append(authorizers, oidcAuthorizer.New())
	}

	*d.authorizer = authzchain.New(authorizers...)

	return errors.Join(errs...)
}

func (d *Daemon) setupUpdatesService(db dbdriver.DBTX) (provisioning.UpdateService, error) {
	repoUpdateFiles, err := localfs.New(
		filepath.Join(d.env.VarDir(), "updates"),
		config.GetUpdates().SignatureVerificationRootCA,
	)
	if err != nil {
		return nil, err
	}

	// Make sure, the files repository learns about changes to the signature certificate.
	config.UpdatesUpdateSignal.AddListener(func(ctx context.Context, cfg api.SystemUpdates) {
		repoUpdateFiles.UpdateConfig(ctx, cfg.SignatureVerificationRootCA)
	})

	updateServiceOptions := []provisioning.UpdateServiceOption{
		provisioning.UpdateServiceWithLatestLimit(3),
		provisioning.UpdateServiceWithFilterExpression(config.GetUpdates().FilterExpression),
		provisioning.UpdateServiceWithFileFilterExpression(config.GetUpdates().FileFilterExpression),
	}

	updateServer := updateserver.New(
		config.GetUpdates().Source,
		config.GetUpdates().SignatureVerificationRootCA,
	)
	config.UpdatesUpdateSignal.AddListener(func(ctx context.Context, cfg api.SystemUpdates) {
		updateServer.UpdateConfig(ctx, cfg.Source, cfg.SignatureVerificationRootCA)
	})

	updateSvcBase := provisioning.NewUpdateService(
		provisioningRepoMiddleware.NewUpdateRepoWithSlog(
			provisioningSqlite.NewUpdate(db),
			slog.Default(),
			provisioningRepoMiddleware.UpdateRepoWithSlogWithInformativeErrFunc(
				func(err error) bool {
					return errors.Is(err, domain.ErrNotFound)
				},
			),
		),
		provisioningRepoMiddleware.NewUpdateFilesRepoWithSlog(
			repoUpdateFiles,
			slog.Default(),
		),
		provisioningAdapterMiddleware.NewUpdateSourcePortWithSlog(
			updateServer,
			slog.Default(),
		),
		updateServiceOptions...,
	)
	config.UpdatesUpdateSignal.AddListener(func(ctx context.Context, cfg api.SystemUpdates) {
		updateSvcBase.UpdateConfig(ctx, cfg.FilterExpression, cfg.FileFilterExpression)
	})

	return provisioningServiceMiddleware.NewUpdateServiceWithSlog(
		updateSvcBase,
		slog.Default(),
	), nil
}

func (d *Daemon) setupTokenService(db dbdriver.DBTX, updateSvc provisioning.UpdateService) provisioning.TokenService {
	imageFlasher := flasher.New(
		config.GetNetwork().OperationsCenterAddress,
		d.serverCertificate,
	)
	// Image flasher needs to learn about updates to the server certificate.
	d.serverCertificateUpdate.AddListener(func(_ context.Context, cert tls.Certificate) {
		imageFlasher.UpdateCertificate(cert)
	})
	// Image flasher needs to learn about updates the public Operations Center address.
	config.NetworkUpdateSignal.AddListener(func(ctx context.Context, cfg api.SystemNetwork) {
		imageFlasher.UpdateServerURL(cfg.OperationsCenterAddress)
	})

	return provisioningServiceMiddleware.NewTokenServiceWithSlog(
		provisioning.NewTokenService(
			provisioningRepoMiddleware.NewTokenRepoWithSlog(
				provisioningSqlite.NewToken(db),
				slog.Default(),
			),
			updateSvc,
			imageFlasher,
		),
		slog.Default(),
	)
}

func (d *Daemon) setupServerService(db dbdriver.DBTX, tokenSvc provisioning.TokenService) provisioning.ServerService {
	return provisioningServiceMiddleware.NewServerServiceWithSlog(
		provisioning.NewServerService(
			provisioningRepoMiddleware.NewServerRepoWithSlog(
				provisioningSqlite.NewServer(db),
				slog.Default(),
			),
			provisioningAdapterMiddleware.NewServerClientPortWithSlog(
				provisioningIncusAdapter.New(
					d.clientCertificate,
					d.clientKey,
				),
				slog.Default(),
				provisioningAdapterMiddleware.ServerClientPortWithSlogWithInformativeErrFunc(
					func(err error) bool {
						// ErrSelfUpdateNotification is used as cause when the context is
						// cancelled. This is an expected success path and therefore not
						// an error.
						return errors.Is(err, provisioning.ErrSelfUpdateNotification)
					},
				),
			),
			tokenSvc,
		),
		slog.Default(),
	)
}

func (d *Daemon) setupClusterService(db dbdriver.DBTX, serverSvc provisioning.ServerService) (provisioning.ClusterService, error) {
	terraformProvisioner, err := terraform.New(
		filepath.Join(d.env.VarDir(), "terraform"),
		d.env.VarDir(),
	)
	if err != nil {
		return nil, err
	}

	return provisioningServiceMiddleware.NewClusterServiceWithSlog(
		provisioning.NewClusterService(
			provisioningRepoMiddleware.NewClusterRepoWithSlog(
				provisioningSqlite.NewCluster(db),
				slog.Default(),
			),
			provisioningAdapterMiddleware.NewClusterClientPortWithSlog(
				provisioningIncusAdapter.New(
					d.clientCertificate,
					d.clientKey,
				),
				slog.Default(),
			),
			serverSvc,
			nil,
			terraformProvisioner,
		),
		slog.Default(),
	), nil
}

func (d *Daemon) setupSystemService() system.SystemService {
	return systemServiceMiddleware.NewSystemServiceWithSlog(
		system.NewSystemService(d.env, d.serverCertificateUpdate),
		slog.Default(),
	)
}

func (d *Daemon) setupAPIRoutes(
	updateSvc provisioning.UpdateService,
	tokenSvc provisioning.TokenService,
	serverSvc provisioning.ServerService,
	clusterSvc provisioning.ClusterService,
	systemSvc system.SystemService,
	db dbdriver.DBTX,
) (*http.ServeMux, []provisioning.InventorySyncer) {
	// serverClientProvider is a provider of a client to access (Incus) servers
	// or clusters.
	serverClientProvider := serverMiddleware.NewServerClientWithSlog(
		inventoryIncusAdapter.New(
			d.clientCertificate,
			d.clientKey,
		),
		slog.Default(),
	)

	serveMux := http.NewServeMux()
	// TODO: Move access log and request ID middlewares here
	router := newRouter(serveMux)

	registerUIHandlers(router, d.env.UsrShareDir())

	const osRouterPrefix = "/os"
	osRouter := router.SubGroup(osRouterPrefix).AddMiddlewares(
		d.authenticator.Middleware,
	)
	registerOSProxy(osRouter, osRouterPrefix, d.authorizer)

	if d.oidcVerifier != nil {
		registerOIDCHandlers(router, d.oidcVerifier)
	}

	isNoAuthenticationRequired := func(r *http.Request) bool {
		// POST /1.0/provisioning/servers is authenticated using a token.
		if r.Method == http.MethodPost && r.URL.Path == "/1.0/provisioning/servers" {
			return true
		}

		// PUT /1.0/provisioning/servers/:self is authenticated using the servers
		// certificate.
		if r.Method == http.MethodPut && r.URL.Path == "/1.0/provisioning/servers/:self" {
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
			d.authenticator.Middleware,
			isNoAuthenticationRequired,
		),
	)
	registerAPI10Handler(api10router)

	provisioningRouter := api10router.SubGroup("/provisioning")

	provisioningTokenRouter := provisioningRouter.SubGroup("/tokens")
	registerProvisioningTokenHandler(provisioningTokenRouter, d.authorizer, tokenSvc)

	provisioningClusterRouter := provisioningRouter.SubGroup("/clusters")
	registerProvisioningClusterHandler(provisioningClusterRouter, d.authorizer, clusterSvc)

	provisioningServerRouter := provisioningRouter.SubGroup("/servers")
	registerProvisioningServerHandler(provisioningServerRouter, d.authorizer, serverSvc, d.clientCertificate)

	provisioningUpdateRouter := provisioningRouter.SubGroup("/updates")
	registerUpdateHandler(provisioningUpdateRouter, d.authorizer, updateSvc)

	systemRouter := api10router.SubGroup("/system")
	registerSystemHandler(systemRouter, d.authorizer, systemSvc)

	inventoryRouter := api10router.SubGroup("/inventory")

	inventorySyncers := registerInventoryRoutes(db, clusterSvc, serverClientProvider, d.authorizer, inventoryRouter)

	return serveMux, inventorySyncers
}

func (d *Daemon) setupBackgroundTasks(
	ctx context.Context,
	updateSvc provisioning.UpdateService,
	serverSvc provisioning.ServerService,
	clusterSvc provisioning.ClusterService,
) {
	// Start background task to refresh updates from the sources.
	refreshUpdatesFromSourcesTask := func(ctx context.Context) {
		slog.InfoContext(ctx, "Refresh updates triggered")
		err := updateSvc.Refresh(ctx)
		if err != nil {
			slog.ErrorContext(ctx, "Refresh updates failed", logger.Err(err))
		} else {
			slog.InfoContext(ctx, "Refresh updates completed")
		}
	}

	var updateSourceOptions []task.EveryOption
	if config.GetUpdates().SourcePollSkipFirst {
		updateSourceOptions = append(updateSourceOptions, task.SkipFirst)
	}

	updateSourceTaskStop, _ := task.Start(ctx, refreshUpdatesFromSourcesTask, task.Every(config.UpdatesSourcePollInterval, updateSourceOptions...))
	d.shutdownFuncs = append(d.shutdownFuncs, func(ctx context.Context) error {
		return updateSourceTaskStop(deadlineFrom(ctx, 60*time.Second))
	})

	// Start background task to poll servers in pending state to become available.
	pollPendingServersTask := func(ctx context.Context) {
		slog.InfoContext(ctx, "Polling for pending servers triggered")
		err := serverSvc.PollServers(ctx, api.ServerStatusPending, true)
		if err != nil {
			slog.ErrorContext(ctx, "Polling for pending servers failed", logger.Err(err))
		} else {
			slog.InfoContext(ctx, "Polling for pending servers completed")
		}
	}

	pollPendingServersTaskStop, _ := task.Start(ctx, pollPendingServersTask, task.Every(config.PendingServerPollInterval))
	d.shutdownFuncs = append(d.shutdownFuncs, func(ctx context.Context) error {
		return pollPendingServersTaskStop(deadlineFrom(ctx, 1*time.Second))
	})

	// Start background task to test connectivity and update configuration with servers in ready state.
	pollReadyServersTask := func(ctx context.Context) {
		slog.InfoContext(ctx, "Connectivity test for ready servers triggered")

		// Within the first connectivityInterval of the hour, we also update the configuration.
		updateConfiguration := time.Since(time.Now().Truncate(time.Hour)) <= config.ConnectivityCheckInterval
		err := serverSvc.PollServers(ctx, api.ServerStatusReady, updateConfiguration)
		if err != nil {
			slog.ErrorContext(ctx, "Connectivity test for some servers failed", logger.Err(err))
		} else {
			slog.InfoContext(ctx, "Connectivity test for ready servers completed")
		}
	}

	pollReadyServersTaskStop, _ := task.Start(ctx, pollReadyServersTask, task.Every(config.ConnectivityCheckInterval))
	d.shutdownFuncs = append(d.shutdownFuncs, func(ctx context.Context) error {
		return pollReadyServersTaskStop(deadlineFrom(ctx, 1*time.Second))
	})

	// Start background task to refresh inventory.
	refreshInventoryTask := func(ctx context.Context) {
		slog.InfoContext(ctx, "Inventory update triggered")
		err := clusterSvc.ResyncInventory(ctx)
		if err != nil {
			slog.ErrorContext(ctx, "Inventory update failed", logger.Err(err))
		} else {
			slog.InfoContext(ctx, "Inventory update completed")
		}
	}

	refreshInventoryTaskStop, _ := task.Start(ctx, refreshInventoryTask, task.Every(config.InventoryUpdateInterval))
	d.shutdownFuncs = append(d.shutdownFuncs, func(ctx context.Context) error {
		return refreshInventoryTaskStop(deadlineFrom(ctx, 10*time.Second))
	})
}

func (d *Daemon) setupSocketListener(ctx context.Context) {
	d.errgroup.Go(func() error {
		// TODO: if the socket file already exists, make a connection attempt. If
		// successful, another instance of operations-centerd is already running.
		// If not successful, it is save to delete the socket file.
		if file.PathExists(d.env.GetUnixSocket()) {
			err := os.Remove(d.env.GetUnixSocket())
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
}

func (d *Daemon) setupTCPListener(ctx context.Context, cfg api.SystemNetwork) error {
	errCh := make(chan error)
	d.errgroup.Go(func() error {
		d.configReloadMu.Lock()
		oldListener := d.listener
		d.configReloadMu.Unlock()

		if oldListener != nil {
			slog.InfoContext(ctx, "Stopping existing https listener", slog.Any("addr", oldListener.Addr().String()))
			err := oldListener.Close()
			if err != nil {
				errCh <- err
				return err
			}
		}

		d.serverCertificateUpdate.RemoveListener("fancyListener")

		if cfg.RestServerAddress == "" {
			d.configReloadMu.Lock()
			d.listener = nil
			d.configReloadMu.Unlock()

			// Unblock the channel here, since we do not start a server.
			errCh <- nil

			return nil
		}

		newAddr := cfg.RestServerAddress
		_, port, err := net.SplitHostPort(cfg.RestServerAddress)
		if err != nil {
			return err
		}

		if port == "" {
			newAddr += config.DefaultRestServerPort
		}

		if port == "0" {
			newAddr = newAddr[:len(newAddr)-1] + config.DefaultRestServerPort
		}

		d.configReloadMu.Lock()
		d.server.Addr = newAddr
		d.configReloadMu.Unlock()

		slog.InfoContext(ctx, "Start https listener", slog.Any("addr", newAddr))
		tcpListener, err := net.Listen("tcp", newAddr)
		if err != nil {
			errCh <- err
			return err
		}

		d.configReloadMu.Lock()
		d.listener = listener.NewFancyTLSListener(tcpListener, d.serverCertificate)
		d.configReloadMu.Unlock()

		d.serverCertificateUpdate.AddListener(func(_ context.Context, cert tls.Certificate) {
			d.configReloadMu.Lock()
			defer d.configReloadMu.Unlock()

			d.serverCertificate = cert
			d.listener.Config(cert)
		}, "fancyListener")

		// Unblock the channel here before we block for the server.
		errCh <- nil

		if d.server != nil {
			err = d.server.Serve(d.listener)
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

		return nil
	})

	return <-errCh
}

func (d *Daemon) Stop(ctx context.Context) error {
	errs := make([]error, 0, len(d.shutdownFuncs)+1)

	for _, shutdown := range d.shutdownFuncs {
		err := shutdown(ctx)
		errs = append(errs, err)
	}

	if d.errgroup != nil {
		errgroupWaitErr := d.errgroup.Wait()
		errs = append(errs, errgroupWaitErr)
	}

	return errors.Join(errs...)
}
