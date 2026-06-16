package xz

import (
	"context"
	"io"

	"github.com/FuturFusion/operations-center/internal/util/archive"
)

var unpackerXZ = archive.Unpacker{"xz", "-d"}

var packerXZ = archive.Packer{"xz"}

type Reader struct {
	r          io.Reader
	cancelFunc func()
}

func NewReader(ctx context.Context, r io.Reader) (*Reader, error) {
	r, cancelFunc, err := archive.Reader(ctx, r, unpackerXZ)
	if err != nil {
		return nil, err
	}

	return &Reader{
		r:          r,
		cancelFunc: cancelFunc,
	}, nil
}

func (r *Reader) Read(p []byte) (n int, err error) {
	return r.r.Read(p)
}

func (r *Reader) Close() error {
	if r.cancelFunc != nil {
		r.cancelFunc()
	}

	return nil
}

type Writer struct {
	w io.WriteCloser
}

func NewWriter(ctx context.Context, w io.Writer) (*Writer, error) {
	wc, err := archive.Writer(ctx, w, packerXZ)

	return &Writer{
		w: wc,
	}, err
}

func (w *Writer) Write(p []byte) (int, error) {
	return w.w.Write(p)
}

func (w *Writer) Close() error {
	return w.w.Close()
}
