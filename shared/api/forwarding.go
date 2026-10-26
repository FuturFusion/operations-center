package api

// Headers a cluster member sets when it forwards a request to another member.
const (
	// HeaderForwardedAddress is the remote address of the client of the forwarded request.
	HeaderForwardedAddress = "X-OperationsCenter-forwarded-address"

	// HeaderForwardedUsername is the username of the client of the forwarded request.
	HeaderForwardedUsername = "X-OperationsCenter-forwarded-username"

	// HeaderForwardedProtocol is the authentication protocol of the client of the forwarded request.
	HeaderForwardedProtocol = "X-OperationsCenter-forwarded-protocol"
)
