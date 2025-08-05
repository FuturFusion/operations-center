package api

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httputil"

	"github.com/FuturFusion/operations-center/internal/authz"
	internalenvironment "github.com/FuturFusion/operations-center/internal/environment"
	"github.com/FuturFusion/operations-center/internal/response"
)

type osProxyHandler struct {
	prefix string
}

func registerOSProxy(router Router, prefix string, authorizer authz.Authorizer) {
	handler := &osProxyHandler{
		prefix: prefix,
	}

	router.HandleFunc("/", response.With(handler.apiOSProxy, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanEdit)))
}

func (o *osProxyHandler) apiOSProxy(r *http.Request) response.Response {
	// Check if this is an Incus OS system.
	if !internalenvironment.IsIncusOS() {
		return response.BadRequest(errors.New("System isn't running Incus OS"))
	}

	// Prepare the proxy.
	proxy := &httputil.ReverseProxy{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", internalenvironment.IncusOSSocket)
			},
		},
		Director: func(r *http.Request) {
			r.URL.Scheme = "http"
			r.URL.Host = "incus-os"
		},
	}

	// Handle the request.
	return response.ManualResponse(func(w http.ResponseWriter) error {
		http.StripPrefix(o.prefix, proxy).ServeHTTP(w, r)

		return nil
	})
}
