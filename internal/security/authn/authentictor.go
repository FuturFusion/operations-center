package authn

import (
	"net/http"
	"sync/atomic"
)

type Auther interface {
	Auth(w http.ResponseWriter, r *http.Request) (trusted bool, username string, protocol string, _ error)
}

// Authenticator authenticates requests. Copies of an Authenticator share its configuration, which Replace swaps while requests are served.
type Authenticator struct {
	current *atomic.Pointer[authenticatorConfig]
}

type authenticatorConfig struct {
	authers []Auther

	starting bool
}

type Option func(a *authenticatorConfig)

// WithStarting flags the authenticator to indicate the daemon is still starting.
func WithStarting() Option {
	return func(a *authenticatorConfig) {
		a.starting = true
	}
}

func New(authers []Auther, opts ...Option) Authenticator {
	cfg := &authenticatorConfig{
		authers: authers,
	}

	for _, opt := range opts {
		opt(cfg)
	}

	a := Authenticator{current: &atomic.Pointer[authenticatorConfig]{}}
	a.current.Store(cfg)

	return a
}

// Replace makes a and all its copies authenticate the next requests like other.
func (a Authenticator) Replace(other Authenticator) {
	a.current.Store(other.current.Load())
}
