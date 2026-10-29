package e2e

import (
	"archive/zip"
	"bufio"
	"encoding/json"
	"io"
	"maps"
	"net/url"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/shared/api"
)

// assertBMCRecording verifies, that the deployment of the server has been
// recorded and that the anonymized export holds none of the identifying values
// of the server and of the Redfish proxy. Both exports are kept in the test
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

	// The test directory is kept between runs, so an export of an earlier run is replaced.
	mustRun(t, `../bin/operations-center.linux.%s admin bmc-recording download %s %s --anonymize --force --quiet --output %s`, cpuArch, name, listed.RecordingID, anonymizedFilename)
	mustRun(t, `../bin/operations-center.linux.%s admin bmc-recording download %s %s --force --quiet --output %s`, cpuArch, name, listed.RecordingID, rawFilename)

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

	for _, content := range [][]byte{manifestContent, exchangesContent} {
		require.NotContains(t, string(content), proxyHost, "expect the address of the Redfish proxy to be anonymized")
		require.NotContains(t, strings.ToLower(string(content)), strings.ToLower(server.BMCData.ServerUUID), "expect the system UUID to be anonymized")
	}

	_, rawExchanges := readBMCRecordingArchive(t, rawFilename)
	require.Contains(t, string(rawExchanges), proxyHost, "expect the raw export to hold the address of the Redfish proxy, so the anonymized export proves a change")
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
