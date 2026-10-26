package authn

import (
	"net/http"
)

type Auther interface {
	Auth(w http.ResponseWriter, r *http.Request) (trusted bool, username string, protocol string, _ error)
}

type Authenticator struct {
	authers []Auther

	starting bool
}

type Option func(a *Authenticator)

// WithStarting flags the authenticator to indicate the daemon is still starting.
func WithStarting() Option {
	return func(a *Authenticator) {
		a.starting = true
	}
}

func New(authers []Auther, opts ...Option) Authenticator {
	a := Authenticator{
		authers: authers,
	}

	for _, opt := range opts {
		opt(&a)
	}

	return a
}
