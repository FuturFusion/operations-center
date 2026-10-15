#!/bin/bash

# Select end to end tests interactively and print a regex for "go test -run".
#
# Usage: select.sh single|multi
#
#   single  Select one test.
#   multi   Select one or more tests (Tab marks a test). Requires fzf.
#
# The script uses fzf if it is installed. Otherwise single mode falls back to
# the bash select menu. The script exits with a non-zero status if the user
# cancels the selection.

set -u -o pipefail

mode="${1:-single}"
if [ "$mode" != "single" ] && [ "$mode" != "multi" ]; then
  echo "Usage: $0 single|multi" >&2
  exit 2
fi

GO="${GO:-go}"

if ! list="$("$GO" test -list '^TestE2E' ./e2e_tests/)"; then
  echo "Failed to list the e2e tests." >&2
  exit 1
fi

mapfile -t tests < <(grep '^TestE2E' <<<"$list")
if [ "${#tests[@]}" -eq 0 ]; then
  echo "No e2e tests found." >&2
  exit 1
fi

if command -v fzf >/dev/null; then
  fzf_args=(
    --height=40%
    --reverse
    --prompt='e2e test> '
    --preview='grep -h -B5 -A30 "^func {}(" e2e_tests/*_test.go'
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

echo "^($(paste -sd'|' <<<"$selected"))\$"
