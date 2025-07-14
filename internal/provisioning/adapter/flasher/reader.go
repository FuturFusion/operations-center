package flasher

import (
	"bytes"
	"errors"
	"io"
)

type parentCloser struct {
	readCloser io.ReadCloser
	parent     io.Closer
}

func newParentCloser(readCloser io.ReadCloser, parent io.Closer) *parentCloser {
	return &parentCloser{
		readCloser: readCloser,
		parent:     parent,
	}
}

func (p *parentCloser) Read(payload []byte) (int, error) {
	return p.readCloser.Read(payload)
}

func (p *parentCloser) Close() error {
	err := p.readCloser.Close()
	parentErr := p.parent.Close()

	return errors.Join(err, parentErr)
}

type injectReader struct {
	srcReader     io.ReadCloser
	payloadReader io.Reader

	remainder        int
	payloadRemainder int
}

func newInjectReader(src io.ReadCloser, injectPos int, payload []byte) *injectReader {
	return &injectReader{
		srcReader:     src,
		payloadReader: bytes.NewBuffer(payload),

		remainder:        injectPos,
		payloadRemainder: len(payload),
	}
}

func (s *injectReader) Read(p []byte) (n int, err error) {
	// Position before tarball
	if s.remainder > 0 {
		chunk := min(s.remainder, len(p))
		n, err := s.srcReader.Read(p[:chunk])
		s.remainder -= n
		return n, err
	}

	if s.payloadRemainder > 0 {
		chunk := min(s.payloadRemainder, len(p))

		// Read from file, to move position while injecting tarball.
		n, err := s.srcReader.Read(p[:chunk])
		if err != nil && !errors.Is(err, io.EOF) {
			return n, err
		}

		// Read from tarball content, overwrite content written from file.
		n, err = s.payloadReader.Read(p[:chunk])
		s.payloadRemainder -= n
		return n, err
	}

	return s.srcReader.Read(p)
}

func (s *injectReader) Close() error {
	return s.srcReader.Close()
}
