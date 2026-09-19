package htmx

import "net/http"

// Request-side header names, as sent by htmx.
const (
	HeaderRequest               = "HX-Request"
	HeaderBoosted               = "HX-Boosted"
	HeaderTriggerID             = "HX-Trigger"
	HeaderTriggerName           = "HX-Trigger-Name"
	HeaderTarget                = "HX-Target"
	HeaderCurrentURL            = "HX-Current-URL"
	HeaderPrompt                = "HX-Prompt"
	HeaderHistoryRestoreRequest = "HX-History-Restore-Request"
)

// IsRequest reports whether r was issued by htmx (HX-Request: true).
func IsRequest(r *http.Request) bool {
	return r.Header.Get(HeaderRequest) == "true"
}

// IsBoosted reports whether the request originated from an hx-boost link
// or form. Boosted requests are full-page navigations with an AJAX twist,
// so they typically still expect a full document in response.
func IsBoosted(r *http.Request) bool {
	return r.Header.Get(HeaderBoosted) == "true"
}

// IsHistoryRestore reports whether htmx is repopulating a history entry
// missing from its history cache. This header does not guarantee an HTTP method.
func IsHistoryRestore(r *http.Request) bool {
	return r.Header.Get(HeaderHistoryRestoreRequest) == "true"
}

// WantsFragment reports whether the request expects a fragment rather
// than a full page under the usual rendering convention. Boosted requests and
// history cache misses receive full pages; apps with custom targets may differ.
func WantsFragment(r *http.Request) bool {
	return IsRequest(r) && !IsBoosted(r) && !IsHistoryRestore(r)
}

// TriggerID returns the DOM id of the element that triggered the request.
func TriggerID(r *http.Request) string { return r.Header.Get(HeaderTriggerID) }

// TriggerName returns the name attribute of the triggering element.
func TriggerName(r *http.Request) string { return r.Header.Get(HeaderTriggerName) }

// Target returns the DOM id of the hx-target element.
func Target(r *http.Request) string { return r.Header.Get(HeaderTarget) }

// CurrentURL returns the browser URL at the time htmx issued the request.
func CurrentURL(r *http.Request) string { return r.Header.Get(HeaderCurrentURL) }

// Prompt returns the user's response to an hx-prompt, if any.
func Prompt(r *http.Request) string { return r.Header.Get(HeaderPrompt) }
