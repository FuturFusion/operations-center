package e2e

import (
	"bytes"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
	"time"

	config "github.com/FuturFusion/operations-center/internal/config/daemon"
)

// redfishProxyBIOSProfileYAML renders redfishProxyBIOSProfileYAMLTemplate for
// the given profile of the Redfish proxy. The profile only matches the model
// the proxy reports for this profile, so a proxy with another profile is not
// deployed by accident. Operations Center enforces the timeouts during the
// deployment, so they are stretched like the timeouts of the tests.
func redfishProxyBIOSProfileYAML(profile redfishProfile) []byte {
	return fmt.Appendf(nil, string(redfishProxyBIOSProfileYAMLTemplate),
		strechedTimeout(5*time.Minute),
		strechedTimeout(config.ServerDeploymentStepCallTimeout),
		strechedTimeout(config.ServerDeploymentStepTimeout),
		strechedTimeout(config.ServerDeploymentMediaIdlePeriod),
		strechedTimeout(config.ServerDeploymentRebootTimeout),
		regexp.QuoteMeta(redfishProxyModel(profile.name)),
		biosAttributesYAML(profile.biosAttributes),
		biosDeferredAttributesYAML(profile.biosDeferredAttributes),
	)
}

// biosAttributesYAML renders the attributes as the entries of a YAML mapping,
// sorted by name.
func biosAttributesYAML(attributes map[string]string) string {
	var b strings.Builder

	for _, name := range slices.Sorted(maps.Keys(attributes)) {
		fmt.Fprintf(&b, "  %s: %q\n", name, attributes[name])
	}

	return b.String()
}

// biosDeferredAttributesYAML renders the deferred attributes. Without any, the
// key is left out.
func biosDeferredAttributesYAML(attributes map[string]string) string {
	if len(attributes) == 0 {
		return ""
	}

	return "deferred_attributes:\n" + biosAttributesYAML(attributes)
}

var (
	operationsCenterSettingsWithRegistrationScriptletYAML = []byte(`---
log_level: INFO
log_levels:
  # The tests poll the API in a loop, so the access log alone produces multiple
  # lines per second and pushes everything else out of the captured journal.
  access_log: WARN
server_registration_scriptlet: |
  def server_registration(candidate):
    server.set_description("some description")
    server.set_properties({ "timezone": candidate.os_data.network.config.time.timezone })
pprof_enabled: false
metrics_enabled: true
metrics_components:
  - runtime
`)

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
  version: "1"
  interfaces:
    - name: enp5s0
      hwaddr: enp5s0
      required_for_online: both
      addresses:
      - dhcp4
      - dhcp6
      - slaac
`)

	// The content differs from incusOSSeedFileYAMLTemplate, so the effect of an
	// edit is visible.
	tokenSeedPutYAMLTemplate = []byte(`---
description: E2E test token seed
public: false
seeds:
  applications:
    version: "1"
    applications:
      - name: incus
  network:
    version: "1"
    interfaces:
      - name: enp5s0
        hwaddr: enp5s0
        required_for_online: both
        addresses:
        - dhcp4
        - dhcp6
        - slaac
    time:
      timezone: Europe/Zurich
`)

	// The timezone differs from the default UTC. The server registration
	// scriptlet copies it to the properties of the server, which makes the
	// effect of the seed visible in Operations Center.
	incusOSFactoryResetSeedFileYAMLTemplate = []byte(`---
applications:
  version: "1"
  applications:
    - name: incus
    - name: debug
network:
  version: "1"
  interfaces:
    - name: enp5s0
      hwaddr: enp5s0
      required_for_online: both
      addresses:
      - dhcp4
      - dhcp6
      - slaac
  time:
    timezone: Europe/Zurich
`)

	deployIncusOSSeedFileYAMLTemplate = []byte(`---
applications:
  version: "1"
  applications:
    - name: incus
    - name: debug
install:
  version: "1"
  force_install: false
  force_reboot: true
network:
  version: "1"
  interfaces:
    - name: enp5s0
      hwaddr: enp5s0
      required_for_online: both
      addresses:
      - dhcp4
      - dhcp6
      - slaac
`)

	redfishProxyBIOSProfileYAMLTemplate = []byte(`---
name: incus-redfish-proxy
description: Incus instance exposed by incus-redfish-proxy
match:
  - manufacturer: "linuxcontainers\\.org"
    model: '%[6]s'
priority: 1000
attributes:
  incus.config.user.e2e.bios-profile: "applied"
%[7]s%[8]sdeployment:
  attach_media_call_timeout: %[1]s
  call_timeout: %[2]s
  step_timeout: %[3]s
  install_media_idle_period: %[4]s
  reboot_timeout: %[5]s
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

	incusOSServiceLVMConfig = []byte(`---
config:
  enabled: true
  system_id: 1999
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

# Upstream may list a new file in the index before the file server serves it.
# Retry all errors, including 404.
CURL_RETRY=(--retry 5 --retry-delay 10 --retry-all-errors)

# download_update downloads all x86_64 files of the given update into DEST.
# It runs in an "if" condition, where "set -e" has no effect. So every step
# returns explicitly on failure.
download_update() {
  local update="$1"
  local version url_path file dest_path
  local -a files

  version=$(echo "$update" | jq -r '.version') || return 1
  url_path=$(echo "$update" | jq -r '.url') || return 1
  echo "Using stable update ${version}"

  # Keep the downloads per version, so files of different updates never mix.
  DEST="tmp_manual_update/${version}"

  mapfile -t files < <(
    echo "$update" | jq -r '
      .files[]
      | select(.architecture == "x86_64" or .architecture == "")
      | .filename
    '
  )
  files+=("update.json" "update.sjson")

  echo "Found ${#files[@]} files to download."

  for file in "${files[@]}"; do
    dest_path="${DEST}/${file}"

    if [[ -f "$dest_path" ]]; then
      echo "  Skipping ${file} (already exists)"
      continue
    fi

    echo "  Downloading ${file}..."
    # Download to a temporary file. An interrupted download must not match
    # the "already exists" check of a later run.
    curl -fsSL "${CURL_RETRY[@]}" --create-dirs -o "${dest_path}.part" "${BASE_URL}/${url_path}/${file}" || return 1
    mv "${dest_path}.part" "$dest_path" || return 1
  done
}

echo "Fetching index..."
index=$(curl -fsSL "${CURL_RETRY[@]}" "$INDEX_URL")

# The index mixes all upstream channels, the other E2E tests use stable.
# Try older updates too, in case the newest one is not fully published yet.
mapfile -t updates < <(
  echo "$index" | jq -c '[ .updates[] | select(.channels | index("stable")) ] | sort_by(.version) | reverse | .[:3] | .[]'
)
if [[ ${#updates[@]} -eq 0 ]]; then
  echo "No update in the stable channel found in ${INDEX_URL}" >&2
  exit 1
fi

downloaded=false
for update in "${updates[@]}"; do
  if download_update "$update"; then
    downloaded=true
    break
  fi

  echo "Failed to download update, trying the next older one." >&2
done

if [[ "$downloaded" != "true" ]]; then
  echo "Failed to download any of the ${#updates[@]} newest stable updates." >&2
  exit 1
fi

echo "Done. Files saved to ${DEST}/"

echo "Creating tar archive..."
(cd "$DEST" && tar -cf ../../manual_update.tar *)
echo "Archive created: manual_update.tar"
`)
)

func replacePlaceholders(in []byte, vars map[string]string) []byte {
	for key, value := range vars {
		in = bytes.ReplaceAll(in, []byte(key), []byte(value))
	}

	return in
}
