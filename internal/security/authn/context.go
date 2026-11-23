package authn

import "github.com/FuturFusion/operations-center/shared/api"

// CtxKey is the type used for all fields stored in the request context.
type ctxKey string

// Context keys.
const (
	// CtxAuthenticated is the authenticated field in request context. It is
	// of type bool and set to true, if the request has been successfully,
	// authenticated and false otherwise.
	CtxAuthenticated ctxKey = "authenticated"

	// CtxUsername is the username field in request context.
	CtxUsername ctxKey = "username"

	// CtxProtocol is the protocol field in request context.
	CtxProtocol ctxKey = "protocol"

	// CtxForwardedAddress is the address of the client of a request forwarded by a cluster member.
	CtxForwardedAddress ctxKey = "forwarded_address"

	// CtxForwardedCertificate is the *x509.Certificate of the client of a request forwarded by a cluster member.
	CtxForwardedCertificate ctxKey = "forwarded_certificate"
)

// Headers.
const (
	HeaderForwardedAddress     = api.HeaderForwardedAddress
	HeaderForwardedUsername    = api.HeaderForwardedUsername
	HeaderForwardedProtocol    = api.HeaderForwardedProtocol
	HeaderForwardedCertificate = api.HeaderForwardedCertificate
)
