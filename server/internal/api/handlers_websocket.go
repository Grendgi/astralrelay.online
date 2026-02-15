package api

import (
	"net/http"
	"strings"

	"github.com/gorilla/websocket"
	"github.com/messenger/server/internal/stream"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type streamHandler struct {
	hub    *stream.Hub
	auth   AuthService
	domain string
}

// extractWSToken reads ws_token from Sec-WebSocket-Protocol (bearer.TOKEN) or query param (fallback).
func extractWSToken(r *http.Request) string {
	if s := r.URL.Query().Get("ws_token"); s != "" {
		return s
	}
	for _, p := range strings.Split(r.Header.Get("Sec-WebSocket-Protocol"), ",") {
		p = strings.TrimSpace(p)
		if strings.HasPrefix(p, "bearer.") && len(p) > 7 {
			return p[7:]
		}
	}
	return ""
}

func (h *streamHandler) serveWs(w http.ResponseWriter, r *http.Request) {
	wsToken := extractWSToken(r)
	if wsToken == "" {
		writeError(w, http.StatusUnauthorized, "missing_token", "ws_token required (Sec-WebSocket-Protocol: bearer.TOKEN or query param)")
		return
	}
	userID, _, err := h.auth.ValidateWSToken(wsToken)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid_token", "Invalid token")
		return
	}
	recipient := r.URL.Query().Get("as")
	if recipient == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "Query param 'as' (@user:domain) required")
		return
	}

	username, _ := h.auth.GetUsername(r.Context(), userID)
	userAddr := "@" + username + ":" + h.domain
	respHeader := http.Header{}
	if r.Header.Get("Sec-WebSocket-Protocol") != "" {
		respHeader.Set("Sec-WebSocket-Protocol", "bearer."+wsToken)
	}
	conn, err := upgrader.Upgrade(w, r, respHeader)
	if err != nil {
		return
	}
	send := make(chan []byte, 256)
	client := h.hub.Register(recipient, userAddr, send)
	defer func() {
		h.hub.Unregister(client)
		conn.Close()
	}()

	go func() {
		for msg := range send {
			if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		}
	}()

	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			break
		}
	}
}
