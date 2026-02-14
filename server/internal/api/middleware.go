package api

import (
	"context"
	"net/http"
	"strings"
)

type contextKey string

const userContextKey contextKey = "user"
const deviceContextKey contextKey = "device"

func AuthMiddleware(auth AuthService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := extractToken(r)
			if token == "" {
				writeError(w, http.StatusUnauthorized, "missing_token", "Authorization required")
				return
			}
			userID, deviceID, err := auth.ValidateToken(r.Context(), token)
			if err != nil {
				writeError(w, http.StatusUnauthorized, "invalid_token", "Invalid or expired token")
				return
			}
			ctx := context.WithValue(r.Context(), userContextKey, userID)
			ctx = context.WithValue(ctx, deviceContextKey, deviceID.String())
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func extractToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if strings.HasPrefix(h, "Bearer ") {
		return strings.TrimPrefix(h, "Bearer ")
	}
	// WebSocket upgrade requests can't set Authorization header; token in query
	if q := r.URL.Query().Get("access_token"); q != "" {
		return q
	}
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
