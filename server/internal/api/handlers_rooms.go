package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/messenger/server/internal/rooms"
)

type roomsHandler struct {
	rooms *rooms.Service
}

func (h *roomsHandler) create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "name required")
		return
	}
	userID := getUserID(r.Context())
	room, err := h.rooms.Create(r.Context(), req.Name, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"id":         room.ID.String(),
		"name":       room.Name,
		"domain":     room.Domain,
		"address":    rooms.RoomAddr(room.ID, room.Domain),
		"creator_id": room.CreatorID,
	})
}

func (h *roomsHandler) list(w http.ResponseWriter, r *http.Request) {
	userID := getUserID(r.Context())
	list, err := h.rooms.List(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	out := make([]map[string]interface{}, len(list))
	for i, room := range list {
		out[i] = map[string]interface{}{
			"id":      room.ID.String(),
			"name":    room.Name,
			"domain":  room.Domain,
			"address": rooms.RoomAddr(room.ID, room.Domain),
		}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"rooms": out})
}

func (h *roomsHandler) get(w http.ResponseWriter, r *http.Request) {
	roomIDStr := chi.URLParam(r, "roomID")
	roomID, err := uuid.Parse(roomIDStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid room ID")
		return
	}
	userID := getUserID(r.Context())
	room, err := h.rooms.Get(r.Context(), roomID, userID)
	if err == rooms.ErrNotMember {
		writeError(w, http.StatusForbidden, "forbidden", "Not a room member")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id":      room.ID.String(),
		"name":    room.Name,
		"domain":  room.Domain,
		"address": rooms.RoomAddr(room.ID, room.Domain),
	})
}

func (h *roomsHandler) invite(w http.ResponseWriter, r *http.Request) {
	roomIDStr := chi.URLParam(r, "roomID")
	roomID, err := uuid.Parse(roomIDStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid room ID")
		return
	}
	var req struct {
		UserID   int64  `json:"user_id"`
		Username string `json:"username"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON")
		return
	}
	var inviteeUserID int64
	if req.Username != "" {
		inviteeUserID, err = h.rooms.ResolveUserID(r.Context(), req.Username)
		if err == rooms.ErrUserNotFound {
			writeError(w, http.StatusNotFound, "not_found", "User not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
			return
		}
	} else if req.UserID > 0 {
		inviteeUserID = req.UserID
	} else {
		writeError(w, http.StatusBadRequest, "invalid_request", "user_id or username required")
		return
	}
	userID := getUserID(r.Context())
	err = h.rooms.Invite(r.Context(), roomID, userID, inviteeUserID)
	if err == rooms.ErrForbidden {
		writeError(w, http.StatusForbidden, "forbidden", "Not allowed")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"status": "invited"})
}

func (h *roomsHandler) leave(w http.ResponseWriter, r *http.Request) {
	roomIDStr := chi.URLParam(r, "roomID")
	roomID, err := uuid.Parse(roomIDStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid room ID")
		return
	}
	userID := getUserID(r.Context())
	err = h.rooms.Leave(r.Context(), roomID, userID)
	if err == rooms.ErrCreatorCannotLeave {
		writeError(w, http.StatusBadRequest, "bad_request", "Creator cannot leave without transferring ownership")
		return
	}
	if err == rooms.ErrNotMember {
		writeError(w, http.StatusForbidden, "forbidden", "Not a room member")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"status": "left"})
}

func (h *roomsHandler) members(w http.ResponseWriter, r *http.Request) {
	roomIDStr := chi.URLParam(r, "roomID")
	roomID, err := uuid.Parse(roomIDStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid room ID")
		return
	}
	userID := getUserID(r.Context())
	list, err := h.rooms.Members(r.Context(), roomID, userID)
	if err == rooms.ErrNotMember {
		writeError(w, http.StatusForbidden, "forbidden", "Not a room member")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	out := make([]map[string]interface{}, len(list))
	for i, m := range list {
		out[i] = map[string]interface{}{
			"user_id":   m.UserID,
			"username":  m.Username,
			"domain":    m.Domain,
			"address":   "@" + m.Username + ":" + m.Domain,
			"role":      m.Role,
			"joined_at": m.JoinedAt,
		}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"members": out})
}

func (h *roomsHandler) transferCreator(w http.ResponseWriter, r *http.Request) {
	roomIDStr := chi.URLParam(r, "roomID")
	roomID, err := uuid.Parse(roomIDStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid room ID")
		return
	}
	var req struct {
		UserID   int64  `json:"user_id"`
		Username string `json:"username"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON")
		return
	}
	var targetUserID int64
	if req.Username != "" {
		targetUserID, err = h.rooms.ResolveUserID(r.Context(), req.Username)
		if err == rooms.ErrUserNotFound {
			writeError(w, http.StatusNotFound, "not_found", "User not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
			return
		}
	} else if req.UserID > 0 {
		targetUserID = req.UserID
	} else {
		writeError(w, http.StatusBadRequest, "invalid_request", "user_id or username required")
		return
	}
	userID := getUserID(r.Context())
	err = h.rooms.TransferCreator(r.Context(), roomID, userID, targetUserID)
	if err == rooms.ErrForbidden {
		writeError(w, http.StatusForbidden, "forbidden", "Not allowed")
		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"status": "transferred"})
}

func (h *roomsHandler) removeMember(w http.ResponseWriter, r *http.Request) {
	roomIDStr := chi.URLParam(r, "roomID")
	roomID, err := uuid.Parse(roomIDStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid room ID")
		return
	}
	var req struct {
		UserID   int64  `json:"user_id"`
		Username string `json:"username"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON")
		return
	}
	var targetUserID int64
	if req.Username != "" {
		targetUserID, err = h.rooms.ResolveUserID(r.Context(), req.Username)
		if err == rooms.ErrUserNotFound {
			writeError(w, http.StatusNotFound, "not_found", "User not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
			return
		}
	} else if req.UserID > 0 {
		targetUserID = req.UserID
	} else {
		writeError(w, http.StatusBadRequest, "invalid_request", "user_id or username required")
		return
	}
	userID := getUserID(r.Context())
	err = h.rooms.Remove(r.Context(), roomID, userID, targetUserID)
	if err == rooms.ErrForbidden {
		writeError(w, http.StatusForbidden, "forbidden", "Not allowed")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"status": "removed"})
}
