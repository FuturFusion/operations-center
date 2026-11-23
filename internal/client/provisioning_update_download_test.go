package client_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/lxc/incus-os/incus-osd/api/images"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/security/signature/signaturetest"
	"github.com/FuturFusion/operations-center/shared/api"
)

func Test_DownloadUpdate(t *testing.T) {
	d := daemonSetup(t)

	// Setup
	const (
		version       = "202610272234"
		fileName      = "x86_64/IncusOS_202610272234.efi.gz"
		legacyChannel = "legacy"
	)

	caCert, cert, key := signaturetest.GenerateCertChain(t)

	fileContent := []byte("update-file")
	checksum := sha256.Sum256(fileContent)

	manifest, err := json.Marshal(map[string]any{
		"format":       "1.0",
		"channels":     []string{"stable"},
		"origin":       "test-origin",
		"published_at": time.Date(2026, 10, 27, 22, 34, 0, 0, time.UTC),
		"severity":     images.UpdateSeverityNone,
		"version":      version,
		"files": provisioning.UpdateFiles{
			{
				Filename:     fileName,
				Size:         len(fileContent),
				Sha256:       hex.EncodeToString(checksum[:]),
				Component:    images.UpdateFileComponentOS,
				Type:         images.UpdateFileTypeUpdateEFI,
				Architecture: images.UpdateFileArchitecture64BitX86,
			},
		},
	})
	require.NoError(t, err)

	signedManifest := signaturetest.SignContent(t, cert, key, manifest)

	// The index does not list the version. The update source still provides it.
	signedIndex := signaturetest.SignContent(t, cert, key, []byte(`{"format":"1.0","updates":[]}`))

	updatesSource := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/index.sjson":
			_, _ = w.Write(signedIndex)

		case "/" + version + "/update.sjson":
			_, _ = w.Write(signedManifest)

		case "/" + version + "/" + fileName:
			_, _ = w.Write(fileContent)

		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(updatesSource.Close)

	updatesConfig, err := d.socketClient.GetSystemUpdatesConfig(t.Context())
	require.NoError(t, err)

	updatesPut := updatesConfig.UpdatesPut
	updatesPut.Source = updatesSource.URL
	updatesPut.SignatureVerificationRootCA = string(caCert)
	updatesPut.FilterExpression = ""
	updatesPut.FileFilterExpression = ""
	err = d.socketClient.UpdateSystemUpdatesConfig(t.Context(), updatesPut)
	require.NoError(t, err)

	err = d.socketClient.CreateChannel(t.Context(), api.ChannelPost{
		Name: legacyChannel,
	})
	require.NoError(t, err)

	// Run test
	err = d.unauthorizedHTTPClient.DownloadUpdate(t.Context(), api.UpdateDownloadPost{Version: version})
	require.ErrorIs(t, err, domain.ErrNotAuthenticated, "expect the download to require authentication")

	err = d.socketClient.DownloadUpdate(t.Context(), api.UpdateDownloadPost{
		Version:  "202601010000",
		Channels: []string{legacyChannel},
	})
	require.ErrorIs(t, err, domain.ErrNotFound, "expect an error for a version the update source does not provide")

	err = d.socketClient.DownloadUpdate(t.Context(), api.UpdateDownloadPost{
		Version:  version,
		Channels: []string{legacyChannel},
	})
	require.NoError(t, err)

	// Assert
	updates, err := d.socketClient.GetUpdates(t.Context())
	require.NoError(t, err)
	require.Len(t, updates, 1, "expect the downloaded update")
	require.Equal(t, version, updates[0].Version, "expect the requested version")
	require.Equal(t, api.UpdateStatusReady, updates[0].Status, "expect the update to be ready")
	require.Equal(t, []string{legacyChannel}, updates[0].Channels, "expect the update in the requested channel")

	rc, err := d.socketClient.GetUpdateArchive(t.Context(), updates[0].UUID.String())
	require.NoError(t, err, "expect the downloaded update to be exportable")
	require.NoError(t, rc.Close())

	err = d.socketClient.DownloadUpdate(t.Context(), api.UpdateDownloadPost{
		Version: version,
	})
	require.ErrorContains(t, err, "is already present", "expect an error for an update, which is already present")
}
