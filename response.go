package htmx

import (
	"maps"
	"net/http"
	"sort"
	"strings"
)

// Response-side header names, as consumed by htmx on the client.
const (
	HeaderLocation           = "HX-Location"
	HeaderPushURL            = "HX-Push-Url"
	HeaderReplaceURL         = "HX-Replace-Url"
	HeaderRedirect           = "HX-Redirect"
	HeaderRefresh            = "HX-Refresh"
	HeaderReswap             = "HX-Reswap"
	HeaderRetarget           = "HX-Retarget"
	HeaderReselect           = "HX-Reselect"
	HeaderTrigger            = "HX-Trigger"
	HeaderTriggerAfterSettle = "HX-Trigger-After-Settle"
	HeaderTriggerAfterSwap   = "HX-Trigger-After-Swap"
)

// SwapStrategy mirrors the hx-swap values htmx accepts in HX-Reswap.
type SwapStrategy string

const (
	SwapInnerHTML   SwapStrategy = "innerHTML"
	SwapOuterHTML   SwapStrategy = "outerHTML"
	SwapTextContent SwapStrategy = "textContent"
	SwapBeforeBegin SwapStrategy = "beforebegin"
	SwapAfterBegin  SwapStrategy = "afterbegin"
	SwapBeforeEnd   SwapStrategy = "beforeend"
	SwapAfterEnd    SwapStrategy = "afterend"
	SwapDelete      SwapStrategy = "delete"
	SwapNone        SwapStrategy = "none"
)

// Response is immutable and can be applied concurrently to distinct writers.
// Its zero value is a no-op. Apply before committing a status or body.
// Sharing one writer or header map concurrently is unsupported.
type Response struct{ state responseState }

// ResponseOpt configures a response. Its zero value does nothing.
// Constructors return opaque options; invalid values are reported when consumed
// by [NewResponse], [Response.With], [Respond], or [StopPolling]. See the package
// documentation for composition rules and [Detail] for payload snapshot timing.
type ResponseOpt struct{ apply func(*responseState) error }

type navigation struct {
	kind, path string
	location   *locationData
}

type history struct{ kind, path string }

type responseState struct {
	triggers           [3]eventSet
	nav                navigation
	history            history
	swap               setting[swapSpec]
	retarget, reselect setting[string]
}

func (s responseState) empty() bool {
	return len(s.triggers[0].events) == 0 &&
		len(s.triggers[1].events) == 0 &&
		len(s.triggers[2].events) == 0 &&
		s.nav.kind == "" && s.history.kind == "" &&
		!s.swap.present && !s.retarget.present && !s.reselect.present
}

func (s *responseState) navigation(n navigation) error {
	if s.nav.kind != "" && (s.nav.kind != n.kind || s.nav.path != n.path || !s.nav.location.equal(n.location)) {
		return fault(ErrConflict, HeaderLocation, "navigation")
	}
	s.nav = n
	return nil
}

func (s *responseState) setHistory(h history) error {
	if s.history.kind != "" && s.history != h {
		return fault(ErrConflict, HeaderPushURL, "history")
	}
	s.history = h
	return nil
}

func (s *responseState) validate(polling bool) error {
	if s.nav.kind != "" && (s.history.kind != "" || s.swap.present || s.retarget.present || s.reselect.present) {
		return fault(ErrConflict, navHeader(s.nav.kind), "navigation")
	}
	if s.nav.kind != "" && (len(s.triggers[1].events) > 0 || len(s.triggers[2].events) > 0) {
		return fault(ErrConflict, navHeader(s.nav.kind), "navigation.triggers")
	}
	if s.swap.value.strategy == SwapTextContent && s.reselect.present {
		return fault(ErrConflict, HeaderReselect, "swap.selection")
	}
	if polling && s.nav.kind != "" {
		return fault(ErrConflict, navHeader(s.nav.kind), "polling")
	}
	return nil
}

func navHeader(kind string) string {
	switch kind {
	case "redirect":
		return HeaderRedirect
	case "refresh":
		return HeaderRefresh
	default:
		return HeaderLocation
	}
}

func (s *responseState) merge(n responseState) error {
	for i, p := range n.triggers {
		s.triggers[i].merge(p)
	}
	if n.nav.kind != "" {
		if err := s.navigation(n.nav); err != nil {
			return err
		}
	}
	if n.history.kind != "" {
		if err := s.setHistory(n.history); err != nil {
			return err
		}
	}
	if n.swap.present {
		if err := assign(&s.swap, n.swap.value, HeaderReswap, "swap"); err != nil {
			return err
		}
	}
	if n.retarget.present {
		if err := assign(&s.retarget, n.retarget.value, HeaderRetarget, "retarget"); err != nil {
			return err
		}
	}
	if n.reselect.present {
		if err := assign(&s.reselect, n.reselect.value, HeaderReselect, "reselect"); err != nil {
			return err
		}
	}
	return nil
}

// NewResponse validates the complete set of options and returns an immutable
// response, or nil and an error. It writes nothing. Applying a valid response
// can still fail if existing writer headers conflict or are malformed.
// See [Response.Apply] for application semantics.
func NewResponse(opts ...ResponseOpt) (*Response, error) {
	s, err := construct(opts, "NewResponse")
	if err != nil {
		return nil, err
	}
	return &Response{state: s}, nil
}

func construct(opts []ResponseOpt, op string) (responseState, error) {
	return constructFrom(responseState{}, opts, op)
}

// With returns a new response containing r's instructions followed by opts.
// It does not change r or write headers, status, or body. Errors return nil and
// leave r usable. The zero response is a valid base; a nil receiver is an error.
// No options (or only no-op options) return an equivalent, distinct response.
// Concurrent derivation and application to independent writers are supported.
//
// With uses the same construction rules as [NewResponse]. Equal singleton
// settings agree; different settings conflict. Repeating an event name replaces
// its complete payload and target in that phase. A repeated [Location] must
// agree as a whole: With does not patch nested location fields, override
// singletons, or clear settings with empty options.
//
// Captured payloads remain unchanged; custom marshalers are not rerun. New
// option errors and combined-response conflicts are returned with operation
// With. A successfully derived response can still fail [Response.Apply] if the
// writer's existing headers are malformed or conflict with its instructions.
func (r *Response) With(opts ...ResponseOpt) (*Response, error) {
	if r == nil {
		return nil, contextError(invalid("", "response"), "With", "", "")
	}
	draft := r.state
	for i := range draft.triggers {
		draft.triggers[i].events = maps.Clone(draft.triggers[i].events)
	}
	s, err := constructFrom(draft, opts, "With")
	if err != nil {
		return nil, err
	}
	return &Response{state: s}, nil
}

// The caller owns s's mutable event maps. Payload and location snapshots are
// immutable; option functions write only to the separate per-option draft.
func constructFrom(s responseState, opts []ResponseOpt, op string) (responseState, error) {
	var part responseState
	var conflict error
	for _, o := range opts {
		if o.apply == nil {
			continue
		}
		// Validate each option independently before choosing cross-option conflicts.
		// Reuse the draft; opaque options cannot retain its address.
		part = responseState{}
		if err := o.apply(&part); err != nil {
			return responseState{}, contextError(err, op, "", "")
		}
		conflict = priorConflict(conflict, s.merge(part))
	}
	conflict = priorConflict(conflict, s.validate(false))
	if conflict != nil {
		return responseState{}, contextError(conflict, op, "", "")
	}
	return s, nil
}

func priorConflict(first, next error) error {
	if next == nil {
		return first
	}
	if first == nil {
		return next
	}
	priority := func(err error) int {
		e := err.(*Error)
		switch e.Header {
		case HeaderLocation, HeaderRedirect, HeaderRefresh:
			return 0
		case HeaderPushURL, HeaderReplaceURL:
			return 1
		case HeaderReswap, HeaderRetarget, HeaderReselect:
			return 2
		case HeaderTrigger:
			return 3
		case HeaderTriggerAfterSwap:
			return 4
		case HeaderTriggerAfterSettle:
			return 5
		default:
			return 6
		}
	}
	if priority(next) < priority(first) {
		return next
	}
	return first
}

// MustResponse is NewResponse for static startup configuration. It panics on error.
func MustResponse(opts ...ResponseOpt) *Response {
	r, err := NewResponse(opts...)
	if err != nil {
		panic(err)
	}
	return r
}

// Respond constructs and applies options before any header changes.
// It writes no status or body. Check its error before rendering HTML.
// See [Response.Apply] for merging existing headers and failure atomicity.
func Respond(w http.ResponseWriter, opts ...ResponseOpt) error {
	s, err := construct(opts, "Respond")
	if err != nil {
		return err
	}
	return apply(w, s, false, "Respond")
}

// Apply merges r with existing managed headers. Errors leave the whole header
// map unchanged and write no status or body. A nil response is an error; the
// writer and its header map must be usable. A zero/empty response is a complete
// no-op, even if existing headers are malformed.
//
// A nonempty application reads every managed header, including case aliases
// inserted directly into the map. It rejects multiple values, malformed JSON,
// duplicate object keys, unknown location fields, non-ASCII wire text, and legacy
// HX-Push. Supported values are normalized and merged using the construction
// rules. Trigger names accumulate, with later definitions replacing earlier ones
// in the same phase. Unrelated headers are untouched.
//
// An existing HX-Replace-Url: false normalizes to HX-Push-Url: false, removing
// the old representation. Both history headers together are rejected.
//
// Atomicity applies to this call. Earlier successful headers remain if a later
// call fails; http.Error does not clear them. Collect related options in one
// response, or explicitly remove application-owned headers in an error responder.
// The package cannot detect whether an arbitrary writer already committed its
// response, and cannot undo HTML, status, or application work already performed.
func (r *Response) Apply(w http.ResponseWriter) error {
	if r == nil {
		return contextError(invalid("", "response"), "Apply", "", "")
	}
	return apply(w, r.state, false, "Apply")
}

// StopPolling validates and applies options, then writes status 286 exactly once.
// It always validates existing managed headers, even with no options. Navigation
// is rejected, including navigation already present on the writer. On error it
// changes no headers and writes no status or body. The caller may write a body
// after success.
//
// Polling stops only if the client processes the response. Configuring 286 not
// to swap prevents the pinned client's polling cancellation branch from running.
// See [Response.Apply] for existing-header and per-call atomicity rules.
func StopPolling(w http.ResponseWriter, opts ...ResponseOpt) error {
	s, err := construct(opts, "StopPolling")
	if err != nil {
		return err
	}
	if err = apply(w, s, true, "StopPolling"); err != nil {
		return err
	}
	w.WriteHeader(StatusStopPolling)
	return nil
}

var managedHeaders = [...]string{
	HeaderLocation,
	HeaderRedirect,
	HeaderRefresh,
	HeaderPushURL,
	HeaderReplaceURL,
	HeaderReswap,
	HeaderRetarget,
	HeaderReselect,
	HeaderTrigger,
	HeaderTriggerAfterSwap,
	HeaderTriggerAfterSettle,
	"HX-Push",
}

// Fixed protocol metadata: canonicalize once, not for every application.
var canonicalManagedHeaders = func() [len(managedHeaders)]string {
	var names [len(managedHeaders)]string
	for i, name := range managedHeaders {
		names[i] = http.CanonicalHeaderKey(name)
	}
	return names
}()

func apply(w http.ResponseWriter, s responseState, polling bool, op string) error {
	if s.empty() && !polling {
		return nil
	}
	h := w.Header()
	current, err := importHeaders(h)
	if err != nil {
		return contextError(err, op, "", "")
	}
	if err = current.merge(s); err != nil {
		return contextError(err, op, "", "")
	}
	if err = current.validate(polling); err != nil {
		return contextError(err, op, "", "")
	}
	patch, err := current.encode()
	if err != nil {
		return contextError(err, op, "", "")
	}
	// All possible returned errors precede this commit. Preserve unrelated slices.
	for i, name := range managedHeaders {
		canonical := canonicalManagedHeaders[i]
		for key := range h {
			if strings.EqualFold(key, name) && key != canonical {
				delete(h, key)
			}
		}
		value, ok := patch[name]
		if !ok {
			delete(h, canonical)
			continue
		}
		old := h[canonical]
		if len(old) == 1 && old[0] == value {
			continue
		}
		h[canonical] = []string{value}
	}
	return nil
}

func importHeaders(h http.Header) (responseState, error) {
	var s responseState
	for _, name := range managedHeaders {
		var aliases []string
		for k := range h {
			if strings.EqualFold(k, name) {
				aliases = append(aliases, k)
			}
		}
		sort.Strings(aliases)
		var values []string
		for _, k := range aliases {
			values = append(values, h[k]...)
		}
		if len(values) == 0 {
			continue
		}
		if len(values) != 1 || values[0] == "" || !headerASCII(values[0]) {
			return s, fault(ErrInvalidHeader, name, "header")
		}
		if name == HeaderReplaceURL && s.history.kind != "" {
			return s, fault(ErrConflict, name, "history")
		}
		err := s.importValue(name, values[0])
		if err != nil {
			e := contextError(err, "", name, "").(*Error)
			if e.Kind != ErrConflict {
				e.Kind = ErrInvalidHeader
			}
			return s, e
		}
	}
	return s, nil
}

func (s *responseState) importValue(name, value string) error {
	var o ResponseOpt
	switch name {
	case HeaderTrigger, HeaderTriggerAfterSwap, HeaderTriggerAfterSettle:
		p, err := parseTriggerHeader(value)
		if err != nil {
			return err
		}
		for i, h := range triggerHeaders {
			if h == name {
				s.triggers[i] = p
			}
		}
		return nil
	case HeaderRedirect:
		o = Redirect(value)
	case HeaderLocation:
		var err error
		o, err = parseLocation(value)
		if err != nil {
			return err
		}
	case HeaderRefresh:
		if value != "true" {
			return invalid(name, "refresh")
		}
		o = Refresh()
	case HeaderPushURL, HeaderReplaceURL:
		if value == "false" {
			o = SuppressHistory()
		} else if name == HeaderPushURL {
			o = PushURL(value)
		} else {
			o = ReplaceURL(value)
		}
	case HeaderRetarget:
		o = Retarget(value)
	case HeaderReselect:
		o = Reselect(value)
	case HeaderReswap:
		v, err := parseSwap(value)
		if err != nil {
			return err
		}
		return assign(&s.swap, v, HeaderReswap, "swap")
	default:
		return invalid(name, "header")
	}
	if o.apply == nil {
		return invalid(name, "header")
	}
	return o.apply(s)
}

func (s responseState) encode() (map[string]string, error) {
	out := make(map[string]string)
	for i, p := range s.triggers {
		if len(p.events) > 0 || p.json {
			value, err := encodeTriggerHeader(p)
			if err != nil {
				return nil, err
			}
			out[triggerHeaders[i]] = value
		}
	}
	if s.nav.kind != "" {
		value := s.nav.path
		if s.nav.kind == "refresh" {
			value = "true"
		}
		if s.nav.kind == "location" {
			var err error
			value, err = s.nav.location.wire()
			if err != nil {
				return nil, err
			}
		}
		out[navHeader(s.nav.kind)] = value
	}
	switch s.history.kind {
	case "push":
		out[HeaderPushURL] = s.history.path
	case "replace":
		out[HeaderReplaceURL] = s.history.path
	case "suppress":
		out[HeaderPushURL] = "false"
	}
	if s.swap.present {
		out[HeaderReswap] = s.swap.value.wire()
	}
	if s.retarget.present {
		out[HeaderRetarget] = s.retarget.value
	}
	if s.reselect.present {
		out[HeaderReselect] = s.reselect.value
	}
	return out, nil
}

func navigationOpt(kind, path string) ResponseOpt {
	return ResponseOpt{func(s *responseState) error {
		if path == "" && kind != "refresh" {
			return nil
		}
		if kind != "refresh" && !urlOK(path, false) {
			return invalid(navHeader(kind), "navigation.url")
		}
		return s.navigation(navigation{kind: kind, path: path})
	}}
}

// Redirect requests a full-page navigation. Send it on a non-3xx response.
// The URL must meet the package's supported URL rules; empty means no option.
// Authorization and destination policy remain with the application.
func Redirect(url string) ResponseOpt { return navigationOpt("redirect", url) }

// Refresh requests a full page reload when the client handles the response.
func Refresh() ResponseOpt { return navigationOpt("refresh", "") }

func historyOpt(kind, path string) ResponseOpt {
	return ResponseOpt{func(s *responseState) error {
		if path == "" && kind != "suppress" {
			return nil
		}
		if kind != "suppress" && !urlOK(path, true) {
			return invalid(HeaderPushURL, "history.url")
		}
		return s.setHistory(history{kind, path})
	}}
}

// PushURL adds a fixed URL to browser history after the current response swap.
// It follows the package URL rules and rejects the markers "true" and "false".
// Use [SuppressHistory] to disable history. Empty means no option.
// For AJAX location follow-ups, use [LocationPushURL].
func PushURL(url string) ResponseOpt { return historyOpt("push", url) }

// ReplaceURL replaces the current browser history entry with a fixed URL.
// It follows the package URL rules and rejects the markers "true" and "false".
// Empty means no option. For location follow-ups, use [LocationReplaceURL] or
// [LocationReplaceDestination] for the eventual destination.
func ReplaceURL(url string) ResponseOpt { return historyOpt("replace", url) }

// SuppressHistory asks the client to skip the entire response's history update.
func SuppressHistory() ResponseOpt { return historyOpt("suppress", "") }

// Retarget overrides the response's swap target selector.
func Retarget(selector string) ResponseOpt {
	return responseSelector("retarget", selector)
}

// Reselect chooses a fragment from the response HTML.
func Reselect(selector string) ResponseOpt {
	return responseSelector("reselect", selector)
}

func responseSelector(field, selector string) ResponseOpt {
	return ResponseOpt{func(s *responseState) error {
		if selector == "" {
			return nil
		}
		dst, header := &s.retarget, HeaderRetarget
		if field == "reselect" {
			dst, header = &s.reselect, HeaderReselect
		}
		if !validSelector(selector, true) {
			return invalid(header, field)
		}
		return assign(dst, selector, header, field)
	}}
}
