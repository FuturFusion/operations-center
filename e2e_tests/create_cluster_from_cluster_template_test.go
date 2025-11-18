package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func createClusterFromTemplate(t *testing.T, tmpDir string) {
	t.Helper()

	stop := timeTrack(t)
	defer stop()

	// Pre check
	clusterListResp, err := run(t, "incus exec IncusOS01 -- incus cluster list")
	require.NoError(t, err)
	require.NotEqual(t, 0, clusterListResp.exitCode, "IncusOS01 is already part of a cluster")

	// Setup
	err = os.WriteFile(filepath.Join(tmpDir, "services_template.yaml"), incusOSClusterServicesConfigTemplate, 0o600)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(tmpDir, "application_template.yaml"), incusOSClusterApplicationConfigTemplate, 0o600)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(tmpDir, "variable_definition.yaml"), incusOSClusterTemplateVariableDefinition, 0o600)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(tmpDir, "variables.yaml"), incusOSClusterTemplateVariables, 0o600)
	require.NoError(t, err)

	mustRun(t, `../bin/operations-center.linux.%s provisioning cluster-template add incus-os-cluster --services-config %s --application-seed-config %s --variables %s --description "Cluster template for incus-os-cluster"`, cpuArch, filepath.Join(tmpDir, "services_template.yaml"), filepath.Join(tmpDir, "application_template.yaml"), filepath.Join(tmpDir, "variable_definition.yaml"))

	var firstInstanceIP string

	ipResp := mustRun(t, `incus list -f json | jq -r '.[] | select(.name == "IncusOS01") | .state.network | to_entries[] | .value.addresses[]? | select(.family == "inet" and .scope == "global") | .address' | head -n1`)
	firstInstanceIP = strings.TrimSpace(ipResp.Output())

	instanceNames := make([]string, 0, 3)
	for i := range 3 {
		instanceID := i + 1
		nameResp := mustRun(t, `incus list -f json | jq -r '.[] | select(.name == "IncusOS0%d") | .state.os_info.hostname'`, instanceID)
		instanceNames = append(instanceNames, strings.TrimSpace(nameResp.Output()))
	}

	servers := strings.Join(instanceNames, " --server-names ")

	// Run test
	mustRun(t, "../bin/operations-center.linux.%s provisioning cluster add incus-os-cluster https://%s:8443 --server-names %s --cluster-template incus-os-cluster --cluster-template-variables %s", cpuArch, firstInstanceIP, servers, filepath.Join(tmpDir, "variables.yaml"))

	// Assertions
	assertIncusRemote(t, firstInstanceIP)
	assertInventory(t)
}
