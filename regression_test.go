package htmx

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestRegressionEncodingFailurePreservesHeaders(t *testing.T) {
	w := httptest.NewRecorder()
	w.Header().Set(HeaderTrigger, `{"existing":{"id":9007199254740993}}`)
	before := w.Header().Clone()
	err := Respond(w, Trigger("bad", Detail(make(chan int))))
	if !errors.Is(err, ErrEncode) || !reflect.DeepEqual(before, w.Header()) {
		t.Fatalf("error=%v headers=%v", err, w.Header())
	}
}

func TestRegressionLargeNumbers(t *testing.T) {
	w := httptest.NewRecorder()
	w.Header().Set(HeaderTrigger, `{"existing":{"id":9007199254740993,"nested":[1e100,9007199254740995]}}`)
	if err := Respond(w, Trigger("next")); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(w.Header().Get(HeaderTrigger), `9007199254740993`) || !strings.Contains(w.Header().Get(HeaderTrigger), `9007199254740995`) {
		t.Fatal(w.Header())
	}
}

func TestRegressionMalformedHeader(t *testing.T) {
	w := httptest.NewRecorder()
	w.Header().Set(HeaderTrigger, `{"broken":`)
	before := w.Header().Clone()
	if err := Respond(w, Trigger("next")); !errors.Is(err, ErrInvalidHeader) || !reflect.DeepEqual(before, w.Header()) {
		t.Fatalf("error=%v headers=%v", err, w.Header())
	}
}

func TestRegressionBadTriggerInputs(t *testing.T) {
	for _, opt := range []ResponseOpt{Trigger("one,two"), Trigger("x", Detail(1), Detail(2)), Trigger("x", Detail(json.RawMessage(`{`)))} {
		if _, err := NewResponse(opt); err == nil {
			t.Fatal("accepted invalid input")
		}
	}
}

func TestRegressionRestoreNeedsPage(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set(HeaderRequest, "true")
	r.Header.Set(HeaderHistoryRestoreRequest, "true")
	if WantsFragment(r) {
		t.Fatal("history restore requested a fragment")
	}
}

func TestRegressionInvalidLocation(t *testing.T) {
	w := httptest.NewRecorder()
	w.Header().Set(HeaderLocation, "/old")
	before := w.Header().Clone()
	if err := Respond(w, Location("", LocationTarget("#items"))); err == nil || !reflect.DeepEqual(before, w.Header()) {
		t.Fatalf("error=%v headers=%v", err, w.Header())
	}
}
