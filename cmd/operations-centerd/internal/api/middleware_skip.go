package api

import (
	"net/http"
)

// unless wraps a middleware. If condition evaluates to true, the wrapped middleware
// is skipped and therefore not executed. Otherwise the wrapped middleware is executed.
func unless(middleware MiddlewareFunc, condition func(r *http.Request) bool) MiddlewareFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if condition(r) {
				next(w, r)
				return
			}

			middleware(next)(w, r)
		}
	}
}
