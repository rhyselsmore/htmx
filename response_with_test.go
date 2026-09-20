package htmx

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func responseHeaders(t *testing.T, r *Response) http.Header {
	t.Helper()
	w := newSpy()
	if err := r.Apply(w); err != nil {
		t.Fatal(err)
	}
	if len(w.statuses) != 0 || len(w.body) != 0 {
		t.Fatal("Apply wrote status or body")
	}
	return w.h.Clone()
}

func requireWithError(t *testing.T, r *Response, err, kind error, field string) {
	t.Helper()
	var detail *Error
	if r != nil || !errors.Is(err, kind) || !errors.As(err, &detail) || detail.Op != "With" || detail.Field != field {
		t.Fatalf("response=%v error=%v; want %v field %s", r, err, kind, field)
	}
}

func TestWithEmptyAndNil(t *testing.T) {
	var nilBase *Response
	r, err := nilBase.With()
	requireWithError(t, r, err, ErrInvalidConfig, "response")
	r, err = nilBase.With(Trigger("bad", Detail(make(chan int))))
	requireWithError(t, r, err, ErrInvalidConfig, "response")
	for _, base := range []*Response{new(Response), MustResponse(Retarget("#items"), Trigger("ready", Detail(1)))} {
		before := responseHeaders(t, base)
		for _, opts := range [][]ResponseOpt{nil, {}, {ResponseOpt{}, Retarget(""), Trigger(""), Location(""), Reswap("")}} {
			derived, err := base.With(opts...)
			if err != nil {
				t.Fatal(err)
			}
			if derived == base || !reflect.DeepEqual(responseHeaders(t, derived), before) {
				t.Fatal("no-op derivation differs")
			}
		}
	}
	zero := new(Response)
	r, err = zero.With(Trigger("ready"))
	if err != nil || responseHeaders(t, r).Get(HeaderTrigger) != "ready" || len(responseHeaders(t, zero)) != 0 {
		t.Fatal(err)
	}
}

func TestWithRules(t *testing.T) {
	cases := []struct {
		name      string
		base, add []ResponseOpt
		kind      error
	}{
		{"equal singleton", []ResponseOpt{Retarget("#items")}, []ResponseOpt{Retarget("#items")}, nil},
		{"different singleton", []ResponseOpt{Retarget("#items")}, []ResponseOpt{Retarget("#other")}, ErrConflict},
		{"navigation swap", []ResponseOpt{Redirect("/next")}, []ResponseOpt{Reswap(SwapInnerHTML)}, ErrConflict},
		{"swap navigation", []ResponseOpt{Reswap(SwapInnerHTML)}, []ResponseOpt{Redirect("/next")}, ErrConflict},
		{"navigation later event", []ResponseOpt{Location("/next")}, []ResponseOpt{TriggerAfterSwap("done")}, ErrConflict},
		{"navigation immediate event", []ResponseOpt{Location("/next")}, []ResponseOpt{Trigger("done")}, nil},
		{"history", []ResponseOpt{PushURL("/a")}, []ResponseOpt{ReplaceURL("/a")}, ErrConflict},
		{"selection", []ResponseOpt{Reswap(SwapTextContent)}, []ResponseOpt{Reselect("#part")}, ErrConflict},
		{"equal location", []ResponseOpt{Location("/next", LocationTarget("#main"))}, []ResponseOpt{Location("/next", LocationTarget("#main"))}, nil},
		{"location is not patched", []ResponseOpt{Location("/next")}, []ResponseOpt{Location("/next", LocationTarget("#main"))}, ErrConflict},
		{"empty name with detail", nil, []ResponseOpt{Trigger("", Detail(1))}, ErrInvalidConfig},
		{"bad URL", nil, []ResponseOpt{Redirect("/bad\n")}, ErrInvalidConfig},
		{"multiple details", nil, []ResponseOpt{Trigger("ready", Detail(1), Detail(2))}, ErrConflict},
		{"bad encoding", nil, []ResponseOpt{Trigger("ready", Detail(make(chan int)))}, ErrEncode},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			base := MustResponse(tc.base...)
			before := responseHeaders(t, base)
			r, err := base.With(tc.add...)
			if tc.kind != nil {
				if r != nil || !errors.Is(err, tc.kind) {
					t.Fatalf("%v %v", r, err)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				opts := append(append([]ResponseOpt(nil), tc.base...), tc.add...)
				if !reflect.DeepEqual(responseHeaders(t, r), responseHeaders(t, MustResponse(opts...))) {
					t.Fatal("fresh and derived differ")
				}
			}
			if !reflect.DeepEqual(before, responseHeaders(t, base)) {
				t.Fatal("base changed")
			}
		})
	}
}

func TestWithEventReplacementAndIsolation(t *testing.T) {
	phases := []struct {
		name, header string
		make         func(string, ...TriggerOpt) ResponseOpt
	}{
		{"immediate", HeaderTrigger, Trigger}, {"swap", HeaderTriggerAfterSwap, TriggerAfterSwap}, {"settle", HeaderTriggerAfterSettle, TriggerAfterSettle},
	}
	for _, phase := range phases {
		t.Run(phase.name, func(t *testing.T) {
			base := MustResponse(Trigger("other", Detail("a")), TriggerAfterSwap("other", Detail("b")), TriggerAfterSettle("other", Detail("c")), phase.make("saved", Detail(map[string]int{"id": 1}), TriggerTarget("#old")))
			before := responseHeaders(t, base)
			a, err := base.With(phase.make("saved"))
			if err != nil {
				t.Fatal(err)
			}
			want := strings.Replace(before.Get(phase.header), `{"id":1,"target":"#old"}`, `null`, 1)
			if responseHeaders(t, a).Get(phase.header) != want {
				t.Fatalf("plain replacement: %v", responseHeaders(t, a))
			}
			aBefore := responseHeaders(t, a)
			sibling, err := base.With(phase.make("saved", Detail(2), TriggerTarget("#new")))
			if err != nil {
				t.Fatal(err)
			}
			child, err := a.With(phase.make("child", Detail("next")))
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(responseHeaders(t, child).Get(phase.header), `"child":"next"`) {
				t.Fatal("chained event missing")
			}
			if !strings.Contains(responseHeaders(t, sibling).Get(phase.header), `"saved":{"target":"#new","value":2}`) {
				t.Fatal("replacement payload or target missing")
			}
			if !reflect.DeepEqual(responseHeaders(t, base), before) || !reflect.DeepEqual(responseHeaders(t, a), aBefore) {
				t.Fatal("parent or sibling changed")
			}
			for _, other := range phases {
				if other.header != phase.header && responseHeaders(t, sibling).Get(other.header) != before.Get(other.header) {
					t.Fatal("other phase changed")
				}
			}
		})
	}
	plain := MustResponse(Trigger("ready"))
	rich, err := plain.With(Trigger("ready", Detail(1)))
	if err != nil {
		t.Fatal(err)
	}
	plainAgain, err := rich.With(Trigger("ready"))
	if err != nil {
		t.Fatal(err)
	}
	if responseHeaders(t, plain).Get(HeaderTrigger) != "ready" || responseHeaders(t, plainAgain).Get(HeaderTrigger) != `{"ready":null}` {
		t.Fatal("JSON mode lost")
	}
}

func TestWithFailureAndErrorPrecedence(t *testing.T) {
	base := MustResponse(Retarget("#items"), Trigger("base", Detail(1)))
	before := responseHeaders(t, base)
	r, err := base.With(Trigger("early", Detail("must not leak")), Retarget("#other"))
	requireWithError(t, r, err, ErrConflict, "retarget")
	if !reflect.DeepEqual(responseHeaders(t, base), before) {
		t.Fatal("failed draft leaked")
	}
	r, err = base.With(Retarget("#other"), Trigger("bad", Detail(make(chan int))))
	requireWithError(t, r, err, ErrEncode, "trigger.detail")
	var detail *Error
	if !errors.As(err, &detail) || detail.Header != HeaderTrigger || detail.Event != "bad" {
		t.Fatal(err)
	}
	r, err = base.With(Retarget("#other"), Redirect("/a"), Refresh())
	requireWithError(t, r, err, ErrConflict, "navigation")
	// The base must participate before choosing between conflicts in additions.
	r, err = MustResponse(Redirect("/a")).With(Refresh(), PushURL("/a"), ReplaceURL("/b"))
	requireWithError(t, r, err, ErrConflict, "navigation")
}

type countedDetail struct {
	calls *int
	err   error
}

func (m countedDetail) MarshalJSON() ([]byte, error) {
	*m.calls++
	if m.err != nil {
		return nil, m.err
	}
	return []byte(`{"id":42}`), nil
}

func TestWithSnapshotsAndMarshalErrors(t *testing.T) {
	calls := 0
	opt := Detail(countedDetail{calls: &calls})
	base := MustResponse(Trigger("saved", opt))
	for i := 0; i < 3; i++ {
		r, err := base.With(Trigger("again", opt))
		if err != nil {
			t.Fatal(err)
		}
		_ = responseHeaders(t, r)
	}
	if calls != 1 {
		t.Fatal("marshaler reran", calls)
	}
	cause := errors.New("private marshaler text")
	bad := Detail(countedDetail{calls: &calls, err: cause})
	r, err := base.With(Trigger("failed", bad))
	requireWithError(t, r, err, ErrEncode, "trigger.detail")
	if !errors.Is(err, cause) || strings.Contains(err.Error(), cause.Error()) || calls != 2 {
		t.Fatal(err, calls)
	}
	values := url.Values{"tag": {"one", "two"}}
	headers := map[string]string{"X-App": "one"}
	loc := Location("/items", LocationValues(values), LocationHeaders(headers), LocationSelectOOB(OOB("alerts", SwapOuterHTML)))
	parent := MustResponse(loc)
	before := responseHeaders(t, parent)
	values["tag"][0] = "changed"
	headers["X-App"] = "changed"
	payload := map[string]int{"id": 1}
	triggerOpts := []TriggerOpt{Detail(payload)}
	event := Trigger("loaded", triggerOpts...)
	payload["id"] = 2
	triggerOpts[0] = Detail(3)
	child, err := parent.With(loc, event)
	if err != nil {
		t.Fatal(err)
	}
	got := responseHeaders(t, child)
	if got.Get(HeaderLocation) != before.Get(HeaderLocation) || got.Get(HeaderTrigger) != `{"loaded":{"id":1}}` {
		t.Fatal(got)
	}
	if !reflect.DeepEqual(responseHeaders(t, parent), before) {
		t.Fatal("parent changed")
	}
}

func TestWithApplyAtomicity(t *testing.T) {
	r, err := MustResponse(Retarget("#items")).With(Trigger("ready"))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		h    http.Header
		kind error
	}{
		{http.Header{"HX-Redirect": {"/a"}}, ErrConflict},
		{http.Header{"HX-Trigger": {"{bad"}}, ErrInvalidHeader},
		{http.Header{"Hx-Retarget": {"#other"}}, ErrConflict},
	} {
		w := newSpy()
		w.h = tc.h
		w.h["Unrelated"] = []string{"a", "b"}
		assertUnchanged(t, w, w.h.Clone(), r.Apply(w), tc.kind)
	}
	w := newSpy()
	w.h.Set("X-App", "keep")
	w.h.Set(HeaderTrigger, "earlier")
	if err := r.Apply(w); err != nil {
		t.Fatal(err)
	}
	if w.h.Get("X-App") != "keep" || w.h.Get(HeaderTrigger) != "earlier, ready" || len(w.statuses) > 0 || len(w.body) > 0 {
		t.Fatal(w)
	}
}

func TestWithConcurrentDerivation(t *testing.T) {
	base := MustResponse(Trigger("saved", Detail("base"), TriggerTarget("#base")), Location("/items", LocationValues(url.Values{"tag": {"one"}})))
	before := responseHeaders(t, base)
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := fmt.Sprintf("request-%d", i)
			r, err := base.With(Trigger("saved", Detail(id), TriggerTarget("#"+id)))
			if err != nil {
				t.Error(err)
				return
			}
			h := responseHeaders(t, r)
			want := fmt.Sprintf(`{"saved":{"target":"#%s","value":"%s"}}`, id, id)
			if h.Get(HeaderTrigger) != want || h.Get(HeaderLocation) != before.Get(HeaderLocation) {
				t.Error("cross-request state", h)
			}
			_ = responseHeaders(t, base)
		}(i)
	}
	wg.Wait()
	if !reflect.DeepEqual(responseHeaders(t, base), before) {
		t.Fatal("base mutated")
	}
}
