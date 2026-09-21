package redfish

import "time"

func WithRequestRetryDelay(delay time.Duration) Option {
	return func(r *redfish) {
		r.retryDelay = delay
	}
}
