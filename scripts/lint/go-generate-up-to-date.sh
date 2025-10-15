#!/bin/sh -eu

go generate ./...

if [ -n "$(git status --porcelain -- '**/*_gen.go' '**/*_gen_test.go' '**/*_gen.tsx' '**/*_gen.d.ts')" ]; then
  git status -- '**/*_gen.go' '**/*_gen_test.go' '**/*_gen.tsx' '**/*_gen.d.ts'
  exit 1
fi

exit 0
