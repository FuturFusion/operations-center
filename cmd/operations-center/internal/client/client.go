package client

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/FuturFusion/operations-center/shared/api"
)

const apiVersionPrefix = "/1.0"

type OperationsCenterClient struct {
	httpClient *http.Client
}

func New() OperationsCenterClient {
	httpClient := http.DefaultClient

	httpClient.Transport = &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}

	return OperationsCenterClient{
		httpClient: httpClient,
	}
}

func (c OperationsCenterClient) doRequest(method string, endpoint string, query url.Values, content []byte) (*api.Response, error) {
	apiEndpoint, err := url.JoinPath(apiVersionPrefix, endpoint)
	if err != nil {
		return nil, err
	}

	// FIXME: take URL from config
	u, err := url.Parse(fmt.Sprintf("https://localhost:7443%s?%s", apiEndpoint, query.Encode()))
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
