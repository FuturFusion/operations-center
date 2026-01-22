package provisioning_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/provisioning"
)

func TestChannel_Validate(t *testing.T) {
	tests := []struct {
		name    string
		channel provisioning.Channel

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "valid",
			channel: provisioning.Channel{
				Name: "one",
			},

			assertErr: require.NoError,
		},
		{
			name: "error - empty name",
			channel: provisioning.Channel{
				Name: "", // empty name is invalid
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.channel.Validate()

			tc.assertErr(t, err)
		})
	}
}
