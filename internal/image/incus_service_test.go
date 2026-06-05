package image_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime/multipart"
	"net/textproto"
	"testing"
	"testing/iotest"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/image"
	"github.com/FuturFusion/operations-center/internal/image/repo/mock"
	"github.com/FuturFusion/operations-center/internal/util/testing/boom"
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
		nameArg            string
		versionArg         string
		multipartReaderArg *multipart.Reader
		repoGetByName      []queue.Item[*image.IncusImage]
		repoCreateErr      error
		filesRepoPut       []queue.Item[fileRepoPutValue]
		filesRepoGet       []queue.Item[fileRepoGetValue]
		repoUpdateErr      error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:               "success",
			nameArg:            "almalinux:10:amd64:cloud",
			versionArg:         "20260515",
			multipartReaderArg: validMultipartReader(t),
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

			assertErr: require.NoError,
		},
		{
			name:               "success - new incus image",
			nameArg:            "almalinux:10:amd64:cloud",
			versionArg:         "20260515",
			multipartReaderArg: validMultipartReader(t),
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

			assertErr: require.NoError,
		},
		{
			name:    "error - invalid incus image name",
			nameArg: "invalid", // invalid

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
				require.ErrorContains(tt, err, `Invalid incus image name, expect name in the format "os:release:architecture:variant"`)
			},
		},
		{
			name:       "error - version identifier empty",
			nameArg:    "almalinux:10:amd64:cloud",
			versionArg: "", // empty

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
				require.ErrorContains(tt, err, `Incus image version can not be empty`)
			},
		},
		{
			name:       "error - repo.GetByName",
			nameArg:    "almalinux:10:amd64:cloud",
			versionArg: "20260515",
			repoGetByName: []queue.Item[*image.IncusImage]{
				// lookup
				{
					Err: boom.Error,
				},
			},

			assertErr: boom.ErrorIs,
		},
		{
			name:       "error - new incus image - repo.Create",
			nameArg:    "almalinux:10:amd64:cloud",
			versionArg: "20260515",
			repoGetByName: []queue.Item[*image.IncusImage]{
				// lookup
				{
					Err: domain.ErrNotFound,
				},
			},
			repoCreateErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		// {
		// 	name:       "error - new incus image - validate",
		// 	nameArg:    "almalinux:10:amd64:",
		// 	versionArg: "20260515",
		// 	repoGetByName: []queue.Item[*image.IncusImage]{
		// 		// lookup
		// 		{
		// 			Err: domain.ErrNotFound,
		// 		},
		// 	},

		// 	assertErr: func(tt require.TestingT, err error, a ...any) {
		// 		var verr domain.ErrValidation
		// 		require.ErrorAs(tt, err, &verr)
		// 	},
		// },
		{
			name:       "error - version already exists",
			nameArg:    "almalinux:10:amd64:cloud",
			versionArg: "20260515",
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
			name:               "error - invalid multipart reader",
			nameArg:            "almalinux:10:amd64:cloud",
			versionArg:         "20260515",
			multipartReaderArg: multipart.NewReader(bytes.NewBufferString(`invalid`), ``),
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

			assertErr: require.Error,
		},
		{
			name:               "error - filesRepo.Put",
			nameArg:            "almalinux:10:amd64:cloud",
			versionArg:         "20260515",
			multipartReaderArg: validMultipartReader(t),
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
			nameArg:            "almalinux:10:amd64:cloud",
			versionArg:         "20260515",
			multipartReaderArg: validMultipartReader(t),
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
			nameArg:            "almalinux:10:amd64:cloud",
			versionArg:         "20260515",
			multipartReaderArg: validMultipartReader(t),
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
			name:               "error - multipart reader without metadata file",
			nameArg:            "almalinux:10:amd64:cloud",
			versionArg:         "20260515",
			multipartReaderArg: multipartReaderWithoutMetadataFile(t),
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

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrOperationNotPermitted)
				require.ErrorContains(tt, err, `Incus image version does not have metadata file "incus.tar.xz"`)
			},
		},
		{
			name:               "error - filesRepo.Get metadata for checksum",
			nameArg:            "almalinux:10:amd64:cloud",
			versionArg:         "20260515",
			multipartReaderArg: validMultipartReader(t),
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
			},
			filesRepoGet: []queue.Item[fileRepoGetValue]{
				// incus.tar.xz for root.tar.xz
				{
					Err: boom.Error,
				},
			},

			assertErr: boom.ErrorIs,
		},
		{
			name:               "error - read metadata for checksum",
			nameArg:            "almalinux:10:amd64:cloud",
			versionArg:         "20260515",
			multipartReaderArg: validMultipartReader(t),
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

			assertErr: boom.ErrorIs,
		},
		{
			name:               "error - filesRepo.Get image for checksum",
			nameArg:            "almalinux:10:amd64:cloud",
			versionArg:         "20260515",
			multipartReaderArg: validMultipartReader(t),
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

			assertErr: boom.ErrorIs,
		},
		{
			name:               "error - read image for checksum",
			nameArg:            "almalinux:10:amd64:cloud",
			versionArg:         "20260515",
			multipartReaderArg: validMultipartReader(t),
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

			assertErr: boom.ErrorIs,
		},
		{
			name:               "error - repo.GetByName in transaction for update",
			nameArg:            "almalinux:10:amd64:cloud",
			versionArg:         "20260515",
			multipartReaderArg: validMultipartReader(t),
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

			assertErr: boom.ErrorIs,
		},
		{
			name:               "error - repo.Update",
			nameArg:            "almalinux:10:amd64:cloud",
			versionArg:         "20260515",
			multipartReaderArg: validMultipartReader(t),
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

			imageSvc := image.New(repo, filesRepo)

			// Run test
			err := imageSvc.AddVersion(t.Context(), tc.nameArg, tc.versionArg, tc.multipartReaderArg)

			// Assert
			tc.assertErr(t, err)
			require.Empty(t, tc.repoGetByName)
			require.Empty(t, tc.filesRepoPut)
			require.Empty(t, tc.filesRepoGet)
		})
	}
}

func validMultipartReader(t *testing.T) *multipart.Reader {
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

	_, err = io.WriteString(part, "incus tar xz")
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

			imageSvc := image.New(repo, nil)

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

			imageSvc := image.New(repo, nil)

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

			imageSvc := image.New(repo, nil)

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

			imageSvc := image.New(repo, filesRepo)

			// Run test
			err := imageSvc.DeleteByName(t.Context(), tc.argName)

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

			imageSvc := image.New(repo, filesRepo)

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

			imageSvc := image.New(repo, filesRepo)

			// Run test
			rc, size, err := imageSvc.GetVersionFileByName(t.Context(), tc.argName, tc.argVersion, tc.argFilename)

			// Assert
			tc.assertErr(t, err)
			tc.assert(t, rc, size)
		})
	}
}

func TestServerService_Update(t *testing.T) {
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

			imageSvc := image.New(repo, nil)

			// Run test
			err := imageSvc.Update(t.Context(), tc.incusImage)

			// Assert
			tc.assertErr(t, err)
		})
	}
}
