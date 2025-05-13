package filecache

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/FuturFusion/operations-center/internal/logger"
	"github.com/FuturFusion/operations-center/internal/provisioning"
)

type filecache struct {
	next     provisioning.UpdateSourcePort
	cacheDir string
}

var _ provisioning.UpdateSourcePort = &filecache{}

func New(next provisioning.UpdateSourcePort, cacheDir string) *filecache {
	return &filecache{
		next:     next,
		cacheDir: cacheDir,
	}
}

func (f filecache) GetLatest(ctx context.Context, limit int) (provisioning.Updates, error) {
	return f.next.GetLatest(ctx, limit)
}

func (f filecache) GetUpdateAllFiles(ctx context.Context, update provisioning.Update) (provisioning.UpdateFiles, error) {
	return f.next.GetUpdateAllFiles(ctx, update)
}

func (f filecache) GetUpdateFileByFilename(ctx context.Context, update provisioning.Update, filename string) (io.ReadCloser, int, error) {
	cacheFilename := filepath.Join(f.cacheDir, update.UUID.String(), filename)

	fi, err := os.Stat(cacheFilename)
	if err == nil {
		f, err := os.Open(cacheFilename)
		if err != nil {
			return nil, 0, err
		}

		return f, int(fi.Size()), nil
	}

	r, size, err := f.next.GetUpdateFileByFilename(ctx, update, filename)
	if err != nil {
		return nil, 0, err
	}

	cr, err := newCachingReader(ctx, r, cacheFilename)
	if err != nil {
		return nil, 0, err
	}

	return cr, size, nil
}

func (f filecache) ForgetUpdate(ctx context.Context, update provisioning.Update) error {
	cacheDir := filepath.Join(f.cacheDir, update.UUID.String())
	err := os.RemoveAll(cacheDir)
	if err != nil {
		return err
	}

	return f.next.ForgetUpdate(ctx, update)
}

type cachingReader struct {
	upstream io.ReadCloser

	stream io.ReadCloser
}

func newCachingReader(ctx context.Context, upstream io.ReadCloser, cacheFilename string) (*cachingReader, error) {
	err := os.MkdirAll(filepath.Dir(cacheFilename), 0o700)
	if err != nil {
		return nil, err
	}

	cacheFile, err := os.OpenFile(cacheFilename, os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}

	pr, pw := io.Pipe()
	w := io.MultiWriter(pw, cacheFile)

	go func() {
		defer func() {
			err = pw.Close()
			if err != nil {
				slog.WarnContext(ctx, "Failed to close pipe writer", logger.Err(err))
			}

			err = cacheFile.Close()
			if err != nil {
				slog.WarnContext(ctx, "Failed to close cache file", logger.Err(err))
			}
		}()

		_, err := io.Copy(w, upstream)
		if err != nil {
			slog.ErrorContext(ctx, "Failed to copy content to filecache while updating file by filename", logger.Err(err), slog.String("file", cacheFilename))

			// Close potentially incomplete file before removing.
			err = cacheFile.Close()
			if err != nil {
				slog.WarnContext(ctx, "Failed to close cache file", logger.Err(err))
			}

			// Cleanup potentially incomplete file form cache after error.
			err = os.Remove(cacheFilename)
			if err != nil {
				slog.ErrorContext(ctx, "Failed to remove incomplete file from filecache", logger.Err(err), slog.String("file", cacheFilename))
			}
		}
	}()

	return &cachingReader{
		upstream: upstream,
		stream:   pr,
	}, nil
}

func (c cachingReader) Close() error {
	upstreamErr := c.upstream.Close()
	streamErr := c.stream.Close()

	return errors.Join(upstreamErr, streamErr)
}

func (c cachingReader) Read(p []byte) (n int, err error) {
	return c.stream.Read(p)
}
