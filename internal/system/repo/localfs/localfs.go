package localfs

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/FuturFusion/operations-center/internal/system"
)

type localfs struct {
	cacheDir   string
	namespaces []string
}

var _ system.CacheRepo = localfs{}

// New returns a cache repo, which removes the contents of the given namespaces
// below cacheDir.
func New(cacheDir string, namespaces ...string) localfs {
	return localfs{
		cacheDir:   cacheDir,
		namespaces: namespaces,
	}
}

func (l localfs) CleanupAll(ctx context.Context) error {
	var errs []error

	for _, namespace := range l.namespaces {
		err := l.cleanupNamespace(ctx, namespace)
		if err != nil {
			errs = append(errs, err)
		}

		if ctx.Err() != nil {
			errs = append(errs, ctx.Err())
			break
		}
	}

	if len(errs) != 0 {
		return fmt.Errorf("Cleanup of cache directory %q caused errors, operation might be still partly successful: %w", l.cacheDir, errors.Join(errs...))
	}

	return nil
}

func (l localfs) cleanupNamespace(ctx context.Context, namespace string) error {
	dir := filepath.Join(l.cacheDir, namespace)

	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}

		return fmt.Errorf("Failed to read cache directory %q: %w", dir, err)
	}

	var errs []error

	for _, entry := range entries {
		err = os.RemoveAll(filepath.Join(dir, entry.Name()))
		if err != nil {
			errs = append(errs, err)
		}

		if ctx.Err() != nil {
			errs = append(errs, ctx.Err())
			break
		}
	}

	return errors.Join(errs...)
}
