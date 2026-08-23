package flasher

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/shared/api"
)

const testCacheID = "abcdef012345"

// testFlasher returns a flasher with an enabled cache, which reports plenty of
// free space.
func testFlasher(t *testing.T) *Flasher {
	t.Helper()

	cache := newImageCache(t.TempDir())
	cache.usage = func() (provisioning.UsageInformation, error) {
		return provisioning.UsageInformation{
			TotalSpaceBytes:     1 << 60,
			AvailableSpaceBytes: 1 << 60,
		}, nil
	}

	return &Flasher{
		serverURL: "https://operations-center.local:7443",
		cache:     cache,
	}
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

	entries, err := os.ReadDir(flasher.cache.namespaceDir(public))
	if errors.Is(err, fs.ErrNotExist) {
		return
	}

	require.NoError(t, err)
	require.Empty(t, entries)
}

func generate(t *testing.T, flasher *Flasher, fingerprint string, content string) {
	t.Helper()

	image, _, _, err := flasher.GenerateSeededImage(context.Background(), testCacheID, fingerprint, uuid.Nil, provisioning.TokenImageSeedConfigs{}, true, gzipSource(t, content))
	require.NoError(t, err)
	require.NoError(t, image.Close())
}

type countingReadCloser struct {
	io.ReadCloser

	closed atomic.Int64
}

func (c *countingReadCloser) Close() error {
	c.closed.Add(1)

	return c.ReadCloser.Close()
}

func TestFlasher_GenerateSeededImage(t *testing.T) {
	const content = "incus-os-image"

	ctx := context.Background()

	t.Run("generates the image on a cold cache and serves it from the file afterwards", func(t *testing.T) {
		flasher := testFlasher(t)

		source := &countingReadCloser{ReadCloser: gzipSource(t, content)}

		image, size, modTime, err := flasher.GenerateSeededImage(ctx, testCacheID, "fingerprint", uuid.Nil, provisioning.TokenImageSeedConfigs{}, true, source)
		require.NoError(t, err)

		body, err := io.ReadAll(image)
		require.NoError(t, err)
		require.NoError(t, image.Close())

		require.Equal(t, content, string(body))
		require.Equal(t, int64(len(content)), size)
		require.False(t, modTime.IsZero())
		require.Equal(t, int64(1), source.closed.Load())

		require.FileExists(t, flasher.cache.imageFilename(true, testCacheID, fingerprintID(t, flasher, "fingerprint")))

		// A second request is served from the file, the source is not consumed.
		secondSource := &countingReadCloser{ReadCloser: gzipSource(t, "not used")}

		image, size, secondModTime, err := flasher.GenerateSeededImage(ctx, testCacheID, "fingerprint", uuid.Nil, provisioning.TokenImageSeedConfigs{}, true, secondSource)
		require.NoError(t, err)

		body, err = io.ReadAll(image)
		require.NoError(t, err)
		require.NoError(t, image.Close())

		require.Equal(t, content, string(body))
		require.Equal(t, int64(len(content)), size)
		require.Equal(t, int64(1), secondSource.closed.Load())

		require.Equal(t, modTime, secondModTime)
	})

	t.Run("serves arbitrary byte ranges of the cached image", func(t *testing.T) {
		flasher := testFlasher(t)

		image, size, _, err := flasher.GenerateSeededImage(ctx, testCacheID, "fingerprint", uuid.Nil, provisioning.TokenImageSeedConfigs{}, true, gzipSource(t, content))
		require.NoError(t, err)

		defer func() { _ = image.Close() }()

		// http.ServeContent probes the size by seeking to the end, then serves
		// the requested range, and a reader that jumps back and forth over the image.
		end, err := image.Seek(0, io.SeekEnd)
		require.NoError(t, err)
		require.Equal(t, size, end)

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
		flasher := testFlasher(t)

		generate(t, flasher, "old", content)
		generate(t, flasher, "new", "regenerated")

		for fingerprint, want := range map[string]string{"old": content, "new": "regenerated"} {
			image, size, _, err := flasher.OpenSeededImage(ctx, testCacheID, fingerprintID(t, flasher, fingerprint))
			require.NoError(t, err)

			body, err := io.ReadAll(image)
			require.NoError(t, err)
			require.NoError(t, image.Close())

			require.Equal(t, want, string(body), fingerprint)
			require.Equal(t, int64(len(want)), size, fingerprint)
		}
	})

	t.Run("names the image differently, if what is seeded into it changed", func(t *testing.T) {
		flasher := testFlasher(t)

		before := fingerprintID(t, flasher, "fingerprint")

		flasher.UpdateServerURL("https://operations-center.example:7443")

		require.NotEqual(t, before, fingerprintID(t, flasher, "fingerprint"))
	})

	t.Run("generates the image only once for concurrent requests", func(t *testing.T) {
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

				image, _, _, err := flasher.GenerateSeededImage(ctx, testCacheID, "fingerprint", uuid.Nil, provisioning.TokenImageSeedConfigs{}, true, sources[i])
				if err != nil {
					errs[i] = err

					return
				}

				body, err := io.ReadAll(image)
				errs[i] = errors.Join(err, image.Close())
				bodies[i] = string(body)
			}()
		}

		close(release)
		wg.Wait()

		for i := range concurrency {
			require.NoError(t, errs[i])
			require.Equal(t, content, bodies[i])
		}

		// Only the request, which won the race, ever read from its source.
		require.Equal(t, int64(1), started.Load())
	})

	t.Run("refuses to generate the image, if the space does not suffice", func(t *testing.T) {
		flasher := testFlasher(t)
		flasher.cache.usage = func() (provisioning.UsageInformation, error) {
			return provisioning.UsageInformation{
				TotalSpaceBytes:     100,
				AvailableSpaceBytes: 1,
			}, nil
		}

		source := &countingReadCloser{ReadCloser: gzipSource(t, content)}

		_, _, _, err := flasher.GenerateSeededImage(ctx, testCacheID, "fingerprint", uuid.Nil, provisioning.TokenImageSeedConfigs{}, true, source)
		require.Error(t, err)

		statusCode, ok := api.StatusErrorMatch(err)
		require.True(t, ok)
		require.Equal(t, http.StatusInsufficientStorage, statusCode)

		require.Equal(t, int64(1), source.closed.Load())
		require.NoFileExists(t, flasher.cache.imageFilename(true, testCacheID, fingerprintID(t, flasher, "fingerprint")))

		requireNoLeftovers(t, flasher, true)
	})

	t.Run("leaves no partial file behind, if the context is cancelled", func(t *testing.T) {
		flasher := testFlasher(t)

		cancelledCtx, cancel := context.WithCancel(ctx)
		cancel()

		source := &countingReadCloser{ReadCloser: gzipSource(t, content)}

		_, _, _, err := flasher.GenerateSeededImage(cancelledCtx, testCacheID, "fingerprint", uuid.Nil, provisioning.TokenImageSeedConfigs{}, true, source)
		require.ErrorIs(t, err, context.Canceled)
		require.Equal(t, int64(1), source.closed.Load())

		requireNoLeftovers(t, flasher, true)
	})

	t.Run("rejects a cache ID, which is not usable as a filename", func(t *testing.T) {
		flasher := testFlasher(t)

		source := &countingReadCloser{ReadCloser: gzipSource(t, content)}

		_, _, _, err := flasher.GenerateSeededImage(ctx, "../../etc/passwd", "fingerprint", uuid.Nil, provisioning.TokenImageSeedConfigs{}, true, source)
		require.Error(t, err)
		require.Equal(t, int64(1), source.closed.Load())
	})

	t.Run("reports the cache as unavailable, if it is not configured", func(t *testing.T) {
		flasher := &Flasher{serverURL: "https://operations-center.local:7443"}

		source := &countingReadCloser{ReadCloser: gzipSource(t, content)}

		_, _, _, err := flasher.GenerateSeededImage(ctx, testCacheID, "fingerprint", uuid.Nil, provisioning.TokenImageSeedConfigs{}, true, source)
		require.Error(t, err)
		require.Equal(t, int64(1), source.closed.Load())
	})
}

func TestFlasher_OpenSeededImage(t *testing.T) {
	const content = "incus-os-image"

	ctx := context.Background()

	t.Run("serves an image generated for a public token seed", func(t *testing.T) {
		flasher := testFlasher(t)

		generate(t, flasher, "fingerprint", content)

		image, size, modTime, err := flasher.OpenSeededImage(ctx, testCacheID, fingerprintID(t, flasher, "fingerprint"))
		require.NoError(t, err)

		body, err := io.ReadAll(image)
		require.NoError(t, err)
		require.NoError(t, image.Close())

		require.Equal(t, content, string(body))
		require.Equal(t, int64(len(content)), size)
		require.False(t, modTime.IsZero())
	})

	t.Run("does not serve an image generated for a token seed, which is not public", func(t *testing.T) {
		flasher := testFlasher(t)

		image, _, _, err := flasher.GenerateSeededImage(ctx, testCacheID, "fingerprint", uuid.Nil, provisioning.TokenImageSeedConfigs{}, false, gzipSource(t, content))
		require.NoError(t, err)
		require.NoError(t, image.Close())

		_, _, _, err = flasher.OpenSeededImage(ctx, testCacheID, fingerprintID(t, flasher, "fingerprint"))
		require.ErrorIs(t, err, domain.ErrNotFound)
	})

	t.Run("reports an image, which has never been generated, as not found", func(t *testing.T) {
		flasher := testFlasher(t)

		_, _, _, err := flasher.OpenSeededImage(ctx, testCacheID, "0123456789ab")
		require.ErrorIs(t, err, domain.ErrNotFound)
	})

	t.Run("rejects an ID, which is not usable as a filename", func(t *testing.T) {
		flasher := testFlasher(t)

		for _, id := range []string{"../../etc/passwd", "", "short", "way-too-long-to-be-an-id"} {
			_, _, _, err := flasher.OpenSeededImage(ctx, testCacheID, id)
			require.ErrorIs(t, err, domain.ErrNotFound, id)
		}
	})

	t.Run("waits for an image, which is being generated right now", func(t *testing.T) {
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
			image, _, _, err := flasher.GenerateSeededImage(ctx, testCacheID, "fingerprint", uuid.Nil, provisioning.TokenImageSeedConfigs{}, true, source)
			if err == nil {
				_ = image.Close()
			}
		}()

		require.Eventually(t, func() bool { return started.Load() == 1 }, time.Second, time.Millisecond)

		opened := make(chan string, 1)

		go func() {
			image, _, _, err := flasher.OpenSeededImage(ctx, testCacheID, id)
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

		select {
		case got := <-opened:
			t.Fatalf("the image must not be served before it is generated, got %q", got)

		case <-time.After(50 * time.Millisecond):
		}

		close(release)

		select {
		case got := <-opened:
			require.Equal(t, content, got)

		case <-time.After(10 * time.Second):
			t.Fatal("the image has not been served after the generation completed")
		}
	})

	t.Run("reports the cache as unavailable, if it is not configured", func(t *testing.T) {
		flasher := &Flasher{serverURL: "https://operations-center.local:7443"}

		_, _, _, err := flasher.OpenSeededImage(ctx, testCacheID, "0123456789ab")
		require.Error(t, err)
	})
}

func TestFlasher_PruneCache(t *testing.T) {
	const content = "incus-os-image"

	ctx := context.Background()

	t.Run("keeps a recently accessed image and removes a stale one", func(t *testing.T) {
		flasher := testFlasher(t)

		generate(t, flasher, "fingerprint", content)

		id := fingerprintID(t, flasher, "fingerprint")
		imageFile := flasher.cache.imageFilename(true, testCacheID, id)

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
		flasher := testFlasher(t)

		image, _, _, err := flasher.GenerateSeededImage(ctx, testCacheID, "fingerprint", uuid.Nil, provisioning.TokenImageSeedConfigs{}, false, gzipSource(t, content))
		require.NoError(t, err)
		require.NoError(t, image.Close())

		id := fingerprintID(t, flasher, "fingerprint")
		imageFile := flasher.cache.imageFilename(false, testCacheID, id)
		require.FileExists(t, imageFile)

		flasher.cache.entries[cacheKey(false, testCacheID, id)].lastAccess = time.Now().Add(-2 * time.Hour)

		stale := time.Now().Add(-2 * time.Hour)
		require.NoError(t, os.Chtimes(imageFile, stale, stale))

		require.NoError(t, flasher.PruneCache(ctx, time.Hour))
		require.NoFileExists(t, imageFile)
	})

	t.Run("removes leftovers of an interrupted generation", func(t *testing.T) {
		flasher := testFlasher(t)

		dir := flasher.cache.namespaceDir(true)
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
		require.NoError(t, (&Flasher{}).PruneCache(ctx, time.Hour))
		require.NoError(t, testFlasher(t).PruneCache(ctx, time.Hour))
	})
}

func TestImageCache_open(t *testing.T) {
	ctx := context.Background()

	t.Run("reports a cache miss for an image, which is not stored", func(t *testing.T) {
		cache := newImageCache(t.TempDir())

		_, _, _, err := cache.open(true, testCacheID, "0123456789ab")
		require.ErrorIs(t, err, errCacheMiss)
	})

	t.Run("reports the size and the generation time of the stored image", func(t *testing.T) {
		cache := newImageCache(t.TempDir())

		before := time.Now().Add(-time.Second)

		require.NoError(t, cache.write(ctx, true, testCacheID, "0123456789ab", bytes.NewReader([]byte("incus-os-image"))))

		image, size, modTime, err := cache.open(true, testCacheID, "0123456789ab")
		require.NoError(t, err)
		require.NoError(t, image.Close())

		require.Equal(t, int64(len("incus-os-image")), size)
		require.True(t, modTime.After(before))
	})

	t.Run("keeps the two namespaces apart", func(t *testing.T) {
		cache := newImageCache(t.TempDir())

		require.NoError(t, cache.write(ctx, false, testCacheID, "0123456789ab", bytes.NewReader([]byte("private"))))

		_, _, _, err := cache.open(true, testCacheID, "0123456789ab")
		require.ErrorIs(t, err, errCacheMiss)

		image, _, _, err := cache.open(false, testCacheID, "0123456789ab")
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
