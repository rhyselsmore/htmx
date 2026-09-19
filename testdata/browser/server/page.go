package main

import (
	_ "embed"
	"fmt"
	"html"
	"net/http"
	"strconv"
	"strings"

	"github.com/rhyselsmore/htmx"
)

// Embed the observers so the built fixture does not depend on a source-file path.
//
//go:embed events.js
var eventObservers string

// page renders fresh DOM targets and configures the client for one test case.
func page(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// Prepend overrides so they win over the client's default status rules.
	config := ""
	if q.Get("allow422") == "1" {
		config += `htmx.config.responseHandling.unshift({code:'422',swap:true,error:false});`
	}
	if q.Get("skip286") == "1" {
		config += `htmx.config.responseHandling.unshift({code:'286',swap:false});`
	}
	// Force history restoration through HTTP instead of htmx's snapshot cache.
	config += `htmx.config.historyCacheSize = 0;
htmx.config.historyRestoreAsHxRequest = ` + strconv.FormatBool(q.Get("restoreHx") != "false") + `;`
	inherited := ""
	for _, p := range []struct{ query, attr string }{
		{"inheritPush", "hx-push-url"},
		{"inheritReplace", "hx-replace-url"},
		{"inheritOOB", "hx-select-oob"},
	} {
		if v := q.Get(p.query); v != "" {
			inherited += " " + p.attr + "=\"" + html.EscapeString(v) + "\""
		}
	}
	endpoint := "/response?" + q.Encode()
	if q.Get("case") == "http-redirect" {
		endpoint = "/http-redirect"
	}
	if strings.HasPrefix(r.URL.Path, "/history/") {
		endpoint = "/history/next"
	}
	request := `<button id="request" hx-get="` + html.EscapeString(endpoint) + `" hx-target="#main">request</button>`
	if strings.HasPrefix(r.URL.Path, "/history/") {
		request = `<button id="request" hx-get="/history/next" hx-target="#main" hx-push-url="true">next</button>`
	}
	if q.Get("boosted") == "1" {
		request = `<a id="request" href="` + html.EscapeString(endpoint) + `" hx-boost="true" hx-target="#main" hx-select="#main">request</a>`
	}
	poll := ""
	if q.Get("case") == "poll" {
		poll = `<div id="poll" hx-get="/poll?token=` + html.EscapeString(q.Get("token")) + `" hx-trigger="every 50ms" hx-swap="none"></div>`
	}
	// Targets use distinct wrappers so the specs can distinguish outerHTML,
	// innerHTML, and insertion swaps by inspecting the resulting DOM.
	writeFixture(w, fmt.Sprintf(`<!doctype html>
<html>
<head>
  <title>original title</title>
  <script src="/htmx.js"></script>
  <script>%s</script>
</head>
<body>
  <div id="configuration"%s>%s</div>
  <main id="main">old main</main>
  <section id="items">old items</section>
  <aside id="alerts"><b id="old-alert">old alert</b></aside>
  <strong id="count">0</strong>
  <p id="outside">untouched</p>
  <section id="parent">old parent</section>
  <section id="child">old child</section>
  <div id="scroll-box" style="height:60px;overflow:auto">
    <div style="height:1000px">scroll content</div>
  </div>
  <div style="height:1200px"></div>
  <div id="show-target">show target</div>
  <div id="restore-result" data-restore="%t" data-request="%t" data-fragment="%t"></div>
  %s
  <script src="/fixture-events.js"></script>
</body>
</html>`, config, inherited, request,
		htmx.IsHistoryRestore(r), htmx.IsRequest(r), htmx.WantsFragment(r), poll))
}
