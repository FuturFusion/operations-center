package multipartstreamer_test

import (
	"bytes"
	"io"
	"mime"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/util/multipartstreamer"
)

func TestNew_NoFiles(t *testing.T) {
	mr := multipartstreamer.New()

	buf := bytes.NewBuffer(nil)

	_, err := io.Copy(buf, mr)
	require.NoError(t, err)

	require.Empty(t, buf.String())
}

func TestNew_WithFilesAndFields(t *testing.T) {
	tmpDir := t.TempDir()

	file1 := filepath.Join(tmpDir, "file1.txt")
	err := os.WriteFile(file1, []byte(`file1 content`), 0o600)
	require.NoError(t, err)

	file2 := filepath.Join(tmpDir, "file2.txt")
	err = os.WriteFile(file2, []byte(`file2 content`), 0o600)
	require.NoError(t, err)

	mr := multipartstreamer.NewWithFields(map[string]string{"field": "field value"}, file1, file2)
	defer func() {
		err = mr.Close()
		require.NoError(t, err)
	}()

	buf := bytes.NewBuffer(nil)

	_, err = io.Copy(buf, mr)
	require.NoError(t, err)

	mt, params, err := mime.ParseMediaType(mr.ContentType())
	require.Equal(t, "multipart/form-data", mt)
	boundary, ok := params["boundary"]
	require.True(t, ok)
	require.NotEmpty(t, boundary)

	require.Contains(t, buf.String(), boundary)
	require.Contains(t, buf.String(), `Content-Disposition: form-data; name="field"`)
	require.Contains(t, buf.String(), `field value`)
	require.Contains(t, buf.String(), `Content-Disposition: form-data; name="file00"; filename="file1.txt"`)
	require.Contains(t, buf.String(), `file1 content`)
	require.Contains(t, buf.String(), `Content-Disposition: form-data; name="file01"; filename="file2.txt"`)
	require.Contains(t, buf.String(), `file2 content`)
}

func TestNew_WithInvalidFile(t *testing.T) {
	var err error

	mr := multipartstreamer.New("this-file-does-not-exist")
	defer func() {
		err = mr.Close()
		require.ErrorContains(t, err, "open this-file-does-not-exist: no such file or directory")
	}()

	buf := bytes.NewBuffer(nil)

	_, err = io.Copy(buf, mr)
	require.NoError(t, err)
}
