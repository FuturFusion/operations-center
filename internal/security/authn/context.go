package authn

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
)
