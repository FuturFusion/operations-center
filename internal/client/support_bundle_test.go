package client_test

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"path"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/client"
	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/provisioning/repo/sqlite/entities"
	"github.com/FuturFusion/operations-center/shared/api"
)

func Test_GetSupportBundle(t *testing.T) {
	d := daemonSetup(t)

	_, err := entities.CreateServer(t.Context(), d.db, provisioning.Server{
		Name:          "unreachable",
		Type:          api.ServerTypeIncus,
		ConnectionURL: "https://127.0.0.1:1",
		Channel:       defaultChannelName,
		Status:        api.ServerStatusReady,
		StatusDetail:  api.ServerStatusDetailNone,
		BMCConfig: api.BMCConfig{
			APIType:  api.BMCAPITypeRedfishV1Generic,
			Username: "admin",
			Password: "bmc-secret",
		},
	})
	require.NoError(t, err)

	request := api.SupportBundlePost{Type: api.SupportBundleTypeServer, Server: "unreachable"}

	_, _, err = d.unauthorizedHTTPClient.GetSupportBundle(t.Context(), request)
	require.ErrorIs(t, err, domain.ErrNotAuthenticated)

	_, _, err = d.socketClient.GetSupportBundle(t.Context(), api.SupportBundlePost{Type: api.SupportBundleTypeServer})
	var serverErr *client.ServerError
	require.ErrorAs(t, err, &serverErr)
	require.Equal(t, api.ErrorReasonInvalidArgument, serverErr.Reason)

	_, _, err = d.socketClient.GetSupportBundle(t.Context(), api.SupportBundlePost{Type: api.SupportBundleTypeCluster, Cluster: "missing"})
	require.ErrorIs(t, err, domain.ErrNotFound)

	filename, rc, err := d.socketClient.GetSupportBundle(t.Context(), request)
	require.NoError(t, err)

	content, err := io.ReadAll(rc)
	require.NoError(t, err)
	require.NoError(t, rc.Close())

	require.True(t, strings.HasPrefix(filename, "support-bundle-server-unreachable-"))
	root := strings.TrimSuffix(filename, ".zip")

	archive, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
	require.NoError(t, err)

	files := map[string]string{}
	for _, f := range archive.File {
		r, err := f.Open()
		require.NoError(t, err)

		data, err := io.ReadAll(r)
		require.NoError(t, err)

		files[f.Name] = string(data)
	}

	require.Contains(t, files, path.Join(root, "servers/unreachable/unreachable.error.txt"))

	serverRecord := files[path.Join(root, "operations-center/servers/unreachable.json")]
	require.Contains(t, serverRecord, `"[REDACTED]"`)
	require.NotContains(t, serverRecord, "bmc-secret")

	var m struct {
		Request  api.SupportBundlePost `json:"request"`
		Sections []struct {
			Path   string `json:"path"`
			Status string `json:"status"`
		} `json:"sections"`
	}

	err = json.Unmarshal([]byte(files[path.Join(root, "manifest.json")]), &m)
	require.NoError(t, err)
	require.Equal(t, request, m.Request)
	require.Contains(t, m.Sections, struct {
		Path   string `json:"path"`
		Status string `json:"status"`
	}{Path: "servers/unreachable/unreachable", Status: "skipped"})
}
