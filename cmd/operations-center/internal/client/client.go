package client

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/FuturFusion/operations-center/shared/api"
)

const apiVersionPrefix = "/1.0"

type OperationsCenterClient struct {
	httpClient *http.Client
	baseURL    string

	forceLocal bool
}

type Option func(c *OperationsCenterClient)

func WithForceLocal(forceLocal bool) Option {
	return func(c *OperationsCenterClient) {
		c.forceLocal = forceLocal
	}
}

func New(serverPort string, opts ...Option) OperationsCenterClient {
	c := OperationsCenterClient{}

	for _, opt := range opts {
		opt(&c)
	}

	if c.forceLocal {
		// Setup a Unix socket dialer
		unixDial := func(_ context.Context, network, addr string) (net.Conn, error) {
			raddr, err := net.ResolveUnixAddr("unix", "./tmp/unix.socket")
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
		client := &http.Client{
			Transport: transport,
		}

		c.httpClient = client
		c.baseURL = "http://unix.socket/"

		return c
	}

	httpClient := http.DefaultClient

	httpClient.Transport = &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}

	c.httpClient = httpClient
	c.baseURL = fmt.Sprintf("https://%s", serverPort)

	return c
}

func (c OperationsCenterClient) doRequest(method string, endpoint string, query url.Values, content []byte) (*api.Response, error) {
	apiEndpoint, err := url.JoinPath(apiVersionPrefix, endpoint)
	if err != nil {
		return nil, err
	}

	u, err := url.Parse(fmt.Sprintf("%s%s?%s", c.baseURL, apiEndpoint, query.Encode()))
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(method, u.String(), bytes.NewBuffer(content))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")

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
