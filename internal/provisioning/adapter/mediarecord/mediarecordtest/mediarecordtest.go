// Package mediarecordtest provides media records kept in memory for tests.
package mediarecordtest

import (
	"context"
	"sync"
	"time"

	"github.com/FuturFusion/operations-center/internal/cluster/memberfiles"
	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/provisioning/adapter/mediarecord"
)

// Repo is a media repo kept in memory.
type Repo struct {
	mu     sync.Mutex
	medias map[string]provisioning.Media
	nextID int64
}

var _ provisioning.MediaRepo = &Repo{}

// NewRepo returns an empty media repo.
func NewRepo() *Repo {
	return &Repo{medias: map[string]provisioning.Media{}}
}

// Records returns the records of mediaType of a standalone member kept in repo.
// They are not cached, so changes of repo by a test are seen right away.
func Records(repo *Repo, mediaType provisioning.MediaType, ttl time.Duration) mediarecord.Records {
	return mediarecord.New(repo, memberfiles.NewTargets(func() memberfiles.Cluster { return nil }), mediaType, ttl, mediarecord.WithCacheTTL(0))
}

// ClusterRecords returns the records of mediaType of the local member of cluster kept in repo, not cached.
func ClusterRecords(repo *Repo, cluster memberfiles.Cluster, mediaType provisioning.MediaType, ttl time.Duration) mediarecord.Records {
	return mediarecord.New(repo, memberfiles.NewTargets(func() memberfiles.Cluster { return cluster }), mediaType, ttl, mediarecord.WithCacheTTL(0))
}

func key(mediaType provisioning.MediaType, name string) string {
	return string(mediaType) + "/" + name
}

// Create implements [provisioning.MediaRepo].
func (r *Repo) Create(_ context.Context, newMedia provisioning.Media) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	_, ok := r.medias[key(newMedia.Type, newMedia.Name)]
	if ok {
		return 0, domain.NewErrorf(domain.ErrConstraintViolation, "", "Media already exists")
	}

	r.nextID++
	newMedia.ID = r.nextID
	r.medias[key(newMedia.Type, newMedia.Name)] = newMedia

	return newMedia.ID, nil
}

// GetAllWithFilter implements [provisioning.MediaRepo].
func (r *Repo) GetAllWithFilter(_ context.Context, filter provisioning.MediaFilter) (provisioning.Medias, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var medias provisioning.Medias
	for _, media := range r.medias {
		if filter.Type != nil && media.Type != *filter.Type {
			continue
		}

		if filter.Name != nil && media.Name != *filter.Name {
			continue
		}

		if filter.MemberID != nil && media.MemberID != *filter.MemberID {
			continue
		}

		medias = append(medias, media)
	}

	return medias, nil
}

// GetByTypeAndName implements [provisioning.MediaRepo].
func (r *Repo) GetByTypeAndName(_ context.Context, mediaType provisioning.MediaType, name string) (*provisioning.Media, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	media, ok := r.medias[key(mediaType, name)]
	if !ok {
		return nil, domain.NewErrorf(domain.ErrNotFound, "", "Media not found")
	}

	return &media, nil
}

// Update implements [provisioning.MediaRepo].
func (r *Repo) Update(_ context.Context, media provisioning.Media) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	current, ok := r.medias[key(media.Type, media.Name)]
	if !ok {
		return domain.NewErrorf(domain.ErrNotFound, "", "Media not found")
	}

	media.ID = current.ID
	r.medias[key(media.Type, media.Name)] = media

	return nil
}

// DeleteByTypeAndName implements [provisioning.MediaRepo].
func (r *Repo) DeleteByTypeAndName(_ context.Context, mediaType provisioning.MediaType, name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	_, ok := r.medias[key(mediaType, name)]
	if !ok {
		return domain.NewErrorf(domain.ErrNotFound, "", "Media not found")
	}

	delete(r.medias, key(mediaType, name))

	return nil
}

// Expire ends the lease of all media.
func (r *Repo) Expire() {
	r.mu.Lock()
	defer r.mu.Unlock()

	for k, media := range r.medias {
		media.ExpiresAt = time.Time{}
		r.medias[k] = media
	}
}

// SetMember moves all media to the member memberID.
func (r *Repo) SetMember(memberID int64) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for k, media := range r.medias {
		media.MemberID = memberID
		r.medias[k] = media
	}
}
