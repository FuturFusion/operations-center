package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func assertIncusRemote(t *testing.T, clusterIP string) {
	t.Helper()

	t.Log("Add incus remote")

	mustRun(t, `incus remote add --accept-certificate --auth-type tls incus-os-cluster https://%s:8443`, clusterIP)
	t.Cleanup(func() {
		// in cleanup, t.Context() is cancelled, so we need a detached context.
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		mustRunWithContext(ctx, t, `incus remote remove incus-os-cluster`)
	})

	mustRun(t, `incus cluster list incus-os-cluster: -f json | jq -r -e '. | length == 3'`)
}

func assertInventory(t *testing.T) {
	t.Helper()

	var resp cmdResponse
	var err error
	success := true

	t.Log("Assert inventory content after cluster creation")

	resp, err = run(t, `../bin/operations-center.linux.%s provisioning cluster list -f json | jq -r -e '[ .[] | select(.name == "incus-os-cluster") ] | length == 1'`, cpuArch)
	require.NoError(t, err, "expect 1 cluster entry with name incus-os-cluster")
	if !resp.Success() {
		success = false
		fmt.Println("====[ Cluster List ]====")
		resp := mustRun(t, "../bin/operations-center.linux.%s provisioning cluster list", cpuArch)
		fmt.Println(resp.Output())
	}

	resp, err = run(t, `../bin/operations-center.linux.%s provisioning server list -f json | jq -r -e '[ .[] | select(.server_status == "ready") ] | length == 3'`, cpuArch)
	require.NoError(t, err, "expect 3 servers in ready state")
	if !resp.Success() {
		success = false
		fmt.Println("====[ Server List ]====")
		resp := mustRun(t, "../bin/operations-center.linux.%s provisioning server list", cpuArch)
		fmt.Println(resp.Output())
	}

	// Performing cluster resync of inventory data.
	mustRun(t, "../bin/operations-center.linux.%s provisioning cluster resync incus-os-cluster", cpuArch)

	resp, err = run(t, `../bin/operations-center.linux.%s inventory network list -f json | jq -r -e '[ .[] | select(.cluster == "incus-os-cluster") | .name ] | length == 2'`, cpuArch)
	require.NoError(t, err, "expect 2 networks: incusbr0, meshbr0")
	if !resp.Success() {
		success = false
		fmt.Println("====[ Network List ]====")
		resp = mustRun(t, "../bin/operations-center.linux.%s inventory network list", cpuArch)
		fmt.Println(resp.Output())
	}

	resp, err = run(t, `../bin/operations-center.linux.%s inventory profile list -f json | jq -r -e '[ .[] | select(.cluster == "incus-os-cluster") | .name ] | length == 2'`, cpuArch)
	require.NoError(t, err, "expect 2 profiles: default, internal")
	if !resp.Success() {
		fmt.Println("====[ Profile List ]====")
		resp = mustRun(t, "../bin/operations-center.linux.%s inventory profile list", cpuArch)
		fmt.Println(resp.Output())
	}

	resp, err = run(t, `../bin/operations-center.linux.%s inventory project list -f json | jq -r -e '[ .[] | select(.cluster == "incus-os-cluster") | .name ] | length == 2'`, cpuArch)
	require.NoError(t, err, "expect 2 profiles: default, internal")
	if !resp.Success() {
		fmt.Println("====[ Project List ]====")
		resp = mustRun(t, "../bin/operations-center.linux.%s inventory project list", cpuArch)
		fmt.Println(resp.Output())
	}

	resp, err = run(t, `../bin/operations-center.linux.%s inventory storage-pool list -f json | jq -r -e '[ .[] | select(.cluster == "incus-os-cluster") | .name ] | length == 1'`, cpuArch)
	require.NoError(t, err, "expect 1 storage pool: local")
	if !resp.Success() {
		fmt.Println("====[ Storage Pool List ]====")
		resp = mustRun(t, "../bin/operations-center.linux.%s inventory storage-pool list", cpuArch)
		fmt.Println(resp.Output())
	}

	resp, err = run(t, `../bin/operations-center.linux.%s inventory storage-volume list -f json | jq -r -e '[ .[] | select(.cluster == "incus-os-cluster") | .name ] | length == 6'`, cpuArch)
	require.NoError(t, err, "expect 6 storage-volumes: images and backups for each server")
	if !resp.Success() {
		fmt.Println("====[ Storage Volume List ]====")
		resp = mustRun(t, "../bin/operations-center.linux.%s inventory storage-volume list", cpuArch)
		fmt.Println(resp.Output())
	}

	require.True(t, success, "inventory assertions failed")
}
