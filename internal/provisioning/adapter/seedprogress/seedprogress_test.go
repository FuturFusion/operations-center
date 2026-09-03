package seedprogress_test

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/provisioning/adapter/seedprogress"
)

const testSource = "10.0.0.1"

var testImageID = provisioning.SeedImageID{
	CacheID:       "cache-id",
	FingerprintID: "fingerprint",
}

func TestTracker_overlappingRangeReads(t *testing.T) {
	const content = "0123456789abcdefghij"

	ctx := context.Background()
	tracker := seedprogress.New()

	image := tracker.Track(ctx, testImageID, testSource, provisioning.SeedImageInfo{Size: int64(len(content))}, nopReadSeekCloser{strings.NewReader(content)})

	readRange(t, image, 10, 5)
	readRange(t, image, 0, 5)

	require.NoError(t, image.Close())

	progress, ok := tracker.Get(ctx, testImageID, testSource)
	require.True(t, ok)
	require.Equal(t, int64(10), progress.BytesServed)
	require.Equal(t, int64(10), progress.BytesCovered, "two disjoint ranges cover what they add up to")
	require.Equal(t, 1, progress.RequestCount)
}

func TestTracker_repeatedRangeReads(t *testing.T) {
	const content = "0123456789abcdefghij"

	ctx := context.Background()
	tracker := seedprogress.New()

	image := tracker.Track(ctx, testImageID, testSource, provisioning.SeedImageInfo{Size: int64(len(content))}, nopReadSeekCloser{strings.NewReader(content)})

	readRange(t, image, 0, 5)
	readRange(t, image, 0, 5)

	require.NoError(t, image.Close())

	progress, ok := tracker.Get(ctx, testImageID, testSource)
	require.True(t, ok)
	require.Equal(t, int64(10), progress.BytesServed, "every byte handed out is counted")
	require.Equal(t, int64(5), progress.BytesCovered, "a range read twice is only covered once")
}

func TestTracker_coverageAcrossRequests(t *testing.T) {
	const content = "0123456789abcdefghij"

	ctx := context.Background()
	tracker := seedprogress.New()

	for _, read := range []struct {
		offset int64
		length int64
	}{
		{offset: 0, length: 10},
		{offset: 5, length: 15},
	} {
		image := tracker.Track(ctx, testImageID, testSource, provisioning.SeedImageInfo{Size: int64(len(content))}, nopReadSeekCloser{strings.NewReader(content)})

		readRange(t, image, read.offset, read.length)

		require.NoError(t, image.Close())
	}

	progress, ok := tracker.Get(ctx, testImageID, testSource)
	require.True(t, ok)
	require.Equal(t, int64(25), progress.BytesServed)
	require.Equal(t, int64(20), progress.BytesCovered, "the ranges two requests overlap in are only covered once")
	require.Equal(t, 2, progress.RequestCount)
}

func TestTracker_seekedReadsRecordAbsoluteOffsets(t *testing.T) {
	const content = "0123456789abcdefghij"

	ctx := context.Background()
	tracker := seedprogress.New()

	image := tracker.Track(ctx, testImageID, testSource, provisioning.SeedImageInfo{Size: int64(len(content))}, nopReadSeekCloser{strings.NewReader(content)})

	_, err := image.Seek(-4, io.SeekEnd)
	require.NoError(t, err)

	_, err = io.Copy(io.Discard, io.LimitReader(image, 4))
	require.NoError(t, err)

	_, err = image.Seek(-4, io.SeekCurrent)
	require.NoError(t, err)

	_, err = io.Copy(io.Discard, io.LimitReader(image, 4))
	require.NoError(t, err)

	readRange(t, image, 0, 4)

	require.NoError(t, image.Close())

	progress, ok := tracker.Get(ctx, testImageID, testSource)
	require.True(t, ok)
	require.Equal(t, int64(12), progress.BytesServed)
	require.Equal(t, int64(8), progress.BytesCovered, "a range reached by any kind of seek is covered once")
}

func TestTracker_reset(t *testing.T) {
	const content = "0123456789"

	ctx := context.Background()
	tracker := seedprogress.New()

	otherImage := provisioning.SeedImageID{CacheID: "other", FingerprintID: "fingerprint"}

	for _, read := range []struct {
		imageID provisioning.SeedImageID
		source  string
	}{
		{imageID: testImageID, source: testSource},
		{imageID: testImageID, source: "10.0.0.2"},
		{imageID: otherImage, source: testSource},
	} {
		image := tracker.Track(ctx, read.imageID, read.source, provisioning.SeedImageInfo{Size: int64(len(content))}, nopReadSeekCloser{strings.NewReader(content)})

		readRange(t, image, 0, 5)

		require.NoError(t, image.Close())
	}

	tracker.Reset(ctx, testImageID)

	require.Empty(t, tracker.GetByImage(ctx, testImageID), "every source, that read the image, is dropped")
	require.Len(t, tracker.GetByImage(ctx, otherImage), 1, "another image is kept")
}

func TestTracker_getByImage(t *testing.T) {
	const content = "0123456789"

	ctx := context.Background()
	tracker := seedprogress.New()

	otherImage := provisioning.SeedImageID{CacheID: "other", FingerprintID: "fingerprint"}

	for _, read := range []struct {
		imageID provisioning.SeedImageID
		source  string
		length  int64
	}{
		{imageID: testImageID, source: "10.0.0.2", length: 4},
		{imageID: testImageID, source: "10.0.0.1", length: 7},
		{imageID: otherImage, source: "10.0.0.3", length: 2},
	} {
		image := tracker.Track(ctx, read.imageID, read.source, provisioning.SeedImageInfo{Size: int64(len(content))}, nopReadSeekCloser{strings.NewReader(content)})

		readRange(t, image, 0, read.length)

		require.NoError(t, image.Close())
	}

	progress := tracker.GetByImage(ctx, testImageID)

	require.Len(t, progress, 2)
	require.Equal(t, "10.0.0.1", progress[0].Source)
	require.Equal(t, int64(7), progress[0].BytesServed)
	require.Equal(t, int64(7), progress[0].BytesCovered)
	require.Equal(t, "10.0.0.2", progress[1].Source)
	require.Equal(t, int64(4), progress[1].BytesServed)
	require.Equal(t, int64(4), progress[1].BytesCovered)

	require.Empty(t, tracker.GetByImage(ctx, provisioning.SeedImageID{CacheID: "unread", FingerprintID: "fingerprint"}))
}

func TestTracker_evictionAfterIdlePeriod(t *testing.T) {
	const content = "0123456789"

	ctx := context.Background()
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)

	tracker := seedprogress.New(
		seedprogress.WithIdleEvictionPeriod(time.Hour),
		seedprogress.WithNow(func() time.Time { return now }),
	)

	image := tracker.Track(ctx, testImageID, testSource, provisioning.SeedImageInfo{Size: int64(len(content))}, nopReadSeekCloser{strings.NewReader(content)})

	_, err := io.ReadAll(image)
	require.NoError(t, err)
	require.NoError(t, image.Close())

	now = now.Add(time.Hour)

	progress, ok := tracker.Get(ctx, testImageID, testSource)
	require.True(t, ok)
	require.Equal(t, time.Hour, progress.IdleFor(now))

	now = now.Add(time.Second)

	_, ok = tracker.Get(ctx, testImageID, testSource)
	require.False(t, ok)
}

func readRange(t *testing.T, image io.ReadSeeker, offset int64, length int64) {
	t.Helper()

	_, err := image.Seek(offset, io.SeekStart)
	require.NoError(t, err)

	_, err = io.Copy(io.Discard, io.LimitReader(image, length))
	require.NoError(t, err)
}

type nopReadSeekCloser struct {
	io.ReadSeeker
}

func (nopReadSeekCloser) Close() error { return nil }
