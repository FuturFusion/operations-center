#!/bin/bash

# Select end to end tests interactively and print a regex for "go test -run".
#
# Usage: select.sh single|multi|list
#
#   single  Select one test.
#   multi   Select one or more tests (Tab marks a test). Requires fzf.
#   list    Print all the tests, one per line.
#
# A test is either a top level test, e.g. "TestE2E_OperationsCenter", or a
# scenario of a suite, e.g. "TestE2E_OperationsCenter/oidc_authentication".
# Selecting a suite selects all of its scenarios.
#
# The script uses fzf if it is installed. Otherwise single mode falls back to
# the bash select menu. The script exits with a non-zero status if the user
# cancels the selection.

set -u -o pipefail

mode="${1:-single}"

GO="${GO:-go}"

# preview prints the source of the test or the scenario for the fzf preview.
preview() {
  local test="$1"

  if [[ "$test" == */* ]]; then
    grep -h -B2 -A10 "name: \"${test#*/}\"" e2e_tests/*_test.go
    return
  fi

  grep -h -B5 -A30 "^func ${test}(" e2e_tests/*_test.go
}

# listTests prints the top level tests and the scenarios of the suites. The
# suites only create their subtests in list mode, they do not run anything.
listTests() {
  local output

  if ! output="$(OPERATIONS_CENTER_E2E_TEST_LIST=1 "$GO" test ./e2e_tests/ -run '^TestE2E' -v -count 1 2>&1)"; then
    echo "$output" >&2
    echo "Failed to list the e2e tests." >&2
    return 1
  fi

  sed -n 's/^=== RUN   \(TestE2E.*\)$/\1/p' <<<"$output"
}

# toRegex converts the selected tests to a regex for "go test -run". The
# alternatives are joined on the top level, since "go test" splits the regex
# at a top level "/" into the levels of the subtests.
toRegex() {
  local regex=""
  local test

  while IFS= read -r test; do
    local alternative="^${test%%/*}\$"
    if [[ "$test" == */* ]]; then
      alternative+="/^${test#*/}\$"
    fi

    regex+="${regex:+|}${alternative}"
  done

  echo "$regex"
}

case "$mode" in
  single | multi) ;;
  list)
    listTests
    exit
    ;;
  preview)
    preview "${2:-}"
    exit
    ;;
  *)
    echo "Usage: $0 single|multi|list" >&2
    exit 2
    ;;
esac

if ! list="$(listTests)"; then
  exit 1
fi

mapfile -t tests <<<"$list"
if [ -z "${tests[0]:-}" ]; then
  echo "No e2e tests found." >&2
  exit 1
fi

if command -v fzf >/dev/null; then
  fzf_args=(
    --height=40%
    --reverse
    --prompt='e2e test> '
    --preview="$(printf '%q' "${BASH_SOURCE[0]}") preview {}"
  )
  if [ "$mode" = "multi" ]; then
    fzf_args+=(--multi)
  fi

  if ! selected="$(printf '%s\n' "${tests[@]}" | fzf "${fzf_args[@]}")"; then
    echo "No test selected." >&2
    exit 1
  fi
else
  if [ "$mode" = "multi" ]; then
    echo "Multi select requires fzf. Install fzf and try again." >&2
    exit 1
  fi

  echo "Hint: install fzf for search and preview." >&2
  PS3="Select a test (number): "
  selected=""
  select test in "${tests[@]}"; do
    if [ -n "$test" ]; then
      selected="$test"
      break
    fi

    echo "Invalid selection." >&2
  done

  if [ -z "$selected" ]; then
    echo "No test selected." >&2
    exit 1
  fi
fi

toRegex <<<"$selected"
