package seedprogress

import (
	"time"
)

func WithNow(now func() time.Time) Option {
	return func(t *Tracker) {
		t.now = now
	}
}
