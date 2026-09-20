package htmx

import (
	"bytes"
	"encoding/json"
	"io"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// TriggerOpt supplies one event's payload or target. The zero value does nothing.
type TriggerOpt struct{ apply func(*triggerDraft) error }

type triggerDraft struct {
	detail        json.RawMessage
	detailPresent bool
	target        string
}

type eventSet struct {
	events map[string]triggerDraft
	json   bool
}

type triggerPhase uint8

const (
	phaseImmediate triggerPhase = iota
	phaseAfterSwap
	phaseAfterSettle
)

var triggerHeaders = [3]string{HeaderTrigger, HeaderTriggerAfterSwap, HeaderTriggerAfterSettle}

// Detail snapshots value as JSON when the option is created. Named structs,
// maps, slices, pointers and raw JSON all use the captured encoding: later input
// changes do not change the response. Encoding errors are reported when consumed.
// Custom marshalers run once per option creation; their programming panics are
// not recovered. Do not mutate input concurrently while creating the option.
//
// Use Detail[any](nil) for explicit JSON null; Detail(nil) cannot infer its type.
// Only one Detail is allowed per event. Object payload fields appear directly
// on event.detail; scalar, array and null payloads use event.detail.value.
//
// An object's top-level target must use [TriggerTarget]. This is a package API
// rule, although htmx supports target in JSON. The client owns elt, and error has
// htmx error-event semantics; put domain fields with those names in a value object.
//
// JSON headers escape Unicode and DEL for XHR transport. Large numeric tokens
// survive server-side accumulation, but JavaScript has its usual precision limits.
// Use strings for identifiers beyond its safe integer range.
func Detail[T any](value T) TriggerOpt {
	raw, err := json.Marshal(value)
	if err == nil && !utf8.Valid(raw) {
		err = invalid(HeaderTrigger, "trigger.detail")
	}
	return TriggerOpt{func(d *triggerDraft) error {
		if d.detailPresent {
			return fault(ErrConflict, "", "trigger.detail")
		}
		if err != nil {
			return &Error{Kind: ErrEncode, Field: "trigger.detail", Cause: err}
		}
		if len(raw) > 0 && raw[0] == '{' {
			fields, e := rawObject(raw)
			if e != nil {
				return &Error{Kind: ErrInvalidConfig, Field: "trigger.detail", Cause: e}
			}
			if _, ok := fields["target"]; ok {
				return invalid("", "trigger.target")
			}
		}
		d.detailPresent = true
		// Marshal already owns these bytes. Drafts and responses only read them,
		// so reusing the snapshot avoids a payload-sized copy per construction.
		d.detail = raw
		return nil
	}}
}

// TriggerTarget routes an event to a CSS selector. Empty means no target option.
// Selectors may contain Unicode; they are checked for transport, not CSS syntax
// or target existence. Identical repeated targets are accepted; different targets
// for one event conflict. Use a new [Trigger] with the same name to replace the
// whole event, including its previous target.
func TriggerTarget(selector string) TriggerOpt {
	return TriggerOpt{func(d *triggerDraft) error {
		if selector == "" {
			return nil
		}
		if !validSelector(selector, false) {
			return invalid("", "trigger.target")
		}
		if d.target != "" && d.target != selector {
			return fault(ErrConflict, "", "trigger.target")
		}
		d.target = selector
		return nil
	}}
}

// Trigger fires a named event when the response is handled, before navigation
// or swapping. Names must match [A-Za-z_][A-Za-z0-9_.:-]*; hasOwnProperty is
// rejected because it breaks the pinned client's dispatcher. This is a supported
// subset of DOM event names. An empty name is a no-op only without effective
// payload or target options.
//
// Repeating a name within the same phase replaces its complete payload and target,
// including replacement with a plain event. Names in other phases are independent.
// A phase that has used JSON retains that encoding when an event becomes plain.
// Mixed targeted and ordinary events are supported; the encoder places ordinary
// events first to avoid target leakage in htmx 2.0.10. This is not an event
// sequencing API. See [Detail] and [TriggerTarget] for payload and routing rules.
func Trigger(name string, opts ...TriggerOpt) ResponseOpt {
	return triggerOption(phaseImmediate, name, opts)
}

// TriggerAfterSwap fires after swapping, when the client's response handling
// processes a swap. It cannot accompany navigation options. [SwapNone] can still
// do OOB work and reach this phase. Name, payload and replacement rules are the
// same as [Trigger], independently of the other phases.
func TriggerAfterSwap(name string, opts ...TriggerOpt) ResponseOpt {
	return triggerOption(phaseAfterSwap, name, opts)
}

// TriggerAfterSettle fires after settling, when the client processes a swap.
// It cannot accompany navigation options. Name, payload and replacement rules
// are the same as [Trigger], independently of the other phases.
func TriggerAfterSettle(name string, opts ...TriggerOpt) ResponseOpt {
	return triggerOption(phaseAfterSettle, name, opts)
}

func triggerOption(phase triggerPhase, name string, opts []TriggerOpt) ResponseOpt {
	opts = append([]TriggerOpt(nil), opts...)
	return ResponseOpt{func(s *responseState) error {
		var d triggerDraft
		for _, opt := range opts {
			if opt.apply != nil {
				if err := opt.apply(&d); err != nil {
					return contextError(err, "", triggerHeaders[phase], name)
				}
			}
		}
		if name == "" && !d.detailPresent && d.target == "" {
			return nil
		}
		if !validEvent(name) {
			return &Error{Kind: ErrInvalidConfig, Header: triggerHeaders[phase], Event: name, Field: "trigger.name"}
		}
		p := &s.triggers[phase]
		if p.events == nil {
			p.events = make(map[string]triggerDraft)
		}
		p.events[name] = d
		p.json = p.json || d.detailPresent || d.target != ""
		return nil
	}}
}

func validEvent(s string) bool {
	if s == "" || s == "hasOwnProperty" {
		return false
	}
	for i, c := range []byte(s) {
		if c == '_' || c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' {
			continue
		}
		if i > 0 && (c >= '0' && c <= '9' || c == '.' || c == ':' || c == '-') {
			continue
		}
		return false
	}
	return true
}

func (p *eventSet) merge(n eventSet) {
	if len(n.events) == 0 {
		return
	}
	if p.events == nil {
		p.events = make(map[string]triggerDraft, len(n.events))
	}
	for name, d := range n.events {
		p.events[name] = d
	}
	p.json = p.json || n.json
}

func parseTriggerHeader(value string) (eventSet, error) {
	out := eventSet{events: make(map[string]triggerDraft)}
	if !utf8.ValidString(value) {
		return out, invalid("", "trigger")
	}
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "{") {
		fields, err := rawObject([]byte(value))
		if err != nil {
			return out, err
		}
		out.json = true
		names := sortedKeys(fields)
		for _, name := range names {
			if !validEvent(name) {
				return out, &Error{Kind: ErrInvalidHeader, Event: name, Field: "trigger.name"}
			}
			d := triggerDraft{detail: bytes.Clone(fields[name]), detailPresent: true}
			if len(d.detail) > 0 && d.detail[0] == '{' {
				members, err := rawObject(d.detail)
				if err != nil {
					return out, err
				}
				if raw, ok := members["target"]; ok {
					if len(raw) == 0 || raw[0] != '"' || json.Unmarshal(raw, &d.target) != nil || !validSelector(d.target, false) {
						return out, invalid("", "trigger.target")
					}
					delete(members, "target")
					d.detail, _ = json.Marshal(members)
				}
			}
			out.events[name] = d
		}
	} else {
		for _, name := range strings.Split(value, ",") {
			name = strings.TrimSpace(name)
			if !validEvent(name) {
				return out, &Error{Kind: ErrInvalidHeader, Event: name, Field: "trigger.name"}
			}
			out.events[name] = triggerDraft{}
		}
	}
	return out, nil
}

func encodeTriggerHeader(p eventSet) (string, error) {
	names := sortedKeys(p.events)
	if !p.json {
		return strings.Join(names, ", "), nil
	}
	// The client retains the last target; untargeted events must come first.
	ordered := make([]string, 0, len(names))
	for _, name := range names {
		if p.events[name].target == "" {
			ordered = append(ordered, name)
		}
	}
	for _, name := range names {
		if p.events[name].target != "" {
			ordered = append(ordered, name)
		}
	}
	var out bytes.Buffer
	out.WriteByte('{')
	for i, name := range ordered {
		if i > 0 {
			out.WriteByte(',')
		}
		key, _ := json.Marshal(name)
		out.Write(key)
		out.WriteByte(':')
		d := p.events[name]
		raw := d.detail
		if !d.detailPresent {
			raw = json.RawMessage("null")
		}
		if d.target != "" {
			var fields map[string]json.RawMessage
			if len(raw) > 0 && raw[0] == '{' {
				var err error
				fields, err = rawObject(raw)
				if err != nil {
					return "", err
				}
			} else {
				fields = map[string]json.RawMessage{"value": raw}
			}
			fields["target"], _ = json.Marshal(d.target)
			var err error
			raw, err = json.Marshal(fields)
			if err != nil {
				return "", err
			}
		}
		out.Write(raw)
	}
	out.WriteByte('}')
	return asciiJSON(out.Bytes()), nil
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// rawObject checks exact keys and duplicates without floating-point conversion.
func rawObject(raw []byte) (map[string]json.RawMessage, error) {
	if !utf8.Valid(raw) {
		return nil, invalid("", "json")
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	if tok != json.Delim('{') {
		return nil, invalid("", "json.object")
	}
	out := make(map[string]json.RawMessage)
	for dec.More() {
		tok, err = dec.Token()
		if err != nil {
			return nil, err
		}
		key, ok := tok.(string)
		if !ok {
			return nil, invalid("", "json.key")
		}
		if _, ok := out[key]; ok {
			return nil, invalid("", "json.duplicate")
		}
		var val json.RawMessage
		if err = dec.Decode(&val); err != nil {
			return nil, err
		}
		out[key] = val
	}
	if _, err = dec.Token(); err != nil {
		return nil, err
	}
	if _, err = dec.Token(); err != io.EOF {
		return nil, invalid("", "json.trailing")
	}
	return out, nil
}

func asciiJSON(raw []byte) string {
	var out strings.Builder
	out.Grow(len(raw))
	write := func(r rune) {
		out.WriteString(`\u`)
		s := strconv.FormatInt(int64(r), 16)
		for i := len(s); i < 4; i++ {
			out.WriteByte('0')
		}
		out.WriteString(s)
	}
	for _, r := range string(raw) {
		if r < 127 {
			out.WriteByte(byte(r))
		} else if r <= 0xffff {
			write(r)
		} else {
			a, b := utf16.EncodeRune(r)
			write(a)
			write(b)
		}
	}
	return out.String()
}
