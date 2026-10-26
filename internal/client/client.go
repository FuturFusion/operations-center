package client

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	incusTLS "github.com/lxc/incus/v7/shared/tls"

	oidcClient "github.com/FuturFusion/operations-center/internal/client/oidc"
	"github.com/FuturFusion/operations-center/shared/api"
)

const (
	apiVersionPrefix = "/1.0"
	internalPrefix   = "/internal"
)

type httpClient interface {
	Do(*http.Request) (*http.Response, error)
}

type OperationsCenterClient struct {
	httpClient httpClient
	baseURL    string

	forceLocal         bool
	unixSocket         string
	tlsClientCert      tls.Certificate
	trustedServerCert  *x509.Certificate
	transportProxy     func(*http.Request) (*url.URL, error)
	insecureSkipVerify bool
	oidcTokensFilename *string
	dialTimeout        time.Duration
}

type Option func(c *OperationsCenterClient) error

func WithForceLocal(unixSocket string) Option {
	return func(c *OperationsCenterClient) error {
		c.forceLocal = true
		c.unixSocket = unixSocket

		return nil
	}
}

func WithClientCertificate(certInfo *incusTLS.CertInfo) Option {
	return func(c *OperationsCenterClient) error {
		c.tlsClientCert = certInfo.KeyPair()

		return nil
	}
}

// WithInsecureSkipVerify disables the verification of the server certificate.
// The caller has to verify the presented certificate itself.
func WithInsecureSkipVerify() Option {
	return func(c *OperationsCenterClient) error {
		c.insecureSkipVerify = true

		return nil
	}
}

// WithTrustedServerCertificate pins the certificate the remote server is
// expected to present.
func WithTrustedServerCertificate(trustedServerCert *x509.Certificate) Option {
	return func(c *OperationsCenterClient) error {
		c.trustedServerCert = trustedServerCert

		return nil
	}
}

// WithDialTimeout limits the time to connect to the server.
func WithDialTimeout(timeout time.Duration) Option {
	return func(c *OperationsCenterClient) error {
		c.dialTimeout = timeout

		return nil
	}
}

// ForwardedIdentity is the identity of the client of a request, which a cluster member passes on when it forwards the request.
type ForwardedIdentity struct {
	// Address is the remote address of the client.
	Address string

	// Username and Protocol identify an authenticated client, both are empty otherwise.
	Username string
	Protocol string

	// Certificate is the client certificate, nil if the client presented none.
	Certificate *x509.Certificate
}

// WithForwardedIdentity passes identity on to the server with every request.
func WithForwardedIdentity(identity ForwardedIdentity) Option {
	return func(c *OperationsCenterClient) error {
		c.transportProxy = func(req *http.Request) (*url.URL, error) {
			// Headers set by the caller are never passed on as forwarded identity.
			for _, header := range api.ForwardedHeaders {
				req.Header.Del(header)
			}

			if identity.Username != "" {
				req.Header.Set(api.HeaderForwardedUsername, identity.Username)
			}

			if identity.Protocol != "" {
				req.Header.Set(api.HeaderForwardedProtocol, identity.Protocol)
			}

			req.Header.Set(api.HeaderForwardedAddress, identity.Address)

			if identity.Certificate != nil {
				req.Header.Set(api.HeaderForwardedCertificate, base64.StdEncoding.EncodeToString(identity.Certificate.Raw))
			}

			return nil, nil
		}

		return nil
	}
}

func WithOIDCTokensFile(oidcTokensFilename string) Option {
	return func(c *OperationsCenterClient) error {
		if c.oidcTokensFilename == nil {
			c.oidcTokensFilename = new(string)
		}

		*c.oidcTokensFilename = oidcTokensFilename

		return nil
	}
}

func (c OperationsCenterClient) tlsConfig() *tls.Config {
	tlsConfig := incusTLS.InitTLSConfig()
	tlsConfig.Certificates = []tls.Certificate{c.tlsClientCert}

	if c.trustedServerCert != nil {
		trustedServerCert := *c.trustedServerCert
		incusTLS.TLSConfigWithTrustedCert(tlsConfig, &trustedServerCert)
	} else if c.insecureSkipVerify {
		tlsConfig.InsecureSkipVerify = true
	}

	return tlsConfig
}

func New(addr string, opts ...Option) (OperationsCenterClient, error) {
	c := OperationsCenterClient{
		baseURL: strings.TrimSuffix(addr, "/"),
	}

	for _, opt := range opts {
		err := opt(&c)
		if err != nil {
			return OperationsCenterClient{}, err
		}
	}

	if c.forceLocal {
		// Setup a Unix socket dialer
		unixDial := func(_ context.Context, network, addr string) (net.Conn, error) {
			raddr, err := net.ResolveUnixAddr("unix", c.unixSocket)
			if err != nil {
				return nil, err
			}

			return net.DialUnix("unix", nil, raddr)
		}

		// Define the http transport
		transport := &http.Transport{
			Proxy:                 c.transportProxy,
			DialContext:           unixDial,
			DisableKeepAlives:     true,
			ExpectContinueTimeout: 30 * time.Second,
			ResponseHeaderTimeout: 3600 * time.Second,
			TLSHandshakeTimeout:   5 * time.Second,
		}

		// Define the http client
		c.httpClient = &http.Client{
			Transport: transport,
		}

		return c, nil
	}

	transport := &http.Transport{
		Proxy:                 c.transportProxy,
		TLSClientConfig:       c.tlsConfig(),
		ExpectContinueTimeout: 30 * time.Second,
		ResponseHeaderTimeout: 3600 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second,
	}

	if c.dialTimeout > 0 {
		transport.DialContext = (&net.Dialer{Timeout: c.dialTimeout}).DialContext
	}

	httpClient := &http.Client{
		Transport: transport,
	}

	c.httpClient = httpClient

	if c.oidcTokensFilename != nil {
		// The OIDC client only uses the certificate pinned httpClient for the
		// requests to Operations Center. The identity provider is a separate trust
		// domain and is reached with system trust.
		c.httpClient = oidcClient.NewClient(httpClient, *c.oidcTokensFilename)
	}

	return c, nil
}

func (c OperationsCenterClient) doRequestRawResponse(ctx context.Context, method string, endpoint string, query url.Values, content any, reqOpts ...func(*http.Request)) (*http.Response, error) {
	return c.doRawRequest(ctx, method, apiVersionPrefix, endpoint, query, content, reqOpts...)
}

func (c OperationsCenterClient) doInternalRequestRawResponse(ctx context.Context, method string, endpoint string, query url.Values, content any) (*http.Response, error) {
	return c.doRawRequest(ctx, method, internalPrefix, endpoint, query, content)
}

func (c OperationsCenterClient) doRawRequest(ctx context.Context, method string, prefix string, endpoint string, query url.Values, content any, reqOpts ...func(*http.Request)) (*http.Response, error) {
	apiEndpoint, err := url.JoinPath(prefix, endpoint)
	if err != nil {
		return nil, err
	}

	u, err := url.Parse(fmt.Sprintf("%s%s?%s", strings.TrimSuffix(c.baseURL, "/"), apiEndpoint, query.Encode()))
	if err != nil {
		return nil, err
	}

	contentType := "application/json"

	// In memory bodies are passed as *bytes.Reader, so they can be sent again on redirects.
	// Caller owned readers are wrapped by io.NopCloser, so the transport does not close them.
	var body io.Reader
	switch data := content.(type) {
	case ContentTypeReadCloser:
		contentType = data.ContentType()
		body = data

	case io.Reader:
		contentType = "application/octet-stream"
		body = io.NopCloser(data)

	case []byte:
		body = bytes.NewReader(data)

	case nil:
		body = http.NoBody

	default:
		contentJSON, err := json.Marshal(data)
		if err != nil {
			return nil, err
		}

		body = bytes.NewReader(contentJSON)
	}

	req, err := http.NewRequestWithContext(ctx, method, u.String(), body)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Accept-Encoding", "gzip")

	for _, opt := range reqOpts {
		opt(req)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.Header.Get("Content-Encoding") == "gzip" {
		resp.Body, err = gzip.NewReader(resp.Body)
		if err != nil {
			return nil, err
		}
	}

	return resp, nil
}

func (c OperationsCenterClient) DoRequest(ctx context.Context, method string, endpoint string, query url.Values, content any) (*api.Response, error) {
	resp, err := c.doRequestRawResponse(ctx, method, endpoint, query, content)
	if err != nil {
		return nil, err
	}

	return processResponse(resp)
}

// DoInternalRequest sends a request to the unversioned internal API below /internal.
func (c OperationsCenterClient) DoInternalRequest(ctx context.Context, method string, endpoint string, query url.Values, content any) (*api.Response, error) {
	resp, err := c.doInternalRequestRawResponse(ctx, method, endpoint, query, content)
	if err != nil {
		return nil, err
	}

	return processResponse(resp)
}

func processResponse(resp *http.Response) (*api.Response, error) {
	defer resp.Body.Close()

	decoder := json.NewDecoder(resp.Body)
	response := api.Response{}

	err := decoder.Decode(&response)
	if err != nil {
		if strings.Contains(err.Error(), "invalid character 'C'") {
			return nil, fmt.Errorf("Client sent an HTTP request to an HTTPS server")
		}

		// The response is not an API response, e.g. it has been produced by a
		// proxy in between, so only the status is available.
		if resp.StatusCode >= 400 {
			return nil, &ServerError{
				StatusCode: resp.StatusCode,
				Message:    resp.Status,
				RequestID:  resp.Header.Get(api.RequestIDHeader),
			}
		}

		return nil, fmt.Errorf("Failed to decode server response: %w", err)
	}

	if response.Code != 0 {
		return &response, newServerError(resp, response)
	}

	return &response, nil
}

// newServerError converts an error response into a ServerError.
func newServerError(resp *http.Response, response api.Response) error {
	serverErr := &ServerError{
		StatusCode: response.Code,
		Message:    response.Error,
		RequestID:  resp.Header.Get(api.RequestIDHeader),
	}

	var metadata api.ErrorMetadata

	err := response.MetadataAsStruct(&metadata)
	if err != nil {
		// No metadata, return error as is.
		return serverErr
	}

	serverErr.Reason = metadata.Reason
	serverErr.Hint = metadata.Hint
	serverErr.Details = metadata.Details

	if metadata.RequestID != "" {
		serverErr.RequestID = metadata.RequestID
	}

	return serverErr
}

func (c OperationsCenterClient) GetBaseAddr() string {
	return c.baseURL
}

func (c OperationsCenterClient) DoHTTP(req *http.Request) (*http.Response, error) {
	return c.httpClient.Do(req)
}

func (c OperationsCenterClient) GetAPIServerInfo(ctx context.Context) (api.ServerUntrusted, error) {
	response, err := c.DoRequest(ctx, http.MethodGet, "", url.Values{}, nil)
	if err != nil {
		return api.ServerUntrusted{}, err
	}

	serverInfo := api.ServerUntrusted{}
	err = json.Unmarshal(response.Metadata, &serverInfo)
	if err != nil {
		return api.ServerUntrusted{}, err
	}

	return serverInfo, nil
}

// IsServerTrusted verifies, if the server certificate is trusted. This trust
// can be established by two different ways:
//
//  1. The server has certificate signed by a trusted party, e.g. public CA.
//  2. The certificate matches the provides certificate, which has been trusted
//     by the user manually before.
//
// If the certificate presented by the server is not trusted, the certificate
// presented by the server is returned for further processing, e.g. manual
// verification by the user.
func (c OperationsCenterClient) IsServerTrusted(ctx context.Context, serverCertificate api.Certificate) (actualServerCertificate api.Certificate, _ bool, _ error) {
	tlsConfig := c.tlsConfig()

	var peerCert *x509.Certificate

	tlsConfig.InsecureSkipVerify = true
	tlsConfig.VerifyConnection = func(cs tls.ConnectionState) error {
		peerCert = cs.PeerCertificates[0]

		intermediates := x509.NewCertPool()
		for _, cert := range cs.PeerCertificates[1:] {
			intermediates.AddCert(cert)
		}

		_, err := peerCert.Verify(x509.VerifyOptions{
			DNSName:       cs.ServerName,
			Roots:         tlsConfig.RootCAs,
			Intermediates: intermediates,
		})

		return err
	}

	httpClient := http.Client{
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig,
		},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL, nil)
	if err != nil {
		return api.Certificate{}, false, err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		if peerCert != nil {
			actualServerCertificate = api.Certificate{Certificate: peerCert}
			if serverCertificate.String() != actualServerCertificate.String() {
				return actualServerCertificate, false, nil
			}

			return api.Certificate{}, true, nil
		}

		return api.Certificate{}, false, fmt.Errorf(`Failed to connect: %v`, err)
	}

	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}

	return api.Certificate{}, true, nil
}

type ContentTypeReadCloser interface {
	io.ReadCloser
	ContentType() string
}
