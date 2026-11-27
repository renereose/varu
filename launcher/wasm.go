//go:build js

// The launcher inside a browser (the web app): the same site as on a
// drive, answered from WebAssembly. The page's service worker gives Go
// the drive's files (a small "fs" on globalThis, see web/sw.js) and
// passes each request to varuServe.
package main

import (
	"net/http"
	"net/http/httptest"
	"runtime/debug"
	"syscall/js"
)

var version = "dev"

func main() {
	// A browser's WebAssembly memory only grows: collect harder, so loading
	// the road network and places leaves less behind.
	debug.SetMemoryLimit(96 << 20)
	site := NewHandler(Config{Root: "/drive", Version: version})
	// varuServe(method, url, range) resolves to {status, headers, body}.
	js.Global().Set("varuServe", js.FuncOf(func(_ js.Value, args []js.Value) any {
		method, url, rng := args[0].String(), args[1].String(), args[2].String()
		return js.Global().Get("Promise").New(js.FuncOf(func(_ js.Value, p []js.Value) any {
			resolve, reject := p[0], p[1]
			go func() {
				req, err := http.NewRequest(method, url, nil)
				if err != nil {
					reject.Invoke(err.Error())
					return
				}
				req.Host = "localhost"
				if rng != "" {
					req.Header.Set("Range", rng)
				}
				rec := httptest.NewRecorder()
				site.ServeHTTP(rec, req)
				headers := js.Global().Get("Object").New()
				for k := range rec.Header() {
					headers.Set(k, rec.Header().Get(k))
				}
				body := js.Global().Get("Uint8Array").New(rec.Body.Len())
				js.CopyBytesToJS(body, rec.Body.Bytes())
				out := js.Global().Get("Object").New()
				out.Set("status", rec.Code)
				out.Set("headers", headers)
				out.Set("body", body)
				resolve.Invoke(out)
			}()
			return nil
		}))
	}))
	select {}
}
