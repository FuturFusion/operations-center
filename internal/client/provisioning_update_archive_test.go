package client_test

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/lxc/incus-os/incus-osd/api/images"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/security/signature/signaturetest"
	"github.com/FuturFusion/operations-center/internal/util/testing/uuidgen"
)

func Test_GetUpdateArchive(t *testing.T) {
	d := daemonSetup(t)

	// Setup
	caCert, cert, key := signaturetest.GenerateCertChain(t)

	updatesConfig, err := d.socketClient.GetSystemUpdatesConfig(t.Context())
	require.NoError(t, err)

	updatesPut := updatesConfig.UpdatesPut
	updatesPut.Source = ""
	updatesPut.SignatureVerificationRootCA = string(caCert)
	err = d.socketClient.UpdateSystemUpdatesConfig(t.Context(), updatesPut)
	require.NoError(t, err)

	const archiveFilename = "x86_64/IncusOS_202610272234.efi.gz"

	archiveFileContent := []byte("update-file")
	checksum := sha256.Sum256(archiveFileContent)

	manifest, err := json.Marshal(map[string]any{
		"format":       "1.0",
		"channels":     []string{"stable"},
		"origin":       "test-origin",
		"published_at": time.Date(2026, 10, 27, 22, 34, 0, 0, time.UTC),
		"severity":     images.UpdateSeverityNone,
		"version":      "202610272234",
		"files": provisioning.UpdateFiles{
			{
				Filename:     archiveFilename,
				Size:         len(archiveFileContent),
				Sha256:       hex.EncodeToString(checksum[:]),
				Component:    images.UpdateFileComponentOS,
				Type:         images.UpdateFileTypeUpdateEFI,
				Architecture: images.UpdateFileArchitecture64BitX86,
			},
		},
	})
	require.NoError(t, err)

	signedManifest := signaturetest.SignContent(t, cert, key, manifest)

	var archive bytes.Buffer
	tw := tar.NewWriter(&archive)
	writeTarFile(t, tw, provisioning.UpdateManifestFilename, signedManifest)
	err = tw.WriteHeader(&tar.Header{Typeflag: tar.TypeDir, Name: "x86_64/", Mode: 0o755})
	require.NoError(t, err)
	writeTarFile(t, tw, archiveFilename, archiveFileContent)
	err = tw.Close()
	require.NoError(t, err)

	err = d.socketClient.CreateUpdate(t.Context(), io.NopCloser(&archive))
	require.NoError(t, err)

	updates, err := d.socketClient.GetUpdates(t.Context())
	require.NoError(t, err)
	require.Len(t, updates, 1, "expect the added update")

	updateID := updates[0].UUID.String()

	// Run test
	rc, err := d.unauthorizedHTTPClient.GetUpdateArchive(t.Context(), updateID)
	require.NoError(t, err, "expect the export to succeed without authentication, like the other GET routes of updates")

	exported, err := io.ReadAll(rc)
	require.NoError(t, err)
	require.NoError(t, rc.Close())

	// Assert
	gotFiles := readTarFiles(t, exported)
	require.Equal(t, map[string]string{
		provisioning.UpdateManifestFilename: string(signedManifest),
		archiveFilename:                     string(archiveFileContent),
	}, gotFiles, "expect the manifest and the update files in the archive")

	err = d.socketClient.CleanupAllUpdates(t.Context())
	require.NoError(t, err)

	err = d.socketClient.CreateUpdate(t.Context(), io.NopCloser(bytes.NewReader(exported)))
	require.NoError(t, err, "expect the exported archive to be accepted by add")

	updates, err = d.socketClient.GetUpdates(t.Context())
	require.NoError(t, err)
	require.Len(t, updates, 1, "expect the update to be added again")
	require.Equal(t, "202610272234", updates[0].Version, "expect the version of the exported update")

	_, err = d.socketClient.GetUpdateArchive(t.Context(), uuidgen.FromPattern(t, "9").String())
	require.ErrorIs(t, err, domain.ErrNotFound, "expect an error for an unknown update")
}

func writeTarFile(t *testing.T, tw *tar.Writer, name string, content []byte) {
	t.Helper()

	err := tw.WriteHeader(&tar.Header{Typeflag: tar.TypeReg, Name: name, Mode: 0o644, Size: int64(len(content))})
	require.NoError(t, err)

	_, err = tw.Write(content)
	require.NoError(t, err)
}

func readTarFiles(t *testing.T, archive []byte) map[string]string {
	t.Helper()

	files := map[string]string{}
	tr := tar.NewReader(bytes.NewReader(archive))
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return files
		}

		require.NoError(t, err)

		if hdr.Typeflag != tar.TypeReg {
			continue
		}

		body, err := io.ReadAll(tr)
		require.NoError(t, err)

		files[hdr.Name] = string(body)
	}
}
