package cluster

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"
	"time"

	cowsqlcluster "github.com/cowsql/go-cowsql/cluster"
	"github.com/cowsql/go-cowsql/cluster/db/transaction"
	cowsqlgateway "github.com/cowsql/go-cowsql/cluster/gateway"
	cowsqloptions "github.com/cowsql/go-cowsql/cluster/options"
	cowsqltls "github.com/cowsql/go-cowsql/cluster/tls"
	incustls "github.com/lxc/incus/v7/shared/tls"

	"github.com/FuturFusion/operations-center/internal/api/listener"
	config "github.com/FuturFusion/operations-center/internal/config/daemon"
	"github.com/FuturFusion/operations-center/internal/util/logger"
	"github.com/FuturFusion/operations-center/internal/util/task"
	"github.com/FuturFusion/operations-center/internal/version"
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

	// serverCert is used for the connections to the database.
	serverCert atomic.Pointer[incustls.CertInfo]

	// The background tasks run on the cluster leader only.
	backgroundTasks []task.TaskFunc

	shutdownFuncs []func(context.Context) error
	leading       bool
}

// leadershipCheckInterval is the interval of the cluster leadership check.
const leadershipCheckInterval = 5 * time.Second

func NewClusterMembershipService(ctx context.Context, varRoot *os.Root, listeners *listener.ListenerSet, localRepo LocalRaftRepo, clusterCert *incustls.CertInfo, serverCert *incustls.CertInfo, backgroundTasks []task.TaskFunc) (MembershipService, error) {
	ctx, cancel := context.WithCancel(ctx)

	m := &membership{
		ctx:             ctx,
		cancel:          cancel,
		backgroundTasks: backgroundTasks,
	}

	m.serverCert.Store(serverCert)

	g, err := cowsqlgateway.NewGateway(
		context.TODO(),
		localRepo,
		clusterCert,
		func() cowsqltls.CertInfo { return m.serverCert.Load() },
		newState(varRoot, listeners),
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

func (m *membership) Gateway() cowsqlcluster.Gateway {
	return m.gateway
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

	return m.DetermineNodeID(ctx, clusterAddress)
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
