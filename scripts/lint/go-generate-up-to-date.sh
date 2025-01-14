#!/bin/sh -eu

go generate ./...

if [ -n "$(git status --porcelain -- '**/*_gen.go' '**/*_gen_test.go')" ]; then
  git status -- '**/*_gen.go'
  exit 1
fi

exit 0
