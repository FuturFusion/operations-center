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

func TestTracker_highestOffsetDoesNotRegress(t *testing.T) {
	const content = "0123456789abcdefghij"

	ctx := context.Background()
	tracker := seedprogress.New()

	image := tracker.Track(ctx, testImageID, testSource, int64(len(content)), nopReadSeekCloser{strings.NewReader(content)})

	readRange(t, image, 10, 5)
	readRange(t, image, 0, 5)

	require.NoError(t, image.Close())

	progress, ok := tracker.Get(ctx, testImageID, testSource)
	require.True(t, ok)
	require.Equal(t, int64(15), progress.HighestOffset)
	require.Equal(t, int64(10), progress.BytesServed)
	require.Equal(t, 1, progress.RequestCount)
	require.InDelta(t, 75.0, progress.PercentComplete(), 0.001)
}

func TestTracker_evictionAfterIdlePeriod(t *testing.T) {
	const content = "0123456789"

	ctx := context.Background()
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)

	tracker := seedprogress.New(
		seedprogress.WithIdleEvictionPeriod(time.Hour),
		seedprogress.WithNow(func() time.Time { return now }),
	)

	image := tracker.Track(ctx, testImageID, testSource, int64(len(content)), nopReadSeekCloser{strings.NewReader(content)})

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
