package image_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/image"
	"github.com/FuturFusion/operations-center/internal/util/testing/errassert"
	"github.com/FuturFusion/operations-center/shared/api"
)

func TestIncusImage_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		image image.IncusImage

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "valid",
			image: image.IncusImage{
				Source:          api.IncusImageSourceLocal,
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
			name: "valid - properties in name in lower case",
			image: image.IncusImage{
				Source:          api.IncusImageSourceLocal,
				Name:            "almalinux:10-beta:amd64:cloud",
				OperatingSystem: "Almalinux",
				Release:         "10-Beta",
				Architecture:    "amd64",
				Variant:         "Cloud",
			},

			assertErr: require.NoError,
		},
		{
			name: "error - invalid source",
			image: image.IncusImage{
				Source:          "my-source",
				Name:            "almalinux:10:amd64:cloud",
				OperatingSystem: "almalinux",
				Release:         "10",
				Architecture:    "amd64",
				Variant:         "cloud",
			},

			assertErr: errassert.ValidationErrorContains(`Invalid source, name "my-source" can only contain lower case letters, digits and underscores`),
		},
		{
			name: "error - empty name",
			image: image.IncusImage{
				Source: api.IncusImageSourceLocal,
				Name:   "", // empty name is invalid
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
		{
			name: "error - os empty in name",
			image: image.IncusImage{
				Source: api.IncusImageSourceLocal,
				Name:   ":10:amd64:cloud", // os empty in name
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
		{
			name: "error - release empty in name",
			image: image.IncusImage{
				Source: api.IncusImageSourceLocal,
				Name:   "almalinux::amd64:cloud", // release empty in name
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
		{
			name: "error - architecture unknown",
			image: image.IncusImage{
				Source: api.IncusImageSourceLocal,
				Name:   "almalinux:10:invalid:cloud", // architecture unknown
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
		{
			name: "error - variant empty in name",
			image: image.IncusImage{
				Source: api.IncusImageSourceLocal,
				Name:   "almalinux:10:amd64:", // variant empty in name
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
		{
			name: "error - empty operating system",
			image: image.IncusImage{
				Source:          api.IncusImageSourceLocal,
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
				Source:          api.IncusImageSourceLocal,
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
			name: "error - release is parent directory",
			image: image.IncusImage{
				Source:          api.IncusImageSourceLocal,
				Name:            "almalinux:..:amd64:cloud",
				OperatingSystem: "almalinux",
				Release:         "..",
				Architecture:    "amd64",
				Variant:         "cloud",
			},

			assertErr: errassert.ValidationErrorContains(`Invalid incus image, ".." is not a valid file name`),
		},
		{
			name: "error - variant with path separator",
			image: image.IncusImage{
				Source:          api.IncusImageSourceLocal,
				Name:            "almalinux:10:amd64:cloud/../..",
				OperatingSystem: "almalinux",
				Release:         "10",
				Architecture:    "amd64",
				Variant:         "cloud/../..",
			},

			assertErr: errassert.ValidationErrorContains(`Invalid incus image, "cloud/../.." is not a valid file name`),
		},
		{
			name: "error - empty arch",
			image: image.IncusImage{
				Source:          api.IncusImageSourceLocal,
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
				Source:          api.IncusImageSourceLocal,
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
			name: "error - name not in lower case",
			image: image.IncusImage{
				Source:          api.IncusImageSourceLocal,
				Name:            "Almalinux:10:amd64:cloud",
				OperatingSystem: "Almalinux",
				Release:         "10",
				Architecture:    "amd64",
				Variant:         "cloud",
			},

			assertErr: errassert.ValidationErrorContains(`Invalid incus image, name needs to match "os:release:architecture:variant" in lower case`),
		},
		{
			name: "error - name properties mismatch",
			image: image.IncusImage{
				Source:          api.IncusImageSourceLocal,
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
				Source:          api.IncusImageSourceLocal,
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
			t.Parallel()

			err := tc.image.Validate()

			tc.assertErr(t, err)
		})
	}
}

func TestValidateIncusImageFilename(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		filename string

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:     "success",
			filename: "root.squashfs",

			assertErr: require.NoError,
		},
		{
			name:     "error - empty",
			filename: "",

			assertErr: errassert.ValidationErrorContains(`Invalid incus image, "" is not a valid file name`),
		},
		{
			name:     "error - current directory",
			filename: ".",

			assertErr: errassert.ValidationErrorContains(`Invalid incus image, "." is not a valid file name`),
		},
		{
			name:     "error - parent directory",
			filename: "..",

			assertErr: errassert.ValidationErrorContains(`Invalid incus image, ".." is not a valid file name`),
		},
		{
			name:     "error - relative path",
			filename: "../root.squashfs",

			assertErr: errassert.ValidationErrorContains(`Invalid incus image, "../root.squashfs" is not a valid file name`),
		},
		{
			name:     "error - absolute path",
			filename: "/root.squashfs",

			assertErr: errassert.ValidationErrorContains(`Invalid incus image, "/root.squashfs" is not a valid file name`),
		},
		{
			name:     "error - backslash",
			filename: `..\root.squashfs`,

			assertErr: errassert.ValidationErrorContains(`is not a valid file name`),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := image.ValidateIncusImageFilename(tc.filename)

			tc.assertErr(t, err)
		})
	}
}

func TestIncusImage_FilePath(t *testing.T) {
	t.Parallel()

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
			t.Parallel()

			path := tc.image.FilePath()

			require.Equal(t, tc.wantPath, path)
		})
	}
}
