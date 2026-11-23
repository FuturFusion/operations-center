package flasher

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/provisioning/adapter/mediarecord/mediarecordtest"
	"github.com/FuturFusion/operations-center/internal/util/testing/fsroot"
)

func TestImageCache_records(t *testing.T) {
	t.Parallel()

	newCache := func(t *testing.T) (*imageCache, *mediarecordtest.Repo) {
		t.Helper()

		repo := mediarecordtest.NewRepo()
		records := mediarecordtest.Records(repo, provisioning.MediaTypeSeedImage, time.Hour)

		cache := newImageCache(fsroot.Open(t, t.TempDir()))
		cache.records = &records

		return cache, repo
	}

	flasherOf := func(cache *imageCache) *Flasher {
		return &Flasher{cache: cache}
	}

	t.Run("an image of a public token seed is recorded for this member", func(t *testing.T) {
		t.Parallel()

		cache, _ := newCache(t)
		require.NoError(t, cache.write(t.Context(), true, testCacheID, 0, "0123456789ab", bytes.NewReader([]byte(testContent))))

		memberID, available, err := flasherOf(cache).SeededImageHolder(t.Context(), testCacheID, "0123456789ab")
		require.NoError(t, err)
		require.True(t, available)
		require.Equal(t, int64(1), memberID)
	})

	t.Run("an image of a private token seed is not recorded", func(t *testing.T) {
		t.Parallel()

		cache, _ := newCache(t)
		require.NoError(t, cache.write(t.Context(), false, testCacheID, 0, "0123456789ab", bytes.NewReader([]byte(testContent))))

		_, available, err := flasherOf(cache).SeededImageHolder(t.Context(), testCacheID, "0123456789ab")
		require.NoError(t, err)
		require.False(t, available)
	})

	t.Run("an image recorded for a member, which is gone, is taken over when it is read", func(t *testing.T) {
		t.Parallel()

		cache, repo := newCache(t)
		require.NoError(t, cache.write(t.Context(), true, testCacheID, 0, "0123456789ab", bytes.NewReader([]byte(testContent))))

		repo.SetMember(2)

		image, _, err := cache.open(t.Context(), true, testCacheID, "0123456789ab")
		require.NoError(t, err)
		require.NoError(t, image.Close())

		memberID, available, err := flasherOf(cache).SeededImageHolder(t.Context(), testCacheID, "0123456789ab")
		require.NoError(t, err)
		require.True(t, available)
		require.Equal(t, int64(1), memberID)
	})

	t.Run("the prune removes the images of public token seeds by their records", func(t *testing.T) {
		t.Parallel()

		cache, repo := newCache(t)
		require.NoError(t, cache.write(t.Context(), true, testCacheID, 0, "expired00000", bytes.NewReader([]byte(testContent))))

		repo.Expire()

		require.NoError(t, cache.write(t.Context(), true, testCacheID, 0, "recorded0000", bytes.NewReader([]byte(testContent))))

		unrecorded := absImageFilename(cache, true, testCacheID, "unrecorded00")
		require.NoError(t, os.MkdirAll(filepath.Dir(unrecorded), 0o700))
		require.NoError(t, os.WriteFile(unrecorded, []byte(testContent), 0o600))

		require.NoError(t, cache.prune(t.Context(), time.Hour))

		require.NoFileExists(t, absImageFilename(cache, true, testCacheID, "expired00000"), "An image, whose lease ended, has to be removed")
		require.NoFileExists(t, unrecorded, "An image, which is not recorded for this member, has to be removed")
		require.FileExists(t, absImageFilename(cache, true, testCacheID, "recorded0000"), "An image within its lease has to be kept")

		_, available, err := flasherOf(cache).SeededImageHolder(t.Context(), testCacheID, "expired00000")
		require.NoError(t, err)
		require.False(t, available, "The record of a pruned image is removed")
	})

	t.Run("removing the images of a token seed removes their records", func(t *testing.T) {
		t.Parallel()

		cache, _ := newCache(t)
		otherCacheID := "zzzzzz012345"

		require.NoError(t, cache.write(t.Context(), true, testCacheID, 0, "0123456789ab", bytes.NewReader([]byte(testContent))))
		require.NoError(t, cache.write(t.Context(), true, otherCacheID, 0, "0123456789ab", bytes.NewReader([]byte(testContent))))

		require.NoError(t, flasherOf(cache).RemoveSeededImageRecords(t.Context(), testCacheID[:provisioning.SeedImageCachePrefixLength]))

		_, available, err := flasherOf(cache).SeededImageHolder(t.Context(), testCacheID, "0123456789ab")
		require.NoError(t, err)
		require.False(t, available)

		_, available, err = flasherOf(cache).SeededImageHolder(t.Context(), otherCacheID, "0123456789ab")
		require.NoError(t, err)
		require.True(t, available, "The images of another token seed are kept")
	})
}
