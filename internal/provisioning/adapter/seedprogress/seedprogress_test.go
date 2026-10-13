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

const testDeploymentID = "deployment-1"

func TestTracker_overlappingRangeReads(t *testing.T) {
	t.Parallel()

	const content = "0123456789abcdefghij"

	ctx := context.Background()
	tracker := seedprogress.New()

	image := tracker.Track(ctx, testDeploymentID, provisioning.SeedImageInfo{Size: int64(len(content))}, nopReadSeekCloser{strings.NewReader(content)})

	readRange(t, image, 10, 5)
	readRange(t, image, 0, 5)

	require.NoError(t, image.Close())

	progress, ok := tracker.Get(ctx, testDeploymentID)
	require.True(t, ok)
	require.Equal(t, int64(10), progress.BytesServed)
	require.Equal(t, int64(10), progress.BytesCovered, "two disjoint ranges cover what they add up to")
	require.Equal(t, 1, progress.RequestCount)
}

func TestTracker_repeatedRangeReads(t *testing.T) {
	t.Parallel()

	const content = "0123456789abcdefghij"

	ctx := context.Background()
	tracker := seedprogress.New()

	image := tracker.Track(ctx, testDeploymentID, provisioning.SeedImageInfo{Size: int64(len(content))}, nopReadSeekCloser{strings.NewReader(content)})

	readRange(t, image, 0, 5)
	readRange(t, image, 0, 5)

	require.NoError(t, image.Close())

	progress, ok := tracker.Get(ctx, testDeploymentID)
	require.True(t, ok)
	require.Equal(t, int64(10), progress.BytesServed, "every byte handed out is counted")
	require.Equal(t, int64(5), progress.BytesCovered, "a range read twice is only covered once")
}

func TestTracker_coverageAcrossRequests(t *testing.T) {
	t.Parallel()

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
		image := tracker.Track(ctx, testDeploymentID, provisioning.SeedImageInfo{Size: int64(len(content))}, nopReadSeekCloser{strings.NewReader(content)})

		readRange(t, image, read.offset, read.length)

		require.NoError(t, image.Close())
	}

	progress, ok := tracker.Get(ctx, testDeploymentID)
	require.True(t, ok)
	require.Equal(t, int64(25), progress.BytesServed)
	require.Equal(t, int64(20), progress.BytesCovered, "the ranges two requests overlap in are only covered once")
	require.Equal(t, 2, progress.RequestCount)
}

func TestTracker_seekedReadsRecordAbsoluteOffsets(t *testing.T) {
	t.Parallel()

	const content = "0123456789abcdefghij"

	ctx := context.Background()
	tracker := seedprogress.New()

	image := tracker.Track(ctx, testDeploymentID, provisioning.SeedImageInfo{Size: int64(len(content))}, nopReadSeekCloser{strings.NewReader(content)})

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

	progress, ok := tracker.Get(ctx, testDeploymentID)
	require.True(t, ok)
	require.Equal(t, int64(12), progress.BytesServed)
	require.Equal(t, int64(8), progress.BytesCovered, "a range reached by any kind of seek is covered once")
}

func TestTrackerReset(t *testing.T) {
	t.Parallel()

	const content = "0123456789"

	ctx := context.Background()
	tracker := seedprogress.New()

	const otherDeploymentID = "deployment-2"

	for _, deploymentID := range []string{testDeploymentID, otherDeploymentID} {
		image := tracker.Track(ctx, deploymentID, provisioning.SeedImageInfo{Size: int64(len(content))}, nopReadSeekCloser{strings.NewReader(content)})

		readRange(t, image, 0, 5)

		require.NoError(t, image.Close())
	}

	tracker.Reset(ctx, testDeploymentID)

	_, ok := tracker.Get(ctx, testDeploymentID)
	require.False(t, ok, "what the deployment read is dropped")

	progress, ok := tracker.Get(ctx, otherDeploymentID)
	require.True(t, ok, "the deployment still reading the image keeps its progress")
	require.Equal(t, int64(5), progress.BytesCovered)

	tracker.Reset(ctx, "deployment-never-seen")
}

func TestTracker_readsFromSeveralSources(t *testing.T) {
	t.Parallel()

	const content = "0123456789"

	ctx := context.Background()
	tracker := seedprogress.New()

	for _, read := range []struct {
		offset int64
		length int64
	}{
		{offset: 0, length: 4},
		{offset: 4, length: 3},
	} {
		image := tracker.Track(ctx, testDeploymentID, provisioning.SeedImageInfo{Size: int64(len(content))}, nopReadSeekCloser{strings.NewReader(content)})

		readRange(t, image, read.offset, read.length)

		require.NoError(t, image.Close())
	}

	progress, ok := tracker.Get(ctx, testDeploymentID)
	require.True(t, ok)
	require.Equal(t, int64(7), progress.BytesServed)
	require.Equal(t, int64(7), progress.BytesCovered, "the reads of both addresses add up")
	require.Equal(t, 2, progress.RequestCount)

	_, ok = tracker.Get(ctx, "deployment-never-seen")
	require.False(t, ok)
}

func TestTracker_evictionAfterIdlePeriod(t *testing.T) {
	t.Parallel()

	const content = "0123456789"

	ctx := context.Background()
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)

	tracker := seedprogress.New(
		seedprogress.WithIdleEvictionPeriod(time.Hour),
		seedprogress.WithNow(func() time.Time { return now }),
	)

	image := tracker.Track(ctx, testDeploymentID, provisioning.SeedImageInfo{Size: int64(len(content))}, nopReadSeekCloser{strings.NewReader(content)})

	_, err := io.ReadAll(image)
	require.NoError(t, err)
	require.NoError(t, image.Close())

	now = now.Add(time.Hour)

	progress, ok := tracker.Get(ctx, testDeploymentID)
	require.True(t, ok)
	require.Equal(t, time.Hour, progress.IdleFor(now))

	now = now.Add(time.Second)

	_, ok = tracker.Get(ctx, testDeploymentID)
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
