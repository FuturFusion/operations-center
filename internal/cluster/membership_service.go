package cluster

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
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
	"github.com/FuturFusion/operations-center/internal/cluster/repo/sqlite/entities"
	config "github.com/FuturFusion/operations-center/internal/config/daemon"
	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/lifecycle"
	authtls "github.com/FuturFusion/operations-center/internal/security/authn/tls"
	"github.com/FuturFusion/operations-center/internal/system"
	"github.com/FuturFusion/operations-center/internal/util/logger"
	utilmaps "github.com/FuturFusion/operations-center/internal/util/maps"
	"github.com/FuturFusion/operations-center/internal/util/task"
	"github.com/FuturFusion/operations-center/internal/version"
	"github.com/FuturFusion/operations-center/shared/api"
	apisystem "github.com/FuturFusion/operations-center/shared/api/system"
)

type membership struct {
	gateway cowsqlcluster.Gateway

	mu sync.Mutex

	// rolesMu serializes the changes of the raft roles on the leader.
	rolesMu sync.Mutex

	stopOnce sync.Once
	stopErr  error

	// resetPending removes the database on shutdown.
	resetPending atomic.Bool

	localRepo  LocalRaftRepo
	globalRepo GlobalRaftRepo
	tokenRepo  TokenRepo
	configRepo ConfigRepo

	lastNodeList *cowsqlheartbeat.APIHeartbeat

	clientCertMu    sync.RWMutex
	clientCertCache map[string]x509.Certificate

	joinerFingerprints *utilmaps.TTLMap[string, string]

	// ctx is the lifetime of the service.
	ctx    context.Context
	cancel context.CancelFunc
	env    environment
	system system.SystemService

	// serverCert is used within the cluster.
	serverCert atomic.Pointer[incustls.CertInfo]

	// pendingClusterCert replaces the generated cluster certificate on bootstrap.
	pendingClusterCert cowsqltls.CertInfo

	// The background tasks run on the cluster leader only.
	backgroundTasks []task.TaskFunc
	shutdownFuncs   []func(context.Context) error
	leading         bool
}

// leadershipCheckInterval is the interval of the cluster leadership check.
const leadershipCheckInterval = 5 * time.Second

func NewClusterMembershipService(ctx context.Context, env environment, systemSvc system.SystemService, listeners *listener.ListenerSet, localRepo LocalRaftRepo, clusterCert *incustls.CertInfo, serverCert *incustls.CertInfo, backgroundTasks []task.TaskFunc) (MembershipService, error) {
	ctx, cancel := context.WithCancel(ctx)

	m := &membership{
		ctx:                ctx,
		cancel:             cancel,
		env:                env,
		system:             systemSvc,
		clientCertCache:    map[string]x509.Certificate{},
		joinerFingerprints: utilmaps.NewTTLMap[string, string](ctx, time.Second),
		backgroundTasks:    backgroundTasks,
	}

	m.serverCert.Store(serverCert)

	g, err := cowsqlgateway.NewGateway(
		context.TODO(),
		localRepo,
		clusterCert,
		func() cowsqltls.CertInfo { return m.serverCert.Load() },
		newState(env, systemSvc, listeners, m),
		cowsqloptions.Version(version.Version),
	)
	if err != nil {
		return nil, err
	}

	g.SetHeartbeatNodeHook(func(heartbeatData *cowsqlheartbeat.APIHeartbeat, isLeader bool, unavailableMembers []string) {
		m.onClusterHeartbeat(ctx, g, localRepo, m.globalRepo, heartbeatData, isLeader, unavailableMembers)
	})

	lifecycle.SharedClusterUpdateSignal.AddListener(func(_ context.Context, cfg apisystem.Cluster) {
		g.SetHeartbeatOfflineThreshold(time.Duration(cfg.OfflineThresholdSeconds) * time.Second)
	})

	m.localRepo = localRepo
	m.gateway = g

	m.updateCertificateCacheFromLocal(ctx)

	return m, nil
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

	members, err := m.globalRepo.GetNodes(ctx)
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
	if m.globalRepo == nil {
		return 0, errors.New("Global database is not available")
	}

	return m.globalRepo.GetNodeID(), nil
}

// GetMembersByID returns the cluster members by their ID from the global database.
func (m *membership) GetMembersByID(ctx context.Context) (map[int64]cowsqldb.NodeInfo, error) {
	if m.globalRepo == nil {
		return nil, errors.New("Global database is not available")
	}

	members, err := m.globalRepo.GetNodes(ctx)
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

// Auth implements Auther to trust a joiner by the fingerprint the leader cached on /:trust.
func (m *membership) Auth(w http.ResponseWriter, r *http.Request) (trusted bool, username string, protocol string, _ error) {
	err := m.isLeader(r.Context())
	if err != nil {
		return false, "", "", fmt.Errorf("Rejecting attempt to establish trust with non-leader: %w", err)
	}

	return authtls.New(func() []string { return slices.Collect(maps.Values(m.joinerFingerprints.Dump())) }).Auth(w, r)
}

// CreateToken implements [MembershipService].
func (m *membership) CreateToken(ctx context.Context, req api.ClusterTokenPost) error {
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

	expiry := config.DefaultMembershipJoinTokenExpiry
	if req.Expiry != "" {
		expiry, err = time.ParseDuration(req.Expiry)
		if err != nil {
			return err
		}
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

		return m.tokenRepo.CreateToken(ctx, token)
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

	return m.tokenRepo.DeleteTokenByName(ctx, serverName)
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

	return m.tokenRepo.GetTokenByServerName(ctx, serverName)
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

	return m.tokenRepo.GetTokens(ctx)
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
			_ = os.Remove(filepath.Join(m.env.VarDir(), "cluster."+suffix))
		}
	})

	err = cowsqlmembership.Bootstrap(m.Gateway(), cfg.ServerName)
	if err != nil {
		return err
	}

	reverter.Success()

	err = m.system.PropagateCertificate(ctx)
	if err != nil {
		return fmt.Errorf("Failed to propagate the cluster certificate: %w", err)
	}

	return nil
}

func (m *membership) Accept(ctx context.Context, req api.ClusterAcceptPost) (*membershipapi.InternalClusterAcceptResponse, error) {
	err := m.isLeader(ctx)
	if err != nil {
		return nil, err
	}

	nodes, err := cowsqlmembership.Accept(m.Gateway(), req.Certificate.Certificate, req.ServerName, req.Address, req.SchemaVersion, req.APIVersion, req.Architecture)
	if err != nil {
		return nil, err
	}

	certsByAddress, err := m.globalRepo.GetCertificates(ctx)
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

	// Drop the fingerprint now that Accept has globally trusted the joiner.
	m.joinerFingerprints.Delete(req.ServerName)

	m.updateCertificateCacheFromGlobal(ctx)

	resp := membershipapi.InternalClusterAcceptResponse{
		LeaderAddress:      m.Gateway().State().ClusterAddress(),
		RaftNodes:          make([]membershipapi.InternalRaftNode, 0, len(nodes)),
		PublicKey:          string(m.Gateway().NetworkCert().PublicKey()),
		PrivateKey:         string(m.Gateway().NetworkCert().PrivateKey()),
		ServerCertificates: apiCerts,
	}

	for _, n := range nodes {
		resp.RaftNodes = append(resp.RaftNodes, membershipapi.InternalRaftNode{ID: n.ID, Address: n.Address, Role: int(n.Role)})
	}

	return &resp, nil
}

// RemoveMember removes the named cluster member and resets it to a standalone server unless force is set.
func (m *membership) RemoveMember(ctx context.Context, name string, force bool) error {
	err := m.isLeader(ctx)
	if err != nil {
		return err
	}

	pending := true
	member, err := m.globalRepo.GetNodeByName(ctx, name, true)
	if api.StatusErrorCheck(err, http.StatusNotFound) {
		pending = false
		member, err = m.globalRepo.GetNodeByName(ctx, name, false)
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

	address, err := cowsqlmembership.Leave[struct{}](m.gateway, name, force, pending)
	if err != nil {
		return err
	}

	err = cowsqlmembership.Purge[struct{}](m.gateway, name, pending)
	if err != nil {
		return fmt.Errorf("Failed to remove member from database: %w", err)
	}

	m.updateCertificateCacheFromGlobal(ctx)

	err = m.Rebalance(ctx)
	if err != nil {
		slog.WarnContext(ctx, "Failed to rebalance cluster roles after removing a member", slog.String("name", name), logger.Err(err))
	}

	if force {
		return nil
	}

	c, err := m.MemberClient(address)
	if err != nil {
		return err
	}

	_, err = c.DoInternalRequest(ctx, http.MethodPost, "/membership/:reset", nil, nil)
	if err != nil {
		return fmt.Errorf("Failed to reset the removed cluster member: %w", err)
	}

	return nil
}

// removeLeader transfers the leadership of this member and lets the new leader remove it.
func (m *membership) removeLeader(ctx context.Context, name string, force bool) error {
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

func (m *membership) Trust(ctx context.Context, token string, fingerprint string) error {
	err := m.isLeader(ctx)
	if err != nil {
		return err
	}

	tokenObj, err := api.DecodeJoinToken(token)
	if err != nil {
		return err
	}

	dbToken, err := m.GetTokenByServerName(ctx, tokenObj.ServerName)
	if err != nil {
		return err
	}

	err = m.DeleteTokenByName(ctx, dbToken.ServerName)
	if err != nil {
		return err
	}

	dbTokenStr, err := dbToken.ToAPI().String()
	if err != nil {
		return err
	}

	if dbTokenStr != token || dbToken.Secret != tokenObj.Secret {
		return domain.NewErrorf(domain.ErrNotAuthorized, "", "Unknown join token for server %q", tokenObj.ServerName)
	}

	if fingerprint == "" {
		return errors.New("Failed to record joiner certificate fingerprint")
	}

	m.joinerFingerprints.Set(dbToken.ServerName, fingerprint, time.Until(dbToken.ExpirationDate), nil)

	return nil
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
		return err
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

		if resp.TLS == nil || !slices.ContainsFunc(resp.TLS.PeerCertificates, func(c *x509.Certificate) bool {
			matches := incustls.CertFingerprint(c) == tokenObj.ClusterFingerprint
			if matches && clusterCert == nil {
				clusterCert = c
			}

			return matches
		}) {
			return domain.NewErrorf(domain.ErrInvalidArgument, "", "Cluster member %q did not present the certificate of the join token", address)
		}

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

	q := url.Values{}
	q.Set("token", req.Token)

	resp, err := c.DoInternalRequest(ctx, http.MethodPost, "/membership/:trust", q, nil)
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
		SchemaVersion: m.globalRepo.SchemaVersion(),
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
	// Stop trusting the other members, so a late heartbeat does not restore the raft members.
	m.setCertificateCache(nil)

	for _, suffix := range []string{"ca", "crt", "key"} {
		err := os.Remove(filepath.Join(m.env.VarDir(), "cluster."+suffix))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			slog.ErrorContext(ctx, "Failed to remove cluster certificate", logger.Err(err))
		}
	}

	err := m.Gateway().Reset(m.serverCert.Load())
	if err != nil {
		slog.ErrorContext(ctx, "Failed to reset cluster database", logger.Err(err))
		return
	}

	// The database server is started again standalone, so it is shut down once on restart.
	err = m.Gateway().Initialize(false)
	if err != nil {
		slog.ErrorContext(ctx, "Failed to start standalone database server", logger.Err(err))
	}
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
func (m *membership) Stop(ctx context.Context, handoverRoles bool) error {
	m.stopOnce.Do(func() {
		m.stopErr = m.stop(ctx, handoverRoles)
	})

	return m.stopErr
}

func (m *membership) stop(ctx context.Context, handoverRoles bool) error {
	errs := m.stopLeaderTasks(ctx)

	// Hand over the role unless this member is being reset.
	if m.globalRepo != nil && handoverRoles && !m.resetPending.Load() {
		err := m.handoverRole(ctx)
		if err != nil {
			errs = append(errs, fmt.Errorf("Failed to hand over cluster member role: %w", err))
		}
	}

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
		err = os.RemoveAll(filepath.Join(m.env.VarDir(), "database"))
		if err != nil {
			errs = append(errs, fmt.Errorf("Failed to remove the database of the reset member: %w", err))
		}
	}

	return errors.Join(errs...)
}

func (m *membership) Enable(ctx context.Context, clusterAddress string, tokenRepo TokenRepo, configRepo ConfigRepo, globalRepo GlobalRaftRepo) error {
	m.globalRepo = globalRepo
	m.gateway.SetClusterDB(globalRepo)
	m.tokenRepo = tokenRepo
	m.configRepo = configRepo

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
	m.globalRepo = globalDB
	m.gateway.SetClusterDB(globalDB)
}

func (m *membership) DetermineNodeID(ctx context.Context, address string) error {
	return m.globalRepo.DetermineNodeID(ctx, address)
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

// PersistSharedConfig stores cfg in the global database and lets the other cluster members reload it.
// The change is reverted if an online member fails to reload it.
func (m *membership) PersistSharedConfig(ctx context.Context, cfg apisystem.SharedConfig) error {
	previous, err := m.configRepo.GetSharedConfig(ctx)
	if err != nil {
		return err
	}

	err = m.configRepo.UpdateSharedConfig(ctx, cfg)
	if err != nil {
		return err
	}

	// Offline members pick up the config on restart.
	notified := m.reloadSharedConfigOnMembers(ctx, cowsqlcluster.NotifyAlive)
	if notified == nil {
		return nil
	}

	err = m.configRepo.UpdateSharedConfig(ctx, *previous)
	if err != nil {
		return errors.Join(notified, fmt.Errorf("Failed to revert the shared config: %w", err))
	}

	// Let the members reload the previous config.
	reverted := m.reloadSharedConfigOnMembers(ctx, cowsqlcluster.NotifyTryAll)
	if reverted != nil {
		slog.WarnContext(ctx, "Failed to let all cluster members reload the reverted shared config", logger.Err(reverted))
	}

	return notified
}

// reloadSharedConfigOnMembers lets the cluster members selected by policy reload the shared config.
func (m *membership) reloadSharedConfigOnMembers(ctx context.Context, policy cowsqlcluster.NotifierPolicy) error {
	notifier, err := m.gateway.NewNotifier(ctx, m.gateway.NetworkCert(), m.gateway.ServerCert(), policy)
	if err != nil {
		return fmt.Errorf("Failed to notify cluster members about the shared config change: %w", err)
	}

	var failedMu sync.Mutex
	var failed []string

	errs := notifier(func(ctx context.Context, address string, _ cowsqltls.CertInfo, _ cowsqltls.CertInfo) error {
		c, err := m.MemberClient(address)
		if err == nil {
			ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()

			_, err = c.DoInternalRequest(ctx, http.MethodPost, "/membership/:reload-config", nil, nil)
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

// ReloadSharedConfig applies the shared config stored in the global database by another cluster member.
func (m *membership) ReloadSharedConfig(ctx context.Context) error {
	if m.configRepo == nil {
		return domain.NewErrorf(domain.ErrOperationNotPermitted, "", "Cluster database is still initializing")
	}

	cfg, err := m.configRepo.GetSharedConfig(ctx)
	if err != nil {
		return err
	}

	return config.ReloadShared(ctx, *cfg)
}

// UpdateClusterCertificate applies the cluster certificate on all cluster members. All of them need to be online.
func (m *membership) UpdateClusterCertificate(ctx context.Context, certificatePEM string, keyPEM string) error {
	keyPair, err := tls.X509KeyPair([]byte(certificatePEM), []byte(keyPEM))
	if err != nil {
		return domain.NewValidationErrf("Failed to validate key pair: %v", err)
	}

	notifier, err := m.gateway.NewNotifier(ctx, m.gateway.NetworkCert(), m.gateway.ServerCert(), cowsqlcluster.NotifyAll)
	if err != nil {
		return domain.NewErrorf(domain.ErrOperationNotPermitted, "", "All cluster members need to be online to update the cluster certificate").WithCause(err)
	}

	currentCert, err := m.gateway.NetworkCert().PublicKeyX509()
	if err != nil {
		return err
	}

	newCert, err := x509.ParseCertificate(keyPair.Certificate[0])
	if err != nil {
		return err
	}

	previousCertificate, err := os.ReadFile(filepath.Join(m.env.VarDir(), "cluster.crt"))
	if err != nil {
		return err
	}

	previousKey, err := os.ReadFile(filepath.Join(m.env.VarDir(), "cluster.key"))
	if err != nil {
		return err
	}

	previous := apisystem.CertificatePost{Certificate: string(previousCertificate), Key: string(previousKey)}

	reverter := revert.New()
	defer reverter.Fail()

	var reverterMu sync.Mutex

	post := apisystem.CertificatePost{Certificate: certificatePEM, Key: keyPEM}
	errs := notifier(func(notifyCtx context.Context, address string, _ cowsqltls.CertInfo, _ cowsqltls.CertInfo) error {
		err := m.postClusterCertificate(notifyCtx, address, currentCert, post)
		if err != nil {
			return err
		}

		reverterMu.Lock()
		reverter.Add(func() {
			err := m.postClusterCertificate(ctx, address, newCert, previous)
			if err != nil {
				slog.ErrorContext(ctx, "Failed to put back the previous cluster certificate on cluster member", slog.String("address", address), logger.Err(err))
			}
		})
		reverterMu.Unlock()

		return nil
	})

	err = errors.Join(errs...)
	if err != nil {
		return fmt.Errorf("Failed to update the cluster certificate on all cluster members: %w", err)
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

	if !leader {
		//domain-errors:internal Requests are forwarded to the leader, so this only happens during a leader change.
		return fmt.Errorf("System %q is not the cluster leader", m.gateway.RaftNode().Name)
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

	if m.lastNodeList == nil || m.lastNodeList.Version.Schema != heartbeatData.Version.Schema {
		err := cowsqlmembership.MaybeUpdate(gateway)
		if err != nil {
			slog.ErrorContext(ctx, "Error updating cluster", logger.Err(err))
			return
		}
	}

	m.lastNodeList = heartbeatData

	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	cowsqlmembership.RebalanceMembersHook(ctx, m.Gateway(), localClusterAddress, heartbeatData, isLeader, unavailableMembers, nil, m.changeMemberRole)
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
