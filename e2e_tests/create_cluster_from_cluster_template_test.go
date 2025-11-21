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
	mustNotBeAlreadyClustered(t)

	// Setup
	err := os.WriteFile(filepath.Join(tmpDir, "services_template.yaml"), incusOSClusterServicesConfigTemplate, 0o600)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(tmpDir, "application_template.yaml"), incusOSClusterApplicationConfigTemplate, 0o600)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(tmpDir, "variable_definition.yaml"), incusOSClusterTemplateVariableDefinition, 0o600)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(tmpDir, "variables.yaml"), incusOSClusterTemplateVariables, 0o600)
	require.NoError(t, err)

	mustRun(t, `../bin/operations-center.linux.%s provisioning cluster-template add incus-os-cluster --services-config %s --application-seed-config %s --variables %s --description "Cluster template for incus-os-cluster"`, cpuArch, filepath.Join(tmpDir, "services_template.yaml"), filepath.Join(tmpDir, "application_template.yaml"), filepath.Join(tmpDir, "variable_definition.yaml"))

	instanceIPs, instanceNames := mustGetInstanceIPAndNames(t, []string{"IncusOS01", "IncusOS02", "IncusOS03"})

	servers := strings.Join(instanceNames, " --server-names ")

	// Run test
	t.Log("Create cluster")
	mustRun(t, "../bin/operations-center.linux.%s provisioning cluster add incus-os-cluster https://%s:8443 --server-names %s --cluster-template incus-os-cluster --cluster-template-variables %s", cpuArch, instanceIPs[0], servers, filepath.Join(tmpDir, "variables.yaml"))

	// Assertions
	assertIncusRemote(t, instanceIPs[0])
	assertInventory(t)
	assertTerraformArtifact(t)
	assertWebsocketEventsInventoryUpdate(t)
}
