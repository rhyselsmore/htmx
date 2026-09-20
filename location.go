package htmx

import (
	"encoding/json"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"unicode/utf8"
)

// LocationOpt configures the follow-up AJAX request made by Location.
// Its zero value does nothing.
type LocationOpt struct{ apply func(*locationData) error }

type locationData struct {
	path                       string
	source, target, selectHTML setting[string]
	swap                       setting[swapSpec]
	history                    history
	values                     url.Values
	headers                    map[string]string
	oob                        []OOBSelection
}

// Location requests an AJAX navigation; the destination endpoint supplies HTML.
// The path follows the package URL rules. Empty path is a no-op only when all
// nested options are empty. Send on a non-3xx response; follow-up HTTP redirects
// are allowed but their intermediate headers are not processed.
//
// Configure the follow-up with LocationSource, LocationTarget, LocationSelect,
// LocationSwap, LocationSelectOOB, LocationValues, LocationHeaders, and the
// Location history helpers. These do not describe the current response's swap.
// Repeating Location must agree on the complete value; a later Location does not
// patch nested fields. Destination history headers and client hooks can override
// location history choices.
func Location(path string, opts ...LocationOpt) ResponseOpt {
	opts = append([]LocationOpt(nil), opts...)
	return ResponseOpt{func(s *responseState) error {
		d := locationData{path: path}
		for _, o := range opts {
			if o.apply != nil {
				if err := o.apply(&d); err != nil {
					return contextError(err, "", HeaderLocation, "")
				}
			}
		}
		if path == "" && d.bare() {
			return nil
		}
		if !urlOK(path, false) {
			return invalid(HeaderLocation, "location.path")
		}
		if d.swap.value.strategy == SwapTextContent && (d.selectHTML.present || len(d.oob) > 0) {
			return fault(ErrConflict, HeaderLocation, "location.swap")
		}
		return s.navigation(navigation{kind: "location", path: path, location: &d})
	}}
}

func (d locationData) bare() bool {
	return !d.source.present && !d.target.present && !d.selectHTML.present &&
		!d.swap.present && d.history.kind == "" &&
		len(d.values) == 0 && len(d.headers) == 0 && len(d.oob) == 0
}

func (d *locationData) equal(other *locationData) bool {
	if d == nil || other == nil {
		return d == other
	}
	return d.path == other.path && d.source == other.source &&
		d.target == other.target && d.selectHTML == other.selectHTML &&
		d.swap == other.swap && d.history == other.history &&
		maps.Equal(d.headers, other.headers) &&
		maps.EqualFunc(d.values, other.values, slices.Equal[[]string]) &&
		slices.EqualFunc(d.oob, other.oob, func(a, b OOBSelection) bool {
			return a.id == b.id && a.strategy == b.strategy
		})
}

// LocationSource chooses the element supplying request context to the follow-up.
func LocationSource(selector string) LocationOpt { return locationSelector("source", selector) }

// LocationTarget chooses the main target of the follow-up response.
func LocationTarget(selector string) LocationOpt { return locationSelector("target", selector) }

// LocationSelect chooses the main fragment from the follow-up HTML.
func LocationSelect(selector string) LocationOpt { return locationSelector("select", selector) }

func locationSelector(field, selector string) LocationOpt {
	return LocationOpt{func(d *locationData) error {
		if selector == "" {
			return nil
		}
		if !validSelector(selector, false) {
			return invalid(HeaderLocation, "location."+field)
		}
		dst := &d.source
		switch field {
		case "target":
			dst = &d.target
		case "select":
			dst = &d.selectHTML
		}
		return assign(dst, selector, HeaderLocation, "location."+field)
	}}
}

// LocationSwap configures the follow-up's main swap using the same strategies
// and modifiers as [Reswap]. [SwapTextContent] cannot accompany [LocationSelect]
// or [LocationSelectOOB], because it bypasses selection and OOB processing.
func LocationSwap(strategy SwapStrategy, opts ...SwapOpt) LocationOpt {
	opts = append([]SwapOpt(nil), opts...)
	return LocationOpt{func(d *locationData) error {
		v, err := buildSwap(strategy, opts)
		if err != nil {
			return err
		}
		if v.strategy == "" {
			return nil
		}
		return assign(&d.swap, v, HeaderLocation, "location.swap")
	}}
}

// LocationPushURL asks the follow-up request to push a fixed history URL.
func LocationPushURL(url string) LocationOpt { return locationHistory("push", url) }

// LocationReplaceURL asks the follow-up request to replace history with a fixed URL.
func LocationReplaceURL(url string) LocationOpt { return locationHistory("replace", url) }

// LocationReplaceDestination replaces history with the eventual destination
// path and query, including redirects. If no response URL is available, the
// client falls back to its request path and may retain the original anchor.
// It does not simply copy the path passed to [Location]. Follow-up response
// history headers and client hooks can override this choice.
func LocationReplaceDestination() LocationOpt { return locationHistory("destination", "") }

// LocationSuppressHistory disables the location request's explicit push/replace.
// A boosted source can still push; SuppressHistory on the destination overrides it.
func LocationSuppressHistory() LocationOpt { return locationHistory("suppress", "") }

func locationHistory(kind, path string) LocationOpt {
	return LocationOpt{func(d *locationData) error {
		if path == "" && (kind == "push" || kind == "replace") {
			return nil
		}
		if (kind == "push" || kind == "replace") && !urlOK(path, true) {
			return invalid(HeaderLocation, "location.history")
		}
		h := history{kind, path}
		if d.history.kind != "" && d.history != h {
			return fault(ErrConflict, HeaderLocation, "location.history")
		}
		d.history = h
		return nil
	}}
}

// LocationValues snapshots string form/query parameters at option creation.
// Repeated strings become repeated parameters in slice order; empty slices
// contribute nothing. Nested JSON values are not accepted. The key hasOwnProperty
// is rejected because of the pinned client's form conversion. Strings must be
// valid UTF-8. Equal repeated settings agree; different settings conflict.
func LocationValues(values url.Values) LocationOpt {
	copyValues := make(url.Values, len(values))
	for k, v := range values {
		if len(v) > 0 {
			copyValues[k] = slices.Clone(v)
		}
	}
	return LocationOpt{func(d *locationData) error {
		for _, k := range sortedKeys(copyValues) {
			if k == "hasOwnProperty" || !utf8.ValidString(k) {
				return invalid(HeaderLocation, "location.values")
			}
			for _, v := range copyValues[k] {
				if !utf8.ValidString(v) {
					return invalid(HeaderLocation, "location.values")
				}
			}
		}
		if len(copyValues) == 0 {
			return nil
		}
		if len(d.values) > 0 && !maps.EqualFunc(d.values, copyValues, slices.Equal[[]string]) {
			return fault(ErrConflict, HeaderLocation, "location.values")
		}
		d.values = copyValues
		return nil
	}}
}

// LocationHeaders snapshots request headers for the follow-up at option creation.
// Names are validated and canonicalized; case-insensitive duplicates and __proto__
// are rejected. Values must be ASCII without controls or surrounding whitespace.
// The browser still decides which headers it permits, and client hooks still
// apply. Equal repeated settings agree; different settings conflict.
func LocationHeaders(headers map[string]string) LocationOpt {
	headers = maps.Clone(headers)
	return LocationOpt{func(d *locationData) error {
		if len(headers) == 0 {
			return nil
		}
		normalized := make(map[string]string, len(headers))
		for _, name := range sortedKeys(headers) {
			value := headers[name]
			if !headerToken(name) || strings.EqualFold(name, "__proto__") || !textOK(value, true) {
				return invalid(HeaderLocation, "location.headers")
			}
			canonical := locationHeaderName(name)
			if _, ok := normalized[canonical]; ok {
				return invalid(HeaderLocation, "location.headers")
			}
			normalized[canonical] = value
		}
		if len(d.headers) > 0 && !maps.Equal(d.headers, normalized) {
			return fault(ErrConflict, HeaderLocation, "location.headers")
		}
		d.headers = normalized
		return nil
	}}
}

func headerToken(name string) bool {
	if name == "" {
		return false
	}
	for _, c := range []byte(name) {
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("!#$%&'*+-.^_`|~", rune(c)) {
			continue
		}
		return false
	}
	return true
}

func locationHeaderName(name string) string {
	for _, n := range []string{
		HeaderRequest,
		HeaderBoosted,
		HeaderTriggerID,
		HeaderTriggerName,
		HeaderTarget,
		HeaderCurrentURL,
		HeaderPrompt,
		HeaderHistoryRestoreRequest,
	} {
		if strings.EqualFold(name, n) {
			return n
		}
	}
	return http.CanonicalHeaderKey(name)
}

// OOBSelection describes a fragment ID and swap strategy. Its zero value does nothing.
type OOBSelection struct {
	id       string
	strategy SwapStrategy
	err      error
}

// OOB selects a bare element ID matching [A-Za-z_][A-Za-z0-9_-]*, without #.
// This is the package's supported ID subset. The strategy must be one of the eight
// HTML swap strategies; [SwapTextContent] and modifiers are not supported.
// outerHTML uses the wrapper; inner and insertion strategies use its contents.
// Empty ID and strategy together are a no-op. See [LocationSelectOOB] for fragment
// selection, target requirements, and ordinary OOB markup.
func OOB(id string, strategy SwapStrategy) OOBSelection {
	o := OOBSelection{id: id, strategy: strategy}
	if id == "" && strategy == "" {
		return o
	}
	if !validOOBID(id) {
		o.err = invalid(HeaderLocation, "id")
	} else if !validStrategy(strategy) || strategy == SwapTextContent {
		o.err = invalid(HeaderLocation, "strategy")
	}
	return o
}

func validOOBID(id string) bool {
	if id == "" {
		return false
	}
	for i, c := range []byte(id) {
		if c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' {
			continue
		}
		if i > 0 && (c == '-' || c >= '0' && c <= '9') {
			continue
		}
		return false
	}
	return true
}

// LocationSelectOOB selects ordered additional fragments from follow-up HTML.
// Equal ID/strategy pairs collapse; the same ID with different strategies conflicts.
// Empty selections do not disable inherited hx-select-oob or OOB markup. There is
// no HX-Select-OOB response header.
//
// OOB fragments are consumed before the main selection. Keep selected fragments
// separate: the server cannot check overlap or target existence. A missing source
// fragment is skipped; a source without a page target causes an htmx OOB error.
// For OOB listeners, event.detail.target identifies the destination; event.target
// can be another element in the client's settle list.
//
// For LocationTarget("#items"), LocationSelect("#items"),
// LocationSwap(SwapOuterHTML), and OOB("alerts", SwapOuterHTML), return siblings:
//
//	<section id="items"><p>Updated items</p></section>
//	<div id="alerts">Saved!</div>
//
// Both targets must already exist on the page. The executable example also
// updates a count using innerHTML.
//
// Ordinary responses can carry OOB markup without a location option. Templates
// produce the HTML, for example:
//
//	<p>Primary response content</p>
//	<div id="alerts" hx-swap-oob="true">Saved!</div>
//	<div hx-swap-oob="beforeend:#messages"><p>Another message</p></div>
//
// The last wrapper supplies content to insert inside the existing messages element.
func LocationSelectOOB(selections ...OOBSelection) LocationOpt {
	selections = slices.Clone(selections)
	return LocationOpt{func(d *locationData) error {
		for i, o := range selections {
			if o.err != nil {
				e := contextError(o.err, "", HeaderLocation, "").(*Error)
				e.Field = indexed("location.selectOOB", i, e.Field)
				return e
			}
			if o.id == "" && o.strategy == "" {
				continue
			}
			found := false
			for _, old := range d.oob {
				if old.id == o.id {
					if old.strategy != o.strategy {
						return fault(ErrConflict, HeaderLocation, indexed("location.selectOOB", i, "strategy"))
					}
					found = true
					break
				}
			}
			if !found {
				d.oob = append(d.oob, o)
			}
		}
		return nil
	}}
}

func (d locationData) wire() (string, error) {
	if d.bare() && !strings.HasPrefix(d.path, "{") {
		return d.path, nil
	}
	fields := map[string]json.RawMessage{}
	put := func(k string, v any) { fields[k], _ = json.Marshal(v) }
	put("path", d.path)
	for _, p := range []struct {
		key string
		v   setting[string]
	}{{"source", d.source}, {"target", d.target}, {"select", d.selectHTML}} {
		if p.v.present {
			put(p.key, p.v.value)
		}
	}
	if d.swap.present {
		put("swap", d.swap.value.wire())
	}
	if len(d.values) > 0 {
		put("values", d.values)
	}
	if len(d.headers) > 0 {
		put("headers", d.headers)
	}
	if len(d.oob) > 0 {
		entries := make([]string, 0, len(d.oob))
		for _, o := range d.oob {
			entries = append(entries, "#"+o.id+":"+string(o.strategy))
		}
		put("selectOOB", strings.Join(entries, ","))
	}
	switch d.history.kind {
	case "push":
		put("push", d.history.path)
	case "replace":
		put("push", "false")
		put("replace", d.history.path)
	case "destination":
		put("push", "false")
		put("replace", "true")
	case "suppress":
		put("push", "false")
		put("replace", "false")
	}
	raw, err := json.Marshal(fields)
	if err != nil {
		return "", err
	}
	return asciiJSON(raw), nil
}

func parseLocation(value string) (ResponseOpt, error) {
	if !strings.HasPrefix(strings.TrimSpace(value), "{") {
		return Location(value), nil
	}
	fields, err := rawObject([]byte(value))
	if err != nil {
		return ResponseOpt{}, err
	}
	var opts []LocationOpt
	var path, push, replace string
	var hasPush, hasReplace bool
	for _, key := range sortedKeys(fields) {
		raw := fields[key]
		switch key {
		case "values":
			values, err := rawObject(raw)
			if err != nil {
				return ResponseOpt{}, invalid(HeaderLocation, "location.values")
			}
			v := url.Values{}
			for _, k := range sortedKeys(values) {
				r := values[k]
				switch {
				case len(r) > 0 && r[0] == '"':
					var str string
					if json.Unmarshal(r, &str) != nil {
						return ResponseOpt{}, invalid(HeaderLocation, "location.values")
					}
					v[k] = []string{str}
				case len(r) > 0 && r[0] == '[':
					var items []json.RawMessage
					if json.Unmarshal(r, &items) != nil {
						return ResponseOpt{}, invalid(HeaderLocation, "location.values")
					}
					for _, item := range items {
						var str string
						if len(item) == 0 || item[0] != '"' || json.Unmarshal(item, &str) != nil {
							return ResponseOpt{}, invalid(HeaderLocation, "location.values")
						}
						v[k] = append(v[k], str)
					}
				default:
					return ResponseOpt{}, invalid(HeaderLocation, "location.values")
				}
			}
			opts = append(opts, LocationValues(v))
		case "headers":
			members, err := rawObject(raw)
			if err != nil {
				return ResponseOpt{}, invalid(HeaderLocation, "location.headers")
			}
			headers := map[string]string{}
			for _, k := range sortedKeys(members) {
				r := members[k]
				var v string
				if len(r) == 0 || r[0] != '"' || json.Unmarshal(r, &v) != nil {
					return ResponseOpt{}, invalid(HeaderLocation, "location.headers")
				}
				headers[k] = v
			}
			opts = append(opts, LocationHeaders(headers))
		case "path", "source", "target", "swap", "select", "selectOOB", "push", "replace":
			var v string
			if len(raw) == 0 || raw[0] != '"' || json.Unmarshal(raw, &v) != nil {
				return ResponseOpt{}, invalid(HeaderLocation, "location."+key)
			}
			switch key {
			case "path":
				path = v
			case "source":
				opts = append(opts, LocationSource(v))
			case "target":
				opts = append(opts, LocationTarget(v))
			case "select":
				opts = append(opts, LocationSelect(v))
			case "swap":
				if v != "" {
					swap, err := parseSwap(v)
					if err != nil {
						return ResponseOpt{}, err
					}
					opts = append(opts, LocationOpt{func(d *locationData) error { return assign(&d.swap, swap, HeaderLocation, "location.swap") }})
				}
			case "selectOOB":
				o, err := parseOOB(v)
				if err != nil {
					return ResponseOpt{}, err
				}
				opts = append(opts, LocationSelectOOB(o...))
			case "push":
				hasPush = true
				push = v
			case "replace":
				hasReplace = true
				replace = v
			}
		default:
			return ResponseOpt{}, invalid(HeaderLocation, "location.field")
		}
	}
	if path == "" {
		return ResponseOpt{}, invalid(HeaderLocation, "location.path")
	}
	if hasReplace && replace != "" && replace != "false" && (!hasPush || push != "false") {
		return ResponseOpt{}, fault(ErrConflict, HeaderLocation, "location.history")
	}
	switch {
	case !hasPush && !hasReplace:
	case hasPush && !hasReplace && push == "true":
	case hasPush && !hasReplace && push != "false" && push != "":
		opts = append(opts, LocationPushURL(push))
	case hasPush && hasReplace && push == "false" && replace == "false":
		opts = append(opts, LocationSuppressHistory())
	case hasPush && hasReplace && push == "false" && replace == "true":
		opts = append(opts, LocationReplaceDestination())
	case hasPush && hasReplace && push == "false" && replace != "":
		opts = append(opts, LocationReplaceURL(replace))
	default:
		return ResponseOpt{}, invalid(HeaderLocation, "location.history")
	}
	return Location(path, opts...), nil
}

func parseOOB(value string) ([]OOBSelection, error) {
	if value == "" {
		return nil, nil
	}
	var out []OOBSelection
	for i, entry := range strings.Split(value, ",") {
		if strings.Count(entry, ":") > 1 {
			return nil, invalid(HeaderLocation, indexed("location.selectOOB", i, "strategy"))
		}
		id, style, hasStyle := strings.Cut(entry, ":")
		id = strings.TrimPrefix(strings.TrimSpace(id), "#")
		if !hasStyle || style == "true" {
			style = string(SwapOuterHTML)
		}
		o := OOB(id, SwapStrategy(style))
		if o.err != nil {
			return nil, fault(ErrInvalidConfig, HeaderLocation, indexed("location.selectOOB", i, o.err.(*Error).Field))
		}
		out = append(out, o)
	}
	return out, nil
}
