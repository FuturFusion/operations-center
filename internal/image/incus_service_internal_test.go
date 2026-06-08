package image

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"mime/multipart"
	"net/textproto"
	"testing"
	"testing/iotest"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/util/archive/xz"
	"github.com/FuturFusion/operations-center/internal/util/testing/boom"
)

func Test_metadataFromRequestJSON(t *testing.T) {
	tests := []struct {
		name            string
		multipartReader *multipart.Reader

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:            "success",
			multipartReader: multipartReaderRequestJSON(t, "{}"),

			assertErr: require.NoError,
		},
		{
			name:            "error - invalid JSON",
			multipartReader: multipartReaderRequestJSON(t, "{"),

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, `Failed to decode metadata from "request_json"`)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			part, err := tc.multipartReader.NextPart()
			require.NoError(t, err)

			metadata, incusTarXZ, err := metadataFromRequestJSON(t.Context(), part)

			tc.assertErr(t, err)

			_ = metadata
			_ = incusTarXZ
		})
	}
}

func multipartReaderRequestJSON(t *testing.T, jsonBody string) *multipart.Reader {
	t.Helper()

	var body bytes.Buffer

	writer := multipart.NewWriter(&body)

	// incus.tar.xz
	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition",
		`form-data; name="request_json"`)
	header.Set("Content-Type", "application/json")

	part, err := writer.CreatePart(header)
	require.NoError(t, err)

	_, err = part.Write([]byte(jsonBody))
	require.NoError(t, err)

	err = writer.Close()
	require.NoError(t, err)

	return multipart.NewReader(&body, writer.Boundary())
}

func Test_metadataFromIncusTarXZ(t *testing.T) {
	tests := []struct {
		name            string
		multipartReader *multipart.Reader

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:            "success",
			multipartReader: multipartReaderIncusTarXZ(t, "metadata.yaml", "architecture: amd64"),

			assertErr: require.NoError,
		},
		{
			name: "success - virtual-machine",
			rc:   generateTarGz(t, "rootfs.img"),

			assertErr:     require.NoError,
			wantImageType: "virtual-machine",
		},
		{
			name:           "error - fileRepo.Get",
			fileRepoGetErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - read error",
			rc:   io.NopCloser(iotest.ErrReader(boom.Error)),

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - read error",
			rc: func() io.ReadCloser {
				var buf bytes.Buffer

				gzw := gzip.NewWriter(&buf)
				defer gzw.Close()

				_, err := gzw.Write([]byte("invalid tar"))
				require.NoError(t, err)

				return io.NopCloser(&buf)
			}(),

			assertErr: require.Error,
		},
		{
			name: "error - neither container nor virtual-machine",
			rc:   generateTarGz(t, "foobar"),

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, "Failed to find metadata.yaml in incus.tar.xz")
			},
		},
		{
			name:            "error - invalid XZ compression",
			multipartReader: multipartReaderNotXZ(t),

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, `Failed to read "incus.tar.xz"`)
			},
		},
		{
			name:            "error - invalid metadata body",
			multipartReader: multipartReaderIncusTarXZ(t, "metadata.yaml", "{"),

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, `Failed to decode "metadata.yaml"`)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			part, err := tc.multipartReader.NextPart()
			require.NoError(t, err)

			metadata, incusTarXZ, err := metadataFromIncusTarXZ(t.Context(), part)

			tc.assertErr(t, err)

			_ = metadata
			_ = incusTarXZ
		})
	}
}

func multipartReaderIncusTarXZ(t *testing.T, metadataFilename string, metadataBody string) *multipart.Reader {
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

	buf := bytes.NewBuffer(nil)
	xzw, err := xz.NewWriter(t.Context(), buf)
	require.NoError(t, err)
	tw := tar.NewWriter(xzw)
	err = tw.WriteHeader(&tar.Header{
		Name: metadataFilename,
		Size: int64(len(metadataBody)),
		Mode: 0o600,
	})
	require.NoError(t, err)
	_, err = tw.Write([]byte(metadataBody))
	require.NoError(t, err)
	err = tw.Close()
	require.NoError(t, err)
	err = xzw.Close()
	require.NoError(t, err)

	_, err = part.Write(buf.Bytes())
	require.NoError(t, err)

	err = writer.Close()
	require.NoError(t, err)

	return multipart.NewReader(&body, writer.Boundary())
}

func multipartReaderNotXZ(t *testing.T) *multipart.Reader {
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

	_, err = part.Write([]byte("invalid")) // invalid
	require.NoError(t, err)

	err = writer.Close()
	require.NoError(t, err)

	return multipart.NewReader(&body, writer.Boundary())
}

func generateTarGz(t *testing.T, filename string) io.ReadCloser {
	t.Helper()

	var buf bytes.Buffer

	gzw := gzip.NewWriter(&buf)
	defer gzw.Close()

	tw := tar.NewWriter(gzw)

	content := []byte(filename)

	hdr := &tar.Header{
		Name: filename,
		Mode: 0o600,
		Size: int64(len(content)),
	}

	err := tw.WriteHeader(hdr)
	require.NoError(t, err)

	_, err = tw.Write(content)
	require.NoError(t, err)

	err = tw.Close()
	require.NoError(t, err)

	return io.NopCloser(&buf)
}

type fileRepoMock struct {
	rc  io.ReadCloser
	err error
	t   *testing.T
}

func (f fileRepoMock) Get(ctx context.Context, img *IncusImage, versionIdentifier string, filename string) (_ io.ReadCloser, size int64, _ error) {
	return f.rc, 0, f.err
}

func (fileRepoMock) Exists(ctx context.Context, img *IncusImage, versionIdentifier string, filename string) (bool, error) {
	panic("not implemented")
}

func (fileRepoMock) Put(ctx context.Context, img *IncusImage, versionIdentifier string, filename string, content io.ReadCloser) (_ CommitFunc, _ CancelFunc, size int64, _ error) {
	panic("not implemented")
}

func (fileRepoMock) Delete(ctx context.Context, img *IncusImage) error {
	panic("not implemented")
}

func (fileRepoMock) DeleteVersion(ctx context.Context, img *IncusImage, versionIdentifier string) error {
	panic("not implemented")
}

func (fileRepoMock) DeleteVersionFile(ctx context.Context, img *IncusImage, versionIdentifier string, filename string) error {
	panic("not implemented")
}

func (fileRepoMock) UsageInformation(ctx context.Context) (UsageInformation, error) {
	panic("not implemented")
}
