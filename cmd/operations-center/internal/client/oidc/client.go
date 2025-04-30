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

// OidcClient is a structure encapsulating an HTTP client, OIDC transport, and a token for OpenID Connect (OIDC) operations.
type OidcClient struct {
	httpClient    *http.Client
	oidcTransport *oidcTransport
	tokens        *oidc.Tokens[*oidc.IDTokenClaims]
	tokensFile    string
}

// NewClient constructs a new OidcClient, ensuring the token field is non-nil to prevent panics during authentication.
func NewClient(httpClient *http.Client, tokensFile string) *OidcClient {
	client := OidcClient{
		tokens:        loadTokensFromFile(tokensFile),
		tokensFile:    tokensFile,
		httpClient:    httpClient,
		oidcTransport: &oidcTransport{},
	}

	// Ensure client.tokens is never nil otherwise authenticate() will panic.
	if client.tokens == nil {
		client.tokens = &oidc.Tokens[*oidc.IDTokenClaims]{}
	}

	return &client
}

func loadTokensFromFile(tokensFile string) *oidc.Tokens[*oidc.IDTokenClaims] {
	if tokensFile == "" {
		return nil
	}

	ret := new(oidc.Tokens[*oidc.IDTokenClaims])

	contents, err := os.ReadFile(tokensFile)
	if err != nil {
		return nil
	}

	err = json.Unmarshal(contents, &ret)
	if err != nil {
		return nil
	}

	return ret
}

func saveTokensToFile(tokensFile string, tokens *oidc.Tokens[*oidc.IDTokenClaims]) error {
	if tokensFile == "" {
		return nil
	}

	contents, err := json.Marshal(tokens)
	if err != nil {
		return err
	}

	return os.WriteFile(tokensFile, contents, 0o644)
}

// GetAccessToken returns the Access Token from the OidcClient's tokens, or an empty string if no tokens are present.
func (o *OidcClient) GetAccessToken() string {
	if o.tokens == nil || o.tokens.Token == nil {
		return ""
	}

	return o.tokens.AccessToken
}

// GetOIDCTokens returns the current OIDC tokens, if any.
func (o *OidcClient) GetOIDCTokens() *oidc.Tokens[*oidc.IDTokenClaims] {
	return o.tokens
}

// Do function executes an HTTP request using the OidcClient's http client, and manages authorization by refreshing or authenticating as needed.
// If the request fails with an HTTP Unauthorized status, it attempts to refresh the access token, or perform an OIDC authentication if refresh fails.
func (o *OidcClient) Do(req *http.Request) (*http.Response, error) {
	req.Header.Set("Authorization", "Bearer "+o.GetAccessToken())

	resp, err := o.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	// Return immediately if the error is not HTTP status unauthorized.
	if resp.StatusCode != http.StatusUnauthorized {
		return resp, nil
	}

	issuer := resp.Header.Get("X-OperationsCenter-OIDC-issuer")
	clientID := resp.Header.Get("X-OperationsCenter-OIDC-clientid")
	audience := resp.Header.Get("X-OperationsCenter-OIDC-audience")

	if issuer == "" || clientID == "" {
		return resp, nil
	}

	// Refresh the token.
	err = o.refresh(issuer, clientID)
	if err != nil {
		err = o.authenticate(issuer, clientID, audience)
		if err != nil {
			return nil, err
		}
	}

	// If not dealing with something we can retry, return a clear error.
	if req.Method != "GET" && req.GetBody == nil {
		return resp, ErrOIDCExpired
	}

	// Set the new access token in the header.
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", o.tokens.AccessToken))

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

	err = saveTokensToFile(o.tokensFile, o.tokens)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

// getProvider initializes a new OpenID Connect Relying Party for a given issuer and clientID.
// The function also creates a secure CookieHandler with random encryption and hash keys, and applies a series of configurations on the Relying Party.
func (o *OidcClient) getProvider(issuer string, clientID string) (rp.RelyingParty, error) {
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
func (o *OidcClient) refresh(issuer string, clientID string) error {
	if o.tokens.Token == nil || o.tokens.RefreshToken == "" {
		return errRefreshAccessToken
	}

	provider, err := o.getProvider(issuer, clientID)
	if err != nil {
		return errRefreshAccessToken
	}

	oauthTokens, err := rp.RefreshTokens[*oidc.IDTokenClaims](context.TODO(), provider, o.tokens.RefreshToken, "", "")
	if err != nil {
		return errRefreshAccessToken
	}

	o.tokens.AccessToken = oauthTokens.AccessToken
	o.tokens.TokenType = oauthTokens.TokenType
	o.tokens.Expiry = oauthTokens.Expiry

	if oauthTokens.RefreshToken != "" {
		o.tokens.RefreshToken = oauthTokens.RefreshToken
	}

	return nil
}

// authenticate initiates the OpenID Connect device flow authentication process for the client.
// It presents a user code for the end user to input in the device that has web access and waits for them to complete the authentication,
// subsequently updating the client's tokens upon successful authentication.
func (o *OidcClient) authenticate(issuer string, clientID string, audience string) error {
	tokenURL, resp, provider, err := o.getTokenURL(issuer, clientID, audience)
	if err != nil {
		return err
	}

	fmt.Printf("URL: %s\n", tokenURL)
	fmt.Printf("Code: %s\n\n", resp.UserCode)

	_ = util.OpenBrowser(tokenURL)

	return o.WaitForToken(resp, provider)
}

func (o *OidcClient) FetchNewIncusTokenURL(req *http.Request) (string, *oidc.DeviceAuthorizationResponse, rp.RelyingParty, error) {
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

func (o *OidcClient) getTokenURL(issuer string, clientID string, audience string) (string, *oidc.DeviceAuthorizationResponse, rp.RelyingParty, error) {
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

func (o *OidcClient) WaitForToken(resp *oidc.DeviceAuthorizationResponse, provider rp.RelyingParty) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT)
	defer stop()

	token, err := rp.DeviceAccessToken(ctx, resp.DeviceCode, time.Duration(resp.Interval)*time.Second, provider)
	if err != nil {
		return err
	}

	if o.tokens.Token == nil {
		o.tokens.Token = &oauth2.Token{}
	}

	o.tokens.Expiry = time.Now().Add(time.Duration(token.ExpiresIn))
	o.tokens.IDToken = token.IDToken
	o.tokens.AccessToken = token.AccessToken
	o.tokens.TokenType = token.TokenType

	if token.RefreshToken != "" {
		o.tokens.RefreshToken = token.RefreshToken
	}

	return nil
}
