package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"golang.org/x/sync/errgroup"
)

func setupOperationsCenter(t *testing.T, useSnapshots bool, tmpDir string) {
	t.Helper()

	stop := timeTrack(t)
	defer stop()

	names := []string{"OperationsCenter"}
	if useSnapshots && hasSnapshot(t, "setupOperationsCenter", names) {
		if !hasLastSnapshot(t, "setupOperationsCenter", names) {
			return
		}

		restoreSnapshot(t, "setupOperationsCenter", names)
		replaceOperationsCenterExecutable(t)
		mustWaitOperationsCenterAPIReady(t)
		return
	}

	getOperationsCenterIncusOSISO(t, tmpDir)

	importOperationsCenterIncusOSISOStorageVolume(t, tmpDir)

	installOperationsCenterVM(t)

	removeBootMedia(t)

	mustWaitAgentRunning(t, "OperationsCenter")

	mustWaitExpectedLog(t, "OperationsCenter", "incus-osd", "System is ready")

	replaceOperationsCenterExecutable(t)

	setupLocalOperationsCenterConfig(t)

	mustWaitUpdatesReady(t)

	if useSnapshots {
		createSnapshot(t, "setupOperationsCenter", names)
	}
}

func setupIncusOSWithToken(t *testing.T, useSnapshots bool, tmpDir string) {
	t.Helper()

	stop := timeTrack(t)
	defer stop()

	names := []string{"OperationsCenter", "IncusOS01", "IncusOS02", "IncusOS03"}
	if useSnapshots && hasSnapshot(t, "setup", names) {
		// Restore OperationsCenter first for it to be running when the Incus instances are restored.
		restoreSnapshot(t, "setup", names[:1])
		replaceOperationsCenterExecutable(t)
		mustWaitOperationsCenterAPIReady(t)

		restoreSnapshot(t, "setup", names[1:])
		return
	}

	token := createProvisioningToken(t)

	incusOSPreseededISOFilename := createIncusOSPreseededISO(t, tmpDir, token)

	importIncusOSISOStorageVolume(t, tmpDir, token, incusOSPreseededISOFilename)

	createIncusOSInstances(t, token)

	printServerList(t)

	if useSnapshots {
		createSnapshot(t, "setup", names)
	}
}

func cleanupIncusOS(t *testing.T, useSnapshots bool) func() {
	t.Helper()

	return func() {
		// In t.Cleanup, t.Context() is cancelled, so we need a detached context.
		ctx, cancel := context.WithTimeout(context.Background(), strechedTimeout(30*time.Second))
		defer cancel()

		names := []string{"IncusOS01", "IncusOS02", "IncusOS03"}
		for _, name := range names {
			mustRunWithContext(ctx, t, `incus remove --force %s`, name)
		}

		if useSnapshots {
			mustRunWithContext(ctx, t, `incus snapshot remove OperationsCenter setup`)
		}
	}
}

func setupIncusOSWithTokenSeed(t *testing.T, useSnapshots bool, tmpDir string) {
	t.Helper()

	stop := timeTrack(t)
	defer stop()

	names := []string{"OperationsCenter", "IncusOS01", "IncusOS02", "IncusOS03"}
	if useSnapshots && hasSnapshot(t, "setup", names) {
		// Restore OperationsCenter first for it to be running when the Incus instances are restored.
		restoreSnapshot(t, "setup", names[:1])
		replaceOperationsCenterExecutable(t)
		mustWaitOperationsCenterAPIReady(t)

		restoreSnapshot(t, "setup", names[1:])
		return
	}

	token := createProvisioningToken(t)

	incusOSPreseededISOFilename := createIncusOSPreseededISOFromTokenSeed(t, tmpDir, token)

	importIncusOSISOStorageVolume(t, tmpDir, token, incusOSPreseededISOFilename)

	createIncusOSInstances(t, token)

	printServerList(t)

	if useSnapshots {
		createSnapshot(t, "setup", names)
	}
}

func hasSnapshot(t *testing.T, snapshot string, names []string) bool {
	t.Helper()

	stop := timeTrack(t)
	defer stop()

	for _, name := range names {
		snapshotExistsRes, err := run(t, `incus snapshot list %s -f json | jq -r -e '[ .[] | select(.name == "%s") ] | length > 0'`, name, snapshot)
		require.NoError(t, err)
		if !snapshotExistsRes.Success() {
			return false
		}
	}

	return true
}

func hasLastSnapshot(t *testing.T, snapshot string, names []string) bool {
	t.Helper()

	stop := timeTrack(t)
	defer stop()

	for _, name := range names {
		snapshotExistsRes, err := run(t, `incus snapshot list %s -f json | jq -r -e '. | last | .name == "%s"'`, name, snapshot)
		require.NoError(t, err)
		if !snapshotExistsRes.Success() {
			return false
		}
	}

	return true
}

func restoreSnapshot(t *testing.T, snapshot string, names []string) {
	t.Helper()

	stop := timeTrack(t)
	defer stop()

	errgrp, errgrpctx := errgroup.WithContext(t.Context())
	ok, _ := strconv.ParseBool(concurrentSetup)
	if !ok {
		errgrp.SetLimit(1)
	}

	for _, name := range names {
		errgrp.Go(func() (err error) {
			stop := timeTrack(t, fmt.Sprintf("restoreSnapshot %s", name), "false")
			defer stop()

			defer func() {
				if err != nil {
					err = fmt.Errorf("%s: %w", name, err)
				}
			}()

			err = fmtRunErr(runWithContext(errgrpctx, t, "incus snapshot restore %s %s", name, snapshot))
			if err != nil {
				return err
			}

			err = waitAgentRunningWithContext(errgrpctx, t, name)
			if err != nil {
				return err
			}

			err = waitExpectedLogWithContext(errgrpctx, t, name, "incus-osd", "System is ready", false)
			if err != nil {
				return err
			}

			return nil
		})
	}

	err := errgrp.Wait()
	require.NoError(t, err, "failed to restore %v VMs snapshot", names)
}

func getClientCertificate(t *testing.T) string {
	t.Helper()

	homeDir, err := os.UserHomeDir()
	require.NoError(t, err)
	clientCertPath := filepath.Join(homeDir, ".config/incus/client.crt")

	if !isFile(clientCertPath) {
		stop := timeTrack(t)
		defer stop()

		_, err := run(t, `incus remote generate-certificate`)
		require.NoError(t, err)
	}

	clientCertificate, err := os.ReadFile(clientCertPath)
	require.NoError(t, err)

	return string(clientCertificate)
}

func getOperationsCenterIncusOSISO(t *testing.T, tmpDir string) {
	t.Helper()

	if !isFile(filepath.Join(tmpDir, "IncusOS_OperationsCenter.iso")) {
		stop := timeTrack(t)
		defer stop()

		clientCertificate := getClientCertificate(t)

		clientCertificateJSONString, err := json.Marshal(clientCertificate)
		require.NoError(t, err)

		operationsCenterSeed := replacePlaceholders(operationsCenterSeedTemplate,
			map[string]string{
				"$CLIENT_CERTIFICATE$": string(clientCertificateJSONString),
			},
		)

		req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "https://incusos-customizer.linuxcontainers.org/1.0/images", bytes.NewBuffer(operationsCenterSeed))
		require.NoError(t, err)

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		imagesData, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		err = resp.Body.Close()
		require.NoError(t, err)

		imageDownloadURL := gjson.GetBytes(imagesData, "metadata.image").String()

		mustRunWithTimeout(t, `curl -o %s --compressed https://incusos-customizer.linuxcontainers.org%s`, 5*time.Minute, filepath.Join(tmpDir, "IncusOS_OperationsCenter.iso"), imageDownloadURL)
	}
}

func importOperationsCenterIncusOSISOStorageVolume(t *testing.T, tmpDir string) {
	t.Helper()

	storageVolumes := mustRun(t, "incus storage volume list default -f compact")
	if !strings.Contains(storageVolumes.Output(), "IncusOS_OperationsCenter.iso") {
		stop := timeTrack(t)
		defer stop()

		mustRunWithTimeout(t, `incus storage volume import default %s IncusOS_OperationsCenter.iso --type=iso`, 5*time.Minute, filepath.Join(tmpDir, "IncusOS_OperationsCenter.iso"))
	}
}

func installOperationsCenterVM(t *testing.T) {
	t.Helper()

	incusInstanceList := mustRun(t, "incus list -f compact")
	if !regexp.MustCompile(`OperationsCenter\s+RUNNING`).MatchString(incusInstanceList.Output()) {
		stop := timeTrack(t)
		defer stop()

		mustRun(t, `incus init --empty --vm OperationsCenter -c security.secureboot=false -c limits.cpu=%s -c limits.memory=%s -d root,size=%s`, cpuCount, memorySize, diskSize)
		mustRun(t, `incus config device add OperationsCenter vtpm tpm`)
		mustRun(t, `incus config device add OperationsCenter boot-media disk pool=default source=IncusOS_OperationsCenter.iso boot.priority=10`)
		mustRun(t, `incus start OperationsCenter`)

		t.Log("Waiting for Operations Center to complete installation")
		mustWaitAgentRunningWithTimeout(t, "OperationsCenter", 5*time.Minute)
		mustWaitExpectedLogWithTimeout(t, "OperationsCenter", "incus-osd", "IncusOS was successfully installed", 5*time.Minute)
	}
}

func removeBootMedia(t *testing.T) {
	t.Helper()

	instanceHasBootMedia := mustRun(t, "incus config device list OperationsCenter")
	if strings.Contains(instanceHasBootMedia.Output(), "boot-media") {
		stop := timeTrack(t)
		defer stop()

		_, err := run(t, `incus stop OperationsCenter`)
		require.NoError(t, err)
		mustRun(t, `incus config device remove OperationsCenter boot-media`)
		mustRun(t, `incus start OperationsCenter`)

		t.Log("Waiting for Operations Center to be ready")
	}
}

func replaceOperationsCenterExecutable(t *testing.T) {
	t.Helper()

	stop := timeTrack(t)
	defer stop()

	mustRun(t, `incus exec OperationsCenter -- bash -c "mkdir -p /root/dev/ && mount -t tmpfs tmpfs /root/dev/"`)
	mustRun(t, `incus exec OperationsCenter -- bash -c "systemctl stop operations-center || true"`)
	mustRun(t, `incus exec OperationsCenter -- bash -c "umount -l /usr/local/bin/operations-centerd || true"`)
	mustRun(t, `incus file push ../bin/operations-centerd OperationsCenter/root/dev/operations-centerd`)
	mustRun(t, `incus exec OperationsCenter -- bash -c "mount -o bind /root/dev/operations-centerd /usr/local/bin/operations-centerd && systemctl start operations-center"`)
}

func setupLocalOperationsCenterConfig(t *testing.T) {
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
	operationsCenterIPAddressResp := mustRun(t, `incus list -f json | jq -r '.[] | select(.name == "OperationsCenter") | .state.network | to_entries[] | .value.addresses[]? | select(.family == "inet" and .scope == "global") | .address' | head -n1`)

	operationsCenterIPAddress := strings.TrimSpace(operationsCenterIPAddressResp.Output())

	var operationsCenterCetificate string
	for {
		operationsCenterCetificateResp, err := runWithTimeout(t, `/usr/bin/openssl s_client -connect %s:8443 </dev/null 2>/dev/null | openssl x509 -outform PEM`, 30*time.Second, operationsCenterIPAddress)
		require.NoError(t, err)
		if strings.Contains(operationsCenterCetificateResp.Output(), "-----BEGIN CERTIFICATE-----") {
			operationsCenterCetificate = indent(operationsCenterCetificateResp.Output(), strings.Repeat(" ", 6))
			break
		}
	}

	operationsCenterConfigYAML := replacePlaceholders(operationsCenterConfigYAMLTemplate,
		map[string]string{
			"$OPERATIONS_CENTER_IPADDRESS$":   operationsCenterIPAddress,
			"$OPERATIONS_CENTER_CERTIFICATE$": operationsCenterCetificate,
		},
	)

	err = os.WriteFile(filepath.Join(homeDir, ".config/operations-center/config.yml"), operationsCenterConfigYAML, 0o600)
	require.NoError(t, err)
}

func createProvisioningToken(t *testing.T) string {
	t.Helper()

	tokenResp := mustRun(t, `../bin/operations-center.linux.%s provisioning token list -f json | jq -r '[ .[] | select(.uses_remaining > 20) ]'`, cpuArch)
	token := gjson.Get(tokenResp.Output(), "0.uuid").String()
	if token == "" {
		stop := timeTrack(t)
		defer stop()

		mustRun(t, `../bin/operations-center.linux.%s provisioning token add --description "test" --uses 50`, cpuArch)
		tokenResp := mustRun(t, `../bin/operations-center.linux.%s provisioning token list -f json | jq -r '[ .[] | select(.uses_remaining > 20) ]'`, cpuArch)
		token = gjson.Get(tokenResp.Output(), "0.uuid").String()
	}

	return token
}

func createIncusOSPreseededISO(t *testing.T, tmpDir string, token string) string {
	t.Helper()

	incusOSPreseededISOFilename := fmt.Sprintf("IncusOS-preseeded-%[1]s.iso", token)
	if !isFile(filepath.Join(tmpDir, incusOSPreseededISOFilename)) {
		stop := timeTrack(t)
		defer stop()

		clientCertificate := getClientCertificate(t)

		incusOSSeedFileYAML := replacePlaceholders(incusOSSeedFileYAMLTemplate,
			map[string]string{
				"$CLIENT_CERTIFICATE$": indent(clientCertificate, strings.Repeat(" ", 10)),
			},
		)

		err := os.WriteFile(filepath.Join(tmpDir, "incusos_seed.yaml"), incusOSSeedFileYAML, 0o600)
		require.NoError(t, err)

		mustRunWithTimeout(t, `../bin/operations-center.linux.%[1]s provisioning token get-image %[2]s %[3]s/%[4]s %[3]s/incusos_seed.yaml`, 10*time.Minute, cpuArch, token, tmpDir, incusOSPreseededISOFilename)
	}

	return incusOSPreseededISOFilename
}

func createIncusOSPreseededISOFromTokenSeed(t *testing.T, tmpDir string, token string) string {
	t.Helper()

	incusOSPreseededISOFilename := fmt.Sprintf("IncusOS-preseeded-%[1]s.iso", token)
	if !isFile(filepath.Join(tmpDir, incusOSPreseededISOFilename)) {
		stop := timeTrack(t)
		defer stop()

		clientCertificate := getClientCertificate(t)

		incusOSSeedFileYAML := replacePlaceholders(incusOSSeedFileYAMLTemplate,
			map[string]string{
				"$CLIENT_CERTIFICATE$": indent(clientCertificate, strings.Repeat(" ", 10)),
			},
		)

		err := os.WriteFile(filepath.Join(tmpDir, "incusos_seed.yaml"), incusOSSeedFileYAML, 0o600)
		require.NoError(t, err)

		mustRun(t, `../bin/operations-center.linux.%s provisioning token seed add %s incus-os-cluster %s/incusos_seed.yaml`, cpuArch, token, tmpDir)
		mustRunWithTimeout(t, `../bin/operations-center.linux.%s provisioning token seed get-image %s incus-os-cluster %s/%s`, 10*time.Minute, cpuArch, token, tmpDir, incusOSPreseededISOFilename)
	}

	return incusOSPreseededISOFilename
}

func importIncusOSISOStorageVolume(t *testing.T, tmpDir string, token string, incusOSPreseededISOFilename string) {
	t.Helper()

	storageVolumes := mustRun(t, "incus storage volume list default -f compact")
	if !strings.Contains(storageVolumes.Output(), incusOSPreseededISOFilename) {
		stop := timeTrack(t)
		defer stop()

		mustRunWithTimeout(t, `incus storage volume import default %[2]s/%[3]s %[3]s --type=iso`, 5*time.Minute, token, tmpDir, incusOSPreseededISOFilename)
	}
}

func createIncusOSInstances(t *testing.T, token string) {
	t.Helper()

	stop := timeTrack(t)
	defer stop()

	timeoutCtx, cancel := context.WithTimeout(t.Context(), strechedTimeout(20*time.Minute))
	defer cancel()

	errgrp, errgrpctx := errgroup.WithContext(timeoutCtx)
	ok, _ := strconv.ParseBool(concurrentSetup)
	if !ok {
		errgrp.SetLimit(1)
	}

	names := []string{"IncusOS01", "IncusOS02", "IncusOS03"}
	for _, name := range names {
		errgrp.Go(func() (err error) {
			stop := timeTrack(t, fmt.Sprintf("createIncusOSInstance %s", name), "false")
			defer stop()

			defer func() {
				if err != nil {
					err = fmt.Errorf("%s: %w", name, err)
				}
			}()

			incusInstanceList, err := runWithContext(errgrpctx, t, "incus list -f compact")
			err = fmtRunErr(incusInstanceList, err)
			if err != nil {
				return err
			}

			if !regexp.MustCompile(fmt.Sprintf(`%s\s+RUNNING`, name)).MatchString(incusInstanceList.Output()) {
				t.Logf("Setting up %s", name)

				err = fmtRunErr(runWithContext(errgrpctx, t, `incus init --empty --vm %s -c security.secureboot=false -c limits.cpu=%s -c limits.memory=%s -d root,size=%s`, name, cpuCount, memorySize, diskSize))
				if err != nil {
					return err
				}

				err = fmtRunErr(runWithContext(errgrpctx, t, `incus config device add %s vtpm tpm`, name))
				if err != nil {
					return err
				}

				err = fmtRunErr(runWithContext(errgrpctx, t, `incus config device add %s boot-media disk pool=default source=IncusOS-preseeded-%s.iso boot.priority=10`, name, token))
				if err != nil {
					return err
				}

				err = fmtRunErr(runWithContext(errgrpctx, t, `incus start %s`, name))
				if err != nil {
					return err
				}

				t.Logf("Waiting for %s to complete installation", name)
				err = waitAgentRunningWithContext(errgrpctx, t, name)
				if err != nil {
					return err
				}

				err = waitExpectedLogWithContext(errgrpctx, t, "%s", "incus-osd", "IncusOS was successfully installed", false, name)
				if err != nil {
					return err
				}
			}

			instanceHasBootMedia := mustRun(t, "incus config device list %s", name)
			if strings.Contains(instanceHasBootMedia.Output(), "boot-media") {
				t.Logf("Removing boot media from %s VM", name)
				_, err = run(t, `incus stop %s`, name)
				if err != nil {
					return err
				}

				err = fmtRunErr(runWithContext(errgrpctx, t, `incus config device remove %s boot-media`, name))
				if err != nil {
					return err
				}

				err = fmtRunErr(runWithContext(errgrpctx, t, `incus start %s`, name))
				if err != nil {
					return err
				}
			}

			t.Logf("Waiting for %s to be ready", name)
			err = waitAgentRunningWithContext(errgrpctx, t, name)
			if err != nil {
				return err
			}

			err = waitExpectedLogWithContext(errgrpctx, t, name, "incus-osd", "System is ready", false)
			if err != nil {
				return err
			}

			return nil
		})
	}

	err := errgrp.Wait()
	require.NoError(t, err, "Failed to create IncusOS VMs for e2e test")

	// Wait for instances to self update in Operations Center
	instanceReadyTimeoutCtx, instanceReadyCancel := context.WithTimeout(t.Context(), strechedTimeout(1*time.Minute))
	defer instanceReadyCancel()

	for {
		operationsCenterSelfRegistered := mustRunWithTimeout(t, `../bin/operations-center.linux.%s provisioning server list -f json | jq -r '[ .[] | select(.server_type == "incus" and .server_status == "ready") ] | length == 3'`, 10*time.Second, cpuArch)

		ok, _ := strconv.ParseBool(strings.TrimSpace(operationsCenterSelfRegistered.Output()))
		if ok {
			break
		}

		select {
		case <-instanceReadyTimeoutCtx.Done():
			require.NoError(t, instanceReadyTimeoutCtx.Err())

		case <-time.After(time.Second):
		}
	}
}

func printServerList(t *testing.T) {
	t.Helper()

	stop := timeTrack(t)
	defer stop()

	resp := mustRun(t, "../bin/operations-center.linux.%s provisioning server list", cpuArch)
	fmt.Println(resp.Output())
}

func createSnapshot(t *testing.T, snapshot string, names []string) {
	t.Helper()

	stop := timeTrack(t)
	defer stop()

	for _, name := range names {
		mustRun(t, `incus exec %s -- sync`, name)
		snapshotExistsRes := mustRun(t, `incus snapshot list %s -f json | jq -r '[ .[] | select(.name == "%s") ] | length > 0'`, name, snapshot)
		snapshotExists, _ := strconv.ParseBool(strings.TrimSpace(snapshotExistsRes.Output()))
		if snapshotExists {
			continue
		}

		mustRun(t, "incus snapshot create %s %s", name, snapshot)
	}
}
