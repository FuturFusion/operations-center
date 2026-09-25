package system

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestVerifyArchive(t *testing.T) {
	buf := &bytes.Buffer{}
	gzw := gzip.NewWriter(buf)
	tw := tar.NewWriter(gzw)

	content := bytes.Repeat([]byte("content"), 1024)
	err := tw.WriteHeader(&tar.Header{Name: "file", Mode: 0o600, Size: int64(len(content))})
	require.NoError(t, err)

	_, err = tw.Write(content)
	require.NoError(t, err)
	require.NoError(t, tw.Close())
	require.NoError(t, gzw.Close())

	archive := buf.Bytes()

	tests := []struct {
		name    string
		archive []byte

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:    "success",
			archive: archive,

			assertErr: require.NoError,
		},
		{
			name:    "error - truncated",
			archive: archive[:len(archive)/2],

			assertErr: require.Error,
		},
		{
			name:    "error - trailing data",
			archive: append(bytes.Clone(archive), []byte(`{"error":"boom"}`)...),

			assertErr: require.Error,
		},
		{
			name:    "error - empty",
			archive: nil,

			assertErr: require.Error,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := verifyArchive(bytes.NewReader(tc.archive))
			tc.assertErr(t, err)
		})
	}
}
