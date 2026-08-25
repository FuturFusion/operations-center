// Package seedprogress records how much of a seed image the sources streaming
// it have read.
package seedprogress

import (
	"context"
	"io"
	"log/slog"
	"maps"
	"sync"
	"time"

	"github.com/FuturFusion/operations-center/internal/provisioning"
)

// defaultIdleEvictionPeriod is how long a record without any activity is kept.
// An installation takes 30 to 60 minutes, so the records have to outlive it by
// a fair margin.
const defaultIdleEvictionPeriod = 2 * time.Hour

// logPeriod is how often the progress of an image being read is logged. The
// reads themselves are far too frequent to log one by one.
const logPeriod = 10 * time.Second

// Tracker records the reads served from the seed images it wraps.
type Tracker struct {
	idleEvictionPeriod time.Duration
	now                func() time.Time

	mu      sync.Mutex
	entries map[entryKey]*entry
}

var _ provisioning.SeedImageProgressPort = &Tracker{}

// entryKey is what tells the readers of the seed images apart. Several servers
// can install from the same image at the same time.
type entryKey struct {
	imageID provisioning.SeedImageID
	source  string
}

type entry struct {
	size          int64
	bytesServed   int64
	highestOffset int64
	firstRead     time.Time
	lastRead      time.Time
	requestCount  int

	// lastActivity is the last time the entry has been touched at all, which
	// includes a request not having read anything.
	lastActivity time.Time

	// lastLog is the last time the progress of the entry has been logged.
	lastLog time.Time
}

type Option func(*Tracker)

// WithIdleEvictionPeriod sets how long a record without any activity is kept.
func WithIdleEvictionPeriod(period time.Duration) Option {
	return func(t *Tracker) {
		t.idleEvictionPeriod = period
	}
}

func New(opts ...Option) *Tracker {
	tracker := &Tracker{
		idleEvictionPeriod: defaultIdleEvictionPeriod,
		now:                time.Now,
		entries:            map[entryKey]*entry{},
	}

	for _, opt := range opts {
		opt(tracker)
	}

	return tracker
}

// Track wraps content, so that the reads served from it are recorded as
// progress of source reading the image identified by imageID.
func (t *Tracker) Track(ctx context.Context, imageID provisioning.SeedImageID, source string, size int64, content io.ReadSeekCloser) io.ReadSeekCloser {
	key := entryKey{
		imageID: imageID,
		source:  source,
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	t.evict()

	e := t.entry(key)
	e.size = size
	e.lastActivity = t.now()

	return &trackedContent{
		ctx:     ctx,
		tracker: t,
		key:     key,
		content: content,
	}
}

// Get returns the progress recorded for imageID being read by source and
// reports whether anything has been recorded at all.
func (t *Tracker) Get(_ context.Context, imageID provisioning.SeedImageID, source string) (provisioning.SeedImageProgress, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.evict()

	e, ok := t.entries[entryKey{imageID: imageID, source: source}]
	if !ok {
		return provisioning.SeedImageProgress{}, false
	}

	return e.progress(entryKey{imageID: imageID, source: source}), true
}

// record accounts for n bytes having been read at offset.
func (t *Tracker) record(ctx context.Context, key entryKey, offset int64, n int, isRequestStart bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := t.now()

	e := t.entry(key)
	e.bytesServed += int64(n)

	// A range request can read anywhere in the image, so the highest offset
	// reached never regresses.
	e.highestOffset = max(e.highestOffset, offset+int64(n))

	if e.firstRead.IsZero() {
		e.firstRead = now
	}

	e.lastRead = now
	e.lastActivity = now

	if isRequestStart {
		e.requestCount++
	}

	if now.Sub(e.lastLog) < logPeriod {
		return
	}

	e.lastLog = now

	percentCompleted := 0.0

	if e.size > 0 {
		percentCompleted = min(float64(e.highestOffset)/float64(e.size)*100, 100)
	}

	slog.DebugContext(
		ctx, "Seed image read progress",
		slog.String("image_id", key.imageID.String()),
		slog.String("source", key.source),
		slog.Float64("percent_complete", percentCompleted),
		slog.Int64("bytes_served", e.bytesServed),
		slog.Int64("highest_offset", e.highestOffset),
		slog.Int64("size", e.size),
		slog.Int("request_count", e.requestCount),
	)
}

// progress returns what has been recorded for the entry. The caller holds the
// lock.
func (e *entry) progress(key entryKey) provisioning.SeedImageProgress {
	return provisioning.SeedImageProgress{
		ImageID:       key.imageID,
		Source:        key.source,
		Size:          e.size,
		BytesServed:   e.bytesServed,
		HighestOffset: e.highestOffset,
		FirstRead:     e.firstRead,
		LastRead:      e.lastRead,
		RequestCount:  e.requestCount,
	}
}

// entry returns the entry for key, adding it if it does not exist yet. The
// caller holds the lock.
func (t *Tracker) entry(key entryKey) *entry {
	e, ok := t.entries[key]
	if !ok {
		e = &entry{}
		t.entries[key] = e
	}

	return e
}

// evict drops the records without any activity for the idle eviction period, so
// that a long running daemon does not accumulate one record per source and
// image. The caller holds the lock.
func (t *Tracker) evict() {
	deadline := t.now().Add(-t.idleEvictionPeriod)

	maps.DeleteFunc(t.entries, func(_ entryKey, e *entry) bool {
		return e.lastActivity.Before(deadline)
	})
}

// trackedContent reports the reads served from one image to one source.
type trackedContent struct {
	// ctx is the context of the request being served. It is only used to log
	// the progress along with what identifies the request.
	ctx     context.Context
	tracker *Tracker
	key     entryKey
	content io.ReadSeekCloser

	pos     int64
	hasRead bool
}

func (c *trackedContent) Read(p []byte) (int, error) {
	n, err := c.content.Read(p)
	if n > 0 {
		c.tracker.record(c.ctx, c.key, c.pos, n, !c.hasRead)
		c.hasRead = true
		c.pos += int64(n)
	}

	return n, err
}

func (c *trackedContent) Seek(offset int64, whence int) (int64, error) {
	pos, err := c.content.Seek(offset, whence)
	if err == nil {
		c.pos = pos
	}

	return pos, err
}

func (c *trackedContent) Close() error {
	return c.content.Close()
}
