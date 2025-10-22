package client

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	incusTLS "github.com/lxc/incus/v6/shared/tls"

	oidcClient "github.com/FuturFusion/operations-center/internal/client/oidc"
	"github.com/FuturFusion/operations-center/shared/api"
)

const apiVersionPrefix = "/1.0"

type httpClient interface {
	Do(*http.Request) (*http.Response, error)
}

type OperationsCenterClient struct {
	httpClient httpClient
	baseURL    string

	forceLocal         bool
	unixSocket         string
	tlsClientCert      tls.Certificate
	oidcTokensFilename *string
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

func WithOIDCTokensFile(oidcTokensFilename string) Option {
	return func(c *OperationsCenterClient) error {
		if c.oidcTokensFilename == nil {
			c.oidcTokensFilename = new(string)
		}

		*c.oidcTokensFilename = oidcTokensFilename

		return nil
	}
}

func New(addr string, opts ...Option) (OperationsCenterClient, error) {
	c := OperationsCenterClient{
		baseURL: addr,
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
			DialContext:           unixDial,
			DisableKeepAlives:     true,
			ExpectContinueTimeout: time.Second * 30,
			ResponseHeaderTimeout: time.Second * 3600,
			TLSHandshakeTimeout:   time.Second * 5,
		}

		// Define the http client
		c.httpClient = &http.Client{
			Transport: transport,
		}

		return c, nil
	}

	httpClient := http.DefaultClient

	httpClient.Transport = &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
			Certificates:       []tls.Certificate{c.tlsClientCert},
		},
	}

	c.httpClient = httpClient

	if c.oidcTokensFilename != nil {
		c.httpClient = oidcClient.NewClient(httpClient, *c.oidcTokensFilename)
	}

	return c, nil
}

func (c OperationsCenterClient) doRequestRawResponse(ctx context.Context, method string, endpoint string, query url.Values, content any) (*http.Response, error) {
	apiEndpoint, err := url.JoinPath(apiVersionPrefix, endpoint)
	if err != nil {
		return nil, err
	}

	u, err := url.Parse(fmt.Sprintf("%s%s?%s", strings.TrimSuffix(c.baseURL, "/"), apiEndpoint, query.Encode()))
	if err != nil {
		return nil, err
	}

	contentType := "application/json"

	var body io.ReadCloser
	switch data := content.(type) {
	case io.Reader:
		contentType = "application/octet-stream"
		body = io.NopCloser(data)
	case []byte:
		body = io.NopCloser(bytes.NewBuffer(data))
	case nil:
		body = http.NoBody
	default:
		contentJSON, err := json.Marshal(data)
		if err != nil {
			return nil, err
		}

		body = io.NopCloser(bytes.NewBuffer(contentJSON))
	}

	req, err := http.NewRequestWithContext(ctx, method, u.String(), body)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Accept-Encoding", "gzip")

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

func (c OperationsCenterClient) doRequest(ctx context.Context, method string, endpoint string, query url.Values, content any) (*api.Response, error) {
	resp, err := c.doRequestRawResponse(ctx, method, endpoint, query, content)
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

		if resp.StatusCode >= 400 {
			return nil, fmt.Errorf("Received an error from the server: %s", resp.Status)
		}

		return nil, fmt.Errorf("Failed to decode server response: %w", err)
	}

	if response.Code != 0 {
		return &response, fmt.Errorf("Received an error from the server: %s", response.Error)
	}

	return &response, nil
}

func (c OperationsCenterClient) GetAPIServerInfo(ctx context.Context) (api.ServerUntrusted, error) {
	response, err := c.doRequest(ctx, http.MethodGet, "", url.Values{}, nil)
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
	resp, err := (&http.Client{}).Get(c.baseURL)
	if err != nil {
		switch actualErr := err.(*url.Error).Unwrap().(type) {
		case *tls.CertificateVerificationError:
			actualServerCertificate = api.Certificate{Certificate: actualErr.UnverifiedCertificates[0]}
			if serverCertificate.String() != actualServerCertificate.String() {
				return actualServerCertificate, false, nil
			}

			return api.Certificate{}, true, nil

		default:
			return api.Certificate{}, false, fmt.Errorf(`Failed to connect: %v`, err)
		}
	}

	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}

	return api.Certificate{}, true, nil
}
