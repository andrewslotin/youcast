package main

import (
	"net/http"
	"net/http/pprof"
)

// ProfileMiddleware is a middleware that handles pprof requests.
func ProfileMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch req.URL.Path {
		case "/debug/pprof/":
			pprof.Cmdline(w, req)
		case "/debug/pprof/cmdline":
			pprof.Cmdline(w, req)
		case "/debug/pprof/profile":
			pprof.Profile(w, req)
		case "/debug/pprof/symbol":
			pprof.Symbol(w, req)
		case "/debug/pprof/trace":
			pprof.Trace(w, req)
		case "/debug/pprof/allocs":
			pprof.Handler("allocs").ServeHTTP(w, req)
		case "/debug/pprof/block":
			pprof.Handler("block").ServeHTTP(w, req)
		case "/debug/pprof/goroutine":
			pprof.Handler("goroutine").ServeHTTP(w, req)
		case "/debug/pprof/heap":
			pprof.Handler("heap").ServeHTTP(w, req)
		case "/debug/pprof/mutex":
			pprof.Handler("mutex").ServeHTTP(w, req)
		case "/debug/pprof/threadcreate":
			pprof.Handler("threadcreate").ServeHTTP(w, req)
		default:
			next.ServeHTTP(w, req)
		}
	})
}
