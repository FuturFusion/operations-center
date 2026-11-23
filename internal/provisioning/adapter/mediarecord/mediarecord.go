// Package mediarecord records the generated media in the database, together
// with the cluster member storing them and the end of their lease.
package mediarecord

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/FuturFusion/operations-center/internal/cluster/memberfiles"
	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/provisioning"
)

// defaultCacheTTL limits how long a record read from the database is used, so a
// media read in many ranges is not looked up for every one of them.
const defaultCacheTTL = 30 * time.Second

// Records records the media of one type.
type Records struct {
	repo      provisioning.MediaRepo
	targets   memberfiles.Targets
	mediaType provisioning.MediaType
	ttl       time.Duration
	now       func() time.Time

	cacheTTL time.Duration
	cache    *recordCache
}

// Option configures the records.
type Option func(r *Records)

// WithCacheTTL sets how long a record read from the database is used.
func WithCacheTTL(ttl time.Duration) Option {
	return func(r *Records) {
		r.cacheTTL = ttl
	}
}

// recordCache holds the records read recently.
type recordCache struct {
	mu      sync.Mutex
	entries map[string]cachedRecord
}

type cachedRecord struct {
	media   provisioning.Media
	fetched time.Time
}

// New returns the records of the media of mediaType, whose lease lasts for ttl.
func New(repo provisioning.MediaRepo, targets memberfiles.Targets, mediaType provisioning.MediaType, ttl time.Duration, opts ...Option) Records {
	r := Records{repo: repo, targets: targets, mediaType: mediaType, ttl: ttl, now: time.Now, cacheTTL: defaultCacheTTL, cache: &recordCache{entries: map[string]cachedRecord{}}}

	for _, opt := range opts {
		opt(&r)
	}

	return r
}

// get returns the record of the media name, from the cache if it has been read recently.
func (r Records) get(ctx context.Context, name string) (*provisioning.Media, error) {
	now := r.now()

	r.cache.mu.Lock()
	entry, ok := r.cache.entries[name]
	r.cache.mu.Unlock()

	if ok && now.Sub(entry.fetched) < r.cacheTTL {
		media := entry.media
		return &media, nil
	}

	media, err := r.repo.GetByTypeAndName(ctx, r.mediaType, name)
	if errors.Is(err, domain.ErrNotFound) {
		r.forget(name)
	}

	if err != nil {
		return nil, err
	}

	r.remember(*media)

	return media, nil
}

func (r Records) remember(media provisioning.Media) {
	r.cache.mu.Lock()
	defer r.cache.mu.Unlock()

	r.cache.entries[media.Name] = cachedRecord{media: media, fetched: r.now()}
}

func (r Records) forget(name string) {
	r.cache.mu.Lock()
	defer r.cache.mu.Unlock()

	delete(r.cache.entries, name)
}

// Holder returns the record of the media name and the member storing it, ErrNotFound if it is not recorded or its lease ended.
// A record read less than cacheTTL ago is not read again.
func (r Records) Holder(ctx context.Context, name string) (*provisioning.Media, memberfiles.Member, error) {
	media, err := r.get(ctx, name)
	if err != nil {
		return nil, memberfiles.Member{}, err
	}

	// The holder might have extended a lease, which ended according to the cache.
	if !media.ExpiresAt.After(r.now()) {
		r.forget(name)

		media, err = r.get(ctx, name)
		if err != nil {
			return nil, memberfiles.Member{}, err
		}
	}

	if !media.ExpiresAt.After(r.now()) {
		return nil, memberfiles.Member{}, domain.NewErrorf(domain.ErrNotFound, "", "The media %q has expired", name)
	}

	member, err := r.targets.Member(ctx, media.MemberID)
	if err != nil {
		return nil, memberfiles.Member{}, err
	}

	return media, member, nil
}

// Record records this member as the one storing the media name with a new lease.
func (r Records) Record(ctx context.Context, name string, config any) error {
	localID, err := r.targets.LocalID()
	if err != nil {
		return err
	}

	data, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("Failed to encode the config of the media %q: %w", name, err)
	}

	media := provisioning.Media{
		Type:      r.mediaType,
		Name:      name,
		MemberID:  localID,
		Config:    data,
		ExpiresAt: r.now().Add(r.ttl),
	}

	_, err = r.repo.GetByTypeAndName(ctx, r.mediaType, name)
	if errors.Is(err, domain.ErrNotFound) {
		_, err = r.repo.Create(ctx, media)
	} else if err == nil {
		err = r.repo.Update(ctx, media)
	}

	if err != nil {
		return err
	}

	r.remember(media)

	return nil
}

// Renew extends the lease of media, once half of it has passed. The record is
// read again before, so a lease is never extended for a member, which no longer
// stores the media.
func (r Records) Renew(ctx context.Context, media provisioning.Media) error {
	now := r.now()
	if !media.LeaseDue(now, r.ttl) {
		return nil
	}

	current, err := r.repo.GetByTypeAndName(ctx, r.mediaType, media.Name)
	if errors.Is(err, domain.ErrNotFound) {
		r.forget(media.Name)
		return nil
	}

	if err != nil {
		return err
	}

	if current.MemberID != media.MemberID {
		r.remember(*current)
		return nil
	}

	current.ExpiresAt = now.Add(r.ttl)

	err = r.repo.Update(ctx, *current)
	if err != nil {
		return err
	}

	r.remember(*current)

	return nil
}

// RenewLocal extends the lease of the media name read from this member, also after it ended.
func (r Records) RenewLocal(ctx context.Context, name string) error {
	media, err := r.get(ctx, name)
	if errors.Is(err, domain.ErrNotFound) {
		return nil
	}

	if err != nil {
		return err
	}

	localID, err := r.targets.LocalID()
	if err != nil {
		return err
	}

	if media.MemberID != localID {
		return nil
	}

	return r.Renew(ctx, *media)
}

// Local returns the records of the media stored by this member.
func (r Records) Local(ctx context.Context) (provisioning.Medias, error) {
	localID, err := r.targets.LocalID()
	if err != nil {
		return nil, err
	}

	return r.repo.GetAllWithFilter(ctx, provisioning.MediaFilter{Type: &r.mediaType, MemberID: &localID})
}

// All returns the records of all media of the type.
func (r Records) All(ctx context.Context) (provisioning.Medias, error) {
	return r.repo.GetAllWithFilter(ctx, provisioning.MediaFilter{Type: &r.mediaType})
}

// Expired reports whether the lease of media ended.
func (r Records) Expired(media provisioning.Media) bool {
	return !media.ExpiresAt.After(r.now())
}

// Delete deletes the record of the media name, it is not an error if there is none.
func (r Records) Delete(ctx context.Context, name string) error {
	r.forget(name)

	err := r.repo.DeleteByTypeAndName(ctx, r.mediaType, name)
	if errors.Is(err, domain.ErrNotFound) {
		return nil
	}

	return err
}
