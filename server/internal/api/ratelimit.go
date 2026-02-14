package api

import (
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/httprate"
)

// LimitByIP returns middleware that limits requests per IP.
func LimitByIP(requests int, window time.Duration) func(http.Handler) http.Handler {
	return httprate.Limit(
		requests,
		window,
		httprate.WithKeyFuncs(httprate.KeyByIP),
		httprate.WithLimitHandler(func(w http.ResponseWriter, r *http.Request) {
			writeError(w, http.StatusTooManyRequests, "rate_limited", "Too many requests")
		}),
	)
}

// LimitByFederationDomain limits federation requests per X-Server-Origin domain.
// Use for POST /federation/v1/transaction.
// If alertWebhookURL is set, calls webhook on rate limit.
func LimitByFederationDomain(requests int, window time.Duration, alertWebhookURL string) func(http.Handler) http.Handler {
	return httprate.Limit(
		requests,
		window,
		httprate.WithKeyFuncs(func(r *http.Request) (string, error) {
			origin := r.Header.Get("X-Server-Origin")
			if origin == "" {
				return "unknown", nil
			}
			return "fed:" + origin, nil
		}),
		httprate.WithLimitHandler(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("X-Server-Origin")
			if origin == "" {
				origin = "unknown"
			}
			SendFederationAlert(alertWebhookURL, "rate_limit", origin)
			writeError(w, http.StatusTooManyRequests, "rate_limited", "Federation rate limit exceeded")
		}),
	)
}

// LimitByUser returns middleware that limits requests per authenticated user (from context).
// Falls back to IP when user is not in context.
func LimitByUser(requests int, window time.Duration) func(http.Handler) http.Handler {
	return httprate.Limit(
		requests,
		window,
		httprate.WithKeyFuncs(func(r *http.Request) (string, error) {
			if uid := getUserID(r.Context()); uid != 0 {
				return fmt.Sprintf("user:%d", uid), nil
			}
			return httprate.KeyByIP(r)
		}),
		httprate.WithLimitHandler(func(w http.ResponseWriter, r *http.Request) {
			writeError(w, http.StatusTooManyRequests, "rate_limited", "Too many requests")
		}),
	)
}
