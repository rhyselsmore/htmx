// Package htmx reads htmx request headers and prepares response headers.
//
// Respond(w, opts...) validates the complete response before changing headers.
// Callers handle its error before rendering HTML or writing status. StopPolling
// is the only helper that writes status; no helper writes a body.
//
// Responses are immutable and reusable with independent writers. Request helpers
// inspect client-supplied headers; they do not authenticate a request. Rendering,
// routing, caching policy, and browser assets belong to the application.
package htmx

// StatusStopPolling tells htmx to stop polling under its default response handling.
const StatusStopPolling = 286
