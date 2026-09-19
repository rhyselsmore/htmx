package htmx

import (
	"net/http/httptest"
	"testing"
)

func TestRequestFlags(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set(HeaderRequest, "true")
	r.Header.Set(HeaderBoosted, "true")
	r.Header.Set(HeaderHistoryRestoreRequest, "true")

	if !IsRequest(r) {
		t.Error("IsRequest false for HX-Request: true")
	}
	if !IsBoosted(r) {
		t.Error("IsBoosted false for HX-Boosted: true")
	}
	if !IsHistoryRestore(r) {
		t.Error("IsHistoryRestore false for HX-History-Restore-Request: true")
	}
	if WantsFragment(r) {
		t.Error("WantsFragment true for boosted request, want false")
	}
}

func TestWantsFragment(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set(HeaderRequest, "true")
	if !WantsFragment(r) {
		t.Error("WantsFragment false for htmx, non-boosted request")
	}
}

func TestRequestFieldAccessors(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set(HeaderTriggerID, "submit-btn")
	r.Header.Set(HeaderTriggerName, "save")
	r.Header.Set(HeaderTarget, "#main")
	r.Header.Set(HeaderCurrentURL, "https://example.com/page")
	r.Header.Set(HeaderPrompt, "yes")

	cases := map[string]struct {
		got, want string
	}{
		"trigger":      {TriggerID(r), "submit-btn"},
		"trigger-name": {TriggerName(r), "save"},
		"target":       {Target(r), "#main"},
		"current-url":  {CurrentURL(r), "https://example.com/page"},
		"prompt":       {Prompt(r), "yes"},
	}
	for name, c := range cases {
		if c.got != c.want {
			t.Errorf("%s = %q, want %q", name, c.got, c.want)
		}
	}
}

func TestRequestFlagTruthTable(t *testing.T) {
	for _, request := range []string{"", "false", "TRUE", "true"} {
		for _, boosted := range []string{"", "false", "TRUE", "true"} {
			for _, restore := range []string{"", "false", "TRUE", "true"} {
				r := httptest.NewRequest("POST", "/", nil)
				r.Header.Set(HeaderRequest, request)
				r.Header.Set(HeaderBoosted, boosted)
				r.Header.Set(HeaderHistoryRestoreRequest, restore)
				want := request == "true" && boosted != "true" && restore != "true"
				if WantsFragment(r) != want {
					t.Fatalf("request=%q boosted=%q restore=%q", request, boosted, restore)
				}
			}
		}
	}
}
