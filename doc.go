// Package htmx reads htmx request headers and prepares validated response headers.
// It uses only the standard library. Applications own rendering, routing, cache
// policy, and the client script. Compatibility is tested against htmx 2.0.10.
//
// # Constructing Responses
//
// Use [Respond] for request-time options, or [NewResponse] to prepare an immutable
// [Response] before applying it. [Response.With] derives a new response by
// adding request-specific options to a prepared base. [MustResponse] is the panicking constructor for
// static startup configuration. Options compose with ordinary Go slices and
// conditionals; event payloads can be named structs or other JSON-encodable data.
// Check errors before writing status or HTML. No helper writes a body;
// [StopPolling] is the only helper that writes status.
//
// The package rejects malformed input, conflicting instructions, and values
// outside its supported subset. Returning an error before applying those
// instructions is intentional: the handler decides how to recover. This is not
// a guarantee of the browser's final behavior or a replacement for app policy.
//
// Equal repeated singleton settings are accepted; different settings conflict.
// Choose one navigation action. Navigation cannot accompany top-level history
// or swap controls, or after-swap/after-settle triggers. Put follow-up controls
// inside [Location]. Immediate triggers are allowed, though navigation can end
// the current page's lifetime. These are composition rules: the client can
// otherwise silently ignore instructions because of its processing order.
//
// Empty strings and zero options do nothing unless an effective nested option
// requires an event name, location path, or swap strategy. Whitespace-only input
// is rejected. An empty response is a no-op; see [Response.Apply] for existing
// header handling and per-call atomicity.
//
// # URLs and Selectors
//
// URL arguments accept relative or HTTP(S) URLs without credentials, whitespace,
// control characters, or backslashes. Use ASCII wire spelling: percent-encode
// Unicode paths and use ASCII hostnames. This supported subset is not a claim
// that other URL schemes are universally invalid. Applications decide permitted
// destinations and origin policy; the package has no request origin to check.
// Browser restrictions and client configuration can still reject a destination.
//
// History URL constructors reject the marker strings "true" and "false"; use
// [SuppressHistory], [LocationSuppressHistory], or [LocationReplaceDestination]
// as appropriate. Literal paths /true and /false work. Current-response history
// uses [PushURL], [ReplaceURL], and [SuppressHistory]. Follow-up history uses
// [LocationPushURL], [LocationReplaceURL], [LocationReplaceDestination], and
// [LocationSuppressHistory]; see those functions for client overrides.
//
// Selectors are checked for transport, not parsed as CSS. The package cannot
// check target existence or fragment overlap. Direct-header selectors must be
// ASCII; CSS escapes can represent Unicode. JSON-contained location and event
// targets can contain Unicode. Scroll/show modifiers have further whitespace
// restrictions; see [Scroll] and [Show].
//
// # Requests and Status Codes
//
// Request helpers read client-supplied hints. They neither authenticate a request
// nor establish its HTTP method. Boolean helpers require exactly "true".
// [WantsFragment] describes a rendering convention, not a universal page policy.
// Its example shows application-owned Vary handling for page/fragment variants.
//
// Send navigation headers on non-3xx responses, normally 200. Browsers follow
// HTTP redirects before htmx can inspect intermediate headers. A location
// follow-up may itself redirect; intermediate response headers still do not run.
//
// A validation fragment can use 200 with the client's defaults. To swap a 422
// response, configure the client before making the request, for example:
//
//	htmx.config.responseHandling.unshift({code: '422', swap: true, error: false});
//
// [Retarget] and [Reswap] alone do not enable 422 swapping. A 204 can deliver
// immediate events but normally does not swap. Later trigger phases depend on
// swap processing; [SwapNone] can still perform OOB work and emit those events.
// See [StopPolling] for status 286 and response-handling requirements.
//
// # Extending the Package
//
// Keep this a leaf package: no sibling dependencies, rendering, middleware,
// asset serving, or extension-specific transports. Preserve immutable state,
// input snapshots, and validation before mutation. Add browser fixtures before
// expanding compatibility claims. testdata/standalone.sh checks extraction and
// standard-library-only runtime dependencies; CONTRIBUTING.md lists the checks.
package htmx
