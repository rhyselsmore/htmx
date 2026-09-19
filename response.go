package htmx

import (
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
type Response struct{ state responseState }

// ResponseOpt configures a response. Its zero value does nothing.
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

// NewResponse validates options and snapshots an immutable response.
func NewResponse(opts ...ResponseOpt) (*Response, error) {
	s, err := construct(opts, "NewResponse")
	if err != nil {
		return nil, err
	}
	return &Response{state: s}, nil
}

func construct(opts []ResponseOpt, op string) (responseState, error) {
	var s, part responseState
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

// Respond validates and applies options before any header changes. It writes no status or body.
func Respond(w http.ResponseWriter, opts ...ResponseOpt) error {
	s, err := construct(opts, "Respond")
	if err != nil {
		return err
	}
	return apply(w, s, false, "Respond")
}

// Apply merges r with existing managed headers. Errors leave the whole map unchanged.
// A nil response is an error; the writer and its header map must be usable.
func (r *Response) Apply(w http.ResponseWriter) error {
	if r == nil {
		return contextError(invalid("", "response"), "Apply", "", "")
	}
	return apply(w, r.state, false, "Apply")
}

// StopPolling validates and applies options, then writes status 286 exactly once.
// Navigation is rejected, including navigation already present on the writer.
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

// PushURL adds an entry to browser history after the swap.
func PushURL(url string) ResponseOpt { return historyOpt("push", url) }

// ReplaceURL replaces the current browser history entry with a fixed URL.
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
