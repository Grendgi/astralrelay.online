package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/messenger/server/internal/vpn"
)

type vpnHandler struct {
	vpn    *vpn.Service
	domain string
}

func (h *vpnHandler) listProtocols(w http.ResponseWriter, r *http.Request) {
	list := h.vpn.ListProtocols()
	if list == nil {
		list = []vpn.ProtocolInfo{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"protocols": list})
}

func (h *vpnHandler) listNodes(w http.ResponseWriter, r *http.Request) {
	list, err := h.vpn.Nodes().ListNodes(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"nodes": list})
}

func (h *vpnHandler) myConfigs(w http.ResponseWriter, r *http.Request) {
	userID := getUserID(r.Context())
	list, err := h.vpn.ListMyConfigs(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"configs": list})
}

func (h *vpnHandler) revoke(w http.ResponseWriter, r *http.Request) {
	userID := getUserID(r.Context())
	deviceIDStr := r.URL.Query().Get("device_id")
	if deviceIDStr == "" {
		deviceIDStr = getDeviceID(r.Context()) // current device if not specified
	}
	deviceID, err := uuid.Parse(deviceIDStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid device_id")
		return
	}
	protocol := r.URL.Query().Get("protocol")
	if protocol == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "protocol required")
		return
	}
	// Must revoke own config only (userID + deviceID from user's devices)
	if err := h.vpn.Revoke(r.Context(), userID, deviceID, protocol); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	recordVPNConfigRevoked(protocol)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *vpnHandler) getConfig(w http.ResponseWriter, r *http.Request) {
	protocol := chi.URLParam(r, "protocol")
	if protocol == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "protocol required")
		return
	}
	userID := getUserID(r.Context())
	deviceIDStr := getDeviceID(r.Context())
	deviceID, err := uuid.Parse(deviceIDStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid device")
		return
	}
	var nodeID *uuid.UUID
	if s := r.URL.Query().Get("node_id"); s != "" {
		if id, err := uuid.Parse(s); err == nil {
			nodeID = &id
		}
	}

	content, filename, err := h.vpn.GetConfig(r.Context(), protocol, userID, deviceID, nodeID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "vpn_error", err.Error())
		return
	}
	recordVPNConfigIssued(protocol)

	format := r.URL.Query().Get("format")
	if format == "json" {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"config":   content,
			"filename": filename,
		})
		return
	}

	w.Header().Set("Content-Type", "application/x-config")
	w.Header().Set("Content-Disposition", "attachment; filename="+filename)
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(content))
}
