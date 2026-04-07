package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func assertOperationsCenterSelfRegistration(t *testing.T) {
	t.Helper()

	t.Log("Assert operations-center is self-registered")

	resp := run(t, `../bin/operations-center.linux.%s provisioning server list -f json | jq -r -e '[ .[] | select(.name == "operations-center") ] | length == 1'`, cpuArch)
	require.NoError(t, resp.err, "expect operations-center to be self registered")
	if !resp.Success() {
		t.Errorf("expect operations-center to be self registered")
		fmt.Println("====[ Server List ]====")
		resp := mustRun(t, "../bin/operations-center.linux.%s provisioning server list", cpuArch)
		fmt.Println(resp.Output())
	}

	require.True(t, resp.Success(), "failed to assert self registration of operations-center")
}

func assertOperationsCenterCliAdmin(t *testing.T) {
	t.Helper()

	var resp cmdResponse
	success := true

	t.Log("Assert operations-center cli admin")

	resp = run(t, `../bin/operations-center.linux.%s admin os show -f json | jq -r -e '.environment.os_name == "IncusOS"'`, cpuArch)
	require.NoError(t, resp.err, "expect operations center OS to be IncusOS")
	if !resp.Success() {
		t.Errorf("expect operations center OS to be IncusOS")
		success = false
		resp = mustRun(t, "../bin/operations-center.linux.%s admin os show -f json", cpuArch)
		fmt.Println(resp.Output())
	}

	resp = run(t, `../bin/operations-center.linux.%s admin os application list -f json | jq -r -e '(. | length >= 1) and ([ .[] | select(contains("operations-center")) ] | length == 1)'`, cpuArch)
	require.NoError(t, resp.err, "expect operations center application to be installed")
	if !resp.Success() {
		t.Errorf("expect operations center application to be installed")
		success = false
		resp = mustRun(t, "../bin/operations-center.linux.%s admin os application list -f json", cpuArch)
		fmt.Println(resp.Output())
	}

	resp = run(t, `../bin/operations-center.linux.%s admin os application show operations-center -f json | jq -r -e '.state.initialized'`, cpuArch)
	require.NoError(t, resp.err, "expect operations center application to be initialized")
	if !resp.Success() {
		t.Errorf("expect operations center application to be initialized")
		success = false
		resp = mustRun(t, "../bin/operations-center.linux.%s admin os application show operations-center -f json", cpuArch)
		fmt.Println(resp.Output())
	}

	resp = run(t, `../bin/operations-center.linux.%s admin os debug log -u operations-center -n 10`, cpuArch)
	require.NoError(t, resp.err, "expect operations center debug log to be fetchable")
	if !resp.Success() {
		t.Errorf("expect operations center debug log to be fetchable")
		success = false
		fmt.Println(resp.Output())
	}

	resp = run(t, `../bin/operations-center.linux.%s admin os debug processes | grep operations-center`, cpuArch)
	require.NoError(t, resp.err, "expect operations center process to be contained in the process output")
	if !resp.Success() {
		t.Errorf("expect operations center process to be contained in the process output")
		success = false
		resp := mustRun(t, "../bin/operations-center.linux.%s admin os debug processes", cpuArch)
		fmt.Println(resp.Output())
	}

	resp = run(t, `../bin/operations-center.linux.%s admin os service list -f json | jq -r -e '. | length > 0'`, cpuArch)
	require.NoError(t, resp.err, "expect operations center to have services")
	if !resp.Success() {
		t.Errorf("expect operations center to have services")
		success = false
		resp := mustRun(t, "../bin/operations-center.linux.%s admin os service list -f json", cpuArch)
		fmt.Println(resp.Output())
	}

	resp = run(t, `../bin/operations-center.linux.%s admin os system network show -f json | jq -r -e '. | keys | length >= 2'`, cpuArch)
	require.NoError(t, resp.err, "expect operations center to system network output to contain config and state")
	if !resp.Success() {
		t.Errorf("expect operations center to system network output to contain config and state")
		success = false
		resp := mustRun(t, "../bin/operations-center.linux.%s admin os system network show -f json", cpuArch)
		fmt.Println(resp.Output())
	}

	require.True(t, success, "operations-center cli admin assertions failed")
}

func assertOperationsCenterCliQuery(t *testing.T) {
	t.Helper()

	var resp cmdResponse
	success := true

	t.Log("Assert operations-center cli query")

	resp = run(t, `../bin/operations-center.linux.%s query /system/settings | jq -r -e '.metadata | keys | length > 0'`, cpuArch)
	require.NoError(t, resp.err, "expect operations center query command to work")
	if !resp.Success() {
		t.Errorf("expect operations center query command to work")
		success = false
		resp = mustRun(t, "../bin/operations-center.linux.%s query /system/settings", cpuArch)
		fmt.Println(resp.Output())
	}

	require.True(t, success, "operations-center cli query assertions failed")
}

func assertOperationsCenterCliSystem(t *testing.T) {
	t.Helper()

	var resp cmdResponse
	success := true

	t.Log("Assert operations-center cli system")

	resp = run(t, `../bin/operations-center.linux.%s system network show`, cpuArch)
	require.NoError(t, resp.err, "expect operations center system network show to work")
	if !resp.Success() {
		t.Errorf("expect operations center system network show to work")
		success = false
		resp = mustRun(t, "../bin/operations-center.linux.%s system network show", cpuArch)
		fmt.Println(resp.Output())
	}

	resp = run(t, `../bin/operations-center.linux.%s system security show`, cpuArch)
	require.NoError(t, resp.err, "expect operations center system security show to work")
	if !resp.Success() {
		t.Errorf("expect operations center system security show to work")
		success = false
		resp = mustRun(t, "../bin/operations-center.linux.%s system security show", cpuArch)
		fmt.Println(resp.Output())
	}

	resp = run(t, `../bin/operations-center.linux.%s system settings show`, cpuArch)
	require.NoError(t, resp.err, "expect operations center system settings show to work")
	if !resp.Success() {
		t.Errorf("expect operations center system settings show to work")
		success = false
		resp = mustRun(t, "../bin/operations-center.linux.%s system settings show", cpuArch)
		fmt.Println(resp.Output())
	}

	resp = run(t, `../bin/operations-center.linux.%s system updates show`, cpuArch)
	require.NoError(t, resp.err, "expect operations center system updates show to work")
	if !resp.Success() {
		t.Errorf("expect operations center system updates show to work")
		success = false
		resp = mustRun(t, "../bin/operations-center.linux.%s system updates show", cpuArch)
		fmt.Println(resp.Output())
	}

	require.True(t, success, "operations-center cli system assertions failed")
}

func assertIncusRemote(t *testing.T, clusterName string) {
	t.Helper()

	t.Log("Add incus remote")

	resp := mustRun(t, `../bin/operations-center.linux.%s provisioning cluster list -f json | jq -r '.[] | select(.name == "%s") | .connection_url'`, cpuArch, clusterName)
	clusterConnectionURL := resp.OutputTrimmed()

	mustRun(t, `incus remote add --accept-certificate --auth-type tls %s %s`, clusterName, clusterConnectionURL)
	t.Cleanup(func() {
		if noCleanup {
			return
		}

		// In t.Cleanup, t.Context() is cancelled, so we need a detached context.
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		mustRunWithContext(ctx, t, `incus remote remove %s`, clusterName)
	})

	mustRun(t, `incus cluster list %s: -f json | jq -r -e '. | length == 3'`, clusterName)
}

func assertInventory(t *testing.T, clusterName string) {
	t.Helper()

	var resp cmdResponse
	success := true

	t.Log("Assert inventory content after cluster creation")

	resp = run(t, `../bin/operations-center.linux.%s provisioning cluster list -f json | jq -r -e '[ .[] | select(.name == "%s") ] | length == 1'`, cpuArch, clusterName)
	require.NoError(t, resp.err, "expect 1 cluster entry with name %s", clusterName)
	if !resp.Success() {
		t.Errorf("expect 1 cluster entry with name %s", clusterName)
		success = false
		fmt.Println("====[ Cluster List ]====")
		resp := mustRun(t, "../bin/operations-center.linux.%s provisioning cluster list", cpuArch)
		fmt.Println(resp.Output())
	}

	resp = run(t, `../bin/operations-center.linux.%s provisioning server list -f json | jq -r -e '[ .[] | select(.server_type == "incus" and .server_status == "ready") ] | length == 3'`, cpuArch)
	require.NoError(t, resp.err, "expect 3 incus servers in ready state")
	if !resp.Success() {
		t.Error("expect 3 incus servers in ready state")
		success = false
		fmt.Println("====[ Server List ]====")
		resp := mustRun(t, "../bin/operations-center.linux.%s provisioning server list", cpuArch)
		fmt.Println(resp.Output())
	}

	resp = run(t, `../bin/operations-center.linux.%s provisioning server list -f json | jq -r -e '[ .[] | select(.server_type == "operations-center" and .server_status == "ready") ] | length == 1'`, cpuArch)
	require.NoError(t, resp.err, "expect 1 operations-center in ready state")
	if !resp.Success() {
		t.Error("expect 1 operations-center in ready state")
		success = false
		fmt.Println("====[ Server List ]====")
		resp := mustRun(t, "../bin/operations-center.linux.%s provisioning server list", cpuArch)
		fmt.Println(resp.Output())
	}

	// Performing cluster resync of inventory data.
	mustRun(t, "../bin/operations-center.linux.%s provisioning cluster resync %s", cpuArch, clusterName)

	resp = run(t, `../bin/operations-center.linux.%s inventory network list -f json | jq -r -e '[ .[] | select(.cluster == "%s") | .name ] | length == 2'`, cpuArch, clusterName)
	require.NoError(t, resp.err, "expect 2 networks: incusbr0, meshbr0")
	if !resp.Success() {
		t.Error("expect 2 networks: incusbr0, meshbr0")
		success = false
		fmt.Println("====[ Network List ]====")
		resp = mustRun(t, "../bin/operations-center.linux.%s inventory network list", cpuArch)
		fmt.Println(resp.Output())
	}

	resp = run(t, `../bin/operations-center.linux.%s inventory profile list -f json | jq -r -e '[ .[] | select(.cluster == "%s") | .name ] | length == 2'`, cpuArch, clusterName)
	require.NoError(t, resp.err, "expect 2 profiles: default, internal")
	if !resp.Success() {
		t.Error("expect 2 profiles: default, internal")
		success = false
		fmt.Println("====[ Profile List ]====")
		resp = mustRun(t, "../bin/operations-center.linux.%s inventory profile list", cpuArch)
		fmt.Println(resp.Output())
	}

	resp = run(t, `../bin/operations-center.linux.%s inventory project list -f json | jq -r -e '[ .[] | select(.cluster == "%s") | .name ] | length == 2'`, cpuArch, clusterName)
	require.NoError(t, resp.err, "expect 2 profiles: default, internal")
	if !resp.Success() {
		t.Error("expect 2 profiles: default, internal")
		success = false
		fmt.Println("====[ Project List ]====")
		resp = mustRun(t, "../bin/operations-center.linux.%s inventory project list", cpuArch)
		fmt.Println(resp.Output())
	}

	resp = run(t, `../bin/operations-center.linux.%s inventory storage-pool list -f json | jq -r -e '[ .[] | select(.cluster == "%s") | .name ] | length == 1'`, cpuArch, clusterName)
	require.NoError(t, resp.err, "expect 1 storage pool: local")
	if !resp.Success() {
		t.Error("expect 1 storage pool: local")
		success = false
		fmt.Println("====[ Storage Pool List ]====")
		resp = mustRun(t, "../bin/operations-center.linux.%s inventory storage-pool list", cpuArch)
		fmt.Println(resp.Output())
	}

	resp = run(t, `../bin/operations-center.linux.%s inventory storage-volume list -f json | jq -r -e '[ .[] | select(.cluster == "%s") | .name ] | length == 9'`, cpuArch, clusterName)
	require.NoError(t, resp.err, "expect 9 storage-volumes: images and backups for each server")
	if !resp.Success() {
		t.Error("expect 9 storage-volumes: images and backups for each server")
		success = false
		fmt.Println("====[ Storage Volume List ]====")
		resp = mustRun(t, "../bin/operations-center.linux.%s inventory storage-volume list", cpuArch)
		fmt.Println(resp.Output())
	}

	require.True(t, success, "inventory assertions failed")
}

func assertTerraformArtifact(t *testing.T, clusterName string) {
	t.Helper()

	var resp cmdResponse
	success := true

	tmpDir := t.TempDir()

	t.Log("List cluster artifacts")
	resp = run(t, `../bin/operations-center.linux.%[1]s provisioning cluster artifact list %[2]s -f json | jq -r -e '[ .[] | select(.cluster == "%[2]s") ] | length == 1'`, cpuArch, clusterName)
	require.NoError(t, resp.err, "expect 1 artifact for cluster %s: terraform-cofiguration", clusterName)
	if !resp.Success() {
		success = false
		fmt.Println("====[ Cluster List ]====")
		resp := mustRun(t, "../bin/operations-center.linux.%s provisioning cluster artifact list %s", cpuArch, clusterName)
		fmt.Println(resp.Output())
	}

	require.True(t, success, "terraform artifact assertion failed")

	t.Log("Fetch terraform-configuration cluster artifact")
	mustRun(t, `../bin/operations-center.linux.%s provisioning cluster artifact archive %s terraform-configuration %s/terraform.zip`, cpuArch, clusterName, tmpDir)

	t.Log("Uncompress terraform-configuration cluster artifact")
	mustRun(t, `unzip %[1]s/terraform.zip -d %[1]s`, tmpDir)

	t.Log("Terrafrom init")
	mustRun(t, `tofu -chdir=%s init`, tmpDir)

	t.Log("Terraform plan")
	mustRun(t, `tofu -chdir=%s plan`, tmpDir)
}

func assertWebsocketEventsInventoryUpdate(t *testing.T, clusterName string) {
	t.Helper()

	var resp cmdResponse
	success := true

	t.Log("Launch instance to trigger websocket event")
	mustRun(t, `incus launch images:alpine/edge %s:c1`, clusterName)

	t.Log("Wait for inventory update")
	ok, err := waitForSuccessWithTimeout(t, "instance list", `../bin/operations-center.linux.%s inventory instance list -f json | jq -r -e '[ .[] | select(.cluster == "%s") | .name ] | length == 1'`, 30*time.Second, cpuArch, clusterName)
	require.NoError(t, err, "expect 1 instance: c1")
	if !ok {
		success = false
		fmt.Println("====[ Instance List ]====")
		resp = mustRun(t, "../bin/operations-center.linux.%s inventory instance list", cpuArch)
		fmt.Println(resp.Output())
	}

	require.True(t, success, "inventory assertions failed after websocket events")
}
