package image_test

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"io"
	"mime/multipart"
	"net/textproto"
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
	"github.com/FuturFusion/operations-center/internal/util/testing/boom"
	"github.com/FuturFusion/operations-center/internal/util/testing/errassert"
	"github.com/FuturFusion/operations-center/internal/util/testing/queue"
	"github.com/FuturFusion/operations-center/shared/api"
)

func TestImageIncusService_AddVersion(t *testing.T) {
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

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:               "success - with metadata from incus.tar.xz",
			multipartReaderArg: validMultipartReaderWithIncusTarXZ(t),
			repoGetByName: []queue.Item[*image.IncusImage]{
				// lookup
				{
					Value: &image.IncusImage{
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
				// incus_combined.tar.gz
				{
					Value: fileRepoGetValue{
						reader: io.NopCloser(bytes.NewBufferString(`incus combined`)),
						size:   int64(len(`incus combined`)),
					},
				},
			},

			assertErr: require.NoError,
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
				// incus_combined.tar.gz
				{
					Value: fileRepoGetValue{
						reader: io.NopCloser(bytes.NewBufferString(`incus combined`)),
						size:   int64(len(`incus combined`)),
					},
				},
			},

			assertErr: require.NoError,
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

			assertErr: require.NoError,
		},
		{
			name:               "error - invalid multipart reader",
			multipartReaderArg: multipart.NewReader(bytes.NewBufferString(`invalid`), ``),

			assertErr: require.Error,
		},
		{
			name:               "error - multipart reader without metadata file",
			multipartReaderArg: multipartReaderWithoutMetadataFile(t),

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrOperationNotPermitted)
				require.ErrorContains(tt, err, `First part of the multipart request is required to be either "request_json" or the file "incus.tar.xz", got form-name "file", filename "root.tar.xz"`)
			},
		},
		{
			name:               "error - failed to read metadata",
			multipartReaderArg: validMultipartReaderWithRequestJSON(t, `{`), // invalid metadata

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, "Failed to read metadata")
			},
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

			assertErr: errassert.ValidationErrorContains("Incomplete metadata, not all required properties set"),
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

			assertErr: errassert.ValidationErrorContains("Invalid incus image version"),
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

			assertErr: boom.ErrorIs,
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

			assertErr: boom.ErrorIs,
		},
		{
			name:               "error - version already exists",
			multipartReaderArg: validMultipartReaderWithIncusTarXZ(t),
			repoGetByName: []queue.Item[*image.IncusImage]{
				// lookup
				{
					Value: &image.IncusImage{
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
		},
		{
			name:               "error - filesRepo.Put",
			multipartReaderArg: validMultipartReaderWithIncusTarXZ(t),
			repoGetByName: []queue.Item[*image.IncusImage]{
				// lookup
				{
					Value: &image.IncusImage{
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

			assertErr: boom.ErrorIs,
		},
		{
			name:               "error - filesRepo.Put - cancel",
			multipartReaderArg: validMultipartReaderWithIncusTarXZ(t),
			repoGetByName: []queue.Item[*image.IncusImage]{
				// lookup
				{
					Value: &image.IncusImage{
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
		},
		{
			name:               "error - filesRepo.Put - commit",
			multipartReaderArg: validMultipartReaderWithIncusTarXZ(t),
			repoGetByName: []queue.Item[*image.IncusImage]{
				// lookup
				{
					Value: &image.IncusImage{
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

			assertErr: boom.ErrorIs,
		},
		{
			name:               "error - invalid 2nd part in multipart message",
			multipartReaderArg: multipartReaderWithInvalid2ndPart(t),
			repoGetByName: []queue.Item[*image.IncusImage]{
				// lookup
				{
					Value: &image.IncusImage{
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

			assertErr: require.Error,
		},
		{
			name:               "error - 2nd filesRepo.Put",
			multipartReaderArg: validMultipartReaderWithIncusTarXZ(t),
			repoGetByName: []queue.Item[*image.IncusImage]{
				// lookup
				{
					Value: &image.IncusImage{
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

			assertErr: boom.ErrorIs,
		},
		{
			name:               "error - 2nd filesRepo.Put - cancel",
			multipartReaderArg: validMultipartReaderWithIncusTarXZ(t),
			repoGetByName: []queue.Item[*image.IncusImage]{
				// lookup
				{
					Value: &image.IncusImage{
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
		},
		{
			name:               "error - 2nd filesRepo.Put - commit",
			multipartReaderArg: validMultipartReaderWithIncusTarXZ(t),
			repoGetByName: []queue.Item[*image.IncusImage]{
				// lookup
				{
					Value: &image.IncusImage{
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

			assertErr: boom.ErrorIs,
		},
		{
			name:               "error - filesRepo.Get metadata for checksum",
			multipartReaderArg: validMultipartReaderWithIncusTarXZ(t),
			repoGetByName: []queue.Item[*image.IncusImage]{
				// lookup
				{
					Value: &image.IncusImage{
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
				{},
			},
			filesRepoGet: []queue.Item[fileRepoGetValue]{
				// incus_combined.tar.gz
				{
					Value: fileRepoGetValue{
						reader: io.NopCloser(bytes.NewBufferString(`incus combined`)),
						size:   int64(len(`incus combined`)),
					},
				},
				// incus.tar.xz for root.tar.xz
				{
					Err: boom.Error,
				},
			},

			assertErr: boom.ErrorIs,
		},
		{
			name:               "error - read metadata for checksum",
			multipartReaderArg: validMultipartReaderWithIncusTarXZ(t),
			repoGetByName: []queue.Item[*image.IncusImage]{
				// lookup
				{
					Value: &image.IncusImage{
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
				{},
			},
			filesRepoGet: []queue.Item[fileRepoGetValue]{
				// incus_combined.tar.gz
				{
					Value: fileRepoGetValue{
						reader: io.NopCloser(bytes.NewBufferString(`incus combined`)),
						size:   int64(len(`incus combined`)),
					},
				},
				// incus.tar.xz for root.tar.xz
				{
					Value: fileRepoGetValue{
						reader: io.NopCloser(iotest.ErrReader(boom.Error)),
						size:   int64(len(`incus tar xz`)),
					},
				},
			},

			assertErr: boom.ErrorIs,
		},
		{
			name:               "error - filesRepo.Get image for checksum",
			multipartReaderArg: validMultipartReaderWithIncusTarXZ(t),
			repoGetByName: []queue.Item[*image.IncusImage]{
				// lookup
				{
					Value: &image.IncusImage{
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
				{},
			},
			filesRepoGet: []queue.Item[fileRepoGetValue]{
				// incus_combined.tar.gz
				{
					Value: fileRepoGetValue{
						reader: io.NopCloser(bytes.NewBufferString(`incus combined`)),
						size:   int64(len(`incus combined`)),
					},
				},
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

			assertErr: boom.ErrorIs,
		},
		{
			name:               "error - read image for checksum",
			multipartReaderArg: validMultipartReaderWithIncusTarXZ(t),
			repoGetByName: []queue.Item[*image.IncusImage]{
				// lookup
				{
					Value: &image.IncusImage{
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
				{},
			},
			filesRepoGet: []queue.Item[fileRepoGetValue]{
				// incus_combined.tar.gz
				{
					Value: fileRepoGetValue{
						reader: io.NopCloser(bytes.NewBufferString(`incus combined`)),
						size:   int64(len(`incus combined`)),
					},
				},
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

			assertErr: boom.ErrorIs,
		},
		{
			name:               "error - repo.GetByName in transaction for update",
			multipartReaderArg: validMultipartReaderWithIncusTarXZ(t),
			repoGetByName: []queue.Item[*image.IncusImage]{
				// lookup
				{
					Value: &image.IncusImage{
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
				// incus_combined.tar.gz
				{
					Value: fileRepoGetValue{
						reader: io.NopCloser(bytes.NewBufferString(`incus combined`)),
						size:   int64(len(`incus combined`)),
					},
				},
			},

			assertErr: boom.ErrorIs,
		},
		{
			name:               "error - repo.Update",
			multipartReaderArg: validMultipartReaderWithIncusTarXZ(t),
			repoGetByName: []queue.Item[*image.IncusImage]{
				// lookup
				{
					Value: &image.IncusImage{
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
				// incus_combined.tar.gz
				{
					Value: fileRepoGetValue{
						reader: io.NopCloser(bytes.NewBufferString(`incus combined`)),
						size:   int64(len(`incus combined`)),
					},
				},
			},
			repoUpdateErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &mock.ImageIncusRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*image.IncusImage, error) {
					return queue.Pop(t, &tc.repoGetByName)
				},
				CreateFunc: func(ctx context.Context, newIncusImage image.IncusImage) (int64, error) {
					return 0, tc.repoCreateErr
				},
				UpdateFunc: func(ctx context.Context, newIncusImage image.IncusImage) error {
					// TODO: ensure correct values for size and sha256 hashes
					// t.Log(newIncusImage)
					return tc.repoUpdateErr
				},
			}

			filesRepo := &mock.ImageIncusFileRepoMock{
				PutFunc: func(ctx context.Context, img *image.IncusImage, versionIdentifier, filename string, content io.ReadCloser) (image.CommitFunc, image.CancelFunc, int64, error) {
					size, err := io.ReadAll(content)
					require.NoError(t, err)

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
		})
	}
}

func validMultipartReaderWithIncusTarXZ(t *testing.T) *multipart.Reader {
	t.Helper()

	var body bytes.Buffer

	writer := multipart.NewWriter(&body)

	// incus.tar.xz
	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition",
		`form-data; name="file"; filename="incus.tar.xz"`)
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

	// root.tar.xz
	header = textproto.MIMEHeader{}
	header.Set("Content-Disposition",
		`form-data; name="file"; filename="root.tar.xz"`)
	header.Set("Content-Type", "application/octet-stream")

	part, err = writer.CreatePart(header)
	require.NoError(t, err)

	_, err = io.WriteString(part, "root tar xz")
	require.NoError(t, err)

	// root.squashfs
	header = textproto.MIMEHeader{}
	header.Set("Content-Disposition",
		`form-data; name="file"; filename="root.squashfs"`)
	header.Set("Content-Type", "application/octet-stream")

	part, err = writer.CreatePart(header)
	require.NoError(t, err)

	_, err = io.WriteString(part, "squashfs")
	require.NoError(t, err)

	// disk.qcow2
	header = textproto.MIMEHeader{}
	header.Set("Content-Disposition",
		`form-data; name="file"; filename="disk.qcow2"`)
	header.Set("Content-Type", "application/octet-stream")

	part, err = writer.CreatePart(header)
	require.NoError(t, err)

	_, err = io.WriteString(part, "disk qcow2")
	require.NoError(t, err)

	// incus_combined.tar.gz
	header = textproto.MIMEHeader{}
	header.Set("Content-Disposition",
		`form-data; name="file"; filename="incus_combined.tar.gz"`)
	header.Set("Content-Type", "application/octet-stream")

	part, err = writer.CreatePart(header)
	require.NoError(t, err)

	_, err = io.WriteString(part, "incus combined")
	require.NoError(t, err)

	err = writer.Close()
	require.NoError(t, err)

	return multipart.NewReader(&body, writer.Boundary())
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
		`form-data; name="file"; filename="disk.qcow2"`)
	header.Set("Content-Type", "application/octet-stream")

	part, err = writer.CreatePart(header)
	require.NoError(t, err)

	_, err = io.WriteString(part, "disk qcow2")
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

	// incus.tar.xz
	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition",
		`form-data; name="file"; filename="incus.tar.xz"`)
	header.Set("Content-Type", "application/octet-stream")

	part, err := writer.CreatePart(header)
	require.NoError(t, err)

	metadata := incusapi.ImageMetadata{
		Properties: map[string]string{
			"os":           "almalinux",
			"release":      "10",
			"architecture": "amd64",
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

	// append invalid multipart content
	_, err = body.WriteString(strings.Join([]string{
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

func TestImageIncusService_GetAllNames(t *testing.T) {
	tests := []struct {
		name               string
		repoGetAllNames    []string
		repoGetAllNamesErr error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "success",
			repoGetAllNames: []string{
				"almalinux:10:amd64:cloud",
				"almalinux:10:amd64:default",
			},

			assertErr: require.NoError,
		},
		{
			name:               "error - repo",
			repoGetAllNamesErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &mock.ImageIncusRepoMock{
				GetAllNamesFunc: func(ctx context.Context) ([]string, error) {
					return tc.repoGetAllNames, tc.repoGetAllNamesErr
				},
			}

			imageSvc := image.NewIncusImage(repo, nil, nil)

			// Run test
			images, err := imageSvc.GetAllNames(t.Context())

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.repoGetAllNames, images)
		})
	}
}

func TestImageIncusService_GetByName(t *testing.T) {
	tests := []struct {
		name             string
		nameArg          string
		repoGetByName    *image.IncusImage
		repoGetByNameErr error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:          "success",
			nameArg:       "almalinux:10:amd64:cloud",
			repoGetByName: &image.IncusImage{},

			assertErr: require.NoError,
		},
		{
			name:    "error - empty name",
			nameArg: "", // empty

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
				require.ErrorContains(tt, err, `Invalid incus image name, expect name in the format "os:release:architecture:variant"`)
			},
		},
		{
			name:             "error - repo",
			nameArg:          "almalinux:10:amd64:cloud",
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &mock.ImageIncusRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*image.IncusImage, error) {
					return tc.repoGetByName, tc.repoGetByNameErr
				},
			}

			imageSvc := image.NewIncusImage(repo, nil, nil)

			// Run test
			img, err := imageSvc.GetByName(t.Context(), tc.nameArg)

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.repoGetByName, img)
		})
	}
}

func TestImageIncusService_DeleteByName(t *testing.T) {
	tests := []struct {
		name                string
		argName             string
		filesRepoDeleteErr  error
		repoGetByName       *image.IncusImage
		repoGetByNameErr    error
		repoDeleteByNameErr error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:    "success",
			argName: "almalinux:10:amd64:cloud",
			repoGetByName: &image.IncusImage{
				Name: "almalinux:10:amd64:cloud",
			},

			assertErr: require.NoError,
		},
		{
			name:    "error - invalid name",
			argName: "", // empty name

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
				require.ErrorContains(tt, err, `Invalid incus image name, expect name in the format "os:release:architecture:variant"`)
			},
		},
		{
			name:             "error - repo.GetByName",
			argName:          "almalinux:10:amd64:cloud",
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:    "error - repo.DeleteByName",
			argName: "almalinux:10:amd64:cloud",
			repoGetByName: &image.IncusImage{
				Name: "almalinux:10:amd64:cloud",
			},
			repoDeleteByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:    "error - filesRepo.Delete",
			argName: "almalinux:10:amd64:cloud",
			repoGetByName: &image.IncusImage{
				Name: "almalinux:10:amd64:cloud",
			},
			filesRepoDeleteErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &mock.ImageIncusRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*image.IncusImage, error) {
					return tc.repoGetByName, tc.repoGetByNameErr
				},
				DeleteByNameFunc: func(ctx context.Context, name string) error {
					return tc.repoDeleteByNameErr
				},
			}

			filesRepo := &mock.ImageIncusFileRepoMock{
				DeleteFunc: func(ctx context.Context, img *image.IncusImage) error {
					return tc.filesRepoDeleteErr
				},
			}

			imageSvc := image.NewIncusImage(repo, filesRepo, nil)

			// Run test
			err := imageSvc.DeleteByName(t.Context(), tc.argName)

			// Assert
			tc.assertErr(t, err)
		})
	}
}

func TestImageIncusService_DeleteBySource(t *testing.T) {
	tests := []struct {
		name                    string
		argName                 string
		repoGetAllWithFilter    image.IncusImages
		repoGetAllWithFilterErr error
		filesRepoDeleteErr      error
		repoDeleteByNameErr     error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:    "success",
			argName: "almalinux:10:amd64:cloud",
			repoGetAllWithFilter: image.IncusImages{
				image.IncusImage{
					Name: "almalinux:10:amd64:cloud",
				},
			},

			assertErr: require.NoError,
		},
		{
			name:    "error - invalid name",
			argName: "", // empty name

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
				require.ErrorContains(tt, err, `Invalid incus image name, expect name in the format "os:release:architecture:variant"`)
			},
		},
		{
			name:                    "error - repo.GetByName",
			argName:                 "almalinux:10:amd64:cloud",
			repoGetAllWithFilterErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:    "error - repo.DeleteByName",
			argName: "almalinux:10:amd64:cloud",
			repoGetAllWithFilter: image.IncusImages{
				image.IncusImage{
					Name: "almalinux:10:amd64:cloud",
				},
			},
			repoDeleteByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:    "error - filesRepo.Delete",
			argName: "almalinux:10:amd64:cloud",
			repoGetAllWithFilter: image.IncusImages{
				image.IncusImage{
					Name: "almalinux:10:amd64:cloud",
				},
			},
			filesRepoDeleteErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &mock.ImageIncusRepoMock{
				GetAllWithFilterFunc: func(ctx context.Context, filter image.IncusImageFilter) (image.IncusImages, error) {
					return tc.repoGetAllWithFilter, tc.repoGetAllWithFilterErr
				},
				DeleteByNameFunc: func(ctx context.Context, name string) error {
					return tc.repoDeleteByNameErr
				},
			}

			filesRepo := &mock.ImageIncusFileRepoMock{
				DeleteFunc: func(ctx context.Context, img *image.IncusImage) error {
					return tc.filesRepoDeleteErr
				},
			}

			imageSvc := image.NewIncusImage(repo, filesRepo, nil)

			// Run test
			err := imageSvc.DeleteBySource(t.Context(), tc.argName)

			// Assert
			tc.assertErr(t, err)
		})
	}
}

func TestImageIncusService_DeleteVersionByName(t *testing.T) {
	tests := []struct {
		name                      string
		argName                   string
		argVersion                string
		filesRepoDeleteVersionErr error
		repoGetByName             *image.IncusImage
		repoGetByNameErr          error
		repoUpdateErr             error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:       "success",
			argName:    "almalinux:10:amd64:cloud",
			argVersion: "20260514",
			repoGetByName: &image.IncusImage{
				Name: "almalinux:10:amd64:cloud",
				Versions: map[string]api.IncusImageVersion{
					"20260514": {},
					"20260515": {},
				},
			},

			assertErr: require.NoError,
		},
		{
			name:    "error - invalid name",
			argName: "", // empty name

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
				require.ErrorContains(tt, err, `Invalid incus image name, expect name in the format "os:release:architecture:variant"`)
			},
		},
		{
			name:       "error - invalid version",
			argName:    "almalinux:10:amd64:cloud",
			argVersion: "", // empty version

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
				require.ErrorContains(tt, err, "Incus image version cannot be empty")
			},
		},
		{
			name:             "error - repo.GetByName",
			argName:          "almalinux:10:amd64:cloud",
			argVersion:       "20260514",
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:       "error - version not found",
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
				require.ErrorContains(tt, err, `Failed to delete version "20260514" from incus image "almalinux:10:amd64:cloud"`)
			},
		},
		{
			name:       "error - repo.Update",
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

			assertErr: boom.ErrorIs,
		},
		{
			name:       "error - filesRepo.CleanupAll",
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

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &mock.ImageIncusRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*image.IncusImage, error) {
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
			err := imageSvc.DeleteVersionByName(t.Context(), tc.argName, tc.argVersion)

			// Assert
			tc.assertErr(t, err)
		})
	}
}

func TestIncusImageService_GetVersionFileByName(t *testing.T) {
	assertZeroValues := func(t *testing.T, rc io.ReadCloser, size int64) {
		t.Helper()

		require.Nil(t, rc)
		require.Zero(t, size)
	}

	tests := []struct {
		name             string
		argName          string
		argVersion       string
		argFilename      string
		repoGetByName    *image.IncusImage
		repoGetByNameErr error
		filesRepoGetRC   io.ReadCloser
		fileRepoGetSize  int64
		filesRepoGetErr  error

		assertErr require.ErrorAssertionFunc
		assert    func(t *testing.T, rc io.ReadCloser, size int64)
	}{
		{
			name:        "success",
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
		},
		{
			name:    "error - invalid name",
			argName: "", // empty name

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
				require.ErrorContains(tt, err, `Invalid incus image name, expect name in the format "os:release:architecture:variant"`)
			},
			assert: assertZeroValues,
		},
		{
			name:       "error - invalid version",
			argName:    "almalinux:10:amd64:cloud",
			argVersion: "", // empty version

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
				require.ErrorContains(tt, err, "Incus image version cannot be empty")
			},
			assert: assertZeroValues,
		},
		{
			name:        "error - invalid filename",
			argName:     "almalinux:10:amd64:cloud",
			argVersion:  "20260520",
			argFilename: "", // empty filename

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
				require.ErrorContains(tt, err, "Filename cannot be empty")
			},
			assert: assertZeroValues,
		},
		{
			name:             "error - repo.GetByName",
			argName:          "almalinux:10:amd64:cloud",
			argVersion:       "20260520",
			argFilename:      "somefile.txt",
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
			assert:    assertZeroValues,
		},
		{
			name:        "error - filesRepo.GetFunc",
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

			assertErr: boom.ErrorIs,
			assert:    assertZeroValues,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := &mock.ImageIncusRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*image.IncusImage, error) {
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
			rc, size, err := imageSvc.GetVersionFileByName(t.Context(), tc.argName, tc.argVersion, tc.argFilename)

			// Assert
			tc.assertErr(t, err)
			tc.assert(t, rc, size)
		})
	}
}

func TestIncusImageService_Update(t *testing.T) {
	tests := []struct {
		name          string
		incusImage    image.IncusImage
		repoUpdateErr error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "success",
			incusImage: image.IncusImage{
				Name:            "almalinux:10:amd64:cloud",
				OperatingSystem: "almalinux",
				Release:         "10",
				Architecture:    "amd64",
				Variant:         "cloud",
			},

			assertErr: require.NoError,
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
		},
		{
			name: "error - repo.Update",
			incusImage: image.IncusImage{
				Name:            "almalinux:10:amd64:cloud",
				OperatingSystem: "almalinux",
				Release:         "10",
				Architecture:    "amd64",
				Variant:         "cloud",
			},
			repoUpdateErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
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
		})
	}
}

func TestIncusImageService_ValidateFilterExpression(t *testing.T) {
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
			// Setup
			imageSvc := image.NewIncusImage(nil, nil, nil)

			// Run test
			err := imageSvc.ValidateFilterExpression(t.Context(), tc.argFilterExpression)

			// Assert
			tc.assertErr(t, err)
		})
	}
}

func TestIncusImageService_RefreshFromSource(t *testing.T) {
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
		filesRepoUsageInformation     image.UsageInformation
		filesRepoUsageInformationErr  error
		simplestreamsGetFileRC        io.ReadCloser
		simplestreamsGetFileErr       error
		filesRepoPutCommitErr         error
		filesRepoPutCancelErr         error
		filesRepoPutErr               error
		repoUpsertErr                 error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "success - no updates, no state in the DB",
			ctx:  t.Context(),

			assertErr: require.NoError,
		},
		{
			name:             "success - one update, filtered",
			ctx:              t.Context(),
			filterExpression: "false", // filter everything

			simplestreamsGetImageList: image.IncusImages{
				{
					Name: "alpine:edge:amd64:default",
					Versions: api.IncusImageVersions{
						"20260101": api.IncusImageVersion{
							Items: map[string]api.IncusImageVersionItem{
								"incus.tar.xz": {},
							},
						},
					},
				},
			},

			assertErr: require.NoError,
		},
		{
			name:             "success - one update, already present in DB",
			ctx:              t.Context(),
			filterExpression: "true",

			simplestreamsGetImageList: image.IncusImages{
				{
					Name: "alpine:edge:amd64:default",
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
					Name: "alpine:edge:amd64:default",
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

			assertErr: require.NoError,
		},
		{
			name:             "success - one update, already present in DB but misses files with valid hash",
			ctx:              t.Context(),
			filterExpression: "true",

			simplestreamsGetImageList: image.IncusImages{
				{
					Name: "alpine:edge:amd64:default",
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
					Name: "alpine:edge:amd64:default",
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

			assertErr: require.NoError,
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

			assertErr: require.NoError,
		},

		// Error cases.
		{
			name:             "error - simplestreams.GetImageList",
			ctx:              t.Context(),
			filterExpression: `true`,

			simplestreamsGetImageListErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:             "error - invalid filter",
			ctx:              t.Context(),
			filterExpression: `10`, // invalid filter, does not evaluate to bool value.

			simplestreamsGetImageList: image.IncusImages{},
			repoGetAllWithFilter:      image.IncusImages{},
			repoGetAllWithFilterErr:   boom.Error,

			assertErr: require.Error,
		},
		{
			name:             "error - repo.DeleteByName",
			ctx:              t.Context(),
			filterExpression: `true`,

			simplestreamsGetImageList: image.IncusImages{},
			repoGetAllWithFilter:      image.IncusImages{},
			repoGetAllWithFilterErr:   boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:             "error - repo.DeleteByName",
			ctx:              t.Context(),
			filterExpression: `true`,

			simplestreamsGetImageList: image.IncusImages{},
			repoGetAllWithFilter: image.IncusImages{
				// Supernumerous image.
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
			repoDeleteByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
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

			assertErr: boom.ErrorIs,
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

			assertErr: boom.ErrorIs,
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
				require.ErrorContains(tt, err, "Not enough space available in files repository")
			},
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

			assertErr: boom.ErrorIs,
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

			assertErr: boom.ErrorIs,
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

			assertErr: boom.ErrorIs,
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

			assertErr: boom.ErrorIs,
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
				require.ErrorContains(tt, err, `Image file sha256 mismatch for image "alpine:edge:amd64:cloud", version "20260101", file "root.tar.xz" from source "linuxcontainers.org": manifest: 0000000000000000000000000000000000000000000000000000000000000000, actual: b5a2c96250612366ea272ffac6d9744aaf4b45aacd96aa7cfcb931ee3b558259`)
			},
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

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &mock.ImageIncusRepoMock{
				GetAllWithFilterFunc: func(ctx context.Context, filter image.IncusImageFilter) (image.IncusImages, error) {
					return tc.repoGetAllWithFilter, tc.repoGetAllWithFilterErr
				},
				DeleteByNameFunc: func(ctx context.Context, name string) error {
					return tc.repoDeleteByNameErr
				},
				UpsertFunc: func(ctx context.Context, incusImage image.IncusImage) error {
					return tc.repoUpsertErr
				},
			}

			filesRepo := &mock.ImageIncusFileRepoMock{
				DeleteVersionFileFunc: func(ctx context.Context, img *image.IncusImage, versionIdentifier, filename string) error {
					return tc.filesRepoDeleteVersionFileErr
				},
				UsageInformationFunc: func(ctx context.Context) (image.UsageInformation, error) {
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
					return tc.simplestreamsGetFileRC, tc.simplestreamsGetFileErr
				},
			}

			imageSvc := image.NewIncusImage(repo, filesRepo, simplestreams)

			// Run test
			err := imageSvc.RefreshFromSource(tc.ctx, image.IncusImageSource{
				Name:             "linuxcontainers.org",
				FilterExpression: tc.filterExpression,
			})

			// Assert
			tc.assertErr(t, err)
		})
	}
}

func usageInfoGiB(totalSpaceGiB int, availableSpaceGiB int) image.UsageInformation {
	const GiB = 1024 * 1024 * 1024
	return image.UsageInformation{
		TotalSpaceBytes:     uint64(totalSpaceGiB) * GiB,
		AvailableSpaceBytes: uint64(availableSpaceGiB) * GiB,
		UsedSpaceBytes:      uint64(totalSpaceGiB-availableSpaceGiB) * GiB,
	}
}
