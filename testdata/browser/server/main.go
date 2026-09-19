// Browser fixture: application-owned HTML and routing around the actual package.
package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"sync"

	"github.com/rhyselsmore/htmx"
)

// Parallel polling cases have independent counters, keyed by the test token.
var polls sync.Map

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/htmx.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/javascript")
		http.ServeFile(w, r, "vendor/htmx-2.0.10.js")
	})
	mux.HandleFunc("/fixture-events.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/javascript")
		writeFixture(w, eventObservers)
	})
	mux.HandleFunc("/response", response)
	mux.HandleFunc("/redirect-destination", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/destination?"+r.URL.RawQuery, http.StatusFound)
	})
	mux.HandleFunc("/destination", destination)
	mux.HandleFunc("/http-redirect", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(htmx.HeaderTrigger, "ignored-redirect")
		http.Redirect(w, r, "/final-fragment", http.StatusFound)
	})
	mux.HandleFunc("/final-fragment", func(w http.ResponseWriter, r *http.Request) {
		if !respond(w, htmx.Trigger("final-redirect")) {
			return
		}
		writeFixture(w, "<p id=final>final</p>")
	})
	mux.HandleFunc("/poll", func(w http.ResponseWriter, r *http.Request) {
		key := r.URL.Query().Get("token")
		p, _ := polls.LoadOrStore(key, new(pollCount))
		c := p.(*pollCount)
		c.Lock()
		c.n++
		n := c.n
		c.Unlock()
		if n >= 2 {
			if err := htmx.StopPolling(w, htmx.Trigger("done")); err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
		}
		writeFixture(w, strconv.Itoa(n))
	})
	mux.HandleFunc("/poll-count", func(w http.ResponseWriter, r *http.Request) {
		p, ok := polls.Load(r.URL.Query().Get("token"))
		if !ok {
			writeFixture(w, "0")
			return
		}
		c := p.(*pollCount)
		c.Lock()
		defer c.Unlock()
		writeFixture(w, strconv.Itoa(c.n))
	})
	mux.HandleFunc("/history/next", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Wants-Fragment", strconv.FormatBool(htmx.WantsFragment(r)))
		if htmx.WantsFragment(r) {
			writeFixture(w, "<article id=next>next fragment</article>")
		} else {
			writeFixture(w, "<!doctype html><main id=main><article id=next>next full page</article></main>")
		}
	})
	mux.HandleFunc("/", page)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}
	// setup.cjs reads this first stdout line to discover the ephemeral port.
	fmt.Println("http://" + listener.Addr().String())
	log.Fatal(http.Serve(listener, mux))
}

type pollCount struct {
	sync.Mutex
	n int
}

// respond stops handlers from writing a success body after header validation fails.
func respond(w http.ResponseWriter, opts ...htmx.ResponseOpt) bool {
	if err := htmx.Respond(w, opts...); err != nil {
		http.Error(w, err.Error(), 500)
		return false
	}
	return true
}

// A failed socket write usually means the browser closed the page. Log it for
// test diagnostics; the response is already committed, so no error page is sent.
func writeFixture(w http.ResponseWriter, body string) {
	if _, err := io.WriteString(w, body); err != nil {
		log.Printf("write fixture response: %v", err)
	}
}
