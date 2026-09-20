package htmx

import (
	"strings"
	"testing"
)

func BenchmarkPlain(b *testing.B) {
	r := MustResponse(Retarget("#main"), Reswap(SwapInnerHTML))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		w := newSpy()
		if err := r.Apply(w); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTriggerPhases(b *testing.B) {
	r := MustResponse(Trigger("saved", Detail(map[string]int{"n": 1})), TriggerAfterSwap("swapped"), TriggerAfterSettle("settled"))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		w := newSpy()
		if err := r.Apply(w); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkPayload(b *testing.B) {
	value := struct {
		ID   string `json:"id"`
		Text string `json:"text"`
	}{"9007199254740993", "café 😀"}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := NewResponse(Trigger("saved", Detail(value))); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkApplyExisting(b *testing.B) {
	r := MustResponse(Trigger("saved", Detail(7)))
	w := newSpy()
	if err := r.Apply(w); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if err := r.Apply(w); err != nil {
			b.Fatal(err)
		}
	}
}

// Snapshot creation is outside both loops so this compares construction work
// for the same captured options. The base is prepared once for With.
func BenchmarkResponseDerivation(b *testing.B) {
	cases := []struct {
		name string
		opts []ResponseOpt
	}{
		{"static", []ResponseOpt{Retarget("#items"), Reswap(SwapOuterHTML)}},
		{"events", []ResponseOpt{Trigger("first", Detail(1)), Trigger("second", Detail(2)), TriggerAfterSwap("swapped"), TriggerAfterSettle("settled")}},
		{"large_payload", []ResponseOpt{Trigger("payload", Detail(strings.Repeat("café", 4096)))}},
	}
	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			base := MustResponse(tc.opts...)
			dynamic := Trigger("saved", Detail(42))
			all := append(append([]ResponseOpt(nil), tc.opts...), dynamic)
			b.Run("fresh", func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					if _, err := NewResponse(all...); err != nil {
						b.Fatal(err)
					}
				}
			})
			b.Run("with", func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					if _, err := base.With(dynamic); err != nil {
						b.Fatal(err)
					}
				}
			})
		})
	}
}
