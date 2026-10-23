#!/bin/sh -eu

go generate ./...

if [ -n "$(git status --porcelain -- '**/*_gen.go' '**/*_gen_test.go' '**/*_gen.tsx' '**/*_gen.d.ts' '**/*.mapper.go' 'doc/reference/settings_log_components.md' 'doc/reference/settings_metrics_components.md' 'doc/development/server-deployment-states.md')" ]; then
  git status -- '**/*_gen.go' '**/*_gen_test.go' '**/*_gen.tsx' '**/*_gen.d.ts' '**/*.mapper.go' 'doc/reference/settings_log_components.md' 'doc/reference/settings_metrics_components.md' 'doc/development/server-deployment-states.md'
  exit 1
fi

# The graphs embed a font. Its compression output depends on the Go version,
# so the embedded font data is ignored. The SVGs are compared with HEAD.
strip_font() {
  sed 's|data:application/font-woff;base64,[A-Za-z0-9+/=]*|FONT|g'
}

raw=$(mktemp)
tmp=$(mktemp)
trap 'rm -f "$raw" "$tmp"' EXIT

failed=0
for svg in doc/images/architecture-daemon.svg doc/images/architecture-cli.svg; do
  if ! git show "HEAD:$svg" > "$raw"; then
    echo "cannot read $svg at HEAD" >&2
    failed=1
    continue
  fi

  strip_font < "$raw" > "$tmp"
  if ! strip_font < "$svg" | cmp -s - "$tmp"; then
    echo "$svg is not up to date. Run 'go generate ./...' and commit the result." >&2
    strip_font < "$svg" | diff -u "$tmp" - >&2 || true
    failed=1
  fi
done

exit "$failed"
