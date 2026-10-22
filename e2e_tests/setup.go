package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"golang.org/x/sync/errgroup"
)

const (
	incusOSCustomizerURL            = "https://incusos-customizer.linuxcontainers.org"
	operationsCenterISOName         = "IncusOS_OperationsCenter.iso"
	operationsCenterVMHealthTimeout = 2 * time.Minute
	instanceCreationStagger         = 5 * time.Second
	incusOSClusterName              = "incus-os-cluster"
)

// setupOperationsCenter collects the coverage data of the daemon when owner
// ends. In a suite, owner is the suite, which outlives the scenario t.
func setupOperationsCenter(ctx context.Context, t *testing.T, owner *testing.T, tmpDir string) {
	t.Helper()

	stop := timeTrack(t)
	defer stop()

	fresh := getOperationsCenterIncusOSISO(ctx, t, tmpDir)

	importOperationsCenterIncusOSISOStorageVolume(ctx, t, tmpDir, fresh)

	installed := installOperationsCenterVM(ctx, t)

	removeBootMedia(ctx, t)

	mustWaitAgentRunning(ctx, t, "OperationsCenter")

	mustWaitExpectedLog(ctx, t, "OperationsCenter", "incus-osd", "System is ready")

	replaceOperationsCenterExecutable(t, owner, tmpDir)

	setupLocalOperationsCenterConfig(ctx, t, installed)

	assertOperationsCenterSelfRegistration(t)

	mustWaitUpdatesReady(ctx, t)
}

func setupIncusOSWithToken(names []string) func(ctx context.Context, t *testing.T, tmpDir string) {
	return func(ctx context.Context, t *testing.T, tmpDir string) {
		t.Helper()

		stop := timeTrack(t, "setupIncusOSWithToken "+strings.Join(names, ", "))
		defer stop()

		// Register cleanup
		t.Cleanup(cleanupIncusOS(t, names))

		incusOSPreseededISOFilename := prepareStableTokenISO(ctx, t, tmpDir)

		createIncusOSInstances(ctx, t, incusOSPreseededISOFilename, names)

		printServerList(t)
	}
}

// setupIncusOSWithTokenFreshISO is setupIncusOSWithToken with an ISO, which is
// generated for this test only. Use it, if the test changes a setting of
// Operations Center, which goes into the seed, before the setup. The cached
// ISO of setupIncusOSWithToken does not contain such a change.
func setupIncusOSWithTokenFreshISO(names []string) func(ctx context.Context, t *testing.T, tmpDir string) {
	return func(ctx context.Context, t *testing.T, tmpDir string) {
		t.Helper()

		stop := timeTrack(t, "setupIncusOSWithTokenFreshISO "+strings.Join(names, ", "))
		defer stop()

		token := createProvisioningToken(t)

		incusOSPreseededISOFilename := fmt.Sprintf("IncusOS-preseeded-fresh-%[1]s.iso", token)

		// Register cleanup. The instances use the storage volume of the ISO, so
		// the ISO is removed after the instances.
		t.Cleanup(cleanupIncusOSISO(t, tmpDir, incusOSPreseededISOFilename))
		t.Cleanup(cleanupIncusOS(t, names))

		updateSystemSettingsWithRegistrationScriptlet(t, tmpDir)

		// A left over ISO contains the seed of an earlier run.
		err := removeIncusOSISOWithContext(ctx, t, tmpDir, incusOSPreseededISOFilename)
		require.NoError(t, err)

		generateIncusOSPreseededISO(t, tmpDir, token, incusOSPreseededISOFilename)

		importIncusOSISOStorageVolume(t, tmpDir, incusOSPreseededISOFilename)

		createIncusOSInstances(ctx, t, incusOSPreseededISOFilename, names)

		printServerList(t)
	}
}

func cleanupIncusOSISO(t *testing.T, tmpDir string, incusOSPreseededISOFilename string) func() {
	t.Helper()

	return func() {
		if noCleanup || (noCleanupOnError && t.Failed()) {
			return
		}

		// In t.Cleanup, t.Context() is cancelled, so we need a detached context.
		ctx, cancel := context.WithTimeout(context.Background(), strechedTimeout(2*time.Minute))
		defer cancel()

		stop := timeTrack(t, "cleanup IncusOS ISO")
		defer stop()

		err := removeIncusOSISOWithContext(ctx, t, tmpDir, incusOSPreseededISOFilename)
		if err != nil {
			t.Logf("Failed to remove the ISO %q during cleanup: %v", incusOSPreseededISOFilename, err)
		}
	}
}

// removeIncusOSISOWithContext removes the ISO file and its storage volume.
func removeIncusOSISOWithContext(ctx context.Context, t *testing.T, tmpDir string, incusOSPreseededISOFilename string) error {
	t.Helper()

	err := os.Remove(filepath.Join(tmpDir, incusOSPreseededISOFilename))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	resp := runWithContext(ctx, t, `incus storage volume list default -f json | jq -r -e '[ .[] | select(.name == "%s") ] | length == 1'`, incusOSPreseededISOFilename)
	if !resp.Success() {
		return nil
	}

	return retryStorageCmdWithContext(ctx, t, fmt.Sprintf("delete the storage volume %q", incusOSPreseededISOFilename), `incus storage volume delete default %s`, incusOSPreseededISOFilename)
}

// prepareStableTokenISO returns the ISO storage volume, which installs a server
// with a token of the stable channel. The registration scriptlet sets the
// description and the properties of the server.
func prepareStableTokenISO(ctx context.Context, t *testing.T, tmpDir string) string {
	t.Helper()

	updateSystemSettingsWithRegistrationScriptlet(t, tmpDir)

	token := createProvisioningToken(t)

	incusOSPreseededISOFilename := createIncusOSPreseededISO(t, tmpDir, token)

	importIncusOSISOStorageVolume(t, tmpDir, incusOSPreseededISOFilename)

	return incusOSPreseededISOFilename
}

// prepareProdChannelISO returns the ISO storage volume, which installs a server
// with a token of the prod channel. The prod channel holds the oldest update,
// so the server has updates pending.
func prepareProdChannelISO(ctx context.Context, t *testing.T, tmpDir string) string {
	t.Helper()

	token := createProvisioningTokenWithUpdateChannel(t, "prod", "")

	incusOSPreseededISOFilename := createIncusOSPreseededISO(t, tmpDir, token)

	importIncusOSISOStorageVolume(t, tmpDir, incusOSPreseededISOFilename)

	return incusOSPreseededISOFilename
}

func setupIncusOSFromManualUpload(ctx context.Context, t *testing.T, tmpDir string) {
	t.Helper()

	stop := timeTrack(t)
	defer stop()

	manualUpdateUUIDExistsResp := run(t, `../bin/operations-center.linux.%s provisioning update list -f json | jq -r -e '[ .[] | select(.update_status == "ready" and (.origin | contains("(local)") ) ) ] | length > 0'`, cpuArch)
	if !manualUpdateUUIDExistsResp.Success() {
		err := os.WriteFile(filepath.Join(tmpDir, "create_manual_update.sh"), createManualUpdateScript, 0o700)
		require.NoError(t, err)

		mustRunWithTimeout(t, `cd %s && ./create_manual_update.sh`, 5*time.Minute, tmpDir)

		mustRunWithTimeout(t, `../bin/operations-center.linux.%s provisioning update add %s/manual_update.tar`, 5*time.Minute, cpuArch, tmpDir)
	}

	names := []string{"IncusOS01", "IncusOS02", "IncusOS03"}

	// Register cleanup
	t.Cleanup(cleanupIncusOS(t, names))
	t.Cleanup(channelCleanup(t, "manual"))

	token := createProvisioningTokenWithUpdateChannel(t, "manual", "(local)")

	incusOSPreseededISOFilename := createIncusOSPreseededISO(t, tmpDir, token)

	importIncusOSISOStorageVolume(t, tmpDir, incusOSPreseededISOFilename)

	createIncusOSInstances(ctx, t, incusOSPreseededISOFilename, names)

	printServerList(t)
}

func setupIncusOSWithTokenSeed(ctx context.Context, t *testing.T, tmpDir string) {
	t.Helper()

	stop := timeTrack(t)
	defer stop()

	names := []string{"IncusOS01", "IncusOS02", "IncusOS03"}

	// Register cleanup
	t.Cleanup(cleanupIncusOS(t, names))

	token := createProvisioningToken(t)

	incusOSPreseededISOFilename := createIncusOSPreseededISOFromTokenSeed(t, tmpDir, token)

	importIncusOSISOStorageVolume(t, tmpDir, incusOSPreseededISOFilename)

	createIncusOSInstances(ctx, t, incusOSPreseededISOFilename, names)

	printServerList(t)
}

func cleanupIncusOS(t *testing.T, names []string) func() {
	t.Helper()

	return func() {
		if noCleanup || (noCleanupOnError && t.Failed()) {
			return
		}

		// In t.Cleanup, t.Context() is cancelled, so we need a detached context.
		ctx, cancel := context.WithTimeout(context.Background(), strechedTimeout(time.Duration(len(names))*6*time.Minute+time.Minute))
		defer cancel()

		stop := timeTrack(t, "cleanup IncusOS")
		defer stop()

		for _, name := range names {
			err := removeInstanceWithContext(ctx, t, name)
			if err != nil {
				t.Logf("Failed to remove instance %q during cleanup: %v", name, err)
			}
		}

		resp := runWithContext(ctx, t, `../bin/operations-center.linux.%s provisioning server list -f json | jq -r '.[] | select(.server_type == "incus") | .name'`, cpuArch)
		if !resp.Success() {
			t.Error(resp.Error())
		}

		for server := range strings.Lines(resp.OutputTrimmed()) {
			server = strings.TrimSpace(server)
			resp := runWithContext(ctx, t, `../bin/operations-center.linux.%s provisioning server remove %s`, cpuArch, server)
			if !resp.Success() {
				t.Error(resp.Error())
			}
		}
	}
}

// mustCreateCluster creates the cluster incus-os-cluster from the given
// servers.
func mustCreateCluster(t *testing.T, tmpDir string, channelName string, serverNames []string) {
	t.Helper()

	err := os.WriteFile(filepath.Join(tmpDir, "services.yaml"), incusOSClusterServicesConfig, 0o600)
	require.NoError(t, err)

	clientCertificate := getClientCertificate(t)

	err = os.WriteFile(
		filepath.Join(tmpDir, "application.yaml"),
		replacePlaceholders(
			incusOSClusterApplicationConfig,
			map[string]string{
				"$CLIENT_CERTIFICATE$": indent(clientCertificate, strings.Repeat(" ", 6)),
			},
		),
		0o600,
	)
	require.NoError(t, err)

	instanceIPs, _ := mustGetInstanceIPAndNames(t, serverNames)

	servers := strings.Join(serverNames, " --server-names ")

	t.Log("Create cluster")
	mustRun(t, `../bin/operations-center.linux.%s provisioning cluster add %s https://%s --server-names %s --channel %s --services-config %s --application-seed-config %s`, cpuArch, incusOSClusterName, net.JoinHostPort(instanceIPs[0], "8443"), servers, channelName, filepath.Join(tmpDir, "services.yaml"), filepath.Join(tmpDir, "application.yaml"))
}

func clusterCleanup(t *testing.T) func() {
	t.Helper()

	return func() {
		if noCleanup || (noCleanupOnError && t.Failed()) {
			return
		}

		// In t.Cleanup, t.Context() is cancelled, so we need a detached context.
		ctx, cancel := context.WithTimeout(context.Background(), strechedTimeout(30*time.Second))
		defer cancel()

		stop := timeTrack(t, "cluster cleanup")
		defer stop()

		resp := runWithContext(ctx, t, `../bin/operations-center.linux.%s provisioning cluster list -f json | jq -r '.[].name'`, cpuArch)
		if !resp.Success() {
			t.Error(resp.Error())
			return
		}

		for cluster := range strings.Lines(resp.Output()) {
			cluster = strings.TrimSpace(cluster)
			resp := runWithContext(ctx, t, `../bin/operations-center.linux.%s provisioning cluster remove %s --force`, cpuArch, cluster)
			if !resp.Success() {
				t.Error(resp.Error())
			}
		}
	}
}

func cleanupTokenSeed(t *testing.T, token string, name string) func() {
	t.Helper()

	return func() {
		if noCleanup || (noCleanupOnError && t.Failed()) {
			return
		}

		// In t.Cleanup, t.Context() is cancelled, so we need a detached context.
		ctx, cancel := context.WithTimeout(context.Background(), strechedTimeout(30*time.Second))
		defer cancel()

		stop := timeTrack(t, "cleanup token seed")
		defer stop()

		resp := runWithContext(ctx, t, `../bin/operations-center.linux.%s provisioning token seed remove %s %s`, cpuArch, token, name)
		if !resp.Success() {
			t.Logf("failed to cleanup token seed %s %q: %s", token, name, resp.Error())
		}
	}
}

func createTokenSeed(t *testing.T, token string, name string, seedFilename string) {
	t.Helper()

	resp := run(t, `../bin/operations-center.linux.%s provisioning token seed remove %s %s`, cpuArch, token, name)
	if resp.Success() {
		t.Logf("Removed left over token seed %q of token %q", name, token)
	}

	mustRun(t, `../bin/operations-center.linux.%s provisioning token seed add %s %s %s`, cpuArch, token, name, seedFilename)
}

func getClientCertificate(t *testing.T) string {
	t.Helper()

	homeDir, err := os.UserHomeDir()
	require.NoError(t, err)
	clientCertPath := filepath.Join(homeDir, ".config/incus/client.crt")

	if !isFile(clientCertPath) {
		stop := timeTrack(t)
		defer stop()

		resp := run(t, `incus remote generate-certificate`)
		require.NoError(t, resp.err)
	}

	clientCertificate, err := os.ReadFile(clientCertPath)
	require.NoError(t, err)

	return string(clientCertificate)
}

// getOperationsCenterIncusOSISO makes the IncusOS image of the Operations
// Center VM available in tmpDir and reports, whether it had to be downloaded.
func getOperationsCenterIncusOSISO(ctx context.Context, t *testing.T, tmpDir string) (fresh bool) {
	t.Helper()

	isoPath := filepath.Join(tmpDir, operationsCenterISOName)

	if isFile(isoPath) {
		err := errNotAnIncusOSImage(isoPath)
		if err == nil {
			return false
		}

		t.Logf("Discarding the cached Operations Center image: %v", err)
		require.NoError(t, os.Remove(isoPath))
	}

	stop := timeTrack(t)
	defer stop()

	clientCertificate := getClientCertificate(t)

	clientCertificateJSONString, err := json.Marshal(clientCertificate)
	require.NoError(t, err)

	operationsCenterSeed := replacePlaceholders(
		operationsCenterSeedTemplate,
		map[string]string{
			"$CLIENT_CERTIFICATE$": string(clientCertificateJSONString),
		},
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, incusOSCustomizerURL+"/1.0/images", bytes.NewBuffer(operationsCenterSeed))
	require.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	imagesData, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	err = resp.Body.Close()
	require.NoError(t, err)

	require.Equalf(t, http.StatusOK, resp.StatusCode, "The IncusOS customizer rejected the image request: %s", tail(string(imagesData), 20))

	imageDownloadURL := gjson.GetBytes(imagesData, "metadata.image").String()

	require.NotEmptyf(t, imageDownloadURL, "The IncusOS customizer returned no image path: %s", tail(string(imagesData), 20))

	mustRunWithTimeout(t, `curl -fsSL -o %s --compressed --retry 3 --retry-connrefused --retry-delay 5 %s%s`, 5*time.Minute, isoPath, incusOSCustomizerURL, imageDownloadURL)

	err = errNotAnIncusOSImage(isoPath)
	if err != nil {
		fileResp := runWithContext(ctx, t, `ls -l %s; file %s`, isoPath, isoPath)
		t.Logf("The downloaded Operations Center image is not usable: %s", fileResp.Output())

		require.NoError(t, os.Remove(isoPath))
		require.NoError(t, err)
	}

	return true
}

// importOperationsCenterIncusOSISOStorageVolume imports the IncusOS image of the
// Operations Center VM as a storage volume.
func importOperationsCenterIncusOSISOStorageVolume(ctx context.Context, t *testing.T, tmpDir string, fresh bool) {
	t.Helper()

	storageVolumes := mustRun(t, "incus storage volume list default -f compact")
	volumeExists := strings.Contains(storageVolumes.Output(), operationsCenterISOName)

	if volumeExists && !fresh {
		return
	}

	stop := timeTrack(t)
	defer stop()

	if volumeExists {
		t.Logf("Replacing the storage volume %q with the freshly downloaded image", operationsCenterISOName)

		status := mustInstanceStatus(ctx, t, "OperationsCenter")
		if status != "" {
			require.NoError(t, removeInstanceWithContext(ctx, t, "OperationsCenter"))
		}

		require.NoError(t, retryStorageCmdWithContext(ctx, t, fmt.Sprintf("delete the storage volume %q", operationsCenterISOName), `incus storage volume delete default %s`, operationsCenterISOName))
	}

	mustRunWithTimeout(t, `incus storage volume import default %s %s --type=iso`, 5*time.Minute, filepath.Join(tmpDir, operationsCenterISOName), operationsCenterISOName)
}

func installOperationsCenterVM(ctx context.Context, t *testing.T) (installed bool) {
	t.Helper()

	status := mustInstanceStatus(ctx, t, "OperationsCenter")
	if status != "" && status != instanceStatusRunning {
		// The VM might just be restarting, so give it a moment to settle before recreating it.
		t.Logf("Operations Center VM is in status %q, waiting for it to become running", status)
		status = waitInstanceStatusRunning(ctx, t, "OperationsCenter", 2*time.Minute)
	}

	if status == instanceStatusRunning && operationsCenterVMUsable(ctx, t) {
		return false
	}

	stop := timeTrack(t)
	defer stop()

	if status != "" {
		t.Logf("Operations Center VM is in status %q, removing it in order to install it from scratch", status)
		require.NoError(t, removeInstanceWithContext(ctx, t, "OperationsCenter"))
	}

	mustRun(t, `incus init --empty --vm OperationsCenter -c security.secureboot=false -c limits.cpu=%s -c limits.memory=%s -d root,size=%s -d root,io.cache=unsafe`, cpuCount, memorySize, diskSize)
	mustRun(t, `incus config device add OperationsCenter vtpm tpm`)
	mustRun(t, `incus config device add OperationsCenter boot-media disk pool=default source=%s boot.priority=10`, operationsCenterISOName)
	mustRun(t, `incus config set OperationsCenter systemd.credential.fully-enable-incus-agent=true`)
	require.NoError(t, startInstanceWithContext(ctx, t, "OperationsCenter"))

	t.Log("Waiting for Operations Center to complete installation")
	mustWaitAgentRunningWithTimeout(ctx, t, "OperationsCenter", 5*time.Minute)
	mustWaitExpectedLogWithTimeout(ctx, t, "OperationsCenter", "incus-osd", "IncusOS was successfully installed", 5*time.Minute)

	return true
}

// operationsCenterVMUsable reports, whether the Operations Center VM, which is
// found running, is actually usable, i.e. its incus agent answers. The status
// alone is no evidence: a VM, which is stuck, e.g. at the boot manager of a boot
// media it can not boot, stays in the status "Running" indefinitely and every
// test, which reuses it, only reproduces the same failure.
func operationsCenterVMUsable(ctx context.Context, t *testing.T) bool {
	t.Helper()

	ctx, cancel := context.WithTimeout(ctx, strechedTimeout(operationsCenterVMHealthTimeout))
	defer cancel()

	err := waitAgentRunningWithContext(ctx, t, "OperationsCenter")
	if err != nil {
		t.Logf("Operations Center VM is running, but not usable, installing it from scratch: %v", err)

		return false
	}

	return true
}

func removeBootMedia(ctx context.Context, t *testing.T) {
	t.Helper()

	instanceHasBootMedia := mustRun(t, "incus config device list OperationsCenter")
	if !strings.Contains(instanceHasBootMedia.Output(), "boot-media") {
		return
	}

	stop := timeTrack(t)
	defer stop()

	require.NoError(t, stopInstanceWithContext(ctx, t, "OperationsCenter"))

	mustRun(t, `incus config device remove OperationsCenter boot-media`)

	require.NoError(t, startInstanceWithContext(ctx, t, "OperationsCenter"))

	t.Log("Waiting for Operations Center to be ready")
}

func replaceOperationsCenterExecutable(t *testing.T, owner *testing.T, tmpDir string) {
	t.Helper()

	stop := timeTrack(t)
	defer stop()

	mustRun(t, `incus exec OperationsCenter -- bash -c "mkdir -p /root/dev/ && mount -t tmpfs tmpfs /root/dev/"`)
	mustRun(t, `incus exec OperationsCenter -- bash -c "systemctl stop operations-center || true"`)
	mustRun(t, `incus exec OperationsCenter -- bash -c "umount -l /usr/local/bin/operations-centerd || true"`)
	mustRun(t, `incus file push ../bin/operations-centerd OperationsCenter/root/dev/operations-centerd`)
	if testing.CoverMode() != "" {
		err := os.WriteFile(filepath.Join(tmpDir, "environment"), []byte(`GOCOVERDIR=/tmp/coverdata
`), 0o700)
		require.NoError(t, err)

		mustRun(t, `incus file push %s OperationsCenter/etc/environment`, filepath.Join(tmpDir, "environment"))
		mustRun(t, `incus exec OperationsCenter -- bash -c "rm -rf /tmp/coverdata; mkdir -p /tmp/coverdata"`)

		owner.Cleanup(func() {
			// Stop operations-centerd to flush coverage data.
			mustRunWithContext(context.Background(), owner, `incus exec OperationsCenter -- systemctl stop operations-center`)
			mustRunWithContext(context.Background(), owner, `incus exec OperationsCenter -- tar -czf - -C /tmp/coverdata . | tar -xzf - -C %s`, ocE2EGoCoverDir)
			mustRunWithContext(context.Background(), owner, `incus exec OperationsCenter -- systemctl start operations-center`)
		})
	}

	mustRun(t, `incus exec OperationsCenter -- bash -c "mount -o bind /root/dev/operations-centerd /usr/local/bin/operations-centerd && systemctl start operations-center"`)
}

// restartOperationsCenter restarts the Operations Center daemon and waits for
// its API.
func restartOperationsCenter(ctx context.Context, t *testing.T) {
	t.Helper()

	stop := timeTrack(t)
	defer stop()

	mustRunWithTimeout(t, `incus exec OperationsCenter -- systemctl restart operations-center`, 3*time.Minute)

	success, err := waitForSuccessWithTimeout(ctx, t, "Operations Center API", `../bin/operations-center.linux.%s provisioning server list -f json`, time.Minute, cpuArch)
	require.NoError(t, err)
	require.True(t, success, "expect the Operations Center API to be available after the restart")
}

func setupLocalOperationsCenterConfig(ctx context.Context, t *testing.T, freshInstall bool) {
	t.Helper()

	stop := timeTrack(t)
	defer stop()

	homeDir, err := os.UserHomeDir()
	require.NoError(t, err)

	// Preparing local configuration for operations-center CLI
	err = os.MkdirAll(filepath.Join(homeDir, ".config/operations-center"), 0o700)
	require.NoError(t, err)
	mustRun(t, `cp %[1]s/.config/incus/client.* %[1]s/.config/operations-center/`, homeDir)

	// Adding Operations Center instance as remote
	operationsCenterHostPort := net.JoinHostPort(operationsCenterIPAddress(t), "8443")

	ctxWithTimeout, cancel := context.WithTimeout(ctx, strechedTimeout(60*time.Second))
	err = waitForTCPPort(ctxWithTimeout, t, operationsCenterHostPort, 1*time.Second)
	cancel()
	require.NoError(t, err)

	operationsCenterAddress := fmt.Sprintf("https://%s/", operationsCenterHostPort)

	remoteAddrResp := mustRun(t, `../bin/operations-center.linux.%s remote list -f json | jq -r '.["e2e-test"].addr // empty'`, cpuArch)
	remoteAddr := remoteAddrResp.OutputTrimmed()

	if remoteAddr != "" && (freshInstall || remoteAddr != operationsCenterAddress || operationsCenterRemoteNeedsRetrust(ctx, t)) {
		mustRun(t, `../bin/operations-center.linux.%s remote remove e2e-test`, cpuArch)
		remoteAddr = ""
	}

	if remoteAddr == "" {
		mustRun(t, `../bin/operations-center.linux.%s remote add --accept-certificate e2e-test %s`, cpuArch, operationsCenterAddress)
	}

	resp := mustRun(t, `../bin/operations-center.linux.%s remote list`, cpuArch)
	fmt.Println(resp.Output())

	mustRun(t, `../bin/operations-center.linux.%s remote switch e2e-test`, cpuArch)
}

func updateSystemSettingsWithRegistrationScriptlet(t *testing.T, tmpDir string) {
	t.Helper()

	err := os.WriteFile(filepath.Join(tmpDir, "operations_center_settings.yaml"), operationsCenterSettingsWithRegistrationScriptletYAML, 0o600)
	require.NoError(t, err)

	mustRun(t, `../bin/operations-center.linux.%s system settings edit < %s/operations_center_settings.yaml`, cpuArch, tmpDir)
}

func createProvisioningToken(t *testing.T) string {
	t.Helper()

	tokenResp := mustRun(t, `../bin/operations-center.linux.%s provisioning token list -f json | jq -r '[ .[] | select(.channel == "stable" and .uses_remaining > 20) ] | first | .uuid // empty'`, cpuArch)
	token := tokenResp.OutputTrimmed()
	if token == "" {
		stop := timeTrack(t)
		defer stop()

		mustRun(t, `../bin/operations-center.linux.%s provisioning token add --description "test" --uses 100`, cpuArch)
		tokenResp := mustRun(t, `../bin/operations-center.linux.%s provisioning token list -f json | jq -r '[ .[] | select(.channel == "stable" and .uses_remaining > 20) ] | first | .uuid'`, cpuArch)
		token = tokenResp.OutputTrimmed()
	}

	return token
}

func createProvisioningTokenWithUpdateChannel(t *testing.T, channelName string, originFilter string) string {
	t.Helper()

	channelProdResp := run(t, `../bin/operations-center.linux.%s provisioning channel list -f json | jq -e -r '[ .[]| select(.name == "%s") ] | length > 0'`, cpuArch, channelName)
	require.NoError(t, channelProdResp.err)
	if !channelProdResp.Success() {
		mustRun(t, `../bin/operations-center.linux.%s provisioning channel add %s`, cpuArch, channelName)
	}

	oldestUpdateUUIDResp := mustRun(t, `../bin/operations-center.linux.%s provisioning update list -f json | jq -r '[ .[] | select(.update_status == "ready" and (.origin | contains("%s") ) ) ] | sort_by(.version) | first | .uuid'`, cpuArch, originFilter)

	mustRun(t, `../bin/operations-center.linux.%s provisioning update assign-channels %s --channel stable,%s`, cpuArch, oldestUpdateUUIDResp.OutputTrimmed(), channelName)

	tokenResp := mustRun(t, `../bin/operations-center.linux.%s provisioning token list -f json | jq -r '[ .[] | select(.channel == "%s" and .uses_remaining > 20) ] | first | .uuid // empty'`, cpuArch, channelName)
	token := tokenResp.OutputTrimmed()
	if token == "" {
		stop := timeTrack(t)
		defer stop()

		mustRun(t, `../bin/operations-center.linux.%s provisioning token add --description "test" --uses 50 --channel %s`, cpuArch, channelName)
		tokenResp := mustRun(t, `../bin/operations-center.linux.%s provisioning token list -f json | jq -r '[ .[] | select(.channel == "%s" and .uses_remaining > 20) ] | first | .uuid'`, cpuArch, channelName)
		token = tokenResp.OutputTrimmed()
	}

	return token
}

// channelCleanup assigns all the updates of the given channel to the stable
// channel only. The channel itself is kept, since the provisioning token of the
// channel and its cached ISO are reused by the next run.
func channelCleanup(t *testing.T, channelName string) func() {
	t.Helper()

	return func() {
		if noCleanup || (noCleanupOnError && t.Failed()) {
			return
		}

		// In t.Cleanup, t.Context() is cancelled, so we need a detached context.
		ctx, cancel := context.WithTimeout(context.Background(), strechedTimeout(30*time.Second))
		defer cancel()

		stop := timeTrack(t, fmt.Sprintf("%s channel cleanup", channelName))
		defer stop()

		resp := runWithContext(ctx, t, `../bin/operations-center.linux.%s provisioning update list -f json | jq -r '.[] | select(.channels | index("%s")) | .uuid'`, cpuArch, channelName)
		if !resp.Success() {
			t.Error(resp.Error())
			return
		}

		for updateUUID := range strings.Lines(resp.Output()) {
			updateUUID = strings.TrimSpace(updateUUID)
			resp := runWithContext(ctx, t, `../bin/operations-center.linux.%s provisioning update assign-channels %s --channel stable`, cpuArch, updateUUID)
			if !resp.Success() {
				t.Error(resp.Error())
			}
		}
	}
}

func createIncusOSPreseededISO(t *testing.T, tmpDir string, token string) string {
	t.Helper()

	incusOSPreseededISOFilename := fmt.Sprintf("IncusOS-preseeded-%[1]s.iso", token)
	if !isFile(filepath.Join(tmpDir, incusOSPreseededISOFilename)) {
		generateIncusOSPreseededISO(t, tmpDir, token, incusOSPreseededISOFilename)
	}

	return incusOSPreseededISOFilename
}

func generateIncusOSPreseededISO(t *testing.T, tmpDir string, token string, incusOSPreseededISOFilename string) {
	t.Helper()

	stop := timeTrack(t)
	defer stop()

	err := os.WriteFile(filepath.Join(tmpDir, "incusos_seed.yaml"), incusOSSeedFileYAMLTemplate, 0o600)
	require.NoError(t, err)

	mustRunWithTimeout(t, `../bin/operations-center.linux.%[1]s provisioning token get-image %[2]s %[3]s/%[4]s %[3]s/incusos_seed.yaml`, 10*time.Minute, cpuArch, token, tmpDir, incusOSPreseededISOFilename)
}

func createIncusOSPreseededISOFromTokenSeed(t *testing.T, tmpDir string, token string) string {
	t.Helper()

	const tokenSeedName = "incus-os-cluster"

	t.Cleanup(cleanupTokenSeed(t, token, tokenSeedName))

	incusOSPreseededISOFilename := fmt.Sprintf("IncusOS-preseeded-from-token-seed-%[1]s.iso", token[:8])
	if !isFile(filepath.Join(tmpDir, incusOSPreseededISOFilename)) {
		stop := timeTrack(t)
		defer stop()

		seedFilename := filepath.Join(tmpDir, "incusos_seed.yaml")

		err := os.WriteFile(seedFilename, incusOSSeedFileYAMLTemplate, 0o600)
		require.NoError(t, err)

		createTokenSeed(t, token, tokenSeedName, seedFilename)
		mustRunWithTimeout(t, `../bin/operations-center.linux.%s provisioning token seed get-image %s %s %s/%s`, 10*time.Minute, cpuArch, token, tokenSeedName, tmpDir, incusOSPreseededISOFilename)
	}

	return incusOSPreseededISOFilename
}

func importIncusOSISOStorageVolume(t *testing.T, tmpDir string, incusOSPreseededISOFilename string) {
	t.Helper()

	storageVolumes := mustRun(t, "incus storage volume list default -f compact")
	if !strings.Contains(storageVolumes.Output(), incusOSPreseededISOFilename) {
		stop := timeTrack(t)
		defer stop()

		mustRunWithTimeout(t, `incus storage volume import default %[1]s/%[2]s %[2]s --type=iso`, 5*time.Minute, tmpDir, incusOSPreseededISOFilename)
	}
}

func createIncusOSInstances(ctx context.Context, t *testing.T, incusOSPreseededISOFilename string, names []string) {
	t.Helper()

	stop := timeTrack(t)
	defer stop()

	existingServersResp := mustRun(t, `../bin/operations-center.linux.%s provisioning server list -f json | jq -r '[ .[] | select(.server_type == "incus" and .server_status == "ready") ] | length'`, cpuArch)
	existingServers, err := strconv.ParseInt(existingServersResp.OutputTrimmed(), 10, 64)
	require.NoError(t, err)

	timeout := 20*time.Minute + time.Duration(len(names))*3*time.Minute
	if !concurrentSetup {
		timeout = time.Duration(int(timeout) * len(names))
	}

	// Leave room for one reinstall from scratch, see below.
	timeout += 25 * time.Minute

	timeoutCtx, cancel := context.WithTimeout(ctx, strechedTimeout(timeout))
	defer cancel()

	errgrp, errgrpctx := errgroup.WithContext(timeoutCtx)
	if !concurrentSetup {
		errgrp.SetLimit(1)
	}

	for i, name := range names {
		errgrp.Go(func() (err error) {
			// Reduce the load during instance creation, attempt to mitigate the
			// "Failed to deactivate zvol." issue. Serialized instance creation
			// spreads the load by itself, so the stagger is only needed, if the
			// instances are created concurrently.
			if concurrentSetup && i > 0 {
				err = sleepWithContext(errgrpctx, time.Duration(i)*instanceCreationStagger)
				if err != nil {
					return err
				}
			}

			stop := timeTrack(t, fmt.Sprintf("createIncusOSInstance %s", name), "false")
			defer stop()

			defer func() {
				if err != nil {
					err = fmt.Errorf("%s: %w", name, err)
				}
			}()

			status, err := instanceStatusWithContext(errgrpctx, t, name)
			if err != nil {
				return err
			}

			if status != instanceStatusRunning {
				t.Logf("Setting up %s", name)

				// A left over instance, which is not running, might be in any
				// state, so remove it and install it from scratch.
				if status != "" {
					t.Logf("%s is in status %q, removing it in order to install it from scratch", name, status)

					err = removeInstanceWithContext(errgrpctx, t, name)
					if err != nil {
						return err
					}
				}

				err = installIncusOSInstance(errgrpctx, t, name, incusOSPreseededISOFilename)

				// A restart in place does not help, if incus-osd fails during
				// the installation, since the VM keeps its partially written
				// disk and TPM state. So install it once more from scratch.
				if errors.Is(err, errIncusOSStartupFailed) && errgrpctx.Err() == nil {
					t.Logf("Reinstalling %s from scratch, since incus-osd failed to start during installation: %v", name, err)

					startupErr := err

					err = removeInstanceWithContext(errgrpctx, t, name)
					if err != nil {
						return fmt.Errorf("Failed to remove %q after a failed installation (%w): %w", name, startupErr, err)
					}

					// The debug information of the first attempt is already
					// logged. Collect it again, if the reinstalled VM fails.
					forgetVMDebugInfo(name)

					err = installIncusOSInstance(errgrpctx, t, name, incusOSPreseededISOFilename)
					if err != nil {
						return fmt.Errorf("Failed to reinstall %q after a failed installation (%w): %w", name, startupErr, err)
					}
				}

				if err != nil {
					return err
				}
			}

			deviceListResp := runWithContext(errgrpctx, t, `incus config device list %s`, name)

			err = fmtRunErr(deviceListResp)
			if err != nil {
				return fmt.Errorf("Failed to list the devices of instance %q: %w", name, err)
			}

			if strings.Contains(deviceListResp.Output(), "boot-media") {
				t.Logf("Removing boot media from %s VM", name)

				err = stopInstanceWithContext(errgrpctx, t, name)
				if err != nil {
					return err
				}

				err = retryStorageCmdWithContext(errgrpctx, t, fmt.Sprintf("remove boot media of instance %q", name), `incus config device remove %s boot-media`, name)
				if err != nil {
					return err
				}

				err = startInstanceWithContext(errgrpctx, t, name)
				if err != nil {
					return err
				}
			}

			t.Logf("Waiting for %s to be ready", name)
			agentWaitCtx, cancel := context.WithTimeout(errgrpctx, strechedTimeout(5*time.Minute))
			err = waitAgentRunningWithContext(agentWaitCtx, t, name)
			cancel()
			if err != nil {
				return err
			}

			logWaitCtx, cancel := context.WithTimeout(errgrpctx, strechedTimeout(5*time.Minute))
			err = waitExpectedLogWithContext(logWaitCtx, t, name, "incus-osd", "System is ready", false)
			cancel()
			if err != nil {
				return err
			}

			return nil
		})
	}

	err = errgrp.Wait()
	if err != nil {
		logVMDebugInfo(t, names...)

		require.NoError(t, err, "Failed to create IncusOS VMs for e2e test")
	}

	// Wait for instances to self update in Operations Center
	instanceReadyTimeoutCtx, instanceReadyCancel := context.WithTimeout(ctx, strechedTimeout(2*time.Minute))
	defer instanceReadyCancel()

	for {
		operationsCenterSelfRegistered := runWithTimeout(t, `../bin/operations-center.linux.%s provisioning server list -f json | jq -r -e '[ .[] | select(.server_type == "incus" and .server_status == "ready") ] | length == %d'`, 10*time.Second, cpuArch, int(existingServers)+len(names))
		require.NoError(t, operationsCenterSelfRegistered.err)

		if operationsCenterSelfRegistered.Success() {
			break
		}

		select {
		case <-instanceReadyTimeoutCtx.Done():
			require.NoError(t, instanceReadyTimeoutCtx.Err())

		case <-time.After(time.Second):
		}
	}

	renameServers(t, names)
}

// renameServers renames the records of the given servers from the hostname of
// their VM to the name of their VM. A server registers with its hostname.
func renameServers(t *testing.T, names []string) {
	t.Helper()

	incusServers := mustRun(t, `incus list -f json | jq -r '.[] | select(.name as $n | %s | index($n) ) | .name + "," + .state.os_info.hostname'`, asJSON(t, names))
	incusServerNameHostnamePairs := strings.Split(incusServers.OutputTrimmed(), "\n")
	if len(names) != len(incusServerNameHostnamePairs) {
		t.Fatalf("expected a server %v for each name %v", incusServerNameHostnamePairs, names)
	}

	for _, serverNameHostnamePair := range incusServerNameHostnamePairs {
		nameHostnamePair := strings.SplitN(serverNameHostnamePair, ",", 2)
		mustRunWithTimeout(t, `../bin/operations-center.linux.%s provisioning server rename %s %s`, 10*time.Second, cpuArch, nameHostnamePair[1], nameHostnamePair[0])
	}
}

// installIncusOSInstance creates the VM name, boots it from the preseeded ISO
// and waits for the installation to complete.
func installIncusOSInstance(ctx context.Context, t *testing.T, name string, incusOSPreseededISOFilename string) error {
	t.Helper()

	err := fmtRunErr(runWithContext(ctx, t, `incus init --empty --vm %s -c security.secureboot=false -c limits.cpu=%s -c limits.memory=%s -d root,size=%s -d root,io.cache=unsafe`, name, cpuCount, memorySize, diskSize))
	if err != nil {
		return err
	}

	err = fmtRunErr(runWithContext(ctx, t, `incus config device add %s vtpm tpm`, name))
	if err != nil {
		return err
	}

	err = fmtRunErr(runWithContext(ctx, t, `incus config device add %s boot-media disk pool=default source=%s boot.priority=10`, name, incusOSPreseededISOFilename))
	if err != nil {
		return err
	}

	err = fmtRunErr(runWithContext(ctx, t, `incus config set %s systemd.credential.fully-enable-incus-agent=true`, name))
	if err != nil {
		return err
	}

	err = startInstanceWithContext(ctx, t, name)
	if err != nil {
		return err
	}

	t.Logf("Waiting for %s to complete installation", name)
	agentWaitCtx, cancel := context.WithTimeout(ctx, strechedTimeout(5*time.Minute))
	err = waitAgentRunningWithContext(agentWaitCtx, t, name)
	cancel()
	if err != nil {
		return err
	}

	logWaitCtx, cancel := context.WithTimeout(ctx, strechedTimeout(10*time.Minute))
	defer cancel()

	return waitExpectedLogWithContext(logWaitCtx, t, "%s", "incus-osd", "IncusOS was successfully installed|System is ready", true, name)
}

func asJSON(t *testing.T, in any) string {
	t.Helper()

	b, err := json.Marshal(in)
	require.NoError(t, err)

	return string(b)
}

func printServerList(t *testing.T) {
	t.Helper()

	stop := timeTrack(t)
	defer stop()

	resp := mustRun(t, "../bin/operations-center.linux.%s provisioning server list", cpuArch)
	fmt.Println(resp.Output())
}

func printUpdateList(t *testing.T) {
	t.Helper()

	stop := timeTrack(t)
	defer stop()

	resp := mustRun(t, "../bin/operations-center.linux.%s provisioning update list", cpuArch)
	fmt.Println(resp.Output())
}
