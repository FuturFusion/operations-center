package authn

import "github.com/FuturFusion/operations-center/internal/authn/oidc"

type Authenticator struct {
	// List of trusted certificate fingerprints for client certificate based authentication.
	trustedTLSClientCertFingerprints []string

	// Verifier for OIDC based authentication.
	oidcVerifier *oidc.Verifier
}

type Option func(auth *Authenticator)

func WithTLSClientCertFingerprints(trustedTLSClientCertFingerprints []string) Option {
	return func(auth *Authenticator) {
		auth.trustedTLSClientCertFingerprints = trustedTLSClientCertFingerprints
	}
}

func WithOIDCVerifier(oidcVerifier *oidc.Verifier) Option {
	return func(auth *Authenticator) {
		auth.oidcVerifier = oidcVerifier
	}
}

func New(opts ...Option) *Authenticator {
	auth := &Authenticator{}

	for _, opt := range opts {
		opt(auth)
	}

	return auth
}
