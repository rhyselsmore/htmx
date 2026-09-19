package htmx

import (
	"encoding/json"
	"errors"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func locationWire(t *testing.T, opts ...LocationOpt) string {
	t.Helper()
	w := newSpy()
	if err := Respond(w, Location("/items", opts...)); err != nil {
		t.Fatal(err)
	}
	return w.h.Get(HeaderLocation)
}

func TestLocationHistory(t *testing.T) {
	for _, tc := range []struct {
		opt  LocationOpt
		want string
	}{
		{LocationOpt{}, `/items`},
		{LocationPushURL("/chosen"), `{"path":"/items","push":"/chosen"}`},
		{LocationReplaceURL("/chosen"), `{"path":"/items","push":"false","replace":"/chosen"}`},
		{LocationReplaceDestination(), `{"path":"/items","push":"false","replace":"true"}`},
		{LocationSuppressHistory(), `{"path":"/items","push":"false","replace":"false"}`},
	} {
		got := locationWire(t, tc.opt, tc.opt)
		if got != tc.want {
			t.Fatal(got)
		}
		w := newSpy()
		w.h.Set(HeaderLocation, got)
		if err := Respond(w, Location("/items", tc.opt)); err != nil {
			t.Fatal(err)
		}
		if w.h.Get(HeaderLocation) != got || w.h.Get(HeaderReplaceURL) != "" {
			t.Fatal(w.h)
		}
	}
	modes := []LocationOpt{LocationPushURL("/x"), LocationReplaceURL("/x"), LocationReplaceDestination(), LocationSuppressHistory()}
	for i, a := range modes {
		for j, b := range modes {
			if i == j {
				continue
			}
			w := newSpy()
			assertUnchanged(t, w, w.h.Clone(), Respond(w, Location("/items", a, b)), ErrConflict)
			if err := Respond(w, Location("/items", a)); err != nil {
				t.Fatal(err)
			}
			assertUnchanged(t, w, w.h.Clone(), Respond(w, Location("/items", b)), ErrConflict)
		}
	}
}

func TestLocationOOB(t *testing.T) {
	got := locationWire(t, LocationSelectOOB(OOB("z", SwapOuterHTML), OOB("a", SwapInnerHTML)), LocationSelectOOB(OOB("z", SwapOuterHTML)))
	if got != `{"path":"/items","selectOOB":"#z:outerHTML,#a:innerHTML"}` {
		t.Fatal(got)
	}
	for _, strategy := range []SwapStrategy{SwapInnerHTML, SwapOuterHTML, SwapBeforeBegin, SwapAfterBegin, SwapBeforeEnd, SwapAfterEnd, SwapDelete, SwapNone} {
		locationWire(t, LocationSelectOOB(OOB("x", strategy)))
	}
	for _, o := range []OOBSelection{
		OOB("", SwapOuterHTML),
		OOB("x", ""),
		OOB("#x", SwapOuterHTML),
		OOB("x.y", SwapOuterHTML),
		OOB("x:y", SwapOuterHTML),
		OOB("x,y", SwapOuterHTML),
		OOB("1x", SwapOuterHTML),
		OOB("café", SwapOuterHTML),
		OOB("x", SwapTextContent),
		OOB("x", "true"),
		OOB("x", "outerHTML swap:1s"),
	} {
		w := newSpy()
		assertUnchanged(t, w, w.h.Clone(), Respond(w, Location("/items", LocationSelectOOB(o))), ErrInvalidConfig)
	}
	for _, opts := range [][]LocationOpt{
		{LocationSelectOOB(OOB("x", SwapNone)), LocationSelectOOB(OOB("x", SwapDelete))},
		{LocationSelectOOB(OOB("x", SwapDelete)), LocationSelectOOB(OOB("x", SwapNone))},
		{LocationSwap(SwapTextContent), LocationSelectOOB(OOB("x", SwapNone))},
		{LocationSelect("#x"), LocationSwap(SwapTextContent)},
	} {
		w := newSpy()
		assertUnchanged(t, w, w.h.Clone(), Respond(w, Location("/items", opts...)), ErrConflict)
	}
	locationWire(t, LocationSwap(SwapNone), LocationSelectOOB(OOB("x", SwapOuterHTML)))
	if got := locationWire(t, LocationSelectOOB(), LocationSelectOOB(OOB("", ""), OOBSelection{})); got != "/items" {
		t.Fatal(got)
	}
	w := newSpy()
	w.h.Set(HeaderLocation, got)
	if err := Respond(w, Location("", LocationSelectOOB(OOBSelection{}))); err != nil || w.h.Get(HeaderLocation) != got {
		t.Fatal(err, w.h)
	}
	assertUnchanged(t, w, w.h.Clone(), Respond(w, Location("", LocationSelectOOB(OOB("x", SwapNone)))), ErrInvalidConfig)
}

func TestLocationImport(t *testing.T) {
	valid := []struct{ raw, want string }{
		{`{"path":"/items","push":"true"}`, `/items`},
		{`{"path":"/items","selectOOB":"alerts, #count:true"}`, `{"path":"/items","selectOOB":"#alerts:outerHTML,#count:outerHTML"}`},
		{`{"path":"/items","values":{"tag":"go","empty":[]}}`, `{"path":"/items","values":{"tag":["go"]}}`},
		{`{"path":"/items","headers":{"hx-request":"true","hasOwnProperty":"x"}}`, `{"headers":{"HX-Request":"true","Hasownproperty":"x"},"path":"/items"}`},
	}
	for _, tc := range valid {
		w := newSpy()
		w.h["hX-LoCaTiOn"] = []string{tc.raw}
		if err := Respond(w, Trigger("seen")); err != nil {
			t.Fatalf("%s: %v", tc.raw, err)
		}
		if got := w.h.Get(HeaderLocation); got != tc.want {
			t.Fatal(got)
		}
	}
	invalid := []string{
		`{"Path":"/x"}`,
		`{"path":"/x","handler":"x"}`,
		`{"path":"/x","event":"x"}`,
		`{"path":"/x","path":"/y"}`,
		`{}`,
		`{"path":null}`,
		`{"path":"/x","push":false}`,
		`{"path":"/x","push":"false"}`,
		`{"path":"/x","replace":"false"}`,
		`{"path":"/x","push":""}`,
		`{"path":"/x","replace":""}`,
		`{"path":"/x","values":{"x":1}}`,
		`{"path":"/x","values":{"x":[null]}}`,
		`{"path":"/x","values":{"x":[1]}}`,
		`{"path":"/x","values":null}`,
		`{"path":"/x","values":{"hasOwnProperty":"x"}}`,
		`{"path":"/x","headers":{"x":null}}`,
		`{"path":"/x","headers":{"x":"a","X":"b"}}`,
		`{"path":"/x","headers":{"__proto__":"x"}}`,
		`{"path":"/x","selectOOB":[]}`,
		`{"path":"/x","selectOOB":"#a:"}`,
		`{"path":"/x","selectOOB":"#a:outerHTML:ignored"}`,
		`{"path":"/x","selectOOB":"#a: outerHTML"}`,
		`{"path":"/x","selectOOB":"#a:textContent"}`,
		`{"path":"/x","selectOOB":"#a,"}`,
		`{"path":"/x","selectOOB":"##a"}`,
	}
	for _, raw := range invalid {
		w := newSpy()
		w.h.Set(HeaderLocation, raw)
		assertUnchanged(t, w, w.h.Clone(), Respond(w, Trigger("seen")), ErrInvalidHeader)
	}
	for _, raw := range []string{
		`{"path":"/x","replace":"true"}`,
		`{"path":"/x","push":"/y","replace":"/z"}`,
		`{"path":"/x","selectOOB":"#a:none,#a:delete"}`,
	} {
		w := newSpy()
		w.h.Set(HeaderLocation, raw)
		assertUnchanged(t, w, w.h.Clone(), Respond(w, Trigger("seen")), ErrConflict)
	}
}

func TestLocationValuesSelectorsAndSnapshots(t *testing.T) {
	values := url.Values{"tag": {"go", "htmx"}, "empty": nil}
	headers := map[string]string{"x-custom": "yes"}
	oob := []OOBSelection{OOB("alerts", SwapOuterHTML)}
	opts := []LocationOpt{
		LocationSource("#source"),
		LocationTarget("#café"),
		LocationSelect("#items"),
		LocationValues(values),
		LocationHeaders(headers),
		LocationSelectOOB(oob...),
	}
	option := Location("/items", opts...)
	values["tag"][0] = "changed"
	headers["x-custom"] = "changed"
	oob[0] = OOB("changed", SwapDelete)
	opts[0] = LocationTarget("#changed")
	r := MustResponse(option)
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := newSpy()
			if err := r.Apply(w); err != nil {
				t.Error(err)
			}
			got := w.h.Get(HeaderLocation)
			if strings.Contains(got, "changed") || !strings.Contains(got, `\u00e9`) || !strings.Contains(got, `["go","htmx"]`) {
				t.Error(got)
			}
		}()
	}
	wg.Wait()
	for _, opts := range [][]LocationOpt{
		{LocationTarget("#a"), LocationTarget("#b")},
		{LocationValues(url.Values{"x": {"a"}}), LocationValues(url.Values{"x": {"b"}})},
		{LocationHeaders(map[string]string{"X": "a"}), LocationHeaders(map[string]string{"X": "b"})},
	} {
		if _, err := NewResponse(Location("/", opts...)); !errors.Is(err, ErrConflict) {
			t.Fatal(err)
		}
	}
	for _, opt := range []LocationOpt{
		LocationSource(" "),
		LocationTarget("\n"),
		LocationSelect("\t"),
		LocationValues(url.Values{"hasOwnProperty": {"x"}}),
		LocationHeaders(map[string]string{"Bad Name": "x"}),
		LocationHeaders(map[string]string{"X": "\n"}),
		LocationPushURL("true"),
		LocationReplaceURL("false"),
	} {
		if _, err := NewResponse(Location("/", opt)); !errors.Is(err, ErrInvalidConfig) {
			t.Fatal(err)
		}
	}
	// Equivalent imported values use the same singleton equality as constructed options.
	w := newSpy()
	w.h.Set(HeaderLocation, `{"path":"/items","values":{"x":"a"}}`)
	if err := Respond(w, Location("/items", LocationValues(url.Values{"x": {"a"}}))); err != nil {
		t.Fatal(err)
	}
	// Order has meaning for OOB source consumption; complete locations must match it.
	w = newSpy()
	if err := Respond(w, Location("/items", LocationSelectOOB(OOB("a", SwapNone), OOB("b", SwapNone)))); err != nil {
		t.Fatal(err)
	}
	before := w.h.Clone()
	assertUnchanged(t, w, before, Respond(w, Location("/items", LocationSelectOOB(OOB("b", SwapNone), OOB("a", SwapNone)))), ErrConflict)
	if !reflect.DeepEqual(before, w.h) {
		t.Fatal(w.h)
	}
	// A relative path starting with { requires object form to avoid the client's JSON branch.
	w = newSpy()
	if err := Respond(w, Location("{relative")); err != nil {
		t.Fatal(err)
	}
	var decoded map[string]string
	if err := json.Unmarshal([]byte(w.h.Get(HeaderLocation)), &decoded); err != nil || decoded["path"] != "{relative" {
		t.Fatal(err, decoded)
	}
}

func TestEmptyNestedLocationOptionsAndImportedModifiers(t *testing.T) {
	w := newSpy()
	w.h.Set(HeaderLocation, `{"path":"/items","selectOOB":"#alerts:outerHTML"}`)
	before := w.h.Clone()
	err := Respond(w, Location("", LocationOpt{}, LocationSource(""), LocationTarget(""), LocationSelect(""), LocationSwap("", SwapOpt{}), LocationPushURL(""), LocationReplaceURL(""), LocationValues(nil), LocationHeaders(nil), LocationSelectOOB()))
	if err != nil || !reflect.DeepEqual(before, w.h) {
		t.Fatal(err, w.h)
	}
	for _, opt := range []LocationOpt{
		LocationSwap("", SwapDelay(0)),
		LocationSwap("extension"),
		LocationValues(url.Values{"x": {"\xff"}}),
		LocationHeaders(map[string]string{"": "x"}),
	} {
		assertUnchanged(t, w, w.h.Clone(), Respond(w, Location("/items", opt)), ErrInvalidConfig)
	}
	w = newSpy()
	w.h.Set(HeaderLocation, `{"path":"/items","source":"#source","swap":"innerHTML swap:1s show:none","selectOOB":"","values":{"x":[]}}`)
	if err := Respond(w, Location("/items", LocationSource("#source"), LocationSwap(SwapInnerHTML, SwapDelay(time.Second), Show(ScrollNone, "")))); err != nil {
		t.Fatal(err)
	}
}
