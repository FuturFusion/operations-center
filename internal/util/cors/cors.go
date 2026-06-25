package cors

import (
	"net/http"
)

type CORSConfig struct {
	AllowedOrigins   string
	AllowedMethods   string
	AllowedHeaders   string
	AllowCredentials bool
}

func Handler(config CORSConfig) func(next http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			allowedOrigin := config.AllowedOrigins
			origin := r.Header.Get("Origin")
			if allowedOrigin != "" && origin != "" {
				w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
			}

			allowedMethods := config.AllowedMethods
			if allowedMethods != "" && origin != "" {
				w.Header().Set("Access-Control-Allow-Methods", allowedMethods)
			}

			allowedHeaders := config.AllowedHeaders
			if allowedHeaders != "" && origin != "" {
				w.Header().Set("Access-Control-Allow-Headers", allowedHeaders)
			}

			if config.AllowCredentials {
				r.Header.Set("Access-Control-Allow-Credentials", "true")
			}

			next(w, r)
		}
	}
}
