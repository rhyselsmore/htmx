package main

import (
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"net/url"

	"github.com/rhyselsmore/htmx"
)

// locationResponse sends navigation options; destination handles the follow-up GET.
func locationResponse(w http.ResponseWriter, q url.Values) {
	path := "/destination?" + q.Encode()
	if q.Get("redirect") == "1" {
		path = "/redirect-destination?" + q.Encode()
	}
	if q.Get("anchor") != "" {
		path += "#" + q.Get("anchor")
	}
	swap := htmx.SwapOuterHTML
	if q.Get("mainNone") == "1" {
		swap = htmx.SwapNone
	}
	lo := []htmx.LocationOpt{
		htmx.LocationSource("#request"),
		htmx.LocationTarget("#items"),
		htmx.LocationSelect("#items"),
		htmx.LocationSwap(swap),
	}
	if q.Get("omitOOB") != "1" {
		strategy := htmx.SwapStrategy(q.Get("strategy"))
		if strategy == "" {
			strategy = htmx.SwapOuterHTML
		}
		id := "alerts"
		if q.Get("missing") != "" {
			id = "ghost"
		}
		selections := []htmx.OOBSelection{htmx.OOB(id, strategy), htmx.OOB("count", htmx.SwapInnerHTML)}
		// Both selections consume nodes from the same response fragment. Taking
		// the parent first also consumes its child; reversing the order does not.
		switch q.Get("order") {
		case "parent-first":
			selections = []htmx.OOBSelection{
				htmx.OOB("parent", htmx.SwapOuterHTML),
				htmx.OOB("child", htmx.SwapOuterHTML),
			}
		case "child-first":
			selections = []htmx.OOBSelection{
				htmx.OOB("child", htmx.SwapOuterHTML),
				htmx.OOB("parent", htmx.SwapOuterHTML),
			}
		}
		lo = append(lo, htmx.LocationSelectOOB(selections...))
	} else {
		// An empty option must leave hx-select-oob inherited from the page intact.
		lo = append(lo, htmx.LocationSelectOOB())
	}
	switch q.Get("history") {
	case "push":
		lo = append(lo, htmx.LocationPushURL("/chosen-push"))
	case "replace":
		lo = append(lo, htmx.LocationReplaceURL("/chosen-replace"))
	case "destination":
		lo = append(lo, htmx.LocationReplaceDestination())
	case "suppress":
		lo = append(lo, htmx.LocationSuppressHistory())
	}
	if q.Get("form") == "1" {
		lo = append(lo,
			htmx.LocationValues(url.Values{
				"tag": {"go", "café"},
				"id":  {"9007199254740993"}, // Larger than JavaScript's exact integer range.
			}),
			htmx.LocationHeaders(map[string]string{
				"x-custom":       "yes",
				"hasOwnProperty": "safe",
			}),
		)
	}
	respond(w, htmx.Location(path, lo...))
}

func destination(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	// The follow-up response can override the initial HX-Location history choice.
	switch q.Get("override") {
	case "suppress":
		if !respond(w, htmx.SuppressHistory()) {
			return
		}
	case "push":
		if !respond(w, htmx.PushURL("/override-push")) {
			return
		}
	case "replace":
		if !respond(w, htmx.ReplaceURL("/override-replace")) {
			return
		}
	}
	data, err := json.Marshal(map[string]any{
		"query":    q,
		"custom":   r.Header.Get("X-Custom"),
		"property": r.Header.Get("Hasownproperty"),
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	body := fmt.Sprintf(`<section id="items"><p>new items</p><pre id="params">%s</pre></section><div id="alerts"><i id="new-alert">saved</i></div><span id="count">3</span>`, html.EscapeString(string(data)))
	if q.Get("missing") == "target" {
		body += `<div id="ghost">no page target</div>`
	}
	if q.Get("order") != "" {
		body += `<div id="parent"><div id="child">new child</div></div>`
	}
	writeFixture(w, body)
}
