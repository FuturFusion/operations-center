package api

import (
	"net/http"

	"github.com/FuturFusion/operations-center/internal/authn/oidc"
)

func registerOIDCHandlers(router Router, oidcVerifier *oidc.Verifier) {
	if oidcVerifier == nil {
		return
	}

	router.HandleFunc("GET /oidc/login", func(w http.ResponseWriter, r *http.Request) {
		oidcVerifier.Login(w, r)
	})

	router.HandleFunc("GET /oidc/callback", func(w http.ResponseWriter, r *http.Request) {
		oidcVerifier.Callback(w, r)
	})

	router.HandleFunc("GET /oidc/logout", func(w http.ResponseWriter, r *http.Request) {
		oidcVerifier.Logout(w, r)
	})
}
