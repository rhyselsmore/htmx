package htmx_test

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"time"

	"github.com/rhyselsmore/htmx"
)

func ExampleRespond() {
	w := httptest.NewRecorder()
	err := htmx.Respond(w, htmx.Retarget("#items"), htmx.Reswap(htmx.SwapOuterHTML), htmx.Trigger("itemSaved", htmx.Detail(ItemSaved{ID: "42"})))
	if err != nil {
		http.Error(w, "Could not prepare response", http.StatusInternalServerError)
		return
	}
	if _, err := fmt.Fprint(w, `<section id="items">Saved</section>`); err != nil {
		log.Printf("write response: %v", err)
		return
	}
	fmt.Println(w.Code, w.Header().Get(htmx.HeaderRetarget), w.Header().Get(htmx.HeaderTrigger))
	// Output: 200 #items {"itemSaved":{"id":"42"}}
}

func ExampleNewResponse() {
	response, err := htmx.NewResponse(htmx.Redirect("/sign-in"))
	if err != nil {
		panic(err)
	}
	w := httptest.NewRecorder()
	if err := response.Apply(w); err != nil {
		http.Error(w, "Could not prepare response", 500)
		return
	}
	w.WriteHeader(http.StatusOK)
	fmt.Println(w.Code, w.Header().Get(htmx.HeaderRedirect))
	// Output: 200 /sign-in
}

func ExampleLocation() {
	w := httptest.NewRecorder()
	if err := htmx.Respond(w, htmx.Location("/search", htmx.LocationTarget("#results"), htmx.LocationValues(url.Values{"tag": {"go", "htmx"}}), htmx.LocationReplaceDestination())); err != nil {
		http.Error(w, "Could not prepare navigation", 500)
		return
	}
	w.WriteHeader(http.StatusOK)
	fmt.Println(w.Header().Get(htmx.HeaderLocation))
	// Output: {"path":"/search","push":"false","replace":"true","target":"#results","values":{"tag":["go","htmx"]}}
}

func ExampleLocationSelectOOB() {
	w := httptest.NewRecorder()
	if err := htmx.Respond(w, htmx.Location("/items/latest", htmx.LocationTarget("#items"), htmx.LocationSelect("#items"), htmx.LocationSwap(htmx.SwapOuterHTML), htmx.LocationSelectOOB(htmx.OOB("alerts", htmx.SwapOuterHTML), htmx.OOB("item-count", htmx.SwapInnerHTML)), htmx.LocationReplaceDestination())); err != nil {
		http.Error(w, "Could not prepare navigation", 500)
		return
	}
	w.WriteHeader(http.StatusOK)
	fmt.Println(w.Header().Get(htmx.HeaderLocation))
	// Output: {"path":"/items/latest","push":"false","replace":"true","select":"#items","selectOOB":"#alerts:outerHTML,#item-count:innerHTML","swap":"outerHTML","target":"#items"}
}

func ExampleDetail() {
	w := httptest.NewRecorder()
	opts := []htmx.ResponseOpt{htmx.Trigger("cleared", htmx.Detail[any](nil)), htmx.Trigger("saved", htmx.Detail("ready"), htmx.TriggerTarget("#notifications"))}
	if err := htmx.Respond(w, opts...); err != nil {
		http.Error(w, "Could not prepare events", 500)
		return
	}
	fmt.Println(w.Header().Get(htmx.HeaderTrigger))
	// Output: {"cleared":null,"saved":{"target":"#notifications","value":"ready"}}
}

func ExampleReswap() {
	w := httptest.NewRecorder()
	if err := htmx.Respond(w, htmx.Reswap(htmx.SwapInnerHTML, htmx.SwapDelay(100*time.Millisecond), htmx.SettleDelay(0), htmx.Show(htmx.ScrollNone, ""))); err != nil {
		http.Error(w, "Could not prepare swap", 500)
		return
	}
	fmt.Println(w.Header().Get(htmx.HeaderReswap))
	// Output: innerHTML swap:100ms settle:0ms show:none
}

func ExampleStopPolling() {
	w := httptest.NewRecorder()
	if err := htmx.StopPolling(w, htmx.Trigger("finished")); err != nil {
		http.Error(w, "Could not stop polling", 500)
		return
	}
	if _, err := fmt.Fprint(w, "Done"); err != nil {
		log.Printf("write response: %v", err)
		return
	}
	fmt.Println(w.Code, w.Header().Get(htmx.HeaderTrigger), w.Body.String())
	// Output: 286 finished Done
}

func ExampleRespond_conflict() {
	w := httptest.NewRecorder()
	w.Header().Set("X-App", "keep")
	err := htmx.Respond(w, htmx.Redirect("/sign-in"), htmx.Reswap(htmx.SwapOuterHTML))
	fmt.Println(errors.Is(err, htmx.ErrConflict), w.Header().Get("X-App"), len(w.Header()))
	// Output: true keep 1
}

func ExampleResponse_Apply_accumulation() {
	w := httptest.NewRecorder()
	base := htmx.MustResponse(htmx.Trigger("first", htmx.Detail("ready")))
	if err := base.Apply(w); err != nil {
		http.Error(w, "Could not prepare events", 500)
		return
	}
	if err := htmx.Respond(w, htmx.Trigger("second")); err != nil {
		// Earlier successful headers remain. An error responder that wants a clean
		// response must explicitly remove the headers it owns before writing it.
		w.Header().Del(htmx.HeaderTrigger)
		http.Error(w, "Could not add event", 500)
		return
	}
	fmt.Println(w.Header().Get(htmx.HeaderTrigger))
	// Output: {"first":"ready","second":null}
}

// mergeVary belongs to this example's application, not the htmx package.
func mergeVary(h http.Header, names ...string) {
	seen := map[string]bool{}
	for _, line := range h.Values("Vary") {
		for _, name := range strings.Split(line, ",") {
			name = strings.TrimSpace(name)
			if name == "*" {
				return
			}
			seen[strings.ToLower(name)] = true
		}
	}
	for _, name := range names {
		if !seen[strings.ToLower(name)] {
			h.Add("Vary", name)
			seen[strings.ToLower(name)] = true
		}
	}
}

func ExampleWantsFragment() {
	r := httptest.NewRequest("GET", "/items", nil)
	r.Header.Set(htmx.HeaderRequest, "true")
	w := httptest.NewRecorder()
	w.Header().Set("Vary", "Accept-Encoding")
	// Do this for both page and fragment responses.
	mergeVary(w.Header(), htmx.HeaderRequest, htmx.HeaderBoosted, htmx.HeaderHistoryRestoreRequest)
	fmt.Println(htmx.WantsFragment(r))
	r.Header.Set(htmx.HeaderHistoryRestoreRequest, "true")
	fmt.Println(htmx.WantsFragment(r))
	fmt.Println(w.Header().Values("Vary"))
	// Output:
	// true
	// false
	// [Accept-Encoding HX-Request HX-Boosted HX-History-Restore-Request]
}

// ItemSaved is application data, not a package-owned payload type.
type ItemSaved struct {
	ID string `json:"id"`
}

var itemBase = htmx.MustResponse(
	htmx.Retarget("#items"),
	htmx.Reswap(htmx.SwapOuterHTML),
)

func prepareSaved(w http.ResponseWriter, id string) error {
	response, err := itemBase.With(
		htmx.Trigger("itemSaved", htmx.Detail(ItemSaved{ID: id})),
	)
	if err != nil {
		return err
	}
	return response.Apply(w)
}

func ExampleResponse_With() {
	w := httptest.NewRecorder()
	if err := prepareSaved(w, "42"); err != nil {
		http.Error(w, "Could not prepare response", http.StatusInternalServerError)
		return
	}
	if _, err := fmt.Fprint(w, `<section id="items">Saved</section>`); err != nil {
		log.Printf("write response: %v", err)
		return
	}
	fmt.Println(w.Header().Get(htmx.HeaderRetarget))
	fmt.Println(w.Header().Get(htmx.HeaderTrigger))
	// Output:
	// #items
	// {"itemSaved":{"id":"42"}}
}

func prepareItem(w http.ResponseWriter, id string, created bool) error {
	opts := []htmx.ResponseOpt{
		htmx.Retarget("#items"),
		htmx.Reswap(htmx.SwapOuterHTML),
	}
	if created {
		opts = append(opts, htmx.Trigger("itemCreated", htmx.Detail(ItemSaved{ID: id})))
	}
	return htmx.Respond(w, opts...)
}

func ExampleRespond_conditional() {
	for _, created := range []bool{false, true} {
		w := httptest.NewRecorder()
		if err := prepareItem(w, "42", created); err != nil {
			http.Error(w, "Could not prepare response", http.StatusInternalServerError)
			return
		}
		fmt.Println(created, w.Header().Get(htmx.HeaderTrigger) != "")
	}
	// Output:
	// false false
	// true true
}

func ExampleResponse_With_conflict() {
	base := htmx.MustResponse(htmx.Retarget("#items"))
	response, err := base.With(htmx.Retarget("#other"))
	fmt.Println(response == nil, errors.Is(err, htmx.ErrConflict))
	w := httptest.NewRecorder()
	if err := base.Apply(w); err != nil {
		http.Error(w, "Could not prepare response", http.StatusInternalServerError)
		return
	}
	fmt.Println(w.Header().Get(htmx.HeaderRetarget))
	// Output:
	// true true
	// #items
}
