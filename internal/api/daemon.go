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

	// FIXME: move server cert (as incusTLS.CertInfo) also to daemon level?
	// serverCertLock sync.Mutex
	// serverCert     *incusTLS.CertInfo
	serverCertificate tls.Certificate

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

	certFile := filepath.Join(d.env.VarDir(), "server.crt")
	keyFile := filepath.Join(d.env.VarDir(), "server.key")

	// Ensure that the certificate exists, or create a new one if it does not.
	err = incusTLS.FindOrGenCert(certFile, keyFile, false, true)
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

	d.serverCertificateUpdate = signals.NewSync[tls.Certificate]()

	err = d.securityConfigReload(ctx, config.GetSecurity())
	if err != nil {
		return err
	}

	config.SecurityUpdateSignal.AddListener(func(ctx context.Context, cfg api.SystemSecurity) {
		err := d.securityConfigReload(ctx, cfg)
		if err != nil {
			slog.ErrorContext(ctx, "failed to reload security config", logger.Err(err))
		}
	})

	// Setup Services
	serverClientProvider := serverMiddleware.NewServerClientWithSlog(
		inventoryIncusAdapter.New(
			d.clientCertificate,
			d.clientKey,
		),
		slog.Default(),
	)

	systemSvc := systemServiceMiddleware.NewSystemServiceWithSlog(
		system.NewSystemService(d.env, d.serverCertificateUpdate),
		slog.Default(),
	)

	repoUpdateFiles, err := localfs.New(
		filepath.Join(d.env.VarDir(), "updates"),
		config.GetUpdates().SignatureVerificationRootCA,
	)
	if err != nil {
		return err
	}

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
			provisioningSqlite.NewUpdate(dbWithTransaction),
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

	updateSvc := provisioningServiceMiddleware.NewUpdateServiceWithSlog(
		updateSvcBase,
		slog.Default(),
	)

	imageFlasher := flasher.New(
		config.GetNetwork().OperationsCenterAddress,
		d.serverCertificate,
	)
	d.serverCertificateUpdate.AddListener(func(_ context.Context, cert tls.Certificate) {
		imageFlasher.UpdateCertificate(cert)
	})
	config.NetworkUpdateSignal.AddListener(func(ctx context.Context, cfg api.SystemNetwork) {
		imageFlasher.UpdateServerURL(cfg.OperationsCenterAddress)
	})

	tokenSvc := provisioningServiceMiddleware.NewTokenServiceWithSlog(
		provisioning.NewTokenService(
			provisioningRepoMiddleware.NewTokenRepoWithSlog(
				provisioningSqlite.NewToken(dbWithTransaction),
				slog.Default(),
			),
			updateSvc,
			imageFlasher,
		),
		slog.Default(),
	)

	serverSvc := provisioningServiceMiddleware.NewServerServiceWithSlog(
		provisioning.NewServerService(
			provisioningRepoMiddleware.NewServerRepoWithSlog(
				provisioningSqlite.NewServer(dbWithTransaction),
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

	terraformProvisioner, err := terraform.New(
		filepath.Join(d.env.VarDir(), "terraform"),
		d.env.VarDir(),
	)
	if err != nil {
		return err
	}

	clusterSvc := provisioning.NewClusterService(
		provisioningRepoMiddleware.NewClusterRepoWithSlog(
			provisioningSqlite.NewCluster(dbWithTransaction),
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
	)
	clusterSvcWrapped := provisioningServiceMiddleware.NewClusterServiceWithSlog(
		clusterSvc,
		slog.Default(),
	)

	// Setup Routes
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
	registerProvisioningClusterHandler(provisioningClusterRouter, d.authorizer, clusterSvcWrapped)

	provisioningServerRouter := provisioningRouter.SubGroup("/servers")
	registerProvisioningServerHandler(provisioningServerRouter, d.authorizer, serverSvc, d.clientCertificate)

	provisioningUpdateRouter := provisioningRouter.SubGroup("/updates")
	registerUpdateHandler(provisioningUpdateRouter, d.authorizer, updateSvc)

	systemRouter := api10router.SubGroup("/system")
	registerSystemHandler(systemRouter, d.authorizer, systemSvc)

	inventoryRouter := api10router.SubGroup("/inventory")

	inventorySyncers := registerInventoryRoutes(dbWithTransaction, clusterSvcWrapped, serverClientProvider, d.authorizer, inventoryRouter)

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
		Addr:        fmt.Sprintf("%s:%d", config.GetNetwork().RestServerAddress, config.GetNetwork().RestServerPort),
		ErrorLog:    errorLogger,
	}

	d.shutdownFuncs = append(d.shutdownFuncs, d.server.Shutdown)

	group, errgroupCtx := errgroup.WithContext(context.Background())
	d.errgroup = group

	d.errgroup.Go(func() error {
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

	err = d.networkConfigReload(ctx, config.GetNetwork())
	if err != nil {
		return err
	}

	config.NetworkUpdateSignal.AddListener(func(ctx context.Context, sn api.SystemNetwork) {
		err := d.networkConfigReload(ctx, sn)
		if err != nil {
			slog.ErrorContext(ctx, "Failed to reload network config", logger.Err(err))
		}
	})

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

func (d *Daemon) networkConfigReload(ctx context.Context, cfg api.SystemNetwork) error {
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

		newAddr := fmt.Sprintf("%s:%d", cfg.RestServerAddress, cfg.RestServerPort)
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
			if d.oidcVerifier == nil {
				d.oidcVerifier = &authnoidc.Verifier{}
			}

			*d.oidcVerifier = *newOIDCVerifier

			authers = append(authers, authnoidc.New(newOIDCVerifier))
		}
	}

	// Setup client cert fingerprint authentication.
	if len(cfg.TrustedTLSClientCertFingerprints) > 0 {
		authers = append(authers, authntls.New(cfg.TrustedTLSClientCertFingerprints))
	}

	// Create authenticator
	if d.authenticator == nil {
		d.authenticator = &authn.Authenticator{}
	}

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

	if d.authorizer == nil {
		var authorizer authz.Authorizer = authzchain.New()
		d.authorizer = &authorizer
	}

	*d.authorizer = authzchain.New(authorizers...)

	return errors.Join(errs...)
}

type httpErrorLogger struct{}

func (httpErrorLogger) Write(p []byte) (n int, err error) {
	slog.ErrorContext(context.Background(), string(p)) //nolint:sloglint // error message coming from the http server is the message.
	return len(p), nil
}

// deadlineFrom extracts the deadline from the provided context if present and not yet expired.
// Otherwise the defaultDeadline is returned.
func deadlineFrom(ctx context.Context, defaultDeadline time.Duration) time.Duration {
	deadline, ok := ctx.Deadline()
	if ok {
		deadlineDuration := time.Until(deadline)
		if deadlineDuration > 0 {
			return deadlineDuration
		}
	}

	return defaultDeadline
}
