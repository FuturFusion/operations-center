package e2e

import (
	"archive/zip"
	"bufio"
	"context"
	"encoding/json"
	"io"
	"maps"
	"net/url"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/shared/api"
)

// assertBMCRecording verifies that the deployment of the server has been
// recorded. It also checks that the anonymized export does not hold the
// address of the Redfish proxy, the system UUID, the serial number and the
// token UUID. Both exports are kept in the test
// directory, so they are available after the run. The file names contain the
// profile of the Redfish proxy, so the exports of the profiles do not collide.
func assertBMCRecording(t *testing.T, name string, profile string, endpoint string, tmpDir string) {
	t.Helper()

	stop := timeTrack(t)
	defer stop()

	endpointURL, err := url.Parse(endpoint)
	require.NoError(t, err)

	proxyHost := endpointURL.Hostname()

	var server api.Server

	err = json.Unmarshal([]byte(mustRun(t, `../bin/operations-center.linux.%s provisioning server show %s -f json`, cpuArch, name).Output()), &server)
	require.NoError(t, err, "expect the server to be shown as JSON")
	require.NotEmpty(t, server.BMCData.ServerUUID, "expect the BMC to report the system UUID")

	var manifests []api.BMCRecordingManifest

	err = json.Unmarshal([]byte(mustRun(t, `../bin/operations-center.linux.%s admin bmc-recording list %s -f json`, cpuArch, name).Output()), &manifests)
	require.NoError(t, err, "expect the recordings to be listed as JSON")
	require.Len(t, manifests, 1, "expect exactly one recording of the deployment")

	listed := manifests[0]
	require.Equal(t, api.ServerDeploymentStateCompleted, listed.FinalState, "expect the recording to be finalized with the outcome of the deployment")
	require.Positive(t, listed.Exchanges, "expect BMC exchanges to be recorded")

	anonymizedFilename := filepath.Join(tmpDir, name+"-"+profile+"-bmc-recording.zip")
	rawFilename := filepath.Join(tmpDir, name+"-"+profile+"-bmc-recording-raw.zip")

	require.NoError(t, fmtRunErr(downloadBMCRecording(t.Context(), t, name, listed.RecordingID, anonymizedFilename, "--anonymize")), "expect the anonymized export to be downloaded")
	require.NoError(t, fmtRunErr(downloadBMCRecording(t.Context(), t, name, listed.RecordingID, rawFilename)), "expect the raw export to be downloaded")

	manifestContent, exchangesContent := readBMCRecordingArchive(t, anonymizedFilename)

	var manifest api.BMCRecordingManifest

	err = json.Unmarshal(manifestContent, &manifest)
	require.NoError(t, err, "expect the manifest to be valid JSON")
	require.Equal(t, api.BMCRecordingFormatVersion, manifest.FormatVersion)
	require.Equal(t, api.ServerDeploymentStateCompleted, manifest.FinalState)
	require.Equal(t, "server01", manifest.Server, "expect the server name to be anonymized")
	require.Equal(t, listed.Exchanges, manifest.Exchanges, "expect the export to hold every recorded exchange")
	require.NotEmpty(t, manifest.History, "expect the step history of the deployment in the manifest")

	states := map[api.ServerDeploymentState]bool{}
	sequence := int64(0)

	scanner := bufio.NewScanner(strings.NewReader(string(exchangesContent)))
	scanner.Buffer(nil, 16*1024*1024)

	for scanner.Scan() {
		var exchange api.BMCRecordingExchange

		err = json.Unmarshal(scanner.Bytes(), &exchange)
		require.NoError(t, err, "expect every exchange to be valid JSON")

		sequence++
		require.Equal(t, sequence, exchange.Sequence, "expect the sequence to increase by one from 1")
		require.NotEmpty(t, exchange.Method, "expect every exchange to have a method")
		require.NotEmpty(t, exchange.URL, "expect every exchange to have a URL")

		for header, values := range exchange.RequestHeaders {
			for _, value := range values {
				require.False(t, strings.HasPrefix(value, "Basic "), "expect the credentials in header %q to be redacted", header)
			}
		}

		states[exchange.DeploymentState] = true
	}

	require.NoError(t, scanner.Err())
	require.Equal(t, manifest.Exchanges, sequence, "expect the manifest to count every exchange")

	for _, state := range []api.ServerDeploymentState{
		api.ServerDeploymentStateRefreshBMCData,
		api.ServerDeploymentStateApplyBIOS,
		api.ServerDeploymentStateAttachMedia,
		api.ServerDeploymentStatePowerOnInstall,
	} {
		require.True(t, states[state], "expect the BMC exchanges of state %q to be recorded", state)
	}

	rawManifestContent, rawExchanges := readBMCRecordingArchive(t, rawFilename)
	require.Contains(t, string(rawExchanges), proxyHost, "expect the raw export to hold the address of the Redfish proxy, so the anonymized export proves a change")

	var rawManifest api.BMCRecordingManifest

	err = json.Unmarshal(rawManifestContent, &rawManifest)
	require.NoError(t, err, "expect the raw manifest to be valid JSON")
	require.NotEmpty(t, rawManifest.Request.TokenUUID, "expect the raw manifest to hold the token UUID")

	identifying := map[string]string{
		"the address of the Redfish proxy": proxyHost,
		"the system UUID":                  server.BMCData.ServerUUID,
		"the token UUID":                   rawManifest.Request.TokenUUID,
	}

	if server.BMCData.ServerSerialNumber != "" {
		identifying["the serial number"] = server.BMCData.ServerSerialNumber
	}

	for _, content := range [][]byte{manifestContent, exchangesContent} {
		for description, value := range identifying {
			require.NotContains(t, strings.ToLower(string(content)), strings.ToLower(value), "expect %s to be anonymized", description)
		}
	}
}

// exportBMCRecordingsOnFailure returns a cleanup, which keeps the anonymized
// BMC recordings of a failed deployment in the test directory. Register it
// after the cleanup that removes the server. Cleanups run in reverse order,
// and removing the server deletes its recordings.
func exportBMCRecordingsOnFailure(t *testing.T, name string, profile string, tmpDir string) func() {
	t.Helper()

	return func() {
		if !t.Failed() {
			return
		}

		// In t.Cleanup, t.Context() is cancelled, so we need a detached context.
		ctx, cancel := context.WithTimeout(context.Background(), strechedTimeout(2*time.Minute))
		defer cancel()

		resp := runWithContext(ctx, t, `../bin/operations-center.linux.%s admin bmc-recording list %s -f json`, cpuArch, name)

		err := fmtRunErr(resp)
		if err != nil {
			t.Logf("Failed to list the BMC recordings of %q: %v", name, err)
			return
		}

		var manifests []api.BMCRecordingManifest

		err = json.Unmarshal([]byte(resp.Output()), &manifests)
		if err != nil {
			t.Logf("Failed to parse the BMC recordings of %q: %v", name, err)
			return
		}

		for _, manifest := range manifests {
			filename := filepath.Join(tmpDir, name+"-"+profile+"-bmc-recording-failed-"+manifest.RecordingID+".zip")

			err = fmtRunErr(downloadBMCRecording(ctx, t, name, manifest.RecordingID, filename, "--anonymize"))
			if err != nil {
				t.Logf("Failed to download the BMC recording %s of %q: %v", manifest.RecordingID, name, err)
				continue
			}

			t.Logf("Kept the BMC recording %s of the failed deployment of %q in %s", manifest.RecordingID, name, filename)
		}
	}
}

// downloadBMCRecording downloads a BMC recording of the server as zip archive.
// The test directory is kept between runs, so an export of an earlier run is
// replaced.
func downloadBMCRecording(ctx context.Context, t *testing.T, name string, recordingID string, filename string, flags ...string) cmdResponse {
	t.Helper()

	return runWithContext(ctx, t, `../bin/operations-center.linux.%s admin bmc-recording download %s %s --force --quiet --output %s %s`, cpuArch, name, recordingID, filename, strings.Join(flags, " "))
}

// readBMCRecordingArchive returns the manifest and the exchanges of an exported
// BMC recording.
func readBMCRecordingArchive(t *testing.T, filename string) (manifest []byte, exchanges []byte) {
	t.Helper()

	archive, err := zip.OpenReader(filename)
	require.NoError(t, err, "expect the export to be a zip archive")

	defer func() {
		require.NoError(t, archive.Close())
	}()

	files := map[string][]byte{}
	for _, file := range archive.File {
		rc, err := file.Open()
		require.NoError(t, err)

		content, err := io.ReadAll(rc)
		require.NoError(t, err)
		require.NoError(t, rc.Close())

		files[file.Name] = content
	}

	require.ElementsMatch(t, []string{"manifest.json", "exchanges.jsonl"}, slices.Collect(maps.Keys(files)), "expect exactly the two files of a recording")

	return files["manifest.json"], files["exchanges.jsonl"]
}
