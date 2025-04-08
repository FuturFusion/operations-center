package auth

// CtxKey is the type used for all fields stored in the request context.
type ctxKey string

// Context keys.
const (
	// CtxUsername is the username field in request context.
	CtxUsername ctxKey = "username"

	// CtxProtocol is the protocol field in request context.
	CtxProtocol ctxKey = "protocol"
)
