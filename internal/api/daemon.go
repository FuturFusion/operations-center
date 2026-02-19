package api

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	incusosapi "github.com/lxc/incus-os/incus-osd/api"
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
	"github.com/FuturFusion/operations-center/internal/domain"
	internalenvironment "github.com/FuturFusion/operations-center/internal/environment"
	inventoryEntities "github.com/FuturFusion/operations-center/internal/inventory/repo/sqlite/entities"
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
	provisioningClusterArtifactRepo "github.com/FuturFusion/operations-center/internal/provisioning/repo/localartifact"
	localartifactEntities "github.com/FuturFusion/operations-center/internal/provisioning/repo/localartifact/entities"
	"github.com/FuturFusion/operations-center/internal/provisioning/repo/localfs"
	provisioningRepoMiddleware "github.com/FuturFusion/operations-center/internal/provisioning/repo/middleware"
	provisioningSqlite "github.com/FuturFusion/operations-center/internal/provisioning/repo/sqlite"
	provisioningEntities "github.com/FuturFusion/operations-center/internal/provisioning/repo/sqlite/entities"
	"github.com/FuturFusion/operations-center/internal/sql/dbschema"
	dbdriver "github.com/FuturFusion/operations-center/internal/sql/sqlite"
	"github.com/FuturFusion/operations-center/internal/sql/transaction"
	"github.com/FuturFusion/operations-center/internal/system"
	systemServiceMiddleware "github.com/FuturFusion/operations-center/internal/system/middleware"
	"github.com/FuturFusion/operations-center/internal/util/file"
	"github.com/FuturFusion/operations-center/internal/util/task"
	"github.com/FuturFusion/operations-center/internal/version"
	"github.com/FuturFusion/operations-center/shared/api"
)

type environment interface {
	GetUnixSocket() string
	VarDir() string
	CacheDir() string
	UsrShareDir() string
	IsIncusOS() bool
	GetToken(ctx context.Context) (string, error)
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

	systemSvc system.SystemService

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

		authenticator: &authn.Authenticator{},
		oidcVerifier:  &authnoidc.Verifier{},
		authorizer: func() *authz.Authorizer {
			var authorizer authz.Authorizer = authzchain.New()
			return &authorizer
		}(),
	}

	return d
}

func (d *Daemon) Start(ctx context.Context) error {
	slog.InfoContext(ctx, "Starting up", slog.String("version", version.Version))

	dbWithTransaction, err := d.initDB(ctx)
	if err != nil {
		return err
	}

	// Apply all patches that need to be run before daemon security infrastructure
	// like certificates, authenticators and authorizers are initialized.
	err = patchesApply(ctx, dbWithTransaction, patchPreSecurityInfrastructure)
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
		slog.ErrorContext(ctx, "Failed to load security config", logger.Err(err))
	}

	// On update of the security configuration, perform reload of the security
	// related infrastructure.
	config.SecurityUpdateSignal.AddListener(func(ctx context.Context, cfg api.SystemSecurity) {
		err := d.securityConfigReload(ctx, cfg)
		if err != nil {
			slog.ErrorContext(ctx, "Failed to reload security config", logger.Err(err))
		}
	})

	// On update of ACME configuration, perform renewal of the server certificate.
	config.SecurityACMEUpdateSignal.AddListener(func(ctx context.Context, ssa api.SystemSecurityACME) {
		slog.InfoContext(ctx, "Trigger async ACME renewal after config change")

		go func() {
			// Use detached context to decouple async call from original request.
			// For logging, we keep the original context, such that the original
			// request ID is logged.
			_, err := d.systemSvc.TriggerCertificateRenew(context.Background(), true)
			if err != nil {
				slog.ErrorContext(ctx, "Failed to renew ACME server certificate", logger.Err(err))
			}

			slog.InfoContext(ctx, "Async ACME renewal completed")
		}()
	})

	// Setup Services
	updateSvc, err := d.setupUpdatesService(ctx, dbWithTransaction)
	if err != nil {
		return err
	}

	channelSvc := d.setupChannelService(dbWithTransaction, updateSvc)

	tokenSvc := d.setupTokenService(dbWithTransaction, updateSvc, channelSvc)
	serverSvc := d.setupServerService(dbWithTransaction, tokenSvc, nil, channelSvc, updateSvc)
	clusterSvc, err := d.setupClusterService(dbWithTransaction, serverSvc, tokenSvc)
	if err != nil {
		return err
	}

	channelSvc.SetServerService(serverSvc)
	serverSvc.SetClusterService(clusterSvc)
	clusterTemplateSvc := d.setupClusterTemplateService(dbWithTransaction)

	d.systemSvc = d.setupSystemService(serverSvc)

	// Setup API routes
	serveMux, inventorySyncers := d.setupAPIRoutes(updateSvc, tokenSvc, serverSvc, clusterSvc, clusterTemplateSvc, channelSvc, dbWithTransaction)
	inventorySyncers[domain.ResourceTypeServer] = serverSvc

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

	// Start cluster lifecycle events monitor.
	err = clusterSvc.StartLifecycleEventsMonitor(ctx)
	if err != nil {
		return err
	}

	// Background tasks
	d.setupBackgroundTasks(ctx, updateSvc, serverSvc, clusterSvc)

	err = d.incusOSSelfRegister(ctx)
	if err != nil {
		return fmt.Errorf("IncusOS self registration: %w", err)
	}

	err = d.incusOSSelfPoll(ctx, serverSvc)
	if err != nil {
		slog.WarnContext(ctx, "IncusOS startup self poll", logger.Err(err))
	}

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

func (d *Daemon) initDB(ctx context.Context) (dbdriver.DBTX, error) {
	db, err := dbdriver.Open(d.env.VarDir())
	if err != nil {
		return nil, fmt.Errorf("Failed to open sqlite database: %w", err)
	}

	// TODO: should Ensure take the provided context? If not, document the reason.
	current, err := dbschema.Ensure(context.TODO(), db, d.env.VarDir())
	if err != nil {
		return nil, err
	}

	dbWithTransaction := transaction.Enable(db)
	provisioningEntities.PreparedStmts, err = provisioningEntities.PrepareStmts(dbWithTransaction, false)
	if err != nil {
		return nil, err
	}

	// If we start from scratch, mark all patches as applied.
	if current == 0 {
		for _, patchName := range patchesGetNames() {
			err := markPatchAsApplied(ctx, db, patchName)
			if err != nil {
				return nil, err
			}
		}
	}

	inventoryEntities.PreparedStmts, err = inventoryEntities.PrepareStmts(dbWithTransaction, false)
	if err != nil {
		return nil, err
	}

	localartifactEntities.PreparedStmts, err = localartifactEntities.PrepareStmts(dbWithTransaction, false)
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

	trustedFingerprints := make([]string, 0, len(cfg.TrustedTLSClientCertFingerprints)+1)

	// Always trust our own client certificate if present.
	// This required to self connect if Operations Center is self-registered.
	clientCertFingerprint, err := incusTLS.CertFingerprintStr(d.clientCertificate)
	if err == nil {
		trustedFingerprints = append(trustedFingerprints, clientCertFingerprint)
	}

	// Setup client cert fingerprint authentication.
	trustedFingerprints = append(trustedFingerprints, cfg.TrustedTLSClientCertFingerprints...)
	authers = append(authers, authntls.New(trustedFingerprints))

	// Create authenticator
	*d.authenticator = authn.New(authers)

	authorizers := []authz.Authorizer{
		unixsocket.New(),
		authztls.New(ctx, trustedFingerprints),
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

func (d *Daemon) setupUpdatesService(ctx context.Context, db dbdriver.DBTX) (provisioning.UpdateService, error) {
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
	}

	updateServer := updateserver.New(
		config.GetUpdates().Source,
		config.GetUpdates().SignatureVerificationRootCA,
		d.env,
	)
	listenerKey := uuid.New().String()
	config.UpdatesValidateSignal.AddListenerWithErr(func(ctx context.Context, su api.SystemUpdates) error {
		return updateServer.SourceConnectionTest(ctx, su.Source, su.SignatureVerificationRootCA)
	}, listenerKey)
	config.UpdatesUpdateSignal.AddListener(func(ctx context.Context, cfg api.SystemUpdates) {
		updateServer.UpdateConfig(ctx, cfg.Source, cfg.SignatureVerificationRootCA)
	}, listenerKey)
	runtime.AddCleanup(d, func(listenerKey string) {
		// config.UpdatesValidateSignal.RemoveListener(listenerKey)
		config.UpdatesUpdateSignal.RemoveListener(listenerKey)
	}, listenerKey)

	updateSvcBase := provisioning.NewUpdateService(
		provisioningRepoMiddleware.NewUpdateRepoWithSlog(
			provisioningSqlite.NewUpdate(db),
			provisioningRepoMiddleware.UpdateRepoWithSlogWithInformativeErrFunc(
				func(err error) bool {
					return errors.Is(err, domain.ErrNotFound)
				},
			),
		),
		provisioningRepoMiddleware.NewUpdateFilesRepoWithSlog(
			repoUpdateFiles,
		),
		provisioningAdapterMiddleware.NewUpdateSourcePortWithSlog(
			updateServer,
		),
		updateServiceOptions...,
	)

	err = updateSvcBase.Prune(ctx)
	if err != nil {
		slog.WarnContext(ctx, "Failed to prune pending updates", logger.Err(err))
	}

	return provisioningServiceMiddleware.NewUpdateServiceWithSlog(
		updateSvcBase,
	), nil
}

func (d *Daemon) setupTokenService(db dbdriver.DBTX, updateSvc provisioning.UpdateService, channelSvc provisioning.ChannelService) provisioning.TokenService {
	imageFlasher := flasher.New(
		config.GetNetwork().OperationsCenterAddress,
		d.serverCertificate,
	)
	// Image flasher needs to learn about updates to the server certificate.
	config.ServerCertificateUpdateSignal.AddListener(func(_ context.Context, cert tls.Certificate) {
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
			),
			updateSvc,
			channelSvc,
			imageFlasher,
		),
	)
}

func (d *Daemon) setupServerService(db dbdriver.DBTX, tokenSvc provisioning.TokenService, clusterSvc provisioning.ClusterService, channelSvc provisioning.ChannelService, updateSvc provisioning.UpdateService) provisioning.ServerService {
	serverSvc := provisioning.NewServerService(
		provisioningRepoMiddleware.NewServerRepoWithSlog(
			provisioningSqlite.NewServer(db),
		),
		provisioningAdapterMiddleware.NewServerClientPortWithSlog(
			provisioningIncusAdapter.New(
				d.clientCertificate,
				d.clientKey,
			),
			provisioningAdapterMiddleware.ServerClientPortWithSlogWithInformativeErrFunc(
				func(err error) bool {
					// ErrSelfUpdateNotification is used as cause when the context is
					// cancelled. This is an expected success path and therefore not
					// an error.
					return errors.Is(err, provisioning.ErrSelfUpdateNotification) || errors.Is(err, api.NotIncusOSError)
				},
			),
		),
		tokenSvc,
		clusterSvc,
		channelSvc,
		updateSvc,
		d.serverCertificate,
	)

	// Server service needs to learn about updates of the public Operations Center
	// address.
	config.NetworkUpdateSignal.AddListener(func(ctx context.Context, cfg api.SystemNetwork) {
		// Update operations center server record with updated network config.
		err := serverSvc.SelfRegisterOperationsCenter(ctx)
		if err != nil {
			slog.WarnContext(ctx, "failed to update server URL", logger.Err(err))
		}
	})

	// Server service needs to learn about updates of the server certificate.
	config.ServerCertificateUpdateSignal.AddListener(func(ctx context.Context, c tls.Certificate) {
		err := serverSvc.UpdateServerCertificate(ctx, c)
		if err != nil {
			slog.WarnContext(ctx, "failed to update server URL", logger.Err(err))
		}
	})

	return provisioningServiceMiddleware.NewServerServiceWithSlog(
		serverSvc,
	)
}

func (d *Daemon) setupClusterService(db dbdriver.DBTX, serverSvc provisioning.ServerService, tokenSvc provisioning.TokenService) (provisioning.ClusterService, error) {
	updateSignal := signals.NewSync[provisioning.ClusterUpdateMessage]()

	localClusterArtifactRepo, err := provisioningClusterArtifactRepo.New(db, filepath.Join(d.env.VarDir(), "artifacts"), updateSignal)
	if err != nil {
		return nil, err
	}

	tmpTerraformDir, err := os.MkdirTemp("", "operations-center-terraform-*")
	if err != nil {
		tmpTerraformDir = os.TempDir()
	}

	err = os.WriteFile(filepath.Join(tmpTerraformDir, "client.key"), []byte(d.clientKey), 0o600)
	if err != nil {
		return nil, err
	}

	err = os.WriteFile(filepath.Join(tmpTerraformDir, "client.crt"), []byte(d.clientCertificate), 0o600)
	if err != nil {
		return nil, err
	}

	terraformProvisioner, err := terraform.New(tmpTerraformDir)
	if err != nil {
		return nil, err
	}

	return provisioningServiceMiddleware.NewClusterServiceWithSlog(
		provisioning.NewClusterService(
			provisioningRepoMiddleware.NewClusterRepoWithSlog(
				provisioningSqlite.NewCluster(db),
			),
			provisioningRepoMiddleware.NewClusterArtifactRepoWithSlog(
				localClusterArtifactRepo,
			),
			provisioningAdapterMiddleware.NewClusterClientPortWithSlog(
				provisioningIncusAdapter.New(
					d.clientCertificate,
					d.clientKey,
				),
			),
			serverSvc,
			tokenSvc,
			nil,
			terraformProvisioner,
			provisioning.WithClusterServiceUpdateSignal(updateSignal),
		),
	), nil
}

func (d *Daemon) setupClusterTemplateService(db dbdriver.DBTX) provisioning.ClusterTemplateService {
	return provisioningServiceMiddleware.NewClusterTemplateServiceWithSlog(
		provisioning.NewClusterTemplateService(
			provisioningRepoMiddleware.NewClusterTemplateRepoWithSlog(
				provisioningSqlite.NewClusterTemplate(db),
			),
		),
	)
}

func (d *Daemon) setupChannelService(db dbdriver.DBTX, updateSvc provisioning.UpdateService) provisioning.ChannelService {
	return provisioningServiceMiddleware.NewChannelServiceWithSlog(
		provisioning.NewChannelService(
			provisioningRepoMiddleware.NewChannelRepoWithSlog(
				provisioningSqlite.NewChannel(db),
			),
			updateSvc,
		),
	)
}

func (d *Daemon) setupSystemService(serverSvc provisioning.ServerService) system.SystemService {
	return systemServiceMiddleware.NewSystemServiceWithSlog(
		system.NewSystemService(d.env, serverSvc),
	)
}

func (d *Daemon) setupAPIRoutes(
	updateSvc provisioning.UpdateService,
	tokenSvc provisioning.TokenService,
	serverSvc provisioning.ServerService,
	clusterSvc provisioning.ClusterService,
	clusterTemplateSvc provisioning.ClusterTemplateService,
	channelSvc provisioning.ChannelService,
	db dbdriver.DBTX,
) (*http.ServeMux, map[domain.ResourceType]provisioning.InventorySyncer) {
	// serverClientProvider is a provider of a client to access (Incus) servers
	// or clusters.
	serverClientProvider := serverMiddleware.NewServerClientWithSlog(
		inventoryIncusAdapter.New(
			d.clientCertificate,
			d.clientKey,
		),
		serverMiddleware.ServerClientWithSlogWithInformativeErrFunc(
			func(err error) bool {
				return errors.Is(err, domain.ErrNotFound)
			},
		),
	)

	serveMux := http.NewServeMux()
	// TODO: Move access log and request ID middlewares here
	router := newRouter(serveMux)

	registerUIHandlers(router, d.env.UsrShareDir())
	registerWellKnownHandler(router)

	const osRouterPrefix = "/os"
	osRouter := router.SubGroup(osRouterPrefix).AddMiddlewares(
		d.authenticator.Middleware(),
	)
	registerOSProxy(osRouter, osRouterPrefix, d.authorizer, d.env)

	if d.oidcVerifier != nil {
		registerOIDCHandlers(router, d.oidcVerifier)
	}

	isAuthenticationRequired := func(r *http.Request) bool {
		// POST /1.0/provisioning/servers is authenticated using a token.
		if r.Method == http.MethodPost && r.URL.Path == "/1.0/provisioning/servers" {
			return false
		}

		// PUT /1.0/provisioning/servers/:self is authenticated using the servers
		// certificate.
		if r.Method == http.MethodPut && r.URL.Path == "/1.0/provisioning/servers/:self" {
			return false
		}

		// GET /1.0/provisioning/updates no authentication required to get updates.
		if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/1.0/provisioning/updates") {
			return false
		}

		if r.Pattern == "GET /1.0/provisioning/tokens/{uuid}/seeds/{name}" {
			return false
		}

		return true
	}

	api10router := router.SubGroup("/1.0").AddMiddlewares(
		d.authenticator.Middleware(authn.WithIsAuthenticationRequired(isAuthenticationRequired)),
	)
	registerAPI10Handler(api10router)

	provisioningRouter := api10router.SubGroup("/provisioning")

	provisioningTokenRouter := provisioningRouter.SubGroup("/tokens")
	registerProvisioningTokenHandler(provisioningTokenRouter, d.authorizer, tokenSvc)

	provisioningClusterRouter := provisioningRouter.SubGroup("/clusters")
	registerProvisioningClusterHandler(provisioningClusterRouter, d.authorizer, clusterSvc, clusterTemplateSvc)

	provisioningClusterTemplateRouter := provisioningRouter.SubGroup("/cluster-templates")
	registerProvisioningClusterTemplateHandler(provisioningClusterTemplateRouter, d.authorizer, clusterTemplateSvc)

	provisioningServerRouter := provisioningRouter.SubGroup("/servers")
	registerProvisioningServerHandler(provisioningServerRouter, d.authorizer, serverSvc, d.clientCertificate)

	provisioningUpdateRouter := provisioningRouter.SubGroup("/updates")
	registerUpdateHandler(provisioningUpdateRouter, d.authorizer, updateSvc)

	provisioningChannelRouter := provisioningRouter.SubGroup("/channels")
	registerChannelsHandler(provisioningChannelRouter, d.authorizer, channelSvc)

	systemRouter := api10router.SubGroup("/system")
	registerSystemHandler(systemRouter, d.authorizer, d.systemSvc)

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
	if config.IsBackgroundTasksDisabled() {
		return
	}

	// Start background task to refresh updates from the sources.
	refreshUpdatesFromSourcesTask := func(ctx context.Context) {
		slog.InfoContext(ctx, "Refresh updates triggered")
		err := updateSvc.Refresh(ctx)
		if err != nil {
			slog.ErrorContext(ctx, "Refresh updates failed", logger.Err(err))
			return
		}

		slog.InfoContext(ctx, "Refresh updates completed")
	}

	var updateSourceOptions []task.EveryOption
	if config.SourcePollSkipFirst() {
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
			return
		}

		slog.InfoContext(ctx, "Polling for pending servers completed")
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
			return
		}

		slog.InfoContext(ctx, "Connectivity test for ready servers completed")
	}

	pollReadyServersTaskStop, _ := task.Start(ctx, pollReadyServersTask, task.Every(config.ConnectivityCheckInterval))
	d.shutdownFuncs = append(d.shutdownFuncs, func(ctx context.Context) error {
		return pollReadyServersTaskStop(deadlineFrom(ctx, 1*time.Second))
	})

	// Start background task to refresh inventory through polling.
	refreshInventoryTask := func(ctx context.Context) {
		slog.InfoContext(ctx, "Inventory update triggered")
		err := clusterSvc.ResyncInventory(ctx)
		if err != nil {
			slog.ErrorContext(ctx, "Inventory update failed", logger.Err(err))
			return
		}

		slog.InfoContext(ctx, "Inventory update completed")
	}

	refreshInventoryTaskStop, _ := task.Start(ctx, refreshInventoryTask, task.Every(config.InventoryUpdateInterval))
	d.shutdownFuncs = append(d.shutdownFuncs, func(ctx context.Context) error {
		return refreshInventoryTaskStop(deadlineFrom(ctx, 10*time.Second))
	})

	// Start background task to renew ACME server certificate.
	renewACMEServerCertificateTask := func(ctx context.Context) {
		slog.InfoContext(ctx, "ACME server certificate renewal triggered")
		changed, err := d.systemSvc.TriggerCertificateRenew(ctx, false)
		if err != nil {
			slog.ErrorContext(ctx, "ACME server certificate renewal task failed", logger.Err(err))
			return
		}

		if !changed {
			slog.InfoContext(ctx, "ACME server certificate renewal completed, no change")
		}

		slog.InfoContext(ctx, "ACME server certificate renewal completed")
	}

	renewACMEServerCertificateTaskStop, _ := task.Start(ctx, renewACMEServerCertificateTask, task.Every(config.ACMEServerCertificateRenewInterval))
	d.shutdownFuncs = append(d.shutdownFuncs, func(ctx context.Context) error {
		return renewACMEServerCertificateTaskStop(deadlineFrom(ctx, 10*time.Second))
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

		config.ServerCertificateUpdateSignal.RemoveListener("fancyListener")
		config.SecurityTrustedHTTPSProxiesUpdateSignal.RemoveListener("fancyListener")

		if cfg.RestServerAddress == "" {
			d.configReloadMu.Lock()
			d.listener = nil
			d.configReloadMu.Unlock()

			// Unblock the channel here, since we do not start a server.
			errCh <- nil

			return nil
		}

		d.configReloadMu.Lock()
		d.server.Addr = cfg.RestServerAddress
		d.configReloadMu.Unlock()

		slog.InfoContext(ctx, "Start https listener", slog.Any("addr", cfg.RestServerAddress))
		tcpListener, err := net.Listen("tcp", cfg.RestServerAddress)
		if err != nil {
			errCh <- err
			return err
		}

		d.configReloadMu.Lock()
		d.listener = listener.NewFancyTLSListener(tcpListener, d.serverCertificate)
		d.configReloadMu.Unlock()

		err = d.listener.TrustedProxy(config.GetSecurity().TrustedHTTPSProxies)
		if err != nil {
			slog.WarnContext(ctx, "Failed to set trusted HTTPS proxies during server startup", logger.Err(err))
		}

		config.ServerCertificateUpdateSignal.AddListener(func(_ context.Context, cert tls.Certificate) {
			d.configReloadMu.Lock()
			defer d.configReloadMu.Unlock()

			d.serverCertificate = cert
			d.listener.Config(cert)
		}, "fancyListener")

		config.SecurityTrustedHTTPSProxiesUpdateSignal.AddListener(func(_ context.Context, trustedHTTPSProxies []string) {
			d.configReloadMu.Lock()
			defer d.configReloadMu.Unlock()

			err = d.listener.TrustedProxy(trustedHTTPSProxies)
			if err != nil {
				slog.WarnContext(ctx, "Failed to set trusted HTTPS proxies", logger.Err(err))
			}
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

// incusOSSelfRegister changes the provider in IncusOS from images to it self
// (Operations Center). This will trigger IncusOS to register it self with this
// instance of Operations Center.
// For this, the communication goes through the unix socket.
func (d *Daemon) incusOSSelfRegister(ctx context.Context) error {
	if !d.env.IsIncusOS() {
		return nil
	}

	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _ string, _ string) (net.Conn, error) {
				raddr, err := net.ResolveUnixAddr("unix", internalenvironment.IncusOSSocket)
				if err != nil {
					return nil, err
				}

				return net.DialUnix("unix", nil, raddr)
			},

			DisableKeepAlives: true,
		},
	}

	// Special IncusOS provider configuration with provider "operations-center"
	// but without URL, token and certificate.
	// This will be recognized by IncusOS as the special case of it providing
	// Operations Center as application and will then trigger it to hit the
	// self-registration endpoint on the unix socket.
	provider := incusosapi.SystemProvider{
		Config: incusosapi.SystemProviderConfig{
			Name: "operations-center",
		},
	}

	data, err := json.Marshal(provider)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, "http://unix/1.0/system/provider", bytes.NewBuffer(data))
	if err != nil {
		return err
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	response := api.Response{}
	err = json.Unmarshal(body, &response)
	if err != nil {
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("Failed to fetch %s: %s: %s", resp.Request.URL.String(), resp.Status, string(body))
		}

		return err
	}

	if response.Type == api.ErrorResponse {
		return api.StatusErrorf(resp.StatusCode, "%v", response.Error)
	}

	return nil
}

func (d *Daemon) incusOSSelfPoll(ctx context.Context, serverSvc provisioning.ServerService) error {
	if !d.env.IsIncusOS() {
		return nil
	}

	slog.DebugContext(ctx, "Self poll server status on IncusOS to update own inventory record")

	serverSelf, err := serverSvc.GetByName(ctx, api.ServerNameOperationsCenter)
	if err != nil {
		return fmt.Errorf("Failed to get self server instance: %w", err)
	}

	err = serverSvc.PollServer(ctx, *serverSelf, true)
	if err != nil {
		return fmt.Errorf("Failed to self poll server instalce: %w", err)
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
