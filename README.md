# htmx

[![Go Reference](https://pkg.go.dev/badge/github.com/rhyselsmore/htmx.svg)](https://pkg.go.dev/github.com/rhyselsmore/htmx)
[![Tests](https://github.com/rhyselsmore/htmx/actions/workflows/tests.yml/badge.svg)](https://github.com/rhyselsmore/htmx/actions/workflows/tests.yml)

Read htmx request headers and prepare validated response headers in Go.

## Why Another htmx Package?

Go has established htmx packages, including
[htmx-go](https://github.com/angelofallars/htmx-go) and
[go-htmx](https://github.com/donseba/go-htmx). This package comes from an internal
application where response instructions needed to compose predictably. A handler
might choose a swap target, attach an event, and update browser history. When
those decisions move into helpers, checking the complete response becomes useful.

The package rejects malformed input, conflicting instructions, and values outside
its supported subset. A redirect combined with response swap instructions returns
an error before headers change. Static responses can be checked at startup and
reused; request-specific options use the same rules. Ordinary Go functions,
slices, conditionals, and named payload structs handle composition.

## Scope

The package uses only the standard library and works with `net/http`.

- **Rendering and routing belong to the application.** Handlers know the data
  and choose the HTML and status. `Respond` and `Apply` write neither status nor
  body; `StopPolling` writes status 286 after successfully preparing headers.
- **Applications load and configure htmx.** Browser settings and page markup
  affect response behavior. Selector checks cannot establish CSS validity,
  target existence, or fragment overlap.
- **Authentication and destination policy need application context.** Request
  headers are client-supplied hints. URL validation is not a same-origin or
  authorization check.
- **Caching depends on the endpoint's variants.** Applications set `Vary` and
  cache directives. The [fragment example](https://pkg.go.dev/github.com/rhyselsmore/htmx#example-WantsFragment)
  shows how to preserve existing cache variation.

## Install

Requires **Go 1.21 or later**.

```sh
go get github.com/rhyselsmore/htmx
```

```go
import "github.com/rhyselsmore/htmx"
```

## Examples

### Return a Fragment and an Event

Use a named struct for the event data and prepare all headers before writing HTML:

```go
type ItemSaved struct {
    ID string `json:"id"`
}

func saved(w http.ResponseWriter, r *http.Request) {
    if err := htmx.Respond(w,
        htmx.Retarget("#items"),
        htmx.Reswap(htmx.SwapOuterHTML),
        htmx.Trigger("itemSaved", htmx.Detail(ItemSaved{ID: "42"})),
    ); err != nil {
        http.Error(w, "Could not prepare response", http.StatusInternalServerError)
        return
    }
    if _, err := fmt.Fprint(w, `<section id="items">Saved</section>`); err != nil {
        log.Printf("write response: %v", err)
    }
}
```

`Detail` captures the payload when the option is created. Later changes to the
input do not change the response. Your application can render the fragment with
its usual template library.

### Extend a Prepared Response

Prepare common settings once, then derive a response with request-specific data:

```go
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
```

The handler checks `prepareSaved`'s error before writing status or HTML.
`With` returns a new response and leaves the base unchanged. Responses can be
reused concurrently with separate writers. Use `MustResponse` for static startup
configuration; use error-returning constructors for request data.

`With` follows the existing merge rules: equal settings agree, different singleton
settings conflict, and a repeated event name replaces its payload and target in
that phase. It does not override every setting or patch an existing location.

### Build a Response Conditionally

Build options in a local slice and apply them together:

```go
func prepareItem(w http.ResponseWriter, id string, created bool) error {
    opts := []htmx.ResponseOpt{
        htmx.Retarget("#items"),
        htmx.Reswap(htmx.SwapOuterHTML),
    }
    if created {
        opts = append(opts,
            htmx.Trigger("itemCreated", htmx.Detail(ItemSaved{ID: id})),
        )
    }
    return htmx.Respond(w, opts...)
}
```

The caller handles the returned error before rendering. No context wrapper or
callback lifecycle is needed to assemble a response.

### Handle a Conflict

A redirect and a response swap cannot be combined:

```go
w := httptest.NewRecorder()
w.Header().Set("X-App", "keep")
err := htmx.Respond(w,
    htmx.Redirect("/sign-in"),
    htmx.Reswap(htmx.SwapOuterHTML),
)
fmt.Println(errors.Is(err, htmx.ErrConflict)) // true
fmt.Println(w.Header().Get("X-App"))        // keep
fmt.Println(len(w.Header()))                // 1
```

A failed `With` similarly returns no response and leaves its base usable.
See the [executable examples](example_test.go) for complete examples, including
navigation, history, OOB fragments, polling, and error handling.

> Returning an error before applying unsupported or conflicting instructions is
> intentional: the handler decides how to recover. Request-time values still
> need validation, and applying a response can conflict with existing headers.
> Check errors before writing status or HTML.

Atomicity is per call. Earlier successful headers remain after a later failure;
`http.Error` does not clear them. Derive with `With` and apply once, or collect
related options in one `Respond` call. The package cannot undo application work
or HTML already sent.

## API Guide

The [Go documentation](https://pkg.go.dev/github.com/rhyselsmore/htmx) describes
supported inputs, merge rules, client limitations, and every option.

| Task | Reference |
| --- | --- |
| Construct, derive, and apply responses | [NewResponse](https://pkg.go.dev/github.com/rhyselsmore/htmx#NewResponse), [Response.With](https://pkg.go.dev/github.com/rhyselsmore/htmx#Response.With), [Response.Apply](https://pkg.go.dev/github.com/rhyselsmore/htmx#Response.Apply), [Respond](https://pkg.go.dev/github.com/rhyselsmore/htmx#Respond) |
| Read request hints and choose fragments | [IsRequest](https://pkg.go.dev/github.com/rhyselsmore/htmx#IsRequest), [WantsFragment](https://pkg.go.dev/github.com/rhyselsmore/htmx#WantsFragment) |
| Send events and payloads | [Trigger](https://pkg.go.dev/github.com/rhyselsmore/htmx#Trigger), [Detail](https://pkg.go.dev/github.com/rhyselsmore/htmx#Detail), [TriggerTarget](https://pkg.go.dev/github.com/rhyselsmore/htmx#TriggerTarget) |
| Navigate and manage history | [Redirect](https://pkg.go.dev/github.com/rhyselsmore/htmx#Redirect), [Location](https://pkg.go.dev/github.com/rhyselsmore/htmx#Location), [PushURL](https://pkg.go.dev/github.com/rhyselsmore/htmx#PushURL), [ReplaceURL](https://pkg.go.dev/github.com/rhyselsmore/htmx#ReplaceURL) |
| Choose swaps and OOB fragments | [Reswap](https://pkg.go.dev/github.com/rhyselsmore/htmx#Reswap), [LocationSelectOOB](https://pkg.go.dev/github.com/rhyselsmore/htmx#LocationSelectOOB) |
| Stop polling or inspect errors | [StopPolling](https://pkg.go.dev/github.com/rhyselsmore/htmx#StopPolling), [Error](https://pkg.go.dev/github.com/rhyselsmore/htmx#Error) |

## Compatibility and Testing

Client behavior is tested against **htmx 2.0.10** in Chromium, Firefox, and WebKit.
This is an exact tested version, not a claim that every htmx 2.x release behaves
identically. The client is supplied by your application.

This module uses independent semantic versions. Patch releases fix compatible
bugs, minor releases add compatible functionality, and major releases change the
public contract. See [release notes](https://github.com/rhyselsmore/htmx/releases)
for changes and tested client versions.

```sh
go test ./...
```

Go tests include executable examples and fuzz regression cases without browser
downloads. CI also runs race detection, vet, lint, bounded fuzzing, benchmarks,
standalone extraction, and browser tests. See [contributor checks](CONTRIBUTING.md)
and the [browser fixture README](testdata/browser/README.md) for commands.

## License

Licensed under the [MIT License](LICENSE).
The vendored htmx test client retains its [Zero-Clause BSD license](testdata/browser/vendor/LICENSE).
