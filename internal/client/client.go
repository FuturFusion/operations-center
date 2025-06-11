package client

import (
	"bytes"
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

func WithClientCertificate(clientCertFile string, clientKeyFile string) Option {
	return func(c *OperationsCenterClient) error {
		cert, err := tls.LoadX509KeyPair(clientCertFile, clientKeyFile)
		if err != nil {
			return err
		}

		c.tlsClientCert = cert

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

func New(serverPort string, opts ...Option) (OperationsCenterClient, error) {
	c := OperationsCenterClient{}

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

		c.baseURL = "http://unix.socket"

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

	c.baseURL = fmt.Sprintf("https://%s", serverPort)

	return c, nil
}

func (c OperationsCenterClient) doRequest(ctx context.Context, method string, endpoint string, query url.Values, content any) (*api.Response, error) {
	apiEndpoint, err := url.JoinPath(apiVersionPrefix, endpoint)
	if err != nil {
		return nil, err
	}

	u, err := url.Parse(fmt.Sprintf("%s%s?%s", c.baseURL, apiEndpoint, query.Encode()))
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

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	decoder := json.NewDecoder(resp.Body)
	response := api.Response{}

	err = decoder.Decode(&response)
	if err != nil {
		if strings.Contains(err.Error(), "invalid character 'C'") {
			return nil, fmt.Errorf("Client sent an HTTP request to an HTTPS server")
		}

		return nil, err
	}

	if response.Code != 0 {
		return &response, fmt.Errorf("Received an error from the server: %s", response.Error)
	}

	return &response, nil
}
