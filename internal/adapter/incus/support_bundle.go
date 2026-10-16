package incus

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"

	incusosapi "github.com/lxc/incus-os/incus-osd/api"
	incusapi "github.com/lxc/incus/v7/shared/api"

	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/shared/api"
)

// getOS returns the metadata of a GET request to the IncusOS API.
func (c Client) getOS(ctx context.Context, server provisioning.Server, apiPath string) (json.RawMessage, error) {
	client, err := c.getClient(ctx, server)
	if err != nil {
		return nil, err
	}

	resp, _, err := client.RawQuery(http.MethodGet, apiPath, http.NoBody, "")
	if err != nil {
		err = api.AsNotIncusOSError(err)

		return nil, fmt.Errorf("Failed to get %q from %q (%s): %w", apiPath, server.Name, server.GetConnectionURL(), err)
	}

	return resp.Metadata, nil
}

func (c Client) GetOS(ctx context.Context, server provisioning.Server) (json.RawMessage, error) {
	return c.getOS(ctx, server, "/os/1.0")
}

func (c Client) GetOSApplicationNames(ctx context.Context, server provisioning.Server) ([]string, error) {
	metadata, err := c.getOS(ctx, server, "/os/1.0/applications")
	if err != nil {
		return nil, err
	}

	var applicationURLs []string
	err = json.Unmarshal(metadata, &applicationURLs)
	if err != nil {
		return nil, fmt.Errorf("Unexpected response metadata while fetching applications from %q (%s): %w", server.Name, server.GetConnectionURL(), err)
	}

	names := make([]string, 0, len(applicationURLs))
	for _, applicationURL := range applicationURLs {
		names = append(names, path.Base(applicationURL))
	}

	return names, nil
}

func (c Client) GetOSApplication(ctx context.Context, server provisioning.Server, name string) (json.RawMessage, error) {
	return c.getOS(ctx, server, path.Join("/os/1.0/applications", url.PathEscape(name)))
}

func (c Client) GetOSDebugLog(ctx context.Context, server provisioning.Server, entries int) ([]map[string]any, error) {
	query := url.Values{"entries": {strconv.Itoa(entries)}}

	metadata, err := c.getOS(ctx, server, "/os/1.0/debug/log?"+query.Encode())
	if err != nil {
		return nil, err
	}

	journal := []map[string]any{}
	err = json.Unmarshal(metadata, &journal)
	if err != nil {
		return nil, fmt.Errorf("Unexpected response metadata while fetching the journal from %q (%s): %w", server.Name, server.GetConnectionURL(), err)
	}

	return journal, nil
}

func (c Client) GetOSDebugKernel(ctx context.Context, server provisioning.Server) (incusosapi.DebugKernel, error) {
	metadata, err := c.getOS(ctx, server, "/os/1.0/debug/kernel")
	if err != nil {
		return incusosapi.DebugKernel{}, err
	}

	var kernel incusosapi.DebugKernel
	err = json.Unmarshal(metadata, &kernel)
	if err != nil {
		return incusosapi.DebugKernel{}, fmt.Errorf("Unexpected response metadata while fetching kernel debug information from %q (%s): %w", server.Name, server.GetConnectionURL(), err)
	}

	return kernel, nil
}

func (c Client) GetOSDebugProcesses(ctx context.Context, server provisioning.Server) (string, error) {
	metadata, err := c.getOS(ctx, server, "/os/1.0/debug/processes")
	if err != nil {
		return "", err
	}

	var processes string
	err = json.Unmarshal(metadata, &processes)
	if err != nil {
		return "", fmt.Errorf("Unexpected response metadata while fetching the processes from %q (%s): %w", server.Name, server.GetConnectionURL(), err)
	}

	return processes, nil
}

// GetOSApplicationDebugLogs returns the logs of the application as gzip compressed tar archive.
func (c Client) GetOSApplicationDebugLogs(ctx context.Context, server provisioning.Server, application string) (io.ReadCloser, error) {
	body, err := json.Marshal(incusosapi.ApplicationAction{Action: "get-logs"})
	if err != nil {
		return nil, err
	}

	return c.getStream(ctx, server, http.MethodPost, "/os/1.0/applications/"+url.PathEscape(application)+"/:debug", body, fmt.Sprintf("logs of application %q", application))
}

// GetInstanceLogfile returns the content of a log file of the instance.
func (c Client) GetInstanceLogfile(ctx context.Context, server provisioning.Server, project string, name string, filename string) (io.ReadCloser, error) {
	apiPath := "/1.0/instances/" + url.PathEscape(name) + "/logs/" + url.PathEscape(filename) + "?project=" + url.QueryEscape(project)

	return c.getStream(ctx, server, http.MethodGet, apiPath, nil, fmt.Sprintf("log file %q of instance %q in project %q", filename, name, project))
}

// GetInstanceConsoleLog returns the console log of the instance.
func (c Client) GetInstanceConsoleLog(ctx context.Context, server provisioning.Server, project string, name string) (io.ReadCloser, error) {
	apiPath := "/1.0/instances/" + url.PathEscape(name) + "/console?project=" + url.QueryEscape(project)

	return c.getStream(ctx, server, http.MethodGet, apiPath, nil, fmt.Sprintf("console log of instance %q in project %q", name, project))
}

// getStream returns the response body of a request, which is bound to ctx unlike the streaming calls of the Incus client.
func (c Client) getStream(ctx context.Context, server provisioning.Server, method string, apiPath string, body []byte, what string) (io.ReadCloser, error) {
	client, err := c.getClient(ctx, server)
	if err != nil {
		return nil, err
	}

	info, err := client.GetConnectionInfo()
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, method, info.URL+apiPath, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := client.DoHTTP(req)
	if err != nil {
		return nil, fmt.Errorf("Failed to get %s from %q (%s): %w", what, server.Name, server.GetConnectionURL(), err)
	}

	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()

		errResp := incusapi.Response{}
		err = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&errResp)
		if err != nil || errResp.Error == "" {
			errResp.Error = resp.Status
		}

		return nil, fmt.Errorf("Failed to get %s from %q (%s): %w", what, server.Name, server.GetConnectionURL(), incusapi.StatusErrorf(resp.StatusCode, "%s", errResp.Error))
	}

	return resp.Body, nil
}
