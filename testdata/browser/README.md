# Browser Fixtures

Run these commands from this directory with Node 22 and Go installed:

```sh
npm ci
npm run browsers:install
npm run test:setup
npm run test:client
npm test
```

`npm test` runs Chromium, Firefox, and WebKit. `setup.cjs` builds the Go fixture
into a temporary directory, starts it on an available loopback port, and passes
the origin to workers through `HTMX_TEST_BASE_URL`. Teardown stops the server and
removes the directory. Failed browser tests leave traces in `test-results/`.

CI uploads only `trace.zip` and `error-context.md` from failed runs, retained for
seven days. Traces contain request and response data, DOM snapshots, screenshots,
and test source. Keep these fixtures local and synthetic: traces are diagnostic
captures, not redacted logs. The polling `token` is a generated test identifier,
not an authentication credential.

- `baseline.spec.cjs` serves handwritten headers from a Node server. This checks
  the client independently of the Go encoder.
- `package.spec.cjs` exercises responses produced by the Go package. Query
  parameters select cases in `server/response.go` and `server/location.go`.
- `server/page.go` renders the target elements and per-case client configuration.
  `server/events.js` records DOM events and lifecycle snapshots for assertions.
  Polling cases use separate tokens so parallel tests do not share counters.
- `client-protocol.cjs` and `client-history.cjs` probe functions extracted from
  the pinned source. Their small stubs record parser decisions; they do not
  replace browser coverage of XHR, DOM changes, or navigation.
- `setup.test.cjs` checks startup and cleanup, including failed process creation.

Keep comments focused on why a fixture is constructed that way: client quirks,
source-node consumption, event timing, and the distinction between a skipped
swap and a stopped request. Keep assertions about observable results.

Run `npm run format` after editing JavaScript and `gofmt -w server/*.go` after
editing Go. CI checks JavaScript formatting. The client in `vendor/` is excluded
from formatting; `client-fixture.cjs` checks its checksum before probes or baseline
tests run. See [vendor/README.md](vendor/README.md) for its source and license.
