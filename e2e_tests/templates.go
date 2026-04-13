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
        },
        {
          "name": "debug"
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

	incusOSSeedFileYAMLTemplate = []byte(`---
applications:
  version: "1"
  applications:
    - name: incus
    - name: debug
network:
  interfaces:
    - name: enp5s0
      hwaddr: enp5s0
      required_for_online: both
      addresses:
      - dhcp4
      - dhcp6
      - slaac
incus:
  version: "1"
`)

	incusOSFactoryResetSeedFileYAMLTemplate = []byte(`---
applications:
  version: "1"
  applications:
    - name: incus
    - name: debug
network:
  interfaces:
    - name: enp5s0
      hwaddr: enp5s0
      required_for_online: both
      addresses:
      - dhcp4
      - dhcp6
      - slaac
incus:
  version: "1"
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
certificates:
  - name: admin
    type: client
    certificate: |
$CLIENT_CERTIFICATE$
    description: Initial admin client
`)

	incusOSClusterApplicationConfigPostFactoryReset = []byte(`---
config:
  user.ui.title: E2E Test IncusOS Cluster
certificates:
  - type: client
    name: my-client-cert
    description: "Client certificate for accessing the cluster"
    certificate: |-
$CLIENT_CERTIFICATE$
`)

	incusOSClusterApplicationConfigPostFactoryResetWithTokenSeed = []byte(`---
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
certificates:
  - name: admin
    type: client
    certificate: |
$CLIENT_CERTIFICATE$
    description: Initial admin client
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

	createManualUpdateScript = []byte(`#!/usr/bin/env bash
set -euo pipefail

BASE_URL="https://images.linuxcontainers.org/os"
INDEX_URL="${BASE_URL}/index.json"
DEST="tmp_manual_update"

echo "Fetching index..."
index=$(curl -fsSL "$INDEX_URL")

# Extract filenames for x86_64 from the first update entry
mapfile -t files < <(
  echo "$index" | jq -r '
    .updates[0].files[]
    | select(.architecture == "x86_64" or .architecture == "")
    | .filename
  '
)

update_index=(
  "update.json"
  "update.sjson"
)
files+=("${update_index[@]}")

url_path=$(echo "$index" | jq -r '.updates[0].url')

echo "Found ${#files[@]} files to download."

for file in "${files[@]}"; do
  dest_path="${DEST}/${file}"
  mkdir -p "$(dirname "$dest_path")"

  if [[ -f "$dest_path" ]]; then
    echo "  Skipping ${file} (already exists)"
  else
    echo "  Downloading ${file}..."
    curl -fsSL --create-dirs -o "$dest_path" "${BASE_URL}/${url_path}/${file}"
  fi
done

echo "Done. Files saved to ${DEST}/"

if [[ ! -f "manual_update.tar" ]]; then
  echo "Creating tar archive..."
  cd "$DEST"
  tar -cf ../manual_update.tar *
  cd -
  echo "Archive created: manual_update.tar"
fi
`)
)

func replacePlaceholders(in []byte, vars map[string]string) []byte {
	for key, value := range vars {
		in = bytes.ReplaceAll(in, []byte(key), []byte(value))
	}

	return in
}
