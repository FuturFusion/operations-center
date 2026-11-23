package mediarecord

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/cluster/memberfiles"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/provisioning/repo/mock"
)

func TestRecords_cache(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	stored := provisioning.Media{Type: provisioning.MediaTypeSeedImage, Name: "image", MemberID: 1, ExpiresAt: now.Add(2 * time.Hour)}

	var reads, updates atomic.Int64
	repo := &mock.MediaRepoMock{
		GetByTypeAndNameFunc: func(ctx context.Context, mediaType provisioning.MediaType, name string) (*provisioning.Media, error) {
			reads.Add(1)
			media := stored

			return &media, nil
		},
		UpdateFunc: func(ctx context.Context, media provisioning.Media) error {
			updates.Add(1)
			stored = media

			return nil
		},
	}

	records := New(repo, memberfiles.NewTargets(func() memberfiles.Cluster { return nil }), provisioning.MediaTypeSeedImage, 2*time.Hour)
	records.now = func() time.Time { return now }

	// A media read in many ranges is looked up once per cache period.
	for range 100 {
		_, holder, err := records.Holder(t.Context(), "image")
		require.NoError(t, err)
		require.True(t, holder.Local)
		require.NoError(t, records.RenewLocal(t.Context(), "image"))
	}

	require.Equal(t, int64(1), reads.Load())
	require.Zero(t, updates.Load(), "A lease is only extended once half of it passed")

	// Once half of the lease passed, it is extended after reading the record again.
	now = now.Add(90 * time.Minute)
	require.NoError(t, records.RenewLocal(t.Context(), "image"))
	require.Equal(t, int64(1), updates.Load())
	require.True(t, stored.ExpiresAt.Equal(now.Add(2*time.Hour)))

	// A media taken over by another member meanwhile is not extended for this one.
	stored.MemberID = 2
	stored.ExpiresAt = now.Add(30 * time.Minute)
	now = now.Add(time.Minute)

	cached := stored
	cached.MemberID = 1
	records.remember(cached)

	require.NoError(t, records.Renew(t.Context(), cached))
	require.Equal(t, int64(1), updates.Load())
	require.Equal(t, int64(2), stored.MemberID)
}
