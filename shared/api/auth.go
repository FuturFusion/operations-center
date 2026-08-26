package api

const (
	// AuthenticationMethodTLS is the default authentication method for interacting with Incus remotely.
	AuthenticationMethodTLS = "tls"

	// AuthenticationMethodOIDC is a token based authentication method.
	AuthenticationMethodOIDC = "oidc"

	// AuthenticationMethodUnix is the authentication method for local connections through the unix socket.
	AuthenticationMethodUnix = "unix"

	// AuthenticationUntrusted is reported for a client, which is not authenticated.
	AuthenticationUntrusted = "untrusted"
)
