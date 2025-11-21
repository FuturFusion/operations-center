package e2e

import (
	"bytes"
)

var (
	operationsCenterSeedTemplate = []byte(`{
  "seeds": {
    "install": {
      "version": "1",
      "force_install": false,
      "force_reboot": false
    },
    "applications": {
      "version": "1",
      "applications": [
        {
          "name": "operations-center"
        }
      ]
    },
    "operations-center": {
      "version": "1",
      "trusted_client_certificates": [
        $CLIENT_CERTIFICATE$
      ]
    }
  },
  "type": "iso",
  "architecture": "x86_64"
}
`)

	operationsCenterConfigYAMLTemplate = []byte(`---
default_remote: test
remotes:
  test:
    addr: https://$OPERATIONS_CENTER_IPADDRESS$:8443/
    auth_type: tls
    server_cert: |
$OPERATIONS_CENTER_CERTIFICATE$
`)

	incusOSSeedFileYAMLTemplate = []byte(`---
applications:
  version: 1
  applications:
    - name: incus
incus:
  version: 1
  preseed:
    certificates:
      - name: admin
        type: client
        certificate: |
$CLIENT_CERTIFICATE$
        description: Initial admin client
`)

	// createCluster templates.

	incusOSClusterServicesConfig = []byte(`---
lvm:
  enabled: true
`)

	incusOSClusterApplicationConfig = []byte(`---
config:
  user.ui.title: E2E Test IncusOS Cluster
`)

	// createClusterFromTemplate templates.

	incusOSClusterServicesConfigTemplate = []byte(`---
lvm:
  enabled: @LVM_ENABLED@
`)

	incusOSClusterApplicationConfigTemplate = []byte(`---
config:
  user.ui.title: @USER_UI_TITLE@
`)

	incusOSClusterTemplateVariableDefinition = []byte(`---
LVM_ENABLED:
  description: Is LVM enabled?
USER_UI_TITLE:
  description: UI Title
  default: E2E Test IncusOS Cluster
`)

	incusOSClusterTemplateVariables = []byte(`---
LVM_ENABLED: "true"
`)
)

func replacePlaceholders(in []byte, vars map[string]string) []byte {
	for key, value := range vars {
		in = bytes.ReplaceAll(in, []byte(key), []byte(value))
	}

	return in
}
