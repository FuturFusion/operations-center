package unixsocket

import (
	"net/http"

	"github.com/FuturFusion/operations-center/internal/security/authn"
)

type UnixSocket struct{}

var _ authn.Auther = UnixSocket{}

func (u UnixSocket) Auth(w http.ResponseWriter, r *http.Request) (trusted bool, username string, protocol string, _ error) {
	// Local unix socket queries.
	if r.RemoteAddr == "@" && r.TLS == nil {
		return true, "", "unix", nil
	}

	return false, "", "", nil
}
