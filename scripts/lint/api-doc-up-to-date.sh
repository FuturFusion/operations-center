#!/bin/sh -eu

make update-api

if [ -n "$(git status --porcelain -- 'doc/rest-api.yaml')" ]; then
  git status -- 'doc/rest-api.yaml'
  exit 1
fi

exit 0
