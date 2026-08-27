#!/bin/sh -eu

make update-api

if [ -n "$(git status --porcelain -- 'doc/rest-api.yaml')" ]; then
  git status -- 'doc/rest-api.yaml'
  exit 1
fi

if ! command -v swagger >/dev/null 2>&1; then
  go install -v -x github.com/go-swagger/go-swagger/cmd/swagger
fi

# Warnings we cannot act on are filtered out, so that whatever is left in the
# output is worth looking at.
#
#   * InitPreseed belongs to github.com/lxc/incus/v7/shared/api and documents six
#     slice fields with a string example.
#   * IncusImagePost cannot be referenced from an operation. POST /1.0/images/incus
#     is a multipart upload, and a Swagger 2.0 formData parameter cannot carry a
#     schema, so the json_request parameter names the definition in its
#     description instead.
#   * SystemProviderConfig is embedded into TokenProviderConfig. go-swagger
#     inlines the embedded fields into the parent and still emits the definition,
#     which therefore ends up referenced by nothing.
IGNORED_WARNINGS='preseed\..*\.example in body must be of type array: "string"
definitions\.InitPreseed\..*\.example in body must be of type array: "string"
example value for (token|tokenImagePost|tokenSeedsPost) in body does not validate its schema
in operation "(token_seed_get|tokens_seeds_get_recursion)", example value in response 200 does not validate its schema
definition "#/definitions/(IncusImagePost|SystemProviderConfig)" is not used anywhere
showed up some valid but possibly unwanted constructs
See warnings below:'

set +e
validate_output="$(swagger validate doc/rest-api.yaml 2>&1)"
validate_rc=$?
set -e

echo "${validate_output}" | grep -v -E "$(echo "${IGNORED_WARNINGS}" | paste -sd'|')" || true

exit "${validate_rc}"
