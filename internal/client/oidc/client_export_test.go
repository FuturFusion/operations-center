package oidc

// WithoutOpenBrowser disables the automated opening of the web browser to perform
// the user's authentication flow with the OIDC rely party. In this case
// the client will just print the URL and the code to the console.
//
// This is helpful in headless scenarios or in tests.
func WithoutOpenBrowser() ClientOption {
	return func(c *OIDCClient) {
		c.authenticateOpenBrowser = false
	}
}

// WithAuthenticateCallback takes a callback function, which is called after
// the authentication flow has been started and the token has been received.
// The URL to confirm the token is passed by to the callback function and
// can be queried in the callback allowing for automation.
//
// This is mainly useful in tests, e.g. with mini-oidc, which does not do any
// authentication and just issues the tokens when the URL is queried.
func WithAuthenticateCallback(callback func(tokenURL string)) ClientOption {
	return func(c *OIDCClient) {
		c.authenticateCallback = callback
	}
}
