package api_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/util/testing/errassert"
	"github.com/FuturFusion/operations-center/shared/api"
)

func TestValidateIncusImageArchitecture(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		architecture string

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:         "success - amd64",
			architecture: "amd64",

			assertErr: require.NoError,
		},
		{
			name:         "success - arm64",
			architecture: "arm64",

			assertErr: require.NoError,
		},
		{
			name:         "success - armhf",
			architecture: "armhf",

			assertErr: require.NoError,
		},
		{
			name:         "success - riscv64",
			architecture: "riscv64",

			assertErr: require.NoError,
		},
		{
			name:         "error - invalid",
			architecture: "invalid",

			assertErr: errassert.ValidationErrorContains("Invalid incus image, architecture is not supported"),
		},
		{
			name:         "error - empty",
			architecture: "",

			assertErr: errassert.ValidationErrorContains("Invalid incus image, architecture is not supported"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := api.ValidateIncusImageArchitecture(tc.architecture)

			tc.assertErr(t, err)
		})
	}
}

func TestValidateIncusImageVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		version string

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:    "success",
			version: "20260617",

			assertErr: require.NoError,
		},
		{
			name:    "success - with suffix",
			version: "20260617_1430",

			assertErr: require.NoError,
		},
		{
			name:    "error - too short",
			version: "0",

			assertErr: errassert.ValidationErrorContains(`Invalid version, version is required to be a 8 digits long date in the format "yyyymmdd"`),
		},
		{
			name:    "error - not a date",
			version: "invalidversion",

			assertErr: errassert.ValidationErrorContains(`Invalid version, version is required to be a 8 digits long date in the format "yyyymmdd"`),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := api.ValidateIncusImageVersion(tc.version)

			tc.assertErr(t, err)
		})
	}
}
