package api

import (
	"context"
	"io"
	"net/http"
	"strings"

	"github.com/messenger/server/internal/auth"
)

type contextKey string

const userContextKey contextKey = "user"
const deviceContextKey contextKey = "device"
const proxySessionContextKey contextKey = "proxy_session"

func AuthMiddleware(auth AuthService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := extractToken(r)
			if token == "" {
				writeError(w, http.StatusUnauthorized, "missing_token", "Authorization required")
				return
			}
			userID, deviceID, err := auth.ValidateToken(r.Context(), token)
			if err == nil {
				ctx := context.WithValue(r.Context(), userContextKey, userID)
				ctx = context.WithValue(ctx, deviceContextKey, deviceID.String())
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
			proxySession, err := auth.GetProxySessionByToken(r.Context(), token)
			if err != nil || proxySession == nil {
				writeError(w, http.StatusUnauthorized, "invalid_token", "Invalid or expired token")
				return
			}
			ctx := context.WithValue(r.Context(), proxySessionContextKey, proxySession)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func getProxySession(ctx context.Context) *auth.ProxySession {
	v := ctx.Value(proxySessionContextKey)
	if v == nil {
		return nil
	}
	return v.(*auth.ProxySession)
}

func extractToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if strings.HasPrefix(h, "Bearer ") {
		return strings.TrimPrefix(h, "Bearer ")
	}
	// WebSocket uses ws_token from /auth/ws-token, not access_token
	return ""
}

func getUserID(ctx context.Context) int64 {
	v := ctx.Value(userContextKey)
	if v == nil {
		return 0
	}
	return v.(int64)
}

func getDeviceID(ctx context.Context) string {
	v := ctx.Value(deviceContextKey)
	if v == nil {
		return ""
	}
	return v.(string)
}

// ProxyForwardMiddleware forwards the request to the user's home server when the session is a proxy (federated login).
func ProxyForwardMiddleware() func(http.Handler) http.Handler {
	client := &http.Client{Timeout: 0} // no timeout for streaming
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ps := getProxySession(r.Context())
			if ps == nil {
				next.ServeHTTP(w, r)
				return
			}
			// Forward to home server: https://home_domain + path + query
			scheme := "https"
			if r.TLS == nil && strings.HasPrefix(r.Host, "localhost") {
				scheme = "http"
			}
			homeBase := scheme + "://" + ps.HomeDomain
			path := r.URL.Path
			if r.URL.RawQuery != "" {
				path += "?" + r.URL.RawQuery
			}
			targetURL := homeBase + path
			var body io.Reader
			if r.Body != nil {
				bodyBytes, _ := io.ReadAll(r.Body)
				body = io.NopCloser(strings.NewReader(string(bodyBytes)))
			}
			req, err := http.NewRequestWithContext(r.Context(), r.Method, targetURL, body)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
				return
			}
			req.Header.Set("Authorization", "Bearer "+ps.HomeToken)
			req.Header.Set("Content-Type", r.Header.Get("Content-Type"))
			req.Header.Set("X-Protocol-Version", r.Header.Get("X-Protocol-Version"))
			req.Header.Set("X-Idempotency-Key", r.Header.Get("X-Idempotency-Key"))
			resp, err := client.Do(req)
			if err != nil {
				writeError(w, http.StatusBadGateway, "proxy_error", err.Error())
				return
			}
			defer resp.Body.Close()
			for k, v := range resp.Header {
				if strings.EqualFold(k, "Transfer-Encoding") {
					continue
				}
				for _, vv := range v {
					w.Header().Add(k, vv)
				}
			}
			w.WriteHeader(resp.StatusCode)
			io.Copy(w, resp.Body)
		})
	}
}
