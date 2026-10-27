package flasher

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/util/file"
	"github.com/FuturFusion/operations-center/internal/util/testing/errassert"
	"github.com/FuturFusion/operations-center/internal/util/testing/fsroot"
	"github.com/FuturFusion/operations-center/shared/api"
)

const testCacheID = "abcdef012345"

var testContent = "incus-os-image"

// testFlasher returns a flasher with an enabled cache, which reports plenty of
// free space.
func testFlasher(t *testing.T) *Flasher {
	t.Helper()

	cache := newImageCache(fsroot.Open(t, t.TempDir()))
	cache.usage = func() (file.UsageInformation, error) {
		return file.UsageInformation{
			TotalSpaceBytes:     1 << 60,
			AvailableSpaceBytes: 1 << 60,
		}, nil
	}

	return &Flasher{
		serverURL: "https://operations-center.local:7443",
		cache:     cache,
	}
}

// absNamespaceDir returns the absolute path of the namespace directory.
func absNamespaceDir(c *imageCache, public bool) string {
	return filepath.Join(c.root.Name(), c.namespaceDir(public))
}

// absImageFilename returns the absolute path of the cached image.
func absImageFilename(c *imageCache, public bool, cacheID string, fingerprintID string) string {
	return filepath.Join(c.root.Name(), c.imageFilename(public, cacheID, fingerprintID))
}

func gzipSource(t *testing.T, content string) io.ReadCloser {
	t.Helper()

	var buf bytes.Buffer

	gzipWriter := gzip.NewWriter(&buf)

	_, err := gzipWriter.Write([]byte(content))
	require.NoError(t, err)
	require.NoError(t, gzipWriter.Close())

	return io.NopCloser(bytes.NewReader(buf.Bytes()))
}

func fingerprintID(t *testing.T, flasher *Flasher, fingerprint string) string {
	t.Helper()

	id, err := flasher.SeedImageFingerprintID(context.Background(), fingerprint, uuid.Nil, provisioning.TokenImageSeedConfigs{})
	require.NoError(t, err)

	return id
}

func requireNoLeftovers(t *testing.T, flasher *Flasher, public bool) {
	t.Helper()

	entries, err := os.ReadDir(absNamespaceDir(flasher.cache, public))
	if errors.Is(err, fs.ErrNotExist) {
		return
	}

	require.NoError(t, err)
	require.Empty(t, entries)
}

func generate(t *testing.T, flasher *Flasher, fingerprint string, content string) {
	t.Helper()

	image, _, err := flasher.GenerateSeededImage(context.Background(), testCacheID, 0, fingerprint, uuid.Nil, provisioning.TokenImageSeedConfigs{}, true, gzipSource(t, content))
	require.NoError(t, err)
	require.NoError(t, image.Close())
}

type countingReadCloser struct {
	io.ReadCloser

	reads  atomic.Int64
	closed atomic.Int64
}

func (c *countingReadCloser) Read(p []byte) (int, error) {
	c.reads.Add(1)

	return c.ReadCloser.Read(p)
}

func (c *countingReadCloser) Close() error {
	c.closed.Add(1)

	return c.ReadCloser.Close()
}

func TestFlasher_GenerateSeededImage(t *testing.T) {
	t.Parallel()

	content := testContent

	ctx := context.Background()

	t.Run("generates the image on a cold cache and serves it from the file afterwards", func(t *testing.T) {
		t.Parallel()

		flasher := testFlasher(t)

		source := &countingReadCloser{ReadCloser: gzipSource(t, content)}

		image, info, err := flasher.GenerateSeededImage(ctx, testCacheID, 0, "fingerprint", uuid.Nil, provisioning.TokenImageSeedConfigs{}, true, source)
		require.NoError(t, err)

		body, err := io.ReadAll(image)
		require.NoError(t, err)
		require.NoError(t, image.Close())

		require.Equal(t, content, string(body))
		require.Equal(t, int64(len(content)), info.Size)
		require.False(t, info.ModTime.IsZero())
		require.Equal(t, int64(1), source.closed.Load())

		require.FileExists(t, absImageFilename(flasher.cache, true, testCacheID, fingerprintID(t, flasher, "fingerprint")))

		// A second request is served from the file, the source is not consumed.
		secondSource := &countingReadCloser{ReadCloser: gzipSource(t, "not used")}

		image, secondInfo, err := flasher.GenerateSeededImage(ctx, testCacheID, 0, "fingerprint", uuid.Nil, provisioning.TokenImageSeedConfigs{}, true, secondSource)
		require.NoError(t, err)

		body, err = io.ReadAll(image)
		require.NoError(t, err)
		require.NoError(t, image.Close())

		require.Equal(t, content, string(body))
		require.Equal(t, int64(len(content)), secondInfo.Size)
		require.Equal(t, int64(1), secondSource.closed.Load())

		require.Equal(t, info.ModTime, secondInfo.ModTime)
	})

	t.Run("serves arbitrary byte ranges of the cached image", func(t *testing.T) {
		t.Parallel()

		flasher := testFlasher(t)

		image, info, err := flasher.GenerateSeededImage(ctx, testCacheID, 0, "fingerprint", uuid.Nil, provisioning.TokenImageSeedConfigs{}, true, gzipSource(t, content))
		require.NoError(t, err)

		defer func() { _ = image.Close() }()

		// http.ServeContent probes the size by seeking to the end, then serves
		// the requested range, and a reader that jumps back and forth over the image.
		end, err := image.Seek(0, io.SeekEnd)
		require.NoError(t, err)
		require.Equal(t, info.Size, end)

		for _, offset := range []int64{int64(len(content)) - 1, 0, 5, 2} {
			pos, err := image.Seek(offset, io.SeekStart)
			require.NoError(t, err)
			require.Equal(t, offset, pos)

			body, err := io.ReadAll(image)
			require.NoError(t, err)
			require.Equal(t, content[offset:], string(body))
		}
	})

	t.Run("keeps images of different fingerprints side by side", func(t *testing.T) {
		t.Parallel()

		flasher := testFlasher(t)

		generate(t, flasher, "old", content)
		generate(t, flasher, "new", "regenerated")

		for fingerprint, want := range map[string]string{"old": content, "new": "regenerated"} {
			image, info, err := flasher.OpenSeededImage(ctx, testCacheID, fingerprintID(t, flasher, fingerprint))
			require.NoError(t, err)

			body, err := io.ReadAll(image)
			require.NoError(t, err)
			require.NoError(t, image.Close())

			require.Equal(t, want, string(body), fingerprint)
			require.Equal(t, int64(len(want)), info.Size, fingerprint)
		}
	})

	t.Run("names the image differently, if what is seeded into it changed", func(t *testing.T) {
		t.Parallel()

		flasher := testFlasher(t)

		before := fingerprintID(t, flasher, "fingerprint")

		flasher.UpdateServerURL("https://operations-center.example:7443")

		require.NotEqual(t, before, fingerprintID(t, flasher, "fingerprint"))
	})

	t.Run("generates the image only once for concurrent requests", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			flasher := testFlasher(t)

			const concurrency = 8

			var (
				wg      sync.WaitGroup
				started atomic.Int64
			)

			release := make(chan struct{})

			sources := make([]*blockingReadCloser, concurrency)
			bodies := make([]string, concurrency)
			errs := make([]error, concurrency)

			for i := range concurrency {
				sources[i] = &blockingReadCloser{
					ReadCloser: gzipSource(t, content),
					started:    &started,
					release:    release,
				}

				wg.Add(1)

				go func() {
					defer wg.Done()

					image, _, err := flasher.GenerateSeededImage(ctx, testCacheID, 0, "fingerprint", uuid.Nil, provisioning.TokenImageSeedConfigs{}, true, sources[i])
					if err != nil {
						errs[i] = err

						return
					}

					body, err := io.ReadAll(image)
					errs[i] = errors.Join(err, image.Close())
					bodies[i] = string(body)
				}()
			}

			synctest.Wait()
			require.Equal(t, int64(1), started.Load(), "expect only one request to generate the image")

			close(release)
			wg.Wait()

			for i := range concurrency {
				require.NoError(t, errs[i])
				require.Equal(t, content, bodies[i])
			}

			require.Equal(t, int64(1), started.Load(), "expect the other requests to reuse the generated image")
		})
	})

	t.Run("serves an image, which appeared between the cache miss and the generation being acquired", func(t *testing.T) {
		t.Parallel()

		flasher := testFlasher(t)

		// The hook runs in exactly the window the request below has to survive:
		// it has seen a cache miss already, but has not acquired the generation
		// yet, and in the meantime somebody else generates the very same image.
		var generated bool

		flasher.cache.beforeAcquire = func() {
			if generated {
				return
			}

			generated = true

			generate(t, flasher, "fingerprint", content)
		}

		source := &countingReadCloser{ReadCloser: gzipSource(t, content)}

		image, info, err := flasher.GenerateSeededImage(ctx, testCacheID, 0, "fingerprint", uuid.Nil, provisioning.TokenImageSeedConfigs{}, true, source)
		require.NoError(t, err)

		body, err := io.ReadAll(image)
		require.NoError(t, err)
		require.NoError(t, image.Close())

		require.Equal(t, content, string(body))
		require.Equal(t, int64(len(content)), info.Size)

		// The image is served from the file, the source is not consumed, but
		// still closed.
		require.Equal(t, int64(0), source.reads.Load())
		require.Equal(t, int64(1), source.closed.Load())
	})

	t.Run("refuses to generate the image, if the space does not suffice", func(t *testing.T) {
		t.Parallel()

		flasher := testFlasher(t)
		flasher.cache.usage = func() (file.UsageInformation, error) {
			return file.UsageInformation{
				TotalSpaceBytes:     100,
				AvailableSpaceBytes: 1,
			}, nil
		}

		source := &countingReadCloser{ReadCloser: gzipSource(t, content)}

		_, _, err := flasher.GenerateSeededImage(ctx, testCacheID, 0, "fingerprint", uuid.Nil, provisioning.TokenImageSeedConfigs{}, true, source)
		require.Error(t, err)

		errassert.DomainError(domain.ErrConstraintViolation, api.ErrorReasonInsufficientStorage)(t, err)

		require.Equal(t, int64(1), source.closed.Load())
		require.NoFileExists(t, absImageFilename(flasher.cache, true, testCacheID, fingerprintID(t, flasher, "fingerprint")))

		requireNoLeftovers(t, flasher, true)
	})

	t.Run("leaves no partial file behind, if the context is cancelled", func(t *testing.T) {
		t.Parallel()

		flasher := testFlasher(t)

		cancelledCtx, cancel := context.WithCancel(ctx)
		cancel()

		source := &countingReadCloser{ReadCloser: gzipSource(t, content)}

		_, _, err := flasher.GenerateSeededImage(cancelledCtx, testCacheID, 0, "fingerprint", uuid.Nil, provisioning.TokenImageSeedConfigs{}, true, source)
		require.ErrorIs(t, err, context.Canceled)
		require.Equal(t, int64(1), source.closed.Load())

		requireNoLeftovers(t, flasher, true)
	})

	t.Run("rejects a cache ID, which is not usable as a filename", func(t *testing.T) {
		t.Parallel()

		flasher := testFlasher(t)

		source := &countingReadCloser{ReadCloser: gzipSource(t, content)}

		_, _, err := flasher.GenerateSeededImage(ctx, "../../etc/passwd", 0, "fingerprint", uuid.Nil, provisioning.TokenImageSeedConfigs{}, true, source)
		require.Error(t, err)
		require.Equal(t, int64(1), source.closed.Load())
	})

	t.Run("reports the cache as unavailable, if it is not configured", func(t *testing.T) {
		t.Parallel()

		flasher := &Flasher{serverURL: "https://operations-center.local:7443"}

		source := &countingReadCloser{ReadCloser: gzipSource(t, content)}

		_, _, err := flasher.GenerateSeededImage(ctx, testCacheID, 0, "fingerprint", uuid.Nil, provisioning.TokenImageSeedConfigs{}, true, source)
		require.Error(t, err)
		require.Equal(t, int64(1), source.closed.Load())
	})
}

func TestFlasher_OpenSeededImage(t *testing.T) {
	t.Parallel()

	content := testContent

	ctx := context.Background()

	t.Run("serves an image generated for a public token seed", func(t *testing.T) {
		t.Parallel()

		flasher := testFlasher(t)

		generate(t, flasher, "fingerprint", content)

		image, info, err := flasher.OpenSeededImage(ctx, testCacheID, fingerprintID(t, flasher, "fingerprint"))
		require.NoError(t, err)

		body, err := io.ReadAll(image)
		require.NoError(t, err)
		require.NoError(t, image.Close())

		require.Equal(t, content, string(body))
		require.Equal(t, int64(len(content)), info.Size)
		require.False(t, info.ModTime.IsZero())
	})

	t.Run("does not serve an image generated for a token seed, which is not public", func(t *testing.T) {
		t.Parallel()

		flasher := testFlasher(t)

		image, _, err := flasher.GenerateSeededImage(ctx, testCacheID, 0, "fingerprint", uuid.Nil, provisioning.TokenImageSeedConfigs{}, false, gzipSource(t, content))
		require.NoError(t, err)
		require.NoError(t, image.Close())

		_, _, err = flasher.OpenSeededImage(ctx, testCacheID, fingerprintID(t, flasher, "fingerprint"))
		require.ErrorIs(t, err, domain.ErrNotFound)
	})

	t.Run("reports an image, which has never been generated, as not found", func(t *testing.T) {
		t.Parallel()

		flasher := testFlasher(t)

		_, _, err := flasher.OpenSeededImage(ctx, testCacheID, "0123456789ab")
		require.ErrorIs(t, err, domain.ErrNotFound)
	})

	t.Run("rejects an ID, which is not usable as a filename", func(t *testing.T) {
		t.Parallel()

		flasher := testFlasher(t)

		for _, id := range []string{"../../etc/passwd", "", "short", "way-too-long-to-be-an-id"} {
			_, _, err := flasher.OpenSeededImage(ctx, testCacheID, id)
			require.ErrorIs(t, err, domain.ErrNotFound, id)
		}
	})

	t.Run("waits for an image, which is being generated right now", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			flasher := testFlasher(t)

			var started atomic.Int64

			release := make(chan struct{})

			id := fingerprintID(t, flasher, "fingerprint")

			source := &blockingReadCloser{
				ReadCloser: gzipSource(t, content),
				started:    &started,
				release:    release,
			}

			go func() {
				image, _, err := flasher.GenerateSeededImage(ctx, testCacheID, 0, "fingerprint", uuid.Nil, provisioning.TokenImageSeedConfigs{}, true, source)
				if err == nil {
					_ = image.Close()
				}
			}()

			synctest.Wait()
			require.Equal(t, int64(1), started.Load(), "the generation has started")

			opened := make(chan string, 1)

			go func() {
				image, _, err := flasher.OpenSeededImage(ctx, testCacheID, id)
				if err != nil {
					opened <- err.Error()

					return
				}

				body, err := io.ReadAll(image)
				if err != nil {
					opened <- err.Error()

					return
				}

				_ = image.Close()

				opened <- string(body)
			}()

			synctest.Wait()

			select {
			case got := <-opened:
				t.Fatalf("the image must not be served before it is generated, got %q", got)

			default:
			}

			close(release)

			require.Equal(t, content, <-opened, "the image is served after the generation completed")
		})
	})

	t.Run("reports the cache as unavailable, if it is not configured", func(t *testing.T) {
		t.Parallel()

		flasher := &Flasher{serverURL: "https://operations-center.local:7443"}

		_, _, err := flasher.OpenSeededImage(ctx, testCacheID, "0123456789ab")
		require.Error(t, err)
	})
}

func TestFlasher_PruneCache(t *testing.T) {
	t.Parallel()

	content := testContent

	ctx := context.Background()

	t.Run("keeps a recently accessed image and removes a stale one", func(t *testing.T) {
		t.Parallel()

		flasher := testFlasher(t)

		generate(t, flasher, "fingerprint", content)

		id := fingerprintID(t, flasher, "fingerprint")
		imageFile := absImageFilename(flasher.cache, true, testCacheID, id)

		require.NoError(t, flasher.PruneCache(ctx, time.Hour))
		require.FileExists(t, imageFile)

		flasher.cache.entries[cacheKey(true, testCacheID, id)].lastAccess = time.Now().Add(-2 * time.Hour)

		stale := time.Now().Add(-2 * time.Hour)
		require.NoError(t, os.Chtimes(imageFile, stale, stale))

		require.NoError(t, flasher.PruneCache(ctx, time.Hour))

		requireNoLeftovers(t, flasher, true)
		require.Empty(t, flasher.cache.entries)
	})

	t.Run("prunes the images of a token seed, which is not public, as well", func(t *testing.T) {
		t.Parallel()

		flasher := testFlasher(t)

		image, _, err := flasher.GenerateSeededImage(ctx, testCacheID, 0, "fingerprint", uuid.Nil, provisioning.TokenImageSeedConfigs{}, false, gzipSource(t, content))
		require.NoError(t, err)
		require.NoError(t, image.Close())

		id := fingerprintID(t, flasher, "fingerprint")
		imageFile := absImageFilename(flasher.cache, false, testCacheID, id)
		require.FileExists(t, imageFile)

		flasher.cache.entries[cacheKey(false, testCacheID, id)].lastAccess = time.Now().Add(-2 * time.Hour)

		stale := time.Now().Add(-2 * time.Hour)
		require.NoError(t, os.Chtimes(imageFile, stale, stale))

		require.NoError(t, flasher.PruneCache(ctx, time.Hour))
		require.NoFileExists(t, imageFile)
	})

	t.Run("removes leftovers of an interrupted generation", func(t *testing.T) {
		t.Parallel()

		flasher := testFlasher(t)

		dir := absNamespaceDir(flasher.cache, true)
		require.NoError(t, os.MkdirAll(dir, 0o700))

		partial := filepath.Join(dir, testCacheID+".123456"+cachePartialExt)
		require.NoError(t, os.WriteFile(partial, []byte(content), 0o600))

		unrelated := filepath.Join(dir, "README")
		require.NoError(t, os.WriteFile(unrelated, []byte("keep me"), 0o600))

		require.NoError(t, flasher.PruneCache(ctx, time.Hour))
		require.FileExists(t, partial)

		stale := time.Now().Add(-2 * time.Hour)
		require.NoError(t, os.Chtimes(partial, stale, stale))
		require.NoError(t, os.Chtimes(unrelated, stale, stale))

		require.NoError(t, flasher.PruneCache(ctx, time.Hour))
		require.NoFileExists(t, partial)

		require.FileExists(t, unrelated)
	})

	t.Run("does nothing, if the cache is not configured or empty", func(t *testing.T) {
		t.Parallel()

		require.NoError(t, (&Flasher{}).PruneCache(ctx, time.Hour))
		require.NoError(t, testFlasher(t).PruneCache(ctx, time.Hour))
	})
}

func TestFlasher_RemoveSeededImages(t *testing.T) {
	const (
		cachePrefix      = "abcdef012345"
		cacheID          = cachePrefix + "variant00001"
		otherVariantID   = cachePrefix + "variant00002"
		otherTokenSeedID = "012345abcdef" + "variant00001"
	)

	content := testContent

	ctx := context.Background()

	generateImage := func(t *testing.T, flasher *Flasher, cacheID string, public bool) string {
		t.Helper()

		image, _, err := flasher.GenerateSeededImage(ctx, cacheID, 0, "fingerprint", uuid.Nil, provisioning.TokenImageSeedConfigs{}, public, gzipSource(t, content))
		require.NoError(t, err)
		require.NoError(t, image.Close())

		return absImageFilename(flasher.cache, public, cacheID, fingerprintID(t, flasher, "fingerprint"))
	}

	t.Run("removes all images of the prefix and keeps the others", func(t *testing.T) {
		flasher := testFlasher(t)

		publicImage := generateImage(t, flasher, cacheID, true)
		publicVariantImage := generateImage(t, flasher, otherVariantID, true)
		privateImage := generateImage(t, flasher, cacheID, false)
		otherImage := generateImage(t, flasher, otherTokenSeedID, true)

		err := flasher.RemoveSeededImages(ctx, cachePrefix)
		require.NoError(t, err)

		require.NoFileExists(t, publicImage)
		require.NoFileExists(t, publicVariantImage)
		require.NoFileExists(t, privateImage)
		require.FileExists(t, otherImage, "The image of another token seed is kept.")
		require.Len(t, flasher.cache.entries, 1, "Only the entry of the other token seed is left.")

		_, _, err = flasher.OpenSeededImage(ctx, cacheID, fingerprintID(t, flasher, "fingerprint"))
		require.ErrorIs(t, err, domain.ErrNotFound)
	})

	t.Run("does not store an image being generated", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			flasher := testFlasher(t)

			var started atomic.Int64

			release := make(chan struct{})

			source := &blockingReadCloser{
				ReadCloser: gzipSource(t, content),
				started:    &started,
				release:    release,
			}

			generation := flasher.SeedImageCacheGeneration(ctx, cachePrefix)

			generated := make(chan error, 1)

			go func() {
				image, _, err := flasher.GenerateSeededImage(ctx, cacheID, generation, "fingerprint", uuid.Nil, provisioning.TokenImageSeedConfigs{}, false, source)
				if err == nil {
					err = image.Close()
				}

				generated <- err
			}()

			synctest.Wait()
			require.Equal(t, int64(1), started.Load(), "the generation has started")

			err := flasher.RemoveSeededImages(ctx, cachePrefix)
			require.NoError(t, err)

			close(release)

			require.ErrorIs(t, <-generated, domain.ErrOperationNotPermitted, "The token seed has changed since the generation has been read.")
			requireNoLeftovers(t, flasher, false)
		})
	})

	t.Run("rejects a request, which has read the generation before the removal", func(t *testing.T) {
		flasher := testFlasher(t)

		generation := flasher.SeedImageCacheGeneration(ctx, cachePrefix)

		err := flasher.RemoveSeededImages(ctx, cachePrefix)
		require.NoError(t, err)

		source := &countingReadCloser{ReadCloser: gzipSource(t, content)}

		_, _, err = flasher.GenerateSeededImage(ctx, cacheID, generation, "fingerprint", uuid.Nil, provisioning.TokenImageSeedConfigs{}, true, source)
		require.ErrorIs(t, err, domain.ErrOperationNotPermitted)
		require.Zero(t, source.reads.Load(), "Nothing is generated for a stale request.")
		require.NoFileExists(t, absImageFilename(flasher.cache, true, cacheID, fingerprintID(t, flasher, "fingerprint")))
	})

	t.Run("serves a request, which has read the generation after the removal", func(t *testing.T) {
		flasher := testFlasher(t)

		err := flasher.RemoveSeededImages(ctx, cachePrefix)
		require.NoError(t, err)

		generation := flasher.SeedImageCacheGeneration(ctx, cachePrefix)
		require.NotEqual(t, flasher.SeedImageCacheGeneration(ctx, "012345abcdef"), generation, "The generation of another token seed is not affected.")

		image, _, err := flasher.GenerateSeededImage(ctx, cacheID, generation, "fingerprint", uuid.Nil, provisioning.TokenImageSeedConfigs{}, true, gzipSource(t, content))
		require.NoError(t, err)
		require.NoError(t, image.Close())
		require.FileExists(t, absImageFilename(flasher.cache, true, cacheID, fingerprintID(t, flasher, "fingerprint")))
	})

	t.Run("rejects a prefix, which is not usable as a filename", func(t *testing.T) {
		flasher := testFlasher(t)

		image := generateImage(t, flasher, cacheID, true)

		for _, prefix := range []string{"", "../", "abc/def", "*", "abcdef01234", "abcdef0123456", "abcdef01234/"} {
			err := flasher.RemoveSeededImages(ctx, prefix)
			require.Error(t, err, prefix)
		}

		require.FileExists(t, image)
	})

	t.Run("does nothing, if the cache is not configured or empty", func(t *testing.T) {
		require.NoError(t, (&Flasher{}).RemoveSeededImages(ctx, cachePrefix))
		require.NoError(t, testFlasher(t).RemoveSeededImages(ctx, cachePrefix))
	})
}

func TestImageCache_open(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("reports a cache miss for an image, which is not stored", func(t *testing.T) {
		t.Parallel()

		cache := newImageCache(fsroot.Open(t, t.TempDir()))

		_, _, err := cache.open(true, testCacheID, "0123456789ab")
		require.ErrorIs(t, err, errCacheMiss)
	})

	t.Run("reports the size and the generation time of the stored image", func(t *testing.T) {
		t.Parallel()

		cache := newImageCache(fsroot.Open(t, t.TempDir()))

		before := time.Now().Add(-time.Second)

		require.NoError(t, cache.write(ctx, true, testCacheID, 0, "0123456789ab", bytes.NewReader([]byte("incus-os-image"))))

		image, info, err := cache.open(true, testCacheID, "0123456789ab")
		require.NoError(t, err)
		require.NoError(t, image.Close())

		require.Equal(t, int64(len("incus-os-image")), info.Size)
		require.True(t, info.ModTime.After(before))
	})

	t.Run("keeps the two namespaces apart", func(t *testing.T) {
		t.Parallel()

		cache := newImageCache(fsroot.Open(t, t.TempDir()))

		require.NoError(t, cache.write(ctx, false, testCacheID, 0, "0123456789ab", bytes.NewReader([]byte("private"))))

		_, _, err := cache.open(true, testCacheID, "0123456789ab")
		require.ErrorIs(t, err, errCacheMiss)

		image, _, err := cache.open(false, testCacheID, "0123456789ab")
		require.NoError(t, err)
		require.NoError(t, image.Close())
	})
}

type blockingReadCloser struct {
	io.ReadCloser

	started *atomic.Int64
	release chan struct{}
	once    sync.Once
}

func (b *blockingReadCloser) Read(p []byte) (int, error) {
	b.once.Do(func() {
		b.started.Add(1)
		<-b.release
	})

	return b.ReadCloser.Read(p)
}

func TestImageCache_symlinkOutsideOfRoot(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		run  func(t *testing.T, cache *imageCache) error
	}{
		{
			name: "write",
			run: func(t *testing.T, cache *imageCache) error {
				t.Helper()
				return cache.write(t.Context(), true, testCacheID, cache.generation(testCacheID), "fingerprint", strings.NewReader(testContent))
			},
		},
		{
			name: "open",
			run: func(t *testing.T, cache *imageCache) error {
				t.Helper()
				_, _, err := cache.open(true, testCacheID, "fingerprint")
				return err
			},
		},
		{
			name: "removeByPrefix",
			run: func(t *testing.T, cache *imageCache) error {
				t.Helper()
				return cache.removeByPrefix(testCacheID)
			},
		},
		{
			name: "prune",
			run: func(t *testing.T, cache *imageCache) error {
				t.Helper()
				return cache.prune(t.Context(), 0)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			tmpDir := t.TempDir()
			outsideDir := filepath.Join(tmpDir, "outside")
			require.NoError(t, os.Mkdir(outsideDir, 0o700))
			outsideImage := filepath.Join(outsideDir, testCacheID+"-fingerprint"+cacheImageExt)
			require.NoError(t, os.WriteFile(outsideImage, []byte("outside"), 0o600))

			cacheDir := filepath.Join(tmpDir, "cache")
			require.NoError(t, os.Mkdir(cacheDir, 0o700))
			require.NoError(t, os.Symlink(outsideDir, filepath.Join(cacheDir, cachePublicDir)))

			cache := newImageCache(fsroot.Open(t, cacheDir))

			// Run test
			err := tc.run(t, cache)

			// Assert
			require.ErrorContains(t, err, "path escapes from parent", "the symlink must not be followed out of the root")

			entries, err := os.ReadDir(outsideDir)
			require.NoError(t, err)
			require.Len(t, entries, 1, "files outside of the root must not be created or removed")
			require.Equal(t, filepath.Base(outsideImage), entries[0].Name())
		})
	}
}
