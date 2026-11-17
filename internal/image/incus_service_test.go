package image_test

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/textproto"
	"path"
	"slices"
	"strings"
	"testing"
	"testing/iotest"

	incusapi "github.com/lxc/incus/v7/shared/api"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v4"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/image"
	adapterMock "github.com/FuturFusion/operations-center/internal/image/adapter/mock"
	"github.com/FuturFusion/operations-center/internal/image/repo/mock"
	"github.com/FuturFusion/operations-center/internal/util/archive/xz"
	"github.com/FuturFusion/operations-center/internal/util/file"
	"github.com/FuturFusion/operations-center/internal/util/testing/boom"
	"github.com/FuturFusion/operations-center/internal/util/testing/errassert"
	"github.com/FuturFusion/operations-center/internal/util/testing/queue"
	"github.com/FuturFusion/operations-center/shared/api"
)

func TestImageIncusService_AddVersion(t *testing.T) {
	t.Parallel()

	type fileRepoPutValue struct {
		commitErr error
		cancelErr error
	}

	type fileRepoGetValue struct {
		reader io.ReadCloser
		size   int64
	}

	tests := []struct {
		name               string
		multipartReaderArg *multipart.Reader
		repoGetByName      []queue.Item[*image.IncusImage]
		repoCreateErr      error
		filesRepoPut       []queue.Item[fileRepoPutValue]
		filesRepoGet       []queue.Item[fileRepoGetValue]
		repoUpdateErr      error

		assertErr        require.ErrorAssertionFunc
		wantPutFilenames []string
		wantItemFileType map[string]string
		wantItemPath     map[string]string
	}{
		{
			name:               "success - with metadata from incus.tar.xz",
			multipartReaderArg: validMultipartReaderWithIncusTarXZ(t),
			repoGetByName: []queue.Item[*image.IncusImage]{
				// lookup
				{
					Value: &image.IncusImage{
						Source:          api.IncusImageSourceLocal,
						Name:            "almalinux:10:amd64:cloud",
						OperatingSystem: "AlmaLinux",
						Release:         "10",
						Architecture:    "amd64",
						Variant:         "cloud",
					},
				},
				// in transaction, get before update
				{
					Value: &image.IncusImage{
						Source:          api.IncusImageSourceLocal,
						Name:            "almalinux:10:amd64:cloud",
						OperatingSystem: "almalinux",
						Release:         "10",
						Architecture:    "amd64",
						Variant:         "cloud",
					},
				},
			},
			filesRepoPut: []queue.Item[fileRepoPutValue]{
				{},
				{},
				{},
				{},
			},
			filesRepoGet: []queue.Item[fileRepoGetValue]{
				// incus.tar.xz for root.tar.xz
				{
					Value: fileRepoGetValue{
						reader: io.NopCloser(bytes.NewBufferString(`incus tar xz`)),
						size:   int64(len(`incus tar xz`)),
					},
				},
				// root.tar.xz
				{
					Value: fileRepoGetValue{
						reader: io.NopCloser(bytes.NewBufferString(`root tar xz`)),
						size:   int64(len(`root tar xz`)),
					},
				},
				// incus.tar.xz for root.squashfs
				{
					Value: fileRepoGetValue{
						reader: io.NopCloser(bytes.NewBufferString(`incus tar xz`)),
						size:   int64(len(`incus tar xz`)),
					},
				},
				// root.squashfs
				{
					Value: fileRepoGetValue{
						reader: io.NopCloser(bytes.NewBufferString(`squashfs`)),
						size:   int64(len(`squashfs`)),
					},
				},
				// incus.tar.xz for disk.qcow2
				{
					Value: fileRepoGetValue{
						reader: io.NopCloser(bytes.NewBufferString(`incus tar xz`)),
						size:   int64(len(`incus tar xz`)),
					},
				},
				// disk.qcow2
				{
					Value: fileRepoGetValue{
						reader: io.NopCloser(bytes.NewBufferString(`disk qcow2`)),
						size:   int64(len(`disk qcow2`)),
					},
				},
			},

			assertErr:        require.NoError,
			wantPutFilenames: []string{"incus.tar.xz", "root.tar.xz", "root.squashfs", "disk.qcow2"},
			wantItemFileType: map[string]string{
				"incus.tar.xz":  "incus.tar.xz",
				"root.tar.xz":   "root.tar.xz",
				"root.squashfs": "squashfs",
				"disk.qcow2":    "disk-kvm.img",
			},
			wantItemPath: map[string]string{
				"incus.tar.xz":  "images/local/almalinux/10/amd64/cloud/20260515/incus.tar.xz",
				"root.tar.xz":   "images/local/almalinux/10/amd64/cloud/20260515/root.tar.xz",
				"root.squashfs": "images/local/almalinux/10/amd64/cloud/20260515/root.squashfs",
				"disk.qcow2":    "images/local/almalinux/10/amd64/cloud/20260515/disk.qcow2",
			},
		},
		{
			name:               "success - new incus image with metadata from incus.tar.xz",
			multipartReaderArg: validMultipartReaderWithIncusTarXZ(t),
			repoGetByName: []queue.Item[*image.IncusImage]{
				// lookup
				{
					Err: domain.ErrNotFound,
				},
				// in transaction, get before update
				{
					Value: &image.IncusImage{
						Source:          api.IncusImageSourceLocal,
						Name:            "almalinux:10:amd64:cloud",
						OperatingSystem: "almalinux",
						Release:         "10",
						Architecture:    "amd64",
						Variant:         "cloud",
					},
				},
			},
			filesRepoPut: []queue.Item[fileRepoPutValue]{
				{},
				{},
				{},
				{},
			},
			filesRepoGet: []queue.Item[fileRepoGetValue]{
				// incus.tar.xz for root.tar.xz
				{
					Value: fileRepoGetValue{
						reader: io.NopCloser(bytes.NewBufferString(`incus tar xz`)),
						size:   int64(len(`incus tar xz`)),
					},
				},
				// root.tar.xz
				{
					Value: fileRepoGetValue{
						reader: io.NopCloser(bytes.NewBufferString(`root tar xz`)),
						size:   int64(len(`root tar xz`)),
					},
				},
				// incus.tar.xz for root.squashfs
				{
					Value: fileRepoGetValue{
						reader: io.NopCloser(bytes.NewBufferString(`incus tar xz`)),
						size:   int64(len(`incus tar xz`)),
					},
				},
				// root.squashfs
				{
					Value: fileRepoGetValue{
						reader: io.NopCloser(bytes.NewBufferString(`squashfs`)),
						size:   int64(len(`squashfs`)),
					},
				},
				// incus.tar.xz for disk.qcow2
				{
					Value: fileRepoGetValue{
						reader: io.NopCloser(bytes.NewBufferString(`incus tar xz`)),
						size:   int64(len(`incus tar xz`)),
					},
				},
				// disk.qcow2
				{
					Value: fileRepoGetValue{
						reader: io.NopCloser(bytes.NewBufferString(`disk qcow2`)),
						size:   int64(len(`disk qcow2`)),
					},
				},
			},

			assertErr:        require.NoError,
			wantPutFilenames: []string{"incus.tar.xz", "root.tar.xz", "root.squashfs", "disk.qcow2"},
			wantItemFileType: map[string]string{
				"disk.qcow2":    "disk-kvm.img",
				"incus.tar.xz":  "incus.tar.xz",
				"root.squashfs": "squashfs",
				"root.tar.xz":   "root.tar.xz",
			},
			wantItemPath: map[string]string{
				"disk.qcow2":    "images/local/almalinux/10/amd64/cloud/20260515/disk.qcow2",
				"incus.tar.xz":  "images/local/almalinux/10/amd64/cloud/20260515/incus.tar.xz",
				"root.squashfs": "images/local/almalinux/10/amd64/cloud/20260515/root.squashfs",
				"root.tar.xz":   "images/local/almalinux/10/amd64/cloud/20260515/root.tar.xz",
			},
		},
		{
			name: "success - new incus image with metadata from request_json",
			multipartReaderArg: validMultipartReaderWithRequestJSON(t, `{
  "os": "almalinux",
  "release": "",
  "arch": "amd64",
  "variant": "",
  "version": "20260515"
}
`), // use default release and variant
			repoGetByName: []queue.Item[*image.IncusImage]{
				// lookup
				{
					Err: domain.ErrNotFound,
				},
				// in transaction, get before update
				{
					Value: &image.IncusImage{
						Source:          api.IncusImageSourceLocal,
						Name:            "almalinux:10:amd64:cloud",
						OperatingSystem: "almalinux",
						Release:         "10",
						Architecture:    "amd64",
						Variant:         "cloud",
					},
				},
			},
			filesRepoPut: []queue.Item[fileRepoPutValue]{
				{},
				{},
			},
			filesRepoGet: []queue.Item[fileRepoGetValue]{
				// incus.tar.xz for disk.qcow2
				{
					Value: fileRepoGetValue{
						reader: io.NopCloser(bytes.NewBufferString(`incus tar xz`)),
						size:   int64(len(`incus tar xz`)),
					},
				},
				// disk.qcow2
				{
					Value: fileRepoGetValue{
						reader: io.NopCloser(bytes.NewBufferString(`disk qcow2`)),
						size:   int64(len(`disk qcow2`)),
					},
				},
			},

			assertErr:        require.NoError,
			wantPutFilenames: []string{"incus.tar.xz", "disk.qcow2"},
			wantItemFileType: map[string]string{
				"incus.tar.xz": "incus.tar.xz",
				"disk.qcow2":   "disk-kvm.img",
			},
			wantItemPath: map[string]string{
				"incus.tar.xz": "images/local/almalinux/current/amd64/default/20260515/incus.tar.xz",
				"disk.qcow2":   "images/local/almalinux/current/amd64/default/20260515/disk.qcow2",
			},
		},
		{
			name:               "error - invalid multipart reader",
			multipartReaderArg: multipart.NewReader(bytes.NewBufferString(`invalid`), ``),

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, "Add version failed to get first multipart item: multipart: boundary is empty", a...)
			},
			wantPutFilenames: nil,
			wantItemFileType: map[string]string{},
			wantItemPath:     map[string]string{},
		},
		{
			name:               "error - empty multipart request",
			multipartReaderArg: multipart.NewReader(bytes.NewBufferString(""), "x"),

			assertErr:        errassert.ValidationErrorContains("The multipart request has no parts, the metadata tarball is required"),
			wantPutFilenames: nil,
			wantItemFileType: map[string]string{},
			wantItemPath:     map[string]string{},
		},
		{
			name:               "error - multipart request with closing boundary only",
			multipartReaderArg: multipart.NewReader(bytes.NewBufferString("--x--\r\n"), "x"),

			assertErr:        errassert.ValidationErrorContains("The multipart request has no parts, the metadata tarball is required"),
			wantPutFilenames: nil,
			wantItemFileType: map[string]string{},
			wantItemPath:     map[string]string{},
		},
		{
			name:               "error - unsupported image file",
			multipartReaderArg: multipartReaderWithUnsupportedFile(t),
			repoGetByName: []queue.Item[*image.IncusImage]{
				{
					Value: &image.IncusImage{
						Source:          api.IncusImageSourceLocal,
						Name:            "almalinux:10:amd64:cloud",
						OperatingSystem: "almalinux",
						Release:         "10",
						Architecture:    "amd64",
						Variant:         "cloud",
					},
				},
			},
			filesRepoPut: []queue.Item[fileRepoPutValue]{
				{},
			},

			assertErr:        errassert.ValidationErrorContains(`Unsupported image file "rootfs.img"`),
			wantPutFilenames: []string{"incus.tar.xz"},
			wantItemFileType: map[string]string{},
			wantItemPath:     map[string]string{},
		},
		{
			name:               "error - first part is not a metadata tarball",
			multipartReaderArg: multipartReaderWithoutMetadataFile(t),

			assertErr:        errassert.ValidationErrorContains(`The first part of the multipart request "root.tar.xz" is not a valid metadata tarball`),
			wantPutFilenames: nil,
			wantItemFileType: map[string]string{},
			wantItemPath:     map[string]string{},
		},
		{
			name:               "error - failed to read metadata",
			multipartReaderArg: validMultipartReaderWithRequestJSON(t, `{`), // invalid metadata

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, "Failed to read metadata")
			},
			wantPutFilenames: nil,
			wantItemFileType: map[string]string{},
			wantItemPath:     map[string]string{},
		},
		{
			name: "error - missing metadata",
			multipartReaderArg: validMultipartReaderWithRequestJSON(t, `{
  "os": "",
  "release": "10",
  "arch": "amd64",
  "variant": "cloud",
  "version": "20260515"
}
`), // empty os

			assertErr:        errassert.ValidationErrorContains("Incomplete metadata, not all required properties set"),
			wantPutFilenames: nil,
			wantItemFileType: map[string]string{},
			wantItemPath:     map[string]string{},
		},
		{
			name: "error - invalid version",
			multipartReaderArg: validMultipartReaderWithRequestJSON(t, `{
  "os": "almalinux",
  "release": "10",
  "arch": "amd64",
  "variant": "cloud",
  "version": "invalid"
}
`), // invalid version

			assertErr:        errassert.ValidationErrorContains("Invalid incus image version"),
			wantPutFilenames: nil,
			wantItemFileType: map[string]string{},
			wantItemPath:     map[string]string{},
		},
		{
			name: "error - version with path separator",
			multipartReaderArg: validMultipartReaderWithRequestJSON(t, `{
  "os": "almalinux",
  "release": "10",
  "arch": "amd64",
  "variant": "cloud",
  "version": "20260515/../../escape"
}
`),

			assertErr:        errassert.ValidationErrorContains("Invalid incus image version"),
			wantPutFilenames: nil,
			wantItemFileType: map[string]string{},
			wantItemPath:     map[string]string{},
		},
		{
			name: "error - release with path separator",
			multipartReaderArg: validMultipartReaderWithRequestJSON(t, `{
  "os": "almalinux",
  "release": "../../../escape",
  "arch": "amd64",
  "variant": "cloud",
  "version": "20260515"
}
`),

			assertErr:        errassert.ValidationErrorContains(`Invalid metadata: Invalid incus image, "../../../escape" is not a valid file name`),
			wantPutFilenames: nil,
			wantItemFileType: map[string]string{},
			wantItemPath:     map[string]string{},
		},
		{
			name: "error - operating system is parent directory",
			multipartReaderArg: validMultipartReaderWithRequestJSON(t, `{
  "os": "..",
  "release": "10",
  "arch": "amd64",
  "variant": "cloud",
  "version": "20260515"
}
`),

			assertErr:        errassert.ValidationErrorContains(`Invalid metadata: Invalid incus image, ".." is not a valid file name`),
			wantPutFilenames: nil,
			wantItemFileType: map[string]string{},
			wantItemPath:     map[string]string{},
		},
		{
			name: "error - architecture not supported",
			multipartReaderArg: validMultipartReaderWithRequestJSON(t, `{
  "os": "almalinux",
  "release": "10",
  "arch": "s390x",
  "variant": "cloud",
  "version": "20260515"
}
`),

			assertErr:        errassert.ValidationErrorContains(`Invalid metadata: Invalid incus image, architecture is not supported`),
			wantPutFilenames: nil,
			wantItemFileType: map[string]string{},
			wantItemPath:     map[string]string{},
		},
		{
			name: "error - release with colon",
			multipartReaderArg: validMultipartReaderWithRequestJSON(t, `{
  "os": "almalinux",
  "release": "10:beta",
  "arch": "amd64",
  "variant": "cloud",
  "version": "20260515"
}
`),

			assertErr:        errassert.ValidationErrorContains(`Invalid metadata: Invalid incus image name, expect name in the format "os:release:architecture:variant"`),
			wantPutFilenames: nil,
			wantItemFileType: map[string]string{},
			wantItemPath:     map[string]string{},
		},
		{
			name:               "error - repo.GetByName",
			multipartReaderArg: validMultipartReaderWithIncusTarXZ(t),
			repoGetByName: []queue.Item[*image.IncusImage]{
				// lookup
				{
					Err: boom.Error,
				},
			},

			assertErr:        boom.ErrorIs,
			wantPutFilenames: nil,
			wantItemFileType: map[string]string{},
			wantItemPath:     map[string]string{},
		},
		{
			name:               "error - new incus image - repo.Create",
			multipartReaderArg: validMultipartReaderWithIncusTarXZ(t),
			repoGetByName: []queue.Item[*image.IncusImage]{
				// lookup
				{
					Err: domain.ErrNotFound,
				},
			},
			repoCreateErr: boom.Error,

			assertErr:        boom.ErrorIs,
			wantPutFilenames: nil,
			wantItemFileType: map[string]string{},
			wantItemPath:     map[string]string{},
		},
		{
			name:               "error - version already exists",
			multipartReaderArg: validMultipartReaderWithIncusTarXZ(t),
			repoGetByName: []queue.Item[*image.IncusImage]{
				// lookup
				{
					Value: &image.IncusImage{
						Source:          api.IncusImageSourceLocal,
						Name:            "almalinux:10:amd64:cloud",
						OperatingSystem: "almalinux",
						Release:         "10",
						Architecture:    "amd64",
						Variant:         "cloud",
						Versions: map[string]api.IncusImageVersion{
							"20260515": {}, // version already exists
						},
					},
				},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrOperationNotPermitted)
				require.ErrorContains(tt, err, `Version "20260515" already exists for incus image "almalinux:10:amd64:cloud"`)
			},
			wantPutFilenames: nil,
			wantItemFileType: map[string]string{},
			wantItemPath:     map[string]string{},
		},
		{
			name:               "error - filesRepo.Put",
			multipartReaderArg: validMultipartReaderWithIncusTarXZ(t),
			repoGetByName: []queue.Item[*image.IncusImage]{
				// lookup
				{
					Value: &image.IncusImage{
						Source:          api.IncusImageSourceLocal,
						Name:            "almalinux:10:amd64:cloud",
						OperatingSystem: "almalinux",
						Release:         "10",
						Architecture:    "amd64",
						Variant:         "cloud",
					},
				},
			},
			filesRepoPut: []queue.Item[fileRepoPutValue]{
				{
					Err: boom.Error,
				},
			},

			assertErr:        boom.ErrorIs,
			wantPutFilenames: []string{"incus.tar.xz"},
			wantItemFileType: map[string]string{},
			wantItemPath:     map[string]string{},
		},
		{
			name:               "error - filesRepo.Put - cancel",
			multipartReaderArg: validMultipartReaderWithIncusTarXZ(t),
			repoGetByName: []queue.Item[*image.IncusImage]{
				// lookup
				{
					Value: &image.IncusImage{
						Source:          api.IncusImageSourceLocal,
						Name:            "almalinux:10:amd64:cloud",
						OperatingSystem: "almalinux",
						Release:         "10",
						Architecture:    "amd64",
						Variant:         "cloud",
					},
				},
			},
			filesRepoPut: []queue.Item[fileRepoPutValue]{
				{
					Err: errors.New("some error"),
					Value: fileRepoPutValue{
						cancelErr: boom.Error,
					},
				},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				boom.ErrorIs(tt, err)
				require.ErrorContains(tt, err, "some error")
			},
			wantPutFilenames: []string{"incus.tar.xz"},
			wantItemFileType: map[string]string{},
			wantItemPath:     map[string]string{},
		},
		{
			name:               "error - filesRepo.Put - commit",
			multipartReaderArg: validMultipartReaderWithIncusTarXZ(t),
			repoGetByName: []queue.Item[*image.IncusImage]{
				// lookup
				{
					Value: &image.IncusImage{
						Source:          api.IncusImageSourceLocal,
						Name:            "almalinux:10:amd64:cloud",
						OperatingSystem: "almalinux",
						Release:         "10",
						Architecture:    "amd64",
						Variant:         "cloud",
					},
				},
			},
			filesRepoPut: []queue.Item[fileRepoPutValue]{
				{
					Value: fileRepoPutValue{
						commitErr: boom.Error,
					},
				},
			},

			assertErr:        boom.ErrorIs,
			wantPutFilenames: []string{"incus.tar.xz"},
			wantItemFileType: map[string]string{},
			wantItemPath:     map[string]string{},
		},
		{
			name:               "error - invalid 2nd part in multipart message",
			multipartReaderArg: multipartReaderWithInvalid2ndPart(t),
			repoGetByName: []queue.Item[*image.IncusImage]{
				// lookup
				{
					Value: &image.IncusImage{
						Source:          api.IncusImageSourceLocal,
						Name:            "almalinux:10:amd64:cloud",
						OperatingSystem: "almalinux",
						Release:         "10",
						Architecture:    "amd64",
						Variant:         "cloud",
					},
				},
			},
			filesRepoPut: []queue.Item[fileRepoPutValue]{
				{},
			},

			assertErr:        require.Error,
			wantPutFilenames: []string{"incus.tar.xz"},
			wantItemFileType: map[string]string{},
			wantItemPath:     map[string]string{},
		},
		{
			name:               "error - 2nd filesRepo.Put",
			multipartReaderArg: validMultipartReaderWithIncusTarXZ(t),
			repoGetByName: []queue.Item[*image.IncusImage]{
				// lookup
				{
					Value: &image.IncusImage{
						Source:          api.IncusImageSourceLocal,
						Name:            "almalinux:10:amd64:cloud",
						OperatingSystem: "almalinux",
						Release:         "10",
						Architecture:    "amd64",
						Variant:         "cloud",
					},
				},
			},
			filesRepoPut: []queue.Item[fileRepoPutValue]{
				{},
				{
					Err: boom.Error,
				},
			},

			assertErr:        boom.ErrorIs,
			wantPutFilenames: []string{"incus.tar.xz", "root.tar.xz"},
			wantItemFileType: map[string]string{},
			wantItemPath:     map[string]string{},
		},
		{
			name:               "error - 2nd filesRepo.Put - cancel",
			multipartReaderArg: validMultipartReaderWithIncusTarXZ(t),
			repoGetByName: []queue.Item[*image.IncusImage]{
				// lookup
				{
					Value: &image.IncusImage{
						Source:          api.IncusImageSourceLocal,
						Name:            "almalinux:10:amd64:cloud",
						OperatingSystem: "almalinux",
						Release:         "10",
						Architecture:    "amd64",
						Variant:         "cloud",
					},
				},
			},
			filesRepoPut: []queue.Item[fileRepoPutValue]{
				{},
				{
					Err: errors.New("some error"),
					Value: fileRepoPutValue{
						cancelErr: boom.Error,
					},
				},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				boom.ErrorIs(tt, err)
				require.ErrorContains(tt, err, "some error")
			},
			wantPutFilenames: []string{"incus.tar.xz", "root.tar.xz"},
			wantItemFileType: map[string]string{},
			wantItemPath:     map[string]string{},
		},
		{
			name:               "error - 2nd filesRepo.Put - commit",
			multipartReaderArg: validMultipartReaderWithIncusTarXZ(t),
			repoGetByName: []queue.Item[*image.IncusImage]{
				// lookup
				{
					Value: &image.IncusImage{
						Source:          api.IncusImageSourceLocal,
						Name:            "almalinux:10:amd64:cloud",
						OperatingSystem: "almalinux",
						Release:         "10",
						Architecture:    "amd64",
						Variant:         "cloud",
					},
				},
			},
			filesRepoPut: []queue.Item[fileRepoPutValue]{
				{},
				{
					Value: fileRepoPutValue{
						commitErr: boom.Error,
					},
				},
			},

			assertErr:        boom.ErrorIs,
			wantPutFilenames: []string{"incus.tar.xz", "root.tar.xz"},
			wantItemFileType: map[string]string{},
			wantItemPath:     map[string]string{},
		},
		{
			name:               "error - filesRepo.Get metadata for checksum",
			multipartReaderArg: validMultipartReaderWithIncusTarXZ(t),
			repoGetByName: []queue.Item[*image.IncusImage]{
				// lookup
				{
					Value: &image.IncusImage{
						Source:          api.IncusImageSourceLocal,
						Name:            "almalinux:10:amd64:cloud",
						OperatingSystem: "almalinux",
						Release:         "10",
						Architecture:    "amd64",
						Variant:         "cloud",
					},
				},
			},
			filesRepoPut: []queue.Item[fileRepoPutValue]{
				{},
				{},
				{},
				{},
			},
			filesRepoGet: []queue.Item[fileRepoGetValue]{
				// incus.tar.xz for root.tar.xz
				{
					Err: boom.Error,
				},
			},

			assertErr:        boom.ErrorIs,
			wantPutFilenames: []string{"incus.tar.xz", "root.tar.xz", "root.squashfs", "disk.qcow2"},
			wantItemFileType: map[string]string{},
			wantItemPath:     map[string]string{},
		},
		{
			name:               "error - read metadata for checksum",
			multipartReaderArg: validMultipartReaderWithIncusTarXZ(t),
			repoGetByName: []queue.Item[*image.IncusImage]{
				// lookup
				{
					Value: &image.IncusImage{
						Source:          api.IncusImageSourceLocal,
						Name:            "almalinux:10:amd64:cloud",
						OperatingSystem: "almalinux",
						Release:         "10",
						Architecture:    "amd64",
						Variant:         "cloud",
					},
				},
			},
			filesRepoPut: []queue.Item[fileRepoPutValue]{
				{},
				{},
				{},
				{},
			},
			filesRepoGet: []queue.Item[fileRepoGetValue]{
				// incus.tar.xz for root.tar.xz
				{
					Value: fileRepoGetValue{
						reader: io.NopCloser(iotest.ErrReader(boom.Error)),
						size:   int64(len(`incus tar xz`)),
					},
				},
			},

			assertErr:        boom.ErrorIs,
			wantPutFilenames: []string{"incus.tar.xz", "root.tar.xz", "root.squashfs", "disk.qcow2"},
			wantItemFileType: map[string]string{},
			wantItemPath:     map[string]string{},
		},
		{
			name:               "error - filesRepo.Get image for checksum",
			multipartReaderArg: validMultipartReaderWithIncusTarXZ(t),
			repoGetByName: []queue.Item[*image.IncusImage]{
				// lookup
				{
					Value: &image.IncusImage{
						Source:          api.IncusImageSourceLocal,
						Name:            "almalinux:10:amd64:cloud",
						OperatingSystem: "almalinux",
						Release:         "10",
						Architecture:    "amd64",
						Variant:         "cloud",
					},
				},
			},
			filesRepoPut: []queue.Item[fileRepoPutValue]{
				{},
				{},
				{},
				{},
			},
			filesRepoGet: []queue.Item[fileRepoGetValue]{
				// incus.tar.xz for root.tar.xz
				{
					Value: fileRepoGetValue{
						reader: io.NopCloser(bytes.NewBufferString(`incus tar xz`)),
						size:   int64(len(`incus tar xz`)),
					},
				},
				{
					Err: boom.Error,
				},
			},

			assertErr:        boom.ErrorIs,
			wantPutFilenames: []string{"incus.tar.xz", "root.tar.xz", "root.squashfs", "disk.qcow2"},
			wantItemFileType: map[string]string{},
			wantItemPath:     map[string]string{},
		},
		{
			name:               "error - read image for checksum",
			multipartReaderArg: validMultipartReaderWithIncusTarXZ(t),
			repoGetByName: []queue.Item[*image.IncusImage]{
				// lookup
				{
					Value: &image.IncusImage{
						Source:          api.IncusImageSourceLocal,
						Name:            "almalinux:10:amd64:cloud",
						OperatingSystem: "almalinux",
						Release:         "10",
						Architecture:    "amd64",
						Variant:         "cloud",
					},
				},
			},
			filesRepoPut: []queue.Item[fileRepoPutValue]{
				{},
				{},
				{},
				{},
			},
			filesRepoGet: []queue.Item[fileRepoGetValue]{
				// incus.tar.xz for root.tar.xz
				{
					Value: fileRepoGetValue{
						reader: io.NopCloser(bytes.NewBufferString(`incus tar xz`)),
						size:   int64(len(`incus tar xz`)),
					},
				},
				// root.tar.xz
				{
					Value: fileRepoGetValue{
						reader: io.NopCloser(iotest.ErrReader(boom.Error)),
					},
				},
			},

			assertErr:        boom.ErrorIs,
			wantPutFilenames: []string{"incus.tar.xz", "root.tar.xz", "root.squashfs", "disk.qcow2"},
			wantItemFileType: map[string]string{},
			wantItemPath:     map[string]string{},
		},
		{
			name:               "error - repo.GetByName in transaction for update",
			multipartReaderArg: validMultipartReaderWithIncusTarXZ(t),
			repoGetByName: []queue.Item[*image.IncusImage]{
				// lookup
				{
					Value: &image.IncusImage{
						Source:          api.IncusImageSourceLocal,
						Name:            "almalinux:10:amd64:cloud",
						OperatingSystem: "almalinux",
						Release:         "10",
						Architecture:    "amd64",
						Variant:         "cloud",
					},
				},
				// in transaction, get before update
				{
					Err: boom.Error,
				},
			},
			filesRepoPut: []queue.Item[fileRepoPutValue]{
				{},
				{},
				{},
				{},
			},
			filesRepoGet: []queue.Item[fileRepoGetValue]{
				// incus.tar.xz for root.tar.xz
				{
					Value: fileRepoGetValue{
						reader: io.NopCloser(bytes.NewBufferString(`incus tar xz`)),
						size:   int64(len(`incus tar xz`)),
					},
				},
				// root.tar.xz
				{
					Value: fileRepoGetValue{
						reader: io.NopCloser(bytes.NewBufferString(`root tar xz`)),
						size:   int64(len(`root tar xz`)),
					},
				},
				// incus.tar.xz for root.squashfs
				{
					Value: fileRepoGetValue{
						reader: io.NopCloser(bytes.NewBufferString(`incus tar xz`)),
						size:   int64(len(`incus tar xz`)),
					},
				},
				// root.squashfs
				{
					Value: fileRepoGetValue{
						reader: io.NopCloser(bytes.NewBufferString(`squashfs`)),
						size:   int64(len(`squashfs`)),
					},
				},
				// incus.tar.xz for disk.qcow2
				{
					Value: fileRepoGetValue{
						reader: io.NopCloser(bytes.NewBufferString(`incus tar xz`)),
						size:   int64(len(`incus tar xz`)),
					},
				},
				// disk.qcow2
				{
					Value: fileRepoGetValue{
						reader: io.NopCloser(bytes.NewBufferString(`disk qcow2`)),
						size:   int64(len(`disk qcow2`)),
					},
				},
			},

			assertErr:        boom.ErrorIs,
			wantPutFilenames: []string{"incus.tar.xz", "root.tar.xz", "root.squashfs", "disk.qcow2"},
			wantItemFileType: map[string]string{},
			wantItemPath:     map[string]string{},
		},
		{
			name:               "error - repo.Update",
			multipartReaderArg: validMultipartReaderWithIncusTarXZ(t),
			repoGetByName: []queue.Item[*image.IncusImage]{
				// lookup
				{
					Value: &image.IncusImage{
						Source:          api.IncusImageSourceLocal,
						Name:            "almalinux:10:amd64:cloud",
						OperatingSystem: "almalinux",
						Release:         "10",
						Architecture:    "amd64",
						Variant:         "cloud",
					},
				},
				// in transaction, get before update
				{
					Value: &image.IncusImage{
						Source:          api.IncusImageSourceLocal,
						Name:            "almalinux:10:amd64:cloud",
						OperatingSystem: "almalinux",
						Release:         "10",
						Architecture:    "amd64",
						Variant:         "cloud",
					},
				},
			},
			filesRepoPut: []queue.Item[fileRepoPutValue]{
				{},
				{},
				{},
				{},
			},
			filesRepoGet: []queue.Item[fileRepoGetValue]{
				// incus.tar.xz for root.tar.xz
				{
					Value: fileRepoGetValue{
						reader: io.NopCloser(bytes.NewBufferString(`incus tar xz`)),
						size:   int64(len(`incus tar xz`)),
					},
				},
				// root.tar.xz
				{
					Value: fileRepoGetValue{
						reader: io.NopCloser(bytes.NewBufferString(`root tar xz`)),
						size:   int64(len(`root tar xz`)),
					},
				},
				// incus.tar.xz for root.squashfs
				{
					Value: fileRepoGetValue{
						reader: io.NopCloser(bytes.NewBufferString(`incus tar xz`)),
						size:   int64(len(`incus tar xz`)),
					},
				},
				// root.squashfs
				{
					Value: fileRepoGetValue{
						reader: io.NopCloser(bytes.NewBufferString(`squashfs`)),
						size:   int64(len(`squashfs`)),
					},
				},
				// incus.tar.xz for disk.qcow2
				{
					Value: fileRepoGetValue{
						reader: io.NopCloser(bytes.NewBufferString(`incus tar xz`)),
						size:   int64(len(`incus tar xz`)),
					},
				},
				// disk.qcow2
				{
					Value: fileRepoGetValue{
						reader: io.NopCloser(bytes.NewBufferString(`disk qcow2`)),
						size:   int64(len(`disk qcow2`)),
					},
				},
			},
			repoUpdateErr: boom.Error,

			assertErr:        boom.ErrorIs,
			wantPutFilenames: []string{"incus.tar.xz", "root.tar.xz", "root.squashfs", "disk.qcow2"},
			wantItemFileType: map[string]string{
				"disk.qcow2":    "disk-kvm.img",
				"incus.tar.xz":  "incus.tar.xz",
				"root.squashfs": "squashfs",
				"root.tar.xz":   "root.tar.xz",
			},
			wantItemPath: map[string]string{
				"disk.qcow2":    "images/local/almalinux/10/amd64/cloud/20260515/disk.qcow2",
				"incus.tar.xz":  "images/local/almalinux/10/amd64/cloud/20260515/incus.tar.xz",
				"root.squashfs": "images/local/almalinux/10/amd64/cloud/20260515/root.squashfs",
				"root.tar.xz":   "images/local/almalinux/10/amd64/cloud/20260515/root.tar.xz",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			var gotPutFilenames []string
			gotItemFileType := map[string]string{}
			gotItemPath := map[string]string{}

			repo := &mock.ImageIncusRepoMock{
				GetByNameFunc: func(ctx context.Context, source string, name string) (*image.IncusImage, error) {
					require.Equal(t, api.IncusImageSourceLocal, source, "uploaded images belong to the local source")
					return queue.Pop(t, &tc.repoGetByName)
				},
				CreateFunc: func(ctx context.Context, newIncusImage image.IncusImage) (int64, error) {
					require.Equal(t, api.IncusImageSourceLocal, newIncusImage.Source, "uploaded images belong to the local source")
					return 0, tc.repoCreateErr
				},
				UpdateFunc: func(ctx context.Context, newIncusImage image.IncusImage) error {
					// TODO: ensure correct values for size and sha256 hashes
					for _, version := range newIncusImage.Versions {
						for filename, item := range version.Items {
							gotItemFileType[filename] = item.FileType
							gotItemPath[filename] = item.Path
						}
					}

					return tc.repoUpdateErr
				},
			}

			filesRepo := &mock.ImageIncusFileRepoMock{
				PutFunc: func(ctx context.Context, img *image.IncusImage, versionIdentifier, filename string, content io.ReadCloser) (image.CommitFunc, image.CancelFunc, int64, error) {
					size, err := io.ReadAll(content)
					require.NoError(t, err)

					gotPutFilenames = append(gotPutFilenames, filename)

					value, err := queue.Pop(t, &tc.filesRepoPut)

					commitFunc := func() error { return value.commitErr }

					cancelFunc := func() error { return value.cancelErr }

					return commitFunc, cancelFunc, int64(len(size)), err
				},
				GetFunc: func(ctx context.Context, img *image.IncusImage, versionIdentifier, filename string) (io.ReadCloser, int64, error) {
					value, err := queue.Pop(t, &tc.filesRepoGet)

					return value.reader, value.size, err
				},
			}

			imageSvc := image.NewIncusImage(repo, filesRepo, nil)

			// Run test
			name, err := imageSvc.AddVersion(t.Context(), tc.multipartReaderArg)

			// FIXME: ensure correct value for name
			_ = name

			// Assert
			tc.assertErr(t, err)
			require.Empty(t, tc.repoGetByName)
			require.Empty(t, tc.filesRepoPut)
			require.Empty(t, tc.filesRepoGet)

			require.Equal(t, tc.wantPutFilenames, gotPutFilenames, "files stored in the files repository")
			require.Equal(t, tc.wantItemFileType, gotItemFileType, "file types of the stored version items")
			require.Equal(t, tc.wantItemPath, gotItemPath, "item paths use the lower case file path of the image")
		})
	}
}

func writeMetadataTarballPart(t *testing.T, writer *multipart.Writer, filename string) {
	t.Helper()

	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition",
		fmt.Sprintf(`form-data; name="file"; filename=%q`, filename))
	header.Set("Content-Type", "application/octet-stream")

	part, err := writer.CreatePart(header)
	require.NoError(t, err)

	metadata := incusapi.ImageMetadata{
		Properties: map[string]string{
			"os":           "almalinux",
			"release":      "10",
			"architecture": "x86_64",
			"variant":      "cloud",
			"serial":       "20260515",
			"description":  "almalinux 10 (cloud) (amd64)",
		},
	}
	metadataBody, err := yaml.Marshal(metadata)
	require.NoError(t, err)

	buf := bytes.NewBuffer(nil)
	xzw, err := xz.NewWriter(t.Context(), buf)
	require.NoError(t, err)
	tw := tar.NewWriter(xzw)
	err = tw.WriteHeader(&tar.Header{
		Name: "metadata.yaml",
		Size: int64(len(metadataBody)),
		Mode: 0o600,
	})
	require.NoError(t, err)
	_, err = tw.Write(metadataBody)
	require.NoError(t, err)
	err = tw.Close()
	require.NoError(t, err)
	err = xzw.Close()
	require.NoError(t, err)

	_, err = part.Write(buf.Bytes())
	require.NoError(t, err)
}

func validMultipartReaderWithIncusTarXZ(t *testing.T) *multipart.Reader {
	t.Helper()

	var body bytes.Buffer

	writer := multipart.NewWriter(&body)

	writeMetadataTarballPart(t, writer, "almalinux-10")

	// root.tar.xz
	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition",
		`form-data; name="file"; filename="almalinux-10.tar.xz"`)
	header.Set("Content-Type", "application/octet-stream")

	part, err := writer.CreatePart(header)
	require.NoError(t, err)

	_, err = part.Write(imageFileContent(tarXZMagic))
	require.NoError(t, err)

	// root.squashfs
	header = textproto.MIMEHeader{}
	header.Set("Content-Disposition",
		`form-data; name="file"; filename="rootfs.squashfs"`)
	header.Set("Content-Type", "application/octet-stream")

	part, err = writer.CreatePart(header)
	require.NoError(t, err)

	_, err = part.Write(imageFileContent(squashfsMagic))
	require.NoError(t, err)

	// disk.qcow2
	header = textproto.MIMEHeader{}
	header.Set("Content-Disposition",
		`form-data; name="file"; filename="almalinux-10.root"`)
	header.Set("Content-Type", "application/octet-stream")

	part, err = writer.CreatePart(header)
	require.NoError(t, err)

	_, err = part.Write(imageFileContent(qcow2Magic))
	require.NoError(t, err)

	err = writer.Close()
	require.NoError(t, err)

	return multipart.NewReader(&body, writer.Boundary())
}

var (
	squashfsMagic = []byte{'h', 's', 'q', 's'}
	qcow2Magic    = []byte{'Q', 'F', 'I', 0xfb}
	tarXZMagic    = []byte{0xfd, '7', 'z', 'X', 'Z', 0x00}
)

func imageFileContent(magic []byte) []byte {
	return append(append([]byte{}, magic...), bytes.Repeat([]byte("payload "), 64)...)
}

func validMultipartReaderWithRequestJSON(t *testing.T, requestJSON string) *multipart.Reader {
	t.Helper()

	var body bytes.Buffer

	writer := multipart.NewWriter(&body)

	// request_json
	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition",
		`form-data; name="request_json"`)
	header.Set("Content-Type", "application/json")

	part, err := writer.CreatePart(header)
	require.NoError(t, err)

	_, err = part.Write([]byte(requestJSON))
	require.NoError(t, err)

	// disk.qcow2
	header = textproto.MIMEHeader{}
	header.Set("Content-Disposition",
		`form-data; name="file"; filename="almalinux.root"`)
	header.Set("Content-Type", "application/octet-stream")

	part, err = writer.CreatePart(header)
	require.NoError(t, err)

	_, err = part.Write(imageFileContent(qcow2Magic))
	require.NoError(t, err)

	err = writer.Close()
	require.NoError(t, err)

	return multipart.NewReader(&body, writer.Boundary())
}

func multipartReaderWithUnsupportedFile(t *testing.T) *multipart.Reader {
	t.Helper()

	var body bytes.Buffer

	writer := multipart.NewWriter(&body)

	writeMetadataTarballPart(t, writer, "incus.tar.xz")

	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition",
		`form-data; name="file"; filename="rootfs.img"`)
	header.Set("Content-Type", "application/octet-stream")

	part, err := writer.CreatePart(header)
	require.NoError(t, err)

	_, err = io.WriteString(part, "this is not a supported image file")
	require.NoError(t, err)

	err = writer.Close()
	require.NoError(t, err)

	return multipart.NewReader(&body, writer.Boundary())
}

func multipartReaderWithoutMetadataFile(t *testing.T) *multipart.Reader {
	t.Helper()

	var body bytes.Buffer

	writer := multipart.NewWriter(&body)

	// root.tar.xz
	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition",
		`form-data; name="file"; filename="root.tar.xz"`)
	header.Set("Content-Type", "application/octet-stream")

	part, err := writer.CreatePart(header)
	require.NoError(t, err)

	_, err = io.WriteString(part, "root tar xz")
	require.NoError(t, err)

	err = writer.Close()
	require.NoError(t, err)

	return multipart.NewReader(&body, writer.Boundary())
}

func multipartReaderWithInvalid2ndPart(t *testing.T) *multipart.Reader {
	t.Helper()

	var body bytes.Buffer

	writer := multipart.NewWriter(&body)

	writeMetadataTarballPart(t, writer, "incus.tar.xz")

	// append invalid multipart content
	_, err := body.WriteString(strings.Join([]string{
		"",
		"--" + writer.Boundary(),
		"Invalid Header Without Colon", // malformed header
		"",
		"invalid part",
	}, "\r\n"))
	require.NoError(t, err)

	err = writer.Close()
	require.NoError(t, err)

	return multipart.NewReader(&body, writer.Boundary())
}

func TestImageIncusService_GetAll(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		repoGetAll    image.IncusImages
		repoGetAllErr error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:       "success",
			repoGetAll: image.IncusImages{},

			assertErr: require.NoError,
		},
		{
			name:          "error - repo",
			repoGetAllErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			repo := &mock.ImageIncusRepoMock{
				GetAllFunc: func(ctx context.Context) (image.IncusImages, error) {
					return tc.repoGetAll, tc.repoGetAllErr
				},
			}

			imageSvc := image.NewIncusImage(repo, nil, nil)

			// Run test
			images, err := imageSvc.GetAll(t.Context())

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.repoGetAll, images)
		})
	}
}

func TestImageIncusService_GetAllBySource(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                    string
		repoGetAllWithFilter    image.IncusImages
		repoGetAllWithFilterErr error

		assertErr  require.ErrorAssertionFunc
		wantImages image.IncusImages
	}{
		{
			name: "success",
			repoGetAllWithFilter: image.IncusImages{
				{Source: "images", Name: "almalinux:10:amd64:cloud"},
			},

			assertErr: require.NoError,
			wantImages: image.IncusImages{
				{Source: "images", Name: "almalinux:10:amd64:cloud"},
			},
		},
		{
			name:                    "error - repo",
			repoGetAllWithFilterErr: boom.Error,

			assertErr:  boom.ErrorIs,
			wantImages: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			var gotFilter image.IncusImageFilter
			repo := &mock.ImageIncusRepoMock{
				GetAllWithFilterFunc: func(ctx context.Context, filter image.IncusImageFilter) (image.IncusImages, error) {
					gotFilter = filter
					return tc.repoGetAllWithFilter, tc.repoGetAllWithFilterErr
				},
			}

			imageSvc := image.NewIncusImage(repo, nil, nil)

			// Run test
			images, err := imageSvc.GetAllBySource(t.Context(), "images")

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.wantImages, images, "images returned by the repo")
			require.Equal(t, image.IncusImageFilter{Source: new("images")}, gotFilter, "the repo is filtered by the source")
		})
	}
}

func TestImageIncusService_GetByName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		sourceArg        string
		nameArg          string
		repoGetByName    *image.IncusImage
		repoGetByNameErr error

		assertErr     require.ErrorAssertionFunc
		wantRepoCalls []string
	}{
		{
			name:          "success",
			sourceArg:     "images",
			nameArg:       "almalinux:10:amd64:cloud",
			repoGetByName: &image.IncusImage{},

			assertErr:     require.NoError,
			wantRepoCalls: []string{"images/almalinux:10:amd64:cloud"},
		},
		{
			name:      "error - empty name",
			sourceArg: api.IncusImageSourceLocal,
			nameArg:   "", // empty

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
				require.ErrorContains(tt, err, `Invalid incus image name, expect name in the format "os:release:architecture:variant"`)
			},
			wantRepoCalls: nil,
		},
		{
			name:      "error - invalid source",
			sourceArg: "my-source",
			nameArg:   "almalinux:10:amd64:cloud",

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
				require.ErrorContains(tt, err, "can only contain lower case letters, digits and underscores")
			},
			wantRepoCalls: nil,
		},
		{
			name:             "error - repo",
			sourceArg:        api.IncusImageSourceLocal,
			nameArg:          "almalinux:10:amd64:cloud",
			repoGetByNameErr: boom.Error,

			assertErr:     boom.ErrorIs,
			wantRepoCalls: []string{"local/almalinux:10:amd64:cloud"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			var gotRepoCalls []string
			repo := &mock.ImageIncusRepoMock{
				GetByNameFunc: func(ctx context.Context, source string, name string) (*image.IncusImage, error) {
					gotRepoCalls = append(gotRepoCalls, source+"/"+name)
					return tc.repoGetByName, tc.repoGetByNameErr
				},
			}

			imageSvc := image.NewIncusImage(repo, nil, nil)

			// Run test
			img, err := imageSvc.GetByName(t.Context(), tc.sourceArg, tc.nameArg)

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.repoGetByName, img)
			require.Equal(t, tc.wantRepoCalls, gotRepoCalls, "source and name passed to the repo")
		})
	}
}

func TestImageIncusService_DeleteByName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                string
		argSource           string
		argName             string
		filesRepoDeleteErr  error
		repoGetByName       *image.IncusImage
		repoGetByNameErr    error
		repoDeleteByNameErr error

		assertErr     require.ErrorAssertionFunc
		wantRepoCalls []string
	}{
		{
			name:      "success",
			argSource: api.IncusImageSourceLocal,
			argName:   "almalinux:10:amd64:cloud",
			repoGetByName: &image.IncusImage{
				Source: api.IncusImageSourceLocal,
				Name:   "almalinux:10:amd64:cloud",
			},

			assertErr:     require.NoError,
			wantRepoCalls: []string{"GetByName local/almalinux:10:amd64:cloud", "DeleteByName local/almalinux:10:amd64:cloud", "files.Delete local/almalinux:10:amd64:cloud"},
		},
		{
			name:      "error - invalid name",
			argSource: api.IncusImageSourceLocal,
			argName:   "", // empty name

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
				require.ErrorContains(tt, err, `Invalid incus image name, expect name in the format "os:release:architecture:variant"`)
			},
			wantRepoCalls: nil,
		},
		{
			name:      "error - source is not local",
			argSource: "images",
			argName:   "almalinux:10:amd64:cloud",

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrOperationNotPermitted, a...)
				require.ErrorContains(tt, err, `Images of source "images" can not be changed`, a...)
			},
			wantRepoCalls: nil,
		},
		{
			name:             "error - repo.GetByName",
			argSource:        api.IncusImageSourceLocal,
			argName:          "almalinux:10:amd64:cloud",
			repoGetByNameErr: boom.Error,

			assertErr:     boom.ErrorIs,
			wantRepoCalls: []string{"GetByName local/almalinux:10:amd64:cloud"},
		},
		{
			name:      "error - repo.DeleteByName",
			argSource: api.IncusImageSourceLocal,
			argName:   "almalinux:10:amd64:cloud",
			repoGetByName: &image.IncusImage{
				Source: api.IncusImageSourceLocal,
				Name:   "almalinux:10:amd64:cloud",
			},
			repoDeleteByNameErr: boom.Error,

			assertErr:     boom.ErrorIs,
			wantRepoCalls: []string{"GetByName local/almalinux:10:amd64:cloud", "DeleteByName local/almalinux:10:amd64:cloud"},
		},
		{
			name:      "error - filesRepo.Delete",
			argSource: api.IncusImageSourceLocal,
			argName:   "almalinux:10:amd64:cloud",
			repoGetByName: &image.IncusImage{
				Source: api.IncusImageSourceLocal,
				Name:   "almalinux:10:amd64:cloud",
			},
			filesRepoDeleteErr: boom.Error,

			assertErr:     boom.ErrorIs,
			wantRepoCalls: []string{"GetByName local/almalinux:10:amd64:cloud", "DeleteByName local/almalinux:10:amd64:cloud", "files.Delete local/almalinux:10:amd64:cloud"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			var gotRepoCalls []string
			repo := &mock.ImageIncusRepoMock{
				GetByNameFunc: func(ctx context.Context, source string, name string) (*image.IncusImage, error) {
					gotRepoCalls = append(gotRepoCalls, "GetByName "+source+"/"+name)
					return tc.repoGetByName, tc.repoGetByNameErr
				},
				DeleteByNameFunc: func(ctx context.Context, source string, name string) error {
					gotRepoCalls = append(gotRepoCalls, "DeleteByName "+source+"/"+name)
					return tc.repoDeleteByNameErr
				},
			}

			filesRepo := &mock.ImageIncusFileRepoMock{
				DeleteFunc: func(ctx context.Context, img *image.IncusImage) error {
					gotRepoCalls = append(gotRepoCalls, "files.Delete "+img.Source+"/"+img.Name)
					return tc.filesRepoDeleteErr
				},
			}

			imageSvc := image.NewIncusImage(repo, filesRepo, nil)

			// Run test
			err := imageSvc.DeleteByName(t.Context(), tc.argSource, tc.argName)

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.wantRepoCalls, gotRepoCalls, "source and name passed to the repos")
		})
	}
}

func TestImageIncusService_DeleteBySource(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                    string
		argName                 string
		repoGetAllWithFilter    image.IncusImages
		repoGetAllWithFilterErr error
		filesRepoDeleteErr      error
		repoDeleteByNameErr     error

		assertErr     require.ErrorAssertionFunc
		wantRepoCalls []string
	}{
		{
			name:    "success",
			argName: "images",
			repoGetAllWithFilter: image.IncusImages{
				image.IncusImage{
					Source: "images",
					Name:   "almalinux:10:amd64:cloud",
				},
			},

			assertErr:     require.NoError,
			wantRepoCalls: []string{"GetAllWithFilter images", "files.Delete images/almalinux:10:amd64:cloud", "DeleteByName images/almalinux:10:amd64:cloud"},
		},
		{
			name:    "error - invalid name",
			argName: "", // empty name

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
				require.ErrorContains(tt, err, "Invalid source, name can not be empty", a...)
			},
			wantRepoCalls: nil,
		},
		{
			name:    "error - local source",
			argName: api.IncusImageSourceLocal,

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrOperationNotPermitted, a...)
			},
			wantRepoCalls: nil,
		},
		{
			name:                    "error - repo.GetAllWithFilter",
			argName:                 "images",
			repoGetAllWithFilterErr: boom.Error,

			assertErr:     boom.ErrorIs,
			wantRepoCalls: []string{"GetAllWithFilter images"},
		},
		{
			name:    "error - repo.DeleteByName",
			argName: "images",
			repoGetAllWithFilter: image.IncusImages{
				image.IncusImage{
					Source: "images",
					Name:   "almalinux:10:amd64:cloud",
				},
			},
			repoDeleteByNameErr: boom.Error,

			assertErr:     boom.ErrorIs,
			wantRepoCalls: []string{"GetAllWithFilter images", "files.Delete images/almalinux:10:amd64:cloud", "DeleteByName images/almalinux:10:amd64:cloud"},
		},
		{
			name:    "error - filesRepo.Delete",
			argName: "images",
			repoGetAllWithFilter: image.IncusImages{
				image.IncusImage{
					Source: "images",
					Name:   "almalinux:10:amd64:cloud",
				},
			},
			filesRepoDeleteErr: boom.Error,

			assertErr:     boom.ErrorIs,
			wantRepoCalls: []string{"GetAllWithFilter images", "files.Delete images/almalinux:10:amd64:cloud"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			var gotRepoCalls []string
			repo := &mock.ImageIncusRepoMock{
				GetAllWithFilterFunc: func(ctx context.Context, filter image.IncusImageFilter) (image.IncusImages, error) {
					require.Equal(t, image.IncusImageFilter{Source: new(tc.argName)}, filter, "only the images of the source are deleted")
					gotRepoCalls = append(gotRepoCalls, "GetAllWithFilter "+*filter.Source)
					return tc.repoGetAllWithFilter, tc.repoGetAllWithFilterErr
				},
				DeleteByNameFunc: func(ctx context.Context, source string, name string) error {
					gotRepoCalls = append(gotRepoCalls, "DeleteByName "+source+"/"+name)
					return tc.repoDeleteByNameErr
				},
			}

			filesRepo := &mock.ImageIncusFileRepoMock{
				DeleteFunc: func(ctx context.Context, img *image.IncusImage) error {
					gotRepoCalls = append(gotRepoCalls, "files.Delete "+img.Source+"/"+img.Name)
					return tc.filesRepoDeleteErr
				},
			}

			imageSvc := image.NewIncusImage(repo, filesRepo, nil)

			// Run test
			err := imageSvc.DeleteBySource(t.Context(), tc.argName)

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.wantRepoCalls, gotRepoCalls, "source and name passed to the repos")
		})
	}
}

func TestImageIncusService_DeleteVersionByName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                      string
		argSource                 string
		argName                   string
		argVersion                string
		filesRepoDeleteVersionErr error
		repoGetByName             *image.IncusImage
		repoGetByNameErr          error
		repoUpdateErr             error

		assertErr     require.ErrorAssertionFunc
		wantRepoCalls []string
	}{
		{
			name:       "success",
			argSource:  api.IncusImageSourceLocal,
			argName:    "almalinux:10:amd64:cloud",
			argVersion: "20260514",
			repoGetByName: &image.IncusImage{
				Name: "almalinux:10:amd64:cloud",
				Versions: map[string]api.IncusImageVersion{
					"20260514": {},
					"20260515": {},
				},
			},

			assertErr:     require.NoError,
			wantRepoCalls: []string{"local/almalinux:10:amd64:cloud"},
		},
		{
			name:       "error - source is not local",
			argSource:  "images",
			argName:    "almalinux:10:amd64:cloud",
			argVersion: "20260514",

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrOperationNotPermitted, a...)
				require.ErrorContains(tt, err, `Images of source "images" can not be changed`, a...)
			},
			wantRepoCalls: nil,
		},
		{
			name:      "error - invalid name",
			argSource: api.IncusImageSourceLocal,
			argName:   "", // empty name

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
				require.ErrorContains(tt, err, `Invalid incus image name, expect name in the format "os:release:architecture:variant"`)
			},
			wantRepoCalls: nil,
		},
		{
			name:       "error - invalid version",
			argSource:  api.IncusImageSourceLocal,
			argName:    "almalinux:10:amd64:cloud",
			argVersion: "", // empty version

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
				require.ErrorContains(tt, err, "Incus image version cannot be empty")
			},
			wantRepoCalls: nil,
		},
		{
			name:             "error - repo.GetByName",
			argSource:        api.IncusImageSourceLocal,
			argName:          "almalinux:10:amd64:cloud",
			argVersion:       "20260514",
			repoGetByNameErr: boom.Error,

			assertErr:     boom.ErrorIs,
			wantRepoCalls: []string{"local/almalinux:10:amd64:cloud"},
		},
		{
			name:       "error - version not found",
			argSource:  api.IncusImageSourceLocal,
			argName:    "almalinux:10:amd64:cloud",
			argVersion: "20260514",
			repoGetByName: &image.IncusImage{
				Name: "almalinux:10:amd64:cloud",
				Versions: map[string]api.IncusImageVersion{
					"20260515": {},
				},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrNotFound)
				require.ErrorContains(tt, err, `has no version "20260514"`)
			},
			wantRepoCalls: []string{"local/almalinux:10:amd64:cloud"},
		},
		{
			name:       "error - repo.Update",
			argSource:  api.IncusImageSourceLocal,
			argName:    "almalinux:10:amd64:cloud",
			argVersion: "20260514",
			repoGetByName: &image.IncusImage{
				Name: "almalinux:10:amd64:cloud",
				Versions: map[string]api.IncusImageVersion{
					"20260514": {},
					"20260515": {},
				},
			},
			repoUpdateErr: boom.Error,

			assertErr:     boom.ErrorIs,
			wantRepoCalls: []string{"local/almalinux:10:amd64:cloud"},
		},
		{
			name:       "error - filesRepo.CleanupAll",
			argSource:  api.IncusImageSourceLocal,
			argName:    "almalinux:10:amd64:cloud",
			argVersion: "20260514",
			repoGetByName: &image.IncusImage{
				Name: "almalinux:10:amd64:cloud",
				Versions: map[string]api.IncusImageVersion{
					"20260514": {},
					"20260515": {},
				},
			},
			filesRepoDeleteVersionErr: boom.Error,

			assertErr:     boom.ErrorIs,
			wantRepoCalls: []string{"local/almalinux:10:amd64:cloud"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			var gotRepoCalls []string
			repo := &mock.ImageIncusRepoMock{
				GetByNameFunc: func(ctx context.Context, source string, name string) (*image.IncusImage, error) {
					gotRepoCalls = append(gotRepoCalls, source+"/"+name)
					return tc.repoGetByName, tc.repoGetByNameErr
				},
				UpdateFunc: func(ctx context.Context, newIncusImage image.IncusImage) error {
					return tc.repoUpdateErr
				},
			}

			filesRepo := &mock.ImageIncusFileRepoMock{
				DeleteVersionFunc: func(ctx context.Context, img *image.IncusImage, versionIdentifier string) error {
					return tc.filesRepoDeleteVersionErr
				},
			}

			imageSvc := image.NewIncusImage(repo, filesRepo, nil)

			// Run test
			err := imageSvc.DeleteVersionByName(t.Context(), tc.argSource, tc.argName, tc.argVersion)

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.wantRepoCalls, gotRepoCalls, "source and name passed to the repo")
		})
	}
}

func TestIncusImageService_GetVersionFileByName(t *testing.T) {
	t.Parallel()

	assertZeroValues := func(t *testing.T, rc io.ReadCloser, size int64) {
		t.Helper()

		require.Nil(t, rc)
		require.Zero(t, size)
	}

	tests := []struct {
		name             string
		argSource        string
		argName          string
		argVersion       string
		argFilename      string
		repoGetByName    *image.IncusImage
		repoGetByNameErr error
		filesRepoGetRC   io.ReadCloser
		fileRepoGetSize  int64
		filesRepoGetErr  error

		assertErr     require.ErrorAssertionFunc
		assert        func(t *testing.T, rc io.ReadCloser, size int64)
		wantRepoCalls []string
	}{
		{
			name:        "success",
			argSource:   "images",
			argName:     "almalinux:10:amd64:cloud",
			argVersion:  "20260520",
			argFilename: "somefile.txt",
			repoGetByName: &image.IncusImage{
				Name: "almalinux:10:amd64:cloud",
				Versions: map[string]api.IncusImageVersion{
					"20260520": {
						Items: map[string]api.IncusImageVersionItem{
							"somefile.txt": {},
						},
					},
				},
			},
			filesRepoGetRC:  io.NopCloser(bytes.NewBufferString(`foobar`)),
			fileRepoGetSize: 6,

			assertErr: require.NoError,
			assert: func(t *testing.T, rc io.ReadCloser, size int64) {
				t.Helper()

				body, err := io.ReadAll(rc)
				require.NoError(t, err)
				require.Equal(t, []byte(`foobar`), body)
				require.Equal(t, int64(6), size)
			},
			wantRepoCalls: []string{"images/almalinux:10:amd64:cloud"},
		},
		{
			name:      "error - invalid name",
			argSource: api.IncusImageSourceLocal,
			argName:   "", // empty name

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
				require.ErrorContains(tt, err, `Invalid incus image name, expect name in the format "os:release:architecture:variant"`)
			},
			assert:        assertZeroValues,
			wantRepoCalls: nil,
		},
		{
			name:       "error - invalid version",
			argSource:  api.IncusImageSourceLocal,
			argName:    "almalinux:10:amd64:cloud",
			argVersion: "", // empty version

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
				require.ErrorContains(tt, err, "Incus image version cannot be empty")
			},
			assert:        assertZeroValues,
			wantRepoCalls: nil,
		},
		{
			name:        "error - invalid filename",
			argSource:   api.IncusImageSourceLocal,
			argName:     "almalinux:10:amd64:cloud",
			argVersion:  "20260520",
			argFilename: "", // empty filename

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
				require.ErrorContains(tt, err, "Filename cannot be empty")
			},
			assert:        assertZeroValues,
			wantRepoCalls: nil,
		},
		{
			name:             "error - repo.GetByName",
			argSource:        api.IncusImageSourceLocal,
			argName:          "almalinux:10:amd64:cloud",
			argVersion:       "20260520",
			argFilename:      "somefile.txt",
			repoGetByNameErr: boom.Error,

			assertErr:     boom.ErrorIs,
			assert:        assertZeroValues,
			wantRepoCalls: []string{"local/almalinux:10:amd64:cloud"},
		},
		{
			name:        "error - version not found",
			argSource:   api.IncusImageSourceLocal,
			argName:     "almalinux:10:amd64:cloud",
			argVersion:  "20260520/../../../../escape",
			argFilename: "somefile.txt",
			repoGetByName: &image.IncusImage{
				Name: "almalinux:10:amd64:cloud",
				Versions: map[string]api.IncusImageVersion{
					"20260520": {
						Items: map[string]api.IncusImageVersionItem{
							"somefile.txt": {},
						},
					},
				},
			},

			assertErr:     errassert.NotFoundErrorContains(`Incus image "almalinux:10:amd64:cloud" of source "local" has no file "somefile.txt" in version "20260520/../../../../escape"`),
			assert:        assertZeroValues,
			wantRepoCalls: []string{"local/almalinux:10:amd64:cloud"},
		},
		{
			name:        "error - file not found",
			argSource:   api.IncusImageSourceLocal,
			argName:     "almalinux:10:amd64:cloud",
			argVersion:  "20260520",
			argFilename: "../../../../escape",
			repoGetByName: &image.IncusImage{
				Name: "almalinux:10:amd64:cloud",
				Versions: map[string]api.IncusImageVersion{
					"20260520": {
						Items: map[string]api.IncusImageVersionItem{
							"somefile.txt": {},
						},
					},
				},
			},

			assertErr:     errassert.NotFoundErrorContains(`Incus image "almalinux:10:amd64:cloud" of source "local" has no file "../../../../escape" in version "20260520"`),
			assert:        assertZeroValues,
			wantRepoCalls: []string{"local/almalinux:10:amd64:cloud"},
		},
		{
			name:        "error - filesRepo.GetFunc",
			argSource:   api.IncusImageSourceLocal,
			argName:     "almalinux:10:amd64:cloud",
			argVersion:  "20260520",
			argFilename: "somefile.txt",
			repoGetByName: &image.IncusImage{
				Name: "almalinux:10:amd64:cloud",
				Versions: map[string]api.IncusImageVersion{
					"20260520": {
						Items: map[string]api.IncusImageVersionItem{
							"somefile.txt": {},
						},
					},
				},
			},
			filesRepoGetErr: boom.Error,

			assertErr:     boom.ErrorIs,
			assert:        assertZeroValues,
			wantRepoCalls: []string{"local/almalinux:10:amd64:cloud"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var gotRepoCalls []string
			repo := &mock.ImageIncusRepoMock{
				GetByNameFunc: func(ctx context.Context, source string, name string) (*image.IncusImage, error) {
					gotRepoCalls = append(gotRepoCalls, source+"/"+name)
					return tc.repoGetByName, tc.repoGetByNameErr
				},
			}

			filesRepo := &mock.ImageIncusFileRepoMock{
				GetFunc: func(ctx context.Context, img *image.IncusImage, versionIdentifier, filename string) (io.ReadCloser, int64, error) {
					return tc.filesRepoGetRC, tc.fileRepoGetSize, tc.filesRepoGetErr
				},
			}

			imageSvc := image.NewIncusImage(repo, filesRepo, nil)

			// Run test
			rc, size, err := imageSvc.GetVersionFileByName(t.Context(), tc.argSource, tc.argName, tc.argVersion, tc.argFilename)

			// Assert
			tc.assertErr(t, err)
			tc.assert(t, rc, size)
			require.Equal(t, tc.wantRepoCalls, gotRepoCalls, "source and name passed to the repo")
		})
	}
}

func TestIncusImageService_Update(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		incusImage    image.IncusImage
		repoUpdateErr error

		assertErr           require.ErrorAssertionFunc
		wantRepoUpdateCalls int
	}{
		{
			name: "success",
			incusImage: image.IncusImage{
				Source:          api.IncusImageSourceLocal,
				Name:            "almalinux:10:amd64:cloud",
				OperatingSystem: "almalinux",
				Release:         "10",
				Architecture:    "amd64",
				Variant:         "cloud",
			},

			assertErr:           require.NoError,
			wantRepoUpdateCalls: 1,
		},
		{
			name: "error - source is not local",
			incusImage: image.IncusImage{
				Source:          "images",
				Name:            "almalinux:10:amd64:cloud",
				OperatingSystem: "almalinux",
				Release:         "10",
				Architecture:    "amd64",
				Variant:         "cloud",
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrOperationNotPermitted, a...)
				require.ErrorContains(tt, err, `Images of source "images" can not be changed`, a...)
			},
			wantRepoUpdateCalls: 0,
		},
		{
			name: "error - validation",
			incusImage: image.IncusImage{
				Name: "", // empty name
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr)
			},
			wantRepoUpdateCalls: 0,
		},
		{
			name: "error - duplicate alias",
			incusImage: image.IncusImage{
				Source:          api.IncusImageSourceLocal,
				Name:            "almalinux:10:amd64:cloud",
				OperatingSystem: "almalinux",
				Release:         "10",
				Architecture:    "amd64",
				Variant:         "cloud",
				Aliases:         []string{"almalinux/10", "almalinux/10"},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
				require.ErrorContains(tt, err, `Invalid aliases, "almalinux/10" is contained multiple times`, a...)
			},
			wantRepoUpdateCalls: 0,
		},
		{
			name: "error - repo.Update",
			incusImage: image.IncusImage{
				Source:          api.IncusImageSourceLocal,
				Name:            "almalinux:10:amd64:cloud",
				OperatingSystem: "almalinux",
				Release:         "10",
				Architecture:    "amd64",
				Variant:         "cloud",
			},
			repoUpdateErr: boom.Error,

			assertErr:           boom.ErrorIs,
			wantRepoUpdateCalls: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			repo := &mock.ImageIncusRepoMock{
				UpdateFunc: func(ctx context.Context, newIncusImage image.IncusImage) error {
					return tc.repoUpdateErr
				},
			}

			imageSvc := image.NewIncusImage(repo, nil, nil)

			// Run test
			err := imageSvc.Update(t.Context(), tc.incusImage)

			// Assert
			tc.assertErr(t, err)
			require.Len(t, repo.UpdateCalls(), tc.wantRepoUpdateCalls, "invalid images are not stored")
		})
	}
}

func TestIncusImageService_ValidateFilterExpression(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                string
		argFilterExpression string

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:                "success",
			argFilterExpression: "true",

			assertErr: require.NoError,
		},
		{
			name:                "error - invalid expression",
			argFilterExpression: "10", // invalid expression, does not return bool result

			assertErr: require.Error,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			imageSvc := image.NewIncusImage(nil, nil, nil)

			// Run test
			err := imageSvc.ValidateFilterExpression(t.Context(), tc.argFilterExpression)

			// Assert
			tc.assertErr(t, err)
		})
	}
}

func TestIncusImageService_RefreshFromSource_invalidSource(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		source image.IncusImageSource

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "error - local source",
			source: image.IncusImageSource{
				Name: api.IncusImageSourceLocal,
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrOperationNotPermitted, a...)
			},
		},
		{
			name: "error - URL replaced by the update to schema v44",
			source: image.IncusImageSource{
				Name:             "source_7",
				URL:              "invalid://source_7",
				FilterExpression: "true",
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
				require.ErrorContains(tt, err, "update the URL of the source", a...)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			imageSvc := image.NewIncusImage(&mock.ImageIncusRepoMock{}, &mock.ImageIncusFileRepoMock{}, &adapterMock.SimplestreamsPortMock{})

			// Run test
			err := imageSvc.RefreshFromSource(t.Context(), tc.source)

			// Assert
			tc.assertErr(t, err, "the source is rejected before any repository or simplestreams call")
		})
	}
}

func TestIncusImageService_RefreshFromSource(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                          string
		ctx                           context.Context
		filterExpression              string
		simplestreamsGetImageList     image.IncusImages
		simplestreamsGetImageListErr  error
		repoGetAllWithFilter          image.IncusImages
		repoGetAllWithFilterErr       error
		repoDeleteByNameErr           error
		filesRepoDeleteVersionFileErr error
		filesRepoUsageInformation     file.UsageInformation
		filesRepoUsageInformationErr  error
		simplestreamsGetFileRC        io.ReadCloser
		simplestreamsGetFileErr       error
		simplestreamsGetFileErrPaths  []string
		filesRepoPutCommitErr         error
		filesRepoPutCancelErr         error
		filesRepoPutErr               error
		repoUpsertErr                 error

		assertErr              require.ErrorAssertionFunc
		wantUpsertedImageNames []string
		wantUpsertedFiles      []string
		wantDeletedImageNames  []string
		wantDeletedFiles       []string
	}{
		{
			name: "success - no updates, no state in the DB",
			ctx:  t.Context(),

			assertErr:              require.NoError,
			wantUpsertedImageNames: nil,
			wantUpsertedFiles:      nil,
			wantDeletedImageNames:  nil,
			wantDeletedFiles:       nil,
		},
		{
			name:             "success - one update, filtered",
			ctx:              t.Context(),
			filterExpression: "false", // filter everything

			simplestreamsGetImageList: image.IncusImages{
				{
					Name:            "alpine:edge:amd64:default",
					OperatingSystem: "Alpine",
					Release:         "edge",
					Architecture:    "amd64",
					Variant:         "default",
					Versions: api.IncusImageVersions{
						"20260101": api.IncusImageVersion{
							Items: map[string]api.IncusImageVersionItem{
								"incus.tar.xz": {},
							},
						},
					},
				},
			},

			assertErr:              require.NoError,
			wantUpsertedImageNames: nil,
			wantUpsertedFiles:      nil,
			wantDeletedImageNames:  nil,
			wantDeletedFiles:       nil,
		},
		{
			name:             "success - one update, already present in DB",
			ctx:              t.Context(),
			filterExpression: "true",

			simplestreamsGetImageList: image.IncusImages{
				{
					Name:            "alpine:edge:amd64:default",
					OperatingSystem: "Alpine",
					Release:         "edge",
					Architecture:    "amd64",
					Variant:         "default",
					Versions: api.IncusImageVersions{
						"20260101": api.IncusImageVersion{
							Items: map[string]api.IncusImageVersionItem{
								"incus.tar.xz": {},
							},
						},
					},
				},
			},
			repoGetAllWithFilter: image.IncusImages{
				{
					Source:          "linuxcontainers",
					Name:            "alpine:edge:amd64:default",
					OperatingSystem: "Alpine",
					Release:         "edge",
					Architecture:    "amd64",
					Variant:         "default",
					Versions: api.IncusImageVersions{
						"20260101": api.IncusImageVersion{
							Items: map[string]api.IncusImageVersionItem{
								"incus.tar.xz": {},
							},
						},
					},
				},
			},
			filesRepoUsageInformation: usageInfoGiB(100, 20),

			assertErr:              require.NoError,
			wantUpsertedImageNames: []string{"alpine:edge:amd64:default"},
			wantUpsertedFiles: []string{
				"linuxcontainers/alpine/edge/amd64/default/20260101/incus.tar.xz",
			},
			wantDeletedImageNames: nil,
			wantDeletedFiles:      nil,
		},
		{
			name:             "success - one update, already present in DB but misses files with valid hash",
			ctx:              t.Context(),
			filterExpression: "true",

			simplestreamsGetImageList: image.IncusImages{
				{
					Name:            "alpine:edge:amd64:default",
					OperatingSystem: "Alpine",
					Release:         "edge",
					Architecture:    "amd64",
					Variant:         "default",
					Versions: api.IncusImageVersions{
						"20260101": api.IncusImageVersion{
							Items: map[string]api.IncusImageVersionItem{
								"incus.tar.xz": {},
								"root.tar.xz": {
									// Generate hash: echo -n "dummy" | sha256sum
									HashSha256: "b5a2c96250612366ea272ffac6d9744aaf4b45aacd96aa7cfcb931ee3b558259",
								},
							},
						},
					},
				},
			},
			repoGetAllWithFilter: image.IncusImages{
				{
					Source:          "linuxcontainers",
					Name:            "alpine:edge:amd64:default",
					OperatingSystem: "Alpine",
					Release:         "edge",
					Architecture:    "amd64",
					Variant:         "default",
					Versions: api.IncusImageVersions{
						"20260101": api.IncusImageVersion{
							Items: map[string]api.IncusImageVersionItem{
								"incus.tar.xz": {},
							},
						},
					},
				},
			},
			filesRepoUsageInformation: usageInfoGiB(50, 10),
			simplestreamsGetFileRC:    io.NopCloser(bytes.NewBufferString(`dummy`)),

			assertErr:              require.NoError,
			wantUpsertedImageNames: []string{"alpine:edge:amd64:default"},
			wantUpsertedFiles: []string{
				"linuxcontainers/alpine/edge/amd64/default/20260101/incus.tar.xz",
				"linuxcontainers/alpine/edge/amd64/default/20260101/root.tar.xz",
			},
			wantDeletedImageNames: nil,
			wantDeletedFiles:      nil,
		},
		{
			name: "success - enhanced example",
			// Image source presents 3 images.
			// One image is filtered based on filter expression and therefore skipped.
			// One is not present and is therefore downloaded with all files included.
			// One is present but misses file and has supernumerous file.
			// In the DB we have two images.
			// One is obsolete and gets deleted.
			// One is updated, because it misses a file and has a supernumerous file (see 3rd image from source).
			ctx:              t.Context(),
			filterExpression: `architecture == "amd64"`,

			simplestreamsGetImageList: image.IncusImages{
				// Filtered because of architecture.
				{
					Name:            "alpine:edge:aarch64:default",
					OperatingSystem: "Alpine",
					Release:         "edge",
					Architecture:    "aarch64",
					Variant:         "default",
					Versions: api.IncusImageVersions{
						"20260101": api.IncusImageVersion{
							Items: map[string]api.IncusImageVersionItem{
								"incus.tar.xz": {},
							},
						},
					},
				},
				// Not present in the DB.
				{
					Name:            "alpine:edge:amd64:default",
					OperatingSystem: "Alpine",
					Release:         "edge",
					Architecture:    "amd64",
					Variant:         "default",
					Versions: api.IncusImageVersions{
						"20260101": api.IncusImageVersion{
							Items: map[string]api.IncusImageVersionItem{
								"incus.tar.xz": {},
							},
						},
					},
				},
				// Present in DB, but misses file and has supernumerous file.
				{
					Name:            "alpine:edge:amd64:cloud",
					OperatingSystem: "Alpine",
					Release:         "edge",
					Architecture:    "amd64",
					Variant:         "cloud",
					Versions: api.IncusImageVersions{
						"20260101": api.IncusImageVersion{
							Items: map[string]api.IncusImageVersionItem{
								"incus.tar.xz": {},
								"root.tar.xz":  {}, // additional file
							},
						},
					},
				},
			},
			repoGetAllWithFilter: image.IncusImages{
				// Present in DB, but misses file and has supernumerous file.
				{
					Source:          "linuxcontainers",
					Name:            "alpine:edge:amd64:cloud",
					OperatingSystem: "Alpine",
					Release:         "edge",
					Architecture:    "amd64",
					Variant:         "cloud",
					Versions: api.IncusImageVersions{
						"20260101": api.IncusImageVersion{
							Items: map[string]api.IncusImageVersionItem{
								"incus.tar.xz":       {},
								"supernumerous.file": {},
							},
						},
					},
				},
				// Obsolete update.
				{
					Source:          "linuxcontainers",
					Name:            "alpine:edge:amd64:obsolete",
					OperatingSystem: "Alpine",
					Release:         "edge",
					Architecture:    "amd64",
					Variant:         "obsolete",
					Versions: api.IncusImageVersions{
						"20260101": api.IncusImageVersion{
							Items: map[string]api.IncusImageVersionItem{
								"incus.tar.xz": {},
							},
						},
					},
				},
			},
			filesRepoUsageInformation: usageInfoGiB(50, 10),

			assertErr:              require.NoError,
			wantUpsertedImageNames: []string{"alpine:edge:amd64:default", "alpine:edge:amd64:cloud"},
			wantUpsertedFiles: []string{
				"linuxcontainers/alpine/edge/amd64/default/20260101/incus.tar.xz",
				"linuxcontainers/alpine/edge/amd64/cloud/20260101/incus.tar.xz",
				"linuxcontainers/alpine/edge/amd64/cloud/20260101/root.tar.xz",
			},
			wantDeletedImageNames: []string{"linuxcontainers/alpine:edge:amd64:obsolete"},
			wantDeletedFiles: []string{
				"linuxcontainers/alpine/edge/amd64/cloud/20260101/supernumerous.file",
				"linuxcontainers/alpine/edge/amd64/obsolete/20260101/incus.tar.xz",
			},
		},

		{
			name:             "success - product name from image source is replaced",
			ctx:              t.Context(),
			filterExpression: "true",

			simplestreamsGetImageList: image.IncusImages{
				{
					Name:            "ubuntu:26.04:default:amd64", // format of incus-simplestreams
					OperatingSystem: "Ubuntu",
					Release:         "26.04",
					Architecture:    "x86_64",
					Variant:         "default",
					Versions: api.IncusImageVersions{
						"20260101": api.IncusImageVersion{
							Items: map[string]api.IncusImageVersionItem{
								"incus.tar.xz": {},
							},
						},
					},
				},
				{
					Name:            "com.ubuntu.cloud:server:24.04:amd64", // format of cloud-images.ubuntu.com
					OperatingSystem: "Ubuntu",
					Release:         "24.04",
					Architecture:    "amd64",
					Variant:         "default",
					Versions: api.IncusImageVersions{
						"20260101": api.IncusImageVersion{
							Items: map[string]api.IncusImageVersionItem{
								"incus.tar.xz": {},
							},
						},
					},
				},
			},
			repoGetAllWithFilter: image.IncusImages{
				{
					Source:          "linuxcontainers",
					Name:            "ubuntu:26.04:default:amd64", // stored by an earlier sync
					OperatingSystem: "Ubuntu",
					Release:         "26.04",
					Architecture:    "amd64",
					Variant:         "default",
					Versions: api.IncusImageVersions{
						"20260101": api.IncusImageVersion{
							Items: map[string]api.IncusImageVersionItem{
								"incus.tar.xz": {},
							},
						},
					},
				},
			},
			filesRepoUsageInformation: usageInfoGiB(50, 10),

			assertErr:              require.NoError,
			wantUpsertedImageNames: []string{"ubuntu:26.04:amd64:default", "ubuntu:24.04:amd64:default"},
			wantUpsertedFiles: []string{
				"linuxcontainers/ubuntu/26.04/amd64/default/20260101/incus.tar.xz",
				"linuxcontainers/ubuntu/24.04/amd64/default/20260101/incus.tar.xz",
			},
			wantDeletedImageNames: []string{"linuxcontainers/ubuntu:26.04:default:amd64"},
			wantDeletedFiles:      nil,
		},

		// Error cases.
		{
			name:             "success - invalid images are skipped and removed from the DB",
			ctx:              t.Context(),
			filterExpression: `true`,

			simplestreamsGetImageList: image.IncusImages{
				{
					Name:            "alpine:edge:amd64:cloud",
					OperatingSystem: "Alpine",
					Release:         "edge",
					Architecture:    "amd64",
					Variant:         "cloud",
					Versions: api.IncusImageVersions{
						"20260101": api.IncusImageVersion{
							Items: map[string]api.IncusImageVersionItem{
								"incus.tar.xz": {},
							},
						},
					},
				},
				{
					Name:            "alpine:edge:s390x:cloud", // architecture not supported
					OperatingSystem: "Alpine",
					Release:         "edge",
					Architecture:    "s390x",
					Variant:         "cloud",
					Versions: api.IncusImageVersions{
						"20260101": api.IncusImageVersion{
							Items: map[string]api.IncusImageVersionItem{
								"incus.tar.xz": {},
							},
						},
					},
				},
				{
					Name:            "alpine:..:amd64:cloud", // invalid path element
					OperatingSystem: "Alpine",
					Release:         "..",
					Architecture:    "amd64",
					Variant:         "cloud",
					Versions: api.IncusImageVersions{
						"20260101": api.IncusImageVersion{
							Items: map[string]api.IncusImageVersionItem{
								"incus.tar.xz": {},
							},
						},
					},
				},
			},
			repoGetAllWithFilter: image.IncusImages{
				{
					Source:          "linuxcontainers",
					Name:            "alpine:edge:s390x:cloud", // stored by an earlier version without validation
					OperatingSystem: "Alpine",
					Release:         "edge",
					Architecture:    "s390x",
					Variant:         "cloud",
					Versions: api.IncusImageVersions{
						"20260101": api.IncusImageVersion{
							Items: map[string]api.IncusImageVersionItem{
								"incus.tar.xz": {},
							},
						},
					},
				},
			},
			filesRepoUsageInformation: usageInfoGiB(50, 10),

			assertErr:              require.NoError,
			wantUpsertedImageNames: []string{"alpine:edge:amd64:cloud"},
			wantUpsertedFiles: []string{
				"linuxcontainers/alpine/edge/amd64/cloud/20260101/incus.tar.xz",
			},
			wantDeletedImageNames: []string{"linuxcontainers/alpine:edge:s390x:cloud"},
			wantDeletedFiles:      []string{"linuxcontainers/alpine/edge/s390x/cloud/20260101/incus.tar.xz"},
		},
		{
			name:             "success - multiple products for the same image are skipped and kept in the DB",
			ctx:              t.Context(),
			filterExpression: `true`,

			simplestreamsGetImageList: image.IncusImages{
				{
					Name:            "alpine:edge:amd64:cloud",
					OperatingSystem: "Alpine",
					Release:         "edge",
					Architecture:    "amd64",
					Variant:         "cloud",
					Versions: api.IncusImageVersions{
						"20260101": api.IncusImageVersion{
							Items: map[string]api.IncusImageVersionItem{
								"incus.tar.xz": {},
							},
						},
					},
				},
				{
					Name:            "alpine:edge:amd64:default",
					OperatingSystem: "Alpine",
					Release:         "edge",
					Architecture:    "amd64",
					Variant:         "default",
					Versions: api.IncusImageVersions{
						"20260101": api.IncusImageVersion{
							Items: map[string]api.IncusImageVersionItem{
								"incus.tar.xz": {},
							},
						},
					},
				},
				{
					Name:            "alpine:edge:default:amd64", // same image as the one before
					OperatingSystem: "alpine",
					Release:         "edge",
					Architecture:    "amd64",
					Variant:         "default",
					Versions: api.IncusImageVersions{
						"20260101": api.IncusImageVersion{
							Items: map[string]api.IncusImageVersionItem{
								"incus.tar.xz": {},
							},
						},
					},
				},
			},
			repoGetAllWithFilter: image.IncusImages{
				{
					Source:          "linuxcontainers",
					Name:            "alpine:edge:amd64:default", // stored by an earlier sync
					OperatingSystem: "Alpine",
					Release:         "edge",
					Architecture:    "amd64",
					Variant:         "default",
					Versions: api.IncusImageVersions{
						"20251201": api.IncusImageVersion{
							Items: map[string]api.IncusImageVersionItem{
								"incus.tar.xz": {},
							},
						},
					},
				},
			},
			filesRepoUsageInformation: usageInfoGiB(50, 10),

			assertErr:              require.NoError,
			wantUpsertedImageNames: []string{"alpine:edge:amd64:cloud"},
			wantUpsertedFiles: []string{
				"linuxcontainers/alpine/edge/amd64/cloud/20260101/incus.tar.xz",
			},
			wantDeletedImageNames: nil,
			wantDeletedFiles:      nil,
		},
		{
			name:             "success - invalid product with the file path of a valid image is skipped, the valid image is stored",
			ctx:              t.Context(),
			filterExpression: `true`,

			simplestreamsGetImageList: image.IncusImages{
				{
					Name:            "alpine:edge:amd64:cloud",
					OperatingSystem: "Alpine",
					Release:         "edge",
					Architecture:    "amd64",
					Variant:         "cloud",
					Versions: api.IncusImageVersions{
						"20260101": api.IncusImageVersion{
							Items: map[string]api.IncusImageVersionItem{
								"incus.tar.xz": {},
							},
						},
					},
				},
				{
					Name:            "alpine:edge:amd64:cloud:duplicate-aliases",
					OperatingSystem: "alpine",
					Release:         "edge",
					Architecture:    "amd64",
					Variant:         "cloud",
					Aliases:         []string{"alpine/edge", "alpine/edge"}, // invalid, duplicate alias
					Versions: api.IncusImageVersions{
						"20260101": api.IncusImageVersion{
							Items: map[string]api.IncusImageVersionItem{
								"incus.tar.xz": {},
							},
						},
					},
				},
			},
			filesRepoUsageInformation: usageInfoGiB(50, 10),

			assertErr:              require.NoError,
			wantUpsertedImageNames: []string{"alpine:edge:amd64:cloud"},
			wantUpsertedFiles: []string{
				"linuxcontainers/alpine/edge/amd64/cloud/20260101/incus.tar.xz",
			},
			wantDeletedImageNames: nil,
			wantDeletedFiles:      nil,
		},
		{
			name:             "error - simplestreams.GetImageList",
			ctx:              t.Context(),
			filterExpression: `true`,

			simplestreamsGetImageListErr: boom.Error,

			assertErr:              boom.ErrorIs,
			wantUpsertedImageNames: nil,
			wantUpsertedFiles:      nil,
			wantDeletedImageNames:  nil,
			wantDeletedFiles:       nil,
		},
		{
			name:             "error - invalid filter",
			ctx:              t.Context(),
			filterExpression: `10`, // invalid filter, does not evaluate to bool value.

			simplestreamsGetImageList: image.IncusImages{},
			repoGetAllWithFilter:      image.IncusImages{},
			repoGetAllWithFilterErr:   boom.Error,

			assertErr:              require.Error,
			wantUpsertedImageNames: nil,
			wantUpsertedFiles:      nil,
			wantDeletedImageNames:  nil,
			wantDeletedFiles:       nil,
		},
		{
			name:             "error - repo.GetAllWithFilter",
			ctx:              t.Context(),
			filterExpression: `true`,

			simplestreamsGetImageList: image.IncusImages{},
			repoGetAllWithFilter:      image.IncusImages{},
			repoGetAllWithFilterErr:   boom.Error,

			assertErr:              boom.ErrorIs,
			wantUpsertedImageNames: nil,
			wantUpsertedFiles:      nil,
			wantDeletedImageNames:  nil,
			wantDeletedFiles:       nil,
		},
		{
			name:             "error - repo.DeleteByName",
			ctx:              t.Context(),
			filterExpression: `true`,

			simplestreamsGetImageList: image.IncusImages{},
			repoGetAllWithFilter: image.IncusImages{
				// Supernumerous image.
				{
					Source:          "linuxcontainers",
					Name:            "alpine:edge:amd64:cloud",
					OperatingSystem: "Alpine",
					Release:         "edge",
					Architecture:    "amd64",
					Variant:         "cloud",
					Versions: api.IncusImageVersions{
						"20260101": api.IncusImageVersion{
							Items: map[string]api.IncusImageVersionItem{
								"incus.tar.xz": {},
							},
						},
					},
				},
			},
			repoDeleteByNameErr: boom.Error,

			assertErr:              boom.ErrorIs,
			wantUpsertedImageNames: nil,
			wantUpsertedFiles:      nil,
			wantDeletedImageNames:  []string{"linuxcontainers/alpine:edge:amd64:cloud"},
			wantDeletedFiles:       nil,
		},
		{
			name:             "error - filesRepo.DeleteVersionFile",
			ctx:              t.Context(),
			filterExpression: `true`,

			simplestreamsGetImageList: image.IncusImages{
				{
					Name:            "alpine:edge:amd64:cloud",
					OperatingSystem: "Alpine",
					Release:         "edge",
					Architecture:    "amd64",
					Variant:         "cloud",
					Versions: api.IncusImageVersions{
						"20260101": api.IncusImageVersion{
							Items: map[string]api.IncusImageVersionItem{
								"incus.tar.xz": {},
							},
						},
					},
				},
			},
			repoGetAllWithFilter: image.IncusImages{
				// Present in DB, but has supernumerous file.
				{
					Source:          "linuxcontainers",
					Name:            "alpine:edge:amd64:cloud",
					OperatingSystem: "Alpine",
					Release:         "edge",
					Architecture:    "amd64",
					Variant:         "cloud",
					Versions: api.IncusImageVersions{
						"20260101": api.IncusImageVersion{
							Items: map[string]api.IncusImageVersionItem{
								"incus.tar.xz": {},
								"root.tar.xz":  {}, // Additional file.
							},
						},
					},
				},
			},
			filesRepoDeleteVersionFileErr: boom.Error,

			assertErr:              boom.ErrorIs,
			wantUpsertedImageNames: nil,
			wantUpsertedFiles:      nil,
			wantDeletedImageNames:  nil,
			wantDeletedFiles:       []string{"linuxcontainers/alpine/edge/amd64/cloud/20260101/root.tar.xz"},
		},
		{
			name:             "error - filesRepo.UsageInformation",
			ctx:              t.Context(),
			filterExpression: `true`,

			simplestreamsGetImageList: image.IncusImages{
				{
					Name:            "alpine:edge:amd64:cloud",
					OperatingSystem: "Alpine",
					Release:         "edge",
					Architecture:    "amd64",
					Variant:         "cloud",
					Versions: api.IncusImageVersions{
						"20260101": api.IncusImageVersion{
							Items: map[string]api.IncusImageVersionItem{
								"incus.tar.xz": {},
								"root.tar.xz":  {}, // Additional file.
							},
						},
					},
				},
			},
			repoGetAllWithFilter: image.IncusImages{
				// Present in DB, but misses file.
				{
					Source:          "linuxcontainers",
					Name:            "alpine:edge:amd64:cloud",
					OperatingSystem: "Alpine",
					Release:         "edge",
					Architecture:    "amd64",
					Variant:         "cloud",
					Versions: api.IncusImageVersions{
						"20260101": api.IncusImageVersion{
							Items: map[string]api.IncusImageVersionItem{
								"incus.tar.xz": {},
							},
						},
					},
				},
			},
			filesRepoUsageInformationErr: boom.Error,

			assertErr:              boom.ErrorIs,
			wantUpsertedImageNames: []string{"alpine:edge:amd64:cloud"},
			wantUpsertedFiles: []string{
				"linuxcontainers/alpine/edge/amd64/cloud/20260101/incus.tar.xz",
			},
			wantDeletedImageNames: nil,
			wantDeletedFiles:      nil,
		},
		{
			name:             "error - invalid total disk space",
			ctx:              t.Context(),
			filterExpression: `true`,

			simplestreamsGetImageList: image.IncusImages{
				{
					Name:            "alpine:edge:amd64:cloud",
					OperatingSystem: "Alpine",
					Release:         "edge",
					Architecture:    "amd64",
					Variant:         "cloud",
					Versions: api.IncusImageVersions{
						"20260101": api.IncusImageVersion{
							Items: map[string]api.IncusImageVersionItem{
								"incus.tar.xz": {},
								"root.tar.xz":  {}, // Additional file.
							},
						},
					},
				},
			},
			repoGetAllWithFilter: image.IncusImages{
				// Present in DB, but misses file.
				{
					Source:          "linuxcontainers",
					Name:            "alpine:edge:amd64:cloud",
					OperatingSystem: "Alpine",
					Release:         "edge",
					Architecture:    "amd64",
					Variant:         "cloud",
					Versions: api.IncusImageVersions{
						"20260101": api.IncusImageVersion{
							Items: map[string]api.IncusImageVersionItem{
								"incus.tar.xz": {},
							},
						},
					},
				},
			},
			filesRepoUsageInformation: usageInfoGiB(0, 0), // total disk space invalid.

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, "Files repository reported an invalid total space")
			},
			wantUpsertedImageNames: []string{"alpine:edge:amd64:cloud"},
			wantUpsertedFiles: []string{
				"linuxcontainers/alpine/edge/amd64/cloud/20260101/incus.tar.xz",
			},
			wantDeletedImageNames: nil,
			wantDeletedFiles:      nil,
		},
		{
			name:             "error - not enough disk space available",
			ctx:              t.Context(),
			filterExpression: `true`,

			simplestreamsGetImageList: image.IncusImages{
				{
					Name:            "alpine:edge:amd64:cloud",
					OperatingSystem: "Alpine",
					Release:         "edge",
					Architecture:    "amd64",
					Variant:         "cloud",
					Versions: api.IncusImageVersions{
						"20260101": api.IncusImageVersion{
							Items: map[string]api.IncusImageVersionItem{
								"incus.tar.xz": {},
								"root.tar.xz":  {}, // Additional file.
							},
						},
					},
				},
			},
			repoGetAllWithFilter: image.IncusImages{
				// Present in DB, but misses file.
				{
					Source:          "linuxcontainers",
					Name:            "alpine:edge:amd64:cloud",
					OperatingSystem: "Alpine",
					Release:         "edge",
					Architecture:    "amd64",
					Variant:         "cloud",
					Versions: api.IncusImageVersions{
						"20260101": api.IncusImageVersion{
							Items: map[string]api.IncusImageVersionItem{
								"incus.tar.xz": {},
							},
						},
					},
				},
			},
			filesRepoUsageInformation: usageInfoGiB(10, 0), // total disk space invalid.

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, "Not enough space available in the files repository")
			},
			wantUpsertedImageNames: []string{"alpine:edge:amd64:cloud"},
			wantUpsertedFiles: []string{
				"linuxcontainers/alpine/edge/amd64/cloud/20260101/incus.tar.xz",
			},
			wantDeletedImageNames: nil,
			wantDeletedFiles:      nil,
		},
		{
			name:             "error - simplestreams.GetFile",
			ctx:              t.Context(),
			filterExpression: `true`,

			simplestreamsGetImageList: image.IncusImages{
				{
					Name:            "alpine:edge:amd64:cloud",
					OperatingSystem: "Alpine",
					Release:         "edge",
					Architecture:    "amd64",
					Variant:         "cloud",
					Versions: api.IncusImageVersions{
						"20260101": api.IncusImageVersion{
							Items: map[string]api.IncusImageVersionItem{
								"incus.tar.xz": {},
								"root.tar.xz":  {},
							},
						},
					},
				},
			},
			repoGetAllWithFilter: image.IncusImages{
				// Present in DB, but misses files.
				{
					Source:          "linuxcontainers",
					Name:            "alpine:edge:amd64:cloud",
					OperatingSystem: "Alpine",
					Release:         "edge",
					Architecture:    "amd64",
					Variant:         "cloud",
					Versions: api.IncusImageVersions{
						"20260101": api.IncusImageVersion{
							Items: map[string]api.IncusImageVersionItem{
								"incus.tar.xz": {},
							},
						},
					},
				},
			},
			filesRepoUsageInformation: usageInfoGiB(50, 10),
			simplestreamsGetFileErr:   boom.Error,

			assertErr:              boom.ErrorIs,
			wantUpsertedImageNames: []string{"alpine:edge:amd64:cloud"},
			wantUpsertedFiles: []string{
				"linuxcontainers/alpine/edge/amd64/cloud/20260101/incus.tar.xz",
			},
			wantDeletedImageNames: nil,
			wantDeletedFiles:      nil,
		},
		{
			name:             "error - fileRepo.Put",
			ctx:              t.Context(),
			filterExpression: `true`,

			simplestreamsGetImageList: image.IncusImages{
				{
					Name:            "alpine:edge:amd64:cloud",
					OperatingSystem: "Alpine",
					Release:         "edge",
					Architecture:    "amd64",
					Variant:         "cloud",
					Versions: api.IncusImageVersions{
						"20260101": api.IncusImageVersion{
							Items: map[string]api.IncusImageVersionItem{
								"incus.tar.xz":  {},
								"root01.tar.xz": {}, // 10 additional files all failing.
								"root02.tar.xz": {}, // 10 additional files all failing.
								"root03.tar.xz": {}, // 10 additional files all failing.
								"root04.tar.xz": {}, // 10 additional files all failing.
								"root05.tar.xz": {}, // 10 additional files all failing.
								"root06.tar.xz": {}, // 10 additional files all failing.
								"root07.tar.xz": {}, // 10 additional files all failing.
								"root08.tar.xz": {}, // 10 additional files all failing.
								"root09.tar.xz": {}, // 10 additional files all failing.
								"root10.tar.xz": {}, // 10 additional files all failing.
							},
						},
					},
				},
			},
			repoGetAllWithFilter: image.IncusImages{
				// Present in DB, but misses files.
				{
					Source:          "linuxcontainers",
					Name:            "alpine:edge:amd64:cloud",
					OperatingSystem: "Alpine",
					Release:         "edge",
					Architecture:    "amd64",
					Variant:         "cloud",
					Versions: api.IncusImageVersions{
						"20260101": api.IncusImageVersion{
							Items: map[string]api.IncusImageVersionItem{
								"incus.tar.xz": {},
							},
						},
					},
				},
			},
			filesRepoUsageInformation: usageInfoGiB(50, 10),
			filesRepoPutErr:           boom.Error,

			assertErr:              boom.ErrorIs,
			wantUpsertedImageNames: []string{"alpine:edge:amd64:cloud"},
			wantUpsertedFiles: []string{
				"linuxcontainers/alpine/edge/amd64/cloud/20260101/incus.tar.xz",
			},
			wantDeletedImageNames: nil,
			wantDeletedFiles:      nil,
		},
		{
			name:             "error - fileRepo.Put - commit cancel error",
			ctx:              t.Context(),
			filterExpression: `true`,

			simplestreamsGetImageList: image.IncusImages{
				{
					Name:            "alpine:edge:amd64:cloud",
					OperatingSystem: "Alpine",
					Release:         "edge",
					Architecture:    "amd64",
					Variant:         "cloud",
					Versions: api.IncusImageVersions{
						"20260101": api.IncusImageVersion{
							Items: map[string]api.IncusImageVersionItem{
								"incus.tar.xz": {},
								"root.tar.xz":  {},
							},
						},
					},
				},
			},
			repoGetAllWithFilter: image.IncusImages{
				// Present in DB, but misses files.
				{
					Source:          "linuxcontainers",
					Name:            "alpine:edge:amd64:cloud",
					OperatingSystem: "Alpine",
					Release:         "edge",
					Architecture:    "amd64",
					Variant:         "cloud",
					Versions: api.IncusImageVersions{
						"20260101": api.IncusImageVersion{
							Items: map[string]api.IncusImageVersionItem{
								"incus.tar.xz": {},
							},
						},
					},
				},
			},
			filesRepoUsageInformation: usageInfoGiB(50, 10),
			filesRepoPutCommitErr:     boom.Error,

			assertErr:              boom.ErrorIs,
			wantUpsertedImageNames: []string{"alpine:edge:amd64:cloud"},
			wantUpsertedFiles: []string{
				"linuxcontainers/alpine/edge/amd64/cloud/20260101/incus.tar.xz",
			},
			wantDeletedImageNames: nil,
			wantDeletedFiles:      nil,
		},
		{
			name:             "error - fileRepo.Put - commit cancel error",
			ctx:              t.Context(),
			filterExpression: `true`,

			simplestreamsGetImageList: image.IncusImages{
				{
					Name:            "alpine:edge:amd64:cloud",
					OperatingSystem: "Alpine",
					Release:         "edge",
					Architecture:    "amd64",
					Variant:         "cloud",
					Versions: api.IncusImageVersions{
						"20260101": api.IncusImageVersion{
							Items: map[string]api.IncusImageVersionItem{
								"incus.tar.xz": {},
								"root.tar.xz":  {},
							},
						},
					},
				},
			},
			repoGetAllWithFilter: image.IncusImages{
				// Present in DB, but misses files.
				{
					Source:          "linuxcontainers",
					Name:            "alpine:edge:amd64:cloud",
					OperatingSystem: "Alpine",
					Release:         "edge",
					Architecture:    "amd64",
					Variant:         "cloud",
					Versions: api.IncusImageVersions{
						"20260101": api.IncusImageVersion{
							Items: map[string]api.IncusImageVersionItem{
								"incus.tar.xz": {},
							},
						},
					},
				},
			},
			filesRepoUsageInformation: usageInfoGiB(50, 10),
			filesRepoPutErr:           errors.New("some error"),
			filesRepoPutCancelErr:     boom.Error,

			assertErr:              boom.ErrorIs,
			wantUpsertedImageNames: []string{"alpine:edge:amd64:cloud"},
			wantUpsertedFiles: []string{
				"linuxcontainers/alpine/edge/amd64/cloud/20260101/incus.tar.xz",
			},
			wantDeletedImageNames: nil,
			wantDeletedFiles:      nil,
		},
		{
			name:             "error - fileRepo.Put - invalid file hash",
			ctx:              t.Context(),
			filterExpression: `true`,

			simplestreamsGetImageList: image.IncusImages{
				{
					Name:            "alpine:edge:amd64:cloud",
					OperatingSystem: "Alpine",
					Release:         "edge",
					Architecture:    "amd64",
					Variant:         "cloud",
					Versions: api.IncusImageVersions{
						"20260101": api.IncusImageVersion{
							Items: map[string]api.IncusImageVersionItem{
								"incus.tar.xz": {},
								"root.tar.xz": {
									HashSha256: "0000000000000000000000000000000000000000000000000000000000000000", // incorrect hash
								},
							},
						},
					},
				},
			},
			repoGetAllWithFilter: image.IncusImages{
				// Present in DB, but misses files.
				{
					Source:          "linuxcontainers",
					Name:            "alpine:edge:amd64:cloud",
					OperatingSystem: "Alpine",
					Release:         "edge",
					Architecture:    "amd64",
					Variant:         "cloud",
					Versions: api.IncusImageVersions{
						"20260101": api.IncusImageVersion{
							Items: map[string]api.IncusImageVersionItem{
								"incus.tar.xz": {},
							},
						},
					},
				},
			},
			filesRepoUsageInformation: usageInfoGiB(50, 10),
			simplestreamsGetFileRC:    io.NopCloser(bytes.NewBufferString(`dummy`)),

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrConstraintViolation)
				require.ErrorContains(tt, err, `File "root.tar.xz" of image "alpine:edge:amd64:cloud", version "20260101" does not match the checksum the source "linuxcontainers" declares for it`)
			},
			wantUpsertedImageNames: []string{"alpine:edge:amd64:cloud"},
			wantUpsertedFiles: []string{
				"linuxcontainers/alpine/edge/amd64/cloud/20260101/incus.tar.xz",
			},
			wantDeletedImageNames: nil,
			wantDeletedFiles:      nil,
		},
		{
			name:             "error - repo.Upsert",
			ctx:              t.Context(),
			filterExpression: `true`,

			simplestreamsGetImageList: image.IncusImages{
				{
					Name:            "alpine:edge:amd64:cloud",
					OperatingSystem: "Alpine",
					Release:         "edge",
					Architecture:    "amd64",
					Variant:         "cloud",
					Versions: api.IncusImageVersions{
						"20260101": api.IncusImageVersion{
							Items: map[string]api.IncusImageVersionItem{
								"incus.tar.xz": {},
								"root.tar.xz":  {},
							},
						},
					},
				},
			},
			repoGetAllWithFilter: image.IncusImages{
				// Present in DB, but misses file.
				{
					Source:          "linuxcontainers",
					Name:            "alpine:edge:amd64:cloud",
					OperatingSystem: "Alpine",
					Release:         "edge",
					Architecture:    "amd64",
					Variant:         "cloud",
					Versions: api.IncusImageVersions{
						"20260101": api.IncusImageVersion{
							Items: map[string]api.IncusImageVersionItem{
								"incus.tar.xz": {},
							},
						},
					},
				},
			},
			filesRepoUsageInformation: usageInfoGiB(50, 10),
			repoUpsertErr:             boom.Error,

			assertErr:              boom.ErrorIs,
			wantUpsertedImageNames: []string{"alpine:edge:amd64:cloud"},
			wantUpsertedFiles: []string{
				"linuxcontainers/alpine/edge/amd64/cloud/20260101/incus.tar.xz",
				"linuxcontainers/alpine/edge/amd64/cloud/20260101/root.tar.xz",
			},
			wantDeletedImageNames: nil,
			wantDeletedFiles:      nil,
		},
		{
			name:             "success - filter without metadata file, version kept",
			ctx:              t.Context(),
			filterExpression: `file_type == "squashfs"`,

			simplestreamsGetImageList: image.IncusImages{
				{
					Name:            "alpine:edge:amd64:cloud",
					OperatingSystem: "Alpine",
					Release:         "edge",
					Architecture:    "amd64",
					Variant:         "cloud",
					Versions: api.IncusImageVersions{
						"20260101": api.IncusImageVersion{
							Items: map[string]api.IncusImageVersionItem{
								"incus.tar.xz":  {FileType: "incus.tar.xz"},
								"root.squashfs": {FileType: "squashfs"},
							},
						},
					},
				},
			},
			filesRepoUsageInformation: usageInfoGiB(50, 10),

			assertErr:              require.NoError,
			wantUpsertedImageNames: []string{"alpine:edge:amd64:cloud"},
			wantUpsertedFiles: []string{
				"linuxcontainers/alpine/edge/amd64/cloud/20260101/root.squashfs",
			},
			wantDeletedImageNames: nil,
			wantDeletedFiles:      nil,
		},
		{
			name:             "error - simplestreams.GetFile for one file of a new version, version not stored",
			ctx:              t.Context(),
			filterExpression: `true`,

			simplestreamsGetImageList: image.IncusImages{
				{
					Name:            "alpine:edge:amd64:cloud",
					OperatingSystem: "Alpine",
					Release:         "edge",
					Architecture:    "amd64",
					Variant:         "cloud",
					Versions: api.IncusImageVersions{
						"20260101": api.IncusImageVersion{
							Items: map[string]api.IncusImageVersionItem{
								"incus.tar.xz":  {FileType: "incus.tar.xz", Path: "20260101/incus.tar.xz"},
								"root.squashfs": {FileType: "squashfs", Path: "20260101/root.squashfs"},
								"disk.qcow2":    {FileType: "disk-kvm.img", Path: "20260101/disk.qcow2"},
							},
						},
					},
				},
			},
			filesRepoUsageInformation:    usageInfoGiB(50, 10),
			simplestreamsGetFileErrPaths: []string{"20260101/root.squashfs"},

			assertErr:              boom.ErrorIs,
			wantUpsertedImageNames: nil,
			wantUpsertedFiles:      nil,
			wantDeletedImageNames:  nil,
			wantDeletedFiles: []string{
				"linuxcontainers/alpine/edge/amd64/cloud/20260101/incus.tar.xz",
				"linuxcontainers/alpine/edge/amd64/cloud/20260101/disk.qcow2",
			},
		},
		{
			name:             "error - simplestreams.GetFile for one file of a stored version, version kept with its stored files",
			ctx:              t.Context(),
			filterExpression: `true`,

			simplestreamsGetImageList: image.IncusImages{
				{
					Name:            "alpine:edge:amd64:cloud",
					OperatingSystem: "Alpine",
					Release:         "edge",
					Architecture:    "amd64",
					Variant:         "cloud",
					Versions: api.IncusImageVersions{
						"20260101": api.IncusImageVersion{
							Items: map[string]api.IncusImageVersionItem{
								"incus.tar.xz":  {FileType: "incus.tar.xz", Path: "20260101/incus.tar.xz"},
								"root.squashfs": {FileType: "squashfs", Path: "20260101/root.squashfs"},
								"disk.qcow2":    {FileType: "disk-kvm.img", Path: "20260101/disk.qcow2"},
							},
						},
					},
				},
			},
			repoGetAllWithFilter: image.IncusImages{
				{
					Source:          "linuxcontainers",
					Name:            "alpine:edge:amd64:cloud",
					OperatingSystem: "Alpine",
					Release:         "edge",
					Architecture:    "amd64",
					Variant:         "cloud",
					Versions: api.IncusImageVersions{
						"20260101": api.IncusImageVersion{
							Items: map[string]api.IncusImageVersionItem{
								"incus.tar.xz": {FileType: "incus.tar.xz", Path: "20260101/incus.tar.xz"},
							},
						},
					},
				},
			},
			filesRepoUsageInformation:    usageInfoGiB(50, 10),
			simplestreamsGetFileErrPaths: []string{"20260101/root.squashfs"},

			assertErr:              boom.ErrorIs,
			wantUpsertedImageNames: []string{"alpine:edge:amd64:cloud"},
			wantUpsertedFiles: []string{
				"linuxcontainers/alpine/edge/amd64/cloud/20260101/incus.tar.xz",
			},
			wantDeletedImageNames: nil,
			wantDeletedFiles: []string{
				"linuxcontainers/alpine/edge/amd64/cloud/20260101/disk.qcow2",
			},
		},
		{
			name:             "error - simplestreams.GetFile for metadata file, version removed with its files",
			ctx:              t.Context(),
			filterExpression: `true`,

			simplestreamsGetImageList: image.IncusImages{
				{
					Name:            "alpine:edge:amd64:cloud",
					OperatingSystem: "Alpine",
					Release:         "edge",
					Architecture:    "amd64",
					Variant:         "cloud",
					Versions: api.IncusImageVersions{
						"20260101": api.IncusImageVersion{
							Items: map[string]api.IncusImageVersionItem{
								"incus.tar.xz":  {FileType: "incus.tar.xz", Path: "20260101/incus.tar.xz"},
								"root.squashfs": {FileType: "squashfs", Path: "20260101/root.squashfs"},
							},
						},
						"20260102": api.IncusImageVersion{
							Items: map[string]api.IncusImageVersionItem{
								"incus.tar.xz":  {FileType: "incus.tar.xz", Path: "20260102/incus.tar.xz"},
								"root.squashfs": {FileType: "squashfs", Path: "20260102/root.squashfs"},
							},
						},
					},
				},
			},
			filesRepoUsageInformation:    usageInfoGiB(50, 10),
			simplestreamsGetFileErrPaths: []string{"20260101/incus.tar.xz"},

			assertErr:              boom.ErrorIs,
			wantUpsertedImageNames: []string{"alpine:edge:amd64:cloud"},
			wantUpsertedFiles: []string{
				"linuxcontainers/alpine/edge/amd64/cloud/20260102/incus.tar.xz",
				"linuxcontainers/alpine/edge/amd64/cloud/20260102/root.squashfs",
			},
			wantDeletedImageNames: nil,
			wantDeletedFiles:      []string{"linuxcontainers/alpine/edge/amd64/cloud/20260101/root.squashfs"},
		},
		{
			name:             "error - filesRepo.DeleteVersionFile for files of a removed version",
			ctx:              t.Context(),
			filterExpression: `true`,

			simplestreamsGetImageList: image.IncusImages{
				{
					Name:            "alpine:edge:amd64:cloud",
					OperatingSystem: "Alpine",
					Release:         "edge",
					Architecture:    "amd64",
					Variant:         "cloud",
					Versions: api.IncusImageVersions{
						"20260101": api.IncusImageVersion{
							Items: map[string]api.IncusImageVersionItem{
								"incus.tar.xz":  {FileType: "incus.tar.xz", Path: "20260101/incus.tar.xz"},
								"root.squashfs": {FileType: "squashfs", Path: "20260101/root.squashfs"},
							},
						},
						"20260102": api.IncusImageVersion{
							Items: map[string]api.IncusImageVersionItem{
								"incus.tar.xz":  {FileType: "incus.tar.xz", Path: "20260102/incus.tar.xz"},
								"root.squashfs": {FileType: "squashfs", Path: "20260102/root.squashfs"},
							},
						},
					},
				},
			},
			filesRepoUsageInformation:     usageInfoGiB(50, 10),
			simplestreamsGetFileErrPaths:  []string{"20260101/incus.tar.xz"},
			filesRepoDeleteVersionFileErr: errors.New("delete failed"),

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, boom.Error, a...)
				require.ErrorContains(tt, err, "delete failed", a...)
			},
			wantUpsertedImageNames: []string{"alpine:edge:amd64:cloud"},
			wantUpsertedFiles: []string{
				"linuxcontainers/alpine/edge/amd64/cloud/20260102/incus.tar.xz",
				"linuxcontainers/alpine/edge/amd64/cloud/20260102/root.squashfs",
			},
			wantDeletedImageNames: nil,
			wantDeletedFiles:      []string{"linuxcontainers/alpine/edge/amd64/cloud/20260101/root.squashfs"},
		},
		{
			name:             "error - simplestreams.GetFile for all files of a new image, image not stored",
			ctx:              t.Context(),
			filterExpression: `true`,

			simplestreamsGetImageList: image.IncusImages{
				{
					Name:            "alpine:edge:amd64:cloud",
					OperatingSystem: "Alpine",
					Release:         "edge",
					Architecture:    "amd64",
					Variant:         "cloud",
					Versions: api.IncusImageVersions{
						"20260101": api.IncusImageVersion{
							Items: map[string]api.IncusImageVersionItem{
								"incus.tar.xz":  {FileType: "incus.tar.xz"},
								"root.squashfs": {FileType: "squashfs"},
							},
						},
					},
				},
			},
			filesRepoUsageInformation: usageInfoGiB(50, 10),
			simplestreamsGetFileErr:   boom.Error,

			assertErr:              boom.ErrorIs,
			wantUpsertedImageNames: nil,
			wantUpsertedFiles:      nil,
			wantDeletedImageNames:  nil,
			wantDeletedFiles:       nil,
		},
		{
			name:             "error - simplestreams.GetFile for all new versions of an existing image, image removed from DB",
			ctx:              t.Context(),
			filterExpression: `true`,

			simplestreamsGetImageList: image.IncusImages{
				{
					Name:            "alpine:edge:amd64:cloud",
					OperatingSystem: "Alpine",
					Release:         "edge",
					Architecture:    "amd64",
					Variant:         "cloud",
					Versions: api.IncusImageVersions{
						"20260101": api.IncusImageVersion{
							Items: map[string]api.IncusImageVersionItem{
								"incus.tar.xz": {FileType: "incus.tar.xz"},
							},
						},
					},
				},
			},
			repoGetAllWithFilter: image.IncusImages{
				{
					Source:          "linuxcontainers",
					Name:            "alpine:edge:amd64:cloud",
					OperatingSystem: "Alpine",
					Release:         "edge",
					Architecture:    "amd64",
					Variant:         "cloud",
					Versions: api.IncusImageVersions{
						"20251201": api.IncusImageVersion{
							Items: map[string]api.IncusImageVersionItem{
								"incus.tar.xz": {FileType: "incus.tar.xz"},
							},
						},
					},
				},
			},
			filesRepoUsageInformation: usageInfoGiB(50, 10),
			simplestreamsGetFileErr:   boom.Error,

			assertErr:              boom.ErrorIs,
			wantUpsertedImageNames: nil,
			wantUpsertedFiles:      nil,
			wantDeletedImageNames:  []string{"linuxcontainers/alpine:edge:amd64:cloud"},
			wantDeletedFiles:       []string{"linuxcontainers/alpine/edge/amd64/cloud/20251201/incus.tar.xz"},
		},
		{
			name:             "error - repo.DeleteByName for an image without files",
			ctx:              t.Context(),
			filterExpression: `true`,

			simplestreamsGetImageList: image.IncusImages{
				{
					Name:            "alpine:edge:amd64:cloud",
					OperatingSystem: "Alpine",
					Release:         "edge",
					Architecture:    "amd64",
					Variant:         "cloud",
					Versions: api.IncusImageVersions{
						"20260101": api.IncusImageVersion{
							Items: map[string]api.IncusImageVersionItem{
								"incus.tar.xz": {FileType: "incus.tar.xz"},
							},
						},
					},
				},
			},
			repoGetAllWithFilter: image.IncusImages{
				{
					Source:          "linuxcontainers",
					Name:            "alpine:edge:amd64:cloud",
					OperatingSystem: "Alpine",
					Release:         "edge",
					Architecture:    "amd64",
					Variant:         "cloud",
					Versions: api.IncusImageVersions{
						"20251201": api.IncusImageVersion{
							Items: map[string]api.IncusImageVersionItem{
								"incus.tar.xz": {FileType: "incus.tar.xz"},
							},
						},
					},
				},
			},
			filesRepoUsageInformation: usageInfoGiB(50, 10),
			simplestreamsGetFileErr:   errors.New("download failed"),
			repoDeleteByNameErr:       boom.Error,

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, boom.Error, a...)
				require.ErrorContains(tt, err, "download failed", a...)
			},
			wantUpsertedImageNames: nil,
			wantUpsertedFiles:      nil,
			wantDeletedImageNames:  []string{"linuxcontainers/alpine:edge:amd64:cloud"},
			wantDeletedFiles:       []string{"linuxcontainers/alpine/edge/amd64/cloud/20251201/incus.tar.xz"},
		},
		{
			name:             "error - 10 failed downloads, remaining images stored without their deleted files",
			ctx:              t.Context(),
			filterExpression: `true`,

			simplestreamsGetImageList: image.IncusImages{
				{
					Name:            "alpine:edge:amd64:cloud",
					OperatingSystem: "Alpine",
					Release:         "edge",
					Architecture:    "amd64",
					Variant:         "cloud",
					Versions: api.IncusImageVersions{
						"20260101": api.IncusImageVersion{
							Items: map[string]api.IncusImageVersionItem{
								"incus.tar.xz":  {},
								"root01.tar.xz": {Path: "root01.tar.xz"},
								"root02.tar.xz": {Path: "root02.tar.xz"},
								"root03.tar.xz": {Path: "root03.tar.xz"},
								"root04.tar.xz": {Path: "root04.tar.xz"},
								"root05.tar.xz": {Path: "root05.tar.xz"},
								"root06.tar.xz": {Path: "root06.tar.xz"},
								"root07.tar.xz": {Path: "root07.tar.xz"},
								"root08.tar.xz": {Path: "root08.tar.xz"},
								"root09.tar.xz": {Path: "root09.tar.xz"},
								"root10.tar.xz": {Path: "root10.tar.xz"},
							},
						},
					},
				},
				{
					Name:            "alpine:edge:amd64:default",
					OperatingSystem: "Alpine",
					Release:         "edge",
					Architecture:    "amd64",
					Variant:         "default",
					Versions: api.IncusImageVersions{
						"20260101": api.IncusImageVersion{
							Items: map[string]api.IncusImageVersionItem{
								"incus.tar.xz": {},
							},
						},
						"20260102": api.IncusImageVersion{
							Items: map[string]api.IncusImageVersionItem{
								"incus.tar.xz": {}, // Download would succeed, but the refresh stopped.
							},
						},
					},
				},
			},
			repoGetAllWithFilter: image.IncusImages{
				{
					Source:          "linuxcontainers",
					Name:            "alpine:edge:amd64:cloud",
					OperatingSystem: "Alpine",
					Release:         "edge",
					Architecture:    "amd64",
					Variant:         "cloud",
					Versions: api.IncusImageVersions{
						"20260101": api.IncusImageVersion{
							Items: map[string]api.IncusImageVersionItem{
								"incus.tar.xz": {},
							},
						},
					},
				},
				{
					Source:          "linuxcontainers",
					Name:            "alpine:edge:amd64:default",
					OperatingSystem: "Alpine",
					Release:         "edge",
					Architecture:    "amd64",
					Variant:         "default",
					Versions: api.IncusImageVersions{
						"20251201": api.IncusImageVersion{
							Items: map[string]api.IncusImageVersionItem{
								"incus.tar.xz": {},
							},
						},
						"20260101": api.IncusImageVersion{
							Items: map[string]api.IncusImageVersionItem{
								"incus.tar.xz": {},
							},
						},
					},
				},
			},
			filesRepoUsageInformation: usageInfoGiB(50, 10),
			simplestreamsGetFileErrPaths: []string{
				"root01.tar.xz", "root02.tar.xz", "root03.tar.xz", "root04.tar.xz", "root05.tar.xz",
				"root06.tar.xz", "root07.tar.xz", "root08.tar.xz", "root09.tar.xz", "root10.tar.xz",
			},

			assertErr:              boom.ErrorIs,
			wantUpsertedImageNames: []string{"alpine:edge:amd64:cloud", "alpine:edge:amd64:default"},
			wantUpsertedFiles: []string{
				"linuxcontainers/alpine/edge/amd64/cloud/20260101/incus.tar.xz",
				"linuxcontainers/alpine/edge/amd64/default/20260101/incus.tar.xz",
			},
			wantDeletedImageNames: nil,
			wantDeletedFiles:      []string{"linuxcontainers/alpine/edge/amd64/default/20251201/incus.tar.xz"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			var upsertedImageNames []string
			var upsertedFiles []string
			var deletedImageNames []string
			var deletedFiles []string
			repo := &mock.ImageIncusRepoMock{
				GetAllWithFilterFunc: func(ctx context.Context, filter image.IncusImageFilter) (image.IncusImages, error) {
					require.Equal(t, image.IncusImageFilter{Source: new("linuxcontainers")}, filter, "only the images of the refreshed source are compared")
					return tc.repoGetAllWithFilter, tc.repoGetAllWithFilterErr
				},
				DeleteByNameFunc: func(ctx context.Context, source string, name string) error {
					deletedImageNames = append(deletedImageNames, source+"/"+name)
					return tc.repoDeleteByNameErr
				},
				UpsertFunc: func(ctx context.Context, incusImage image.IncusImage) error {
					upsertedImageNames = append(upsertedImageNames, incusImage.Name)
					for versionIdentifier, version := range incusImage.Versions {
						for filename := range version.Items {
							upsertedFiles = append(upsertedFiles, path.Join(incusImage.FilePath(), versionIdentifier, filename))
						}
					}

					return tc.repoUpsertErr
				},
			}

			filesRepo := &mock.ImageIncusFileRepoMock{
				DeleteVersionFileFunc: func(ctx context.Context, img *image.IncusImage, versionIdentifier, filename string) error {
					deletedFiles = append(deletedFiles, path.Join(img.FilePath(), versionIdentifier, filename))
					return tc.filesRepoDeleteVersionFileErr
				},
				UsageInformationFunc: func(ctx context.Context) (file.UsageInformation, error) {
					return tc.filesRepoUsageInformation, tc.filesRepoUsageInformationErr
				},
				PutFunc: func(ctx context.Context, img *image.IncusImage, versionIdentifier, filename string, content io.ReadCloser) (image.CommitFunc, image.CancelFunc, int64, error) {
					if content != nil {
						_, err := io.ReadAll(content)
						require.NoError(t, err)
					}

					return func() error { return tc.filesRepoPutCommitErr }, func() error { return tc.filesRepoPutCancelErr }, 0, tc.filesRepoPutErr
				},
			}

			simplestreams := &adapterMock.SimplestreamsPortMock{
				GetImageListFunc: func(ctx context.Context, source image.IncusImageSource) (image.IncusImages, error) {
					return tc.simplestreamsGetImageList, tc.simplestreamsGetImageListErr
				},
				GetFileFunc: func(ctx context.Context, source image.IncusImageSource, path string) (io.ReadCloser, error) {
					if slices.Contains(tc.simplestreamsGetFileErrPaths, path) {
						return nil, boom.Error
					}

					return tc.simplestreamsGetFileRC, tc.simplestreamsGetFileErr
				},
			}

			imageSvc := image.NewIncusImage(repo, filesRepo, simplestreams)

			// Run test
			err := imageSvc.RefreshFromSource(tc.ctx, image.IncusImageSource{
				Name:             "linuxcontainers",
				URL:              "https://images.linuxcontainers.org",
				FilterExpression: tc.filterExpression,
			})

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.wantUpsertedImageNames, upsertedImageNames, "names of the images stored in the DB")
			require.ElementsMatch(t, tc.wantUpsertedFiles, upsertedFiles, "files of the images stored in the DB")
			require.Equal(t, tc.wantDeletedImageNames, deletedImageNames, "source and name of the images deleted from the DB")
			require.ElementsMatch(t, tc.wantDeletedFiles, deletedFiles, "files deleted from the files repository")
		})
	}
}

func usageInfoGiB(totalSpaceGiB int, availableSpaceGiB int) file.UsageInformation {
	const GiB = 1024 * 1024 * 1024
	return file.UsageInformation{
		TotalSpaceBytes:     uint64(totalSpaceGiB) * GiB,
		AvailableSpaceBytes: uint64(availableSpaceGiB) * GiB,
		UsedSpaceBytes:      uint64(totalSpaceGiB-availableSpaceGiB) * GiB,
	}
}
