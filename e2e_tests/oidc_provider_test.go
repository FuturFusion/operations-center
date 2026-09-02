package e2e

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lxc/incus/v7/test/mini-oidc/storage"
	"github.com/stretchr/testify/require"
	"github.com/zitadel/oidc/v3/pkg/op"
)

const (
	oidcClientID = "device"
	oidcSubject  = "oidc-e2e-user"

	// oidcAccessTokenExpiration is deliberately short, so a refresh of the access
	// token can be provoked without a long wait. It needs to stay above the early
	// refresh leeway of 15 seconds of the operations-center CLI, otherwise
	// every request would run into a refresh.
	oidcAccessTokenExpiration           = 30 * time.Second
	oidcRefreshTokenExpiration          = 1 * time.Hour
	oidcDeviceAuthorizationPollInterval = 1 * time.Second
)

// oidcProvider is a fake OIDC provider, which is served in-process by the test.
type oidcProvider struct {
	Issuer   string
	ClientID string
	storage  *oidcAutoApproveStorage
}

func (p *oidcProvider) DeviceAuthorizationCount() int64 {
	return p.storage.deviceAuthorizationCount.Load()
}

func startOIDCProvider(t *testing.T) *oidcProvider {
	t.Helper()

	stop := timeTrack(t)
	defer stop()

	providerHostAddress := e2eHostAddress(t)

	// Bind on all the interfaces, since the address, at which the
	// OperationsCenter VM reaches the host, is derived separately.
	listener, err := net.Listen("tcp", "0.0.0.0:0")
	require.NoError(t, err, "Failed to listen for the OIDC provider")

	tcpAddr, ok := listener.Addr().(*net.TCPAddr)
	require.True(t, ok, "Failed to get the port of the OIDC provider listener")

	port := strconv.Itoa(tcpAddr.Port)

	issuer := fmt.Sprintf("http://%s/", net.JoinHostPort(providerHostAddress, port))

	registerOIDCClients()

	storageBackend := &oidcAutoApproveStorage{
		Storage: storage.NewStorage(
			oidcUserStore{},
			storage.WithAccessTokenExpiration(oidcAccessTokenExpiration),
			storage.WithRefreshTokenExpiration(oidcRefreshTokenExpiration),
		),
		subject: oidcSubject,
	}

	config := &op.Config{
		CryptoKey:               sha256.Sum256([]byte("operations-center end 2 end tests")),
		CodeMethodS256:          true,
		AuthMethodPost:          true,
		AuthMethodPrivateKeyJWT: true,
		GrantTypeRefreshToken:   true,
		RequestObjectSupported:  true,
		DeviceAuthorization: op.DeviceAuthorizationConfig{
			Lifetime:     strechedTimeout(2 * time.Minute),
			PollInterval: oidcDeviceAuthorizationPollInterval,
			UserFormPath: "/device",
			UserCode:     op.UserCodeBase20,
		},
	}

	provider, err := op.NewProvider(config, storageBackend, op.StaticIssuer(issuer), op.WithAllowInsecure())
	require.NoError(t, err, "Failed to create the OIDC provider")

	mux := http.NewServeMux()

	// Device authorizations are approved by oidcAutoApproveStorage already, so
	// this endpoint is only reached, if the verification URI is opened manually.
	mux.HandleFunc("GET /device", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("The device authorization has been approved automatically.\n"))
	})

	mux.Handle("/", provider)

	server := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: strechedTimeout(10 * time.Second),
	}

	t.Cleanup(func() {
		if noCleanup || (noCleanupOnError && t.Failed()) {
			return
		}

		// In t.Cleanup, t.Context() is already cancelled, so we need a detached context.
		ctx, cancel := context.WithTimeout(context.Background(), strechedTimeout(30*time.Second))
		defer cancel()

		err := server.Shutdown(ctx)
		if err != nil {
			t.Errorf("Failed to shut down the OIDC provider: %v", err)
		}
	})

	go func() {
		err := server.Serve(listener)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			t.Errorf("OIDC provider stopped unexpectedly: %v", err)
		}
	}()

	ctx, cancel := context.WithTimeout(t.Context(), strechedTimeout(10*time.Second))
	defer cancel()

	err = waitForTCPPort(ctx, t, net.JoinHostPort("127.0.0.1", port), 100*time.Millisecond)
	require.NoError(t, err, "OIDC provider did not come up")

	mustAssertOIDCProviderReachable(t, issuer)

	t.Logf("Fake OIDC provider is listening on %s", issuer)

	return &oidcProvider{
		Issuer:   issuer,
		ClientID: oidcClientID,
		storage:  storageBackend,
	}
}

func mustAssertOIDCProviderReachable(t *testing.T, issuer string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), strechedTimeout(10*time.Second))
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, issuer+".well-known/openid-configuration", nil)
	require.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err, "OIDC provider is not answering")

	defer func() {
		_ = resp.Body.Close()
	}()

	require.Equal(t, http.StatusOK, resp.StatusCode, "expect the OIDC provider to serve the discovery document")

	discovery := struct {
		Issuer                      string `json:"issuer"`
		DeviceAuthorizationEndpoint string `json:"device_authorization_endpoint"`
	}{}

	err = json.NewDecoder(resp.Body).Decode(&discovery)
	require.NoError(t, err)

	require.Equal(t, issuer, discovery.Issuer, "expect the OIDC provider to advertise the expected issuer")
	require.NotEmpty(t, discovery.DeviceAuthorizationEndpoint, "expect the OIDC provider to support the device authorization grant")
}

// oidcAutoApproveStorage approves every device authorization on behalf of the
// user, so the device authorization grant of the operations-center CLI
// completes without a browser. It also counts the device authorizations, which
// allows to verify, that a refresh of the access token does not trigger a new
// interactive login.
type oidcAutoApproveStorage struct {
	*storage.Storage

	subject                  string
	deviceAuthorizationCount atomic.Int64
}

func (s *oidcAutoApproveStorage) StoreDeviceAuthorization(ctx context.Context, clientID string, deviceCode string, userCode string, expires time.Time, scopes []string) error {
	err := s.Storage.StoreDeviceAuthorization(ctx, clientID, deviceCode, userCode, expires, scopes)
	if err != nil {
		return err
	}

	s.deviceAuthorizationCount.Add(1)

	return s.CompleteDeviceAuthorization(ctx, userCode, s.subject)
}

// oidcUserStore serves a single static user for every lookup.
type oidcUserStore struct{}

func (oidcUserStore) ExampleClientID() string {
	return oidcClientID
}

func (oidcUserStore) GetUserByID(string) *storage.User {
	return oidcUser()
}

func (oidcUserStore) GetUserByUsername(string) *storage.User {
	return oidcUser()
}

func oidcUser() *storage.User {
	return &storage.User{
		ID:       oidcSubject,
		Username: oidcSubject,
	}
}

var registerOIDCClientsOnce sync.Once

func registerOIDCClients() {
	registerOIDCClientsOnce.Do(func() {
		storage.RegisterClients(storage.IncusDeviceClient(oidcClientID))
	})
}
