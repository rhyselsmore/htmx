#!/bin/sh
# Copy the Go package and tests into a dependency-free temporary module.
set -eu
package_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
standalone_dir=$(mktemp -d "${TMPDIR:-/tmp}/htmx-standalone.XXXXXX")
trap 'rm -rf "$standalone_dir"' EXIT HUP INT TERM
cp "$package_dir"/go.mod "$package_dir"/*.go "$standalone_dir/"
mkdir -p "$standalone_dir/testdata"
cp -R "$package_dir/testdata/fuzz" "$standalone_dir/testdata/"
export GOWORK=off
go -C "$standalone_dir" version
go -C "$standalone_dir" test -race ./...
go -C "$standalone_dir" vet ./...
nonstandard=$(go -C "$standalone_dir" list -deps -f '{{if not .Standard}}{{.ImportPath}}{{end}}' . | sed '/^$/d;\|^github.com/rhyselsmore/htmx$|d')
if [ -n "$nonstandard" ]; then
    echo "Unexpected nonstandard dependencies: $nonstandard" >&2
    exit 1
fi
