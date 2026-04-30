package e2e

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
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

func assertServerRegistrationScriptletEffects(t *testing.T) {
	t.Helper()

	var resp cmdResponse
	success := true

	t.Log("Assert operations-center server registration scriptlet effects")

	resp = run(t, `../bin/operations-center.linux.%s provisioning server list -f json | jq -r -e '.[] | select(.name == "IncusOS01") | .description == "some description"'`, cpuArch)
	require.NoError(t, resp.err, "expect server description to be set by server registration scriptlet")
	if !resp.Success() {
		t.Errorf("expect server description to be set by server registration scriptlet")
		success = false
		resp = mustRun(t, `../bin/operations-center.linux.%s provisioning server list -f json | jq -r -e '.[] | select(.name == "IncusOS01")`, cpuArch)
		fmt.Println(resp.Output())
	}

	resp = run(t, `../bin/operations-center.linux.%s provisioning server list -f json | jq -r -e '.[] | select(.name == "IncusOS01") | .properties.timezone == "UTC"'`, cpuArch)
	require.NoError(t, resp.err, "expect server properties to be set by server registration scriptlet")
	if !resp.Success() {
		t.Errorf("expect server properties to be set by server registration scriptlet")
		success = false
		resp = mustRun(t, `../bin/operations-center.linux.%s provisioning server list -f json | jq -r -e '.[] | select(.name == "IncusOS01")'`, cpuArch)
		fmt.Println(resp.Output())
	}

	require.True(t, success, "operations-center server registration scriptlet effects failed")
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

	// system network show (backup original value
	resp = mustRun(t, `../bin/operations-center.linux.%s system network show`, cpuArch)
	matches := regexp.MustCompile("(?m)^address: (.*)$").FindAllStringSubmatch(resp.Output(), -1)
	if len(matches) != 1 || len(matches[0]) != 2 {
		t.Fatalf("address match not found, got: %v", matches)
	}

	previousAddress := matches[0][1]

	// system network edit
	resp = run(t, `EDITOR='sed -i "s|^address: .*|address: https://127.0.0.1:8443|"' script -q -c '../bin/operations-center.linux.%s system network edit' /dev/null`, cpuArch)
	require.NoError(t, resp.err, "expect operations-center system network edit to work")
	if !resp.Success() {
		t.Errorf("expect operations-center system network edit to work")
		success = false
		fmt.Println(resp.Output())
	}

	// system network show
	resp = run(t, `../bin/operations-center.linux.%s system network show`, cpuArch)
	require.NoError(t, resp.err, "expect operations-center system network show to work")
	if !resp.Success() {
		t.Errorf("expect operations-center system network show to work")
		success = false
		fmt.Println(resp.Output())
	} else {
		require.Contains(t, resp.Output(), "address: https://127.0.0.1:8443")
	}

	// system network edit (reset)
	resp = run(t, `EDITOR='sed -i "s|^address: .*|address: %s|"' script -q -c '../bin/operations-center.linux.%s system network edit' /dev/null`, previousAddress, cpuArch)
	require.NoError(t, resp.err, "expect operations-center system network edit to work")
	if !resp.Success() {
		t.Errorf("expect operations-center system network edit to work")
		success = false
		fmt.Println(resp.Output())
	}

	// system security edit
	resp = run(t, `EDITOR='sed -i "/^trusted_tls_client_cert_fingerprints:/a\\  - ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"' script -q -c '../bin/operations-center.linux.%s system security edit' /dev/null`, cpuArch)
	require.NoError(t, resp.err, "expect operations-center system security edit to work")
	if !resp.Success() {
		t.Errorf("expect operations-center system security edit to work")
		success = false
		fmt.Println(resp.Output())
	}

	// system security show
	resp = run(t, `../bin/operations-center.linux.%s system security show`, cpuArch)
	require.NoError(t, resp.err, "expect operations-center system security show to work")
	if !resp.Success() {
		t.Errorf("expect operations-center system security show to work")
		success = false
		fmt.Println(resp.Output())
	} else {
		require.Contains(t, resp.Output(), "  - ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff")
	}

	// system security edit (reset)
	resp = run(t, `EDITOR='sed -i "/^  - ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff/d"' script -q -c '../bin/operations-center.linux.%s system security edit' /dev/null`, cpuArch)
	require.NoError(t, resp.err, "expect operations-center system security edit to work")
	if !resp.Success() {
		t.Errorf("expect operations-center system security edit to work")
		success = false
		fmt.Println(resp.Output())
	}

	// system settings edit
	resp = run(t, `EDITOR='sed -i "s|^log_level: .*|log_level: ERROR|"' script -q -c '../bin/operations-center.linux.%s system settings edit' /dev/null`, cpuArch)
	require.NoError(t, resp.err, "expect operations-center system settings edit to work")
	if !resp.Success() {
		t.Errorf("expect operations-center system settings edit to work")
		success = false
		fmt.Println(resp.Output())
	}

	// system settings show
	resp = run(t, `../bin/operations-center.linux.%s system settings show`, cpuArch)
	require.NoError(t, resp.err, "expect operations-center system settings show to work")
	if !resp.Success() {
		t.Errorf("expect operations-center system settings show to work")
		success = false
		fmt.Println(resp.Output())
	} else {
		require.Contains(t, resp.Output(), "log_level: ERROR")
	}

	// system settings edit (reset)
	resp = run(t, `EDITOR='sed -i "s|^log_level: .*|log_level: INFO|"' script -q -c '../bin/operations-center.linux.%s system settings edit' /dev/null`, cpuArch)
	require.NoError(t, resp.err, "expect operations-center system settings edit to work")
	if !resp.Success() {
		t.Errorf("expect operations-center system settings edit to work")
		success = false
		fmt.Println(resp.Output())
	}

	// system updates edit
	resp = run(t, `EDITOR='sed -i "s|^filter_expression: .*|filter_expression: \"true\"|"' script -q -c '../bin/operations-center.linux.%s system updates edit' /dev/null`, cpuArch)
	require.NoError(t, resp.err, "expect operations-center system updates edit to work")
	if !resp.Success() {
		t.Errorf("expect operations-center system updates edit to work")
		success = false
		fmt.Println(resp.Output())
	}

	// system updates show
	resp = run(t, `../bin/operations-center.linux.%s system updates show`, cpuArch)
	require.NoError(t, resp.err, "expect operations-center system updates show to work")
	if !resp.Success() {
		t.Errorf("expect operations-center system updates show to work")
		success = false
		fmt.Println(resp.Output())
	} else {
		require.Contains(t, resp.Output(), `filter_expression: "true"`)
	}

	// system updates edit (reset)
	resp = run(t, `EDITOR='sed -i "s|^filter_expression: .*|filter_expression: '\''\"stable\" in upstream_channels'\''|"' script -q -c '../bin/operations-center.linux.%s system updates edit' /dev/null`, cpuArch)
	require.NoError(t, resp.err, "expect operations-center system updates edit to work")
	if !resp.Success() {
		t.Errorf("expect operations-center system updates edit to work")
		success = false
		fmt.Println(resp.Output())
	}

	require.True(t, success, "operations-center cli system assertions failed")
}

func assertOperationsCenterCliUpdateCleanupAndRefresh(t *testing.T) {
	t.Helper()

	mustRun(t, `../bin/operations-center.linux.%s provisioning update cleanup`, cpuArch)

	// assert no updates
	mustRun(t, `../bin/operations-center.linux.%s provisioning update list -f json | jq -e -r '. | length == 0'`, cpuArch)

	mustRun(t, `../bin/operations-center.linux.%s provisioning update refresh`, cpuArch)

	// wait for updates to become ready
	mustWaitUpdatesReady(t)
}

func assertOperationsCenterCliProvisioningTokenSeed(t *testing.T, tmpDir string) {
	t.Helper()

	t.Log("Assert operations-center cli provisioning token seed")

	t.Cleanup(tokenSeedCleanup(t))

	// Create token.
	mustRun(t, `../bin/operations-center.linux.%s provisioning token add --description "CRUD" --uses 1 --lifetime 1h`, cpuArch)
	tokenResp := mustRun(t, `../bin/operations-center.linux.%s provisioning token list -f json | jq -r '.[] | select(.description == "CRUD") | .uuid'`, cpuArch)
	token := tokenResp.OutputTrimmed()

	err := os.WriteFile(filepath.Join(tmpDir, "incusos_seed.yaml"), incusOSSeedFileYAMLTemplate, 0o600)
	require.NoError(t, err)

	// Create seed.
	mustRun(t, `../bin/operations-center.linux.%s provisioning token seed add %s test %s/incusos_seed.yaml`, cpuArch, token, tmpDir)

	// List seeds.
	mustRun(t, `../bin/operations-center.linux.%s provisioning token seed list %s -f json | jq -e -r '. | length == 1'`, cpuArch, token)

	// Show seed
	mustRun(t, `../bin/operations-center.linux.%s provisioning token seed show %s test`, cpuArch, token)

	// Edit seed.
	mustRun(t, `../bin/operations-center.linux.%s provisioning token seed edit %s test < %s/incusos_seed.yaml`, cpuArch, token, tmpDir)

	// Remove seed
	mustRun(t, `../bin/operations-center.linux.%s provisioning token seed remove %s test`, cpuArch, token)

	// Create seed.
	mustRun(t, `../bin/operations-center.linux.%s provisioning token seed add %s test2 %s/incusos_seed.yaml`, cpuArch, token, tmpDir)

	// Remove token with seed.
	mustRun(t, `../bin/operations-center.linux.%s provisioning token remove %s`, cpuArch, token)
}

func tokenSeedCleanup(t *testing.T) func() {
	t.Helper()

	return func() {
		if noCleanup || (noCleanupOnError && t.Failed()) {
			return
		}

		// In t.Cleanup, t.Context() is cancelled, so we need a detached context.
		ctx, cancel := context.WithTimeout(context.Background(), strechedTimeout(30*time.Second))
		defer cancel()

		resp := runWithContext(ctx, t, `../bin/operations-center.linux.%s provisioning token list -f json | jq -r '.[] | select(.description == "CRUD") | .uuid'`, cpuArch)
		if !resp.Success() {
			return
		}

		token := resp.OutputTrimmed()
		resp = runWithContext(ctx, t, `../bin/operations-center.linux.%s provisioning token remove %s || true`, cpuArch, token)
		if !resp.Success() {
			t.Error(resp.Error())
			return
		}
	}
}

func assertOperationsCenterCliProvisioningClusterTemplate(t *testing.T, tmpDir string) {
	t.Helper()

	t.Log("Assert operations-center cli provisioning cluster-template")

	// Create cluster template.
	err := os.WriteFile(filepath.Join(tmpDir, "services_template.yaml"), incusOSClusterServicesConfigTemplate, 0o600)
	require.NoError(t, err)

	clientCertificate := getClientCertificate(t)

	err = os.WriteFile(
		filepath.Join(tmpDir, "application_template.yaml"),
		replacePlaceholders(
			incusOSClusterApplicationConfigTemplate,
			map[string]string{
				"$CLIENT_CERTIFICATE$": indent(clientCertificate, strings.Repeat(" ", 6)),
			},
		),
		0o600,
	)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(tmpDir, "variable_definition.yaml"), incusOSClusterTemplateVariableDefinition, 0o600)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(tmpDir, "variables.yaml"), incusOSClusterTemplateVariables, 0o600)
	require.NoError(t, err)

	mustRun(t, `../bin/operations-center.linux.%s provisioning cluster-template add test --services-config %s --application-seed-config %s --variables %s --description "Cluster template for incus-os-cluster"`, cpuArch, filepath.Join(tmpDir, "services_template.yaml"), filepath.Join(tmpDir, "application_template.yaml"), filepath.Join(tmpDir, "variable_definition.yaml"))

	// List cluster template.
	mustRun(t, `../bin/operations-center.linux.%s provisioning cluster-template list -f json | jq -e -r '. | length == 1'`, cpuArch)

	// Show cluster template.
	mustRun(t, `../bin/operations-center.linux.%s provisioning cluster-template show test`, cpuArch)

	// Remove cluster template.
	mustRun(t, `../bin/operations-center.linux.%s provisioning cluster-template remove test`, cpuArch)
}

func assertIncusRemote(t *testing.T, clusterName string, serverNames []string) {
	t.Helper()

	t.Log("Add incus remote")

	resp := mustRun(t, `../bin/operations-center.linux.%s provisioning cluster list -f json | jq -r '.[] | select(.name == "%s") | .connection_url'`, cpuArch, clusterName)
	clusterConnectionURL := resp.OutputTrimmed()

	mustRun(t, `incus remote add --accept-certificate --auth-type tls %s %s`, clusterName, clusterConnectionURL)
	t.Cleanup(func() {
		if noCleanup || (noCleanupOnError && t.Failed()) {
			return
		}

		// In t.Cleanup, t.Context() is cancelled, so we need a detached context.
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		mustRunWithContext(ctx, t, `incus remote remove %s`, clusterName)
	})

	mustRun(t, `incus cluster list %s: -f json | jq -r -e '. | length == %d'`, clusterName, len(serverNames))
}

func assertInventory(t *testing.T, clusterName string, names []string) {
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

	resp = run(t, `../bin/operations-center.linux.%s provisioning server list -f json | jq -r -e '[ .[] | select(.cluster == "%s" and .server_type == "incus" and .server_status == "ready") ] | length == %d'`, cpuArch, clusterName, len(names))
	require.NoError(t, resp.err, "expect %d incus servers in ready state", len(names))
	if !resp.Success() {
		t.Errorf("expect %d incus servers in ready state", len(names))
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

	resp = run(t, `../bin/operations-center.linux.%s inventory storage-volume list -f json | jq -r -e '[ .[] | select(.cluster == "%s") | .name ] | length == %d'`, cpuArch, clusterName, len(names)*3)
	require.NoError(t, resp.err, "expect 9 storage-volumes: images, backups and logs for each server of the cluster")
	if !resp.Success() {
		t.Error("expect 9 storage-volumes: images, backups and logs for each server of the cluster")
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

func assertClusterMembers(t *testing.T, clusterName string, clusterMembers []string) {
	t.Helper()

	printServerList(t)

	servers := mustRun(t, `../bin/operations-center.linux.%s provisioning server list -f json | jq -r '[ .[] | select(.cluster == %q and .server_type == "incus" and .server_status == "ready" and (.name as $n | %s | index($n) ) ) ] | .[].name'`, cpuArch, clusterName, asJSON(t, clusterMembers))
	serverNames := strings.Split(servers.OutputTrimmed(), "\n")
	if len(clusterMembers) != len(serverNames) {
		t.Fatalf("expected cluster %q to have a server %v for each name %v", clusterName, serverNames, clusterMembers)
	}
}
