package htmx

import (
	"math/big"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// SwapOpt configures a main swap. Zero does nothing; OOB swaps do not take modifiers.
type SwapOpt struct{ apply func(*swapSpec) error }

// ScrollPosition is a viewport position used by Scroll and Show.
type ScrollPosition string

const (
	ScrollTop    ScrollPosition = "top"
	ScrollBottom ScrollPosition = "bottom"
	// ScrollNone disables Show; it is invalid for Scroll or with a selector.
	ScrollNone ScrollPosition = "none"
)

type scrollSpec struct {
	position ScrollPosition
	selector string
}

type swapSpec struct {
	strategy                             SwapStrategy
	delay, settle                        setting[int64]
	transition, ignoreTitle, focusScroll setting[bool]
	scroll, show                         setting[scrollSpec]
}

func buildSwap(strategy SwapStrategy, opts []SwapOpt) (swapSpec, error) {
	var s swapSpec
	for _, o := range opts {
		if o.apply != nil {
			if err := o.apply(&s); err != nil {
				return s, err
			}
		}
	}
	if strategy == "" && s == (swapSpec{}) {
		return s, nil
	}
	if !validStrategy(strategy) {
		return s, invalid(HeaderReswap, "swap.strategy")
	}
	s.strategy = strategy
	return s, nil
}

// Reswap overrides the current response's swap strategy and modifiers.
func Reswap(strategy SwapStrategy, opts ...SwapOpt) ResponseOpt {
	opts = append([]SwapOpt(nil), opts...)
	return ResponseOpt{func(s *responseState) error {
		v, err := buildSwap(strategy, opts)
		if err != nil {
			return err
		}
		if v.strategy == "" {
			return nil
		}
		if !textOK(v.wire(), true) {
			return invalid(HeaderReswap, "swap")
		}
		return assign(&s.swap, v, HeaderReswap, "swap")
	}}
}

// SwapDelay sets the delay before swapping, in exact milliseconds, up to 2147483647ms.
func SwapDelay(d time.Duration) SwapOpt { return durationOption(d, false) }

// SettleDelay sets the delay before settling, including an explicit zero.
func SettleDelay(d time.Duration) SwapOpt { return durationOption(d, true) }

func durationOption(d time.Duration, settle bool) SwapOpt {
	return SwapOpt{func(s *swapSpec) error {
		field := "swap.delay"
		dst := &s.delay
		if settle {
			field = "swap.settle"
			dst = &s.settle
		}
		if d < 0 || d%time.Millisecond != 0 || d/time.Millisecond > 2147483647 {
			return invalid(HeaderReswap, field)
		}
		return assign(dst, int64(d/time.Millisecond), HeaderReswap, field)
	}}
}

// Transition enables or disables a view transition for the main swap.
func Transition(enabled bool) SwapOpt { return boolSwap(enabled, "transition") }

// IgnoreTitle controls whether the client ignores a title in the response.
func IgnoreTitle(enabled bool) SwapOpt { return boolSwap(enabled, "ignoreTitle") }

// FocusScroll controls scrolling to the focused element after the swap.
func FocusScroll(enabled bool) SwapOpt { return boolSwap(enabled, "focus-scroll") }

func boolSwap(value bool, field string) SwapOpt {
	return SwapOpt{func(s *swapSpec) error {
		dst := &s.transition
		switch field {
		case "ignoreTitle":
			dst = &s.ignoreTitle
		case "focus-scroll":
			dst = &s.focusScroll
		}
		return assign(dst, value, HeaderReswap, "swap."+field)
	}}
}

// Scroll scrolls an element's contents. Empty selector means the swap target.
func Scroll(position ScrollPosition, selector string) SwapOpt {
	return scrollOption(position, selector, false)
}

// Show scrolls an element into view. Show(ScrollNone, "") disables automatic showing.
func Show(position ScrollPosition, selector string) SwapOpt {
	return scrollOption(position, selector, true)
}

func scrollOption(position ScrollPosition, selector string, show bool) SwapOpt {
	return SwapOpt{func(s *swapSpec) error {
		field := "swap.scroll"
		dst := &s.scroll
		if show {
			field = "swap.show"
			dst = &s.show
		}
		validPosition := position == ScrollTop || position == ScrollBottom
		disabledShow := show && position == ScrollNone && selector == ""
		if !validPosition && !disabledShow {
			return invalid(HeaderReswap, field)
		}
		if selector != "" && (!validSelector(selector, false) || strings.ContainsFunc(selector, unicode.IsSpace)) {
			return invalid(HeaderReswap, field)
		}
		return assign(dst, scrollSpec{position, selector}, HeaderReswap, field)
	}}
}

func validStrategy(s SwapStrategy) bool {
	switch s {
	case SwapInnerHTML, SwapOuterHTML, SwapTextContent, SwapBeforeBegin, SwapAfterBegin, SwapBeforeEnd, SwapAfterEnd, SwapDelete, SwapNone:
		return true
	}
	return false
}

func (s swapSpec) wire() string {
	var b strings.Builder
	b.WriteString(string(s.strategy))
	add := func(key, value string) {
		b.WriteByte(' ')
		b.WriteString(key)
		b.WriteByte(':')
		b.WriteString(value)
	}
	if s.transition.present {
		add("transition", strconv.FormatBool(s.transition.value))
	}
	if s.delay.present {
		add("swap", strconv.FormatInt(s.delay.value, 10)+"ms")
	}
	if s.settle.present {
		add("settle", strconv.FormatInt(s.settle.value, 10)+"ms")
	}
	if s.ignoreTitle.present {
		add("ignoreTitle", strconv.FormatBool(s.ignoreTitle.value))
	}
	for _, p := range []struct {
		key string
		v   setting[scrollSpec]
	}{{"scroll", s.scroll}, {"show", s.show}} {
		if p.v.present {
			v := string(p.v.value.position)
			if p.v.value.selector != "" {
				v = p.v.value.selector + ":" + v
			}
			add(p.key, v)
		}
	}
	if s.focusScroll.present {
		add("focus-scroll", strconv.FormatBool(s.focusScroll.value))
	}
	return b.String()
}

func parseSwap(value string) (swapSpec, error) {
	fields := strings.Fields(value)
	if len(fields) == 0 {
		return swapSpec{}, invalid(HeaderReswap, "swap")
	}
	var opts []SwapOpt
	for _, field := range fields[1:] {
		key, value, ok := strings.Cut(field, ":")
		if !ok {
			return swapSpec{}, invalid(HeaderReswap, "swap.modifier")
		}
		switch key {
		case "swap", "settle":
			d, err := parseInterval(value)
			if err != nil {
				return swapSpec{}, invalid(HeaderReswap, "swap."+key)
			}
			if key == "swap" {
				opts = append(opts, SwapDelay(d))
			} else {
				opts = append(opts, SettleDelay(d))
			}

		case "transition", "ignoreTitle", "focus-scroll":
			if value != "true" && value != "false" {
				return swapSpec{}, invalid(HeaderReswap, "swap."+key)
			}
			opts = append(opts, boolSwap(value == "true", key))
		case "scroll", "show":
			selector := ""
			pos := value
			if i := strings.LastIndexByte(value, ':'); i >= 0 {
				selector = value[:i]
				pos = value[i+1:]
			}
			opts = append(opts, scrollOption(ScrollPosition(pos), selector, key == "show"))
		default:
			return swapSpec{}, invalid(HeaderReswap, "swap.modifier")
		}
	}
	return buildSwap(SwapStrategy(fields[0]), opts)
}

// Convert decimal milliseconds exactly. time.ParseDuration can round sub-nanosecond
// fractions to zero, which would hide an invalid fractional-millisecond input.
func parseInterval(value string) (time.Duration, error) {
	factor := int64(1)
	switch {
	case strings.HasSuffix(value, "ms"):
		value = strings.TrimSuffix(value, "ms")
	case strings.HasSuffix(value, "s"):
		value = strings.TrimSuffix(value, "s")
		factor = 1000
	case strings.HasSuffix(value, "m"):
		value = strings.TrimSuffix(value, "m")
		factor = 60000
	}
	for _, c := range value {
		if (c < '0' || c > '9') && c != '.' && c != '+' && c != '-' {
			return 0, invalid(HeaderReswap, "swap.duration")
		}
	}
	n, ok := new(big.Rat).SetString(value)
	if !ok {
		return 0, invalid(HeaderReswap, "swap.duration")
	}
	n.Mul(n, new(big.Rat).SetInt64(factor))
	if !n.IsInt() || n.Sign() < 0 || !n.Num().IsInt64() || n.Num().Int64() > 2147483647 {
		return 0, invalid(HeaderReswap, "swap.duration")
	}
	return time.Duration(n.Num().Int64()) * time.Millisecond, nil
}
