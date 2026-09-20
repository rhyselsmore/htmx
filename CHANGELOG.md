# Changelog

## v1.0.0 - 2026-09-20

Initial public release, extracted from an internal application without its
earlier Git history.

### Added

- Request-header helpers, including fragment and history-restore detection.
- Typed response options for navigation, history, swaps, events, and polling.
- Immutable responses with `Response.With` for adding request-specific options
  to a prepared base.
- Validation before header changes, explicit conflict errors, and unchanged
  headers on failure within one call.
- Captured event payloads, event targeting, and location follow-up options for
  request data and OOB fragments.
- Go documentation, executable examples, and browser regression tests.

### Compatibility

- Requires Go 1.21 or later; runtime dependencies are standard library only.
- Tested against htmx 2.0.10 in Chromium, Firefox, and WebKit.
- Applications own HTML rendering, routing, caching policy, and the client script.
- Distributed under MIT; the vendored test client retains its own license.
