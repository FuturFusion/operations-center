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

	// rolesMu serializes the changes of the raft roles and members on the leader.
	rolesMu sync.Mutex

	// postCertificate replaces the request applying a cluster certificate on another member in tests.
	postCertificate func(ctx context.Context, address string, trusted *x509.Certificate, certificate apisystem.CertificatePost) error

	// reloadConfigOnMember replaces the request letting another member reload the shared config in tests.
	reloadConfigOnMember func(ctx context.Context, address string) error

	// removalChecks refuse the removal of a member, which is not forced.
	removalChecks []RemovalCheck

	stopOnce sync.Once
	stopErr  error

	// resetPending removes the database on shutdown.
	resetPending atomic.Bool

	localRepo LocalRaftRepo

	// The repositories of the global database are set once it is open, while requests and heartbeats already read them.
	globalRepo atomic.Pointer[GlobalRaftRepo]
	tokenRepo  atomic.Pointer[JoinTokenRepo]
	configRepo atomic.Pointer[ConfigRepo]

	// joinCheck refuses a join, which is not forced, while this server still manages something.
	joinCheck    func(ctx context.Context) error
	lastNodeList atomic.Pointer[cowsqlheartbeat.APIHeartbeat]

	clientCertMu    sync.RWMutex
	clientCertCache map[string]x509.Certificate

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
	backgroundTasks []LeaderTask

	// taskStatus records the leader work of this member for the next leader.
	taskStatus leaderTaskStatus

	// busyMemberTasks returns the addresses of the other members still running or stopping each leader task, by task name.
	busyMemberTasks func(ctx context.Context) (map[string][]string, error)

	handoverTimeout  time.Duration
	handoverInterval time.Duration
	handover         sync.WaitGroup

	warnings      warning.WarningEmitter
	shutdownFuncs []func(context.Context) error
	leading       bool

	termMu  sync.Mutex
	term    context.Context
	endTerm context.CancelFunc
}

// leadershipCheckInterval is the interval of the cluster leadership check.
const leadershipCheckInterval = 5 * time.Second

// leaderHandoverTimeout is how long a new leader waits for the other members to stop a leader task, before it starts it anyway.
const leaderHandoverTimeout = 30 * time.Second

// leaderHandoverInterval is the interval in which a new leader asks the other members about their leader tasks.
const leaderHandoverInterval = time.Second

// leaderHandoverScope is the scope of the warning about a leader task started while another member still ran it.
var leaderHandoverScope = api.WarningScope{Scope: "leader_handover", EntityType: "system", Entity: "operations-center"}

// Option configures the membership service.
type Option func(m *membership)

// WithJoinCheck sets the check refusing a join of this server, which is not forced, while it still manages something.
func WithJoinCheck(check func(ctx context.Context) error) Option {
	return func(m *membership) {
		m.joinCheck = check
	}
}

// RemovalCheck refuses the removal of the cluster member name, unless it is forced.
type RemovalCheck func(ctx context.Context, name string) error

// WithRemovalChecks adds checks run right before a member is removed, which refuse the removal with an error.
func WithRemovalChecks(checks ...RemovalCheck) Option {
	return func(m *membership) {
		m.removalChecks = append(m.removalChecks, checks...)
	}
}

func NewClusterMembershipService(ctx context.Context, env environment, varRoot *os.Root, systemSvc system.SystemService, listeners *listener.ListenerSet, localRepo LocalRaftRepo, clusterCert *incustls.CertInfo, serverCert *incustls.CertInfo, backgroundTasks []LeaderTask, warnings warning.WarningEmitter, opts ...Option) (MembershipService, error) {
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

	for _, opt := range opts {
		opt(m)
	}

	m.serverCert.Store(serverCert)
	m.busyMemberTasks = m.busyLeaderTasks

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

	g.SetHeartbeatNodeHook(func(heartbeatData *cowsqlheartbeat.APIHeartbeat, isLeader bool, unavailableMembers []string) {
		m.onClusterHeartbeat(ctx, g, localRepo, m.global(), heartbeatData, isLeader, unavailableMembers)
	})

	lifecycle.SharedClusterUpdateSignal.AddListener(func(_ context.Context, cfg apisystem.Cluster) {
		g.SetHeartbeatOfflineThreshold(time.Duration(cfg.OfflineThresholdSeconds) * time.Second)
	})

	m.localRepo = localRepo
	m.gateway = g

	m.updateCertificateCacheFromLocal(ctx)

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

// tokens returns the repository of the join tokens, nil until the global database is open.
func (m *membership) tokens() JoinTokenRepo {
	repo := m.tokenRepo.Load()
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

func (m *membership) GetMembers(ctx context.Context) ([]api.ClusterMember, error) {
	localMembers, err := m.localRepo.GetRaftNodes(ctx)
	if err != nil {
		return nil, err
	}

	localMembersByAddress := make(map[string]cowsqldb.RaftNode, len(localMembers))
	for _, m := range localMembers {
		localMembersByAddress[m.Address] = m
	}

	leaderAddress, err := m.LeaderAddress(ctx)
	if err != nil {
		return nil, err
	}

	members, err := m.global().GetNodes(ctx)
	if err != nil {
		return nil, err
	}

	offlineThreshold := m.gateway.HeartbeatOfflineThreshold()

	apiMembers := make([]api.ClusterMember, 0, len(members))
	for _, member := range members {
		localMember, ok := localMembersByAddress[member.Address]
		if !ok {
			continue
		}

		isLeader := leaderAddress == member.Address

		apiMembers = append(apiMembers, api.ClusterMember{
			Name:          member.Name,
			Address:       member.Address,
			LastHeartbeat: member.Heartbeat,
			Roles:         []string{localMember.Role.String()},
			Leader:        isLeader,
			// The leader is online without heartbeats.
			Offline: !isLeader && member.IsOffline(offlineThreshold),
		})
	}

	return apiMembers, nil
}

// LocalMemberID returns the ID of this member in the global database.
func (m *membership) LocalMemberID() (int64, error) {
	if m.global() == nil {
		return 0, errors.New("Global database is not available")
	}

	return m.global().GetNodeID(), nil
}

// GetMembersByID returns the cluster members by their ID from the global database.
func (m *membership) GetMembersByID(ctx context.Context) (map[int64]cowsqldb.NodeInfo, error) {
	if m.global() == nil {
		return nil, errors.New("Global database is not available")
	}

	members, err := m.global().GetNodes(ctx)
	if err != nil {
		return nil, err
	}

	membersByID := make(map[int64]cowsqldb.NodeInfo, len(members))
	for _, member := range members {
		membersByID[member.ID] = member
	}

	return membersByID, nil
}

// HeartbeatOfflineThreshold returns the time after which a member without heartbeat is offline.
func (m *membership) HeartbeatOfflineThreshold() time.Duration {
	return m.gateway.HeartbeatOfflineThreshold()
}

// Auth implements Auther to trust a joiner by the fingerprint /:trust recorded with its token, which any leader reads.
func (m *membership) Auth(w http.ResponseWriter, r *http.Request) (trusted bool, username string, protocol string, _ error) {
	err := m.isLeader(r.Context())
	if err != nil {
		return false, "", "", fmt.Errorf("Rejecting attempt to establish trust with non-leader: %w", err)
	}

	tokens, err := m.joinTokens(r.Context())
	if err != nil {
		return false, "", "", fmt.Errorf("Failed to get the join tokens: %w", err)
	}

	fingerprints := make([]string, 0, len(tokens))
	for _, token := range tokens {
		if token.JoinerFingerprint != "" {
			fingerprints = append(fingerprints, token.JoinerFingerprint)
		}
	}

	return authtls.New(func() []string { return fingerprints }).Auth(w, r)
}

// CreateToken implements [MembershipService].
func (m *membership) CreateToken(ctx context.Context, req api.ClusterTokenPost) error {
	expiry := config.DefaultMembershipJoinTokenExpiry
	if req.Expiry != "" {
		var err error
		expiry, err = time.ParseDuration(req.Expiry)
		if err != nil {
			return domain.NewValidationErrf("Invalid token expiry %q: %v", req.Expiry, err)
		}
	}

	// An expired token would be deleted before it can be used.
	if expiry <= 0 {
		return domain.NewValidationErrf("Token expiry %q must be positive", req.Expiry)
	}

	enabled, err := cowsqlmembership.Enabled(m.localRepo)
	if err != nil {
		return err
	}

	if !enabled {
		return domain.NewErrorf(domain.ErrOperationNotPermitted, "", "This server is not clustered")
	}

	err = validate.IsAPIName(req.ServerName, false)
	if err != nil {
		return fmt.Errorf("Invalid server name for token: %w", err)
	}

	buf := make([]byte, 32)
	n, err := rand.Read(buf)
	if err != nil {
		return err
	}

	if n != len(buf) {
		return errors.New("Not enough random bytes read")
	}

	token := entities.ClusterJoinToken{
		ServerName:         req.ServerName,
		ClusterFingerprint: m.gateway.NetworkCert().Fingerprint(),
		ClusterAddresses:   []string{},
		Secret:             hex.EncodeToString(buf),
		ExpirationDate:     time.Now().UTC().Add(expiry),
	}

	var raftNodes []cowsqldb.RaftNode
	err = transaction.Do(ctx, m.gateway.Node(), func(ctx context.Context) error {
		tx := m.Gateway().Node()
		var err error
		raftNodes, err = tx.GetRaftNodes(ctx)
		return err
	})
	if err != nil {
		return err
	}

	return transaction.Do(ctx, m.Gateway().Cluster(), func(ctx context.Context) error {
		tx := m.Gateway().Cluster()

		nodes, err := tx.GetNodes(ctx)
		if err != nil {
			return err
		}

		raftNodesByAddress := make(map[string]cowsqldb.RaftNode, len(raftNodes))
		for _, n := range raftNodes {
			raftNodesByAddress[n.Address] = n
		}

		for _, n := range nodes {
			if n.Name == token.ServerName {
				return domain.NewErrorf(domain.ErrConstraintViolation, "", "Cluster member %q already exists", n.Name)
			}

			raftNode, ok := raftNodesByAddress[n.Address]
			if !ok || raftNode.Role != cowsqldb.RaftVoter {
				continue
			}

			if n.IsOffline(m.gateway.HeartbeatOfflineThreshold()) {
				continue
			}

			token.ClusterAddresses = append(token.ClusterAddresses, n.Address)
		}

		if len(token.ClusterAddresses) == 0 {
			return domain.NewErrorf(domain.ErrOperationNotPermitted, "", "No cluster member is available to include in join token")
		}

		return m.tokens().CreateToken(ctx, token)
	})
}

// DeleteTokenByName implements [MembershipService].
func (m *membership) DeleteTokenByName(ctx context.Context, serverName string) error {
	enabled, err := cowsqlmembership.Enabled(m.localRepo)
	if err != nil {
		return err
	}

	if !enabled {
		return domain.NewErrorf(domain.ErrOperationNotPermitted, "", "This server is not clustered")
	}

	return m.tokens().DeleteTokenByName(ctx, serverName)
}

// GetTokenByServerName implements [MembershipService].
func (m *membership) GetTokenByServerName(ctx context.Context, serverName string) (*entities.ClusterJoinToken, error) {
	enabled, err := cowsqlmembership.Enabled(m.localRepo)
	if err != nil {
		return nil, err
	}

	if !enabled {
		return nil, domain.NewErrorf(domain.ErrOperationNotPermitted, "", "This server is not clustered")
	}

	return m.tokens().GetTokenByServerName(ctx, serverName)
}

// GetTokens implements [MembershipService].
func (m *membership) GetTokens(ctx context.Context) (entities.ClusterJoinTokens, error) {
	enabled, err := cowsqlmembership.Enabled(m.localRepo)
	if err != nil {
		return nil, err
	}

	if !enabled {
		return nil, domain.NewErrorf(domain.ErrOperationNotPermitted, "", "This server is not clustered")
	}

	return m.tokens().GetTokens(ctx)
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

func (m *membership) Accept(ctx context.Context, req api.ClusterAcceptPost) (*membershipapi.InternalClusterAcceptResponse, error) {
	err := m.isLeader(ctx)
	if err != nil {
		return nil, err
	}

	// Adding the joiner waits for other role changes, like a rebalance or handover.
	m.rolesMu.Lock()
	defer m.rolesMu.Unlock()

	nodes, err := cowsqlmembership.Accept(m.Gateway(), req.Certificate.Certificate, req.ServerName, req.Address, req.SchemaVersion, req.APIVersion, req.Architecture)
	if err != nil {
		return nil, err
	}

	certsByAddress, err := m.global().GetCertificates(ctx)
	if err != nil {
		return nil, err
	}

	apiCerts := make([]api.ServerCertificate, 0, len(nodes))
	for _, n := range nodes {
		cert, ok := certsByAddress[n.Address]
		if ok && n.Address != "" && n.Name != "" {
			apiCerts = append(apiCerts, api.ServerCertificate{ServerName: n.Name, Certificate: cert})
		}
	}

	// The token is used up now that Accept has globally trusted the joiner.
	err = m.DeleteTokenByName(ctx, req.ServerName)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return nil, fmt.Errorf("Failed to delete the used join token: %w", err)
	}

	m.updateCertificateCacheFromGlobal(ctx)

	// The other members learn the certificate of the joiner with the next heartbeat, which may come too late if the leader goes down first.
	// go-cowsql's Accept restarts a heartbeat round only while one is running, so one is started otherwise.
	if !m.gateway.HeartbeatRestart() {
		heartbeatCtx := m.ctx
		if heartbeatCtx == nil {
			heartbeatCtx = context.Background()
		}

		go m.gateway.Heartbeat(heartbeatCtx, cowsqlheartbeat.HeartbeatImmediate)
	}

	clientCertificate, clientKey, err := securitytls.LoadClientKeypair(m.varRoot)
	if err != nil {
		return nil, err
	}

	resp := membershipapi.InternalClusterAcceptResponse{
		LeaderAddress:      m.Gateway().State().ClusterAddress(),
		RaftNodes:          make([]membershipapi.InternalRaftNode, 0, len(nodes)),
		PublicKey:          string(m.Gateway().NetworkCert().PublicKey()),
		PrivateKey:         string(m.Gateway().NetworkCert().PrivateKey()),
		ServerCertificates: apiCerts,
		ClientCertificate:  clientCertificate,
		ClientKey:          clientKey,
	}

	for _, n := range nodes {
		resp.RaftNodes = append(resp.RaftNodes, membershipapi.InternalRaftNode{ID: n.ID, Address: n.Address, Role: int(n.Role)})
	}

	return &resp, nil
}

// RemoveMember removes the named cluster member and, unless force is set, tries to reset it to a standalone server.
func (m *membership) RemoveMember(ctx context.Context, name string, force bool) error {
	err := m.isLeader(ctx)
	if err != nil {
		return err
	}

	pending := true
	member, err := m.global().GetNodeByName(ctx, name, true)
	if api.StatusErrorCheck(err, http.StatusNotFound) {
		pending = false
		member, err = m.global().GetNodeByName(ctx, name, false)
	}

	if api.StatusErrorCheck(err, http.StatusNotFound) {
		return domain.NewErrorf(domain.ErrNotFound, "", "Cluster member %q not found", name)
	}

	if err != nil {
		return err
	}

	if !pending && member.Address == m.Gateway().State().ClusterAddress() {
		return m.removeLeader(ctx, name, force)
	}

	address, err := m.leave(ctx, name, force, pending)
	if err != nil {
		return err
	}

	if force {
		return nil
	}

	c, err := m.MemberClient(address)
	if err != nil {
		slog.WarnContext(ctx, "Failed to reset the removed cluster member", slog.String("name", name), logger.Err(err))
		return nil
	}

	// The member is already removed from the cluster, an unreachable member is just not reset.
	_, err = c.DoInternalRequest(ctx, http.MethodPost, "/membership/:reset", nil, nil)

	// A member, which answered, is still clustered on its own and needs attention.
	var serverErr *client.ServerError
	if errors.As(err, &serverErr) {
		return fmt.Errorf("Removed cluster member %q failed to reset itself to a standalone server: %w", name, err)
	}

	if err != nil {
		slog.WarnContext(ctx, "Failed to reset the removed cluster member", slog.String("name", name), logger.Err(err))
	}

	return nil
}

// leaveAttempts and leaveRetryWait bound the retries of a removal while raft takes another configuration change.
const (
	leaveAttempts  = 10
	leaveRetryWait = time.Second
)

// leave removes the member name from the raft cluster and the database and rebalances the roles.
// It holds the lock of the role changes, so no join, handover or rebalance runs meanwhile, and runs the removal checks under it.
func (m *membership) leave(ctx context.Context, name string, force bool, pending bool) (string, error) {
	m.rolesMu.Lock()
	defer m.rolesMu.Unlock()

	// The checks run right before the removal, so files assigned to the member meanwhile are seen.
	if !force {
		for _, check := range m.removalChecks {
			err := check(ctx, name)
			if err != nil {
				return "", err
			}
		}
	}

	var err error

	// The heartbeat changes the raft roles of the members on its own, and raft takes one configuration change at a time.
	var address string
	for attempt := 1; ; attempt++ {
		address, err = cowsqlmembership.Leave[struct{}](m.gateway, name, force, pending)
		if err == nil || !cowsqlmembership.IsClusterBusyErr(err) {
			break
		}

		if attempt == leaveAttempts {
			return "", domain.NewErrorf(domain.ErrUnavailable, "", "Another change of the cluster members is in progress").WithCause(err).WithHintf("Retry the removal.")
		}

		select {
		case <-ctx.Done():
			return "", ctx.Err()

		case <-time.After(leaveRetryWait):
		}
	}

	if err != nil {
		return "", err
	}

	err = cowsqlmembership.Purge[struct{}](m.gateway, name, pending)
	if err != nil {
		return "", fmt.Errorf("Failed to remove member from database: %w", err)
	}

	m.updateCertificateCacheFromGlobal(ctx)

	err = m.rebalance(ctx)
	if err != nil {
		slog.WarnContext(ctx, "Failed to rebalance cluster roles after removing a member", slog.String("name", name), logger.Err(err))
	}

	return address, nil
}

// removeLeader transfers the leadership of this member and lets the new leader remove it.
func (m *membership) removeLeader(ctx context.Context, name string, force bool) error {
	err := m.transferLeadership(ctx, name)
	if err != nil {
		return err
	}

	leader, err := m.Gateway().LeaderAddress()
	if err != nil {
		return err
	}

	c, err := m.MemberClient(leader)
	if err != nil {
		return err
	}

	query := url.Values{}
	if force {
		query.Set("force", "1")
	}

	_, err = c.DoRequest(ctx, http.MethodDelete, "/membership/members/"+url.PathEscape(name), query, nil)

	return err
}

// Reset resets this member to an empty standalone server after it has been removed from the cluster.
func (m *membership) Reset(ctx context.Context) {
	m.resetToStandalone(ctx)
}

// errTokensNotReady reports the join tokens as not available before the global database is open.
var errTokensNotReady = domain.NewErrorf(domain.ErrOperationNotPermitted, "", "Cluster database is still initializing")

// joinTokens returns the valid join tokens of the leader, which the joiners present.
func (m *membership) joinTokens(ctx context.Context) ([]entities.ClusterJoinToken, error) {
	if m.tokens() == nil {
		return nil, errTokensNotReady
	}

	return m.tokens().GetTokens(ctx)
}

// joinToken returns the valid join token of serverName.
func (m *membership) joinToken(ctx context.Context, serverName string) (*entities.ClusterJoinToken, error) {
	if m.tokens() == nil {
		return nil, errTokensNotReady
	}

	return m.tokens().GetTokenByServerName(ctx, serverName)
}

func (m *membership) Trust(ctx context.Context, token string, fingerprint string) error {
	err := m.isLeader(ctx)
	if err != nil {
		return err
	}

	tokenObj, err := api.DecodeJoinToken(token)
	if err != nil {
		return domain.NewValidationErrf("Invalid join token: %v", err)
	}

	if fingerprint == "" {
		return errors.New("Failed to record joiner certificate fingerprint")
	}

	// An unknown name and a wrong secret are answered alike, so the names of the pending members stay unknown.
	unknown := domain.NewErrorf(domain.ErrNotAuthorized, "", "Unknown join token")

	dbToken, err := m.joinToken(ctx, tokenObj.ServerName)
	if errors.Is(err, domain.ErrNotFound) {
		return unknown
	}

	if err != nil {
		return err
	}

	// The token is only trusted for a joiner presenting its secret.
	if subtle.ConstantTimeCompare([]byte(dbToken.Secret), []byte(tokenObj.Secret)) != 1 {
		return unknown
	}

	// The fingerprint is stored with the token, so any leader accepts the joiner, and the token is used up by the accept.
	return m.tokens().SetJoinerFingerprint(ctx, dbToken.ServerName, fingerprint)
}

func (m *membership) Join(ctx context.Context, req api.ClusterJoinPost) (err error) {
	err = validate.IsAPIName(req.ServerName, false)
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

	// The join replaces the data of this server, which a server still managing something refuses unless forced.
	if !req.Force && m.joinCheck != nil {
		err = m.joinCheck(ctx)
		if err != nil {
			return err
		}
	}

	err = config.ValidateClusterAddress(req.ClusterAddress)
	if err != nil {
		return err
	}

	err = config.UpdateNetworkClusterAddress(ctx, req.ClusterAddress)
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

	tokenObj, err := api.DecodeJoinToken(req.Token)
	if err != nil {
		return domain.NewValidationErrf("Invalid join token: %v", err)
	}

	var clusterCert *x509.Certificate
	var reachableAddress string
	for _, address := range tokenObj.ClusterAddresses {
		insecureClient, err := client.New("https://"+address, client.WithInsecureSkipVerify(), client.WithDialTimeout(MemberDialTimeout))
		if err != nil {
			return err
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://"+address, nil)
		if err != nil {
			return err
		}

		resp, err := insecureClient.DoHTTP(req)
		if err != nil {
			slog.WarnContext(ctx, "Could not connect to cluster member, trying next", slog.String("address", address), logger.Err(err))
			continue
		}

		_ = resp.Body.Close()

		// Only the leaf certificate is proven by the TLS handshake, the rest of the chain is unverified.
		if resp.TLS == nil || len(resp.TLS.PeerCertificates) == 0 || incustls.CertFingerprint(resp.TLS.PeerCertificates[0]) != tokenObj.ClusterFingerprint {
			return domain.NewErrorf(domain.ErrInvalidArgument, "", "Cluster member %q did not present the certificate of the join token", address)
		}

		clusterCert = resp.TLS.PeerCertificates[0]
		reachableAddress = address
		break
	}

	if reachableAddress == "" || clusterCert == nil {
		return errors.New("Failed to connect to any cluster member address")
	}

	reverter := revert.New()
	defer reverter.Fail()

	// Replace the server certificate before joining.
	revertServerCert, err := m.prepareServerCertificate(ctx, false)
	if err != nil {
		return err
	}

	reverter.Add(revertServerCert)

	certInfo, ok := m.Gateway().ServerCert().(*incustls.CertInfo)
	if !ok {
		//domain-errors:internal The server certificate is always loaded by the daemon itself.
		return fmt.Errorf("Invalid certificate info: %T", m.Gateway().ServerCert())
	}

	c, err := client.New("https://"+reachableAddress, client.WithClientCertificate(certInfo), client.WithTrustedServerCertificate(clusterCert), client.WithDialTimeout(MemberDialTimeout))
	if err != nil {
		return err
	}

	resp, err := c.DoInternalRequest(ctx, http.MethodPost, "/membership/:trust", nil, membershipapi.InternalClusterPostTrustRequest{Token: req.Token})
	if err != nil {
		return fmt.Errorf("Failed to connect to cluster trust endpoint: %w", err)
	}

	if resp.Code != 0 {
		//domain-errors:internal Unexpected response of another cluster member.
		return fmt.Errorf("Unexpected status code from trust endpoint %d: %q", resp.Code, resp.Error)
	}

	serverCertx509, err := m.Gateway().ServerCert().PublicKeyX509()
	if err != nil {
		return err
	}

	arch, err := osarch.ArchitectureGetLocalID()
	if err != nil {
		return err
	}

	// FIXME: api extensions
	post := api.ClusterAcceptPost{
		ServerName:    req.ServerName,
		Address:       m.Gateway().State().ClusterAddress(),
		SchemaVersion: m.global().SchemaVersion(),
		APIVersion:    0,
		Architecture:  arch,
		Certificate:   api.Certificate{Certificate: serverCertx509},
	}

	b, err := json.Marshal(post)
	if err != nil {
		return err
	}

	resp, err = c.DoInternalRequest(ctx, http.MethodPost, "/membership/:accept", nil, b)
	if err != nil {
		return fmt.Errorf("Failed to connect to cluster accept endpoint: %w", err)
	}

	if resp.Code != 0 {
		//domain-errors:internal Unexpected response of another cluster member.
		return fmt.Errorf("Unexpected status code from accept endpoint %d: %q", resp.Code, resp.Error)
	}

	// Remove the pending member from the leader on failure.
	leaderAddress := reachableAddress
	reverter.Add(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()

		leader, err := client.New("https://"+leaderAddress, client.WithClientCertificate(certInfo), client.WithTrustedServerCertificate(clusterCert), client.WithDialTimeout(MemberDialTimeout))
		if err == nil {
			_, err = leader.DoRequest(ctx, http.MethodDelete, "/membership/members/"+url.PathEscape(req.ServerName), url.Values{"force": []string{"1"}}, nil)
		}

		if err != nil {
			slog.ErrorContext(ctx, "Failed to remove pending cluster member after failed join", logger.Err(err))
		}
	})

	var cfg membershipapi.InternalClusterAcceptResponse
	err = json.Unmarshal(resp.Metadata, &cfg)
	if err != nil {
		return err
	}

	if cfg.LeaderAddress != "" {
		leaderAddress = cfg.LeaderAddress
	}

	nodes := make([]cowsqldb.RaftNode, 0, len(cfg.RaftNodes))
	for _, r := range cfg.RaftNodes {
		nodes = append(nodes, cowsqldb.RaftNode{ID: r.ID, Address: r.Address, Role: cowsqlclient.NodeRole(r.Role), Name: r.Name})
	}

	dbCerts := make([]api.Certificate, 0, len(cfg.ServerCertificates))
	for _, c := range cfg.ServerCertificates {
		dbCerts = append(dbCerts, c.Certificate)
	}

	m.setCertificateCache(dbCerts)

	networkCert, err := tls.X509KeyPair([]byte(cfg.PublicKey), []byte(cfg.PrivateKey))
	if err != nil {
		return err
	}

	// Reset this member to an empty standalone server on failure.
	reverter.Add(func() {
		m.resetToStandalone(context.WithoutCancel(ctx))
		err = fmt.Errorf("%w (%w)", err, ErrJoinReset)
	})

	// Stop the background tasks before the files and the database are replaced.
	m.stopLeaderTasks(ctx)

	// Remove the stored files of the local database.
	err = memberfiles.DeleteStores(m.varRoot)
	if err != nil {
		return fmt.Errorf("Failed to remove the files of the local database: %w", err)
	}

	err = m.applyClientKeypair(ctx, cfg.ClientCertificate, cfg.ClientKey)
	if err != nil {
		return err
	}

	err = cowsqlmembership.Join[struct{}](m.Gateway(), networkCert, req.ServerName, nodes)
	if err != nil {
		return err
	}

	err = m.DetermineNodeID(ctx, m.Gateway().State().ClusterAddress())
	if err != nil {
		return err
	}

	// On failure the leader assigns the role with a later heartbeat.
	resp, err = c.DoInternalRequest(ctx, http.MethodPost, "/membership/:rebalance", nil, nil)
	if err == nil && resp.Code != 0 {
		//domain-errors:internal Unexpected response of another cluster member.
		err = fmt.Errorf("Unexpected status code from rebalance endpoint %d: %q", resp.Code, resp.Error)
	}

	if err != nil {
		slog.WarnContext(ctx, "Failed to trigger the rebalance of the cluster roles", logger.Err(err))
	}

	reverter.Success()

	return nil
}

// MemberDialTimeout limits the time to connect to another cluster member.
const MemberDialTimeout = 10 * time.Second

// ErrJoinReset reports a failed join after which this member has been reset to an empty standalone server.
var ErrJoinReset = errors.New("this server has been reset to an empty standalone server")

// resetToStandalone resets this member to an empty standalone server.
func (m *membership) resetToStandalone(ctx context.Context) {
	// The marker lets the next start complete the reset, if the daemon dies before its shutdown removes the database.
	err := m.varRoot.WriteFile(resetPendingFilename, nil, 0o600)
	if err != nil {
		slog.ErrorContext(ctx, "Failed to mark the reset to a standalone server", logger.Err(err))
	}

	// Stop trusting the other members, so a late heartbeat does not restore the raft members.
	m.setCertificateCache(nil)

	err = memberfiles.DeleteStores(m.varRoot)
	if err != nil {
		slog.ErrorContext(ctx, "Failed to remove the files of the cluster database", logger.Err(err))
	}

	// Replace the client keypair of the cluster.
	err = m.generateClientKeypair(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "Failed to replace the client keypair of the cluster", logger.Err(err))
	}

	for _, suffix := range []string{"ca", "crt", "key"} {
		err := m.varRoot.Remove("cluster." + suffix)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			slog.ErrorContext(ctx, "Failed to remove cluster certificate", logger.Err(err))
		}
	}

	// Remove the database on shutdown, since another member could still restore the raft members.
	m.resetPending.Store(true)
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

// HandOverRole stops the leader tasks and hands over the role of this member to another one.
func (m *membership) HandOverRole(ctx context.Context) error {
	errs := m.stopLeaderTasks(ctx)

	// Hand over the role unless this member is being reset.
	if m.global() != nil && !m.resetPending.Load() {
		err := m.handoverRole(ctx)
		if err != nil {
			errs = append(errs, fmt.Errorf("Failed to hand over cluster member role: %w", err))
		}
	}

	return errors.Join(errs...)
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

	// Remove the database, so the restart begins as a standalone server.
	if m.resetPending.Load() {
		err = m.varRoot.RemoveAll("database")
		if err != nil {
			errs = append(errs, fmt.Errorf("Failed to remove the database of the reset member: %w", err))
		} else {
			err = m.varRoot.Remove(resetPendingFilename)
			if err != nil && !errors.Is(err, fs.ErrNotExist) {
				errs = append(errs, fmt.Errorf("Failed to remove the reset marker: %w", err))
			}
		}
	}

	return errors.Join(errs...)
}

func (m *membership) Enable(ctx context.Context, clusterAddress string, tokenRepo JoinTokenRepo, configRepo ConfigRepo, globalRepo GlobalRaftRepo) error {
	m.globalRepo.Store(&globalRepo)
	m.gateway.SetClusterDB(globalRepo)
	m.tokenRepo.Store(&tokenRepo)
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
		// The leadership is unknown, so the tasks keep their state: every run checks the leadership itself.
		slog.WarnContext(ctx, "Failed to check for cluster leadership", logger.Err(err))
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.ctx.Err() != nil {
		return
	}

	// A leader whose term ended without the leadership moving starts a new one.
	if isLeader == m.leading && (!isLeader || m.termRunning()) {
		return
	}

	m.leading = isLeader

	if isLeader {
		slog.InfoContext(ctx, "Starting background tasks on the cluster leader, once the other members stopped them")

		term, endTerm := context.WithCancel(m.ctx)
		m.termMu.Lock()
		m.term, m.endTerm = term, endTerm
		m.termMu.Unlock()

		m.shutdownFuncs = make([]func(context.Context) error, 0, len(m.backgroundTasks))

		// The handover runs beside the leadership check, so the check keeps noticing a leadership change meanwhile.
		m.handover.Go(func() { m.handOverTasks(term) })

		return
	}

	slog.InfoContext(ctx, "Stopping background tasks, not the cluster leader anymore")

	m.stopBackgroundTasks(ctx)
}

// termRunning reports whether the leader tasks have a term, which has not ended.
func (m *membership) termRunning() bool {
	m.termMu.Lock()
	defer m.termMu.Unlock()

	return m.term != nil && m.term.Err() == nil
}

// LeaderContext returns a context for work of the leader task name, ended when this member stops running the leader tasks, and the function marking the work done.
// It returns false, if the task is not running on this member.
func (m *membership) LeaderContext(name string) (context.Context, func(), bool) {
	m.termMu.Lock()
	term := m.term
	m.termMu.Unlock()

	if term == nil || term.Err() != nil {
		return nil, nil, false
	}

	done, ok := m.taskStatus.begin(name)
	if !ok {
		return nil, nil, false
	}

	return term, done, true
}

// TaskStatus returns the state of the leader tasks of this member.
func (m *membership) TaskStatus() membershipapi.InternalClusterTaskStatus {
	names := make([]string, 0, len(m.backgroundTasks))
	for _, t := range m.backgroundTasks {
		names = append(names, t.Name)
	}

	return membershipapi.InternalClusterTaskStatus{
		Address: m.gateway.State().ClusterAddress(),
		Tasks:   m.taskStatus.list(names),
	}
}

// handOverTasks starts each leader task of the term, once no other member runs or stops it anymore, or once the handover timed out.
func (m *membership) handOverTasks(term context.Context) {
	pending := slices.Clone(m.backgroundTasks)
	deadline := time.Now().Add(m.handoverTimeout)
	busyAtTimeout := map[string][]string{}

	for len(pending) > 0 {
		// The leadership can move again while the other members stop their tasks.
		isLeader, err := m.gateway.IsLeader(term)
		if term.Err() != nil {
			return
		}

		if err == nil && !isLeader {
			m.checkLeadership(m.ctx)
			return
		}

		// While the leadership is unknown, no task is started and the round is repeated.
		var busy map[string][]string
		if err != nil {
			slog.WarnContext(term, "Failed to check for cluster leadership during the handover of the leader tasks", logger.Err(err))
		} else {
			busy, err = m.busyMemberTasks(term)
			if err != nil {
				slog.WarnContext(term, "Failed to get the leader tasks of the other cluster members", logger.Err(err))
			}
		}

		timedOut := !time.Now().Before(deadline)

		waiting := make([]LeaderTask, 0, len(pending))
		for _, t := range pending {
			free := err == nil && len(busy[t.Name]) == 0
			if !free && !timedOut {
				waiting = append(waiting, t)
				continue
			}

			if !free {
				busyAtTimeout[t.Name] = busy[t.Name]
			}

			if t.BeforeStart != nil {
				t.BeforeStart(term)
			}

			if !m.startLeaderTask(term, t) {
				return
			}
		}

		pending = waiting
		if len(pending) == 0 {
			break
		}

		select {
		case <-term.Done():
			return

		case <-time.After(m.handoverInterval):
		}
	}

	slog.InfoContext(term, "Started background tasks on the cluster leader")
	m.reportHandover(term, busyAtTimeout)
}

// startLeaderTask starts the leader task t of term, false if the term has ended.
func (m *membership) startLeaderTask(term context.Context, t LeaderTask) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	if term.Err() != nil || !m.leading {
		return false
	}

	// Marked first, so the first run of the task already counts as its work.
	m.taskStatus.setStarted(t.Name, true)
	m.shutdownFuncs = append(m.shutdownFuncs, t.Start(term))

	return true
}

// reportHandover warns about the leader tasks started while other members still ran them, and removes the warning after a clean handover.
func (m *membership) reportHandover(ctx context.Context, busyAtTimeout map[string][]string) {
	if m.warnings == nil {
		return
	}

	if len(busyAtTimeout) == 0 {
		m.warnings.RemoveStale(ctx, leaderHandoverScope, nil)
		return
	}

	var addresses []string
	tasks := make([]string, 0, len(busyAtTimeout))
	for name, taskAddresses := range busyAtTimeout {
		tasks = append(tasks, name)
		for _, address := range taskAddresses {
			if !slices.Contains(addresses, address) {
				addresses = append(addresses, address)
			}
		}
	}

	slices.Sort(tasks)

	members, err := m.memberNames(ctx, addresses)
	if err != nil {
		members = make([]string, 0, len(addresses))
		for _, address := range addresses {
			members = append(members, strconv.Quote(address))
		}
	}

	message := fmt.Sprintf("Background tasks %s were started, while cluster members %s still ran them", strings.Join(tasks, ", "), strings.Join(members, ", "))
	if len(addresses) == 0 {
		message = fmt.Sprintf("Background tasks %s were started, without knowing whether the other cluster members still ran them", strings.Join(tasks, ", "))
	}

	m.warnings.Emit(ctx, warning.NewWarning(api.WarningTypeLeaderHandoverIncomplete, leaderHandoverScope, message))
}

// busyLeaderTasks returns the addresses of the other members still running or stopping each leader task, by task name.
// Members counted as offline and unreachable members are skipped.
func (m *membership) busyLeaderTasks(ctx context.Context) (map[string][]string, error) {
	notifier, err := m.gateway.NewNotifier(ctx, m.gateway.NetworkCert(), m.gateway.ServerCert(), cowsqlcluster.NotifyAlive)
	if err != nil {
		return nil, err
	}

	var busyMu sync.Mutex
	busy := map[string][]string{}

	errs := notifier(func(ctx context.Context, address string, _ cowsqltls.CertInfo, _ cowsqltls.CertInfo) error {
		status, err := m.memberTaskStatus(ctx, address)
		if err != nil {
			slog.InfoContext(ctx, "Skipping the leader tasks of an unreachable cluster member", slog.String("address", address), logger.Err(err))
			return nil
		}

		busyMu.Lock()
		defer busyMu.Unlock()

		for _, t := range status.Tasks {
			if t.State != membershipapi.TaskStopped {
				busy[t.Name] = append(busy[t.Name], address)
			}
		}

		return nil
	})

	return busy, errors.Join(errs...)
}

// memberTaskStatus returns the state of the leader tasks of the member at address.
func (m *membership) memberTaskStatus(ctx context.Context, address string) (*membershipapi.InternalClusterTaskStatus, error) {
	c, err := m.MemberClient(address)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	resp, err := c.DoInternalRequest(ctx, http.MethodPost, "/membership/tasks/:status", nil, nil)
	if err != nil {
		return nil, err
	}

	var status membershipapi.InternalClusterTaskStatus

	err = resp.MetadataAsStruct(&status)
	if err != nil {
		return nil, err
	}

	return &status, nil
}

// stopLeaderTasks stops the background tasks until the next restart.
func (m *membership) stopLeaderTasks(ctx context.Context) []error {
	// Cancel first, so the leadership check does not start the tasks again.
	m.cancel()

	// A handover in progress ends with the term, it needs mu to start a task.
	m.handover.Wait()

	m.mu.Lock()
	defer m.mu.Unlock()

	errs := m.stopBackgroundTasks(ctx)
	m.leading = false

	return errs
}

// stopBackgroundTasks stops the running background tasks in parallel. The caller must hold mu.
func (m *membership) stopBackgroundTasks(ctx context.Context) []error {
	m.termMu.Lock()
	if m.endTerm != nil {
		m.endTerm()
	}

	m.termMu.Unlock()

	// The tasks count as stopping, while work of them is still in flight.
	for _, t := range m.backgroundTasks {
		m.taskStatus.setStarted(t.Name, false)
	}

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

func (m *membership) TrustedClients() map[string]x509.Certificate {
	m.clientCertMu.RLock()
	defer m.clientCertMu.RUnlock()

	cache := make(map[string]x509.Certificate, len(m.clientCertCache))
	maps.Copy(cache, m.clientCertCache)

	return cache
}

func (m *membership) HandlerFuncs() map[string]http.HandlerFunc {
	return m.Gateway().HandlerFuncs(m.gatewayAuthenticator())
}

// PersistSharedConfig stores the section changed by change in the global database, if the ETag carried by ctx matches.
// The other sections are kept as stored, so a member which missed a change of them can't overwrite it.
func (m *membership) PersistSharedConfig(ctx context.Context, change config.SharedChange) (config.SharedConfig, config.SharedConfig, error) {
	var previous config.SharedConfig
	var merged config.SharedConfig
	err := transaction.ForceTx(ctx, m.global(), func(ctx context.Context, _ transaction.TX) error {
		stored, err := m.sharedConfig().GetSharedConfig(ctx)
		if err != nil {
			return err
		}

		err = change.CheckETag(ctx, *stored)
		if err != nil {
			return err
		}

		previous = *stored
		merged = change.Apply(previous)

		return m.sharedConfig().UpdateSharedConfig(ctx, merged)
	})
	if err != nil {
		return config.SharedConfig{}, config.SharedConfig{}, err
	}

	return merged, previous, nil
}

// PublishSharedConfig lets the other cluster members reload a change stored by PersistSharedConfig.
// The change is reverted if a reachable member fails to reload it.
func (m *membership) PublishSharedConfig(ctx context.Context, change config.SharedChange, previous config.SharedConfig) error {
	// Members out of reach pick up the config when they start.
	notified := m.reloadSharedConfigOnMembers(ctx)
	if notified == nil {
		return nil
	}

	// The revert still runs after the client went away.
	revertCtx := context.WithoutCancel(ctx)

	err := transaction.ForceTx(revertCtx, m.global(), func(ctx context.Context, _ transaction.TX) error {
		current, err := m.sharedConfig().GetSharedConfig(ctx)
		if err != nil {
			return err
		}

		// A later change of the section by another member is kept.
		if !change.Applied(*current) {
			return nil
		}

		return m.sharedConfig().UpdateSharedConfig(ctx, change.Revert(*current, previous))
	})
	if err != nil {
		return errors.Join(notified, fmt.Errorf("Failed to revert the shared config: %w", err))
	}

	// Let this and the other members reload the previous config.
	reverted := errors.Join(m.ReloadSharedConfig(revertCtx), m.reloadSharedConfigOnMembers(revertCtx))
	if reverted != nil {
		slog.WarnContext(ctx, "Failed to let all cluster members reload the reverted shared config", logger.Err(reverted))
	}

	return notified
}

// reloadSharedConfigOnMembers lets the other cluster members reload the shared config.
// The members counted as offline by the heartbeats are asked as well, since a member which just started counts as
// offline for a while. One of them failing is skipped: it applies the stored config when it starts.
func (m *membership) reloadSharedConfigOnMembers(ctx context.Context) error {
	notifier, err := m.gateway.NewNotifier(ctx, m.gateway.NetworkCert(), m.gateway.ServerCert(), cowsqlcluster.NotifyTryAll)
	if err != nil {
		return fmt.Errorf("Failed to notify cluster members about the shared config change: %w", err)
	}

	offline := m.offlineAddresses(ctx)

	var failedMu sync.Mutex
	var failed []string

	errs := notifier(func(ctx context.Context, address string, _ cowsqltls.CertInfo, _ cowsqltls.CertInfo) error {
		err := m.reloadSharedConfigOnMember(ctx, address)
		if err != nil && offline[address] {
			slog.WarnContext(ctx, "Cluster member counted as offline did not reload the shared config, it applies the stored config when it starts", slog.String("address", address), logger.Err(err))
			return nil
		}

		if err != nil {
			failedMu.Lock()
			failed = append(failed, address)
			failedMu.Unlock()
		}

		return err
	})

	if len(failed) == 0 {
		return nil
	}

	names, err := m.memberNames(ctx, failed)
	if err != nil {
		return errors.Join(append(errs, err)...)
	}

	return domain.NewErrorf(domain.ErrUnavailable, "", "Cluster members %s did not apply the shared config change, it has been reverted", strings.Join(names, ", ")).
		WithHintf("Retry once all cluster members are reachable.").
		WithCause(errors.Join(errs...))
}

// reloadSharedConfigOnMember lets the cluster member at address reload the shared config.
func (m *membership) reloadSharedConfigOnMember(ctx context.Context, address string) error {
	if m.reloadConfigOnMember != nil {
		return m.reloadConfigOnMember(ctx, address)
	}

	c, err := m.MemberClient(address)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	_, err = c.DoInternalRequest(ctx, http.MethodPost, "/membership/:reload-config", nil, nil)

	return err
}

// neverReached tells whether the error of a request to another member says that no connection to it was made, so it never saw the request.
func neverReached(err error) bool {
	var opErr *net.OpError

	return errors.As(err, &opErr) && opErr.Op == "dial"
}

// offlineAddresses returns the addresses of the cluster members counted as offline by the heartbeats.
func (m *membership) offlineAddresses(ctx context.Context) map[string]bool {
	if m.global() == nil {
		return nil
	}

	members, err := m.global().GetNodes(ctx)
	if err != nil {
		slog.WarnContext(ctx, "Failed to get the cluster members, counting all of them as online", logger.Err(err))
		return nil
	}

	threshold := m.gateway.HeartbeatOfflineThreshold()

	offline := map[string]bool{}
	for _, member := range members {
		if member.IsOffline(threshold) {
			offline[member.Address] = true
		}
	}

	return offline
}

// memberNames returns the quoted names of the cluster members at addresses, sorted.
func (m *membership) memberNames(ctx context.Context, addresses []string) ([]string, error) {
	members, err := m.GetMembersByID(ctx)
	if err != nil {
		return nil, err
	}

	byAddress := make(map[string]string, len(members))
	for _, member := range members {
		byAddress[member.Address] = member.Name
	}

	names := make([]string, 0, len(addresses))
	for _, address := range addresses {
		name, ok := byAddress[address]
		if !ok {
			name = address
		}

		names = append(names, strconv.Quote(name))
	}

	slices.Sort(names)

	return names, nil
}

// ContactMembers checks that all other cluster members except the named ones can be reached.
func (m *membership) ContactMembers(ctx context.Context, except ...string) error {
	enabled, err := cowsqlmembership.Enabled(m.Gateway().Node())
	if err != nil {
		return err
	}

	if !enabled {
		return nil
	}

	exceptAddresses := make([]string, 0, len(except))
	if len(except) > 0 {
		members, err := m.GetMembersByID(ctx)
		if err != nil {
			return err
		}

		for _, member := range members {
			if slices.Contains(except, member.Name) {
				exceptAddresses = append(exceptAddresses, member.Address)
			}
		}
	}

	notifier, err := m.gateway.NewNotifier(ctx, m.gateway.NetworkCert(), m.gateway.ServerCert(), cowsqlcluster.NotifyTryAll)
	if err != nil {
		return err
	}

	var unreachableMu sync.Mutex
	var unreachable []string

	_ = notifier(func(_ context.Context, address string, networkCert cowsqltls.CertInfo, serverCert cowsqltls.CertInfo) error {
		if slices.Contains(exceptAddresses, address) {
			return nil
		}

		if !cowsqlcluster.HasConnectivity(networkCert, serverCert, address, m.gateway.Options().RestrictTLS()) {
			unreachableMu.Lock()
			unreachable = append(unreachable, address)
			unreachableMu.Unlock()
		}

		return nil
	})

	if len(unreachable) == 0 {
		return nil
	}

	names, err := m.memberNames(ctx, unreachable)
	if err != nil {
		return err
	}

	return domain.NewErrorf(domain.ErrUnavailable, "", "Cluster member %s can not be reached, all members need to be reachable", strings.Join(names, ", ")).
		WithHintf("Bring the cluster members back online or remove the ones that are down.")
}

// ReloadSharedConfig applies the shared config stored in the global database by another cluster member.
func (m *membership) ReloadSharedConfig(ctx context.Context) error {
	if m.sharedConfig() == nil {
		return domain.NewErrorf(domain.ErrOperationNotPermitted, "", "Cluster database is still initializing")
	}

	return config.ReloadShared(ctx, func(ctx context.Context) (config.SharedConfig, error) {
		cfg, err := m.sharedConfig().GetSharedConfig(ctx)
		if err != nil {
			return config.SharedConfig{}, err
		}

		return *cfg, nil
	})
}

// CheckMembersOnline fails unless all cluster members are online.
func (m *membership) CheckMembersOnline(ctx context.Context) error {
	_, err := m.gateway.NewNotifier(ctx, m.gateway.NetworkCert(), m.gateway.ServerCert(), cowsqlcluster.NotifyAll)
	if err != nil {
		return domain.NewErrorf(domain.ErrOperationNotPermitted, "", "All cluster members need to be online to update the cluster certificate").WithCause(err)
	}

	return nil
}

// UpdateClusterCertificate applies the cluster certificate on all cluster members. All of them need to be online.
func (m *membership) UpdateClusterCertificate(ctx context.Context, certificatePEM string, keyPEM string) error {
	keyPair, err := tls.X509KeyPair([]byte(certificatePEM), []byte(keyPEM))
	if err != nil {
		return domain.NewValidationErrf("Failed to validate key pair: %v", err)
	}

	currentCert, err := m.gateway.NetworkCert().PublicKeyX509()
	if err != nil {
		return err
	}

	newCert, err := x509.ParseCertificate(keyPair.Certificate[0])
	if err != nil {
		return err
	}

	// A certificate for the same key reaches the online members now and the others when they start.
	// The members counted as offline by the heartbeats are tried as well, since a member which just started counts as offline for a while.
	// A new key would cut off an offline member for good, so it needs all members.
	policy := cowsqlcluster.NotifyTryAll
	if !sameKey(currentCert, newCert) {
		policy = cowsqlcluster.NotifyAll
	}

	notifier, err := m.gateway.NewNotifier(ctx, m.gateway.NetworkCert(), m.gateway.ServerCert(), policy)
	if err != nil {
		return domain.NewErrorf(domain.ErrOperationNotPermitted, "", "All cluster members need to be online to replace the key of the cluster certificate").WithCause(err)
	}

	previousCertificate, err := m.varRoot.ReadFile("cluster.crt")
	if err != nil {
		return err
	}

	previousKey, err := m.varRoot.ReadFile("cluster.key")
	if err != nil {
		return err
	}

	previous := apisystem.CertificatePost{Certificate: string(previousCertificate), Key: string(previousKey)}

	reverter := revert.New()
	defer reverter.Fail()

	// Rollbacks still run after the client went away.
	revertCtx := context.WithoutCancel(ctx)

	var reverterMu sync.Mutex

	offline := m.offlineAddresses(ctx)
	skipped := map[string]bool{}

	post := apisystem.CertificatePost{Certificate: certificatePEM, Key: keyPEM}
	errs := notifier(func(notifyCtx context.Context, address string, _ cowsqltls.CertInfo, _ cowsqltls.CertInfo) error {
		// A member whose answer is lost may have applied the certificate, so it is put back whatever the answer.
		// A member which could not be connected to never saw it.
		reverterMu.Lock()
		reverter.Add(func() {
			if skipped[address] {
				return
			}

			err := m.postClusterCertificateTrusting(revertCtx, address, []*x509.Certificate{newCert, currentCert}, previous)
			if err != nil {
				slog.ErrorContext(revertCtx, "Failed to put back the previous cluster certificate on cluster member", slog.String("address", address), logger.Err(err))
			}
		})
		reverterMu.Unlock()

		err := m.postClusterCertificate(notifyCtx, address, currentCert, post)
		if err != nil && neverReached(err) {
			reverterMu.Lock()
			skipped[address] = true
			reverterMu.Unlock()
		}

		if err != nil && policy == cowsqlcluster.NotifyTryAll && offline[address] {
			reverterMu.Lock()
			skipped[address] = true
			reverterMu.Unlock()

			slog.WarnContext(notifyCtx, "Cluster member counted as offline did not apply the cluster certificate, it adopts it when it starts", slog.String("address", address), logger.Err(err))

			return nil
		}

		return err
	})

	err = errors.Join(errs...)
	if err != nil {
		return domain.NewErrorf(domain.ErrUnavailable, "", "Failed to update the cluster certificate on all cluster members").WithCause(err).WithHintf("Retry once all cluster members are reachable.")
	}

	// This member is updated last, so it is unchanged if another one fails.
	err = m.setClusterCertificate(keyPair)
	if err != nil {
		return err
	}

	reverter.Success()

	return nil
}

// postClusterCertificate applies the cluster certificate on the member at address, which must present trusted.
func (m *membership) postClusterCertificate(ctx context.Context, address string, trusted *x509.Certificate, certificate apisystem.CertificatePost) error {
	if m.postCertificate != nil {
		return m.postCertificate(ctx, address, trusted, certificate)
	}

	certInfo, ok := m.gateway.ServerCert().(*incustls.CertInfo)
	if !ok {
		//domain-errors:internal The server certificate is always loaded by the daemon itself.
		return fmt.Errorf("Invalid certificate info: %T", m.gateway.ServerCert())
	}

	c, err := client.New("https://"+address, client.WithClientCertificate(certInfo), client.WithTrustedServerCertificate(trusted), client.WithDialTimeout(MemberDialTimeout))
	if err != nil {
		return err
	}

	_, err = c.DoInternalRequest(ctx, http.MethodPost, "/membership/:update-certificate", nil, certificate)

	return err
}

// postClusterCertificateTrusting is postClusterCertificate for a member which presents one of the trusted certificates.
func (m *membership) postClusterCertificateTrusting(ctx context.Context, address string, trusted []*x509.Certificate, certificate apisystem.CertificatePost) error {
	var errs []error

	for _, cert := range trusted {
		err := m.postClusterCertificate(ctx, address, cert, certificate)
		if err == nil {
			return nil
		}

		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

// ApplyClusterCertificate applies the cluster certificate updated by another cluster member.
func (m *membership) ApplyClusterCertificate(ctx context.Context, certificatePEM string, keyPEM string) error {
	keyPair, err := tls.X509KeyPair([]byte(certificatePEM), []byte(keyPEM))
	if err != nil {
		return domain.NewValidationErrf("Failed to validate key pair: %v", err)
	}

	err = m.setClusterCertificate(keyPair)
	if err != nil {
		return err
	}

	return lifecycle.ServerCertificateUpdateSignal.TryEmit(ctx, keyPair)
}

// setClusterCertificate persists the cluster certificate and applies it to the listeners and the database gateway.
func (m *membership) setClusterCertificate(keyPair tls.Certificate) error {
	certInfo, err := m.gateway.State().SetClusterCertificate(cowsqltls.NewCertInfo(keyPair, nil, nil))
	if err != nil {
		return fmt.Errorf("Failed to persist cluster certificate: %w", err)
	}

	m.gateway.NetworkUpdateCert(certInfo)

	return nil
}

// MemberClient returns a client for the cluster member with the given address.
func (m *membership) MemberClient(address string) (client.OperationsCenterClient, error) {
	networkCert, err := m.gateway.NetworkCert().PublicKeyX509()
	if err != nil {
		return client.OperationsCenterClient{}, err
	}

	certInfo, ok := m.gateway.ServerCert().(*incustls.CertInfo)
	if !ok {
		//domain-errors:internal The daemon always loads its own server certificate.
		return client.OperationsCenterClient{}, fmt.Errorf("Invalid certificate info: %T", m.gateway.ServerCert())
	}

	return client.New("https://"+address, client.WithClientCertificate(certInfo), client.WithTrustedServerCertificate(networkCert), client.WithDialTimeout(MemberDialTimeout))
}

func (m *membership) LeaderAddress(ctx context.Context) (string, error) {
	leader, err := m.Gateway().IsLeader(ctx)
	if err != nil {
		return "", fmt.Errorf("Failed to check for leader status: %w", err)
	}

	if leader {
		return m.Gateway().State().ClusterAddress(), nil
	}

	return m.Gateway().LeaderAddress()
}

func (m *membership) isLeader(ctx context.Context) error {
	leader, err := m.Gateway().IsLeader(ctx)
	if err != nil {
		return fmt.Errorf("Failed to check for leader status: %w", err)
	}

	// Requests are forwarded to the leader, so this only happens during a leader change.
	if !leader {
		return domain.NewErrorf(domain.ErrUnavailable, "", "Cluster member %q is not the cluster leader anymore", m.gateway.RaftNode().Name).
			WithHintf("Retry the request.")
	}

	return nil
}

func (m *membership) handoverRole(ctx context.Context) error {
	// Give the handover its own timeout, since the shutdown may have used up the deadline.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()

	// If we aren't clustered, there's nothing to do.
	clustered := m.gateway.NetworkCert().Fingerprint() != m.gateway.ServerCert().Fingerprint()
	if !clustered {
		return nil
	}

	enabled, err := cowsqlmembership.Enabled(m.gateway.Node())
	if err != nil || !enabled {
		return err
	}

	// Figure out our own cluster address.
	localClusterAddress, err := m.gateway.Node().GetClusterAddress(ctx)
	if err != nil {
		return err
	}

	log := slog.With(slog.String("local_address", localClusterAddress))

	// Retry for a while on transient errors (leader changes, unreachable
	// leader or another configuration change in progress), as leaving
	// without a successful handover can cost the cluster its quorum.
	// Individual attempts can be slow (each internal call has its own
	// timeout), so bound the retries by time rather than count to avoid
	// hanging shutdown when the cluster has lost quorum.
	for i := 0; ctx.Err() == nil; i++ {
		if i > 0 {
			select {
			case <-ctx.Done():
				return errors.Join(err, ctx.Err())

			case <-time.After(time.Second):
			}
		}

		// Find the cluster leader, which may change between attempts.
		var leader string
		leader, err = m.gateway.LeaderAddress()
		if err != nil {
			continue
		}

		if leader == "" {
			err = errors.New("No leader address found")
			continue
		}

		if leader == localClusterAddress {
			log.InfoContext(ctx, "Transferring leadership")
			transferCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			err = m.Gateway().TransferLeadership(transferCtx)
			cancel()

			if errors.Is(err, cowsqlmembership.ErrNoOnlineVoter) {
				log.InfoContext(ctx, "No other online voter to transfer leadership to")
				return nil
			}

			if err != nil {
				err = fmt.Errorf("Failed to transfer leadership: %w", err)

				continue
			}

			leader, err = m.gateway.LeaderAddress()
			if err != nil {
				continue
			}

			if leader == "" || leader == localClusterAddress {
				err = errors.New("No leader address found")
				continue
			}
		}

		log.InfoContext(ctx, "Handing over cluster member role")
		post := &membershipapi.InternalClusterPostHandoverRequest{
			Address: localClusterAddress,
		}

		c, err := m.MemberClient(leader)
		if err != nil {
			return err
		}

		requestCtx, cancel := context.WithTimeout(ctx, time.Minute)
		_, err = c.DoInternalRequest(requestCtx, http.MethodPost, "/membership/:handover", nil, post)
		cancel()
		if err == nil {
			return nil
		}

		// Give up right away on errors that won't resolve by retrying.
		// The errors come back from the leader as text, usually wrapped in extra context and with
		// inconsistent capitalization ("Not leader", "503 not leader", "not leader (10250)"), so
		// case-insensitive substring matching is the best we can do.
		errText := strings.ToLower(err.Error())
		if !cowsqlmembership.IsClusterBusyErr(err) && !strings.Contains(errText, "not leader") && !strings.Contains(errText, "unable to connect") && !strings.Contains(errText, "client.timeout") {
			return err
		}
	}

	return err
}

func (m *membership) onClusterHeartbeat(ctx context.Context, gateway cowsqlcluster.Gateway, localDB LocalRaftRepo, globalDB GlobalRaftRepo, heartbeatData *cowsqlheartbeat.APIHeartbeat, isLeader bool, unavailableMembers []string) {
	// Don't process the heartbeat until we're fully online.
	if gateway.Cluster() == nil || globalDB.GetNodeID() == 0 {
		return
	}

	localClusterAddress, err := localDB.GetClusterAddress(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "Failed to get cluster address during heartbeat", logger.Err(err))
		return
	}

	if !heartbeatData.FullStateList || len(heartbeatData.Members) <= 0 {
		slog.ErrorContext(ctx, "Heartbeat member refresh task called with partial state list", slog.String("local_address", localClusterAddress))
		return
	}

	// FIXME: API extensions

	lastNodeList := m.lastNodeList.Load()
	if lastNodeList == nil || lastNodeList.Version.Schema != heartbeatData.Version.Schema {
		err := cowsqlmembership.MaybeUpdate(gateway)
		if err != nil {
			slog.ErrorContext(ctx, "Error updating cluster", logger.Err(err))
			return
		}
	}

	m.lastNodeList.Store(heartbeatData)

	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	// The rebalance waits for other role changes, and does nothing if this member lost the leadership meanwhile.
	if isLeader && unavailableMembers != nil {
		m.rolesMu.Lock()
		defer m.rolesMu.Unlock()
	}

	cowsqlmembership.RebalanceMembersHook(ctx, m.Gateway(), localClusterAddress, heartbeatData, isLeader, unavailableMembers, nil, m.changeMemberRole)
}

// transferLeadership hands the leadership of this member, named name, over to another voter.
// It holds the lock of the role changes, so a heartbeat rebalance can't undo the promotion of the remaining member of a two member cluster.
func (m *membership) transferLeadership(ctx context.Context, name string) error {
	m.rolesMu.Lock()
	defer m.rolesMu.Unlock()

	localAddress := m.Gateway().State().ClusterAddress()

	nodes, err := m.gateway.CurrentRaftNodes(ctx)
	if err != nil {
		return err
	}

	// Promote the remaining member of a two member cluster.
	if len(nodes) == 2 {
		for i := range nodes {
			if nodes[i].Address == localAddress || nodes[i].Role == cowsqldb.RaftVoter {
				continue
			}

			nodes[i].Role = cowsqldb.RaftVoter
			err = m.changeMemberRole(ctx, nodes[i].Address, nodes)
			if err != nil {
				return fmt.Errorf("Failed to promote remaining cluster member: %w", err)
			}

			break
		}
	}

	err = m.Gateway().TransferLeadership(ctx)
	if errors.Is(err, cowsqlmembership.ErrNoOnlineVoter) {
		return domain.NewErrorf(domain.ErrOperationNotPermitted, "", "Cluster member %q is the leader and there is no other online voter to take over", name)
	}

	if err != nil {
		return fmt.Errorf("Failed to transfer leadership: %w", err)
	}

	return nil
}

// Handover hands the raft role of the member with the given address over to
// another member. It must be called on the leader.
func (m *membership) Handover(ctx context.Context, address string) error {
	m.rolesMu.Lock()
	defer m.rolesMu.Unlock()

	target, nodes, err := cowsqlmembership.Handover(m.gateway, address)
	if err != nil {
		return err
	}

	if target == "" {
		return nil
	}

	slog.InfoContext(ctx, "Promoting member during handover", slog.String("losing_address", address), slog.String("candidate_address", target))
	err = m.changeMemberRole(ctx, target, nodes)
	if err != nil {
		return err
	}

	for i, raftNode := range nodes {
		if raftNode.Address == address {
			nodes[i].Role = cowsqldb.RaftSpare
		}
	}

	slog.InfoContext(ctx, "Demoting member during handover", slog.String("losing_address", address))

	return m.changeMemberRole(ctx, address, nodes)
}

// Rebalance promotes and demotes members until the raft roles match the
// desired number of voters and stand-bys. It must be called on the leader.
func (m *membership) Rebalance(ctx context.Context) error {
	m.rolesMu.Lock()
	defer m.rolesMu.Unlock()

	return m.rebalance(ctx)
}

// rebalance is Rebalance for a caller holding rolesMu.
func (m *membership) rebalance(ctx context.Context) error {
	for ctx.Err() == nil {
		address, nodes, err := cowsqlmembership.Rebalance(m.gateway, nil, nil)
		if err != nil {
			return err
		}

		// Nothing to do.
		if address == "" {
			return nil
		}

		demoted := false
		for _, member := range nodes {
			if member.Address != address {
				continue
			}

			reachable := cowsqlcluster.HasConnectivity(m.gateway.NetworkCert(), m.gateway.ServerCert(), address, m.gateway.Options().RestrictTLS())
			if member.Role != cowsqldb.RaftSpare {
				if !reachable {
					return nil
				}

				slog.InfoContext(ctx, "Promoting cluster member", slog.String("name", member.Name), slog.String("role", member.Role.String()))
				break
			}

			if reachable {
				break
			}

			slog.InfoContext(ctx, "Demoting cluster member", slog.String("name", member.Name), slog.String("role", member.Role.String()))

			err := m.gateway.DemoteOfflineNode(member.ID)
			if err != nil {
				return fmt.Errorf("Failed to demote cluster member %q: %w", member.Name, err)
			}

			demoted = true
			break
		}

		if demoted {
			continue
		}

		err = m.changeMemberRole(ctx, address, nodes)
		if err != nil {
			return err
		}
	}

	return ctx.Err()
}

// Post a change role request to the member with the given address. The nodes
// slice contains details about all members, including the one being changed.
func (m *membership) changeMemberRole(ctx context.Context, address string, nodes []cowsqldb.RaftNode) error {
	post := &membershipapi.InternalClusterPostAssignRequest{}
	for _, raftNode := range nodes {
		post.RaftNodes = append(post.RaftNodes, membershipapi.InternalRaftNode{
			ID:      raftNode.ID,
			Address: raftNode.Address,
			Role:    int(raftNode.Role),
			Name:    raftNode.Name,
		})
	}

	c, err := m.MemberClient(address)
	if err != nil {
		return err
	}

	b, err := json.Marshal(post)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

	_, err = c.DoInternalRequest(ctx, http.MethodPost, "/membership/:assign", nil, b)

	return err
}

// gatewayAuthenticator validates that the connection is from a trusted server.
func (m *membership) gatewayAuthenticator() func(w http.ResponseWriter, r *http.Request) bool {
	return func(w http.ResponseWriter, r *http.Request) bool {
		authn := authtls.New(func() []string {
			// Also trust this member's server certificate, since cowsql connects back to it early.
			return append(slices.Collect(maps.Keys(m.getTrustedCerts())), m.gateway.ServerCert().Fingerprint())
		})

		trusted, _, _, _ := authn.Auth(w, r)
		if !trusted {
			http.Error(w, "403 invalid client certificate", http.StatusForbidden)

			return false
		}

		return true
	}
}
