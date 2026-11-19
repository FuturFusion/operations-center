// Package xz provides xz compression for the image service. It runs the xz binary.
package xz

import (
	"context"
	"io"

	utilxz "github.com/FuturFusion/operations-center/internal/util/archive/xz"
)

type xz struct{}

// New returns an adapter, that compresses and decompresses with the xz binary.
func New() *xz {
	return &xz{}
}

// NewReader returns a reader, that decompresses the content of r.
func (xz) NewReader(ctx context.Context, r io.Reader) (io.ReadCloser, error) {
	return utilxz.NewReader(ctx, r)
}

// NewWriter returns a writer, that compresses everything written to it into w.
func (xz) NewWriter(ctx context.Context, w io.Writer) (io.WriteCloser, error) {
	return utilxz.NewWriter(ctx, w)
}
