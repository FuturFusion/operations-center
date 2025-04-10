// Inspired by https://github.com/DBarbosaDev/supermuxer, MIT License

package api

import (
	"fmt"
	"net/http"
	"slices"
	"strings"
)

type MiddlewareFunc func(next http.HandlerFunc) http.HandlerFunc

type Router interface {
	Handle(pattern string, handler http.Handler)
	HandleFunc(pattern string, handler http.HandlerFunc)

	AddMiddlewares(middleware ...MiddlewareFunc) Router

	SubGroup(basePath string) Router
}

type router struct {
	mux         *http.ServeMux
	basePath    string
	middlewares []MiddlewareFunc
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

func handlerWithMiddlewares(handler http.HandlerFunc, middlewares []MiddlewareFunc) http.HandlerFunc {
	if len(middlewares) <= 0 {
		return handler
	}

	next := handler

	for _, middleware := range slices.Backward(middlewares) {
		next = middleware(next)
	}

	return next
}

func (r *router) Handle(pattern string, handler http.Handler) {
	fullPath := getFullPath(pattern, r.basePath)
	wrappedHandler := handlerWithMiddlewares(handler.ServeHTTP, r.middlewares)
	r.mux.Handle(fullPath, wrappedHandler)
}

func (r *router) HandleFunc(pattern string, handlerFunc http.HandlerFunc) {
	fullPath := getFullPath(pattern, r.basePath)
	wrappedHandler := handlerWithMiddlewares(handlerFunc, r.middlewares)
	r.mux.HandleFunc(fullPath, wrappedHandler)
}

func (r *router) AddMiddlewares(middlewares ...MiddlewareFunc) Router {
	r.middlewares = append(r.middlewares, middlewares...)
	return r
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
		mux:         mux,
		middlewares: []MiddlewareFunc{},
	}
}
