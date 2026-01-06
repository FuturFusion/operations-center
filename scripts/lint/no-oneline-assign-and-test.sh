#!/bin/sh -eu

echo "no-oneline-assign-and-test.sh disabled temporarily, replaced by gocritic ruleguard"
exit 0

echo "Checking for oneline assign & test..."

# Recursively grep go files for if statements that contain assignments.
! git grep --untracked -P -n '^\s+if.*:=.*;.*{\s*$' -- '*.go' ':!:internal/migratekit/*.go'
