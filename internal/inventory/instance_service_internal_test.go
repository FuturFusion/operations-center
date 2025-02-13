package inventory

import (
	"time"
)

func WithNow(now func() time.Time) InstanceServiceOption {
	return func(s *instanceService) {
		s.now = now
	}
}
