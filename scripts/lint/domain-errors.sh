#!/bin/sh -eu

# Reports the places, which do not follow the error handling conventions of
# doc/development/error-handling.md.

go run ./cmd/domain-errors ./...
