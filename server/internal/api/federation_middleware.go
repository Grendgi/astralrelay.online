package api

import (
	"encoding/json"
	"log"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

// federationLogger logs federation requests with domain, status, duration; records Prometheus metrics.
func federationLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		origin := r.Header.Get("X-Server-Origin")
		if origin == "" {
			origin = "-"
		}
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)
		dur := time.Since(start)
		status := ww.Status()
		recordFederationRequest(origin, r.URL.Path, status, dur.Seconds())
		entry := map[string]interface{}{
			"type":     "federation",
			"method":   r.Method,
			"path":     r.URL.Path,
			"domain":   origin,
			"status":   status,
			"duration": dur.Milliseconds(),
		}
		data, _ := json.Marshal(entry)
		log.Printf("%s", string(data))
	})
}

// federationRecover catches panics in federation handlers and logs with stack.
func federationRecover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				origin := r.Header.Get("X-Server-Origin")
				log.Printf("federation panic: domain=%s path=%s panic=%v\n%s", origin, r.URL.Path, rec, string(debug.Stack()))
				w.WriteHeader(http.StatusInternalServerError)
				writeError(w, http.StatusInternalServerError, "internal_error", "Request failed")
			}
		}()
		next.ServeHTTP(w, r)
	})
}
