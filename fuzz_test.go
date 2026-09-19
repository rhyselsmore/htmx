package htmx

import (
	"bytes"
	"encoding/json"
	"io"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func numbers(raw []byte) []string {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var out []string
	for {
		tok, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}
		if n, ok := tok.(json.Number); ok {
			out = append(out, n.String())
		}
	}
	sort.Strings(out)
	return out
}

func FuzzTriggerRoundTrip(f *testing.F) {
	for _, s := range []string{
		`a, b`,
		`{"a":9007199254740993,"b":{"n":[1e100,-0,1.50]}}`,
		`{"z":null,"a":{"target":"#a","value":"caf\u00e9"}}`,
		`{"x":`,
		`{"x":1,"x":2}`,
		`{"__proto__":null}`,
		`{}`,
		`hasOwnProperty`,
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		p, err := parseTriggerHeader(raw)
		if err != nil {
			return
		}
		encoded, err := encodeTriggerHeader(p)
		if err != nil {
			t.Fatal(err)
		}
		if !headerASCII(encoded) {
			t.Fatalf("non-ASCII output %q", encoded)
		}
		next, err := parseTriggerHeader(encoded)
		if err != nil {
			t.Fatalf("cannot parse %q: %v", encoded, err)
		}
		again, err := encodeTriggerHeader(next)
		if err != nil || encoded != again {
			t.Fatalf("unstable %q -> %q (%v)", encoded, again, err)
		}
		if p.json && !reflect.DeepEqual(numbers([]byte(raw)), numbers([]byte(encoded))) {
			t.Fatalf("numeric tokens changed %q -> %q", raw, encoded)
		}
	})
}

func FuzzTriggerAccumulation(f *testing.F) {
	f.Add("first", "second", `{"id":9007199254740993}`)
	f.Add("same", "same", `null`)
	f.Add("a", "b", `[1e100,1.25]`)
	f.Fuzz(func(t *testing.T, first, second, payload string) {
		w := newSpy()
		if err := Respond(w, Trigger(first, Detail(json.RawMessage(payload)))); err != nil {
			return
		}
		before := w.h.Clone()
		err := Respond(w, Trigger(second))
		if err != nil {
			if !reflect.DeepEqual(w.h, before) {
				t.Fatal("failed accumulation changed headers")
			}
			return
		}
		p, err := parseTriggerHeader(w.h.Get(HeaderTrigger))
		if err != nil {
			t.Fatal(err)
		}
		if first != second && first != "" {
			old, ok := p.events[first]
			if !ok || !reflect.DeepEqual(numbers(old.detail), numbers([]byte(payload))) {
				t.Fatalf("lost prior event: %v", p)
			}
		}
		if second != "" {
			if _, ok := p.events[second]; !ok {
				t.Fatal("new event missing")
			}
		}
	})
}

func FuzzLocationRoundTrip(f *testing.F) {
	for _, raw := range []string{
		`/items`,
		`{"path":"/items","push":"false","replace":"true","selectOOB":"#alerts:true,#count:innerHTML"}`,
		`{"path":"/items","values":{"tag":["go","htmx"]}}`,
		`{"path":"/","headers":{"hx-request":"true"}}`,
		`{"path":"/","values":{"bad":[null]}}`,
		`{"Path":"/"}`,
		`{"path":"/","swap":"none swap:1m30s"}`,
		`{"path":"/","swap":"none swap:0.00000000000001ms"}`,
		`{"path":"/","swap":"none swap:0.00005m"}`,
	} {
		f.Add(raw)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		w := newSpy()
		w.h.Set(HeaderLocation, raw)
		before := w.h.Clone()
		err := Respond(w, Trigger("probe"))
		if err != nil {
			assertUnchanged(t, w, before, err, err.(*Error).Kind)
			return
		}
		encoded := w.h.Get(HeaderLocation)
		if !headerASCII(encoded) {
			t.Fatal("non-ASCII location")
		}
		if err := Respond(w, Trigger("probe")); err != nil || w.h.Get(HeaderLocation) != encoded {
			t.Fatalf("unstable %q: %v", encoded, err)
		}
	})
}

func FuzzApplyAtomicity(f *testing.F) {
	f.Add(uint8(0), `{"path":"/"}`, "saved", `{"id":9007199254740993}`)
	f.Add(uint8(8), `{"old":1}`, "saved", `{"n":1}`)
	f.Add(uint8(9), `bad,,name`, "bad,name", `{`)
	f.Fuzz(func(t *testing.T, index uint8, value, name, payload string) {
		w := newSpy()
		header := managedHeaders[int(index)%len(managedHeaders)]
		w.h[strings.ToLower(header)] = []string{value}
		w.h["X-Unmanaged"] = []string{"first", "second"}
		before := w.h.Clone()
		opt := Trigger(name, Detail(json.RawMessage(payload)))
		err := Respond(w, opt)
		if err != nil {
			if !reflect.DeepEqual(w.h, before) {
				t.Fatal("failure changed headers")
			}
		} else {
			after := w.h.Clone()
			if err := Respond(w, opt); err != nil || !reflect.DeepEqual(after, w.h) {
				t.Fatalf("non-idempotent application: %v", err)
			}
			for k, values := range w.h {
				if strings.HasPrefix(strings.ToLower(k), "hx-") {
					for _, v := range values {
						if !headerASCII(v) {
							t.Fatal("unsafe wire output")
						}
					}
				}
			}
		}
		if len(w.statuses) > 0 || len(w.body) > 0 {
			t.Fatal("application wrote status/body")
		}
	})
}
