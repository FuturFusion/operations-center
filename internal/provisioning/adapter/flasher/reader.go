package flasher

import (
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
	srcReader io.ReadCloser
	injectPos int64
	payload   []byte

	pos int64
}

func newInjectReader(src io.ReadCloser, injectPos int64, payload []byte) *injectReader {
	return &injectReader{
		srcReader: src,
		injectPos: injectPos,
		payload:   payload,
	}
}

func (s *injectReader) Read(p []byte) (n int, err error) {
	n, err = s.srcReader.Read(p)
	overlay(p[:n], s.pos, s.injectPos, s.payload)
	s.pos += int64(n)

	return n, err
}

func (s *injectReader) Close() error {
	return s.srcReader.Close()
}

func overlay(buf []byte, bufPos int64, injectPos int64, payload []byte) {
	// Intersect [bufPos, bufPos+len(buf)) with [injectPos, injectPos+len(payload)).
	start := max(bufPos, injectPos)
	end := min(bufPos+int64(len(buf)), injectPos+int64(len(payload)))

	if start >= end {
		return
	}

	copy(buf[start-bufPos:end-bufPos], payload[start-injectPos:end-injectPos])
}
