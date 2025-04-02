package provisioning_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/ptr"
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
				Cluster:       ptr.To("one"),
				ConnectionURL: "http://one/",
			},

			assertErr: require.NoError,
		},
		{
			name: "error - name empty",
			server: provisioning.Server{
				Name:          "", // invalid
				Cluster:       ptr.To("one"),
				ConnectionURL: "http://one/",
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
				Cluster:       ptr.To("one"),
				ConnectionURL: "", // invalid
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
				Cluster:       ptr.To("one"),
				ConnectionURL: ":|\\", // invalid
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
