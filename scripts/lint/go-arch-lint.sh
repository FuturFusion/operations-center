#!/bin/sh -eu

# Checks the layer rules of .go-arch-lint.yml. See doc/development/architecture-rules.md.

cd "$(dirname "$0")/../.."
go tool go-arch-lint check --project-path .
