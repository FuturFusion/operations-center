package localfs

import (
	"context"
	"encoding/binary"
	"encoding/gob"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/timpalpant/gzran"

	"github.com/FuturFusion/operations-center/internal/util/logger"
)

const (
	gzIndexExt = ".gzidx" // file extension for index sidecar.
)

// gzIndexInterval is the amount of uncompressed data between two points of the
// gzran index.
var gzIndexInterval int64 = 16 * 1024 * 1024

type gzIndex struct {
	Size  int64
	Index gzran.Index
}

func isGzIndexFilename(filename string) bool {
	return strings.Contains(filepath.Base(filename), gzIndexExt)
}

func loadGzIndex(ctx context.Context, compressedFilename string) gzIndex {
	f, err := os.Open(compressedFilename + gzIndexExt)
	if err != nil {
		return gzIndex{}
	}

	defer func() { _ = f.Close() }()

	var idx gzIndex

	err = gob.NewDecoder(f).Decode(&idx)
	if err != nil {
		slog.WarnContext(ctx, "Discarding unreadable gzip index", slog.String("filename", f.Name()), logger.Err(err))
		return gzIndex{}
	}

	return idx
}

func saveGzIndex(compressedFilename string, idx gzIndex) error {
	filename := compressedFilename + gzIndexExt

	// A unique temporary file keeps concurrent writers from clobbering each
	// other's partial state; the rename then atomically picks a winner.
	tmp, err := os.CreateTemp(filepath.Dir(filename), filepath.Base(filename)+".*.partial")
	if err != nil {
		return err
	}

	err = gob.NewEncoder(tmp).Encode(idx)
	err = errors.Join(err, tmp.Close())
	if err != nil {
		return errors.Join(err, os.Remove(tmp.Name()))
	}

	err = os.Rename(tmp.Name(), filename)
	if err != nil {
		return errors.Join(err, os.Remove(tmp.Name()))
	}

	return nil
}

// uncompressedSize determines the size of the uncompressed contents of a gzip
// file.
//
// The size is taken from the ISIZE field of the gzip trailer.
// ISIZE holds the uncompressed size modulo 2^32, so it is only trustworthy as
// long as the uncompressed contents are smaller than 4 GiB.
// Anything at or above that limit falls back to counting the bytes of a full
// decompression pass.
func uncompressedSize(f *os.File, compressedSize int64) (int64, error) {
	if compressedSize < 4<<30 {
		var trailer [4]byte

		_, err := f.ReadAt(trailer[:], compressedSize-int64(len(trailer)))
		if err != nil {
			return 0, fmt.Errorf("Failed to read gzip trailer: %w", err)
		}

		return int64(binary.LittleEndian.Uint32(trailer[:])), nil
	}

	gzr, err := gzran.NewReader(f)
	if err != nil {
		return 0, fmt.Errorf("Failed to initialize gzip reader: %w", err)
	}

	size, err := io.Copy(io.Discard, gzr)
	if err != nil {
		return 0, fmt.Errorf("Failed to determine uncompressed size: %w", err)
	}

	_, err = f.Seek(0, io.SeekStart)
	if err != nil {
		return 0, err
	}

	return size, nil
}

type seekableFile struct {
	*gzran.Reader

	ctx context.Context //nolint:containedctx

	file     *os.File
	filename string
	size     int64

	indexPoints int
}

func newSeekableFile(ctx context.Context, f *os.File, filename string, size int64, cached gzIndex) (*seekableFile, error) {
	gzr, err := gzran.NewReaderInterval(f, gzIndexInterval)
	if err != nil {
		return nil, fmt.Errorf("Failed to initialize gzip reader: %w", err)
	}

	// The reader appends to the index as it reads, so it must not share the
	// backing array with the cached index handed to other readers as well.
	if len(cached.Index) > len(gzr.Index) {
		gzr.Index = slices.Clone(cached.Index)
	}

	return &seekableFile{
		Reader:      gzr,
		ctx:         ctx,
		file:        f,
		filename:    filename,
		size:        size,
		indexPoints: len(gzr.Index),
	}, nil
}

func (s *seekableFile) Seek(offset int64, whence int) (int64, error) {
	if whence == io.SeekEnd {
		return s.Reader.Seek(s.size+offset, io.SeekStart)
	}

	return s.Reader.Seek(offset, whence)
}

func (s *seekableFile) Close() error {
	err := errors.Join(s.Reader.Close(), s.file.Close())

	// Persist the index if this reader extended it. Failing to do so only costs
	// the next reader some decompression, so it must not fail the read.
	if len(s.Index) > s.indexPoints {
		saveErr := saveGzIndex(s.filename, gzIndex{
			Size:  s.size,
			Index: s.Index,
		})
		if saveErr != nil {
			slog.WarnContext(s.ctx, "Failed to save gzip index", slog.String("filename", s.filename), logger.Err(saveErr))
		}
	}

	return err
}
