package api

// Headers a cluster member sets when it forwards a request to another member.
const (
	// HeaderForwardedAddress is the remote address of the client of the forwarded request.
	HeaderForwardedAddress = "X-OperationsCenter-forwarded-address"

	// HeaderForwardedUsername is the username of the client of the forwarded request.
	HeaderForwardedUsername = "X-OperationsCenter-forwarded-username"

	// HeaderForwardedProtocol is the authentication protocol of the client of the forwarded request.
	HeaderForwardedProtocol = "X-OperationsCenter-forwarded-protocol"

	// HeaderForwardedCertificate is the base64 encoded DER certificate of the client of the forwarded request.
	HeaderForwardedCertificate = "X-OperationsCenter-forwarded-certificate"
)

// ForwardedHeaders are set by a cluster member forwarding a request and can not be set by a client.
var ForwardedHeaders = []string{HeaderForwardedAddress, HeaderForwardedUsername, HeaderForwardedProtocol, HeaderForwardedCertificate}
