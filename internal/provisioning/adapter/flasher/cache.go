package flasher

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/util/file"
	"github.com/FuturFusion/operations-center/shared/api"
)

const (
	cacheImageExt       = ".img"
	cachePartialExt     = ".partial"
	cacheFreeSpaceRatio = 0.1
)

const (
	cachePublicDir  = "public"
	cachePrivateDir = "private"
)

const seedImageSizeUpperBound = 6 * 1024 * 1024 * 1024 // bytes, ~double the current size

var errCacheMiss = errors.New("Seed image is not cached")

var cacheIDRegexp = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// imageCache stores completely generated, decompressed seed images as files.
type imageCache struct {
	dir string

	// usage reports the space situation of the filesystem holding the cache.
	usage func() (file.UsageInformation, error)

	mu      sync.Mutex
	entries map[string]*cacheEntry
}

type cacheEntry struct {
	// generating is non nil while a caller is generating the image and is closed
	// as soon as it is done, successfully or not.
	generating chan struct{}

	// lastAccess is the time the image has been handed out for the last time.
	// It is zero for an entry, which has not been accessed since the last
	// restart, in which case the modification time of the file is used instead.
	lastAccess time.Time
}

func newImageCache(dir string) *imageCache {
	cache := &imageCache{
		dir:     dir,
		entries: map[string]*cacheEntry{},
	}

	cache.usage = cache.usageInformation

	return cache
}

// SeedImageFingerprintID returns the ID of the image generated from fingerprint
// and the given seed configuration, without generating anything.
//
// It lets a caller name an image before it exists, so that the address an image
// is served under can be handed out while it is still being generated.
func (f *Flasher) SeedImageFingerprintID(ctx context.Context, fingerprint string, id uuid.UUID, seedConfig provisioning.TokenImageSeedConfigs) (string, error) {
	_, tarball, err := f.seedTarball(ctx, id, seedConfig)
	if err != nil {
		return "", err
	}

	return seedImageFingerprintID(fingerprint, tarball), nil
}

// seedImageFingerprintID folds what the flasher itself seeds into every image
// into the fingerprint of the caller.
func seedImageFingerprintID(fingerprint string, tarball []byte) string {
	tarballSum := sha256.Sum256(tarball)

	return provisioning.SeedImageFingerprintID(fingerprint + "\x00" + base64.RawURLEncoding.EncodeToString(tarballSum[:]))
}

// GenerateSeededImage stores the seeded image for cacheID in the cache, unless
// it is stored already, and returns it as a seekable stream.
//
// public tells whether the image may be served to a request naming it, which is
// only true for an image generated for a public token seed. An image generated
// for any other token seed is kept apart, so that it can never be reached
// without the request having been authorized for it.
//
// The call blocks until the image is stored. Concurrent calls for the same
// image generate it only once. source is closed in all cases.
func (f *Flasher) GenerateSeededImage(ctx context.Context, cacheID string, fingerprint string, id uuid.UUID, seedConfig provisioning.TokenImageSeedConfigs, public bool, source io.ReadCloser) (_ io.ReadSeekCloser, _ provisioning.SeedImageInfo, err error) {
	// source is only consumed, if the image has to be generated.
	defer func() {
		if source != nil {
			err = errors.Join(err, source.Close())
		}
	}()

	if f.cache == nil {
		return nil, provisioning.SeedImageInfo{}, errors.New("Seed image cache is not configured")
	}

	if !cacheIDRegexp.MatchString(cacheID) {
		return nil, provisioning.SeedImageInfo{}, fmt.Errorf("Invalid seed image cache ID %q", cacheID)
	}

	// The seed tarball is deterministic for a given configuration and cheap to
	// build compared to the image, so the image can be named before it is known,
	// whether it has to be generated at all.
	offset, tarball, err := f.seedTarball(ctx, id, seedConfig)
	if err != nil {
		return nil, provisioning.SeedImageInfo{}, err
	}

	fingerprintID := seedImageFingerprintID(fingerprint, tarball)

	for {
		image, info, err := f.cache.open(public, cacheID, fingerprintID)
		if err == nil {
			return image, info, nil
		}

		if !errors.Is(err, errCacheMiss) {
			return nil, provisioning.SeedImageInfo{}, err
		}

		wait, release := f.cache.acquire(public, cacheID, fingerprintID)
		if release == nil {
			// The very same image is already being generated, wait for it
			// instead of generating it a second time.
			select {
			case <-ctx.Done():
				return nil, provisioning.SeedImageInfo{}, ctx.Err()

			case <-wait:
			}

			continue
		}

		generateSource := source

		// generate takes ownership of the source and closes it.
		source = nil

		err = func() error {
			defer release()

			return f.generate(ctx, public, cacheID, fingerprintID, offset, tarball, generateSource)
		}()
		if err != nil {
			return nil, provisioning.SeedImageInfo{}, err
		}

		image, info, err = f.cache.open(public, cacheID, fingerprintID)
		if err != nil {
			return nil, provisioning.SeedImageInfo{}, fmt.Errorf("Failed to open the just generated cached seed image: %w", err)
		}

		return image, info, nil
	}
}

// OpenSeededImage returns the generated image addressed by cacheID and
// fingerprintID as a seekable stream.
//
// It only ever hands out an image generated for a public token seed.
// An image being generated right now is waited for.
func (f *Flasher) OpenSeededImage(ctx context.Context, cacheID string, fingerprintID string) (_ io.ReadSeekCloser, _ provisioning.SeedImageInfo, _ error) {
	if f.cache == nil {
		return nil, provisioning.SeedImageInfo{}, errors.New("Seed image cache is not configured")
	}

	if !cacheIDRegexp.MatchString(cacheID) || !cacheIDRegexp.MatchString(fingerprintID) {
		return nil, provisioning.SeedImageInfo{}, fmt.Errorf("No seed image %q is available: %w", fingerprintID, domain.ErrNotFound)
	}

	for {
		image, info, err := f.cache.open(true, cacheID, fingerprintID)
		if err == nil {
			return image, info, nil
		}

		if !errors.Is(err, errCacheMiss) {
			return nil, provisioning.SeedImageInfo{}, err
		}

		wait := f.cache.generating(true, cacheID, fingerprintID)
		if wait == nil {
			return nil, provisioning.SeedImageInfo{}, fmt.Errorf("No seed image %q is available: %w", fingerprintID, domain.ErrNotFound)
		}

		select {
		case <-ctx.Done():
			return nil, provisioning.SeedImageInfo{}, ctx.Err()

		case <-wait:
		}
	}
}

// PruneCache removes cached images, which have not been accessed within ttl,
// together with leftovers of interrupted generations.
func (f *Flasher) PruneCache(ctx context.Context, ttl time.Duration) error {
	if f.cache == nil {
		return nil
	}

	return f.cache.prune(ctx, ttl)
}

// generate writes the seeded image to the cache. It closes source.
func (f *Flasher) generate(ctx context.Context, public bool, cacheID string, fingerprintID string, offset int64, tarball []byte, source io.ReadCloser) (err error) {
	image, err := seededImage(source, offset, tarball)
	if err != nil {
		return errors.Join(fmt.Errorf("Failed to generate seeded image: %w", err), source.Close())
	}

	// Closing the generated image also closes source.
	defer func() {
		err = errors.Join(err, image.Close())
	}()

	err = f.cache.checkFreeSpace(seedImageSizeUpperBound)
	if err != nil {
		return err
	}

	return f.cache.write(ctx, public, cacheID, fingerprintID, image)
}

// acquire either returns a channel to wait on, because another caller is
// generating the image already, or a function to release the generation with,
// because the caller has taken it over.
func (c *imageCache) acquire(public bool, cacheID string, fingerprintID string) (wait <-chan struct{}, release func()) {
	key := cacheKey(public, cacheID, fingerprintID)

	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.entries[key]
	if !ok {
		entry = &cacheEntry{}
		c.entries[key] = entry
	}

	if entry.generating != nil {
		return entry.generating, nil
	}

	generating := make(chan struct{})
	entry.generating = generating

	return nil, func() {
		c.mu.Lock()
		defer c.mu.Unlock()

		entry.generating = nil
		close(generating)
	}
}

// generating returns a channel closed as soon as the image is done being
// generated, or nil, if nobody is generating it. Unlike acquire it only reports
// what is going on and never takes the generation over.
func (c *imageCache) generating(public bool, cacheID string, fingerprintID string) <-chan struct{} {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.entries[cacheKey(public, cacheID, fingerprintID)]
	if !ok {
		return nil
	}

	return entry.generating
}

// open returns the cached image, or reports errCacheMiss, if it is not stored.
func (c *imageCache) open(public bool, cacheID string, fingerprintID string) (_ io.ReadSeekCloser, _ provisioning.SeedImageInfo, _ error) {
	imageFile, err := os.Open(c.imageFilename(public, cacheID, fingerprintID))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, provisioning.SeedImageInfo{}, errCacheMiss
		}

		return nil, provisioning.SeedImageInfo{}, fmt.Errorf("Failed to open cached seed image %q: %w", fingerprintID, err)
	}

	fileInfo, err := imageFile.Stat()
	if err != nil {
		return nil, provisioning.SeedImageInfo{}, errors.Join(fmt.Errorf("Failed to stat cached seed image %q: %w", fingerprintID, err), imageFile.Close())
	}

	c.touch(cacheKey(public, cacheID, fingerprintID))

	return imageFile, provisioning.SeedImageInfo{
		Size:    fileInfo.Size(),
		ModTime: fileInfo.ModTime(),
	}, nil
}

// write stores the image under the name it is addressed by.
func (c *imageCache) write(ctx context.Context, public bool, cacheID string, fingerprintID string, image io.Reader) (err error) {
	dir := c.namespaceDir(public)

	err = os.MkdirAll(dir, 0o700)
	if err != nil {
		return fmt.Errorf("Failed to create seed image cache directory %q: %w", dir, err)
	}

	partial, err := os.CreateTemp(dir, imageBasename(cacheID, fingerprintID)+".*"+cachePartialExt)
	if err != nil {
		return fmt.Errorf("Failed to create partial file for cached seed image %q: %w", fingerprintID, err)
	}

	partialName := partial.Name()
	closePartial := sync.OnceValue(partial.Close)

	defer func() {
		if err == nil {
			return
		}

		err = errors.Join(err, closePartial())

		if partialName != "" {
			err = errors.Join(err, os.Remove(partialName))
		}
	}()

	_, err = file.SafeCopy(partial, newContextReader(ctx, image))
	if err != nil {
		return fmt.Errorf("Failed to write cached seed image %q: %w", fingerprintID, err)
	}

	err = closePartial()
	if err != nil {
		return fmt.Errorf("Failed to close partial file for cached seed image %q: %w", fingerprintID, err)
	}

	err = os.Rename(partialName, c.imageFilename(public, cacheID, fingerprintID))
	if err != nil {
		return fmt.Errorf("Failed to move cached seed image %q into place: %w", fingerprintID, err)
	}

	partialName = ""

	c.touch(cacheKey(public, cacheID, fingerprintID))

	return nil
}

// prune removes cached images not accessed within ttl and leftovers of
// interrupted generations.
func (c *imageCache) prune(ctx context.Context, ttl time.Duration) error {
	var errs []error

	for _, namespace := range []string{cachePublicDir, cachePrivateDir} {
		err := c.pruneNamespace(ctx, namespace, ttl)
		if err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

func (c *imageCache) pruneNamespace(ctx context.Context, namespace string, ttl time.Duration) error {
	dir := filepath.Join(c.dir, namespace)

	dirEntries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}

		return fmt.Errorf("Failed to read seed image cache directory %q: %w", dir, err)
	}

	var errs []error

	for _, dirEntry := range dirEntries {
		if dirEntry.IsDir() {
			continue
		}

		filename := dirEntry.Name()

		fileInfo, err := dirEntry.Info()
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}

			errs = append(errs, err)
			continue
		}

		switch {
		case strings.HasSuffix(filename, cachePartialExt):
			// Partial files of a generation still in flight are younger than the
			// TTL by a wide margin.
			if time.Since(fileInfo.ModTime()) < ttl {
				continue
			}

		case strings.HasSuffix(filename, cacheImageExt):
			key := namespace + "/" + strings.TrimSuffix(filename, cacheImageExt)

			keep, err := c.keep(key, fileInfo.ModTime(), ttl)
			if err != nil {
				errs = append(errs, err)
				continue
			}

			if keep {
				continue
			}

		default:
			// Not something the cache has written, leave it alone.
			continue
		}

		err = os.Remove(filepath.Join(dir, filename))
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			errs = append(errs, err)
		}

		if ctx.Err() != nil {
			errs = append(errs, ctx.Err())
			break
		}
	}

	return errors.Join(errs...)
}

// keep reports, whether the file of a cached image is still in use. modTime is
// the modification time of the file being considered, used as the last access
// for an entry, which has not been accessed since the last restart.
func (c *imageCache) keep(key string, modTime time.Time, ttl time.Duration) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.entries[key]
	if ok && entry.generating != nil {
		return true, nil
	}

	lastAccess := modTime
	if ok && entry.lastAccess.After(lastAccess) {
		lastAccess = entry.lastAccess
	}

	if time.Since(lastAccess) < ttl {
		return true, nil
	}

	delete(c.entries, key)

	return false, nil
}

// touch records, that a cached image has been used.
func (c *imageCache) touch(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.entries[key]
	if !ok {
		entry = &cacheEntry{}
		c.entries[key] = entry
	}

	entry.lastAccess = time.Now()
}

// cacheKey identifies one cached image, both in the entries map and, with the
// extension appended, as the name of the file holding it.
func cacheKey(public bool, cacheID string, fingerprintID string) string {
	return cacheNamespace(public) + "/" + imageBasename(cacheID, fingerprintID)
}

// imageBasename is the name a cached image is addressed by: what was asked for,
// and what the image has been generated from.
func imageBasename(cacheID string, fingerprintID string) string {
	return cacheID + "-" + fingerprintID
}

func cacheNamespace(public bool) string {
	if public {
		return cachePublicDir
	}

	return cachePrivateDir
}

func (c *imageCache) namespaceDir(public bool) string {
	return filepath.Join(c.dir, cacheNamespace(public))
}

func (c *imageCache) imageFilename(public bool, cacheID string, fingerprintID string) string {
	return filepath.Join(c.namespaceDir(public), imageBasename(cacheID, fingerprintID)+cacheImageExt)
}

// checkFreeSpace reports an error, if writing requiredSize bytes would not leave
// enough free space on the filesystem holding the cache.
func (c *imageCache) checkFreeSpace(requiredSize int64) error {
	usage, err := c.usage()
	if err != nil {
		return err
	}

	if usage.TotalSpaceBytes < 1 {
		return fmt.Errorf("Seed image cache reported an invalid total space: %d", usage.TotalSpaceBytes)
	}

	if (float64(usage.AvailableSpaceBytes)-float64(requiredSize))/float64(usage.TotalSpaceBytes) < cacheFreeSpaceRatio {
		return api.StatusErrorf(http.StatusInsufficientStorage, "Not enough space available to cache the seeded image, require: %d, available: %d, required headroom: %.0f%%", requiredSize, usage.AvailableSpaceBytes, cacheFreeSpaceRatio*100)
	}

	return nil
}

// contextReader aborts a read, as soon as the context is done, so that a client
// going away does not leave a multi gigabyte copy running.
type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func newContextReader(ctx context.Context, r io.Reader) *contextReader {
	return &contextReader{
		ctx: ctx,
		r:   r,
	}
}

func (c *contextReader) Read(p []byte) (int, error) {
	err := c.ctx.Err()
	if err != nil {
		return 0, err
	}

	return c.r.Read(p)
}
