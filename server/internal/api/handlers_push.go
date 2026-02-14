package api

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"github.com/messenger/server/internal/push"
)

type pushHandler struct {
	push *push.Service
}

func (h *pushHandler) vapidPublic(w http.ResponseWriter, r *http.Request) {
	if !h.push.Enabled() {
		writeError(w, http.StatusNotFound, "not_available", "Push not configured")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"public_key": h.push.VAPIDPublicKey()})
}

func (h *pushHandler) subscribe(w http.ResponseWriter, r *http.Request) {
	if !h.push.Enabled() {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		return
	}
	var req struct {
		Endpoint string `json:"endpoint"`
		Keys     struct {
			P256dh string `json:"p256dh"`
			Auth   string `json:"auth"`
		} `json:"keys"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON")
		return
	}
	if req.Endpoint == "" || req.Keys.P256dh == "" || req.Keys.Auth == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "endpoint and keys required")
		return
	}
	userID := getUserID(r.Context())
	deviceIDStr := getDeviceID(r.Context())
	deviceID, err := uuid.Parse(deviceIDStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid device")
		return
	}
	if err := h.push.Subscribe(r.Context(), userID, deviceID, req.Endpoint, req.Keys.P256dh, req.Keys.Auth); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *pushHandler) unsubscribe(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Endpoint string `json:"endpoint"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON")
		return
	}
	if req.Endpoint != "" {
		_ = h.push.Unsubscribe(r.Context(), req.Endpoint)
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
