package provisioning

import (
	"bytes"
	"io"
	"testing"
)

// Based on test from https://pkg.go.dev/io#TeeReader
func Test_teeReadCloserer(t *testing.T) {
	src := []byte("hello, world")
	dst := make([]byte, len(src))
	rb := io.NopCloser(bytes.NewBuffer(src))
	wb := new(bytes.Buffer)
	r := newTeeReadCloser(rb, wb)
	defer func() { _ = r.Close() }()

	n, err := io.ReadFull(r, dst)
	if err != nil || n != len(src) {
		t.Fatalf("ReadFull(r, dst) = %d, %v; want %d, nil", n, err, len(src))
	}

	if !bytes.Equal(dst, src) {
		t.Errorf("bytes read = %q want %q", dst, src)
	}

	if !bytes.Equal(wb.Bytes(), src) {
		t.Errorf("bytes written = %q want %q", wb.Bytes(), src)
	}

	n, err = r.Read(dst)
	if n != 0 || err != io.EOF {
		t.Errorf("r.Read at EOF = %d, %v want 0, EOF", n, err)
	}

	rb = io.NopCloser(bytes.NewBuffer(src))
	pr, pw := io.Pipe()
	_ = pr.Close()
	r = newTeeReadCloser(rb, pw)
	defer func() { _ = r.Close() }()

	n, err = io.ReadFull(r, dst)
	if n != 0 || err != io.ErrClosedPipe {
		t.Errorf("closed tee: ReadFull(r, dst) = %d, %v; want 0, EPIPE", n, err)
	}
}
