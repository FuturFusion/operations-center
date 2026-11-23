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
	tokenRepo  atomic.Pointer[JoinTokenRepo]
	configRepo atomic.Pointer[ConfigRepo]

	// joinCheck refuses a join, which is not forced, while this server still manages something.
	joinCheck func(ctx context.Context) error

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

// Option configures the membership service.
type Option func(m *membership)

// WithJoinCheck sets the check refusing a join of this server, which is not forced, while it still manages something.
func WithJoinCheck(check func(ctx context.Context) error) Option {
	return func(m *membership) {
		m.joinCheck = check
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
	return merged, nil
}
