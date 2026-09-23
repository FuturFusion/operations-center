package archive_test

import (
	"bytes"
	"io"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/util/archive"
	"github.com/FuturFusion/operations-center/internal/util/testing/boom"
)

func TestDetectType(t *testing.T) {
	squashfs := append([]byte("hsqs"), bytes.Repeat([]byte("s"), 512)...)
	qcow2 := append([]byte{'Q', 'F', 'I', 0xfb}, bytes.Repeat([]byte("q"), 512)...)
	tarXZ := append([]byte{0xfd, '7', 'z', 'X', 'Z', 0x00}, bytes.Repeat([]byte("x"), 512)...)

	tests := []struct {
		name   string
		reader io.Reader

		assertErr     require.ErrorAssertionFunc
		wantExtension string
		wantContent   []byte
	}{
		{
			name:   "squashfs",
			reader: bytes.NewReader(squashfs),

			assertErr:     require.NoError,
			wantExtension: ".squashfs",
			wantContent:   squashfs,
		},
		{
			name:   "qcow2",
			reader: bytes.NewReader(qcow2),

			assertErr:     require.NoError,
			wantExtension: ".qcow2",
			wantContent:   qcow2,
		},
		{
			name:   "tar.xz",
			reader: bytes.NewReader(tarXZ),

			assertErr:     require.NoError,
			wantExtension: ".tar.xz",
			wantContent:   tarXZ,
		},
		{
			name:   "qcow2 - single byte reads",
			reader: iotest.OneByteReader(bytes.NewReader(qcow2)),

			assertErr:     require.NoError,
			wantExtension: ".qcow2",
			wantContent:   qcow2,
		},
		{
			name:   "unrecognized content",
			reader: strings.NewReader("this is not an image"),

			assertErr:     require.NoError,
			wantExtension: "",
			wantContent:   []byte("this is not an image"),
		},
		{
			name:   "content shorter than the header",
			reader: bytes.NewReader([]byte("hsqs")),

			assertErr:     require.NoError,
			wantExtension: ".squashfs",
			wantContent:   []byte("hsqs"),
		},
		{
			name:   "empty content",
			reader: bytes.NewReader(nil),

			assertErr:     require.NoError,
			wantExtension: "",
			wantContent:   []byte{},
		},
		{
			name:   "error - read failed",
			reader: iotest.ErrReader(boom.Error),

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			extension, content, err := archive.DetectType(tc.reader)

			tc.assertErr(t, err)
			if err != nil {
				return
			}

			require.Equal(t, tc.wantExtension, extension)

			body, err := io.ReadAll(content)
			require.NoError(t, err)
			require.Equal(t, tc.wantContent, body)
		})
	}
}
