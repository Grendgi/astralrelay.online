package api

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/messenger/server/internal/federation"
	"github.com/messenger/server/internal/keydir"
	"github.com/messenger/server/internal/logjson"
	"github.com/messenger/server/internal/push"
	"github.com/messenger/server/internal/relay"
	"github.com/messenger/server/internal/rooms"
	"github.com/messenger/server/internal/stream"
	"strings"
)

func extractDomain(addr string) string {
	if idx := strings.Index(addr, ":"); idx >= 0 && idx+1 < len(addr) {
		return addr[idx+1:]
	}
	return ""
}

type keysHandler struct {
	keydir *keydir.Service
	fed    *federation.Client
}

type relayHandler struct {
	relay          *relay.Service
	rooms          *rooms.Service
	fed            *federation.Client
	domain         string
	hub            *stream.Hub
	push           *push.Service
	auth           AuthService
	e2eeStrictOnly bool // reject non-Signal (MVP/plain) messages when true
}

const signalPrefix = "sig1:"

func (h *keysHandler) getBundleForUser(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "userID")
	if userID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "userID required")
		return
	}
	if d, err := url.PathUnescape(userID); err == nil {
		userID = d
	}
	bundle, deviceID, err := h.keydir.GetBundleForUser(r.Context(), userID)
	if err == keydir.ErrNotFound || err == keydir.ErrInvalidUserID {
		logjson.Log("keys_get_bundle", map[string]interface{}{"user_id": userID, "error": err.Error()})
		writeError(w, http.StatusNotFound, "not_found", "Bundle not found")
		return
	}
	if err == keydir.ErrRemoteUser {
		writeError(w, http.StatusNotFound, "remote_user", "Use /keys/bundle/{userID}/{deviceID} for remote users")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	h.writeBundle(w, bundle, deviceID)
}

func (h *keysHandler) writeBundle(w http.ResponseWriter, bundle *keydir.Bundle, deviceID string) {
	resp := map[string]interface{}{
		"identity_key": base64.StdEncoding.EncodeToString(bundle.IdentityKey),
		"signed_prekey": map[string]interface{}{
			"key":       base64.StdEncoding.EncodeToString(bundle.SignedPrekey),
			"signature": base64.StdEncoding.EncodeToString(bundle.SignedPrekeySig),
			"key_id":    bundle.SignedPrekeyID,
		},
	}
	if deviceID != "" {
		resp["device_id"] = deviceID
		if dev, err := uuid.Parse(deviceID); err == nil {
			resp["signal_device_id"] = keydir.UUIDToSignalDeviceID(dev)
		}
	}
	if bundle.OneTimePrekey != nil {
		resp["one_time_prekey"] = map[string]interface{}{
			"key":   base64.StdEncoding.EncodeToString(bundle.OneTimePrekey.Key),
			"key_id": bundle.OneTimePrekey.KeyID,
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *keysHandler) updateKeys(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IdentitySigningKey string `json:"identity_signing_key"` // Ed25519 public, base64 — для проверки signed_prekey
		SignedPrekey       *struct {
			Key       string `json:"key"`
			Signature string `json:"signature"`
			KeyID     int64  `json:"key_id"`
		} `json:"signed_prekey"`
		OneTimePrekeys []struct {
			Key   string `json:"key"`
			KeyID int64  `json:"key_id"`
		} `json:"one_time_prekeys"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON")
		return
	}
	userID := getUserID(r.Context())
	deviceIDStr := getDeviceID(r.Context())
	deviceID, err := uuid.Parse(deviceIDStr)
	if err != nil || deviceID == uuid.Nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid device")
		return
	}
	var spk, spkSig []byte
	var spkID int64
	if req.SignedPrekey != nil {
		spk, _ = base64.StdEncoding.DecodeString(req.SignedPrekey.Key)
		spkSig, _ = base64.StdEncoding.DecodeString(req.SignedPrekey.Signature)
		spkID = req.SignedPrekey.KeyID
		if len(spk) == 0 || len(spkSig) == 0 {
			writeError(w, http.StatusBadRequest, "invalid_request", "signed_prekey required")
			return
		}
	}
	oneTimePrekeys := make([]keydir.PrekeyItem, 0, len(req.OneTimePrekeys))
	for _, pk := range req.OneTimePrekeys {
		keyBytes, _ := base64.StdEncoding.DecodeString(pk.Key)
		if len(keyBytes) > 0 {
			oneTimePrekeys = append(oneTimePrekeys, keydir.PrekeyItem{Key: keyBytes, KeyID: pk.KeyID})
		}
	}
	if req.SignedPrekey == nil && len(oneTimePrekeys) == 0 {
		writeError(w, http.StatusBadRequest, "invalid_request", "signed_prekey or one_time_prekeys required")
		return
	}
	var spkSlice, spkSigSlice []byte
	var identitySigningKey []byte
	if req.IdentitySigningKey != "" {
		identitySigningKey, _ = base64.StdEncoding.DecodeString(req.IdentitySigningKey)
	}
	if req.SignedPrekey != nil {
		spkSlice, spkSigSlice = spk, spkSig
	}
	err = h.keydir.UpdateKeys(r.Context(), userID, deviceID, spkSlice, spkSigSlice, spkID, oneTimePrekeys, identitySigningKey)
	if err == keydir.ErrInvalidSignature {
		writeError(w, http.StatusBadRequest, "invalid_request", "signed prekey signature invalid")
		return
	}
	if err == keydir.ErrPrekeyQuotaExceeded {
		writeError(w, http.StatusBadRequest, "invalid_request", "one-time prekeys quota exceeded (max 500 per device)")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	if req.SignedPrekey != nil {
		logjson.Log("audit", map[string]interface{}{"action": "signed_prekey_rotation", "user_id": userID, "device_id": deviceID.String()})
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{})
}

func (h *keysHandler) keysStatus(w http.ResponseWriter, r *http.Request) {
	deviceIDStr := getDeviceID(r.Context())
	deviceID, err := uuid.Parse(deviceIDStr)
	if err != nil || deviceID == uuid.Nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid device")
		return
	}
	st, err := h.keydir.GetKeyStatus(r.Context(), deviceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"unconsumed_prekeys":       st.UnconsumedPrekeys,
		"signed_prekey_updated_at": st.SignedPrekeyUpdatedAt,
		"next_one_time_key_id":     st.NextOneTimePrekeyKeyID,
	})
}

func (h *keysHandler) getBundle(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "userID")
	deviceID := chi.URLParam(r, "deviceID")
	if userID == "" || deviceID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "userID and deviceID required")
		return
	}
	if d, err := url.PathUnescape(userID); err == nil {
		userID = d
	}
	if d, err := url.PathUnescape(deviceID); err == nil {
		deviceID = d
	}

	bundle, err := h.keydir.GetBundle(r.Context(), userID, deviceID)
	if err == keydir.ErrNotFound || err == keydir.ErrInvalidUserID || err == keydir.ErrInvalidDeviceID {
		writeError(w, http.StatusNotFound, "not_found", "Bundle not found")
		return
	}
	if err == keydir.ErrRemoteUser && h.fed != nil {
		var username, domain string
		if _, scanErr := fmt.Sscanf(userID, "@%[^:]:%s", &username, &domain); scanErr == nil {
			data, fedErr := h.fed.FetchKeys(r.Context(), domain, userID, deviceID)
			if fedErr == nil {
				w.Header().Set("Content-Type", "application/json")
				w.Write(data)
				return
			}
		}
	}
	if err == keydir.ErrRemoteUser {
		writeError(w, http.StatusNotFound, "remote_user", "Could not fetch remote keys")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	h.writeBundle(w, bundle, "")
}

func (h *keysHandler) listDevicesForUser(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "userID")
	if userID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "userID required")
		return
	}
	if d, err := url.PathUnescape(userID); err == nil {
		userID = d
	}
	deviceIDs, err := h.keydir.ListDevicesForUser(r.Context(), userID)
	if err == keydir.ErrInvalidUserID {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid userID")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	devices := make([]map[string]interface{}, 0, len(deviceIDs))
	for _, id := range deviceIDs {
		d := map[string]interface{}{"device_id": id, "status": "active"}
		if dev, err := uuid.Parse(id); err == nil {
			d["signal_device_id"] = keydir.UUIDToSignalDeviceID(dev)
		}
		devices = append(devices, d)
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"devices": devices})
}

func (h *relayHandler) send(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Type      string `json:"type"`
		Sender    string `json:"sender"`
		Recipient string `json:"recipient"`
		DeviceID  string `json:"device_id"`
		Timestamp int64  `json:"timestamp"`
		Content   struct {
			Ciphertext  string            `json:"ciphertext"`
			Ciphertexts map[string]string `json:"ciphertexts"` // room E2EE: user_address -> base64
			SessionID   string            `json:"session_id"`
		} `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON")
		return
	}

	ciphertext, _ := base64.StdEncoding.DecodeString(req.Content.Ciphertext)
	idempotencyKey := r.Header.Get("X-Idempotency-Key")

	// Capability pinning: when E2EE_STRICT_ONLY, reject non-Signal messages
	if h.e2eeStrictOnly {
		if len(req.Content.Ciphertexts) > 0 {
			for _, ct := range req.Content.Ciphertexts {
				if ct != "" && !strings.HasPrefix(ct, signalPrefix) {
					writeError(w, http.StatusBadRequest, "strict_signal_only", "Server requires Signal Protocol only (no MVP/plain)")
					return
				}
			}
		} else if len(ciphertext) > 0 && !strings.HasPrefix(string(ciphertext), signalPrefix) {
			writeError(w, http.StatusBadRequest, "strict_signal_only", "Server requires Signal Protocol only (no MVP/plain)")
			return
		}
	}

	in := relay.SendInput{
		Sender:         req.Sender,
		Recipient:      req.Recipient,
		SenderUserID:   getUserID(r.Context()),
		SenderDevice:   req.DeviceID,
		Ciphertext:     ciphertext,
		Ciphertexts:    req.Content.Ciphertexts,
		SessionID:      req.Content.SessionID,
		Timestamp:      req.Timestamp,
		IdempotencyKey: idempotencyKey,
	}

	eventID, err := h.relay.Send(r.Context(), in)
	if err == relay.ErrNotRoomMember {
		writeError(w, http.StatusForbidden, "forbidden", "Not a room member")
		return
	}
	if err == relay.ErrRecipientNotFound {
		writeError(w, http.StatusNotFound, "not_found", "Recipient not found")
		return
	}
	if err == relay.ErrReplayTimestamp {
		writeError(w, http.StatusBadRequest, "replay_protection", "Message timestamp outside acceptable window")
		return
	}
	if err == relay.ErrRemoteRecipient && h.fed != nil {
		domain := extractDomain(req.Recipient)
		if domain != "" {
			ev := federation.TransactionEvent{
				EventID:   eventID,
				Type:      "m.room.encrypted",
				Sender:    req.Sender,
				Recipient: req.Recipient,
				Timestamp: req.Timestamp,
			}
			ev.Content.Ciphertext = base64.StdEncoding.EncodeToString(in.Ciphertext)
			ev.Content.SessionID = req.Content.SessionID
			txn := &federation.Transaction{
				TransactionID: "txn_" + uuid.New().String(),
				Events:        []federation.TransactionEvent{ev},
			}
			if err := h.fed.SendTransaction(r.Context(), domain, txn); err != nil {
				writeError(w, http.StatusBadGateway, "federation_failed", err.Error())
				return
			}
			if h.hub != nil {
				h.hub.Notify(req.Recipient, eventID)
			}
			writeJSON(w, http.StatusAccepted, map[string]interface{}{"event_id": eventID, "status": "queued"})
			return
		}
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	if h.hub != nil {
		h.hub.Notify(in.Recipient, eventID)
	}
	senderLabel := in.Sender
	if idx := strings.Index(senderLabel, ":"); idx > 0 {
		senderLabel = strings.TrimPrefix(senderLabel[:idx], "@")
	} else {
		senderLabel = strings.TrimPrefix(senderLabel, "@")
	}
	if senderLabel == "" {
		senderLabel = "Кто-то"
	}
	if h.push != nil && h.push.Enabled() {
		if strings.HasPrefix(in.Recipient, "!") && h.rooms != nil {
			if roomIDStr, _, ok := rooms.ParseRoomAddr(in.Recipient); ok {
				if roomID, err := uuid.Parse(roomIDStr); err == nil {
					if addrs, err := h.rooms.MemberAddresses(r.Context(), roomID); err == nil {
						for _, addr := range addrs {
							if addr != in.Sender {
								h.push.NotifyNewMessage(r.Context(), addr, senderLabel)
							}
						}
					}
				}
			}
		} else {
			h.push.NotifyNewMessage(r.Context(), in.Recipient, senderLabel)
		}
	}
	writeJSON(w, http.StatusAccepted, map[string]interface{}{
		"event_id": eventID,
		"status":   "queued",
	})
}

func (h *relayHandler) typing(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Recipient string `json:"recipient"`
		Typing    bool   `json:"typing"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON")
		return
	}
	if req.Recipient == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "recipient required")
		return
	}
	userID := getUserID(r.Context())
	username, err := h.auth.GetUsername(r.Context(), userID)
	if err != nil || username == "" {
		writeError(w, http.StatusUnauthorized, "invalid_request", "user not found")
		return
	}
	sender := "@" + username + ":" + h.domain
	if h.hub == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"status": "ok"})
		return
	}
	if rooms.IsRoomAddr(req.Recipient) && h.rooms != nil {
		roomIDStr, _, ok := rooms.ParseRoomAddr(req.Recipient)
		if !ok {
			writeError(w, http.StatusBadRequest, "invalid_request", "invalid room address")
			return
		}
		roomID, err := uuid.Parse(roomIDStr)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "invalid room id")
			return
		}
		members, err := h.rooms.Members(r.Context(), roomID, userID)
		if err != nil {
			writeError(w, http.StatusForbidden, "forbidden", "not a room member")
			return
		}
		for _, m := range members {
			addr := "@" + m.Username + ":" + m.Domain
			h.hub.NotifyTypingToMember(addr, sender, req.Typing, req.Recipient)
		}
	} else {
		h.hub.NotifyTyping(req.Recipient, sender, req.Typing)
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"status": "ok"})
}

func (h *relayHandler) read(w http.ResponseWriter, r *http.Request) {
	var req struct {
		EventIDs []string `json:"event_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON")
		return
	}
	if len(req.EventIDs) == 0 {
		writeJSON(w, http.StatusOK, map[string]interface{}{"status": "ok"})
		return
	}
	userID := getUserID(r.Context())
	username, err := h.auth.GetUsername(r.Context(), userID)
	if err != nil || username == "" {
		writeError(w, http.StatusUnauthorized, "invalid_request", "user not found")
		return
	}
	readerAddr := "@" + username + ":" + h.domain

	toNotify, err := h.relay.MarkRead(r.Context(), readerAddr, userID, req.EventIDs)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	if h.hub != nil {
		readAt := time.Now().Format(time.RFC3339Nano)
		for _, d := range toNotify {
			h.hub.NotifyRead(d.SenderAddr, d.EventID, readAt)
		}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"status": "ok"})
}

func (h *relayHandler) sync(w http.ResponseWriter, r *http.Request) {
	since := r.URL.Query().Get("since")
	limitStr := r.URL.Query().Get("limit")
	limit := 100
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}

	userID := getUserID(r.Context())
	deviceIDStr := getDeviceID(r.Context())
	deviceID, _ := uuid.Parse(deviceIDStr)

	events, nextCursor, delivered, err := h.relay.Sync(r.Context(), userID, deviceID, since, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	for _, d := range delivered {
		h.hub.NotifyDelivery(d.SenderAddr, d.EventID, "delivered")
	}

	// Convert to API response format
	type eventResp struct {
		EventID      string `json:"event_id"`
		Type         string `json:"type"`
		Sender       string `json:"sender"`
		Recipient    string `json:"recipient"`
		SenderDevice string `json:"sender_device,omitempty"`
		Timestamp    int64  `json:"timestamp"`
		Ciphertext   string `json:"ciphertext"`
		SessionID    string `json:"session_id"`
		Status       string `json:"status,omitempty"`
	}
	respEvents := make([]eventResp, len(events))
	for i, e := range events {
		respEvents[i] = eventResp{
			EventID:      e.EventID,
			Type:         e.Type,
			Sender:       e.Sender,
			Recipient:    e.Recipient,
			SenderDevice: e.SenderDevice,
			Timestamp:    e.Timestamp,
			Ciphertext:   base64.StdEncoding.EncodeToString(e.Ciphertext),
			SessionID:    e.SessionID,
			Status:       e.Status,
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"events":      respEvents,
		"next_cursor": nextCursor,
	})
}
