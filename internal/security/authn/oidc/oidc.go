package oidc

import (
	"net/http"

	"github.com/zitadel/oidc/v3/pkg/oidc"

	"github.com/FuturFusion/operations-center/internal/security/authn"
	"github.com/FuturFusion/operations-center/shared/api"
)

var defaultOidcScopes = []string{oidc.ScopeOpenID, oidc.ScopeOfflineAccess}

type OIDC struct {
	oidcVerifier *Verifier
}

var _ authn.Auther = OIDC{}

func New(oidcVerifier *Verifier) OIDC {
	return OIDC{
		oidcVerifier: oidcVerifier,
	}
}

func (o OIDC) Auth(w http.ResponseWriter, r *http.Request) (trusted bool, username string, protocol string, _ error) {
	// Check for JWT token signed by an OpenID Connect provider.
	if o.oidcVerifier == nil || !o.oidcVerifier.IsRequest(r) {
		return false, "", "", nil
	}

	userName, err := o.oidcVerifier.Auth(r.Context(), w, r)
	if err != nil {
		_, ok := err.(*AuthError)
		if ok {
			// Ensure the OIDC headers are set if needed.
			_ = o.oidcVerifier.WriteHeaders(w)
		}

		return false, "", "", err
	}

	return true, userName, api.AuthenticationMethodOIDC, nil
}
