package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func assertIncusRemote(t *testing.T, clusterName string, clusterIP string) {
	t.Helper()

	t.Log("Add incus remote")

	mustRun(t, `incus remote add --accept-certificate --auth-type tls %s https://%s:8443`, clusterName, clusterIP)
	t.Cleanup(func() {
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
	var err error
	success := true

	t.Log("Assert inventory content after cluster creation")

	resp, err = run(t, `../bin/operations-center.linux.%s provisioning cluster list -f json | jq -r -e '[ .[] | select(.name == "%s") ] | length == 1'`, cpuArch, clusterName)
	require.NoError(t, err, "expect 1 cluster entry with name %s", clusterName)
	if !resp.Success() {
		t.Errorf("expect 1 cluster entry with name %s", clusterName)
		success = false
		fmt.Println("====[ Cluster List ]====")
		resp := mustRun(t, "../bin/operations-center.linux.%s provisioning cluster list", cpuArch)
		fmt.Println(resp.Output())
	}

	resp, err = run(t, `../bin/operations-center.linux.%s provisioning server list -f json | jq -r -e '[ .[] | select(.server_type == "incus" and .server_status == "ready") ] | length == 3'`, cpuArch)
	require.NoError(t, err, "expect 3 incus servers in ready state")
	if !resp.Success() {
		t.Error("expect 3 incus servers in ready state")
		success = false
		fmt.Println("====[ Server List ]====")
		resp := mustRun(t, "../bin/operations-center.linux.%s provisioning server list", cpuArch)
		fmt.Println(resp.Output())
	}

	resp, err = run(t, `../bin/operations-center.linux.%s provisioning server list -f json | jq -r -e '[ .[] | select(.server_type == "operations-center" and .server_status == "ready") ] | length == 1'`, cpuArch)
	require.NoError(t, err, "expect 1 operations-center in ready state")
	if !resp.Success() {
		t.Error("expect 1 operations-center in ready state")
		success = false
		fmt.Println("====[ Server List ]====")
		resp := mustRun(t, "../bin/operations-center.linux.%s provisioning server list", cpuArch)
		fmt.Println(resp.Output())
	}

	// Performing cluster resync of inventory data.
	mustRun(t, "../bin/operations-center.linux.%s provisioning cluster resync %s", cpuArch, clusterName)

	resp, err = run(t, `../bin/operations-center.linux.%s inventory network list -f json | jq -r -e '[ .[] | select(.cluster == "%s") | .name ] | length == 2'`, cpuArch, clusterName)
	require.NoError(t, err, "expect 2 networks: incusbr0, meshbr0")
	if !resp.Success() {
		t.Error("expect 2 networks: incusbr0, meshbr0")
		success = false
		fmt.Println("====[ Network List ]====")
		resp = mustRun(t, "../bin/operations-center.linux.%s inventory network list", cpuArch)
		fmt.Println(resp.Output())
	}

	resp, err = run(t, `../bin/operations-center.linux.%s inventory profile list -f json | jq -r -e '[ .[] | select(.cluster == "%s") | .name ] | length == 2'`, cpuArch, clusterName)
	require.NoError(t, err, "expect 2 profiles: default, internal")
	if !resp.Success() {
		t.Error("expect 2 profiles: default, internal")
		success = false
		fmt.Println("====[ Profile List ]====")
		resp = mustRun(t, "../bin/operations-center.linux.%s inventory profile list", cpuArch)
		fmt.Println(resp.Output())
	}

	resp, err = run(t, `../bin/operations-center.linux.%s inventory project list -f json | jq -r -e '[ .[] | select(.cluster == "%s") | .name ] | length == 2'`, cpuArch, clusterName)
	require.NoError(t, err, "expect 2 profiles: default, internal")
	if !resp.Success() {
		t.Error("expect 2 profiles: default, internal")
		success = false
		fmt.Println("====[ Project List ]====")
		resp = mustRun(t, "../bin/operations-center.linux.%s inventory project list", cpuArch)
		fmt.Println(resp.Output())
	}

	resp, err = run(t, `../bin/operations-center.linux.%s inventory storage-pool list -f json | jq -r -e '[ .[] | select(.cluster == "%s") | .name ] | length == 1'`, cpuArch, clusterName)
	require.NoError(t, err, "expect 1 storage pool: local")
	if !resp.Success() {
		t.Error("expect 1 storage pool: local")
		success = false
		fmt.Println("====[ Storage Pool List ]====")
		resp = mustRun(t, "../bin/operations-center.linux.%s inventory storage-pool list", cpuArch)
		fmt.Println(resp.Output())
	}

	resp, err = run(t, `../bin/operations-center.linux.%s inventory storage-volume list -f json | jq -r -e '[ .[] | select(.cluster == "%s") | .name ] | length == 6'`, cpuArch, clusterName)
	require.NoError(t, err, "expect 6 storage-volumes: images and backups for each server")
	if !resp.Success() {
		t.Error("expect 6 storage-volumes: images and backups for each server")
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
	var err error
	success := true

	tmpDir := t.TempDir()

	t.Log("List cluster artifacts")
	resp, err = run(t, `../bin/operations-center.linux.%[1]s provisioning cluster artifact list %[2]s -f json | jq -r -e '[ .[] | select(.cluster == "%[2]s") ] | length == 1'`, cpuArch, clusterName)
	require.NoError(t, err, "expect 1 artifact for cluster %s: terraform-cofiguration", clusterName)
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
