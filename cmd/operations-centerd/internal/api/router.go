package api

import (
	"fmt"
	"net/http"
	"strings"
)

type Router interface {
	Handle(pattern string, handler http.Handler)
	HandleFunc(pattern string, handler http.HandlerFunc)

	SubGroup(basePath string) Router
}

type router struct {
	mux      *http.ServeMux
	basePath string
}

var _ Router = &router{}

func getFullPath(pattern, basePath string) string {
	method, rest := "", pattern
	i := strings.IndexAny(pattern, " \t")
	if i >= 0 {
		// We keep the space with the method.
		method, rest = pattern[:i+1], strings.TrimLeft(pattern[i+1:], " \t")
	}

	// Remove tailing slash for root resources, if in a sub group.
	if basePath != "" && rest == "/{$}" {
		rest = ""
	}

	return fmt.Sprintf("%s%s%s", method, basePath, rest)
}

func (r *router) Handle(pattern string, handler http.Handler) {
	fullPath := getFullPath(pattern, r.basePath)
	r.mux.Handle(fullPath, handler)
}

func (r *router) HandleFunc(pattern string, handler http.HandlerFunc) {
	fullPath := getFullPath(pattern, r.basePath)
	r.mux.HandleFunc(fullPath, handler)
}

func (r *router) SubGroup(basePath string) Router {
	rCopy := *r

	rCopy.basePath = fmt.Sprintf("%s%s", rCopy.basePath, basePath)

	return &rCopy
}

// newRouter returns a router implementing the Router interface, which allows
// to define sub routers and attach handlers and handler functions.
func newRouter(mux *http.ServeMux) Router {
	return &router{
		mux: mux,
	}
}
