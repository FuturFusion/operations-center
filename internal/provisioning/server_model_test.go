package provisioning_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	incusosapi "github.com/lxc/incus-os/incus-osd/api"
	"github.com/lxc/incus-os/incus-osd/api/images"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/provisioning"
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
				Cluster:       new("one"),
				ConnectionURL: "http://one/",
				Certificate: new(`-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`),
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
				Cluster:       new("one"),
				ConnectionURL: "",
				Certificate: new(`-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`),
				Status:  api.ServerStatusReady,
				Channel: "stable",
			},

			assertErr: require.NoError,
		},
		{
			name: "valid - with BMC",
			server: provisioning.Server{
				Name:          "one",
				Type:          api.ServerType(""), // not yet defined at this point
				Cluster:       nil,                // not yet part of a cluster when created with BMC
				ConnectionURL: "",                 // not yet known when created with BMC
				Certificate:   nil,                // not yet known when created with BMC
				Status:        api.ServerStatusUnregistered,
				Channel:       "stable",
				BMCConfig: api.BMCConfig{
					APIType:  api.BMCAPITypeRedfishV1Generic,
					Endpoint: "https://1.2.3.4",
				},
			},

			assertErr: require.NoError,
		},
		{
			name: "error - name empty",
			server: provisioning.Server{
				Name:          "", // invalid
				Type:          api.ServerTypeIncus,
				Cluster:       new("one"),
				ConnectionURL: "http://one/",
				Certificate: new(`-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`),
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
				Cluster:       new("one"),
				ConnectionURL: "http://one/",
				Certificate: new(`-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`),
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
				Cluster:       new("one"),
				ConnectionURL: "http://one/",
				Certificate: new(`-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`),
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
				Cluster:       new("one"),
				ConnectionURL: "http://one/",
				Certificate: new(`-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`),
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
				Cluster:       new("one"),
				ConnectionURL: "", // invalid
				Certificate: new(`-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`),
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
				Cluster:       new("one"),
				ConnectionURL: ":|\\", // invalid
				Certificate: new(`-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`),
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
				Cluster:             new("one"),
				ConnectionURL:       "http://one/",
				PublicConnectionURL: ":|\\", // invalid
				Certificate: new(`-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`),
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
				Cluster:       new("one"),
				ConnectionURL: "http://one/",
				Certificate:   new(``), // invalid
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
				Cluster:       new("one"),
				ConnectionURL: "http://one/",
				Certificate: new(`-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`),
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
				Cluster:       new("one"),
				ConnectionURL: "http://one/",
				Certificate: new(`-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`),
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
				Cluster:       new("one"),
				ConnectionURL: "http://one/",
				Certificate: new(`-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`),
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
				Cluster:       new("one"),
				ConnectionURL: "http://one/",
				Certificate: new(`-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`),
				Status:  api.ServerStatusPending,
				Channel: "", // empty
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
		{
			name: "error - BMC type invalid",
			server: provisioning.Server{
				Name:          "one",
				Type:          api.ServerTypeIncus,
				Cluster:       new("one"),
				ConnectionURL: "http://one/",
				Certificate: new(`-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`),
				Status:  api.ServerStatusPending,
				Channel: "stable",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPIType("invalid"), // invalid
				},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
		{
			name: "error - BMC endpoint empty with BMC type not none",
			server: provisioning.Server{
				Name:          "one",
				Type:          api.ServerTypeIncus,
				Cluster:       new("one"),
				ConnectionURL: "http://one/",
				Certificate: new(`-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`),
				Status:  api.ServerStatusPending,
				Channel: "stable",
				BMCConfig: api.BMCConfig{
					APIType:  api.BMCAPITypeRedfishV1Generic,
					Endpoint: "", // empty
				},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
		{
			name: "error - BMC endpoint invalid",
			server: provisioning.Server{
				Name:          "one",
				Type:          api.ServerTypeIncus,
				Cluster:       new("one"),
				ConnectionURL: "http://one/",
				Certificate: new(`-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`),
				Status:  api.ServerStatusPending,
				Channel: "stable",
				BMCConfig: api.BMCConfig{
					APIType:  api.BMCAPITypeRedfishV1Generic,
					Endpoint: ":|\\", // invalid
				},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
		{
			name: "error - BMC certificate invalid",
			server: provisioning.Server{
				Name:          "one",
				Type:          api.ServerTypeIncus,
				Cluster:       new("one"),
				ConnectionURL: "http://one/",
				Certificate: new(`-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`),
				Status:  api.ServerStatusPending,
				Channel: "stable",
				BMCConfig: api.BMCConfig{
					APIType:     api.BMCAPITypeRedfishV1Generic,
					Endpoint:    "https://1.2.3.4",
					Certificate: "invalid", // invalid
				},
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

func TestServer_UpdateState(t *testing.T) {
	tests := []struct {
		name   string
		server provisioning.Server

		want api.ServerUpdateState
	}{
		{
			name: "success",
			server: provisioning.Server{
				Status: api.ServerStatusReady,
			},

			want: api.ServerUpdateStateUpToDate,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			state := tc.server.UpdateState()

			require.Equal(t, tc.want, state)
		})
	}
}

func TestServer_Filter(t *testing.T) {
	tests := []struct {
		name   string
		filter provisioning.ServerFilter

		want      string
		wantEmpty bool
	}{
		{
			name:   "empty filter",
			filter: provisioning.ServerFilter{},

			want:      ``,
			wantEmpty: true,
		},
		{
			name: "complete filter",
			filter: provisioning.ServerFilter{
				Cluster:     new("cluster"),
				Status:      new(api.ServerStatusReady),
				Certificate: new("certificate"),
				Type:        new(api.ServerTypeIncus),
				Expression:  new("true"),
			},

			want:      `certificate=certificate&cluster=cluster&filter=true&status=ready&type=incus`,
			wantEmpty: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, tc.filter.String())
			require.Equal(t, tc.wantEmpty, tc.filter.IsEmpty())
		})
	}
}

func TestServer_Getters(t *testing.T) {
	server := provisioning.Server{
		Name:          "server1",
		ConnectionURL: "http://example.com:443/",
		Certificate:   new("cert"),
	}

	require.Equal(t, server.Name, server.GetName())
	require.Equal(t, *server.Certificate, server.GetCertificate())
	require.Equal(t, server.ConnectionURL, server.GetConnectionURL())

	// Cluster with not cluster certificate set.
	server.Cluster = new("cluster")
	require.Empty(t, server.GetCertificate())

	// Cluster with cluster certificate set.
	server.ClusterCertificate = new("cluster cert")
	require.Equal(t, *server.ClusterCertificate, server.GetCertificate())
}

func TestServer_Clone(t *testing.T) {
	server := provisioning.Server{
		Name:    "name",
		Cluster: new("cluster"),
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

func TestServer_Clone_statusInternal(t *testing.T) {
	startedAt := time.Date(2026, 8, 31, 10, 0, 0, 0, time.UTC)

	server := provisioning.Server{
		Name:    "name",
		Type:    api.ServerTypeIncus,
		Status:  api.ServerStatusDeploying,
		Channel: "stable",
		StatusInternal: provisioning.ServerStatusInternal{
			Deployment: &provisioning.ServerDeployment{
				State: api.ServerDeploymentStateWaitInstall,
				Request: provisioning.ServerDeploymentRequest{
					TokenUUID:      uuid.MustParse("e9de436e-b94e-4aef-8563-883aec84096e"),
					Seed:           "default",
					ImageType:      api.ImageTypeISO,
					Architecture:   images.UpdateFileArchitecture64BitX86,
					VirtualMediaID: "system:1",
				},
				ForceReboot:    true,
				BIOSProfiles:   []string{"profile"},
				BIOSAttributes: map[string]any{"BootMode": "Uefi"},
				MediaURL:       "https://oc.local/image.iso",
				MediaBytesRead: 42,
				StartedAt:      startedAt,
				StateEnteredAt: startedAt,
				History: []api.ServerDeploymentStep{
					{State: api.ServerDeploymentStateAttachMedia, EnteredAt: startedAt},
				},
			},
		},
	}

	cloned := server.Clone()

	require.Equal(t, server.StatusInternal, cloned.StatusInternal)

	// The clone is deep, so mutating the source does not reach it.
	server.StatusInternal.Deployment.State = api.ServerDeploymentStateFailed

	require.Equal(t, api.ServerDeploymentStateWaitInstall, cloned.StatusInternal.Deployment.State)
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
				ClusterConnectionURL: new("http://cluster.com:443/"),
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
