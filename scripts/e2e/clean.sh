#!/bin/bash

# Clean up the artifacts of the end to end tests.
#
# Usage: clean.sh soft|full
#
#   soft  Keep the Operations Center VM and its ISO.
#   full  Remove everything.
#
# Errors for resources, that are already gone, are hidden. All other errors
# are reported. The script continues after an error and exits with a non-zero
# status at the end.

set -u -o pipefail

mode="${1:-}"
if [ "$mode" != "soft" ] && [ "$mode" != "full" ]; then
  echo "Usage: $0 soft|full" >&2
  exit 2
fi

tmp_dir="${OPERATIONS_CENTER_E2E_TEST_TMP_DIR:?OPERATIONS_CENTER_E2E_TEST_TMP_DIR is not set}"
oc="${OPERATIONS_CENTER_BIN:-bin/operations-center.linux.amd64}"

missing='not found|doesn.t exist'
iso_volumes='.[] | select(.name | test("IncusOS-.*|IncusOS.*-boot-media\\.iso")) | .name'

failed=0

step() {
  echo "==> $*"
}

report() {
  local out="$1"
  shift

  echo "Error: command failed: $*" >&2
  if [ -n "$out" ]; then
    printf '%s\n' "$out" | sed 's/^/    /' >&2
  fi

  failed=1
}

# Run a command. Hide its output, if it succeeds or if its output matches the
# given pattern.
ignore_errors() {
  local pattern="$1"
  shift

  local out status
  out=$("$@" 2>&1)
  status=$?
  if [ "$status" -eq 0 ]; then
    return 0
  fi

  # Status 126 and 127 mean, that the shell could not run the command.
  if [ "$status" -lt 126 ] && printf '%s\n' "$out" | grep -qiE "$pattern"; then
    return 0
  fi

  report "$out" "$@"
}

# Run a command. Hide the error, if the resource is already gone.
ignore_missing() {
  ignore_errors "$missing" "$@"
}

# Run a command and print its standard output. Report the error, if it fails.
list() {
  local out err_file
  err_file=$(mktemp)

  if out=$("$@" 2> "$err_file"); then
    printf '%s\n' "$out"
  else
    report "$(cat "$err_file")" "$@"
  fi

  rm -f "$err_file"
}

remove_iso_volumes() {
  local volume
  for volume in $(list incus storage volume list default -f json | jq -r "$iso_volumes"); do
    ignore_missing incus storage volume delete default "$volume"
  done
}

clean_operations_center() {
  local name uuid seed

  step "Remove clusters"
  ignore_missing "$oc" provisioning cluster remove incus-os-cluster --force
  ignore_missing "$oc" provisioning cluster remove incus-os-cluster-after-factory-reset --force

  step "Cancel deployments"
  ignore_errors "$missing|has no deployment" "$oc" provisioning server deploy-cancel IncusOS01 --skip-cleanup
  # Only wait for the deployment to end. Its result does not matter.
  ignore_errors "$missing|has never been deployed|has been cancelled|failed in state" "$oc" provisioning server deploy-status IncusOS01 --wait

  step "Remove servers"
  for name in $(list "$oc" provisioning server list -f json | jq -r '.[] | select(.server_type == "incus") | .name'); do
    ignore_missing "$oc" provisioning server remove "$name"
  done

  step "Reset update channels"
  for uuid in $(list "$oc" provisioning update list -f json | jq -r '.[] | .uuid'); do
    ignore_missing "$oc" provisioning update assign-channels "$uuid" --channel stable
  done

  step "Remove token seeds"
  for uuid in $(list "$oc" provisioning token list -f json | jq -r '.[].uuid'); do
    for seed in incus-os-cluster incus-os-cluster-factory-reset incus-os-deploy; do
      ignore_missing "$oc" provisioning token seed remove "$uuid" "$seed"
    done
  done

  step "Remove tokens"
  for uuid in $(list "$oc" provisioning token list -f json | jq -r '.[] | select(.description == "CRUD" or .description == "e2e OIDC write access" or .description == "e2e OpenFGA authorization") | .uuid'); do
    ignore_missing "$oc" provisioning token remove "$uuid"
  done

  step "Remove image sources"
  for name in $(list "$oc" image incus source list -f json | jq -r '.[].name'); do
    ignore_missing "$oc" image incus source remove "$name"
  done

  step "Remove images"
  for name in $(list "$oc" image incus list -f json | jq -r '.[].name'); do
    ignore_missing "$oc" image incus remove "$name"
  done

  # Reset the OIDC part and the OpenFGA part of the security config in a single update.
  # Both services of the tests are gone. Operations Center rejects an update of one part
  # if the other part points at a missing service.
  step "Reset security config"
  local config out
  config=$(list "$oc" system security show -f json | jq -ce '.oidc = { issuer: "", client_id: "", scopes: "", audience: "", claim: "" } | .openfga = { api_url: "", api_token: "", store_id: "" }')
  # Without a trusted client certificate, the update would lock out the user.
  if [ -n "$config" ] && printf '%s\n' "$config" | jq -e '.trusted_tls_client_cert_fingerprints | length > 0' > /dev/null; then
    if ! out=$(printf '%s\n' "$config" | "$oc" system security edit 2>&1); then
      report "$out" "$oc" system security edit
    fi
  fi
}

step "Remove temporary files"
rm -rf "$tmp_dir/image-downloads"
rm -rf "$tmp_dir/oidc-cli-config"
rm -rf "$tmp_dir/openfga-cli-config"
rm -rf "$tmp_dir/images"
rm -rf "$tmp_dir"/coverage_*
# One preseeded ISO is created per provisioning token, so these accumulate
# over the runs. The ISO of Operations Center is kept, it is expensive to
# recreate and its name does not match this pattern.
rm -f "$tmp_dir"/IncusOS-preseeded-*.iso

step "Remove Incus remotes"
ignore_missing incus remote remove incus-os-cluster
ignore_missing incus remote remove incus-os-cluster-after-factory-reset

step "Remove Incus instances"
for instance in IncusOS01 IncusOS02 IncusOS03 IncusOS04; do
  ignore_missing incus remove --force "$instance"
done

# Remove the preseeded ISO storage volumes, which accumulate over the runs.
# This has to happen after the instances using them are gone.
step "Remove ISO storage volumes"
remove_iso_volumes

if out=$("$oc" provisioning server list -f json 2>&1); then
  clean_operations_center
elif [ "$mode" = "full" ]; then
  echo "Operations Center is not reachable. Skip the cleanup of its records."
else
  echo "Error: Operations Center is not reachable. Skip the cleanup of its records." >&2
  printf '%s\n' "$out" | sed 's/^/    /' >&2
  failed=1
fi

if [ "$mode" = "full" ]; then
  step "Remove Operations Center"
  rm -rf "$tmp_dir"
  rm -rf "$HOME/.config/operations-center/"
  ignore_missing incus remove --force OperationsCenter
  ignore_missing incus storage volume delete default IncusOS_OperationsCenter.iso
  remove_iso_volumes
fi

exit "$failed"
