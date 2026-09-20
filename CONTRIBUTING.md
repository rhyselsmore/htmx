# Contributing

Keep this a standard-library-only leaf package. Add tests for observable behavior,
including failure atomicity, and browser fixtures when expanding client support.
Preserve payload snapshots and immutable responses. Rendering, routing, middleware,
asset serving, and application policy belong to callers.

## Checks

Run from the module root with Go 1.21 or later:

```sh
go test ./... ./testdata/browser/server
go vet ./... ./testdata/browser/server
go test -race -coverprofile=/tmp/htmx.cover ./... ./testdata/browser/server
sh testdata/standalone.sh
go test . -run='^$' -bench=. -benchmem
```

Standalone verification copies the module and Go tests into a temporary directory,
checks that runtime dependencies are standard library, and runs tests and vet.
CI runs the Go 1.21 minimum and the current stable toolchain.

Run each target listed in `.github/workflows/tests.yml` for 30 seconds:

```sh
for target in FuzzTriggerRoundTrip FuzzTriggerAccumulation FuzzLocationRoundTrip FuzzApplyAtomicity FuzzResponseWith; do
    go test . -run='^$' -fuzz="^${target}$" -fuzztime=30s -parallel=2
done
```

Use the configured golangci-lint v2.13.0 on its supported Go toolchain:

```sh
golangci-lint run ./... ./testdata/browser/server
```

The [browser fixture README](testdata/browser/README.md) covers Node 22, the pinned
Playwright engines, formatting, setup checks, and source probes. Source probes
supplement the real browser suite. Keep artifact data local and synthetic.

## Documentation

Keep the README focused on motivations, scope, and normal usage. Put exact input
rules and limitations in Go comments next to the relevant API, and provide
executable examples. Distinguish transport constraints, known client failures,
composition rules, and deliberate supported-subset choices. Review rendered Go
documentation and links when moving material or adding symbols.

## Releases

Version this module independently of htmx. Inspect existing tags before choosing
a release number. Use v1.0.0 when ready to commit to API compatibility; use a v0
release while the API is unsettled or an rc prerelease for release testing.
Validation and merge rules are part of the public contract. A fix restoring
documented behavior can be a patch; deliberately rejecting supported inputs or
dropping a documented client guarantee needs compatibility review. Adding an API
requires a minor release; incompatible changes require a major release and, from
v2 onward, a matching Go module/import path suffix.

Record exact tested client versions in the README and release notes. Run browser
fixtures against retained and newly supported versions before expanding support.
An upstream release alone does not require a module version change.

Record user-facing changes under Unreleased in [CHANGELOG.md](CHANGELOG.md).
Include migration instructions for breaking changes and identify changes to
tested client versions or Go requirements. When preparing a release, move those
entries under its version and release date, then use them for GitHub release
notes. Keep entries about observable changes rather than individual commits.

For a hotfix, branch from the affected release tag if main contains unrelated
work. Add a regression, fix it, run the release checks, tag the next patch, and
merge the fix forward as needed. Never move a published tag or use +hotfix build
metadata instead of a patch increment. Backports to older lines are decided per
release; there is no promise to maintain every historical line.

Before publishing, run all checks above and the workflow's browser and lint
checks, review compatibility changes, verify documentation links, and inspect
the release files for generated artifacts or sensitive data.
