#!/bin/bash

# Compare the BMC recordings of the end to end tests with the recordings of the
# real servers the Redfish proxy profiles are drawn from.
#
# Usage: compare-recordings.sh [<recording.zip>...]
#
# Without arguments, the script compares every anonymized recording in
# OPERATIONS_CENTER_E2E_TEST_TMP_DIR. The file name of a recording names the
# profile: <server>-<profile>-bmc-recording[-failed-<id>].zip. A profile without
# a recording of a real server is skipped. The report of a recording is written
# next to it, with the extension .compare.md.
#
# The reference recordings come from the incus-redfish-proxy module in the Go
# module cache, in the version that go.mod requires.

set -u -o pipefail

go="${GO:-go}"
module="github.com/FuturFusion/incus-redfish-proxy"

module_dir="$("$go" list -m -f '{{.Dir}}' "$module")" || exit 1
if [ -z "$module_dir" ]; then
  "$go" mod download "$module" || exit 1
  module_dir="$("$go" list -m -f '{{.Dir}}' "$module")" || exit 1
fi

if [ "$#" -gt 0 ]; then
  recordings=("$@")
else
  tmp_dir="${OPERATIONS_CENTER_E2E_TEST_TMP_DIR:?OPERATIONS_CENTER_E2E_TEST_TMP_DIR is not set}"
  shopt -s nullglob
  recordings=("$tmp_dir"/*-bmc-recording.zip "$tmp_dir"/*-bmc-recording-failed-*.zip)
fi

if [ "${#recordings[@]}" -eq 0 ]; then
  echo "No BMC recordings found" >&2
  exit 1
fi

status=0
compared=0

for recording in "${recordings[@]}"; do
  name="$(basename "$recording" .zip)"
  # Server names have no dash, profile names may have one.
  profile="${name#*-}"
  profile="${profile%%-bmc-recording*}"
  reference="$module_dir/profiles/$profile"

  if [ ! -f "$reference/recording.jsonl.gz" ]; then
    echo "Skip $recording: profile $profile has no recording of a real server"
    continue
  fi

  report="${recording%.zip}.compare.md"
  if ! "$go" run "$module/cmd/redfish-recording-compare" --out "$report" "$reference" "$recording"; then
    echo "Failed to compare $recording" >&2
    status=1
    continue
  fi

  compared=$((compared + 1))
  echo "== $recording ($profile): $report"
  sed -n '/^## Summary/,/^## /{/^- /p}' "$report"
done

if [ "$compared" -eq 0 ] && [ "$status" -eq 0 ]; then
  echo "No recording of a profile with a recording of a real server found" >&2
  exit 1
fi

exit "$status"
