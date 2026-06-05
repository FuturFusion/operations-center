package image_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/image"
)

func TestIncusImage_Validate(t *testing.T) {
	tests := []struct {
		name  string
		image image.IncusImage

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "valid",
			image: image.IncusImage{
				Name:            "almalinux:10:amd64:cloud",
				OperatingSystem: "almalinux",
				Release:         "10",
				Architecture:    "amd64",
				Variant:         "cloud",
				Aliases:         []string{"almalinux/10/cloud/amd64", "some/alias"},
			},

			assertErr: require.NoError,
		},
		{
			name: "error - empty name",
			image: image.IncusImage{
				Name: "", // empty name is invalid
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
		{
			name: "error - os empty in name",
			image: image.IncusImage{
				Name: ":10:amd64:cloud", // os empty in name
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
		{
			name: "error - release empty in name",
			image: image.IncusImage{
				Name: "almalinux::amd64:cloud", // release empty in name
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
		{
			name: "error - architecture unknown",
			image: image.IncusImage{
				Name: "almalinux:10:invalid:cloud", // architecture unknown
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
		{
			name: "error - variant empty in name",
			image: image.IncusImage{
				Name: "almalinux:10:amd64:", // variant empty in name
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
		{
			name: "error - empty operating system",
			image: image.IncusImage{
				Name:            "almalinux:10:amd64:cloud",
				OperatingSystem: "", // empty operating system
				Release:         "10",
				Architecture:    "amd64",
				Variant:         "cloud",
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
		{
			name: "error - empty release",
			image: image.IncusImage{
				Name:            "almalinux:10:amd64:cloud",
				OperatingSystem: "almalinux",
				Release:         "", // empty release
				Architecture:    "amd64",
				Variant:         "cloud",
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
		{
			name: "error - empty arch",
			image: image.IncusImage{
				Name:            "almalinux:10:amd64:cloud",
				OperatingSystem: "almalinux",
				Release:         "10",
				Architecture:    "", // empty arch
				Variant:         "cloud",
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
		{
			name: "error - empty variant",
			image: image.IncusImage{
				Name:            "almalinux:10:amd64:cloud",
				OperatingSystem: "almalinux",
				Release:         "10",
				Architecture:    "amd64",
				Variant:         "", // empty variant
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
		{
			name: "error - name properties mismatch",
			image: image.IncusImage{
				Name:            "almalinux:10:amd64:cloud",
				OperatingSystem: "alpine", // does not match name
				Release:         "10",
				Architecture:    "amd64",
				Variant:         "cloud",
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
		{
			name: "error - redundant alias",
			image: image.IncusImage{
				Name:            "almalinux:10:amd64:cloud",
				OperatingSystem: "almalinux",
				Release:         "10",
				Architecture:    "amd64",
				Variant:         "cloud",
				Aliases:         []string{"almalinux/10/cloud/amd64", "some/alias", "almalinux/10/cloud/amd64"},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.image.Validate()

			tc.assertErr(t, err)
		})
	}
}

func TestIncusImage_FilePath(t *testing.T) {
	tests := []struct {
		name  string
		image image.IncusImage

		wantPath string
	}{
		{
			name: "success",
			image: image.IncusImage{
				OperatingSystem: "almalinux",
				Release:         "10",
				Architecture:    "amd64",
				Variant:         "cloud",
			},

			wantPath: "almalinux/10/amd64/cloud",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := tc.image.FilePath()

			require.Equal(t, tc.wantPath, path)
		})
	}
}
