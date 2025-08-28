package authn

import (
	"net/http"
)

type Auther interface {
	Auth(w http.ResponseWriter, r *http.Request) (trusted bool, username string, protocol string, _ error)
}

type Authenticator struct {
	authers []Auther
}

func New(authers []Auther) Authenticator {
	return Authenticator{
		authers: authers,
	}
}
