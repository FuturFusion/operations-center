package cluster

import (
	"bytes"
	"context"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	cowsqldb "github.com/cowsql/go-cowsql/cluster/db"
	cowstate "github.com/cowsql/go-cowsql/cluster/state"
	cowsqltls "github.com/cowsql/go-cowsql/cluster/tls"
	incustls "github.com/lxc/incus/v7/shared/tls"
	incusutil "github.com/lxc/incus/v7/shared/util"

	"github.com/FuturFusion/operations-center/internal/api/listener"
	"github.com/FuturFusion/operations-center/internal/system"
)

type environment interface {
	VarDir() string
	CacheDir() string
}

type state struct {
	env       environment
	system    system.SystemService
	svc       *membership
	listeners *listener.ListenerSet
}

// NewClusterCertificate implements [state.State].
func (s *state) NewClusterCertificate() (cowsqltls.CertInfo, error) {
	certDir := s.env.VarDir()
	// The cluster CA certificate is a symlink against the regular server CA certificate.
	if incusutil.PathExists(filepath.Join(certDir, "server.ca")) {
		err := os.Symlink("server.ca", filepath.Join(certDir, "cluster.ca"))
		if err != nil {
			return nil, fmt.Errorf("Failed to symlink server CA cert to cluster CA cert: %w", err)
		}
	}

	for _, suffix := range []string{"ca", "crt", "key"} {
		certPath := filepath.Join(certDir, "cluster."+suffix)
		_, err := os.Stat(certPath)
		if err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("Failed to check for stale certificate path %q: %w", certPath, err)
		}

		if err != nil {
			continue
		}

		err = os.RemoveAll(certPath)
		if err != nil {
			return nil, fmt.Errorf("Failed to remove stale certificate path %q: %w", certPath, err)
		}
	}

	// An ACME issued certificate is used instead of a self-signed one if prepared.
	pending := s.svc.takePendingClusterCertificate()
	if pending != nil {
		return s.SetClusterCertificate(pending)
	}

	certInfo, err := incustls.KeyPairAndCA(certDir, "cluster", incustls.CertServer, true)
	if err != nil {
		return nil, err
	}

	if s.listeners != nil {
		s.listeners.UpdateCertificate(certInfo.KeyPair())
	}

	return certInfo, nil
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

	certDir := s.env.VarDir()
	err := os.WriteFile(filepath.Join(certDir, "cluster.crt"), chain.Bytes(), 0o644)
	if err != nil {
		return nil, err
	}

	err = os.WriteFile(filepath.Join(certDir, "cluster.key"), i.PrivateKey(), 0o600)
	if err != nil {
		return nil, err
	}

	certInfo, err := incustls.KeyPairAndCA(certDir, "cluster", incustls.CertServer, true)
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
	s.svc.updateCertificateCacheFromGlobal(ctx)

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

func newState(env environment, systemSvc system.SystemService, listeners *listener.ListenerSet, m *membership) cowstate.State {
	return &state{
		env:       env,
		system:    systemSvc,
		svc:       m,
		listeners: listeners,
	}
}
