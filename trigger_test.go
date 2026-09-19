package htmx

import (
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func TestTriggerPhases(t *testing.T) {
	constructors := []func(string, ...TriggerOpt) ResponseOpt{Trigger, TriggerAfterSwap, TriggerAfterSettle}
	for i, makeTrigger := range constructors {
		t.Run(triggerHeaders[i], func(t *testing.T) {
			w := newSpy()
			if err := Respond(w, makeTrigger("b"), makeTrigger("a")); err != nil {
				t.Fatal(err)
			}
			if got := w.h.Get(triggerHeaders[i]); got != "a, b" {
				t.Fatal(got)
			}
			if err := Respond(w, makeTrigger("a", Detail(map[string]int{"count": 3}))); err != nil {
				t.Fatal(err)
			}
			if got := w.h.Get(triggerHeaders[i]); got != `{"a":{"count":3},"b":null}` {
				t.Fatal(got)
			}
			if err := Respond(w, makeTrigger("a")); err != nil {
				t.Fatal(err)
			}
			if got := w.h.Get(triggerHeaders[i]); got != `{"a":null,"b":null}` {
				t.Fatal(got)
			}
			before := w.h.Clone()
			assertUnchanged(t, w, before, Respond(w, makeTrigger("bad", Detail(make(chan int)))), ErrEncode)
			if err := Respond(w, makeTrigger("", TriggerOpt{}, TriggerTarget(""))); err != nil || !reflect.DeepEqual(w.h, before) {
				t.Fatal(err, w.h)
			}
		})
	}
}

func TestTargetOrderingAndShapes(t *testing.T) {
	for i, makeTrigger := range []func(string, ...TriggerOpt) ResponseOpt{Trigger, TriggerAfterSwap, TriggerAfterSettle} {
		w := newSpy()
		if err := Respond(w, makeTrigger("a-target", TriggerTarget("#a"), Detail(7)), makeTrigger("z-plain", Detail("café 😀")), makeTrigger("b-target", Detail(map[string]int{"n": 1}), TriggerTarget("#b"))); err != nil {
			t.Fatal(err)
		}
		want := `{"z-plain":"caf\u00e9 \ud83d\ude00","a-target":{"target":"#a","value":7},"b-target":{"n":1,"target":"#b"}}`
		if got := w.h.Get(triggerHeaders[i]); got != want {
			t.Fatalf("%s", got)
		}
		if err := Respond(w, makeTrigger("a-target")); err != nil {
			t.Fatal(err)
		}
		got := w.h.Get(triggerHeaders[i])
		if strings.Index(got, "a-target") > strings.Index(got, "b-target") || strings.Contains(got, "#a") {
			t.Fatal(got)
		}
	}
	for _, detail := range []TriggerOpt{Detail[any](nil), Detail([]int{1, 2}), Detail(true), Detail("x"), {}} {
		w := newSpy()
		if err := Respond(w, Trigger("x", detail, TriggerTarget("#t"))); err != nil {
			t.Fatal(err)
		}
		var fields map[string]map[string]json.RawMessage
		if err := json.Unmarshal([]byte(w.h.Get(HeaderTrigger)), &fields); err != nil {
			t.Fatal(err)
		}
		if _, ok := fields["x"]["value"]; !ok {
			t.Fatal(fields)
		}
	}
}

func TestTriggerParsing(t *testing.T) {
	valid := []string{
		`a, b, a`,
		`{"x":9007199254740993}`,
		`{"x":{"target":"#t","id":9007199254740993}}`,
		`{"__proto__":null,"constructor":1,"prototype":2}`,
		`null, true, false`,
		` a , b `,
	}
	for _, raw := range valid {
		w := newSpy()
		w.h.Set(HeaderTrigger, raw)
		if err := Respond(w, Trigger("new")); err != nil {
			t.Fatalf("%s: %v", raw, err)
		}
	}
	invalid := []string{
		`{"x":`,
		`{"x":1,"x":2}`,
		`{"x":{"target":"#a","target":"#b"}}`,
		`{"x":{"target":1}}`,
		`{"x":{"target":""}}`,
		`{"hasOwnProperty":null}`,
		`a,,b`,
		`a,`,
		`["a"]`,
		`"a"`,
		`{"x":1} {}`,
		`café`,
		"\xff",
		`{"x":"` + string([]byte{255}) + `"}`,
	}
	for _, raw := range invalid {
		w := newSpy()
		w.h.Set(HeaderTrigger, raw)
		assertUnchanged(t, w, w.h.Clone(), Respond(w, Trigger("new")), ErrInvalidHeader)
	}
	w := newSpy()
	w.h.Set(HeaderTriggerAfterSwap, `{"a-target":{"target":"#t"},"z-plain":null}`)
	if err := Respond(w, Trigger("unrelated")); err != nil {
		t.Fatal(err)
	}
	if got := w.h.Get(HeaderTriggerAfterSwap); strings.Index(got, "z-plain") > strings.Index(got, "a-target") {
		t.Fatal(got)
	}
}

type brokenMarshal struct{}

func (brokenMarshal) MarshalJSON() ([]byte, error) { return nil, errors.New("sensitive secret") }

type countingMarshal struct{ calls *int }

func (v countingMarshal) MarshalJSON() ([]byte, error) {
	*v.calls++
	return []byte(`{"count":1}`), nil
}

func TestTriggerErrors(t *testing.T) {
	cyclic := map[string]any{}
	cyclic["self"] = cyclic
	for _, opt := range []TriggerOpt{
		Detail(make(chan int)),
		Detail(func() {}),
		Detail(cyclic),
		Detail(math.NaN()),
		Detail(math.Inf(1)),
		Detail(json.RawMessage(`{`)),
		Detail(brokenMarshal{}),
	} {
		_, err := NewResponse(Trigger("x", opt))
		if !errors.Is(err, ErrEncode) || strings.Contains(err.Error(), "sensitive") {
			t.Fatal(err)
		}
		var e *Error
		if !errors.As(err, &e) || e.Event != "x" || e.Header != HeaderTrigger {
			t.Fatal(err)
		}
	}
	_, err := NewResponse(Trigger("x", Detail(brokenMarshal{})))
	var marshalerError *json.MarshalerError
	if !errors.As(err, &marshalerError) {
		t.Fatal(err)
	}
	for _, opt := range []ResponseOpt{
		Trigger("x", Detail(1), Detail(1)),
		Trigger("x", TriggerTarget("#a"), TriggerTarget("#b")),
		Trigger("", Detail[any](nil)),
		Trigger("x", Detail(map[string]string{"target": "#a"})),
		Trigger("hasOwnProperty"),
		Trigger("x y"),
		Trigger("é"),
		Trigger("1x"),
		Trigger(" x"),
		Trigger("x", TriggerTarget("\n")),
	} {
		if _, err := NewResponse(opt); err == nil {
			t.Fatal("accepted invalid trigger")
		}
	}
	for _, nav := range []ResponseOpt{Redirect("/"), Refresh(), Location("/")} {
		for _, event := range []ResponseOpt{TriggerAfterSwap("x"), TriggerAfterSettle("x")} {
			for _, opts := range [][]ResponseOpt{{nav, event}, {event, nav}} {
				if _, err := NewResponse(opts...); !errors.Is(err, ErrConflict) {
					t.Fatal(err)
				}
			}
		}
		if _, err := NewResponse(nav, Trigger("x")); err != nil {
			t.Fatal(err)
		}
	}
}

func TestPayloadAndOptionSnapshots(t *testing.T) {
	payload := map[string][]string{"items": {"first"}}
	raw := json.RawMessage(`{"n":1}`)
	n := 7
	calls := 0
	nested := []TriggerOpt{Detail(payload)}
	opts := []ResponseOpt{
		Trigger("map", nested...),
		Trigger("raw", Detail(raw)),
		Trigger("pointer", Detail(&n)),
		Trigger("custom", Detail(countingMarshal{&calls})),
	}
	payload["items"][0] = "changed"
	raw[5] = '2'
	n = 8
	nested[0] = Detail("changed")
	r := MustResponse(opts...)
	opts[0] = Trigger("changed")
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := newSpy()
			if err := r.Apply(w); err != nil {
				t.Error(err)
			}
			got := w.h.Get(HeaderTrigger)
			if strings.Contains(got, "changed") || !strings.Contains(got, `"pointer":7`) || !strings.Contains(got, `"raw":{"n":1}`) {
				t.Error(got)
			}
		}()
	}
	wg.Wait()
	if calls != 1 {
		t.Fatal(calls)
	}
}

func TestDetailReuseAcrossConcurrentConstructions(t *testing.T) {
	raw := json.RawMessage(`{"id":9007199254740993}`)
	detail := Detail(raw)
	// Detail must own its snapshot even when MarshalJSON returns the input bytes.
	raw[6] = '1'
	opts := []ResponseOpt{
		Trigger("plain", detail),
		Trigger("targeted", detail, TriggerTarget("#alerts")),
	}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			response, err := NewResponse(opts...)
			if err != nil {
				t.Error(err)
				return
			}
			w := newSpy()
			if err := response.Apply(w); err != nil {
				t.Error(err)
				return
			}
			var events map[string]map[string]json.RawMessage
			if err := json.Unmarshal([]byte(w.h.Get(HeaderTrigger)), &events); err != nil {
				t.Error(err)
				return
			}
			for _, name := range []string{"plain", "targeted"} {
				if got := string(events[name]["id"]); got != "9007199254740993" {
					t.Errorf("%s payload changed: %s", name, got)
				}
			}
			if _, ok := events["plain"]["target"]; ok {
				t.Error("target leaked into the shared detail")
			}
			if got := string(events["targeted"]["target"]); got != `"#alerts"` {
				t.Errorf("target missing: %s", got)
			}
		}()
	}
	wg.Wait()
}

func TestDELIsEscapedForHeaderTransport(t *testing.T) {
	w := newSpy()
	opt := Trigger("control", Detail("\x7f"))
	if err := Respond(w, opt); err != nil {
		t.Fatal(err)
	}
	want := `{"control":"\u007f"}`
	if got := w.h.Get(HeaderTrigger); got != want {
		t.Fatalf("got %q", got)
	}
	if err := Respond(w, opt); err != nil {
		t.Fatal(err)
	}
	var decoded map[string]string
	if err := json.Unmarshal([]byte(w.h.Get(HeaderTrigger)), &decoded); err != nil || decoded["control"] != "\x7f" {
		t.Fatal(err, decoded)
	}
}

func TestEmptyImportedJSONRetainsPhaseMode(t *testing.T) {
	for i, makeTrigger := range []func(string, ...TriggerOpt) ResponseOpt{Trigger, TriggerAfterSwap, TriggerAfterSettle} {
		w := newSpy()
		w.h.Set(triggerHeaders[i], `{}`)
		if err := Respond(w, Retarget("#main")); err != nil {
			t.Fatal(err)
		}
		if got := w.h.Get(triggerHeaders[i]); got != `{}` {
			t.Fatalf("empty JSON phase was erased: %q", got)
		}
		if err := Respond(w, makeTrigger("new")); err != nil {
			t.Fatal(err)
		}
		if got := w.h.Get(triggerHeaders[i]); got != `{"new":null}` {
			t.Fatalf("phase lost JSON mode: %q", got)
		}
	}
}
