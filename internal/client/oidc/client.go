package oidc

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/lxc/incus/v6/shared/util"
	"github.com/zitadel/oidc/v3/pkg/client/rp"
	httphelper "github.com/zitadel/oidc/v3/pkg/http"
	"github.com/zitadel/oidc/v3/pkg/oidc"
	"golang.org/x/oauth2"
)

// ErrOIDCExpired is returned when the token is expired and we can't retry the request ourselves.
var ErrOIDCExpired = fmt.Errorf("OIDC token expired, please re-try the request")

// Custom transport that modifies requests to inject the audience field.
type oidcTransport struct {
	deviceAuthorizationEndpoint string
	audience                    string
}

// oidcTransport is a custom HTTP transport that injects the audience field into requests directed at the device authorization endpoint.
// RoundTrip is a method of oidcTransport that modifies the request, adds the audience parameter if appropriate, and sends it along.
func (o *oidcTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	// Don't modify the request if it's not to the device authorization endpoint, or there are no
	// URL parameters which need to be set.
	if r.URL.String() != o.deviceAuthorizationEndpoint || len(o.audience) == 0 {
		return http.DefaultTransport.RoundTrip(r)
	}

	err := r.ParseForm()
	if err != nil {
		return nil, err
	}

	if o.audience != "" {
		r.Form.Add("audience", o.audience)
	}

	// Update the body with the new URL parameters.
	body := r.Form.Encode()
	r.Body = io.NopCloser(strings.NewReader(body))
	r.ContentLength = int64(len(body))

	return http.DefaultTransport.RoundTrip(r)
}

var errRefreshAccessToken = fmt.Errorf("Failed refreshing access token")

var oidcScopes = []string{oidc.ScopeOpenID, oidc.ScopeOfflineAccess, oidc.ScopeEmail}

// OIDCClient is a structure encapsulating an HTTP client, OIDC transport, and OIDC context (token, trust tupple) for OpenID Connect (OIDC) operations.
type OIDCClient struct {
	httpClient    *http.Client
	oidcTransport *oidcTransport

	// FIXME: do we need to add a mutex to prevent concurrent changes to the oidcContext?
	// FIXME: is OIDCContext required to be a pointer?
	oidcContext     *OIDCContext
	oidcContextFile string

	authenticateOpenBrowser bool
	authenticateCallback    func(tokenURL string)
}

// OIDCContext holds the OIDC context information, which is required to
// authenticate with OIDC and to refresh the OIDC tokens if necessary.
type OIDCContext struct {
	TrustTuple OIDCTrustTuple                    `json:"trust_tuple"`
	Tokens     *oidc.Tokens[*oidc.IDTokenClaims] `json:"tokens"`
}

// OIDCTrustTuple is the issuer, clientid and the audience shared by the server
// in order to authenticate (or refresh) with the OIDC relying party.
type OIDCTrustTuple struct {
	Issuer   string `json:"issuer"`
	ClientID string `json:"client_id"`
	Audience string `json:"audience"`
}

type ClientOption func(c *OIDCClient)

func WithoutOpenBrowser() ClientOption {
	return func(c *OIDCClient) {
		c.authenticateOpenBrowser = false
	}
}

func WithAuthenticateCallback(callback func(tokenURL string)) ClientOption {
	return func(c *OIDCClient) {
		c.authenticateCallback = callback
	}
}

// NewClient constructs a new OidcClient, ensuring the token field is non-nil to prevent panics during authentication.
func NewClient(httpClient *http.Client, oidcContextFile string, opts ...ClientOption) *OIDCClient {
	client := &OIDCClient{
		oidcContext:     loadOIDCContextFromFile(oidcContextFile),
		oidcContextFile: oidcContextFile,
		httpClient:      httpClient,
		oidcTransport:   &oidcTransport{},

		authenticateOpenBrowser: true,
	}

	for _, opt := range opts {
		opt(client)
	}

	return client
}

func loadOIDCContextFromFile(oidcContextFile string) *OIDCContext {
	empty := &OIDCContext{
		Tokens: &oidc.Tokens[*oidc.IDTokenClaims]{
			Token: &oauth2.Token{},
		},
	}

	if oidcContextFile == "" {
		return empty
	}

	contents, err := os.ReadFile(oidcContextFile)
	if err != nil {
		return empty
	}

	ret := &OIDCContext{}

	err = json.Unmarshal(contents, &ret)
	if err != nil {
		return empty
	}

	return ret
}

func saveOIDCContextToFile(oidcContextFile string, oidcContext *OIDCContext) error {
	if oidcContextFile == "" {
		return nil
	}

	contents, err := json.Marshal(oidcContext)
	if err != nil {
		return err
	}

	return os.WriteFile(oidcContextFile, contents, 0o600)
}

// GetAccessToken returns the Access Token from the OidcClient's tokens, or an empty string if no tokens are present.
func (o *OIDCClient) GetAccessToken() string {
	if o.oidcContext == nil || o.oidcContext.Tokens == nil || o.oidcContext.Tokens.Token == nil {
		return ""
	}

	return o.oidcContext.Tokens.AccessToken
}

// GetOIDCTokens returns the current OIDC tokens, if any.
func (o *OIDCClient) GetOIDCTokens() *oidc.Tokens[*oidc.IDTokenClaims] {
	if o.oidcContext == nil {
		return nil
	}

	return o.oidcContext.Tokens
}

const earlyRefreshLeeway = 5 * time.Second

// Do function executes an HTTP request using the OidcClient's http client, and manages authorization by refreshing or authenticating as needed.
// If the request fails with an HTTP Unauthorized status, it attempts to refresh the access token, or perform an OIDC authentication if refresh fails.
func (o *OIDCClient) Do(req *http.Request) (*http.Response, error) {
	if o.oidcContext != nil && o.oidcContext.Tokens != nil && o.oidcContext.Tokens.Token != nil {
		// If we have a set of tokens, early refresh the access token if it is soon to be expired.
		if !o.oidcContext.Tokens.Expiry.IsZero() && time.Now().Add(earlyRefreshLeeway).After(o.oidcContext.Tokens.Expiry) {
			err := o.refresh(o.oidcContext.TrustTuple.Issuer, o.oidcContext.TrustTuple.ClientID)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: Failed to refresh OIDC access token\n")
			} else {
				err = saveOIDCContextToFile(o.oidcContextFile, o.oidcContext)
				if err != nil {
					return nil, err
				}
			}
		}
	}

	req.Header.Set("Authorization", "Bearer "+o.GetAccessToken())

	resp, err := o.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	trustTupleChanged := false

	if o.oidcContext == nil {
		o.oidcContext = &OIDCContext{
			Tokens: &oidc.Tokens[*oidc.IDTokenClaims]{
				Token: &oauth2.Token{},
			},
		}
	}

	if o.oidcContext.TrustTuple.Issuer != resp.Header.Get("X-OperationsCenter-OIDC-issuer") && resp.Header.Get("X-OperationsCenter-OIDC-issuer") != "" {
		o.oidcContext.TrustTuple.Issuer = resp.Header.Get("X-OperationsCenter-OIDC-issuer")
		trustTupleChanged = true
	}

	if o.oidcContext.TrustTuple.ClientID != resp.Header.Get("X-OperationsCenter-OIDC-clientid") && resp.Header.Get("X-OperationsCenter-OIDC-clientid") != "" {
		o.oidcContext.TrustTuple.ClientID = resp.Header.Get("X-OperationsCenter-OIDC-clientid")
		trustTupleChanged = true
	}

	if o.oidcContext.TrustTuple.Audience != resp.Header.Get("X-OperationsCenter-OIDC-audience") && resp.Header.Get("X-OperationsCenter-OIDC-audience") != "" {
		o.oidcContext.TrustTuple.Audience = resp.Header.Get("X-OperationsCenter-OIDC-audience")
		trustTupleChanged = true
	}

	if trustTupleChanged {
		err = saveOIDCContextToFile(o.oidcContextFile, o.oidcContext)
		if err != nil {
			return nil, err
		}
	}

	// Return immediately if the error is not HTTP status unauthorized.
	if resp.StatusCode != http.StatusUnauthorized {
		return resp, nil
	}

	if o.oidcContext.TrustTuple.Issuer == "" || o.oidcContext.TrustTuple.ClientID == "" {
		return resp, nil
	}

	// Refresh the token.
	err = o.refresh(o.oidcContext.TrustTuple.Issuer, o.oidcContext.TrustTuple.ClientID)
	if err != nil {
		err = o.authenticate(o.oidcContext.TrustTuple.Issuer, o.oidcContext.TrustTuple.ClientID, o.oidcContext.TrustTuple.Audience)
		if err != nil {
			return nil, err
		}
	}

	// If not dealing with something we can retry, return a clear error.
	if req.Method != "GET" && req.GetBody == nil {
		return resp, ErrOIDCExpired
	}

	// Set the new access token in the header.
	req.Header.Set("Authorization", "Bearer "+o.GetAccessToken())

	// Reset the request body.
	if req.GetBody != nil {
		body, err := req.GetBody()
		if err != nil {
			return nil, err
		}

		req.Body = body
	}

	resp, err = o.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	err = saveOIDCContextToFile(o.oidcContextFile, o.oidcContext)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

// getProvider initializes a new OpenID Connect Relying Party for a given issuer and clientID.
// The function also creates a secure CookieHandler with random encryption and hash keys, and applies a series of configurations on the Relying Party.
func (o *OIDCClient) getProvider(issuer string, clientID string) (rp.RelyingParty, error) {
	hashKey := make([]byte, 16)
	encryptKey := make([]byte, 16)

	_, err := rand.Read(hashKey)
	if err != nil {
		return nil, err
	}

	_, err = rand.Read(encryptKey)
	if err != nil {
		return nil, err
	}

	cookieHandler := httphelper.NewCookieHandler(hashKey, encryptKey, httphelper.WithUnsecure())
	options := []rp.Option{
		rp.WithCookieHandler(cookieHandler),
		rp.WithVerifierOpts(rp.WithIssuedAtOffset(5 * time.Second)),
		rp.WithPKCE(cookieHandler),
		rp.WithHTTPClient(o.httpClient),
	}

	provider, err := rp.NewRelyingPartyOIDC(context.TODO(), issuer, clientID, "", "", oidcScopes, options...)
	if err != nil {
		return nil, err
	}

	return provider, nil
}

// refresh attempts to refresh the OpenID Connect access token for the client using the refresh token.
// If no token is present or the refresh token is empty, it returns an error. If successful, it updates the access token and other relevant token fields.
func (o *OIDCClient) refresh(issuer string, clientID string) error {
	if o.oidcContext == nil || o.oidcContext.Tokens == nil || o.oidcContext.Tokens.Token == nil || o.oidcContext.Tokens.RefreshToken == "" {
		return errRefreshAccessToken
	}

	provider, err := o.getProvider(issuer, clientID)
	if err != nil {
		return errRefreshAccessToken
	}

	oauthTokens, err := rp.RefreshTokens[*oidc.IDTokenClaims](context.TODO(), provider, o.oidcContext.Tokens.RefreshToken, "", "")
	if err != nil {
		return errRefreshAccessToken
	}

	o.oidcContext.Tokens.AccessToken = oauthTokens.AccessToken
	o.oidcContext.Tokens.TokenType = oauthTokens.TokenType
	o.oidcContext.Tokens.Expiry = oauthTokens.Expiry

	if oauthTokens.RefreshToken != "" {
		o.oidcContext.Tokens.RefreshToken = oauthTokens.RefreshToken
	}

	return nil
}

// authenticate initiates the OpenID Connect device flow authentication process for the client.
// It presents a user code for the end user to input in the device that has web access and waits for them to complete the authentication,
// subsequently updating the client's tokens upon successful authentication.
func (o *OIDCClient) authenticate(issuer string, clientID string, audience string) error {
	tokenURL, resp, provider, err := o.getTokenURL(issuer, clientID, audience)
	if err != nil {
		return err
	}

	fmt.Printf("URL: %s\n", tokenURL)
	fmt.Printf("Code: %s\n\n", resp.UserCode)

	if o.authenticateOpenBrowser {
		_ = util.OpenBrowser(tokenURL)
	}

	if o.authenticateCallback != nil {
		o.authenticateCallback(tokenURL)
	}

	return o.WaitForToken(resp, provider)
}

func (o *OIDCClient) FetchNewIncusTokenURL(req *http.Request) (string, *oidc.DeviceAuthorizationResponse, rp.RelyingParty, error) {
	resp, err := o.httpClient.Do(req)
	if err != nil {
		return "", nil, nil, err
	}

	defer resp.Body.Close()

	// Return immediately if the error is not HTTP status unauthorized.
	if resp.StatusCode != http.StatusUnauthorized {
		return "", nil, nil, fmt.Errorf("Status != unauthorized")
	}

	issuer := resp.Header.Get("X-Incus-OIDC-issuer")
	clientID := resp.Header.Get("X-Incus-OIDC-clientid")
	audience := resp.Header.Get("X-Incus-OIDC-audience")

	if issuer == "" || clientID == "" {
		return "", nil, nil, fmt.Errorf("Missing issuer or clientID")
	}

	// Request a new token.
	return o.getTokenURL(issuer, clientID, audience)
}

func (o *OIDCClient) getTokenURL(issuer string, clientID string, audience string) (string, *oidc.DeviceAuthorizationResponse, rp.RelyingParty, error) {
	// Store the old transport and restore it in the end.
	oldTransport := o.httpClient.Transport
	o.oidcTransport.audience = audience
	o.httpClient.Transport = o.oidcTransport

	defer func() {
		o.httpClient.Transport = oldTransport
	}()

	provider, err := o.getProvider(issuer, clientID)
	if err != nil {
		return "", nil, nil, err
	}

	o.oidcTransport.deviceAuthorizationEndpoint = provider.GetDeviceAuthorizationEndpoint()

	resp, err := rp.DeviceAuthorization(context.TODO(), oidcScopes, provider, nil)
	if err != nil {
		return "", nil, nil, err
	}

	u, _ := url.Parse(resp.VerificationURIComplete)

	return u.String(), resp, provider, nil
}

func (o *OIDCClient) WaitForToken(resp *oidc.DeviceAuthorizationResponse, provider rp.RelyingParty) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT)
	defer stop()

	token, err := rp.DeviceAccessToken(ctx, resp.DeviceCode, time.Duration(resp.Interval)*time.Second, provider)
	if err != nil {
		return err
	}

	if o.oidcContext == nil {
		o.oidcContext = &OIDCContext{
			Tokens: &oidc.Tokens[*oidc.IDTokenClaims]{
				Token: &oauth2.Token{},
			},
		}
	}

	if o.oidcContext.Tokens == nil {
		o.oidcContext.Tokens = &oidc.Tokens[*oidc.IDTokenClaims]{
			Token: &oauth2.Token{},
		}
	}

	if o.oidcContext.Tokens.Token == nil {
		o.oidcContext.Tokens.Token = &oauth2.Token{}
	}

	o.oidcContext.Tokens.Expiry = time.Now().UTC().Add(time.Duration(token.ExpiresIn) * time.Second)
	o.oidcContext.Tokens.ExpiresIn = int64(token.ExpiresIn)
	o.oidcContext.Tokens.IDToken = token.IDToken
	o.oidcContext.Tokens.AccessToken = token.AccessToken
	o.oidcContext.Tokens.TokenType = token.TokenType

	if token.RefreshToken != "" {
		o.oidcContext.Tokens.RefreshToken = token.RefreshToken
	}

	return nil
}
