#!/bin/sh -eu

go generate ./...

if [ -n "$(git status --porcelain -- '**/*_gen.go' '**/*_gen_test.go' '**/*_gen.tsx' '**/*_gen.d.ts' '**/*.mapper.go')" ]; then
  git status -- '**/*_gen.go' '**/*_gen_test.go' '**/*_gen.tsx' '**/*_gen.d.ts' '**/*.mapper.go'
  exit 1
fi

exit 0
