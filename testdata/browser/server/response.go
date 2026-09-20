package main

import (
	"html"
	"net/http"
	"strconv"
	"time"

	"github.com/rhyselsmore/htmx"
)

// response translates the case selected by package.spec.cjs into public API calls.
func response(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	kind := q.Get("case")
	var opts []htmx.ResponseOpt
	switch kind {
	case "derived":
		derivedResponse(w, r)
		return
	case "location":
		locationResponse(w, q)
		return
	case "redirect":
		respond(w, htmx.Redirect("/landed"))
		return
	case "refresh":
		respond(w, htmx.Refresh())
		return
	case "events":
		opts = append(opts,
			htmx.Trigger("controls", htmx.Detail("\x7f")),
			htmx.Trigger("z-plain", htmx.Detail("café 😀")),
			htmx.Trigger("a-target", htmx.TriggerTarget("#alerts"), htmx.Detail([]int{1, 2})),
			htmx.Trigger("b-target", htmx.TriggerTarget("#count"), htmx.Detail(map[string]int{"n": 7})),
			htmx.Trigger("object", htmx.Detail(map[string]int{"n": 3})),
			htmx.Trigger("scalar", htmx.Detail(7)),
			htmx.Trigger("array", htmx.Detail([]int{1, 2})),
			htmx.Trigger("nil", htmx.Detail[any](nil)),
			// Replacing an event must discard its old target as well as its detail.
			htmx.Trigger("duplicate", htmx.TriggerTarget("#alerts"), htmx.Detail("old")),
			htmx.Trigger("duplicate", htmx.Detail("new")),
			// Domain fields that collide with client metadata belong under value.
			htmx.Trigger("metadata", htmx.Detail(map[string]any{
				"value": map[string]string{"elt": "domain", "error": "domain"},
			})),
		)
	case "phase-routing":
		makeTrigger := htmx.Trigger
		if q.Get("phase") == "swap" {
			makeTrigger = htmx.TriggerAfterSwap
		}
		if q.Get("phase") == "settle" {
			makeTrigger = htmx.TriggerAfterSettle
		}
		// Deliberately pass the targeted event first. Encoding must reorder it
		// to avoid leaking that target into the ordinary event in htmx 2.0.10.
		opts = append(opts,
			makeTrigger("a-target", htmx.TriggerTarget("#alerts"), htmx.Detail(7)),
			makeTrigger("z-plain", htmx.Detail("ordinary")),
		)
	case "scroll":
		opts = append(opts, htmx.Reswap(htmx.SwapInnerHTML,
			htmx.Scroll(htmx.ScrollBottom, "#scroll-box:not(.absent)"),
			htmx.Show(htmx.ScrollBottom, "#show-target"),
		))
	case "status", "none", "modifiers":
		opts = append(opts,
			htmx.Trigger("immediate"),
			htmx.TriggerAfterSwap("after-swap"),
			htmx.TriggerAfterSettle("after-settle"),
		)
		if kind == "none" {
			opts = append(opts, htmx.Reswap(htmx.SwapNone))
		}
		if kind == "status" {
			opts = append(opts, htmx.Retarget("#main"), htmx.Reswap(htmx.SwapInnerHTML))
		}
		if kind == "modifiers" {
			opts = append(opts, htmx.Reswap(htmx.SwapInnerHTML,
				htmx.SwapDelay(60*time.Millisecond),
				htmx.SettleDelay(40*time.Millisecond),
				htmx.IgnoreTitle(q.Get("ignoreTitle") == "1"),
				htmx.Transition(q.Get("transition") == "1"),
				htmx.Show(htmx.ScrollNone, ""),
				htmx.FocusScroll(false),
			))
		}
	}
	if !respond(w, opts...) {
		return
	}
	if code, _ := strconv.Atoi(q.Get("status")); code != 0 {
		w.WriteHeader(code)
		if code == http.StatusNoContent {
			return // A 204 delivers headers, but cannot carry a response body.
		}
	}
	body := `<p id="changed">updated main</p>`
	if kind == "none" {
		body += `<div id="alerts" hx-swap-oob="true">markup OOB</div>`
	}
	if kind == "modifiers" {
		body = `<title>new title</title>` + body
	}
	writeFixture(w, body)
}

// Every request derives from the same base. Browser checks use different data
// and targets, then apply the base itself to detect leaked request state.
var derivedBase = htmx.MustResponse(
	htmx.Retarget("#main"),
	htmx.Reswap(htmx.SwapInnerHTML),
	htmx.Trigger("a-target", htmx.Detail("base"), htmx.TriggerTarget("#alerts")),
	htmx.Trigger("z-plain", htmx.Detail("ordinary")),
	htmx.TriggerAfterSwap("after-swap", htmx.Detail("base")),
)

func derivedResponse(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	value := q.Get("value")
	response := derivedBase
	if q.Get("mode") == "base" {
		value = "base"
	} else {
		event := htmx.Trigger("a-target", htmx.Detail(value), htmx.TriggerTarget(q.Get("target")))
		if q.Get("mode") == "plain" {
			// Plain replacement must remove both the base's payload and its target.
			event = htmx.Trigger("a-target")
		}
		var err error
		response, err = derivedBase.With(event, htmx.TriggerAfterSwap("after-swap", htmx.Detail(value)))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	if err := response.Apply(w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeFixture(w, `<p id="changed">`+html.EscapeString(value)+`</p>`)
}
