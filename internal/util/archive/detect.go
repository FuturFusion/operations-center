package archive

import (
	"bytes"
	"errors"
	"io"

	incusArchive "github.com/lxc/incus/v7/shared/archive"
)

// detectHeaderSize is the number of bytes, which are inspected in order to
// determine the type of the content.
const detectHeaderSize = 263

// DetectType returns the file name extension matching the magic bytes at the
// beginning of r, e.g. ".squashfs", or an empty string, if the content is not
// recognized. The returned reader replays the bytes consumed for the detection,
// so it yields the complete content of r.
func DetectType(r io.Reader) (string, io.Reader, error) {
	header := make([]byte, detectHeaderSize)

	// incusArchive.DetectCompressionFile performs a single read, which is not
	// guaranteed to fill the buffer, if r is a stream. Content shorter than the
	// header is not an error, the detection works on what is available.
	n, err := io.ReadFull(r, header)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return "", nil, err
	}

	header = header[:n]
	content := io.MultiReader(bytes.NewReader(header), r)

	_, extension, _, err := incusArchive.DetectCompressionFile(bytes.NewReader(header))
	if err != nil {
		return "", content, nil
	}

	return extension, content, nil
}
