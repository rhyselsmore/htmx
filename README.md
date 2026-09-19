# htmx

[![Go Reference](https://pkg.go.dev/badge/github.com/rhyselsmore/htmx.svg)](https://pkg.go.dev/github.com/rhyselsmore/htmx)
[![Tests](https://github.com/rhyselsmore/htmx/actions/workflows/tests.yml/badge.svg)](https://github.com/rhyselsmore/htmx/actions/workflows/tests.yml)

Read htmx request headers and construct response headers from Go. The package
uses only the standard library. Applications own HTML rendering, routing,
caching policy, and the client script.

Requires **Go 1.21 or later**. Client compatibility is tested against
**htmx 2.0.10** in Chromium, Firefox, and WebKit.

```sh
go get github.com/rhyselsmore/htmx
```

```go
import "github.com/rhyselsmore/htmx"
```

## Send a Fragment and an Event

```go
if err := htmx.Respond(w,
    htmx.Retarget("#items"),
    htmx.Reswap(htmx.SwapOuterHTML),
    htmx.Trigger("saved", htmx.Detail(struct {
        ID string `json:"id"`
    }{ID: "42"})),
); err != nil {
    http.Error(w, "Could not prepare response", http.StatusInternalServerError)
    return
}
if _, err := fmt.Fprint(w, `<section id="items">Saved</section>`); err != nil {
    log.Printf("write response: %v", err)
    return
}
```

Check the error before writing status or HTML. `Respond` writes neither; success
prepares the headers and failure leaves the entire existing header map unchanged.
The [executable examples](example_test.go) cover construction, reuse, events,
navigation, polling, conflicts, and cache variation.

Use `NewResponse(opts...)` to prepare an immutable `*Response`, then `Apply(w)`
to use it. `MustResponse` panics on invalid configuration and is intended for
static startup settings. A response can be reused concurrently with different
writers. Sharing one writer or header map concurrently is unsupported.

Options are opaque values and compose with ordinary Go slices:

```go
opts := []htmx.ResponseOpt{htmx.Retarget("#items")}
opts = append(opts, htmx.Trigger("saved"))
if err := htmx.Respond(w, opts...); err != nil {
    http.Error(w, "Could not prepare response", http.StatusInternalServerError)
    return
}
```

## Available Options

| Purpose | Functions |
| --- | --- |
| Navigation | `Redirect`, `Refresh`, `Location` |
| Response history | `PushURL`, `ReplaceURL`, `SuppressHistory` |
| Current response swap | `Retarget`, `Reselect`, `Reswap` |
| Events | `Trigger`, `TriggerAfterSwap`, `TriggerAfterSettle` |
| Event options | `Detail`, `TriggerTarget` |
| Location selectors | `LocationSource`, `LocationTarget`, `LocationSelect` |
| Location fragments | `LocationSwap`, `LocationSelectOOB`, `OOB` |
| Location request data | `LocationValues`, `LocationHeaders` |
| Location history | `LocationPushURL`, `LocationReplaceURL`, `LocationReplaceDestination`, `LocationSuppressHistory` |
| Swap modifiers | `SwapDelay`, `SettleDelay`, `Transition`, `IgnoreTitle`, `FocusScroll`, `Scroll`, `Show` |

Equal repeated settings are accepted; differing settings conflict. These rules
apply within one response and when applying over existing headers. Empty strings
and zero options do nothing. An empty event name, location path, or swap strategy
with effective nested options is an error. Whitespace-only input is invalid.

Choose one navigation action. Navigation cannot accompany top-level history or
swap controls; put location controls inside `Location(...)`. Navigation also
cannot accompany after-swap or after-settle triggers. Immediate triggers are
allowed, though navigation can end the current page's lifetime.

## Events and Payloads

Each trigger call describes one named browser event. The three constructors
choose when it fires: on receipt, after swapping, or after settling. The later
phases depend on the client actually processing a swap; `SwapNone` can still
perform OOB work and emit those events.

`Detail(value)` marshals the value when the option is created. Later changes to
maps, slices, pointers, or raw JSON do not change the response. Encoding errors
are returned when the option is consumed. Custom marshalers run once per option
creation; their programming panics are not recovered. Do not mutate a value
concurrently while its option is being created.

Use `Detail[any](nil)` for explicit JSON null. `Detail(nil)` cannot infer a type.
Only one detail is allowed per event. Repeating an event name in the same phase
replaces its complete payload **and target**, including a replacement with
neither. Names in different phases are independent.

`TriggerTarget("#notifications")` sets routing. Object payloads expose their
fields directly on `event.detail`; scalar, array, and null payloads use
`event.detail.value`. A top-level `target` must use `TriggerTarget`. The client
owns `elt`, and an `error` field has htmx error-event semantics; put domain fields
with those names inside a `value` object.

Names must match `[A-Za-z_][A-Za-z0-9_.:-]*`; `hasOwnProperty` is rejected because
it breaks the pinned client's dispatcher. This is the package's supported
subset of DOM event names. Mixed targeted/ordinary events are supported. The
encoder orders them to avoid target leakage in htmx 2.0.10; this is not an event
sequencing API.

JSON headers escape Unicode and DEL for XHR transport. Large numeric tokens
survive server-side accumulation, but JavaScript still has its usual numeric
limits. Use strings for identifiers beyond its safe integer range.

## Navigation and History

`Redirect("/sign-in")` performs a full-page navigation. `Refresh()` reloads the
current page. `Location(path, opts...)` makes an AJAX navigation and receives its
HTML from the destination endpoint.

Send navigation headers on a non-3xx response, normally 200. Browsers follow HTTP
redirects before htmx can inspect the intermediate headers. An HTTP redirect in
a location's follow-up request is fine; headers on that intermediate redirect
are still not processed.

| Intent | Current response | Location follow-up |
| --- | --- | --- |
| Push a fixed URL | `PushURL("/chosen")` | `LocationPushURL("/chosen")` |
| Replace with a fixed URL | `ReplaceURL("/chosen")` | `LocationReplaceURL("/chosen")` |
| Replace with the eventual destination | — | `LocationReplaceDestination()` |
| Suppress history | `SuppressHistory()` | `LocationSuppressHistory()` with the limitation below |

`LocationReplaceDestination` uses the follow-up's final response path and query,
including redirects. The client falls back to its request path if no response
URL is available and may retain the original request anchor. It does not copy
the path supplied to `Location` on the server.

`LocationSuppressHistory` disables explicit push/replace, but a boosted source
can still cause a push. The destination can emit `SuppressHistory()` to override
that behavior. All location history choices can be overridden by the destination
response's own history headers or client hooks.

URL arguments accept relative or HTTP(S) URLs without credentials, control
characters, or backslashes. They must use ASCII wire spelling: percent-encode
Unicode paths and use ASCII hostnames. The package cannot establish same origin
without a request origin; authorization and destination policy belong to the app.
History URL constructors reject the markers `"true"` and `"false"`; use the named
helpers instead. Literal paths `/true` and `/false` work.

## Location Data and OOB Fragments

```go
if err := htmx.Respond(w, htmx.Location("/items/latest",
    htmx.LocationTarget("#items"),
    htmx.LocationSelect("#items"),
    htmx.LocationSwap(htmx.SwapOuterHTML),
    htmx.LocationSelectOOB(
        htmx.OOB("alerts", htmx.SwapOuterHTML),
        htmx.OOB("item-count", htmx.SwapInnerHTML),
    ),
    htmx.LocationReplaceDestination(),
)); err != nil {
    http.Error(w, "Could not prepare navigation", http.StatusInternalServerError)
    return
}
w.WriteHeader(http.StatusOK)
```

The destination returns sibling fragments:

```html
<section id="items"><p>Updated items</p></section>
<div id="alerts">Saved!</div>
<span id="item-count">3</span>
```

The page must already contain the corresponding targets. The client replaces
`items` and `alerts`, then updates the contents of the existing `item-count`
element. OOB fragments are consumed before the main selection. Keep the selected
fragments separate; the server cannot check DOM overlap or target existence.
A missing source fragment is skipped. A source without a page target causes an
htmx OOB error. For OOB event listeners, `event.detail.target` identifies the
destination; `event.target` can be another element in the client's settle list.

`OOB` takes a bare ID matching `[A-Za-z_][A-Za-z0-9_-]*`, without `#`. It supports
all eight HTML swap strategies, excluding `SwapTextContent`. `outerHTML` uses
the wrapper; inner and insertion strategies use its contents. Selection order
is preserved. Identical pairs collapse; the same ID with different strategies
conflicts. Empty selections do not disable inherited `hx-select-oob` or OOB
markup. There is no `HX-Select-OOB` response header.

Ordinary responses can also carry OOB markup, with no location option:

```html
<p>Primary response content</p>
<div id="alerts" hx-swap-oob="true">Saved!</div>
<div hx-swap-oob="beforeend:#messages"><p>Another message</p></div>
```

Your templates produce that HTML. The final wrapper supplies content to insert
inside the existing `messages` element.

Use `LocationValues(url.Values{...})` for string form/query parameters. Repeated
strings become repeated parameters in slice order; empty slices contribute
nothing. Nested JSON values are not accepted. The key `hasOwnProperty` is
rejected because of the client's form conversion.

`LocationHeaders(map[string]string{...})` supplies request headers. Names are
validated and canonicalized; case-insensitive duplicates and `__proto__` are
rejected. Values must be ASCII without controls. Browser restrictions and
client hooks still apply. Both helpers copy their inputs at option creation.

## Swaps

Main swaps support `SwapInnerHTML`, `SwapOuterHTML`, `SwapTextContent`,
`SwapBeforeBegin`, `SwapAfterBegin`, `SwapBeforeEnd`, `SwapAfterEnd`, `SwapDelete`,
and `SwapNone`. Use typed modifiers rather than strings containing extra tokens:

```go
htmx.Reswap(htmx.SwapInnerHTML,
    htmx.SwapDelay(100*time.Millisecond),
    htmx.SettleDelay(0),
    htmx.IgnoreTitle(true),
    htmx.Show(htmx.ScrollNone, ""),
)
```

Delays must be exact whole milliseconds between 0 and 2147483647ms. Zero and false
are explicit settings, distinct from omission. `Scroll` and `Show` accept top or
bottom and an optional selector; empty means the swap target and `window` is
supported. `Show(ScrollNone, "")` disables showing. Scroll selectors cannot
contain whitespace because the client splits modifiers into whitespace tokens.

`SwapTextContent` bypasses HTML selection and OOB processing. Explicitly combining
it with `Reselect`, or location selection/OOB options, returns a conflict. Selectors
are checked for safe transport, not parsed as CSS. Direct-header selectors must
be ASCII; CSS escapes can represent Unicode. JSON-contained location and event
target selectors may contain Unicode.

## Errors and Existing Headers

| Error family | Meaning |
| --- | --- |
| `ErrInvalidConfig` | An option value or required nested setting is invalid. |
| `ErrConflict` | Instructions disagree, or one event has multiple details. |
| `ErrInvalidHeader` | A managed header already on the writer is malformed or unsupported. |
| `ErrEncode` | A trigger payload could not be encoded as JSON. |

Use `errors.Is` for the family and `errors.As` for `*htmx.Error`. It includes the
operation, header, event, and field. Its message does not echo payloads, raw header
values, or arbitrary marshaler error text. Causes remain available through
unwrapping; logging a cause is a separate application decision.

Nonempty application reads all managed headers, including case aliases inserted
directly into the map. It rejects multiple values, malformed JSON, duplicate
object keys, unknown location fields, non-ASCII wire text, and legacy `HX-Push`.
Supported existing values are normalized and merged. Singleton settings must
agree; trigger names accumulate across calls. Other headers are untouched.

The empty response is a complete no-op, even over malformed existing headers.
`StopPolling` always validates existing headers before writing status 286.
An existing `HX-Replace-Url: false` normalizes to `HX-Push-Url: false`, removing
the old representation. Both history headers together are rejected.

Atomicity applies to **one call**. Earlier successful htmx headers remain if a
later call fails. `http.Error` does not clear them. Collect options into one call,
or have the application's error responder explicitly remove the headers it owns.
No helper can detect whether an arbitrary writer already committed the response.

## Requests, Caching, and Status Codes

`IsRequest`, `IsBoosted`, `IsHistoryRestore`, `TriggerID`, `TriggerName`, `Target`,
`CurrentURL`, and `Prompt` read client-supplied headers. They do not authenticate
a request or prove an HTTP method.

`WantsFragment(r)` means request=true, boosted=false, and history-restore=false.
It is a rendering convention; apps with custom boosted targets may need their
own policy. History cache misses need a full page.

For endpoints serving both page and fragment variants, merge `HX-Request`,
`HX-Boosted`, and `HX-History-Restore-Request` into `Vary` on **both** variants.
Preserve existing fields and `Vary: *`. The application-owned helper in
[example_test.go](example_test.go) shows this without adding caching middleware
to the package.

A validation fragment can use 200 with htmx's defaults. To swap a 422 response,
configure the client first, for example:

```js
htmx.config.responseHandling.unshift({code: '422', swap: true, error: false});
```

`Retarget` and `Reswap` alone do not enable 422 swapping. A 204 response can still
deliver immediate events, but normally does not swap.

`StopPolling(w, opts...)` validates, applies headers, and writes 286 exactly once.
It rejects navigation and writes nothing on error. The caller may write a body
on success. Polling stops only if the client processes that response; configuring
286 not to swap prevents the pinned client's cancellation branch from running.

## Verification and Maintenance

From the repository root:

```sh
go test ./...
go vet ./... ./testdata/browser/server
go test -race -coverprofile=/tmp/htmx.cover ./...
go test . -run='^$' -bench=. -benchmem
sh testdata/standalone.sh
```

Normal Go tests include the executable examples and fuzz regression corpus;
they need no browser tools or downloads. CI also runs each fuzz target for 30s.

Browser tests require **Node 22**, Go, and the pinned Playwright engines:

```sh
npm --prefix testdata/browser ci
npm --prefix testdata/browser run browsers:install
npm --prefix testdata/browser run test:setup
npm --prefix testdata/browser run test:client
npm --prefix testdata/browser test
```

The fixture serves the pinned client locally, starts its Go server on an available
port, and shuts it down after testing. Failed tests retain Playwright traces.
The [fixture README](testdata/browser/README.md) describes the files and test setup.
Run `npm --prefix testdata/browser run format` after editing
the JavaScript fixtures; CI checks their formatting. The vendored client is
excluded from formatting and verified against its pinned checksum.
The isolated client probes cover branches such as missing XHR response URLs;
they supplement the real browser suite.

Keep this a leaf package: no sibling dependencies, rendering, middleware, asset
serving, or extension-specific transports. Preserve typed immutable state,
payload snapshots, explicit errors, and validation before header mutation when
extending the API. Add client fixtures before expanding the compatibility claim.

Standalone verification copies the module and Go tests into a temporary directory
and checks that runtime dependencies are all standard library. CI tests the Go
1.21 minimum and the current stable Go release. The test workflow also runs race
detection, vet, bounded fuzzing, benchmarks, and the browser fixtures.

The lint job runs [golangci-lint](https://github.com/golangci/golangci-lint-action)
v2.13.0 with the repository configuration. To run the same checks locally:

```sh
golangci-lint run ./... ./testdata/browser/server
```

## License

Licensed under the [MIT License](LICENSE).
The vendored htmx test client retains its [Zero-Clause BSD license](testdata/browser/vendor/LICENSE).
