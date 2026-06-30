package e2e

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v4"
)

func basicOperationsCenterInteractions(t *testing.T, tmpDir string) {
	t.Helper()

	assertOperationsCenterCliAdmin(t)
	assertOperationsCenterCliQuery(t)
	assertOperationsCenterCliSystem(t)
	assertOperationsCenterCliProvisioningTokenSeed(t, tmpDir)
	assertOperationsCenterCliProvisioningClusterTemplate(t, tmpDir)
}

func basicOperationsCenterInteractionsUpdatesCleanupAndRefresh(t *testing.T, tmpDir string) {
	t.Helper()

	assertOperationsCenterCliUpdateCleanupAndRefresh(t)
}

func registerServer(t *testing.T, tmpDir string) {
	t.Helper()

	assertServerRegistrationScriptletEffects(t)
}

func ocIncusImagesRemoteLaunchInstance(names []string) func(t *testing.T, tmpDir string) {
	return func(t *testing.T, tmpDir string) {
		t.Helper()

		ctx := t.Context()

		createCluster(names)(t, tmpDir)

		// Only test on the first server
		name := names[0]

		t.Logf("Apply operations-center server certificate to %s", name)
		resp := mustRun(t, `incus exec OperationsCenter -- cat /var/lib/operations-center/server.crt`)
		ocServerCert := resp.OutputTrimmed()

		resp = mustRun(t, `../bin/operations-center.linux.%s provisioning server os system security show %s:`, cpuArch, name)

		config := map[string]any{}
		err := yaml.Unmarshal([]byte(resp.OutputTrimmed()), &config)
		require.NoError(t, err)

		config["config"].(map[string]any)["custom_ca_certs"] = []string{ocServerCert}

		configBody, err := yaml.Marshal(config)
		require.NoError(t, err)

		configFilename := filepath.Join(tmpDir, fmt.Sprintf("system_security_config_%s.yaml", name))
		err = os.WriteFile(configFilename, configBody, 0o600)

		mustRun(t, `../bin/operations-center.linux.%s provisioning server os system security edit %s: < %s`, cpuArch, name, configFilename)

		mustRun(t, `incus restart %s`, name)

		t.Logf("Waiting for %s to be ready after restart with updated CA certificates", name)
		func() {
			timeoutCtx, cancel := context.WithTimeout(ctx, strechedTimeout(5*time.Minute))
			defer cancel()
			err = waitAgentRunningWithContext(timeoutCtx, t, name)
			require.NoError(t, err)

			err = waitExpectedLogWithContext(timeoutCtx, t, name, "incus-osd", "System is ready", false)
			require.NoError(t, err)
		}()

		t.Logf("Add /etc/hosts entry for OperationsCenter on %s", name)
		resp = mustRun(t, `incus list -f json | jq -r '.[] | select(.name == "OperationsCenter") | .state.network | to_entries[] | .value.addresses[]? | select(.family == "inet" and .scope == "global") | .address' | head -n1`)
		ocIPAddress := resp.OutputTrimmed()
		resp = mustRun(t, `incus exec OperationsCenter -- hostname`)
		ocHostname := resp.OutputTrimmed()
		mustRun(t, `incus exec %s -- bash -c "echo '%s	%s' >> /etc/hosts"`, name, ocIPAddress, ocHostname)

		t.Logf("Download images")
		resp = mustRun(t, `curl -s "https://images.linuxcontainers.org/streams/v1/images.json" | jq -r '.products."alpine:edge:%s:default".versions | keys | last'`, cpuArch)
		currentAlpineVersion := resp.OutputTrimmed()

		tmpImagesDir := filepath.Join(tmpDir, "images")
		err = os.MkdirAll(tmpImagesDir, 0o700)
		require.NoError(t, err)
		mustRun(t, `curl "https://images.linuxcontainers.org/images/alpine/edge/%s/default/%s/incus.tar.xz" -sLo %s/incus.tar.xz`, cpuArch, currentAlpineVersion, tmpImagesDir)
		mustRun(t, `curl "https://images.linuxcontainers.org/images/alpine/edge/%s/default/%s/rootfs.squashfs" -sLo %s/root.squashfs`, cpuArch, currentAlpineVersion, tmpImagesDir)
		mustRun(t, `curl "https://images.linuxcontainers.org/images/alpine/edge/%s/default/%s/disk.qcow2" -sLo %s/disk.qcow2`, cpuArch, currentAlpineVersion, tmpImagesDir)
		resp = mustRun(t, `ls -l %s`, tmpImagesDir)
		fmt.Println(resp.Output())

		t.Logf("Add images to operations-center")
		mustRun(t, `../bin/operations-center.linux.%[1]s image incus add %[2]s/incus.tar.xz %[2]s/root.squashfs %[2]s/disk.qcow2`, cpuArch, tmpImagesDir)
		resp = mustRun(t, `../bin/operations-center.linux.%[1]s image incus list`, cpuArch)
		fmt.Println(resp.Output())

		t.Cleanup(ocIncusImagesCleanup(t))

		t.Logf("Add operations-center as image remote to %s", name)
		mustRun(t, `incus exec %s -- incus remote add operations-center-images https://%s:8443/incus-images --protocol simplestreams`, name, ocHostname)
		resp = mustRun(t, `incus exec %s -- incus image list operations-center-images:`, name)
		fmt.Println(resp.Output())

		t.Logf("Start container from operations-center-images remote on %s", name)
		resp = mustRun(t, `incus exec %s -- incus image list operations-center-images: --format json | jq -r '[ .[] | select(.type == "container" and .properties.os == "alpinelinux") | .fingerprint ] | first'`, name)
		alpineContainerFingerprint := resp.OutputTrimmed()

		mustRun(t, `incus exec %s -- incus launch operations-center-images:%s a1`, name, alpineContainerFingerprint)
		resp = mustRun(t, `incus exec %s -- incus list`, name)
		fmt.Println(resp.Output())
	}
}

func ocIncusImagesCleanup(t *testing.T) func() {
	t.Helper()

	return func() {
		if noCleanup || (noCleanupOnError && t.Failed()) {
			return
		}

		// In t.Cleanup, t.Context() is cancelled, so we need a detached context.
		ctx, cancel := context.WithTimeout(context.Background(), strechedTimeout(30*time.Second))
		defer cancel()

		stop := timeTrack(t, "Operations Center images cleanup")
		defer stop()

		resp := runWithContext(ctx, t, `../bin/operations-center.linux.%s image incus list -f json | jq -r '.[].name'`, cpuArch)
		if !resp.Success() {
			t.Error(resp.Error())
			return
		}

		for incusImage := range strings.Lines(resp.Output()) {
			incusImage = strings.TrimSpace(incusImage)
			resp := runWithContext(ctx, t, `../bin/operations-center.linux.%s image incus remove %s`, cpuArch, incusImage)
			if !resp.Success() {
				t.Error(resp.Error())
			}
		}
	}
}
