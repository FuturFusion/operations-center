package localfs

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/FuturFusion/operations-center/internal/system"
	"github.com/FuturFusion/operations-center/internal/util/file"
)

type localfs struct {
	cacheRoot  *os.Root
	namespaces []string
}

var _ system.CacheRepo = localfs{}

// New returns a cache repo, which removes the contents of the given namespaces
// in cacheRoot.
func New(cacheRoot *os.Root, namespaces ...string) localfs {
	return localfs{
		cacheRoot:  cacheRoot,
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
		return fmt.Errorf("Cleanup of cache directory %q caused errors, operation might be still partly successful: %w", l.cacheRoot.Name(), errors.Join(errs...))
	}

	return nil
}

func (l localfs) cleanupNamespace(ctx context.Context, namespace string) error {
	if !file.IsPathElement(namespace) {
		return fmt.Errorf("Invalid cache namespace %q", namespace)
	}

	entries, err := fs.ReadDir(l.cacheRoot.FS(), namespace)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}

		return fmt.Errorf("Failed to read cache directory %q: %w", namespace, err)
	}

	var errs []error

	for _, entry := range entries {
		err = l.cacheRoot.RemoveAll(filepath.Join(namespace, entry.Name()))
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
