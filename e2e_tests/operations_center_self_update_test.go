package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const (
	selfUpdateVMName      = "OperationsCenterSelfUpdate"
	selfUpdateRemoteName  = "e2e-test-self-update"
	selfUpdateConfDirName = "self-update-cli-config"
)

// TestE2E_OperationsCenterSelfUpdate installs the previous stable release of
// Operations Center, updates it to the newest stable release and reboots it.
// It uses its own VM, so the shared Operations Center VM is not needed.
func TestE2E_OperationsCenterSelfUpdate(t *testing.T) {
	if listOnly {
		t.Skip("OPERATIONS_CENTER_E2E_TEST_LIST env var set, only listing end 2 end tests.")
	}

	skipUnlessE2ETestEnabled(t)

	runE2EScenario(t, "operations center self update", operationsCenterSelfUpdate)
}

func operationsCenterSelfUpdate(ctx context.Context, t *testing.T, tmpDir string) {
	t.Helper()

	// Setup
	fromVersion := selfUpdateFromVersion
	if fromVersion == "" {
		fromVersion = previousStableVersion(t)
	}

	t.Logf("Install Operations Center of the stable release %q", fromVersion)

	installPinnedOperationsCenterVM(ctx, t, tmpDir, fromVersion)

	oc := setupSelfUpdateCLI(ctx, t, tmpDir)

	assertOperationsCenterSelfRegistered(t, oc)

	mustWaitOperationsCenterVersionData(ctx, t, oc)

	versions := mustOperationsCenterVersions(t, oc)
	require.Equal(t, fromVersion, versions.application, "expect the previous release of the operations-center application to be installed")
	require.Equal(t, fromVersion, versions.os, "expect the previous release of IncusOS to be installed")

	serverVersionBefore := mustOperationsCenterServerVersion(t, oc)

	t.Log("Wait for Operations Center to fetch the updates")
	mustRun(t, `%s provisioning update refresh`, oc)

	ok, err := waitForSuccessWithTimeout(ctx, t, "Operations Center to need an update",
		`%s provisioning server list -f json | jq -r -e '.[] | select(.name == "operations-center") | .server_status == "ready" and .version_data.os.needs_update == true and ([ .version_data.applications[] | select(.name == "operations-center" and .needs_update == true) ] | length == 1)'`,
		15*time.Minute, oc)
	require.NoError(t, err)
	if !ok {
		t.Log(mustRun(t, `%s provisioning update list`, oc).Output())
		t.Log(mustRun(t, `%s provisioning server list -f json`, oc).Output())
	}

	require.True(t, ok, "expect Operations Center to report an available update for itself and its OS")

	targetVersion := mustRun(t, `%s provisioning server list -f json | jq -r '.[] | select(.name == "operations-center") | .version_data.applications[] | select(.name == "operations-center") | .available_version'`, oc).OutputTrimmed()
	targetOSVersion := mustRun(t, `%s provisioning server list -f json | jq -r '.[] | select(.name == "operations-center") | .version_data.os.available_version'`, oc).OutputTrimmed()

	if selfUpdateReplaceDaemon {
		t.Log("OPERATIONS_CENTER_E2E_TEST_SELF_UPDATE_REPLACE_DAEMON is set: the local build triggers the update, the target is still the released version")
		stageLocalOperationsCenterExecutable(t, selfUpdateVMName)
		startLocalOperationsCenterExecutable(t, selfUpdateVMName)

		ok, err := waitForSuccessWithTimeout(ctx, t, "the local build of Operations Center to be ready",
			`%s provisioning server list -f json | jq -r -e '.[] | select(.name == "operations-center") | .server_status == "ready"'`,
			5*time.Minute, oc)
		require.NoError(t, err)
		require.True(t, ok, "expect the local build of Operations Center to start on the database of the previous release")
	}

	bootID := mustBootID(t, selfUpdateVMName)

	// Run test
	t.Logf("Trigger the self update from %q to %q (OS %q)", fromVersion, targetVersion, targetOSVersion)
	// The application update restarts Operations Center, so the request might
	// fail, even if the update is applied. The effect is asserted below.
	resp := run(t, `%s provisioning server system update operations-center`, oc)
	require.NoError(t, resp.err)
	if !resp.Success() {
		t.Logf("The trigger of the self update returned an error, wait for its effect anyway: %s", resp.Output())
	}

	// The restarted daemon polls its own record on startup.
	ok, err = waitForSuccessWithTimeout(ctx, t, "the operations-center application update to be applied",
		`%s provisioning server list -f json | jq -r -e '.[] | select(.name == "operations-center") | [ .version_data.applications[] | select(.name == "operations-center" and .version == "%s") ] | length == 1'`,
		20*time.Minute, oc, targetVersion)
	require.NoError(t, err)
	if !ok {
		t.Log(mustRun(t, `%s provisioning server list -f json`, oc).Output())
		logVMDebugInfo(t, selfUpdateVMName)
	}

	require.True(t, ok, "expect the operations-center application to be updated")

	// IncusOS stages the OS update after the application update. Operations
	// Center ignores the update events of IncusOS for its own record, so the
	// resync refreshes the record. The record and IncusOS must both report the
	// staged OS. The "admin os" commands use the proxy of Operations Center.
	ok, err = waitForSuccessWithTimeout(ctx, t, "the OS update to be staged",
		`%[1]s provisioning server resync operations-center && %[1]s provisioning server list -f json | jq -r -e '.[] | select(.name == "operations-center") | .version_data.os.version_next == "%[2]s" and .version_data.os.needs_reboot == true' && %[1]s admin os show -f json | jq -r -e '.environment.os_version_next == "%[2]s"' && %[1]s admin os system update show -f json | jq -r -e '.state.needs_reboot == true'`,
		20*time.Minute, oc, targetOSVersion)
	require.NoError(t, err)
	if !ok {
		t.Log(run(t, `%[1]s provisioning server list -f json; %[1]s admin os show -f json; %[1]s admin os system update show -f json`, oc).Output())
		logVMDebugInfo(t, selfUpdateVMName)
	}

	require.True(t, ok, "expect Operations Center and IncusOS to report the staged OS update")

	t.Log("Reboot Operations Center")
	// Operations Center might reboot before it responds.
	resp = run(t, `%s provisioning server system reboot operations-center`, oc)
	require.NoError(t, resp.err)
	if !resp.Success() {
		t.Logf("The trigger of the reboot returned an error, wait for the reboot anyway: %s", resp.Output())
	}

	mustWaitRebooted(ctx, t, selfUpdateVMName, bootID)
	mustWaitSelfUpdateVMLog(ctx, t, "System is ready")
	mustWaitOperationsCenterPort(ctx, t)

	// Assert
	ok, err = waitForSuccessWithTimeout(ctx, t, "Operations Center to be ready after the reboot",
		`%s provisioning server list -f json | jq -r -e '.[] | select(.name == "operations-center") | .server_status == "ready" and .server_status_detail == ""'`,
		5*time.Minute, oc)
	require.NoError(t, err)
	if !ok {
		t.Log(mustRun(t, `%s provisioning server list -f json`, oc).Output())
	}

	require.True(t, ok, "expect Operations Center to be ready after the reboot")

	assertOperationsCenterSelfRegistered(t, oc)

	mustWaitOperationsCenterVersionData(ctx, t, oc)

	versions = mustOperationsCenterVersions(t, oc)
	require.Equal(t, targetVersion, versions.application, "expect the new release of the operations-center application to run after the reboot")
	require.Equal(t, targetOSVersion, versions.os, "expect the new release of IncusOS to run after the reboot")
	require.False(t, versions.needsUpdate, "expect no pending update after the self update")
	require.False(t, versions.needsReboot, "expect no pending reboot after the reboot")

	mustRun(t, `incus exec %s -- systemctl is-active operations-center`, selfUpdateVMName)

	resp = run(t, `incus exec %s -- findmnt /usr/local/bin/operations-centerd`, selfUpdateVMName)
	require.NoError(t, resp.err)
	require.False(t, resp.Success(), "expect the daemon of the new release to run, not a bind mounted local build")

	// Match the level of the text handler of slog and the short level of tint.
	// The port check of the test closes the connection without a TLS
	// handshake, which the daemon logs as an error, so ignore it.
	errorLogs := mustRun(t, `incus exec %s -- journalctl -b -u operations-center --no-pager | { grep -E "level=ERROR| ERR " || true; } | { grep -v -E "http: TLS handshake error from [^ ]+: EOF" || true; }`, selfUpdateVMName).OutputTrimmed()
	require.Empty(t, errorLogs, "expect no errors in the log of Operations Center after the reboot")

	serverVersionAfter := mustOperationsCenterServerVersion(t, oc)

	// The server version is a constant in the code. Two releases may share
	// it, so it is only logged.
	t.Logf("Operations Center server version before the update: %q, after the update: %q", serverVersionBefore, serverVersionAfter)
}

// previousStableVersion returns the second newest version of the stable channel
// of the official image server.
func previousStableVersion(t *testing.T) string {
	t.Helper()

	resp := mustRunWithTimeout(t, `curl -fsSL https://images.linuxcontainers.org/os/index.json | jq -r '[ .updates[] | select(.channels | index("stable")) | .version ] | sort | reverse | .[1] // empty'`, time.Minute)
	version := resp.OutputTrimmed()
	if version == "" {
		t.Skip("The stable channel of the image server provides less than 2 versions")
	}

	return version
}

// installPinnedOperationsCenterVM installs the VM selfUpdateVMName from the
// IncusOS image of version. The offline mode of the customizer bundles the
// applications of the same version on a separate resources image. Without it,
// IncusOS installs the newest applications.
func installPinnedOperationsCenterVM(ctx context.Context, t *testing.T, tmpDir string, version string) {
	t.Helper()

	stop := timeTrack(t)
	defer stop()

	isoName := fmt.Sprintf("IncusOS_OperationsCenter_%s.iso", version)
	resourcesName := fmt.Sprintf("IncusOS_OperationsCenter_resources_%s.iso", version)

	t.Cleanup(cleanupIncusOSISOs(t, tmpDir, []string{isoName, resourcesName}))

	mustGetPinnedOperationsCenterIncusOSImages(ctx, t, tmpDir, version, isoName, resourcesName)

	importIncusOSISOStorageVolume(t, tmpDir, isoName)
	importIncusOSISOStorageVolume(t, tmpDir, resourcesName)

	// Registered after the ISO cleanup, so the VM is removed before its
	// volumes.
	t.Cleanup(cleanupSelfUpdateVM(t))

	// The debug output of the suite only covers the shared Operations Center VM.
	t.Cleanup(saveSelfUpdateVMJournal(t, tmpDir))

	status := mustInstanceStatus(ctx, t, selfUpdateVMName)
	if status != "" {
		require.NoError(t, removeInstanceWithContext(ctx, t, selfUpdateVMName))
	}

	mustRun(t, `incus init --empty --vm %s -c security.secureboot=false -c limits.cpu=%s -c limits.memory=%s -d root,size=%s -d root,io.cache=unsafe`, selfUpdateVMName, cpuCount, memorySize, diskSize)
	mustRun(t, `incus config device add %s vtpm tpm`, selfUpdateVMName)
	mustRun(t, `incus config device add %s boot-media disk pool=default source=%s boot.priority=10`, selfUpdateVMName, isoName)
	mustRun(t, `incus config device add %s resources disk pool=default source=%s`, selfUpdateVMName, resourcesName)
	mustRun(t, `incus config set %s systemd.credential.fully-enable-incus-agent=true`, selfUpdateVMName)
	require.NoError(t, startInstanceWithContext(ctx, t, selfUpdateVMName))

	t.Log("Waiting for the installation to complete")
	mustWaitSelfUpdateVMLog(ctx, t, "IncusOS was successfully installed")

	// The applications are installed on the first boot of the installed
	// system, so the resources stay attached until then.
	mustRemoveDevicesAndStart(ctx, t, selfUpdateVMName, "boot-media")
	mustWaitSelfUpdateVMLog(ctx, t, "System is ready")

	// Remove the resources, so the updates can only come from the network.
	mustRemoveDevicesAndStart(ctx, t, selfUpdateVMName, "resources")
	mustWaitSelfUpdateVMLog(ctx, t, "System is ready")
}

// mustGetPinnedOperationsCenterIncusOSImages downloads the offline IncusOS
// image of version and its resources image to tmpDir.
func mustGetPinnedOperationsCenterIncusOSImages(ctx context.Context, t *testing.T, tmpDir string, version string, isoName string, resourcesName string) {
	t.Helper()

	isoPath := filepath.Join(tmpDir, isoName)
	resourcesPath := filepath.Join(tmpDir, resourcesName)

	if errNotAnIncusOSImage(isoPath) == nil && isFile(resourcesPath) {
		return
	}

	clientCertificateJSONString, err := json.Marshal(getClientCertificate(t))
	require.NoError(t, err)

	seed := replacePlaceholders(
		operationsCenterSeedTemplate,
		map[string]string{
			"$CLIENT_CERTIFICATE$": string(clientCertificateJSONString),
		},
	)

	request := map[string]any{}
	err = json.Unmarshal(seed, &request)
	require.NoError(t, err)

	request["channel"] = "stable"
	request["version"] = version
	request["offline"] = true

	body, err := json.Marshal(request)
	require.NoError(t, err)

	imagesData := mustPostIncusOSCustomizerImages(ctx, t, body)

	mustDownloadIncusOSCustomizerFile(t, imagesData, "metadata.image", isoPath)
	mustDownloadIncusOSCustomizerFile(t, imagesData, "metadata.resources", resourcesPath)

	require.NoError(t, errNotAnIncusOSImage(isoPath))
}

func cleanupSelfUpdateVM(t *testing.T) func() {
	t.Helper()

	return func() {
		if noCleanup || (noCleanupOnError && t.Failed()) {
			return
		}

		// In t.Cleanup, t.Context() is cancelled, so we need a detached context.
		ctx, cancel := context.WithTimeout(context.Background(), strechedTimeout(6*time.Minute))
		defer cancel()

		err := removeInstanceWithContext(ctx, t, selfUpdateVMName)
		if err != nil {
			t.Logf("Failed to remove instance %q during cleanup: %v", selfUpdateVMName, err)
		}
	}
}

// mustRemoveDevicesAndStart stops the VM name, removes the devices and starts
// it again.
func mustRemoveDevicesAndStart(ctx context.Context, t *testing.T, name string, devices ...string) {
	t.Helper()

	require.NoError(t, stopInstanceWithContext(ctx, t, name))

	for _, device := range devices {
		require.NoError(t, retryStorageCmdWithContext(ctx, t, fmt.Sprintf("remove device %q of instance %q", device, name), `incus config device remove %s %s`, name, device))
	}

	require.NoError(t, startInstanceWithContext(ctx, t, name))
}

// setupSelfUpdateCLI configures an isolated config dir for the CLI, so the
// remote of the shared Operations Center VM stays untouched. It returns the
// command prefix to run the CLI against the VM selfUpdateVMName.
func setupSelfUpdateCLI(ctx context.Context, t *testing.T, tmpDir string) string {
	t.Helper()

	confDir := mustCreateIsolatedCLIConfigDir(t, tmpDir, selfUpdateConfDirName)

	homeDir, err := os.UserHomeDir()
	require.NoError(t, err)

	// The seed trusts the client certificate of incus.
	mustRun(t, `cp %s/.config/incus/client.crt %s/.config/incus/client.key %s/`, homeDir, homeDir, confDir)

	mustWaitOperationsCenterPort(ctx, t)

	address, err := instanceIPAddressWithContext(ctx, t, selfUpdateVMName)
	require.NoError(t, err)

	oc := fmt.Sprintf("OPERATIONS_CENTER_CONF=%s ../bin/operations-center.linux.%s", confDir, cpuArch)

	mustRun(t, `%s remote add --accept-certificate %s https://%s/`, oc, selfUpdateRemoteName, net.JoinHostPort(address, "8443"))
	mustRun(t, `%s remote switch %s`, oc, selfUpdateRemoteName)

	return oc
}

// mustWaitOperationsCenterPort waits for the API port of the VM
// selfUpdateVMName.
func mustWaitOperationsCenterPort(ctx context.Context, t *testing.T) {
	t.Helper()

	address, err := instanceIPAddressWithContext(ctx, t, selfUpdateVMName)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(ctx, strechedTimeout(5*time.Minute))
	defer cancel()

	require.NoError(t, waitForTCPPort(ctx, t, net.JoinHostPort(address, "8443"), time.Second))
}

func assertOperationsCenterSelfRegistered(t *testing.T, oc string) {
	t.Helper()

	resp := run(t, `%s provisioning server list -f json | jq -r -e '[ .[] | select(.name == "operations-center") ] | length == 1'`, oc)
	require.NoError(t, resp.err)
	require.True(t, resp.Success(), "expect operations-center to be self registered: %s", resp.Output())
}

// mustWaitOperationsCenterVersionData waits until the record operations-center
// holds the version data of the OS and of the operations-center application.
// The record is created before its first poll completes, so the resync makes
// sure the data is current.
func mustWaitOperationsCenterVersionData(ctx context.Context, t *testing.T, oc string) {
	t.Helper()

	ok, err := waitForSuccessWithTimeout(ctx, t, "the version data of operations-center",
		`%[1]s provisioning server resync operations-center && %[1]s provisioning server list -f json | jq -r -e '.[] | select(.name == "operations-center") | (.version_data.os.version // "") != "" and ((.version_data.applications // []) | any(.name == "operations-center" and (.version // "") != ""))'`,
		5*time.Minute, oc)
	require.NoError(t, err)
	if !ok {
		t.Log(run(t, `%s provisioning server list -f json`, oc).Output())
	}

	require.True(t, ok, "expect the record operations-center to hold the version data")
}

// saveSelfUpdateVMJournal saves the journal of the VM selfUpdateVMName to
// tmpDir, if the test failed.
func saveSelfUpdateVMJournal(t *testing.T, tmpDir string) func() {
	t.Helper()

	return func() {
		if !t.Failed() {
			return
		}

		// In t.Cleanup, t.Context() is cancelled, so we need a detached context.
		ctx, cancel := context.WithTimeout(context.Background(), strechedTimeout(30*time.Second))
		defer cancel()

		filename := filepath.Join(tmpDir, fmt.Sprintf("%s_journal_%s.log", selfUpdateVMName, time.Now().Format("2006-01-02-15-04-05")))

		resp := runWithContext(ctx, t, `incus exec %s -- journalctl -u operations-center -u incus-osd --no-pager -n %d`, selfUpdateVMName, debugJournalMaxLines)
		if !resp.Success() {
			t.Logf("Failed to get the journal of %q: %s", selfUpdateVMName, resp.Error())
			return
		}

		err := os.WriteFile(filename, resp.output.Bytes(), 0o600)
		if err != nil {
			t.Errorf("Failed to write the journal of %q to %q: %v", selfUpdateVMName, filename, err)
			return
		}

		fmt.Printf("%s journal saved in %q\n", selfUpdateVMName, filename)
	}
}

type operationsCenterVersions struct {
	application string
	os          string
	needsUpdate bool
	needsReboot bool
}

func mustOperationsCenterVersions(t *testing.T, oc string) operationsCenterVersions {
	t.Helper()

	resp := mustRun(t, `%s provisioning server list -f json | jq -c '.[] | select(.name == "operations-center") | .version_data | { application: (.applications[] | select(.name == "operations-center") | .version), os: .os.version, needs_update: ((.os.needs_update // false) or ([ .applications[] | select(.needs_update == true) ] | length > 0)), needs_reboot: (.os.needs_reboot // false) }'`, oc)

	var versions struct {
		Application string `json:"application"`
		OS          string `json:"os"`
		NeedsUpdate bool   `json:"needs_update"`
		NeedsReboot bool   `json:"needs_reboot"`
	}

	err := json.Unmarshal([]byte(resp.OutputTrimmed()), &versions)
	require.NoError(t, err, "failed to parse the version data of operations-center: %s", resp.Output())

	return operationsCenterVersions{
		application: versions.Application,
		os:          versions.OS,
		needsUpdate: versions.NeedsUpdate,
		needsReboot: versions.NeedsReboot,
	}
}

func mustOperationsCenterServerVersion(t *testing.T, oc string) string {
	t.Helper()

	return mustRun(t, `%s query /1.0 | jq -r '.metadata.server_version'`, oc).OutputTrimmed()
}

func mustBootID(t *testing.T, name string) string {
	t.Helper()

	return mustRun(t, `incus exec %s -- cat /proc/sys/kernel/random/boot_id`, name).OutputTrimmed()
}

// mustWaitRebooted waits until the VM name runs with a boot ID other than
// bootID. Without this, the checks after a reboot might still see the old
// boot.
func mustWaitRebooted(ctx context.Context, t *testing.T, name string, bootID string) {
	t.Helper()

	ok, err := waitForSuccessWithTimeout(ctx, t, name+" to reboot",
		`incus exec %s -- cat /proc/sys/kernel/random/boot_id | grep -v -x %s`,
		10*time.Minute, name, bootID)
	require.NoError(t, err)
	if !ok {
		logVMDebugInfo(t, name)
	}

	require.True(t, ok, "expect %s to reboot", name)
}

// mustWaitSelfUpdateVMLog waits for the incus agent of the VM selfUpdateVMName
// and for the log line want of incus-osd in the current boot.
func mustWaitSelfUpdateVMLog(ctx context.Context, t *testing.T, want string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(ctx, strechedTimeout(15*time.Minute))
	defer cancel()

	mustWaitAgentRunning(ctx, t, selfUpdateVMName)
	mustWaitExpectedLog(ctx, t, selfUpdateVMName, "incus-osd", want)
}
