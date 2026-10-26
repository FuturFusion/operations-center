package authn

import (
	"net/http"
)

type Auther interface {
	Auth(w http.ResponseWriter, r *http.Request) (trusted bool, username string, protocol string, _ error)
}

type Authenticator struct {
	authers []Auther

	checkTrustedForward func(r *http.Request) bool

	starting bool
}

type Option func(a *Authenticator)

// WithTrustedForward sets the trusted forwarded request checker for requests forwarded from cluster members.
func WithTrustedForward(checkTrustedForward func(r *http.Request) bool) Option {
	return func(a *Authenticator) {
		a.checkTrustedForward = checkTrustedForward
	}
}

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
