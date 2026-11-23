package cluster

import (
	"bytes"
	"context"
	"encoding/pem"
	"errors"
	"fmt"
	"io/fs"
	"os"

	cowsqldb "github.com/cowsql/go-cowsql/cluster/db"
	cowstate "github.com/cowsql/go-cowsql/cluster/state"
	cowsqltls "github.com/cowsql/go-cowsql/cluster/tls"
	incustls "github.com/lxc/incus/v7/shared/tls"

	"github.com/FuturFusion/operations-center/internal/api/listener"
	"github.com/FuturFusion/operations-center/internal/system"
)

type environment interface {
	CacheDir() string
}

type state struct {
	varRoot   *os.Root
	system    system.SystemService
	svc       *membership
	listeners *listener.ListenerSet
}

// NewClusterCertificate implements [state.State].
func (s *state) NewClusterCertificate() (cowsqltls.CertInfo, error) {
	for _, suffix := range []string{"ca", "crt", "key"} {
		name := "cluster." + suffix

		_, err := s.varRoot.Stat(name)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("Failed to check for stale certificate path %q: %w", name, err)
		}

		if err != nil {
			continue
		}

		err = s.varRoot.RemoveAll(name)
		if err != nil {
			return nil, fmt.Errorf("Failed to remove stale certificate path %q: %w", name, err)
		}
	}

	// An ACME issued certificate is used instead of a self-signed one if prepared.
	pending := s.svc.takePendingClusterCertificate()
	if pending != nil {
		return s.SetClusterCertificate(pending)
	}

	certPEM, keyPEM, err := incustls.GenerateMemCert(false, true)
	if err != nil {
		return nil, err
	}

	return s.storeClusterCertificate(certPEM, keyPEM)
}

// SetClusterCertificate implements [state.State].
func (s *state) SetClusterCertificate(i cowsqltls.CertInfo) (cowsqltls.CertInfo, error) {
	var chain bytes.Buffer
	for _, cert := range i.KeyPair().Certificate {
		err := pem.Encode(&chain, &pem.Block{Type: "CERTIFICATE", Bytes: cert})
		if err != nil {
			return nil, err
		}
	}

	return s.storeClusterCertificate(chain.Bytes(), i.PrivateKey())
}

// storeClusterCertificate writes the cluster certificate to the var dir and returns it, loaded for the listeners.
func (s *state) storeClusterCertificate(certPEM []byte, keyPEM []byte) (cowsqltls.CertInfo, error) {
	err := s.varRoot.WriteFile("cluster.crt", certPEM, 0o644)
	if err != nil {
		return nil, err
	}

	err = s.varRoot.WriteFile("cluster.key", keyPEM, 0o600)
	if err != nil {
		return nil, err
	}

	certInfo, err := incustls.KeyPairFromRaw(certPEM, keyPEM)
	if err != nil {
		return nil, err
	}

	if s.listeners != nil {
		s.listeners.UpdateCertificate(certInfo.KeyPair())
	}

	return certInfo, nil
}

// HasListener implements [state.State].
func (s *state) HasListener(_ context.Context) error {
	if s.listeners == nil || !s.listeners.Active() {
		return errors.New("Network listeners have not been set up")
	}

	return nil
}

// UpdateAuthenticator implements [state.State].
func (s *state) UpdateAuthenticator(ctx context.Context) error {
	return nil
}

// OnHeartbeatNotification implements [state.State].
func (s *state) OnHeartbeatNotification(members map[int64]cowsqldb.HeartbeatMember) {
}

// ClusterAddress implements [state.State].
func (s *state) ClusterAddress() string {
	cfg := s.system.GetNetworkConfig(context.TODO())

	return cfg.ClusterAddress
}

func newState(varRoot *os.Root, systemSvc system.SystemService, listeners *listener.ListenerSet, m *membership) cowstate.State {
	return &state{
		varRoot:   varRoot,
		system:    systemSvc,
		svc:       m,
		listeners: listeners,
	}
}
