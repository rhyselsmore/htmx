package htmx

import (
	"errors"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"testing"
)

type spyWriter struct {
	h        http.Header
	statuses []int
	body     []byte
}

func newSpy() *spyWriter { return &spyWriter{h: make(http.Header)} }

func (w *spyWriter) Header() http.Header { return w.h }

func (w *spyWriter) WriteHeader(n int) { w.statuses = append(w.statuses, n) }

func (w *spyWriter) Write(p []byte) (int, error) {
	w.body = append(w.body, p...)
	return len(p), nil
}

func assertUnchanged(t *testing.T, w *spyWriter, before http.Header, err error, kind error) {
	t.Helper()
	if !errors.Is(err, kind) || !reflect.DeepEqual(w.h, before) || len(w.statuses) != 0 || len(w.body) != 0 {
		t.Fatalf("error=%v headers=%v status=%v body=%q", err, w.h, w.statuses, w.body)
	}
}

func TestCoreOptions(t *testing.T) {
	cases := []struct {
		opt           ResponseOpt
		header, value string
	}{
		{Redirect("/next"), HeaderRedirect, "/next"},
		{Refresh(), HeaderRefresh, "true"},
		{Location("/next"), HeaderLocation, "/next"},
		{PushURL("/next"), HeaderPushURL, "/next"},
		{ReplaceURL("/next"), HeaderReplaceURL, "/next"},
		{SuppressHistory(), HeaderPushURL, "false"},
		{Retarget("#main"), HeaderRetarget, "#main"},
		{Reselect("#part"), HeaderReselect, "#part"},
		{Reswap(SwapNone), HeaderReswap, "none"},
	}
	for _, tc := range cases {
		t.Run(tc.header+tc.value, func(t *testing.T) {
			w := newSpy()
			if err := Respond(w, tc.opt, tc.opt); err != nil {
				t.Fatal(err)
			}
			if w.h.Get(tc.header) != tc.value || len(w.statuses) != 0 || len(w.body) != 0 {
				t.Fatal(w)
			}
			before := w.h.Clone()
			if err := Respond(w, tc.opt); err != nil || !reflect.DeepEqual(before, w.h) {
				t.Fatalf("reapply %v %v", err, w.h)
			}
		})
	}
}

func TestCoreConflicts(t *testing.T) {
	pairs := [][2]ResponseOpt{
		{Redirect("/a"), Redirect("/b")},
		{Redirect("/a"), Refresh()},
		{Location("/a"), Redirect("/a")},
		{PushURL("/a"), ReplaceURL("/a")},
		{PushURL("/a"), SuppressHistory()},
		{Retarget("#a"), Retarget("#b")},
		{Reselect("#a"), Reselect("#b")},
		{Reswap(SwapNone), Reswap(SwapDelete)},
		{Reswap(SwapTextContent), Reselect("#a")},
	}
	for _, nav := range []ResponseOpt{Redirect("/a"), Refresh(), Location("/a")} {
		for _, control := range []ResponseOpt{PushURL("/a"), ReplaceURL("/a"), SuppressHistory(), Retarget("#a"), Reselect("#a"), Reswap(SwapNone)} {
			pairs = append(pairs, [2]ResponseOpt{nav, control})
		}
	}
	for i, pair := range pairs {
		for reverse := 0; reverse < 2; reverse++ {
			a, b := pair[reverse], pair[1-reverse]
			w := newSpy()
			w.h["Untouched"] = []string{"a", "b"}
			before := w.h.Clone()
			assertUnchanged(t, w, before, Respond(w, a, b), ErrConflict)
			if err := Respond(w, a); err != nil {
				t.Fatalf("pair %d: %v", i, err)
			}
			before = w.h.Clone()
			assertUnchanged(t, w, before, Respond(w, b), ErrConflict)
		}
	}
}

func TestCoreInvalid(t *testing.T) {
	for _, opt := range []ResponseOpt{
		Redirect("javascript:alert(1)"),
		Redirect("https://u:p@example.test"),
		Redirect("/bad\n"),
		Redirect("/café"),
		Redirect("/a\\b"),
		Redirect(" /a"),
		PushURL("true"),
		ReplaceURL("false"),
		Reswap("outerHTML swap:1s"),
		Retarget("\t"),
		Retarget("#café"),
		Reselect("#a\r\n"),
	} {
		w := newSpy()
		w.h.Set("Keep", "v")
		assertUnchanged(t, w, w.h.Clone(), Respond(w, opt), ErrInvalidConfig)
	}
}

func TestEmptyResponsePreservesMalformedHeaders(t *testing.T) {
	w := newSpy()
	w.h["hx-trigger"] = []string{"{invalid"}
	before := w.h.Clone()
	if err := Respond(w, ResponseOpt{}, Redirect(""), Location(""), PushURL(""), ReplaceURL(""), Reswap(""), Retarget(""), Reselect("")); err != nil || !reflect.DeepEqual(w.h, before) {
		t.Fatal(err, w.h)
	}
	if err := new(Response).Apply(w); err != nil {
		t.Fatal(err)
	}
	var nilResponse *Response
	assertUnchanged(t, w, before, nilResponse.Apply(w), ErrInvalidConfig)
}

func TestManagedHeaderImports(t *testing.T) {
	for _, h := range []http.Header{
		{"HX-Redirect": {"/a", "/a"}},
		{"HX-Redirect": {"/a"}, "hx-redirect": {"/a"}},
		{"HX-Refresh": {"false"}},
		{"HX-Retarget": {""}},
		{"HX-Push": {"/legacy"}},
		{"HX-Redirect": {"/café"}},
	} {
		w := newSpy()
		w.h = h
		assertUnchanged(t, w, w.h.Clone(), Respond(w, Retarget("#main")), ErrInvalidHeader)
	}
	w := newSpy()
	w.h["HX-Replace-Url"] = []string{"false"}
	w.h["X-Unmanaged"] = []string{"keep", "all"}
	old := w.h["X-Unmanaged"]
	if err := Respond(w, Retarget("#main")); err != nil {
		t.Fatal(err)
	}
	if w.h.Get(HeaderPushURL) != "false" || len(w.h.Values(HeaderReplaceURL)) != 0 || &w.h["X-Unmanaged"][0] != &old[0] {
		t.Fatal(w.h)
	}
	canonical := w.h[http.CanonicalHeaderKey(HeaderPushURL)]
	if err := Respond(w, SuppressHistory()); err != nil {
		t.Fatal(err)
	}
	if &canonical[0] != &w.h[http.CanonicalHeaderKey(HeaderPushURL)][0] {
		t.Fatal("unchanged slice replaced")
	}
	w = newSpy()
	w.h.Set(HeaderPushURL, "false")
	w.h.Set(HeaderReplaceURL, "false")
	assertUnchanged(t, w, w.h.Clone(), Respond(w, Retarget("#main")), ErrConflict)
	w = newSpy()
	w.h["HX-Redirect"] = nil
	if err := Respond(w, Retarget("#main")); err != nil {
		t.Fatal(err)
	}
}

func TestStopPollingAtomic(t *testing.T) {
	w := newSpy()
	if err := StopPolling(w, Retarget("#main")); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(w.statuses, []int{286}) || len(w.body) != 0 {
		t.Fatal(w)
	}
	for _, nav := range []ResponseOpt{Redirect("/a"), Location("/a"), Refresh()} {
		w = newSpy()
		assertUnchanged(t, w, w.h.Clone(), StopPolling(w, nav), ErrConflict)
		if err := Respond(w, nav); err != nil {
			t.Fatal(err)
		}
		assertUnchanged(t, w, w.h.Clone(), StopPolling(w), ErrConflict)
	}
	w = newSpy()
	w.h.Set(HeaderTrigger, "{bad")
	assertUnchanged(t, w, w.h.Clone(), StopPolling(w), ErrInvalidHeader)
}

func TestImmutableCoreReuse(t *testing.T) {
	r := MustResponse(Retarget("#main"), Reswap(SwapOuterHTML))
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := newSpy()
			if err := r.Apply(w); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	defer func() {
		if recover() == nil {
			t.Error("MustResponse did not panic")
		}
	}()
	MustResponse(Redirect("bad:scheme"))
}

func TestSafeError(t *testing.T) {
	cause := errors.New("secret payload")
	e := &Error{Op: "Respond", Header: HeaderTrigger, Event: strings.Repeat("x", 200) + "\nsecret", Kind: ErrEncode, Cause: cause}
	if strings.Contains(e.Error(), "secret") || !errors.Is(e, ErrEncode) || !errors.Is(e, cause) {
		t.Fatal(e)
	}
}

func TestPollingEncodingErrorPreservesPriorInstructions(t *testing.T) {
	w := newSpy()
	w.h.Set(HeaderTrigger, `{"old":1}`)
	assertUnchanged(t, w, w.h.Clone(), StopPolling(w, Trigger("bad", Detail(make(chan int)))), ErrEncode)
}

func TestValidationErrorPrecedence(t *testing.T) {
	_, err := NewResponse(Redirect("/a"), Redirect("/b"), Trigger("bad", Detail(make(chan int))))
	if !errors.Is(err, ErrEncode) {
		t.Fatalf("local encoding failure must precede cross-option conflict: %v", err)
	}
	_, err = NewResponse(Retarget("#a"), Retarget("#b"), Redirect("/a"), Refresh())
	var detail *Error
	if !errors.As(err, &detail) || detail.Field != "navigation" {
		t.Fatalf("navigation conflicts must precede swap conflicts: %v", err)
	}
}
