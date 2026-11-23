package cluster

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"maps"
	"net"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	cowsqlclient "github.com/cowsql/go-cowsql/client"
	cowsqlcluster "github.com/cowsql/go-cowsql/cluster"
	cowsqlapi "github.com/cowsql/go-cowsql/cluster/api"
	cowsqldb "github.com/cowsql/go-cowsql/cluster/db"
	"github.com/cowsql/go-cowsql/cluster/db/transaction"
	cowsqlgateway "github.com/cowsql/go-cowsql/cluster/gateway"
	cowsqlheartbeat "github.com/cowsql/go-cowsql/cluster/heartbeat"
	cowsqlmembership "github.com/cowsql/go-cowsql/cluster/membership"
	cowsqloptions "github.com/cowsql/go-cowsql/cluster/options"
	cowsqltls "github.com/cowsql/go-cowsql/cluster/tls"
	"github.com/lxc/incus/v7/shared/osarch"
	"github.com/lxc/incus/v7/shared/revert"
	incustls "github.com/lxc/incus/v7/shared/tls"
	"github.com/lxc/incus/v7/shared/validate"

	"github.com/FuturFusion/operations-center/internal/api/listener"
	"github.com/FuturFusion/operations-center/internal/client"
	membershipapi "github.com/FuturFusion/operations-center/internal/cluster/api"
	"github.com/FuturFusion/operations-center/internal/cluster/memberfiles"
	"github.com/FuturFusion/operations-center/internal/cluster/repo/sqlite/entities"
	config "github.com/FuturFusion/operations-center/internal/config/daemon"
	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/lifecycle"
	authtls "github.com/FuturFusion/operations-center/internal/security/authn/tls"
	securitytls "github.com/FuturFusion/operations-center/internal/security/tls"
	"github.com/FuturFusion/operations-center/internal/system"
	"github.com/FuturFusion/operations-center/internal/util/logger"
	"github.com/FuturFusion/operations-center/internal/util/task"
	"github.com/FuturFusion/operations-center/internal/version"
	"github.com/FuturFusion/operations-center/internal/warning"
	"github.com/FuturFusion/operations-center/shared/api"
	apisystem "github.com/FuturFusion/operations-center/shared/api/system"
)

type membership struct {
	gateway cowsqlcluster.Gateway

	mu sync.Mutex

	stopOnce sync.Once
	stopErr  error

	// The repositories of the global database are set once it is open, while requests already read them.
	globalRepo atomic.Pointer[GlobalRaftRepo]
	configRepo atomic.Pointer[ConfigRepo]

	// ctx is the lifetime of the service.
	ctx    context.Context
	cancel context.CancelFunc
	env    environment

	// varRoot is the var dir, which holds the certificates, the database and the stored files.
	varRoot *os.Root
	system  system.SystemService

	// serverCert is used within the cluster.
	serverCert atomic.Pointer[incustls.CertInfo]

	// pendingClusterCert replaces the generated cluster certificate on bootstrap.
	pendingClusterCert cowsqltls.CertInfo

	// The background tasks run on the cluster leader only.
	backgroundTasks []task.TaskFunc

	shutdownFuncs []func(context.Context) error
	leading       bool
}

// leadershipCheckInterval is the interval of the cluster leadership check.
const leadershipCheckInterval = 5 * time.Second

func NewClusterMembershipService(ctx context.Context, env environment, varRoot *os.Root, systemSvc system.SystemService, listeners *listener.ListenerSet, localRepo LocalRaftRepo, clusterCert *incustls.CertInfo, serverCert *incustls.CertInfo, backgroundTasks []LeaderTask, warnings warning.WarningEmitter) (MembershipService, error) {
	ctx, cancel := context.WithCancel(ctx)

	m := &membership{
		ctx:              ctx,
		cancel:           cancel,
		env:              env,
		varRoot:          varRoot,
		system:           systemSvc,
		clientCertCache:  map[string]x509.Certificate{},
		backgroundTasks:  backgroundTasks,
		handoverTimeout:  leaderHandoverTimeout,
		handoverInterval: leaderHandoverInterval,
		warnings:         warnings,
	}

	m.serverCert.Store(serverCert)

	g, err := cowsqlgateway.NewGateway(
		context.TODO(),
		localRepo,
		clusterCert,
		func() cowsqltls.CertInfo { return m.serverCert.Load() },
		newState(varRoot, systemSvc, listeners, m),
		cowsqloptions.Version(version.Version),
	)
	if err != nil {
		return nil, err
	}

	m.gateway = g

	return m, nil
}

// global returns the repository of the global database, nil until the database is open.
func (m *membership) global() GlobalRaftRepo {
	repo := m.globalRepo.Load()
	if repo == nil {
		return nil
	}

	return *repo
}

// sharedConfig returns the repository of the shared config, nil until the global database is open.
func (m *membership) sharedConfig() ConfigRepo {
	repo := m.configRepo.Load()
	if repo == nil {
		return nil
	}

	return *repo
}

func (m *membership) Bootstrap(ctx context.Context, cfg api.ClusterBootstrapPost) error {
	err := validate.IsAPIName(cfg.ServerName, false)
	if err != nil {
		return err
	}

	enabled, err := cowsqlmembership.Enabled(m.Gateway().Node())
	if err != nil {
		return err
	}

	if enabled {
		return domain.NewErrorf(domain.ErrOperationNotPermitted, "", "This server is already clustered")
	}

	err = config.ValidateClusterAddress(cfg.ClusterAddress)
	if err != nil {
		return err
	}

	err = config.UpdateNetworkClusterAddress(ctx, cfg.ClusterAddress)
	if err != nil {
		return err
	}

	localClusterAddress, err := m.gateway.Node().GetClusterAddress(ctx)
	if err != nil {
		return err
	}

	if localClusterAddress == "" {
		return errors.New("Cluster address must be set")
	}

	reverter := revert.New()
	defer reverter.Fail()

	// The cluster is reached through its first member until another address is configured.
	if config.GetNetwork().OperationsCenterAddress == "" {
		err = config.UpdateNetwork(ctx, apisystem.NetworkGlobalPut{OperationsCenterAddress: "https://" + localClusterAddress})
		if err != nil {
			return fmt.Errorf("Failed to set the address of Operations Center to the cluster address: %w", err)
		}

		reverter.Add(func() {
			err := config.UpdateNetwork(context.WithoutCancel(ctx), apisystem.NetworkGlobalPut{})
			if err != nil {
				slog.WarnContext(ctx, "Failed to reset the address of Operations Center", logger.Err(err))
			}
		})
	}

	revertServerCert, err := m.prepareServerCertificate(ctx, true)
	if err != nil {
		return err
	}

	reverter.Add(revertServerCert)

	// Ensure cluster certs are cleaned up as they are used for cluster status detection.
	reverter.Add(func() {
		for _, suffix := range []string{"ca", "crt", "key"} {
			_ = m.varRoot.Remove("cluster." + suffix)
		}
	})

	err = cowsqlmembership.Bootstrap(m.Gateway(), cfg.ServerName)
	if err != nil {
		return err
	}

	reverter.Success()

	// The cluster is bootstrapped, so the daemon restarts whatever happens now; the polls push the certificate to the servers missed here.
	err = m.system.PropagateCertificate(ctx)
	if err != nil {
		slog.WarnContext(ctx, "Failed to propagate the cluster certificate, the servers get it with their next poll", logger.Err(err))
	}

	return nil
}

func (m *membership) Gateway() cowsqlcluster.Gateway {
	return m.gateway
}

func (m *membership) NotifyUpgradeCompleted(ctx context.Context) error {
	notifier, err := m.gateway.NewNotifier(ctx, m.gateway.NetworkCert(), m.gateway.ServerCert(), cowsqlcluster.NotifyTryAll)
	if err != nil {
		return err
	}

	errs := notifier(func(ctx context.Context, address string, networkCert, serverCert cowsqltls.CertInfo) error {
		url := "https://" + address

		networkCertx509, err := networkCert.PublicKeyX509()
		if err != nil {
			return err
		}

		certInfo, ok := m.Gateway().ServerCert().(*incustls.CertInfo)
		if !ok {
			//domain-errors:internal The server certificate is always loaded by the daemon itself.
			return fmt.Errorf("Invalid certificate info: %T", serverCert)
		}

		c, err := client.New(url, client.WithClientCertificate(certInfo), client.WithTrustedServerCertificate(networkCertx509), client.WithDialTimeout(MemberDialTimeout))
		if err != nil {
			return err
		}

		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		req, err := http.NewRequestWithContext(ctx, http.MethodPatch, url+"/internal/database", nil)
		if err != nil {
			return err
		}

		cowsqlapi.SetCOWSQLVersionHeader(req)
		response, err := c.DoHTTP(req)
		if err != nil {
			return err
		}

		defer func() { _ = response.Body.Close() }()

		if response.StatusCode != http.StatusOK {
			//domain-errors:internal Unexpected response of another cluster member.
			return fmt.Errorf("database upgrade notification failed: %s", response.Status)
		}

		return nil
	})

	var notifyErrs []error
	for _, err := range errs {
		var opErr *net.OpError
		if errors.As(err, &opErr) {
			slog.DebugContext(ctx, "Skipping unreachable cluster member for database upgrade notification", logger.Err(err))
			continue
		}

		notifyErrs = append(notifyErrs, err)
	}

	return errors.Join(notifyErrs...)
}

// Stop stops the membership service. Further calls return the first result.
func (m *membership) Stop(ctx context.Context) error {
	m.stopOnce.Do(func() {
		m.stopErr = m.stop(ctx)
	})

	return m.stopErr
}

func (m *membership) stop(ctx context.Context) error {
	errs := m.stopLeaderTasks(ctx)

	// Close the database cleanly even if the deadline is used up.
	timeout := 5 * time.Second
	deadline, ok := ctx.Deadline()
	if ok && time.Until(deadline) > 0 {
		timeout = time.Until(deadline)
	}

	err := m.Gateway().Stop(timeout)
	if err != nil {
		errs = append(errs, fmt.Errorf("Failed to stop cluster database gateway: %w", err))
	}

	return errors.Join(errs...)
}

func (m *membership) Enable(ctx context.Context, clusterAddress string, configRepo ConfigRepo, globalRepo GlobalRaftRepo) error {
	m.globalRepo.Store(&globalRepo)
	m.gateway.SetClusterDB(globalRepo)
	m.configRepo.Store(&configRepo)

	err := m.DetermineNodeID(ctx, clusterAddress)
	if err != nil {
		return err
	}

	err = m.gateway.State().UpdateAuthenticator(ctx)
	if err != nil {
		slog.WarnContext(ctx, "Failed to initialize trusted cluster member certificate cache", logger.Err(err))
	}

	err = m.NotifyUpgradeCompleted(ctx)
	if err != nil {
		// Ignore the error, since it's not fatal for this particular
		// node. In most cases it just means that some nodes are
		// offline.
		slog.WarnContext(ctx, "Could not notify all nodes of database upgrade", logger.Err(err))
	}

	m.Gateway().SetHeartbeatOfflineThreshold(time.Duration(config.GetCluster().OfflineThresholdSeconds) * time.Second)

	return nil
}

// StartLeaderTasks runs the background tasks whenever this member is the cluster leader.
func (m *membership) StartLeaderTasks() {
	_, _ = task.Start(m.ctx, m.checkLeadership, task.Every(leadershipCheckInterval))
}

// checkLeadership starts or stops the background tasks according to the cluster leadership.
func (m *membership) checkLeadership(ctx context.Context) {
	isLeader, err := m.gateway.IsLeader(ctx)
	if err != nil {
		// Stop the tasks, so they don't run on two members.
		slog.WarnContext(ctx, "Failed to check for cluster leadership", logger.Err(err))
		isLeader = false
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.ctx.Err() != nil {
		return
	}

	if isLeader == m.leading {
		return
	}

	m.leading = isLeader

	if isLeader {
		slog.InfoContext(ctx, "Starting background tasks on the cluster leader")

		m.shutdownFuncs = make([]func(context.Context) error, 0, len(m.backgroundTasks))
		for _, taskFunc := range m.backgroundTasks {
			m.shutdownFuncs = append(m.shutdownFuncs, taskFunc(m.ctx))
		}

		return
	}

	slog.InfoContext(ctx, "Stopping background tasks, not the cluster leader anymore")

	m.stopBackgroundTasks(ctx)
}

// stopLeaderTasks stops the background tasks until the next restart.
func (m *membership) stopLeaderTasks(ctx context.Context) []error {
	// Cancel first, so the leadership check does not start the tasks again.
	m.cancel()

	m.mu.Lock()
	defer m.mu.Unlock()

	errs := m.stopBackgroundTasks(ctx)
	m.leading = false

	return errs
}

// stopBackgroundTasks stops the running background tasks in parallel. The caller must hold mu.
func (m *membership) stopBackgroundTasks(ctx context.Context) []error {
	var errsMu sync.Mutex
	errs := make([]error, 0, len(m.shutdownFuncs))

	var wg sync.WaitGroup
	for _, shutdown := range m.shutdownFuncs {
		wg.Go(func() {
			err := shutdown(ctx)
			if err != nil {
				slog.ErrorContext(ctx, "Failed to stop background task", logger.Err(err))

				errsMu.Lock()
				errs = append(errs, err)
				errsMu.Unlock()
			}
		})
	}

	wg.Wait()

	m.shutdownFuncs = nil

	return errs
}

func (m *membership) SetGlobalDB(globalDB GlobalRaftRepo) {
	m.globalRepo.Store(&globalDB)
	m.gateway.SetClusterDB(globalDB)
}

func (m *membership) DetermineNodeID(ctx context.Context, address string) error {
	return m.global().DetermineNodeID(ctx, address)
}

func (m *membership) HandlerFuncs() map[string]http.HandlerFunc {
	return m.Gateway().HandlerFuncs(m.gatewayAuthenticator())
}

// PersistSharedConfig stores the section changed by change in the global database.
// The other sections are kept as stored.
func (m *membership) PersistSharedConfig(ctx context.Context, change config.SharedChange) (config.SharedConfig, error) {
	var merged config.SharedConfig
	err := transaction.ForceTx(ctx, m.global(), func(ctx context.Context, _ transaction.TX) error {
		stored, err := m.sharedConfig().GetSharedConfig(ctx)
		if err != nil {
			return err
		}

		merged = change.Apply(*stored)

		return m.sharedConfig().UpdateSharedConfig(ctx, merged)
	})
	if err != nil {
		return config.SharedConfig{}, err
	}

	return merged, nil
}
