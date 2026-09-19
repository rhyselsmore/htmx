package htmx

import (
	"errors"
	"testing"
	"time"
)

func TestSwapModifiers(t *testing.T) {
	opts := []SwapOpt{
		FocusScroll(false),
		Show(ScrollNone, ""),
		Scroll(ScrollBottom, "#a:hover"),
		IgnoreTitle(true),
		SettleDelay(0),
		SwapDelay(1500 * time.Millisecond),
		Transition(false),
	}
	w := newSpy()
	if err := Respond(w, Reswap(SwapOuterHTML, opts...)); err != nil {
		t.Fatal(err)
	}
	want := "outerHTML transition:false swap:1500ms settle:0ms ignoreTitle:true scroll:#a:hover:bottom show:none focus-scroll:false"
	if w.h.Get(HeaderReswap) != want {
		t.Fatal(w.h)
	}
	if err := Respond(w, Reswap(SwapOuterHTML, opts...)); err != nil {
		t.Fatal(err)
	}
	w = newSpy()
	w.h.Set(HeaderReswap, "outerHTML swap:1.5s settle:0")
	if err := Respond(w, Reswap(SwapOuterHTML, SwapDelay(1500*time.Millisecond), SettleDelay(0))); err != nil {
		t.Fatal(err)
	}
	for _, strategy := range []SwapStrategy{
		SwapInnerHTML,
		SwapOuterHTML,
		SwapTextContent,
		SwapBeforeBegin,
		SwapAfterBegin,
		SwapBeforeEnd,
		SwapAfterEnd,
		SwapDelete,
		SwapNone,
	} {
		if _, err := NewResponse(Reswap(strategy)); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSwapInvalid(t *testing.T) {
	invalid := []SwapOpt{
		SwapDelay(-time.Millisecond),
		SwapDelay(time.Nanosecond),
		SettleDelay(2147483648 * time.Millisecond),
		Scroll(ScrollNone, ""),
		Show(ScrollNone, "#x"),
		Scroll("", ""),
		Show("invalid", ""),
		Scroll(ScrollTop, "#a #b"),
	}
	for _, opt := range invalid {
		if _, err := NewResponse(Reswap(SwapInnerHTML, opt)); !errors.Is(err, ErrInvalidConfig) {
			t.Fatal(err)
		}
	}
	if _, err := NewResponse(Reswap("", SwapDelay(0))); !errors.Is(err, ErrInvalidConfig) {
		t.Fatal(err)
	}
	if _, err := NewResponse(Reswap("", SwapOpt{})); err != nil {
		t.Fatal(err)
	}
	if _, err := NewResponse(Reswap(SwapNone, Scroll(ScrollTop, "#café"))); !errors.Is(err, ErrInvalidConfig) {
		t.Fatal(err)
	}
	if _, err := NewResponse(Location("/", LocationSwap(SwapNone, Scroll(ScrollTop, "#café")))); err != nil {
		t.Fatal(err)
	}
	for _, pair := range [][2]SwapOpt{
		{SwapDelay(0), SwapDelay(time.Second)},
		{Transition(true), Transition(false)},
		{IgnoreTitle(true), IgnoreTitle(false)},
		{FocusScroll(true), FocusScroll(false)},
		{Scroll(ScrollTop, ""), Scroll(ScrollBottom, "")},
		{Show(ScrollTop, ""), Show(ScrollNone, "")},
	} {
		for i := 0; i < 2; i++ {
			if _, err := NewResponse(Reswap(SwapNone, pair[i], pair[1-i])); !errors.Is(err, ErrConflict) {
				t.Fatal(err)
			}
		}
	}
	for _, raw := range []string{
		"outerHTML bogus:1",
		"outerHTML swap:1m30s",
		"outerHTML swap:0.5ms",
		"outerHTML swap:2147483648ms",
		"outerHTML transition:TRUE",
		"outerHTML show:#x:none",
		"outerHTML scroll:none",
		"outerHTML swap",
		"outerHTML scroll:#x:",
		"unknown",
	} {
		w := newSpy()
		w.h.Set(HeaderReswap, raw)
		assertUnchanged(t, w, w.h.Clone(), Respond(w, Trigger("x")), ErrInvalidHeader)
	}
	for _, opts := range [][]SwapOpt{
		{SwapDelay(0), SwapDelay(0)},
		{SettleDelay(2147483647 * time.Millisecond)},
		{Scroll(ScrollTop, "window")},
		{Show(ScrollBottom, "")},
	} {
		if _, err := NewResponse(Reswap(SwapNone, opts...)); err != nil {
			t.Fatal(err)
		}
	}
	opts := []SwapOpt{Transition(true)}
	response := Reswap(SwapNone, opts...)
	opts[0] = Transition(false)
	w := newSpy()
	if err := Respond(w, response); err != nil || w.h.Get(HeaderReswap) != "none transition:true" {
		t.Fatal(err, w.h)
	}
}

func TestImportedIntervalPrecision(t *testing.T) {
	for _, value := range []string{"0.000000000000001ms", "0.000000000000001s", "1.0000000000000001ms", "2147483647.000000000001ms"} {
		w := newSpy()
		w.h.Set(HeaderReswap, "none swap:"+value)
		assertUnchanged(t, w, w.h.Clone(), Respond(w, Trigger("x")), ErrInvalidHeader)
	}
	for _, tc := range []struct{ value, want string }{
		{"010", "10ms"},
		{"0010.0ms", "10ms"},
		{".00005m", "3ms"},
		{".5s", "500ms"},
		{"1.0000ms", "1ms"},
		{"2147483647ms", "2147483647ms"},
	} {
		w := newSpy()
		w.h.Set(HeaderReswap, "none swap:"+tc.value)
		if err := Respond(w, Trigger("x")); err != nil {
			t.Fatal(err)
		}
		if got := w.h.Get(HeaderReswap); got != "none swap:"+tc.want {
			t.Fatal(got)
		}
	}
}
