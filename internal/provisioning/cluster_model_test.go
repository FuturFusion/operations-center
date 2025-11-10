package provisioning_test

import (
	"database/sql/driver"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/ptr"
	"github.com/FuturFusion/operations-center/shared/api"
)

func TestCluster_ValidateCreate(t *testing.T) {
	tests := []struct {
		name    string
		cluster provisioning.Cluster

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "valid",
			cluster: provisioning.Cluster{
				Name:          "one",
				ServerNames:   []string{"server1", "server2"},
				ConnectionURL: "http://one/",
				ServerType:    api.ServerTypeIncus,
			},

			assertErr: require.NoError,
		},
		{
			name: "error - name empty",
			cluster: provisioning.Cluster{
				Name:          "", // invalid
				ServerNames:   []string{"server1", "server2"},
				ConnectionURL: "http://one/",
				ServerType:    api.ServerTypeIncus,
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
		{
			name: "error - name prohibited character",
			cluster: provisioning.Cluster{
				Name:          "foo/bar", // "/" is prohibited
				ServerNames:   []string{"server1", "server2"},
				ConnectionURL: "http://one/",
				ServerType:    api.ServerTypeIncus,
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
		{
			name: "error - server name list empty",
			cluster: provisioning.Cluster{
				Name:          "one",
				ServerNames:   nil, // invalid
				ConnectionURL: "http://one/",
				ServerType:    api.ServerTypeIncus,
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
		{
			name: "error - connection URL invalid",
			cluster: provisioning.Cluster{
				Name:          "one",
				ServerNames:   []string{"server1", "server2"},
				ConnectionURL: ":|\\", // invalid
				ServerType:    api.ServerTypeIncus,
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
		{
			name: "error - connection URL invalid",
			cluster: provisioning.Cluster{
				Name:          "one",
				ServerNames:   []string{"server1", "server2"},
				ConnectionURL: "http://one/",
				ServerType:    api.ServerTypeUnknown, // invalid
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
		{
			name: "error - application seed config marshal",
			cluster: provisioning.Cluster{
				Name:          "one",
				ServerNames:   []string{"server1", "server2"},
				ConnectionURL: "http://one/",
				ServerType:    api.ServerTypeIncus,
				ApplicationSeedConfig: map[string]any{
					"foo": func() {}, // a func can not be marshalled to JSON.
				},
			},

			assertErr: require.Error,
		},
		{
			name: "error - application seed config unmarshal",
			cluster: provisioning.Cluster{
				Name:          "one",
				ServerNames:   []string{"server1", "server2"},
				ConnectionURL: "http://one/",
				ServerType:    api.ServerTypeIncus,
				ApplicationSeedConfig: map[string]any{
					"networks": map[string]any{}, // networks are expected to be a slice.
				},
			},

			assertErr: require.Error,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cluster.ValidateCreate()

			tc.assertErr(t, err)
		})
	}
}

func TestCluster_Filter(t *testing.T) {
	tests := []struct {
		name   string
		filter provisioning.ClusterFilter

		want string
	}{
		{
			name:   "empty filter",
			filter: provisioning.ClusterFilter{},

			want: ``,
		},
		{
			name: "complete filter",
			filter: provisioning.ClusterFilter{
				Expression: ptr.To("true"),
			},

			want: `filter=true`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, tc.filter.String())
		})
	}
}

func TestClusterArtifactFiles_Value(t *testing.T) {
	tests := []struct {
		name  string
		files provisioning.ClusterArtifactFiles

		assertErr require.ErrorAssertionFunc
		wantValue driver.Value
	}{
		{
			name: "success",
			files: provisioning.ClusterArtifactFiles{
				{
					Name:     "one.txt",
					MimeType: "text/plain",
					Size:     10,
				},
				{
					Name:     "two.txt",
					MimeType: "text/plain",
					Size:     5,
				},
			},

			assertErr: require.NoError,
			wantValue: []byte(`[{"MimeType":"text/plain","Name":"one.txt","Size":10},{"MimeType":"text/plain","Name":"two.txt","Size":5}]`),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.files.Value()

			tc.assertErr(t, err)
			require.Equal(t, tc.wantValue, got)
		})
	}
}

func TestClusterArtifactFiles_Scan(t *testing.T) {
	tests := []struct {
		name string

		value any

		assertErr require.ErrorAssertionFunc
		want      provisioning.ClusterArtifactFiles
	}{
		{
			name: "success - []byte",

			value: []byte(`[{"MimeType":"text/plain","Name":"one.txt","Size":10},{"MimeType":"text/plain","Name":"two.txt","Size":5}]`),

			assertErr: require.NoError,
			want: provisioning.ClusterArtifactFiles{
				{
					Name:     "one.txt",
					MimeType: "text/plain",
					Size:     10,
				},
				{
					Name:     "two.txt",
					MimeType: "text/plain",
					Size:     5,
				},
			},
		},
		{
			name: "success - []byte zero length",

			value: []byte(``),

			assertErr: require.NoError,
			want:      provisioning.ClusterArtifactFiles{},
		},
		{
			name: "success - string",

			value: `[{"MimeType":"text/plain","Name":"one.txt","Size":10},{"MimeType":"text/plain","Name":"two.txt","Size":5}]`,

			assertErr: require.NoError,
			want: provisioning.ClusterArtifactFiles{
				{
					Name:     "one.txt",
					MimeType: "text/plain",
					Size:     10,
				},
				{
					Name:     "two.txt",
					MimeType: "text/plain",
					Size:     5,
				},
			},
		},
		{
			name: "success - string zero length",

			value: ``,

			assertErr: require.NoError,
			want:      provisioning.ClusterArtifactFiles{},
		},
		{
			name: "error - nil",

			assertErr: require.Error,
			want:      provisioning.ClusterArtifactFiles{},
		},
		{
			name: "error - unsupported type",

			value: 1, // not supported for TokenImageSeeds

			assertErr: require.Error,
			want:      provisioning.ClusterArtifactFiles{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clusterArtifactFiles := provisioning.ClusterArtifactFiles{}

			err := clusterArtifactFiles.Scan(tc.value)

			tc.assertErr(t, err)
			require.Equal(t, tc.want, clusterArtifactFiles)
		})
	}
}
