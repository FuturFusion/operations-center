package server_test

import (
	"crypto/tls"
	"sync"
	"testing"

	incustls "github.com/lxc/incus/v6/shared/tls"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/util/ptr"
	"github.com/FuturFusion/operations-center/shared/api"
)

type serverOpt func(s *provisioning.Server)

var (
	validServerOnce sync.Once
	certPEM         []byte
	keyPEM          []byte
	certificate     tls.Certificate
)

func validServer(t *testing.T, opts ...serverOpt) provisioning.Server {
	t.Helper()

	validServerOnce.Do(func() {
		var err error
		certPEM, keyPEM, err = incustls.GenerateMemCert(false, false)
		require.NoError(t, err)

		certificate, err = tls.X509KeyPair(certPEM, keyPEM)
		require.NoError(t, err)
	})

	server := provisioning.Server{
		Name:          "one",
		Cluster:       ptr.To("one"),
		ConnectionURL: "http://one/",
		Certificate:   string(certPEM),
		Status:        api.ServerStatusReady,
		Type:          api.ServerTypeIncus,
		Channel:       "stable",
		VersionData: api.ServerVersionData{
			OS: api.OSVersionData{
				Name:        "os",
				Version:     "2",
				VersionNext: "2",
			},
			Applications: []api.ApplicationVersionData{
				{
					Name:    "incus",
					Version: "2",
				},
				{
					Name:    "incus-ceph",
					Version: "2",
				},
			},
		},
	}

	for _, opt := range opts {
		opt(&server)
	}

	return server
}

func withName(name string) serverOpt {
	return func(server *provisioning.Server) {
		server.Name = name
	}
}

//nolint:unparam
func withCluster(cluster *string) serverOpt {
	return func(server *provisioning.Server) {
		server.Cluster = cluster
	}
}

func withClusterCertificate(clusterCertificate *string) serverOpt {
	return func(server *provisioning.Server) {
		server.ClusterCertificate = clusterCertificate
	}
}

func withType(serverType api.ServerType) serverOpt {
	return func(server *provisioning.Server) {
		server.Type = serverType
	}
}

func withStatus(status api.ServerStatus) serverOpt {
	return func(server *provisioning.Server) {
		server.Status = status
	}
}

func withStatusDetail(statusDetail api.ServerStatusDetail) serverOpt {
	return func(server *provisioning.Server) {
		server.StatusDetail = statusDetail
	}
}

func withChannel(channel string) serverOpt {
	return func(server *provisioning.Server) {
		server.Channel = channel
	}
}
