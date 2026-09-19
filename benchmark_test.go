package htmx

import "testing"

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
