package image

import (
	"archive/tar"
	"bytes"
	"mime/multipart"
	"net/textproto"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/util/archive/xz"
	"github.com/FuturFusion/operations-center/shared/api"
)

func Test_filterImageVersionFilesByFilterExpression(t *testing.T) {
	tests := []struct {
		name             string
		in               IncusImages
		filterExpression string

		assertErr require.ErrorAssertionFunc
		want      IncusImages
	}{
		{
			name: "success - no filter expression",
			in: IncusImages{
				IncusImage{
					Versions: api.IncusImageVersions{
						"1": api.IncusImageVersion{
							Items: map[string]api.IncusImageVersionItem{
								"file": {},
							},
						},
					},
				},
			},
			filterExpression: "", // empty filter expression

			assertErr: require.NoError,
			want:      IncusImages{},
		},
		{
			name:             "success - empty",
			filterExpression: "true",

			assertErr: require.NoError,
		},
		{
			name: "success - without filtering",
			in: IncusImages{
				IncusImage{
					Versions: api.IncusImageVersions{
						"1": api.IncusImageVersion{
							Items: map[string]api.IncusImageVersionItem{
								"file": {},
							},
						},
					},
				},
			},
			filterExpression: "true",

			assertErr: require.NoError,
			want: IncusImages{
				IncusImage{
					Versions: api.IncusImageVersions{
						"1": api.IncusImageVersion{
							Items: map[string]api.IncusImageVersionItem{
								"file": {},
							},
						},
					},
				},
			},
		},
		{
			name: "success - with filtering for architecture, version and file_type",
			in: IncusImages{
				IncusImage{
					Architecture: "amd64",
					Versions: api.IncusImageVersions{
						"1": api.IncusImageVersion{
							Items: map[string]api.IncusImageVersionItem{
								"root.squashfs": {
									FileType: "squashfs",
								},
								// filtered, since file_type != "squashfs"
								"disk.qcow2": {
									FileType: "disk-kvm.img",
								},
							},
						},
						// filtered, since version != "1"
						"2": api.IncusImageVersion{
							Items: map[string]api.IncusImageVersionItem{
								"root.squashfs": {
									FileType: "squashfs",
								},
							},
						},
					},
				},
				// filtered, since architecture != "amd64"
				IncusImage{
					Architecture: "aarch64",
					Versions: api.IncusImageVersions{
						"1": api.IncusImageVersion{
							Items: map[string]api.IncusImageVersionItem{
								"root.squashfs": {
									FileType: "squashfs",
								},
							},
						},
					},
				},
			},
			filterExpression: `architecture == "amd64" and version == "1" and file_type == "squashfs"`,

			assertErr: require.NoError,
			want: IncusImages{
				IncusImage{
					Architecture: "amd64",
					Versions: api.IncusImageVersions{
						"1": api.IncusImageVersion{
							Items: map[string]api.IncusImageVersionItem{
								"root.squashfs": {
									FileType: "squashfs",
								},
							},
						},
					},
				},
			},
		},

		{
			name:             "error - invalid filter expression",
			filterExpression: `"string"`, // invalid, does not return boolean result

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, "expected bool, but got string")
			},
		},
		{
			name: "error - filter expression run",
			in: IncusImages{
				IncusImage{
					Versions: api.IncusImageVersions{
						"1": api.IncusImageVersion{
							Items: map[string]api.IncusImageVersionItem{
								"file": {},
							},
						},
					},
				},
			},
			filterExpression: `fromBase64("~invalid") == ""`, // invalid, returns runtime error during evauluation of the expression.

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, "illegal base64 data")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := filterImageVersionFilesByFilterExpression(tc.in, tc.filterExpression)
			tc.assertErr(t, err)

			require.Equal(t, tc.want, got)
		})
	}
}

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
			name:            "error - not metadata.yaml",
			multipartReader: multipartReaderIncusTarXZ(t, "invalid", "architecture: amd64"),

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

func Test_fixArchitectureMapping(t *testing.T) {
	tests := []struct {
		name         string
		architecture string

		want string
	}{
		{
			name:         "x86_64 - amd64",
			architecture: "x86_64",

			want: "amd64",
		},
		{
			name:         "aarch64 - arm64",
			architecture: "aarch64",

			want: "arm64",
		},
		{
			name:         "other",
			architecture: "other",

			want: "other",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := fixArchitectureMapping(tc.architecture)

			require.Equal(t, tc.want, got)
		})
	}
}
