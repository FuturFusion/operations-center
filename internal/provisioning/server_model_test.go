package provisioning_test

import (
	"testing"

	incusosapi "github.com/lxc/incus-os/incus-osd/api"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/util/ptr"
	"github.com/FuturFusion/operations-center/shared/api"
)

func TestServer_Validate(t *testing.T) {
	tests := []struct {
		name   string
		server provisioning.Server

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "valid",
			server: provisioning.Server{
				Name:          "one",
				Type:          api.ServerTypeIncus,
				Cluster:       ptr.To("one"),
				ConnectionURL: "http://one/",
				Certificate: `-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`,
				Status:  api.ServerStatusReady,
				Channel: "stable",
			},

			assertErr: require.NoError,
		},
		{
			name: "valid - type operations center with empty connection URL",
			server: provisioning.Server{
				Name:          "one",
				Type:          api.ServerTypeOperationsCenter,
				Cluster:       ptr.To("one"),
				ConnectionURL: "",
				Certificate: `-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`,
				Status:  api.ServerStatusReady,
				Channel: "stable",
			},

			assertErr: require.NoError,
		},
		{
			name: "error - name empty",
			server: provisioning.Server{
				Name:          "", // invalid
				Type:          api.ServerTypeIncus,
				Cluster:       ptr.To("one"),
				ConnectionURL: "http://one/",
				Certificate: `-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`,
				Status:  api.ServerStatusReady,
				Channel: "stable",
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
		{
			name: "error - name :self",
			server: provisioning.Server{
				Name:          ":self", // reserved for internal use, not allowed
				Type:          api.ServerTypeIncus,
				Cluster:       ptr.To("one"),
				ConnectionURL: "http://one/",
				Certificate: `-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`,
				Status:  api.ServerStatusReady,
				Channel: "stable",
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
		{
			name: "error - empty type",
			server: provisioning.Server{
				Name:          "one",
				Type:          "", // empty
				Cluster:       ptr.To("one"),
				ConnectionURL: "http://one/",
				Certificate: `-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`,
				Status:  api.ServerStatusReady,
				Channel: "stable",
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
		{
			name: "error - invalid type",
			server: provisioning.Server{
				Name:          "one",
				Type:          api.ServerType("invalid"), // invalid
				Cluster:       ptr.To("one"),
				ConnectionURL: "http://one/",
				Certificate: `-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`,
				Status:  api.ServerStatusReady,
				Channel: "stable",
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
		{
			name: "error - connection URL empty",
			server: provisioning.Server{
				Name:          "one",
				Type:          api.ServerTypeIncus,
				Cluster:       ptr.To("one"),
				ConnectionURL: "", // invalid
				Certificate: `-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`,
				Status:  api.ServerStatusReady,
				Channel: "stable",
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
		{
			name: "error - connection URL invalid",
			server: provisioning.Server{
				Name:          "one",
				Type:          api.ServerTypeIncus,
				Cluster:       ptr.To("one"),
				ConnectionURL: ":|\\", // invalid
				Certificate: `-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`,
				Status:  api.ServerStatusReady,
				Channel: "stable",
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
		{
			name: "error - public connection URL invalid",
			server: provisioning.Server{
				Name:                "one",
				Type:                api.ServerTypeIncus,
				Cluster:             ptr.To("one"),
				ConnectionURL:       "http://one/",
				PublicConnectionURL: ":|\\", // invalid
				Certificate: `-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`,
				Status: api.ServerStatusReady,
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
		{
			name: "error - certificate empty",
			server: provisioning.Server{
				Name:          "one",
				Type:          api.ServerTypeIncus,
				Cluster:       ptr.To("one"),
				ConnectionURL: "http://one/",
				Certificate:   ``, // invalid
				Status:        api.ServerStatusReady,
				Channel:       "stable",
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
		{
			name: "error - status empty",
			server: provisioning.Server{
				Name:          "one",
				Type:          api.ServerTypeIncus,
				Cluster:       ptr.To("one"),
				ConnectionURL: "http://one/",
				Certificate: `-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`,
				Status:  "", // empty
				Channel: "stable",
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
		{
			name: "error - status invalid",
			server: provisioning.Server{
				Name:          "one",
				Type:          api.ServerTypeIncus,
				Cluster:       ptr.To("one"),
				ConnectionURL: "http://one/",
				Certificate: `-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`,
				Status:  api.ServerStatus("invalid"), // invalid
				Channel: "stable",
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
		{
			name: "error - status detail invalid",
			server: provisioning.Server{
				Name:          "one",
				Type:          api.ServerTypeIncus,
				Cluster:       ptr.To("one"),
				ConnectionURL: "http://one/",
				Certificate: `-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`,
				Status:       api.ServerStatusReady,
				StatusDetail: api.ServerStatusDetail("invalid"), // invalid
				Channel:      "stable",
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
		{
			name: "error - channel empty",
			server: provisioning.Server{
				Name:          "one",
				Type:          api.ServerTypeIncus,
				Cluster:       ptr.To("one"),
				ConnectionURL: "http://one/",
				Certificate: `-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`,
				Status:  api.ServerStatusPending,
				Channel: "", // empty
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.server.Validate()

			tc.assertErr(t, err)
		})
	}
}

func TestServer_Filter(t *testing.T) {
	tests := []struct {
		name   string
		filter provisioning.ServerFilter

		want string
	}{
		{
			name:   "empty filter",
			filter: provisioning.ServerFilter{},

			want: ``,
		},
		{
			name: "complete filter",
			filter: provisioning.ServerFilter{
				Cluster:     ptr.To("cluster"),
				Status:      ptr.To(api.ServerStatusReady),
				Certificate: ptr.To("certificate"),
				Type:        ptr.To(api.ServerTypeIncus),
				Expression:  ptr.To("true"),
			},

			want: `certificate=certificate&cluster=cluster&filter=true&status=ready&type=incus`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, tc.filter.String())
		})
	}
}

func TestServer_Getters(t *testing.T) {
	server := provisioning.Server{
		Name:          "server1",
		ConnectionURL: "http://example.com:443/",
		Certificate:   "cert",
	}

	require.Equal(t, server.Name, server.GetName())
	require.Equal(t, server.Certificate, server.GetCertificate())
	require.Equal(t, server.ConnectionURL, server.GetConnectionURL())

	// Cluster with not cluster certificate set.
	server.Cluster = ptr.To("cluster")
	require.Empty(t, server.GetCertificate())

	// Cluster with cluster certificate set.
	server.ClusterCertificate = ptr.To("cluster cert")
	require.Equal(t, *server.ClusterCertificate, server.GetCertificate())
}

func TestServer_Clone(t *testing.T) {
	server := provisioning.Server{
		Name:    "name",
		Cluster: ptr.To("cluster"),
		Type:    api.ServerTypeIncus,
		Status:  api.ServerStatusReady,
		VersionData: api.ServerVersionData{
			Applications: []api.ApplicationVersionData{
				{
					Name: "one",
				},
				{
					Name: "two",
				},
			},
		},
	}

	cloned := server.Clone()

	require.Equal(t, server, cloned)

	// Mutate the source
	server.Name = "new name"
	server.VersionData.Applications[0].Name = "new"
	server.VersionData.Applications = server.VersionData.Applications[:1]

	require.NotEqual(t, server.Name, cloned.Name)
	require.NotEqual(t, server.VersionData.Applications[0].Name, cloned.VersionData.Applications[0].Name)
	require.NotEqual(t, len(server.VersionData.Applications), len(cloned.VersionData.Applications))
}

func TestServer_GetServerName(t *testing.T) {
	tests := []struct {
		name   string
		server provisioning.Server

		assertErr  require.ErrorAssertionFunc
		serverName string
	}{
		{
			name: "success - server",
			server: provisioning.Server{
				ConnectionURL: "http://example.com:443/",
			},

			assertErr:  require.NoError,
			serverName: "example.com",
		},
		{
			name: "success - cluster",
			server: provisioning.Server{
				ConnectionURL:        "http://example.com:443/",
				ClusterConnectionURL: ptr.To("http://cluster.com:443/"),
			},

			assertErr:  require.NoError,
			serverName: "cluster.com",
		},
		{
			name: "error - invalid connection URL",
			server: provisioning.Server{
				ConnectionURL: ":|\\", // invalid
			},

			assertErr:  require.Error,
			serverName: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			serverName, err := tc.server.GetServerName()
			tc.assertErr(t, err)

			require.Equal(t, tc.serverName, serverName)
		})
	}
}

func TestDetermineManagementRoleURL(t *testing.T) {
	tests := []struct {
		name string
		in   api.OSData

		assertErr require.ErrorAssertionFunc
		want      string
	}{
		{
			name: "success",
			in: api.OSData{
				Network: incusosapi.SystemNetwork{
					State: incusosapi.SystemNetworkState{
						Interfaces: map[string]incusosapi.SystemNetworkInterfaceState{
							"eth0": {
								Addresses: []string{
									"1.2.3.4",
								},
								Roles: []string{incusosapi.SystemNetworkInterfaceRoleManagement},
							},
						},
					},
				},
			},

			assertErr: require.NoError,
			want:      "https://1.2.3.4:8443",
		},
		{
			name: "error",
			in: api.OSData{
				Network: incusosapi.SystemNetwork{
					State: incusosapi.SystemNetworkState{
						Interfaces: map[string]incusosapi.SystemNetworkInterfaceState{
							"eth0": {
								Addresses: []string{
									"1.2.3.4",
								},
								Roles: []string{},
							},
						},
					},
				},
			},

			assertErr: require.Error,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := provisioning.DetermineManagementRoleURL(tc.in)

			tc.assertErr(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}
