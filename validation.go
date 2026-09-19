package htmx

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

var (
	// ErrInvalidConfig reports an invalid option value or missing required setting.
	ErrInvalidConfig = errors.New("htmx: invalid configuration")
	// ErrConflict reports incompatible instructions or repeated payload assignments.
	ErrConflict = errors.New("htmx: conflicting instructions")
	// ErrInvalidHeader reports malformed or unsupported existing managed headers.
	ErrInvalidHeader = errors.New("htmx: invalid existing header")
	// ErrEncode reports a failure to marshal a trigger payload.
	ErrEncode = errors.New("htmx: encoding failed")
)

// Error identifies a failed operation without including header values or payloads.
// Cause is available through errors.Is/As; logging it may expose application data.
type Error struct {
	Op, Header, Event, Field string
	Kind, Cause              error
}

func (e *Error) Error() string {
	message := "htmx"
	for _, part := range []struct{ name, value string }{{"op", e.Op}, {"header", e.Header}, {"event", e.Event}, {"field", e.Field}} {
		if part.value != "" {
			v := part.value
			if len(v) > 96 {
				v = v[:96] + "…"
			}
			message += " " + part.name + "=" + strconv.QuoteToASCII(v)
		}
	}
	switch e.Kind {
	case ErrConflict:
		message += ": conflicting instructions"
	case ErrEncode:
		message += ": encoding failed"
	case ErrInvalidHeader:
		message += ": invalid existing header"
	default:
		message += ": invalid configuration"
	}
	return message
}

// Unwrap exposes both the error family and any underlying cause.
func (e *Error) Unwrap() []error {
	out := make([]error, 0, 2)
	if e.Kind != nil {
		out = append(out, e.Kind)
	}
	if e.Cause != nil {
		out = append(out, e.Cause)
	}
	return out
}

func fault(kind error, header, field string) error {
	return &Error{Kind: kind, Header: header, Field: field}
}

func contextError(err error, op, header, event string) error {
	if err == nil {
		return nil
	}
	var e *Error
	if errors.As(err, &e) {
		c := *e
		c.Op = op
		if header != "" {
			c.Header = header
		}
		if event != "" {
			c.Event = event
		}
		return &c
	}
	return &Error{Op: op, Header: header, Event: event, Kind: ErrInvalidConfig, Cause: err}
}

func textOK(s string, ascii bool) bool {
	if !utf8.ValidString(s) || strings.TrimSpace(s) != s {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) || (ascii && r > 127) {
			return false
		}
	}
	return true
}

func urlOK(s string, history bool) bool {
	if s == "" || !textOK(s, true) || strings.Contains(s, "\\") || history && (s == "true" || s == "false") {
		return false
	}
	for _, r := range s {
		if unicode.IsSpace(r) {
			return false
		}
	}
	u, err := url.Parse(s)
	if err != nil || u.User != nil {
		return false
	}
	if u.Scheme != "" && u.Scheme != "http" && u.Scheme != "https" {
		return false
	}
	return u.Scheme == "" || u.Host != ""
}

func validSelector(s string, ascii bool) bool { return s != "" && textOK(s, ascii) }

type setting[T comparable] struct {
	value   T
	present bool
}

func assign[T comparable](dst *setting[T], value T, header, field string) error {
	if dst.present && dst.value != value {
		return fault(ErrConflict, header, field)
	}
	*dst = setting[T]{value, true}
	return nil
}

func invalid(header, field string) error { return fault(ErrInvalidConfig, header, field) }

func indexed(field string, i int, member string) string {
	return fmt.Sprintf("%s[%d].%s", field, i, member)
}

func headerASCII(s string) bool {
	for _, c := range []byte(s) {
		if c < 32 || c > 126 {
			return false
		}
	}
	return true
}
