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
	env    interface{ IsIncusOS() bool }
}

func registerOSProxy(router Router, prefix string, authorizer *authz.Authorizer, env interface{ IsIncusOS() bool }) {
	handler := &osProxyHandler{
		prefix: prefix,
		env:    env,
	}

	router.HandleFunc("/", response.With(handler.apiOSProxy, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanEdit)))
}

func (o *osProxyHandler) apiOSProxy(r *http.Request) response.Response {
	// Check if this is an IncusOS system.
	if !o.env.IsIncusOS() {
		return response.BadRequest(errors.New("System isn't running IncusOS"))
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

	// Allow IncusOS to adjust the returned paths to the prefix used by the proxy.
	r.Header.Add("X-IncusOS-Proxy", o.prefix)

	// Handle the request.
	return response.ManualResponse(func(w http.ResponseWriter) error {
		http.StripPrefix(o.prefix, proxy).ServeHTTP(w, r)

		return nil
	})
}
