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
	"maps"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/cowsql/go-cowsql/cluster/db/transaction"
	cowsqlheartbeat "github.com/cowsql/go-cowsql/cluster/heartbeat"
	cowsqllogging "github.com/cowsql/go-cowsql/cluster/logging"
	cowsqlmembership "github.com/cowsql/go-cowsql/cluster/membership"
	cowsqlDriver "github.com/cowsql/go-cowsql/driver"
	"github.com/google/uuid"
	incusosapi "github.com/lxc/incus-os/incus-osd/api"
	incusScriptlet "github.com/lxc/incus/v7/shared/scriptlet"
	incustls "github.com/lxc/incus/v7/shared/tls"
	"golang.org/x/sync/errgroup"

	incusAdapter "github.com/FuturFusion/operations-center/internal/adapter/incus"
	"github.com/FuturFusion/operations-center/internal/api/listener"
	clusteringSvc "github.com/FuturFusion/operations-center/internal/cluster"
	"github.com/FuturFusion/operations-center/internal/cluster/memberfiles"
	clusterRepo "github.com/FuturFusion/operations-center/internal/cluster/repo/sqlite"
	clusterEntities "github.com/FuturFusion/operations-center/internal/cluster/repo/sqlite/entities"
	config "github.com/FuturFusion/operations-center/internal/config/daemon"
	"github.com/FuturFusion/operations-center/internal/domain"
	internalenvironment "github.com/FuturFusion/operations-center/internal/environment"
	"github.com/FuturFusion/operations-center/internal/image"
	"github.com/FuturFusion/operations-center/internal/image/adapter/imagefiletarget"
	imageAdapterMiddleware "github.com/FuturFusion/operations-center/internal/image/adapter/middleware"
	"github.com/FuturFusion/operations-center/internal/image/adapter/simplestreams"
	imageServiceMiddleware "github.com/FuturFusion/operations-center/internal/image/middleware"
	imageLocalfs "github.com/FuturFusion/operations-center/internal/image/repo/localfs"
	imageRepoMiddleware "github.com/FuturFusion/operations-center/internal/image/repo/middleware"
	imageSqlite "github.com/FuturFusion/operations-center/internal/image/repo/sqlite"
	imageEntities "github.com/FuturFusion/operations-center/internal/image/repo/sqlite/entities"
	"github.com/FuturFusion/operations-center/internal/inventory"
	inventoryServiceMiddleware "github.com/FuturFusion/operations-center/internal/inventory/middleware"
	inventoryRepoMiddleware "github.com/FuturFusion/operations-center/internal/inventory/repo/middleware"
	inventorySqlite "github.com/FuturFusion/operations-center/internal/inventory/repo/sqlite"
	inventoryEntities "github.com/FuturFusion/operations-center/internal/inventory/repo/sqlite/entities"
	serverMiddleware "github.com/FuturFusion/operations-center/internal/inventory/server/middleware"
	"github.com/FuturFusion/operations-center/internal/lifecycle"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/provisioning/adapter/bios"
	"github.com/FuturFusion/operations-center/internal/provisioning/adapter/bmc/redfish"
	"github.com/FuturFusion/operations-center/internal/provisioning/adapter/flasher"
	provisioningAdapterMiddleware "github.com/FuturFusion/operations-center/internal/provisioning/adapter/middleware"
	"github.com/FuturFusion/operations-center/internal/provisioning/adapter/scriptlet"
	"github.com/FuturFusion/operations-center/internal/provisioning/adapter/securebootcerts"
	"github.com/FuturFusion/operations-center/internal/provisioning/adapter/securebootmedia"
	"github.com/FuturFusion/operations-center/internal/provisioning/adapter/seedprogress"
	"github.com/FuturFusion/operations-center/internal/provisioning/adapter/terraform"
	"github.com/FuturFusion/operations-center/internal/provisioning/adapter/updatefiletarget"
	"github.com/FuturFusion/operations-center/internal/provisioning/adapter/updateserver"
	provisioningChannel "github.com/FuturFusion/operations-center/internal/provisioning/channel"
	provisioningCluster "github.com/FuturFusion/operations-center/internal/provisioning/cluster"
	provisioningClusterTemplate "github.com/FuturFusion/operations-center/internal/provisioning/cluster_template"
	provisioningServiceMiddleware "github.com/FuturFusion/operations-center/internal/provisioning/middleware"
	provisioningClusterArtifactRepo "github.com/FuturFusion/operations-center/internal/provisioning/repo/localartifact"
	localartifactEntities "github.com/FuturFusion/operations-center/internal/provisioning/repo/localartifact/entities"
	provisioningLocalfs "github.com/FuturFusion/operations-center/internal/provisioning/repo/localfs"
	provisioningRepoMiddleware "github.com/FuturFusion/operations-center/internal/provisioning/repo/middleware"
	provisioningSqlite "github.com/FuturFusion/operations-center/internal/provisioning/repo/sqlite"
	provisioningEntities "github.com/FuturFusion/operations-center/internal/provisioning/repo/sqlite/entities"
	provisioningServer "github.com/FuturFusion/operations-center/internal/provisioning/server"
	provisioningToken "github.com/FuturFusion/operations-center/internal/provisioning/token"
	provisioningUpdate "github.com/FuturFusion/operations-center/internal/provisioning/update"
	"github.com/FuturFusion/operations-center/internal/security/acme"
	"github.com/FuturFusion/operations-center/internal/security/authn"
	authnmiddleware "github.com/FuturFusion/operations-center/internal/security/authn/middleware"
	authnoidc "github.com/FuturFusion/operations-center/internal/security/authn/oidc"
	authntls "github.com/FuturFusion/operations-center/internal/security/authn/tls"
	authnunixsocket "github.com/FuturFusion/operations-center/internal/security/authn/unixsocket"
	"github.com/FuturFusion/operations-center/internal/security/authz"
	authzchain "github.com/FuturFusion/operations-center/internal/security/authz/chain"
	authzmiddleware "github.com/FuturFusion/operations-center/internal/security/authz/middleware"
	oidcAuthorizer "github.com/FuturFusion/operations-center/internal/security/authz/oidc"
	authzopenfga "github.com/FuturFusion/operations-center/internal/security/authz/openfga"
	authztls "github.com/FuturFusion/operations-center/internal/security/authz/tls"
	"github.com/FuturFusion/operations-center/internal/security/authz/unixsocket"
	securitytls "github.com/FuturFusion/operations-center/internal/security/tls"
	"github.com/FuturFusion/operations-center/internal/sql/cowsql"
	"github.com/FuturFusion/operations-center/internal/sql/dbschema"
	"github.com/FuturFusion/operations-center/internal/sql/patches"
	dbdriver "github.com/FuturFusion/operations-center/internal/sql/sqlite"
	internalTx "github.com/FuturFusion/operations-center/internal/sql/transaction"
	"github.com/FuturFusion/operations-center/internal/system"
	systemServiceMiddleware "github.com/FuturFusion/operations-center/internal/system/middleware"
	systemLocalfs "github.com/FuturFusion/operations-center/internal/system/repo/localfs"
	systemRepoMiddleware "github.com/FuturFusion/operations-center/internal/system/repo/middleware"
	"github.com/FuturFusion/operations-center/internal/util/certificate"
	"github.com/FuturFusion/operations-center/internal/util/cors"
	"github.com/FuturFusion/operations-center/internal/util/file"
	"github.com/FuturFusion/operations-center/internal/util/logger"
	"github.com/FuturFusion/operations-center/internal/util/metrics"
	"github.com/FuturFusion/operations-center/internal/util/ptr"
	"github.com/FuturFusion/operations-center/internal/util/response"
	"github.com/FuturFusion/operations-center/internal/util/task"
	"github.com/FuturFusion/operations-center/internal/version"
	"github.com/FuturFusion/operations-center/internal/warning"
	warningServiceMiddleware "github.com/FuturFusion/operations-center/internal/warning/middleware"
	warningRepoMiddleware "github.com/FuturFusion/operations-center/internal/warning/repo/middleware"
	warningSqlite "github.com/FuturFusion/operations-center/internal/warning/repo/sqlite"
	warningEntities "github.com/FuturFusion/operations-center/internal/warning/repo/sqlite/entities"
	"github.com/FuturFusion/operations-center/shared/api"
	apisystem "github.com/FuturFusion/operations-center/shared/api/system"
	"github.com/FuturFusion/operations-center/ui"
)

// seedImageCacheDir is the namespace below the cache dir holding the generated
// seed images.
const seedImageCacheDir = "seed-images"

type environment interface {
	GetUnixSocket() string
	VarDir() string
	CacheDir() string
	UsrShareDir() string
	IsIncusOS() bool
	GetToken(ctx context.Context) (string, error)
	GetSecureBootCertificates(ctx context.Context) (incusosapi.InternalSecureBootCertificates, error)
	GetSecureBootPlatformKeyUpdate(ctx context.Context) ([]byte, error)
	Clustered() bool
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

	// securityRetryCancel stops the background connection retries of the
	// security infrastructure from the previous configuration.
	securityRetryCancel context.CancelFunc

	server    *http.Server
	listeners *listener.ListenerSet

	globalRepo clusteringSvc.GlobalRaftRepo

	systemSvc     system.SystemService
	membershipSvc clusteringSvc.MembershipService

	// apiCertificate is the certificate presented to API clients.
	apiCertificate tls.Certificate

	shutdownFuncs []func(context.Context) error
	errgroup      *errgroup.Group

	restartCh chan struct{}
}

type dbtxWrapper struct {
	transaction.DBTX
}

func (d *dbtxWrapper) Register(dbtx transaction.DBTX) {
	d.DBTX = dbtx
}

func NewDaemon(ctx context.Context, env environment) *Daemon {
	registerComponentsValidation()

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
		restartCh:         make(chan struct{}, 1),
		listeners:         &listener.ListenerSet{},

		oidcVerifier: &authnoidc.Verifier{},
		authorizer: func() *authz.Authorizer {
			var authorizer authz.Authorizer = authzmiddleware.NewAuthorizerWithSlog(
				authzchain.New(),
				authzmiddleware.AuthorizerWithSlogWithComponent(componentAuthzChain),
			)
			return &authorizer
		}(),
	}

	// Until the security configuration is loaded, only the unix socket and the cluster members are trusted.
	startingAuthenticator := authn.New([]authn.Auther{
		authnmiddleware.NewAutherWithSlog(
			authnunixsocket.UnixSocket{},
			authnmiddleware.AutherWithSlogWithComponent(componentAuthnUnixSocket),
		),
		authnmiddleware.NewAutherWithSlog(
			authntls.New(d.clusterMemberFingerprints),
			authnmiddleware.AutherWithSlogWithComponent(componentAuthnTLS),
		),
	}, authn.WithStarting())
	d.authenticator = &startingAuthenticator

	return d
}

func (d *Daemon) Start(ctx context.Context) error {
	ctx = logger.ContextWithComponent(ctx, componentDaemon)

	slog.InfoContext(ctx, "Starting up", slog.String("version", version.Version))

	localDB, err := d.initDB(ctx)
	if err != nil {
		return err
	}

	err = d.initAndLoadServerCert()
	if err != nil {
		return err
	}

	// On update of the OIDC or OpenFGA configuration, verify the configured
	// services are reachable, so a broken configuration update is rejected
	// right away. Connection failures after a successful update are handled
	// by background retries.
	lifecycle.SecurityValidateSignal.AddListenerWithErr(func(ctx context.Context, cfg apisystem.Security) error {
		if cfg.OIDC.Issuer != "" && cfg.OIDC.ClientID != "" {
			err := authnoidc.CheckConnectivity(ctx, cfg.OIDC.Issuer)
			if err != nil {
				// The cause can contain the response of the remote service. Only log it.
				return domain.NewErrorf(domain.ErrInvalidArgument, "", "Failed to reach OIDC issuer %q", cfg.OIDC.Issuer).WithCause(err)
			}
		}

		if cfg.OpenFGA.APIURL != "" && cfg.OpenFGA.APIToken != "" && cfg.OpenFGA.StoreID != "" {
			err := authzopenfga.CheckConnectivity(ctx, cfg.OpenFGA.APIURL, cfg.OpenFGA.APIToken, cfg.OpenFGA.StoreID)
			if err != nil {
				return domain.NewErrorf(domain.ErrInvalidArgument, "", "Failed to reach OpenFGA at %q", cfg.OpenFGA.APIURL).WithCause(err)
			}
		}

		return nil
	})

	// On update of the security configuration, perform reload of the security
	// related infrastructure.
	lifecycle.SecurityUpdateSignal.AddListener(func(ctx context.Context, cfg apisystem.Security) {
		err := d.securityConfigReload(ctx, cfg)
		if err != nil {
			slog.ErrorContext(ctx, "Failed to reload security config", logger.Err(err))
		}
	})

	// On update of ACME configuration, perform renewal of the server certificate.
	lifecycle.SecurityACMEUpdateSignal.AddListener(func(ctx context.Context, ssa apisystem.SecurityACME) {
		// Only the leader renews and distributes the shared certificate.
		isLeader, err := d.membershipSvc.Gateway().IsLeader(ctx)
		if err != nil {
			slog.ErrorContext(ctx, "Failed to check for cluster leadership before ACME renewal", logger.Err(err))
			return
		}

		if !isLeader {
			return
		}

		slog.InfoContext(ctx, "Trigger async ACME renewal after config change")

		go func() {
			// Use detached context to decouple async call from original request.
			ctx := logger.DetachedContext(ctx)
			_, err := d.systemSvc.TriggerCertificateRenew(ctx, true)
			if err != nil {
				slog.ErrorContext(ctx, "Failed to renew ACME server certificate", logger.Err(err))
				return
			}

			slog.InfoContext(ctx, "Async ACME renewal completed")
		}()
	})

	client := incusAdapter.New(
		d.clientCertificate,
		d.clientKey,
		incusAdapter.WithEnvironment(d.env),
		incusAdapter.WithSkipGetServer(true),
	)

	loader := incusScriptlet.NewLoader()
	runner, err := scriptlet.New(
		loader,
		provisioningAdapterMiddleware.NewScriptletClientPortWithSlog(
			client,
		),
	)
	if err != nil {
		return err
	}

	// API starts before DB in restricted mode.
	dbWithTransaction := &dbtxWrapper{}

	// Setup Services
	incusImageSvc, err := d.setupIncusImageService(dbWithTransaction)
	if err != nil {
		return err
	}

	imageSourceSvc := d.setupImageSourceService(
		dbWithTransaction,
		incusImageSvc,
	)

	warningSvc := d.setupWarningService(dbWithTransaction)
	// Temporary use log only warn service.
	// warningLogEmitter := provisioning.LogWarningService{}
	warningLogEmitter := warningSvc

	inventoryInventoryAggregateSvc := inventoryServiceMiddleware.NewInventoryAggregateServiceWithSlog(
		inventory.NewInventoryAggregateService(
			inventoryRepoMiddleware.NewInventoryAggregateRepoWithSlog(
				inventorySqlite.NewInventoryAggregate(dbWithTransaction),
			),
		),
	)

	updateSvc, err := d.setupUpdatesService(dbWithTransaction)
	if err != nil {
		return err
	}

	channelSvc := d.setupChannelService(dbWithTransaction, updateSvc)

	tokenSvc, imageFlasher := d.setupTokenService(dbWithTransaction, client, updateSvc, channelSvc)

	seedImageProgress := provisioningAdapterMiddleware.NewSeedImageProgressPortWithSlog(
		seedprogress.New(),
	)

	biosProfileCatalog, err := bios.New()
	if err != nil {
		return fmt.Errorf("Failed to load the BIOS profile catalog: %w", err)
	}

	biosProfile := provisioningAdapterMiddleware.NewBIOSProfilePortWithSlog(
		biosProfileCatalog,
	)

	secureBootCatalog, err := securebootcerts.New()
	if err != nil {
		return fmt.Errorf("Failed to load the secure boot certificate catalog: %w", err)
	}

	secureBootMedia := provisioningAdapterMiddleware.NewSecureBootMediaPortWithSlog(
		securebootmedia.New(filepath.Join(d.env.CacheDir(), "secure-boot-media")),
	)

	serverSvc := d.setupServerService(dbWithTransaction, client, runner, tokenSvc, nil, channelSvc, updateSvc, warningLogEmitter, biosProfile, seedImageProgress, secureBootMedia, secureBootCatalog)
	clusterArtifactRepo, err := provisioningClusterArtifactRepo.New(
		dbWithTransaction,
		filepath.Join(d.env.VarDir(), "artifacts"),
		memberfiles.NewTargets(func() memberfiles.Cluster { return d.membershipSvc }),
	)
	if err != nil {
		return err
	}

	clusterSvc, err := d.setupClusterService(dbWithTransaction, client, serverSvc, tokenSvc, inventoryInventoryAggregateSvc, warningLogEmitter, clusterArtifactRepo)
	if err != nil {
		return err
	}

	updateSvc.SetServerService(serverSvc)
	channelSvc.SetServerService(serverSvc)
	tokenSvc.SetServerService(serverSvc)
	serverSvc.SetClusterService(clusterSvc)
	clusterTemplateSvc := d.setupClusterTemplateService(dbWithTransaction)

	d.systemSvc = d.setupSystemService(serverSvc)

	// Background tasks
	// Monitor lifecycle events before the other tasks, e.g. the initial inventory refresh.
	tasks := append([]task.TaskFunc{lifecycleEventsMonitorTask(clusterSvc)}, d.backgroundTasks(updateSvc, imageSourceSvc, serverSvc, clusterSvc, imageFlasher, secureBootMedia, warningLogEmitter)...)

	// Setup API routes
	membershipSvc, isClustered, err := d.setupClusterMembershipService(ctx, localDB, tasks)
	if err != nil {
		return err
	}

	d.membershipSvc = membershipSvc

	serveMux, inventorySyncers := d.setupAPIRoutes(
		ctx,
		updateSvc,
		tokenSvc,
		serverSvc,
		clusterSvc,
		clusterTemplateSvc,
		channelSvc,
		warningSvc,
		inventoryInventoryAggregateSvc,
		imageSourceSvc,
		incusImageSvc,
		seedImageProgress,
		secureBootMedia,
		membershipSvc,
		memberfiles.NewMover(
			func() memberfiles.Cluster { return d.membershipSvc },
			provisioningSqlite.NewUpdate(dbWithTransaction),
			imageSqlite.NewIncusImage(dbWithTransaction),
			clusterArtifactRepo,
		),
		dbWithTransaction,
	)
	inventorySyncers[domain.ResourceTypeServer] = serverSvc

	clusterSvc.SetInventorySyncers(inventorySyncers)

	// Setup API server
	errorLogger := &log.Logger{}
	errorLogger.SetOutput(httpErrorLogger{})

	d.server = &http.Server{
		Handler: componentMiddleware(
			logger.RequestIDMiddleware(
				logger.AccessLogMiddleware(
					serveMux,
				),
			),
		),
		IdleTimeout: 30 * time.Second,
		ErrorLog:    errorLogger,
	}

	d.shutdownFuncs = append(d.shutdownFuncs, d.server.Shutdown)

	group, errgroupCtx := errgroup.WithContext(context.Background())
	d.errgroup = group

	// If the network configuration changes, we need to reload the API server on TCP.
	lifecycle.NetworkUpdateSignal.AddListener(func(ctx context.Context, sn apisystem.Network) {
		err := d.setupTCPListener(ctx, sn)
		if err != nil {
			slog.ErrorContext(ctx, "Failed to reload network config", logger.Err(err))
		}
	})

	// API server on unix socket
	err = d.setupSocketListener(ctx)
	if err != nil {
		return err
	}

	// API server on TCP
	err = d.setupTCPListener(ctx, config.GetLocalNetwork())
	if err != nil {
		return err
	}

	dbWithTransaction.DBTX, err = d.initGlobalDB(ctx, isClustered, membershipSvc, config.GetLocalNetwork().ClusterAddress)
	if err != nil {
		return err
	}

	// Load the shared security configuration from the database.
	err = d.securityConfigReload(ctx, config.GetSecurity())
	if err != nil {
		slog.ErrorContext(ctx, "Failed to load security config", logger.Err(err))
	}

	err = clusterArtifactRepo.MigrateNameLayout(ctx)
	if err != nil {
		return fmt.Errorf("Failed to migrate cluster artifacts: %w", err)
	}

	// File records deleted on other members while this one was offline should be removed.
	err = d.pruneLocalFiles(ctx, updateSvc, incusImageSvc, clusterArtifactRepo.Prune)
	if err != nil {
		slog.WarnContext(ctx, "Failed to prune stale local files", logger.Err(err))
	}

	membershipSvc.StartLeaderTasks()

	timeoutCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	err = d.incusOSSelfRegister(timeoutCtx)
	cancel()
	if err != nil {
		return fmt.Errorf("IncusOS self registration: %w", err)
	}

	timeoutCtx, cancel = context.WithTimeout(ctx, 3*time.Second)
	err = d.incusOSSelfPoll(timeoutCtx, serverSvc)
	cancel()
	if err != nil {
		slog.WarnContext(ctx, "IncusOS startup self poll", logger.Err(err))
	}

	clusterTaskStop := clusterHeartbeatTask(ctx, membershipSvc)
	d.shutdownFuncs = append(d.shutdownFuncs, func(ctx context.Context) error {
		return clusterTaskStop(deadlineFrom(ctx, time.Second*3))
	})

	// Stop the membership after the heartbeats, which need the database.
	d.shutdownFuncs = append(d.shutdownFuncs, func(ctx context.Context) error {
		return membershipSvc.Stop(ctx, true)
	})

	// Finalize daemon start
	// Wait for immediate errors during startup.
	select {
	case <-errgroupCtx.Done():
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer shutdownCancel()
		return d.Stop(shutdownCtx)

	case <-time.After(50 * time.Millisecond):
		// Grace period we wait for potential immediate errors from serving the http server.
	}

	return nil
}

func prepare(db clusteringSvc.GlobalRaftRepo) (transaction.DBTX, error) {
	internalTx.Register(db)
	dbWithTransaction, ok := db.DBTX().(transaction.Preparer)
	if !ok {
		return nil, fmt.Errorf("Unexpected transaction container %T", db.DBTX())
	}

	var err error

	clusterEntities.PreparedStmts, err = clusterEntities.PrepareStmts(dbWithTransaction, false)
	if err != nil {
		return nil, err
	}

	imageEntities.PreparedStmts, err = imageEntities.PrepareStmts(dbWithTransaction, false)
	if err != nil {
		return nil, err
	}

	provisioningEntities.PreparedStmts, err = provisioningEntities.PrepareStmts(dbWithTransaction, false)
	if err != nil {
		return nil, err
	}

	inventoryEntities.PreparedStmts, err = inventoryEntities.PrepareStmts(dbWithTransaction, false)
	if err != nil {
		return nil, err
	}

	localartifactEntities.PreparedStmts, err = localartifactEntities.PrepareStmts(dbWithTransaction, false)
	if err != nil {
		return nil, err
	}

	warningEntities.PreparedStmts, err = warningEntities.PrepareStmts(dbWithTransaction, false)
	if err != nil {
		return nil, err
	}

	return dbWithTransaction, nil
}

func (d *Daemon) setupClusterMembershipService(ctx context.Context, localDB *clusterRepo.LocalDB, tasks []task.TaskFunc) (clusteringSvc.MembershipService, bool, error) {
	localRepo := clusterRepo.NewRaftRepo(localDB, filepath.Join(d.env.VarDir(), "database"), d.systemSvc)
	isClustered, err := cowsqlmembership.Enabled(localRepo)
	if err != nil {
		return nil, false, err
	}

	localCert, err := incustls.KeyPairAndCA(d.env.VarDir(), "server", incustls.CertServer, true)
	if err != nil {
		return nil, false, err
	}

	// Until clustered, the server certificate is used as the cluster certificate.
	clusterCert := localCert
	if isClustered {
		clusterCert, err = incustls.KeyPairAndCA(d.env.VarDir(), "cluster", incustls.CertServer, true)
		if err != nil {
			return nil, false, err
		}
	}

	svc, err := clusteringSvc.NewClusterMembershipService(ctx, d.env, d.systemSvc, d.listeners, localRepo, clusterCert, localCert, tasks)
	if err != nil {
		return nil, false, err
	}

	return svc, isClustered, nil
}

func (d *Daemon) initGlobalDB(ctx context.Context, isClustered bool, membership clusteringSvc.MembershipService, clusterAddress string) (dbdriver.DBTX, error) {
	globalRepo, err := openCowsqlGlobalDB(ctx, isClustered, membership, clusterAddress)
	if err != nil {
		return nil, err
	}

	d.globalRepo = globalRepo

	dbtx, err := prepare(globalRepo)
	if err != nil {
		return nil, err
	}

	slog.InfoContext(ctx, "Cluster database startup completed")

	configRepo := clusterRepo.NewConfigRepo(globalRepo)
	err = membership.Enable(ctx, clusterAddress, clusterRepo.NewTokenRepo(globalRepo), configRepo, globalRepo)
	if err != nil {
		return nil, err
	}

	sharedCfg, err := configRepo.GetSharedConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("Failed to read shared config: %w", err)
	}

	err = config.LoadShared(ctx, *sharedCfg, membership.PersistSharedConfig)
	if err != nil {
		return nil, err
	}

	// Apply all patches that need to be run before daemon security infrastructure
	// like certificates, authenticators and authorizers are initialized.
	err = patches.Apply(ctx, dbtx, patches.PreSecurityInfrastructure)
	if err != nil {
		return nil, err
	}

	return dbtx, nil
}

// openCowsqlGlobalDB opens the global db with cowsql backend.
func openCowsqlGlobalDB(ctx context.Context, isClustered bool, membership clusteringSvc.MembershipService, clusterAddress string) (clusteringSvc.GlobalRaftRepo, error) {
	contextTimeout := 30 * time.Second
	if !isClustered {
		// This is a workaround for #5234. We set a very
		// high timeout when we're not clustered, since there's
		// actually no networking involved.
		contextTimeout = time.Minute
	}

	options := []cowsqlDriver.Option{
		cowsqlDriver.WithDialFunc(membership.Gateway().DialFunc()),
		cowsqlDriver.WithConnectionTimeout(10 * time.Second), //nolint:staticcheck
		cowsqlDriver.WithLogFunc(cowsqllogging.CowsqlLog),
		cowsqlDriver.WithContextTimeout(contextTimeout), //nolint:staticcheck
	}

	var globalRepo clusteringSvc.GlobalRaftRepo
	for {
		dir := filepath.Dir(membership.Gateway().Node().GlobalDatabaseDir())
		globalDB, err := cowsql.Open(ctx, "db.bin", membership.Gateway().NodeStore(), clusterAddress, dir, 36*time.Hour, options...)
		if err != nil && !errors.Is(err, cowsql.ErrSomeNodesAreBehind) {
			return nil, err
		}

		globalRepo = clusterRepo.NewGlobalRaftRepo(globalDB)
		if err != nil {
			membership.SetGlobalDB(globalRepo)

			clusterTaskStop := clusterHeartbeatTask(ctx, membership)
			membership.Gateway().WaitUpgradeNotification()
			_ = clusterTaskStop(time.Second)

			membership.SetGlobalDB(nil)
			_ = globalDB.Close()

			continue
		}

		break
	}

	return globalRepo, nil
}

func (d *Daemon) initDB(ctx context.Context) (*clusterRepo.LocalDB, error) {
	dir := filepath.Join(d.env.VarDir(), "database")
	err := os.MkdirAll(dir, 0o700)
	if err != nil {
		return nil, fmt.Errorf("Failed to create database directory: %w", err)
	}

	db, err := dbdriver.Open(dir)
	if err != nil {
		return nil, fmt.Errorf("Failed to open sqlite database: %w", err)
	}

	_, err = dbschema.EnsureLocal(ctx, db.DB, dir, dbschema.LocalUpdates(), dbschema.LocalFreshSchema())
	if err != nil {
		return nil, err
	}

	return &clusterRepo.LocalDB{DB: db.DB}, nil
}

func (d *Daemon) initAndLoadServerCert() error {
	certFile := filepath.Join(d.env.VarDir(), config.ServerCertificateFilename)
	keyFile := filepath.Join(d.env.VarDir(), config.ServerKeyFilename)

	// Ensure that the certificate exists, or create a new one if it does not.
	err := incustls.FindOrGenCert(certFile, keyFile, false, true)
	if err != nil {
		return err
	}

	// When clustered, the server.crt is only used for member trust.
	if d.env.Clustered() {
		certFile = filepath.Join(d.env.VarDir(), "cluster.crt")
		keyFile = filepath.Join(d.env.VarDir(), "cluster.key")
	}

	certificatePEM, err := os.ReadFile(certFile)
	if err != nil {
		return fmt.Errorf("Failed to read certificate from %q: %w", certFile, err)
	}

	keyPEM, err := os.ReadFile(keyFile)
	if err != nil {
		return fmt.Errorf("Failed to read key from %q: %w", keyFile, err)
	}

	d.apiCertificate, err = tls.X509KeyPair(certificatePEM, keyPEM)
	if err != nil {
		return fmt.Errorf("Failed to validate certificate key pair: %w", err)
	}

	return nil
}

// clusterMemberFingerprints returns the fingerprints of the certificates of the cluster members.
func (d *Daemon) clusterMemberFingerprints() []string {
	if d.membershipSvc == nil {
		return []string{}
	}

	return slices.Collect(maps.Keys(d.membershipSvc.TrustedClients()))
}

func (d *Daemon) securityConfigReload(ctx context.Context, cfg apisystem.Security) error {
	d.configReloadMu.Lock()
	defer d.configReloadMu.Unlock()

	// Cancel background connection retries from the previous configuration.
	if d.securityRetryCancel != nil {
		d.securityRetryCancel()
	}

	// Detached context for background connection retries, which needs to
	// outlive ctx, e.g. if the reload is triggered by an API request.
	retryCtx, retryCancel := context.WithCancel(logger.DetachedContext(ctx))
	d.securityRetryCancel = retryCancel

	var errs []error

	// UnixSocket authenticator is always available.
	authers := []authn.Auther{
		authnmiddleware.NewAutherWithSlog(
			authnunixsocket.UnixSocket{},
			authnmiddleware.AutherWithSlogWithComponent(componentAuthnUnixSocket),
		),
	}

	// Setup OIDC authentication.
	if cfg.OIDC.Issuer != "" && cfg.OIDC.ClientID != "" {
		var err error
		newOIDCVerifier, err := authnoidc.NewVerifier(retryCtx, cfg.OIDC.Issuer, cfg.OIDC.ClientID, cfg.OIDC.Scope, cfg.OIDC.Audience, cfg.OIDC.Claim)
		if err != nil {
			errs = append(errs, err)
		} else {
			*d.oidcVerifier = *newOIDCVerifier

			authers = append(authers,
				authnmiddleware.NewAutherWithSlog(
					authnoidc.New(newOIDCVerifier),
					authnmiddleware.AutherWithSlogWithComponent(authnoidc.Component),
				),
			)
		}
	}

	trustedFingerprints := make([]string, 0, len(cfg.TrustedTLSClientCertFingerprints)+1)

	// Always trust our own client certificate if present.
	// This required to self connect if Operations Center is self-registered.
	clientCertFingerprint, err := incustls.CertFingerprintStr(d.clientCertificate)
	if err == nil {
		trustedFingerprints = append(trustedFingerprints, clientCertFingerprint)
	}

	// Setup client cert fingerprint authentication.
	trustedFingerprints = append(trustedFingerprints, cfg.TrustedTLSClientCertFingerprints...)

	// The trusted client certificates grant access just like a bare fingerprint does.
	// The fingerprints of the valid certificates are used even if others fail to
	// parse, so a single invalid certificate does not lock everybody out.
	certificateFingerprints, err := securitytls.CertificateFingerprints(cfg.TrustedTLSClientCertificates)
	if err != nil {
		errs = append(errs, fmt.Errorf("Failed to derive fingerprints from the trusted client certificates: %w", err))
	}

	trustedFingerprints = append(trustedFingerprints, certificateFingerprints...)

	configTLS := func() []string { return trustedFingerprints }

	authers = append(authers,
		authnmiddleware.NewAutherWithSlog(
			authntls.New(configTLS, d.clusterMemberFingerprints),
			authnmiddleware.AutherWithSlogWithComponent(componentAuthnTLS),
		),
	)

	*d.authenticator = authn.New(authers, authn.WithTrustedForward(d.isClusterMemberRequest))

	authorizers := []authz.Authorizer{
		authzmiddleware.NewAuthorizerWithSlog(
			unixsocket.New(),
			authzmiddleware.AuthorizerWithSlogWithComponent(componentAuthzUnixSocket),
		),
		authzmiddleware.NewAuthorizerWithSlog(
			authztls.New(ctx, configTLS, d.clusterMemberFingerprints),
			authzmiddleware.AuthorizerWithSlogWithComponent(componentAuthzTLS),
		),
	}

	if cfg.OpenFGA.APIURL != "" && cfg.OpenFGA.APIToken != "" && cfg.OpenFGA.StoreID != "" {
		openfgaAuthorizer, err := authzopenfga.New(retryCtx, cfg.OpenFGA.APIURL, cfg.OpenFGA.APIToken, cfg.OpenFGA.StoreID)
		if err != nil {
			errs = append(errs, err)
		} else {
			authorizers = append(authorizers,
				authzmiddleware.NewAuthorizerWithSlog(
					openfgaAuthorizer,
					authzmiddleware.AuthorizerWithSlogWithComponent(authzopenfga.Component),
				),
			)
		}
	}

	// If OIDC is configured and OpenFGA is explicitly not configured, grant
	// unrestricted access to all authenticated OIDC users.
	if cfg.OIDC.Issuer != "" && cfg.OIDC.ClientID != "" && cfg.OpenFGA.APIURL == "" && cfg.OpenFGA.APIToken == "" && cfg.OpenFGA.StoreID == "" {
		authorizers = append(authorizers,
			authzmiddleware.NewAuthorizerWithSlog(
				oidcAuthorizer.New(),
				authzmiddleware.AuthorizerWithSlogWithComponent(componentAuthzOIDC),
			),
		)
	}

	*d.authorizer = authzmiddleware.NewAuthorizerWithSlog(
		authzchain.New(authorizers...),
		authzmiddleware.AuthorizerWithSlogWithComponent(componentAuthzChain),
	)

	return errors.Join(errs...)
}

func (d *Daemon) setupIncusImageService(db dbdriver.DBTX) (image.ImageIncusService, error) {
	imageFilesRepo, err := imageLocalfs.New(filepath.Join(d.env.VarDir(), "images"))
	if err != nil {
		return nil, err
	}

	imageIncusSvc := imageServiceMiddleware.NewImageIncusServiceWithSlog(
		image.NewIncusImage(
			imageRepoMiddleware.NewImageIncusRepoWithSlog(
				imageSqlite.NewIncusImage(db),
			),
			imageRepoMiddleware.NewImageIncusFileRepoWithSlog(
				imageFilesRepo,
			),
			simplestreams.New(),
			image.WithFileTargets(
				imageAdapterMiddleware.NewImageFileTargetPortWithSlog(
					imagefiletarget.New(memberfiles.NewTargets(func() memberfiles.Cluster { return d.membershipSvc })),
				),
			),
		),
	)

	return imageIncusSvc, nil
}

func (d *Daemon) setupImageSourceService(db dbdriver.DBTX, incusImageSourcer image.IncusImageSourcePort) image.IncusImageSourceService {
	imageSourceSvc := imageServiceMiddleware.NewIncusImageSourceServiceWithSlog(
		image.NewIncusSource(
			imageRepoMiddleware.NewIncusImageSourceRepoWithSlog(
				imageSqlite.NewImageSource(db),
			),
			incusImageSourcer,
		),
	)

	return imageSourceSvc
}

func (d *Daemon) setupWarningService(db dbdriver.DBTX) warning.WarningService {
	warningSvc := warningServiceMiddleware.NewWarningServiceWithSlog(
		warning.NewWarningService(
			warningRepoMiddleware.NewWarningRepoWithSlog(
				warningSqlite.NewWarning(db),
			),
		),
	)

	return warningSvc
}

func (d *Daemon) setupUpdatesService(db dbdriver.DBTX) (provisioning.UpdateService, error) {
	repoUpdateFiles, err := provisioningLocalfs.New(
		filepath.Join(d.env.VarDir(), "updates"),
		config.GetUpdates().SignatureVerificationRootCA,
	)
	if err != nil {
		return nil, err
	}

	// Make sure, the files repository learns about changes to the signature certificate.
	lifecycle.UpdatesUpdateSignal.AddListener(func(ctx context.Context, cfg apisystem.Updates) {
		repoUpdateFiles.UpdateConfig(ctx, cfg.SignatureVerificationRootCA)
	})

	updateServiceOptions := []provisioningUpdate.Option{
		provisioningUpdate.WithLatestLimit(3),
		provisioningUpdate.WithFileTargets(
			provisioningAdapterMiddleware.NewUpdateFileTargetPortWithSlog(
				updatefiletarget.New(memberfiles.NewTargets(func() memberfiles.Cluster { return d.membershipSvc })),
			),
		),
	}

	updateServer := updateserver.New(
		config.GetUpdates().Source,
		config.GetUpdates().SignatureVerificationRootCA,
		config.GetUpdates().ImageServerAuthenticationByQueryParam,
		d.env,
	)
	listenerKey := uuid.New().String()
	lifecycle.UpdatesValidateSignal.AddListenerWithErr(func(ctx context.Context, su apisystem.Updates) error {
		return updateServer.SourceConnectionTest(ctx, su.Source, su.SignatureVerificationRootCA, su.ImageServerAuthenticationByQueryParam)
	}, listenerKey)
	lifecycle.UpdatesUpdateSignal.AddListener(func(ctx context.Context, cfg apisystem.Updates) {
		updateServer.UpdateConfig(ctx, cfg.Source, cfg.SignatureVerificationRootCA, cfg.ImageServerAuthenticationByQueryParam)
	}, listenerKey)
	runtime.AddCleanup(d, func(listenerKey string) {
		// config.UpdatesValidateSignal.RemoveListener(listenerKey)
		lifecycle.UpdatesUpdateSignal.RemoveListener(listenerKey)
	}, listenerKey)

	updateSvcBase := provisioningUpdate.New(
		provisioningRepoMiddleware.NewUpdateRepoWithSlog(
			provisioningSqlite.NewUpdate(db),
		),
		provisioningRepoMiddleware.NewUpdateFilesRepoWithSlog(
			repoUpdateFiles,
		),
		provisioningAdapterMiddleware.NewUpdateSourcePortWithSlog(
			updateServer,
		),
		nil,
		updateServiceOptions...,
	)

	return provisioningServiceMiddleware.NewUpdateServiceWithSlog(
		updateSvcBase,
	), nil
}

// pruneLocalFiles removes the local files of updates, images and cluster artifacts not assigned to this member.
func (d *Daemon) pruneLocalFiles(ctx context.Context, updateSvc provisioning.UpdateService, imageSvc image.ImageIncusService, pruneClusterArtifacts func(ctx context.Context) error) error {
	localMemberID, err := d.membershipSvc.LocalMemberID()
	if err != nil {
		return err
	}

	updates, err := updateSvc.GetAll(ctx)
	if err != nil {
		return err
	}

	knownUpdateFiles := map[string]bool{}
	for _, update := range updates {
		if update.MemberID != localMemberID {
			continue
		}

		for _, updateFile := range update.Files {
			knownUpdateFiles[path.Join(update.UUID.String(), updateFile.Filename)] = true
		}
	}

	images, err := imageSvc.GetAll(ctx)
	if err != nil {
		return err
	}

	knownImageFiles := map[string]bool{}
	for _, img := range images {
		if img.MemberID != localMemberID {
			continue
		}

		for versionIdentifier, imageVersion := range img.Versions {
			for filename := range imageVersion.Items {
				knownImageFiles[path.Join(filepath.ToSlash(img.FilePath()), versionIdentifier, filename)] = true
			}
		}
	}

	return errors.Join(
		memberfiles.DeleteUnknownFiles(filepath.Join(d.env.VarDir(), memberfiles.StoreUpdates), knownUpdateFiles),
		memberfiles.DeleteUnknownFiles(filepath.Join(d.env.VarDir(), memberfiles.StoreImages), knownImageFiles),
		pruneClusterArtifacts(ctx),
	)
}

func (d *Daemon) setupTokenService(db dbdriver.DBTX, client provisioning.TokenClientPort, updateSvc provisioning.UpdateService, channelSvc provisioning.ChannelService) (provisioning.TokenService, *flasher.Flasher) {
	imageFlasher := flasher.New(
		config.GetNetwork().OperationsCenterAddress,
		d.apiCertificate,
		flasher.WithCacheDir(filepath.Join(d.env.CacheDir(), seedImageCacheDir)),
	)
	// Image flasher needs to learn about updates to the server certificate.
	lifecycle.ServerCertificateUpdateSignal.AddListener(func(_ context.Context, cert tls.Certificate) {
		imageFlasher.UpdateCertificate(cert)
	})
	// Image flasher needs to learn about updates the public Operations Center address.
	lifecycle.SharedNetworkUpdateSignal.AddListener(func(ctx context.Context, cfg apisystem.NetworkGlobal) {
		imageFlasher.UpdateServerURL(cfg.OperationsCenterAddress)
	})

	tokenSvc := provisioningServiceMiddleware.NewTokenServiceWithSlog(
		provisioningToken.New(
			provisioningRepoMiddleware.NewTokenRepoWithSlog(
				provisioningSqlite.NewToken(db),
			),
			updateSvc,
			channelSvc,
			imageFlasher,
			client,
		),
	)

	lifecycle.BMCVirtualMediaSignal.AddListener(func(ctx context.Context, msg lifecycle.BMCVirtualMediaMessage) {
		switch msg.Operation {
		case lifecycle.BMCVirtualMediaOperationPreAttach:
			go func() {
				// Use a detached context, so that generating the image is neither
				// bound to a DB transaction nor to the request having triggered it.
				ctx := context.WithoutCancel(ctx)

				slog.InfoContext(ctx, "Preparation of installation media triggered", slog.String("name", msg.Server))

				err := tokenSvc.PrepareTokenSeedImage(ctx, msg.TokenUUID, msg.Seed, msg.ImageType, msg.Architecture, msg.Channel)
				if err != nil {
					slog.WarnContext(ctx, "Failed to prepare installation media", logger.Err(err), slog.String("name", msg.Server))
					return
				}

				slog.InfoContext(ctx, "Preparation of installation media completed", slog.String("name", msg.Server))
			}()

		case lifecycle.BMCVirtualMediaOperationAttach:
			slog.DebugContext(ctx, "Installation media attached", slog.String("name", msg.Server), slog.String("virtual_media_id", msg.VirtualMediaID))

		case lifecycle.BMCVirtualMediaOperationDetach:
			// A cached image is shared by all servers installed from the same
			// token seed, so detaching one of them must not remove it. Cached
			// images are removed time based instead.
			slog.DebugContext(ctx, "Installation media detached", slog.String("name", msg.Server), slog.String("virtual_media_id", msg.VirtualMediaID))
		}
	})

	return tokenSvc, imageFlasher
}

func (d *Daemon) setupServerService(
	db dbdriver.DBTX,
	client provisioning.ServerClientPort,
	runner scriptlet.Runner,
	tokenSvc provisioning.TokenService,
	clusterSvc provisioning.ClusterService,
	channelSvc provisioning.ChannelService,
	updateSvc provisioning.UpdateService,
	warningSvc provisioning.WarningServicePort,
	biosProfile provisioning.BIOSProfilePort,
	seedImageProgress provisioning.SeedImageProgressPort,
	secureBootMedia provisioning.SecureBootMediaPort,
	secureBootCatalog provisioning.SecureBootCertificateCatalogPort,
) provisioning.ServerService {
	serverSvc := provisioningServer.New(
		provisioningRepoMiddleware.NewServerRepoWithSlog(
			provisioningSqlite.NewServer(db),
		),
		provisioningAdapterMiddleware.NewServerClientPortWithSlog(
			provisioningAdapterMiddleware.NewServerClientPortWithErrorWrapper(
				client,
				domain.RetryableWrapper(),
			),
		),
		runner,
		tokenSvc,
		clusterSvc,
		channelSvc,
		updateSvc,
		d.apiCertificate,
		provisioningServer.WithWarningEmitter(warningSvc),
		provisioningServer.WithBIOSProfilePort(biosProfile),
		provisioningServer.WithSeedImageProgressPort(seedImageProgress),
		provisioningServer.WithSecureBootMediaPort(secureBootMedia, d.env, secureBootCatalog),
		provisioningServer.AddBMCServerClient(
			api.BMCAPITypeRedfishV1Generic,
			provisioningAdapterMiddleware.NewBMCServerClientPortWithSlog(
				provisioningAdapterMiddleware.NewBMCServerClientPortWithErrorWrapper(
					redfish.New(
						redfish.WithSecureBootCertificates(d.env),
					),
					redfish.ErrorWrapper(),
				),
			),
		),
	)

	// Server service needs to learn about updates of the public Operations Center
	// address.
	lifecycle.NetworkUpdateSignal.AddListener(func(ctx context.Context, cfg apisystem.Network) {
		// Update operations center server record with updated network config.
		err := serverSvc.SelfRegisterOperationsCenter(ctx)
		if err != nil {
			slog.WarnContext(ctx, "failed to update server URL", logger.Err(err))
		}
	})

	// Server service needs to learn about updates of the server certificate.
	lifecycle.ServerCertificateUpdateSignal.AddListenerWithErr(func(ctx context.Context, c tls.Certificate) error {
		return serverSvc.UpdateServerCertificate(ctx, c)
	})

	return provisioningServiceMiddleware.NewServerServiceWithSlog(
		serverSvc,
	)
}

func (d *Daemon) setupClusterService(
	db dbdriver.DBTX,
	client provisioning.ClusterClientPort,
	serverSvc provisioning.ServerService,
	tokenSvc provisioning.TokenService,
	inventoryAggregateSvc inventory.InventoryAggregateService,
	warningSvc provisioning.WarningServicePort,
	clusterArtifactRepo provisioning.ClusterArtifactRepo,
) (provisioning.ClusterService, error) {
	tmpTerraformDir, err := os.MkdirTemp("", "operations-center-terraform-*")
	if err != nil {
		tmpTerraformDir = os.TempDir()
	}

	terraformProvisioner, err := terraform.New(
		tmpTerraformDir,
		d.clientCertificate,
		d.clientKey,
	)
	if err != nil {
		return nil, err
	}

	return provisioningServiceMiddleware.NewClusterServiceWithSlog(
		provisioningCluster.New(
			provisioningRepoMiddleware.NewClusterRepoWithSlog(
				provisioningSqlite.NewCluster(db),
			),
			provisioningRepoMiddleware.NewClusterArtifactRepoWithSlog(
				clusterArtifactRepo,
			),
			provisioningAdapterMiddleware.NewClusterClientPortWithSlog(
				provisioningAdapterMiddleware.NewClusterClientPortWithErrorWrapper(
					client,
					domain.RetryableWrapper(),
				),
			),
			serverSvc,
			tokenSvc,
			nil,
			terraformProvisioner,
			inventoryAggregateSvc,
			provisioningCluster.WithWarningEmitter(warningSvc),
		),
	), nil
}

func (d *Daemon) setupClusterTemplateService(db dbdriver.DBTX) provisioning.ClusterTemplateService {
	return provisioningServiceMiddleware.NewClusterTemplateServiceWithSlog(
		provisioningClusterTemplate.New(
			provisioningRepoMiddleware.NewClusterTemplateRepoWithSlog(
				provisioningSqlite.NewClusterTemplate(db),
			),
		),
	)
}

func (d *Daemon) setupChannelService(db dbdriver.DBTX, updateSvc provisioning.UpdateService) provisioning.ChannelService {
	return provisioningServiceMiddleware.NewChannelServiceWithSlog(
		provisioningChannel.New(
			provisioningRepoMiddleware.NewChannelRepoWithSlog(
				provisioningSqlite.NewChannel(db),
			),
			updateSvc,
		),
	)
}

func (d *Daemon) setupSystemService(serverSvc provisioning.ServerService) system.SystemService {
	return systemServiceMiddleware.NewSystemServiceWithSlog(
		system.NewSystemService(
			d.env, serverSvc,
			systemRepoMiddleware.NewCacheRepoWithSlog(
				systemLocalfs.New(d.env.CacheDir(), seedImageCacheDir),
			),
			system.WithClusterCertificateUpdater(func(ctx context.Context, certificatePEM string, keyPEM string) error {
				return d.membershipSvc.UpdateClusterCertificate(ctx, certificatePEM, keyPEM)
			}),
		),
	)
}

// requestRestart asks the daemon to restart. It does not block.
func (d *Daemon) requestRestart() {
	select {
	case d.restartCh <- struct{}{}:
	default:
	}
}

// RestartRequested is signaled, once the daemon asks to be restarted.
func (d *Daemon) RestartRequested() <-chan struct{} {
	return d.restartCh
}

func (d *Daemon) setupAPIRoutes(
	ctx context.Context,
	updateSvc provisioning.UpdateService,
	tokenSvc provisioning.TokenService,
	serverSvc provisioning.ServerService,
	clusterSvc provisioning.ClusterService,
	clusterTemplateSvc provisioning.ClusterTemplateService,
	channelSvc provisioning.ChannelService,
	warningSvc warning.WarningService,
	inventoryInventoryAggregateSvc inventory.InventoryAggregateService,
	imageSourceSvc image.IncusImageSourceService,
	incusImageSvc image.ImageIncusService,
	seedImageProgress provisioning.SeedImageProgressPort,
	secureBootMedia provisioning.SecureBootMediaPort,
	membershipSvc clusteringSvc.MembershipService,
	fileMover memberfiles.Mover,
	db *dbtxWrapper,
) (*http.ServeMux, map[domain.ResourceType]provisioning.InventorySyncer) {
	// serverClientProvider is a provider of a client to access (Incus) servers
	// or clusters.
	serverClientProvider := serverMiddleware.NewServerClientWithSlog(
		serverMiddleware.NewServerClientWithErrorWrapper(
			incusAdapter.New(
				d.clientCertificate,
				d.clientKey,
			),
			domain.RetryableWrapper(),
		),
	)

	serveMux := http.NewServeMux()
	// TODO: Move access log and request ID middlewares here
	router := newRouter(serveMux)

	registerUIHandlers(ctx, router, ui.FS(), d.env.UsrShareDir())
	for p, f := range membershipSvc.HandlerFuncs() {
		router.HandleFunc(p, f)
	}

	registerWellKnownHandler(router)

	const osRouterPrefix = "/os"
	osRouter := router.SubGroup(osRouterPrefix).AddMiddlewares(
		d.authenticator.Middleware(),
	)
	registerOSProxy(osRouter, osRouterPrefix, d.authorizer, d.env)

	requireDatabase := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if db.DBTX == nil {
				err := response.SmartError(api.StatusErrorf(http.StatusTooEarly, "Database is still initializing")).Render(w)
				if err != nil {
					slog.WarnContext(r.Context(), "Render error response failed", logger.Err(err))
				}

				return
			}

			next(w, r)
		}
	}

	// The internal API is not versioned. It can change at any time.
	internalRouter := router.SubGroup("/internal").AddMiddlewares(
		d.authenticator.Middleware(),
	)
	registerInternalHandler(internalRouter.SubGroup("").AddMiddlewares(requireDatabase), d.authorizer, db)

	debugRouter := internalRouter.SubGroup("/debug")
	registerDebugHandler(debugRouter, d.authorizer)

	internalUpdatesRouter := internalRouter.SubGroup("/updates").AddMiddlewares(requireDatabase)
	registerInternalUpdateFilesHandler(internalUpdatesRouter, d.authorizer, updateSvc)

	internalImagesRouter := internalRouter.SubGroup("/images").AddMiddlewares(requireDatabase)
	registerInternalImageFilesHandler(internalImagesRouter, d.authorizer, incusImageSvc, imageSourceSvc)

	internalFilesRouter := internalRouter.SubGroup("/files")
	registerInternalMemberFilesHandler(internalFilesRouter, d.authorizer, memberfiles.NewLocal(d.env.VarDir()), membershipSvc.MemberClient)

	internalMembershipRouter := router.SubGroup("/internal/membership")
	registerInternalClusterMembershipHandler(internalMembershipRouter, d.authenticator, d.isClusterMemberRequest, membershipSvc, d.requestRestart)

	if d.oidcVerifier != nil {
		registerOIDCHandlers(router, d.oidcVerifier)
	}

	isAuthenticationRequired := func(r *http.Request) bool {
		// GET /1.0 signals the available authentication methods
		if r.Method == http.MethodGet && r.URL.Path == "/1.0" {
			return false
		}

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

		if r.Pattern == "GET /1.0/provisioning/tokens/{uuid}/seeds/{name}/{params...}" {
			return false
		}

		if r.Pattern == "GET /1.0/provisioning/secure-boot-media/{filename}" {
			return false
		}

		return true
	}

	// Endpoints that should not be forwarded to the leader.
	memberLocalPaths := []string{
		// Local configuration.
		"/1.0/system/network",
		"/1.0/system/settings",
		"/1.0/system/:clean-cache",
		// Cluster bootstrap APIs are local only.
		"/1.0/membership/:enable",
		"/1.0/membership/:join",
		// Only accepted through the unix socket of this member.
		"/1.0/provisioning/servers/:self_register",
	}

	forward := forwarder{
		clustered:  d.env.Clustered,
		membership: membershipSvc,
		targets:    memberfiles.NewTargets(func() memberfiles.Cluster { return d.membershipSvc }),
	}

	requireLeader := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			// IncusOS of this member sends its self updates through the unix socket.
			localSelfUpdate := r.URL.Path == "/1.0/provisioning/servers/:self" && r.TLS == nil

			if !slices.Contains([]string{http.MethodGet, http.MethodHead}, r.Method) && !slices.Contains(memberLocalPaths, r.URL.Path) && !localSelfUpdate {
				resp := forward.toLeader(r)
				if resp != nil {
					err := resp.Render(w)
					if err != nil {
						slog.WarnContext(r.Context(), "Render forwarded response failed", logger.Err(err))
					}

					return
				}
			}

			next(w, r)
		}
	}

	simplestreamsRouter := router.SubGroup("/incus-images")
	simplestreamsRouter.AddMiddlewares(cors.Handler(cors.CORSConfig{
		AllowedOrigins:   "*",
		AllowedMethods:   "GET",
		AllowedHeaders:   "*",
		AllowCredentials: false,
	}))

	authenticatorMiddleware := d.authenticator.Middleware(authn.WithIsAuthenticationRequired(isAuthenticationRequired))

	api10PreInit := router.SubGroup("/1.0")
	api10PreInitAuth := api10PreInit.SubGroup("").AddMiddlewares(authenticatorMiddleware)
	registerAPI10Handler(api10PreInitAuth)

	api10router := api10PreInitAuth.AddMiddlewares(requireDatabase, requireLeader)

	imageRouter := api10router.SubGroup("/images")
	imageSourceRouter := imageRouter.SubGroup("/sources")
	registerImageSourceHandler(imageSourceRouter, d.authorizer, imageSourceSvc)
	imageIncusRouter := imageRouter.SubGroup("/incus")
	registerImageIncusHandler(imageIncusRouter, simplestreamsRouter, d.authorizer, incusImageSvc, forward)
	imageIncusSourceRouter := imageIncusRouter.SubGroup("/sources")
	registerImageSourceHandler(imageIncusSourceRouter, d.authorizer, imageSourceSvc)

	metricsRouter := api10router.SubGroup("/metrics")
	registerMetricsHandler(metricsRouter, d.authorizer, metrics.Gatherer())

	membershipRouter := api10router.SubGroup("/membership")
	registerClusterMembershipHandler(membershipRouter, d.authorizer, membershipSvc, d.requestRestart, fileMover)

	provisioningRouter := api10router.SubGroup("/provisioning")

	provisioningTokenRouter := provisioningRouter.SubGroup("/tokens")
	registerProvisioningTokenHandler(provisioningTokenRouter, d.authorizer, tokenSvc, seedImageProgress, forward)

	provisioningSecureBootMediaRouter := provisioningRouter.SubGroup("/secure-boot-media")
	registerSecureBootMediaHandler(provisioningSecureBootMediaRouter, secureBootMedia)

	provisioningClusterRouter := provisioningRouter.SubGroup("/clusters")
	registerProvisioningClusterHandler(provisioningClusterRouter, d.authorizer, clusterSvc, clusterTemplateSvc, forward)

	provisioningClusterTemplateRouter := provisioningRouter.SubGroup("/cluster-templates")
	registerProvisioningClusterTemplateHandler(provisioningClusterTemplateRouter, d.authorizer, clusterTemplateSvc)

	provisioningServerRouter := provisioningRouter.SubGroup("/servers")
	registerProvisioningServerHandler(
		provisioningServerRouter,
		"/1.0/provisioning/servers",
		d.authorizer,
		serverSvc,
		d.clientCertificate,
		d.clientKey,
	)

	provisioningUpdateRouter := provisioningRouter.SubGroup("/updates")
	registerUpdateHandler(provisioningUpdateRouter, d.authorizer, updateSvc, forward)

	provisioningChannelRouter := provisioningRouter.SubGroup("/channels")
	registerChannelsHandler(provisioningChannelRouter, d.authorizer, channelSvc)

	systemRouter := api10router.SubGroup("/system")
	registerSystemHandler(systemRouter, d.authorizer, d.systemSvc)

	warningRouter := api10router.SubGroup("/warnings")
	registerWarningHandler(warningRouter, d.authorizer, warningSvc)

	inventoryRouter := api10router.SubGroup("/inventory")

	inventorySyncers := registerInventoryRoutes(db, clusterSvc, serverClientProvider, d.authorizer, inventoryRouter, inventoryInventoryAggregateSvc)

	return serveMux, inventorySyncers
}

// isClusterMemberRequest reports whether r is sent by another cluster member.
func (d *Daemon) isClusterMemberRequest(r *http.Request) bool {
	if d.membershipSvc == nil || r.TLS == nil {
		return false
	}

	members := d.membershipSvc.TrustedClients()
	for _, cert := range r.TLS.PeerCertificates {
		_, ok := members[incustls.CertFingerprint(cert)]
		if ok {
			return true
		}
	}

	return false
}

// lifecycleEventsMonitorTask monitors the lifecycle events of the clusters until it is stopped.
func lifecycleEventsMonitorTask(clusterSvc provisioning.ClusterService) task.TaskFunc {
	return func(ctx context.Context) func(context.Context) error {
		ctx, cancel := context.WithCancel(ctx)

		err := clusterSvc.StartLifecycleEventsMonitor(ctx)
		if err != nil {
			slog.ErrorContext(ctx, "Failed to start cluster lifecycle events monitor", logger.Err(err))
		}

		return func(context.Context) error {
			cancel()

			return nil
		}
	}
}

func clusterHeartbeatTask(ctx context.Context, membershipSvc clusteringSvc.MembershipService) func(time.Duration) error {
	clusterTask := func(ctx context.Context) {
		if membershipSvc.Gateway().HeartbeatCancelFunc() == nil {
			ch := make(chan struct{})
			go func() {
				membershipSvc.Gateway().Heartbeat(ctx, cowsqlheartbeat.HeartbeatNormal)
				close(ch)
			}()
			select {
			case <-ch:
			case <-ctx.Done():
			}
		}
	}

	clusterTaskStop, _ := task.Start(ctx, clusterTask, task.Every(membershipSvc.Gateway().HeartbeatInterval()))

	return clusterTaskStop
}

// firstRunDelay runs prepare and waits for delay on the first call, false if the context ends while waiting.
func firstRunDelay(prepare func(context.Context), delay time.Duration) func(context.Context) bool {
	var mu sync.Mutex
	prepared := false
	waited := delay == 0

	return func(ctx context.Context) bool {
		mu.Lock()
		defer mu.Unlock()

		if !prepared && prepare != nil {
			prepare(ctx)
		}

		prepared = true

		if waited {
			return true
		}

		select {
		case <-ctx.Done():
			return false

		case <-time.After(delay):
		}

		waited = true

		return true
	}
}

func (d *Daemon) backgroundTasks(
	updateSvc provisioning.UpdateService,
	imageSourceSvc image.IncusImageSourceService,
	serverSvc provisioning.ServerService,
	clusterSvc provisioning.ClusterService,
	imageFlasher *flasher.Flasher,
	secureBootMedia provisioning.SecureBootMediaPort,
	warningSvc warning.WarningEmitter,
) []task.TaskFunc {
	if config.IsBackgroundTasksDisabled() {
		return []task.TaskFunc{}
	}

	pruneUpdates := func(ctx context.Context) {
		err := updateSvc.Prune(ctx)
		if err != nil {
			slog.WarnContext(ctx, "Failed to prune pending updates", logger.Err(err))
		}
	}

	// Give IncusOS some time to apply seed data before actually starting to
	// perform the refresh of the updates.
	delayFirstUpdates := firstRunDelay(pruneUpdates, 0)
	if d.env.IsIncusOS() {
		delayFirstUpdates = firstRunDelay(func(ctx context.Context) {
			pruneUpdates(ctx)
			slog.InfoContext(ctx, "Refresh updates delayed for 30s on IncusOS")
		}, 30*time.Second)
	}

	// Start background task to refresh updates from the sources.
	refreshUpdatesFromSourcesTask := componentTask(componentTaskRefreshUpdates, func(ctx context.Context) {
		if !delayFirstUpdates(ctx) {
			return
		}

		slog.InfoContext(ctx, "Refresh updates triggered")
		err := updateSvc.Refresh(ctx)
		scope := api.WarningScope{
			Scope:      "refresh",
			EntityType: "update",
			Entity:     "-",
		}
		if err != nil {
			warningSvc.Emit(
				ctx,
				warning.NewWarningFromError(
					api.WarningTypeUpdateRefreshFailed,
					scope,
					err,
					"Refresh update failed",
				),
			)

			return
		}

		warningSvc.RemoveStale(ctx, scope, nil)

		slog.InfoContext(ctx, "Refresh updates completed")
	})

	var updateSourceOptions []task.EveryOption
	if config.SourcePollSkipFirst() {
		updateSourceOptions = append(updateSourceOptions, task.SkipFirst)
	}

	// Give IncusOS some time to download update before starting to refresh
	// image sources.
	delayFirstImageSourcesRefresh := firstRunDelay(nil, 0)
	if d.env.IsIncusOS() {
		delayFirstImageSourcesRefresh = firstRunDelay(func(ctx context.Context) {
			slog.InfoContext(ctx, "Refresh image sources delayed for 10m on IncusOS")
		}, 10*time.Minute)
	}

	// Start background task to refresh images from the sources.
	refreshImageSourcesTask := componentTask(componentTaskRefreshImageSources, func(ctx context.Context) {
		if !delayFirstImageSourcesRefresh(ctx) {
			return
		}

		slog.InfoContext(ctx, "Refresh image sources triggered")
		err := imageSourceSvc.RefreshAll(ctx)
		scope := api.WarningScope{
			Scope:      "refresh",
			EntityType: "image_sources",
			Entity:     "-",
		}
		if err != nil {
			warningSvc.Emit(
				ctx,
				warning.NewWarningFromError(
					api.WarningTypeUpdateRefreshFailed,
					scope,
					err,
					"Refresh image sources failed",
				),
			)

			return
		}

		warningSvc.RemoveStale(ctx, scope, nil)

		slog.InfoContext(ctx, "Refresh image sources completed")
	})

	// Start background task for cluster update control loop.
	clusterUpdateControlLoop := componentTask(componentTaskClusterUpdate, func(ctx context.Context) {
		slog.InfoContext(ctx, "Cluster update control loop triggered")
		err := clusterSvc.ClusterUpdateControlLoop(ctx, nil)
		if err != nil {
			logCtx := slog.ErrorContext
			if domain.IsRetryableError(err) {
				logCtx = slog.InfoContext
			}

			logCtx(ctx, "Cluster update control loop failed", logger.Err(err))

			return
		}

		slog.InfoContext(ctx, "Cluster update control loop completed")
	})

	// runServerDeploymentControlLoop advances the automated deployments. A name
	// restricts it to a single server, so an event, that satisfies a wait
	// condition, resolves within seconds instead of on the next tick.
	runServerDeploymentControlLoop := func(ctx context.Context, trigger string, name *string) {
		slog.DebugContext(ctx, "Server deployment control loop triggered", slog.String("trigger", trigger), slog.String("server", ptr.From(name)))

		err := serverSvc.DeploymentControlLoop(ctx, name)
		if err != nil {
			logCtx := slog.ErrorContext
			if domain.IsRetryableError(err) {
				logCtx = slog.InfoContext
			}

			logCtx(ctx, "Server deployment control loop failed", logger.Err(err), slog.String("trigger", trigger), slog.String("server", ptr.From(name)))

			return
		}

		slog.DebugContext(ctx, "Server deployment control loop completed", slog.String("trigger", trigger), slog.String("server", ptr.From(name)))
	}

	serverDeploymentControlLoopTask := componentTask(componentTaskServerDeployment, func(ctx context.Context) {
		runServerDeploymentControlLoop(ctx, "tick", nil)
	})

	checkClusterLeader := func(ctx context.Context, leaderLog *slog.Logger) bool {
		leaderCtx, cancel := context.WithTimeout(ctx, time.Second*10)
		defer cancel()

		isLeader, err := d.membershipSvc.Gateway().IsLeader(leaderCtx)
		if err != nil {
			leaderLog.ErrorContext(ctx, "Failed to check for cluster leadership background task signal", logger.Err(err))
			return false
		}

		return isLeader
	}

	// Trigger the server deployment control loop from the virtual media events,
	// so attaching and detaching the installation media resolves promptly.
	lifecycle.BMCVirtualMediaSignal.AddListener(func(ctx context.Context, msg lifecycle.BMCVirtualMediaMessage) {
		if msg.Operation == lifecycle.BMCVirtualMediaOperationPreAttach {
			return
		}

		isLeader := checkClusterLeader(ctx, slog.Default())
		if !isLeader {
			return
		}

		name := msg.Server

		go func() {
			time.Sleep(config.ServerDeploymentVirtualMediaTriggerDelay)

			runServerDeploymentControlLoop(context.WithoutCancel(ctx), "virtual media", &name)
		}()
	})

	// Trigger ClusterUpdateControlLoop also from server lifecycle events.
	lifecycle.ServerLifecycleSignal.AddListener(func(ctx context.Context, slm lifecycle.ServerLifecycleMessage) {
		log := slog.With(slog.String("server", slm.Server), slog.String("cluster", ptr.From(slm.Cluster)), slog.String("update_state", slm.ServerUpdateState.String()))
		isLeader := checkClusterLeader(ctx, log)
		if !isLeader {
			return
		}

		slog.InfoContext(ctx, "Server lifecycle event triggered", slog.String("server", slm.Server), slog.String("cluster", ptr.From(slm.Cluster)), slog.String("update_state", slm.ServerUpdateState.String()))

		// A server, that just registered itself, ends the installation it was
		// deployed with, so the deployment is advanced right away. It runs on its
		// own goroutine, since it blocks on the BMC of the server.
		name := slm.Server

		go runServerDeploymentControlLoop(context.WithoutCancel(ctx), "server lifecycle", &name)

		err := clusterSvc.ClusterUpdateControlLoop(ctx, slm.Cluster)
		if err != nil {
			logCtx := slog.ErrorContext
			if domain.IsRetryableError(err) {
				logCtx = slog.InfoContext
			}

			logCtx(ctx, "Failed to handle server lifecycle event", logger.Err(err), slog.String("server", slm.Server), slog.String("cluster", ptr.From(slm.Cluster)), slog.String("update_state", slm.ServerUpdateState.String()))

			return
		}

		slog.InfoContext(ctx, "Server lifecycle event completed", slog.String("server", slm.Server), slog.String("cluster", ptr.From(slm.Cluster)), slog.String("update_state", slm.ServerUpdateState.String()))
	})

	// Start background task to test connectivity and update configuration with servers in ready state.
	pollReadyServersTask := componentTask(componentTaskPollReadyServers, func(ctx context.Context) {
		slog.InfoContext(ctx, "Connectivity test for ready servers triggered")

		// Within the first connectivityInterval of the hour, we also update the configuration.
		updateConfiguration := time.Since(time.Now().Truncate(time.Hour)) <= config.ConnectivityCheckInterval
		err := serverSvc.PollServers(ctx, provisioning.ServerFilter{
			Status: new(api.ServerStatusReady),
		}, updateConfiguration)
		if err != nil {
			logCtx := slog.ErrorContext
			if domain.IsRetryableError(err) {
				logCtx = slog.DebugContext
			}

			logCtx(ctx, "Connectivity test for some servers failed", logger.Err(err))

			return
		}

		slog.InfoContext(ctx, "Connectivity test for ready servers completed")
	})

	// Start background task to refresh inventory through polling.
	refreshInventoryTask := componentTask(componentTaskRefreshInventory, func(ctx context.Context) {
		slog.InfoContext(ctx, "Inventory update triggered")
		err := clusterSvc.ResyncInventory(ctx)
		if err != nil {
			logCtx := slog.ErrorContext
			if domain.IsRetryableError(err) {
				logCtx = slog.DebugContext
			}

			logCtx(ctx, "Inventory update failed", logger.Err(err))

			return
		}

		slog.InfoContext(ctx, "Inventory update completed")
	})

	// Start background task to refresh BMC data.
	refreshBMCDataTask := componentTask(componentTaskRefreshBMCData, func(ctx context.Context) {
		slog.InfoContext(ctx, "BMC data resync triggered")
		err := serverSvc.ResyncBMCData(ctx)
		if err != nil {
			logCtx := slog.ErrorContext
			if domain.IsRetryableError(err) {
				logCtx = slog.DebugContext
			}

			logCtx(ctx, "BMC data resync failed", logger.Err(err))

			return
		}

		slog.InfoContext(ctx, "BMC data resync completed")
	})

	// Start background task to prune the cache of generated seed images.
	pruneSeedImageCacheTask := componentTask(componentTaskPruneSeedImageCache, func(ctx context.Context) {
		slog.InfoContext(ctx, "Seed image cache prune triggered")

		err := imageFlasher.PruneCache(ctx, config.SeedImageCacheTTL)
		if err != nil {
			slog.WarnContext(ctx, "Seed image cache prune failed", logger.Err(err))

			return
		}

		slog.InfoContext(ctx, "Seed image cache prune completed")
	})

	// Start background task to prune the generated secure boot enrollment media.
	pruneSecureBootMediaTask := func(ctx context.Context) {
		slog.InfoContext(ctx, "Secure boot enrollment media prune triggered")

		// A media, that a deployment has attached to a BMC, is kept: the BMC
		// reads it only when the server boots, so it would otherwise be pruned
		// while the server still needs it.
		inUse, err := attachedSecureBootMediaIDs(ctx, serverSvc)
		if err != nil {
			slog.WarnContext(ctx, "Secure boot enrollment media prune failed", logger.Err(err))

			return
		}

		err = secureBootMedia.Prune(ctx, config.SecureBootMediaCacheTTL, inUse)
		if err != nil {
			slog.WarnContext(ctx, "Secure boot enrollment media prune failed", logger.Err(err))

			return
		}

		slog.InfoContext(ctx, "Secure boot enrollment media prune completed")
	}

	// Start background task to renew ACME server certificate.
	renewACMEServerCertificateTask := componentTask(componentTaskRenewACMECertificate, func(ctx context.Context) {
		slog.InfoContext(ctx, "ACME server certificate renewal triggered")

		scope := api.WarningScope{
			Scope:      "acme_certificate_update",
			EntityType: "system",
			Entity:     "operations-center",
		}

		changed, err := d.systemSvc.TriggerCertificateRenew(ctx, false)
		if err != nil {
			warningSvc.Emit(
				ctx,
				warning.NewWarningFromError(
					api.WarningTypeACMECertificateUpdateFailed,
					scope,
					err,
					"ACME server certificate renewal task failed",
				),
			)
			return
		}

		warningSvc.RemoveStale(ctx, scope, nil)

		if !changed {
			slog.InfoContext(ctx, "ACME server certificate renewal completed, no change")
			return
		}

		slog.InfoContext(ctx, "ACME server certificate renewal completed")
	})

	// Start background task to check certificate validity of server and cluster certificates.
	certificatesValidityCheckTask := componentTask(componentTaskCertificatesValidityCheck, func(ctx context.Context) {
		slog.InfoContext(ctx, "Certificates validity check triggered")

		clusters, err := clusterSvc.GetAll(ctx)
		if err != nil {
			slog.ErrorContext(ctx, "Failed to get clusters during certificates validity check", logger.Err(err))
		}

		warnings := warning.Warnings{}
		for _, cluster := range clusters {
			if cluster.Certificate == nil {
				continue
			}

			scope := api.WarningScope{
				Scope:      "certificate_validity_check",
				EntityType: "cluster",
				Entity:     cluster.Name,
			}

			valid, err := certificate.Validate(*cluster.Certificate, config.CertificateExpiryWarningThreshold)
			if err != nil {
				if valid {
					warnings = append(warnings, warning.NewWarningFromError(api.WarningTypeCertificateExpiration, scope, err, ""))

					continue
				}

				warnings = append(warnings, warning.NewWarningFromError(api.WarningTypeCertificateInvalid, scope, err, ""))
			}
		}

		warningSvc.RemoveStale(ctx, api.WarningScope{
			Scope:      "certificate_validity_check",
			EntityType: "cluster",
		}, warnings)

		for _, w := range warnings {
			warningSvc.Emit(ctx, w)
		}

		servers, err := serverSvc.GetAll(ctx)
		if err != nil {
			slog.ErrorContext(ctx, "Failed to get servers during certificates validity check", logger.Err(err))
		}

		acmeCfg := config.GetSecurity().ACME
		warnings = warning.Warnings{}
		for _, server := range servers {
			if server.Certificate == nil {
				continue
			}

			scope := api.WarningScope{
				Scope:      "certificate_validity_check",
				EntityType: "server",
				Entity:     server.Name,
			}

			valid, err := validateServerCertificate(server, acmeCfg)
			if err != nil {
				if valid {
					warnings = append(warnings, warning.NewWarningFromError(api.WarningTypeCertificateExpiration, scope, err, ""))

					continue
				}

				warnings = append(warnings, warning.NewWarningFromError(api.WarningTypeCertificateInvalid, scope, err, ""))
			}
		}

		warningSvc.RemoveStale(ctx, api.WarningScope{
			Scope:      "certificate_validity_check",
			EntityType: "server",
		}, warnings)

		for _, w := range warnings {
			warningSvc.Emit(ctx, w)
		}

		slog.InfoContext(ctx, "Certificates validity check completed")
	})

	return []task.TaskFunc{
		task.New(refreshUpdatesFromSourcesTask, task.Every(config.UpdatesSourcePollInterval, updateSourceOptions...), 60*time.Second),
		task.New(refreshImageSourcesTask, task.Every(config.ImageSourcePollInterval), 60*time.Second),
		task.New(clusterUpdateControlLoop, task.Every(config.PendingServerPollInterval), 5*time.Second),
		task.New(serverDeploymentControlLoopTask, task.Every(config.ServerDeploymentControlLoopInterval), 5*time.Second),
		task.New(pollReadyServersTask, task.Every(config.ConnectivityCheckInterval), 1*time.Second),
		task.New(refreshInventoryTask, task.Every(config.InventoryUpdateInterval), 10*time.Second),
		task.New(refreshBMCDataTask, task.Every(config.BMCDataResyncInterval), 10*time.Second),
		task.New(pruneSeedImageCacheTask, task.Every(config.SeedImageCachePruneInterval), 10*time.Second),
		task.New(pruneSecureBootMediaTask, task.Every(config.SecureBootMediaPruneInterval), 10*time.Second),
		task.New(renewACMEServerCertificateTask, task.Every(config.ACMEServerCertificateRenewInterval), 10*time.Second),
		task.New(certificatesValidityCheckTask, task.Every(config.CertificatesValidityCheckInterval), 10*time.Second),

		// Start background task to poll servers in pending state to become available.
		d.backgroundPollingTask(
			componentTaskPollPendingServers,
			serverSvc,
			"pending",
			provisioning.ServerFilter{
				Status: new(api.ServerStatusPending),
			},
			true,
			config.PendingServerPollInterval,
		),

		// Start background task to poll servers in updating state to become available.
		d.backgroundPollingTask(
			componentTaskPollUpdatingServers,
			serverSvc,
			"updating",
			provisioning.ServerFilter{
				Status:       new(api.ServerStatusReady),
				StatusDetail: new(api.ServerStatusDetailReadyUpdatingOS),
			},
			false,
			config.UpdatingServerPollInterval,
		),

		// Start background task to poll servers updating an application to become available.
		d.backgroundPollingTask(
			componentTaskPollUpdatingAppServers,
			serverSvc,
			"updating application",
			provisioning.ServerFilter{
				Status:       new(api.ServerStatusReady),
				StatusDetail: new(api.ServerStatusDetailReadyUpdatingApplication),
			},
			true,
			config.UpdatingServerPollInterval,
		),

		// Start background task to poll servers in evacuating state to become available.
		d.backgroundPollingTask(
			componentTaskPollEvacuatingServers,
			serverSvc,
			"evacuating",
			provisioning.ServerFilter{
				Status:       new(api.ServerStatusReady),
				StatusDetail: new(api.ServerStatusDetailReadyEvacuating),
			},
			false,
			config.EvacuatingServerPollInterval,
		),

		// Start background task to poll servers in restoring state to become available.
		d.backgroundPollingTask(
			componentTaskPollRestoringServers,
			serverSvc,
			"restoring",
			provisioning.ServerFilter{
				Status:       new(api.ServerStatusReady),
				StatusDetail: new(api.ServerStatusDetailReadyRestoring),
			},
			false,
			config.RestoringServerPollInterval,
		),

		// Start background task to poll servers in rebooting state to become available.
		d.backgroundPollingTask(
			componentTaskPollRebootingServers,
			serverSvc,
			"rebooting",
			provisioning.ServerFilter{
				Status:       new(api.ServerStatusOffline),
				StatusDetail: new(api.ServerStatusDetailOfflineRebooting),
			},
			false,
			config.RebootingServerPollInterval,
		),

		// Start background task to poll servers in unresponsive state to become available.
		d.backgroundPollingTask(
			componentTaskPollUnresponsiveServers,
			serverSvc,
			"unresponsive",
			provisioning.ServerFilter{
				Status:       new(api.ServerStatusOffline),
				StatusDetail: new(api.ServerStatusDetailOfflineUnresponsive),
			},
			false,
			config.UnresponsiveServerPollInterval,
		),
	}
}

// attachedSecureBootMediaIDs returns the secure boot enrollment media, that the
// deployment of a server still names (including failed deployments), so the
// prune leaves it alone.
func attachedSecureBootMediaIDs(ctx context.Context, serverSvc provisioning.ServerService) ([]string, error) {
	servers, err := serverSvc.GetAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("Failed to get the servers to keep their secure boot enrollment media: %w", err)
	}

	var ids []string

	for _, server := range servers {
		if server.StatusInternal.Deployment == nil {
			continue
		}

		id := server.StatusInternal.Deployment.SecureBootMediaID
		if id == "" || slices.Contains(ids, id) {
			continue
		}

		ids = append(ids, id)
	}

	return ids, nil
}

func (d *Daemon) backgroundPollingTask(
	component logger.Component,
	serverSvc provisioning.ServerService,
	stateDescription string,
	serverFilter provisioning.ServerFilter,
	updateServerConfiguration bool,
	interval time.Duration,
) task.TaskFunc {
	backgroundTask := componentTask(component, func(ctx context.Context) {
		slog.InfoContext(ctx, "Polling servers triggered", slog.String("state_description", stateDescription))
		err := serverSvc.PollServers(ctx, serverFilter, updateServerConfiguration)
		if err != nil {
			logCtx := slog.ErrorContext
			if domain.IsRetryableError(err) {
				logCtx = slog.DebugContext
			}

			logCtx(ctx, "Polling servers failed", slog.String("state_description", stateDescription), logger.Err(err))

			return
		}

		slog.InfoContext(ctx, "Polling servers completed", slog.String("state_description", stateDescription))
	})

	return task.New(backgroundTask, task.Every(interval), time.Second)
}

func (d *Daemon) setupSocketListener(ctx context.Context) error {
	// TODO: if the socket file already exists, make a connection attempt. If
	// successful, another instance of operations-centerd is already running.
	// If not successful, it is save to delete the socket file.
	if file.PathExists(d.env.GetUnixSocket()) {
		err := os.Remove(d.env.GetUnixSocket())
		if err != nil {
			return err
		}
	}

	// The listener is created synchronously, so the socket is guaranteed to accept
	// connections once this function returns.
	unixListener, err := net.Listen("unix", d.env.GetUnixSocket())
	if err != nil {
		return err
	}

	slog.InfoContext(ctx, "Start unix socket listener", slog.Any("addr", unixListener.Addr()))

	d.errgroup.Go(func() error {
		err := d.server.Serve(unixListener)
		if errors.Is(err, http.ErrServerClosed) {
			// Ignore error from graceful shutdown.
			return nil
		}

		return err
	})

	return nil
}

func (d *Daemon) setupTCPListener(ctx context.Context, cfg apisystem.Network) error {
	errCh := make(chan error)

	d.errgroup.Go(func() error {
		d.configReloadMu.Lock()
		oldListeners := d.listeners
		d.configReloadMu.Unlock()

		if oldListeners.ServerListener != nil {
			slog.InfoContext(ctx, "Stopping existing https listener", slog.Any("addr", oldListeners.ServerListener.Addr().String()))
			err := oldListeners.ServerListener.Close()
			if err != nil {
				errCh <- err
				return err
			}
		}

		lifecycle.ServerCertificateUpdateSignal.RemoveListener("fancyListener")
		lifecycle.SecurityTrustedHTTPSProxiesUpdateSignal.RemoveListener("fancyListener")

		listenerCert := d.apiCertificate

		if cfg.ClusterAddress != "" {
			isCovered := cfg.RestServerAddress != "" && listener.IsAddressCovered(cfg.ClusterAddress, cfg.RestServerAddress)

			// Stop the cluster listener if the https listener covers its address.
			if isCovered && d.listeners.ClusterListener != nil {
				slog.InfoContext(ctx, "Stopping existing cluster https listener, the https listener covers its address", slog.Any("addr", d.listeners.ClusterListener.Addr().String()))
				err := d.listeners.ClusterListener.Close()
				if err != nil {
					errCh <- err
					return err
				}

				d.configReloadMu.Lock()
				d.listeners.ClusterListener = nil
				d.configReloadMu.Unlock()
			}

			if !isCovered {
				if d.listeners.ClusterListener != nil {
					slog.InfoContext(ctx, "Stopping existing cluster https listener", slog.Any("addr", oldListeners.ClusterListener.Addr().String()))
					err := oldListeners.ClusterListener.Close()
					if err != nil {
						errCh <- err
						return err
					}
				}

				slog.InfoContext(ctx, "Start https cluster listener", slog.Any("addr", cfg.ClusterAddress))
				tcpListener, err := net.Listen("tcp", cfg.ClusterAddress)
				if err != nil {
					errCh <- err
					return err
				}

				d.configReloadMu.Lock()
				d.listeners.ClusterListener = listener.NewFancyTLSListener(tcpListener, listenerCert)
				d.configReloadMu.Unlock()

				err = d.listeners.ClusterListener.TrustedProxy(config.GetSecurity().TrustedHTTPSProxies)
				if err != nil {
					slog.WarnContext(ctx, "Failed to set trusted HTTPS proxies during server startup", logger.Err(err))
				}
			}
		}

		if cfg.RestServerAddress == "" {
			d.configReloadMu.Lock()
			d.listeners.ServerListener = nil
			d.configReloadMu.Unlock()
		} else {
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
			d.listeners.ServerListener = listener.NewFancyTLSListener(tcpListener, listenerCert)
			d.configReloadMu.Unlock()

			err = d.listeners.ServerListener.TrustedProxy(config.GetSecurity().TrustedHTTPSProxies)
			if err != nil {
				slog.WarnContext(ctx, "Failed to set trusted HTTPS proxies during server startup", logger.Err(err))
			}
		}

		if d.listeners.ServerListener == nil && d.listeners.ClusterListener == nil {
			// Unblock the channel here, since we do not start a server.
			errCh <- nil

			return nil
		}

		lifecycle.ServerCertificateUpdateSignal.AddListener(func(_ context.Context, cert tls.Certificate) {
			d.configReloadMu.Lock()
			defer d.configReloadMu.Unlock()

			d.apiCertificate = cert
			d.listeners.UpdateCertificate(cert)
		}, "fancyListener")

		lifecycle.SecurityTrustedHTTPSProxiesUpdateSignal.AddListener(func(_ context.Context, trustedHTTPSProxies []string) {
			d.configReloadMu.Lock()
			defer d.configReloadMu.Unlock()

			d.listeners.UpdateTrustedProxies(ctx, trustedHTTPSProxies)
		}, "fancyListener")

		// Unblock the channel here before we block for the server.
		errCh <- nil

		if d.server != nil {
			return d.listeners.Serve(d.server)
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

	apiResponse := api.Response{}
	err = json.Unmarshal(body, &apiResponse)
	if err != nil {
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("Failed to fetch %s: %s: %s", resp.Request.URL.String(), resp.Status, string(body))
		}

		return err
	}

	if apiResponse.Type == api.ErrorResponse {
		return api.StatusErrorf(resp.StatusCode, "%v", apiResponse.Error)
	}

	return nil
}

func validateServerCertificate(server provisioning.Server, acmeCfg apisystem.SecurityACME) (valid bool, _ error) {
	// ACME renews late in the validity period. A fixed threshold warns before the renewal starts.
	if server.Type == api.ServerTypeOperationsCenter && acme.IsEnabled(acmeCfg) {
		return certificate.ValidateLifetime(*server.Certificate, config.ACMECertificateExpiryWarningLifetimeRatio)
	}

	return certificate.Validate(*server.Certificate, config.CertificateExpiryWarningThreshold)
}

func (d *Daemon) incusOSSelfPoll(ctx context.Context, serverSvc provisioning.ServerService) error {
	if !d.env.IsIncusOS() {
		return nil
	}

	slog.DebugContext(ctx, "Self poll server status on IncusOS to update own inventory record")

	operationCenters, err := serverSvc.GetAllWithFilter(ctx, provisioning.ServerFilter{
		Type: new(api.ServerTypeOperationsCenter),
	})
	if err != nil {
		return fmt.Errorf("Failed to get self server instance: %w", err)
	}

	if len(operationCenters) != 1 {
		return fmt.Errorf(`Expected exactly 1 server of type "operations-center", got: %d`, len(operationCenters))
	}

	serverSelf := operationCenters[0]

	err = serverSvc.PollServer(ctx, serverSelf, true)
	if err != nil {
		return fmt.Errorf("Failed to self poll server instalce: %w", err)
	}

	return nil
}

func (d *Daemon) Stop(ctx context.Context) error {
	ctx = logger.ContextWithComponent(ctx, componentDaemon)

	d.configReloadMu.Lock()
	if d.securityRetryCancel != nil {
		d.securityRetryCancel()
	}

	d.configReloadMu.Unlock()

	errs := make([]error, 0, len(d.shutdownFuncs)+1)

	for _, shutdown := range d.shutdownFuncs {
		err := shutdown(ctx)
		errs = append(errs, err)
	}

	if d.errgroup != nil {
		errgroupWaitErr := d.errgroup.Wait()
		errs = append(errs, errgroupWaitErr)
	}

	// Remove all signal listeners.
	lifecycle.ServerCertificateUpdateSignal.Reset()
	lifecycle.NetworkUpdateSignal.Reset()
	lifecycle.SecurityValidateSignal.Reset()
	lifecycle.SecurityUpdateSignal.Reset()
	lifecycle.SecurityTrustedHTTPSProxiesUpdateSignal.Reset()
	lifecycle.SecurityACMEUpdateSignal.Reset()
	lifecycle.UpdatesValidateSignal.Reset()
	lifecycle.UpdatesUpdateSignal.Reset()
	lifecycle.ClusterUpdateSignal.Reset()
	lifecycle.ServerLifecycleSignal.Reset()
	lifecycle.BMCVirtualMediaSignal.Reset()

	return errors.Join(errs...)
}
