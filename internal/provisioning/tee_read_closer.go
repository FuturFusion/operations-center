package provisioning

import "io"

func newTeeReadCloser(r io.ReadCloser, w io.Writer) io.ReadCloser {
	return &teeReadCloser{r, w}
}

// teeReadCloser based on https://pkg.go.dev/io#TeeReader, implements
// additionally the io.Closer interface (https://pkg.go.dev/io#Closer).
type teeReadCloser struct {
	r io.ReadCloser
	w io.Writer
}

func (t *teeReadCloser) Read(p []byte) (int, error) {
	n, err := t.r.Read(p)
	if n > 0 {
		n, err := t.w.Write(p[:n])
		if err != nil {
			return n, err
		}
	}

	return n, err
}

func (t *teeReadCloser) Close() error {
	return t.r.Close()
}
