package api

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/messenger/server/internal/federation"
	"github.com/messenger/server/internal/logjson"
	"github.com/messenger/server/internal/keydir"
	"github.com/messenger/server/internal/relay"
	"github.com/messenger/server/internal/stream"
)

type federationHandler struct {
	keydir          *keydir.Service
	relay           *relay.Service
	keys            *federation.ServerKeys
	domain          string
	hub             *stream.Hub
	security        *federation.SecurityService
	maxBody         int
	fedClient       *federation.Client
	mainOnlyDomain  string
	alertWebhookURL string // optional: POST on rate limit / blocklist
}

func (h *federationHandler) wellKnown(w http.ResponseWriter, r *http.Request) {
	scheme := "https"
	if r.TLS == nil && strings.HasPrefix(r.Host, "localhost") {
		scheme = "http"
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"federation_endpoint": scheme + "://" + r.Host + "/federation/v1",
		"server_key":          h.keys.PublicKeyBase64(),
		"version":             "0.1",
	})
}

func (h *federationHandler) getKeys(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "userID")
	deviceID := chi.URLParam(r, "deviceID")
	if userID == "" || deviceID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "userID and deviceID required")
		return
	}
	bundle, err := h.keydir.GetBundle(r.Context(), userID, deviceID)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "Bundle not found")
		return
	}
	resp := map[string]interface{}{
		"identity_key": base64.StdEncoding.EncodeToString(bundle.IdentityKey),
		"signed_prekey": map[string]interface{}{
			"key":       base64.StdEncoding.EncodeToString(bundle.SignedPrekey),
			"signature": base64.StdEncoding.EncodeToString(bundle.SignedPrekeySig),
			"key_id":    bundle.SignedPrekeyID,
		},
	}
	if bundle.OneTimePrekey != nil {
		resp["one_time_prekey"] = map[string]interface{}{
			"key":   base64.StdEncoding.EncodeToString(bundle.OneTimePrekey.Key),
			"key_id": bundle.OneTimePrekey.KeyID,
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *federationHandler) transaction(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("X-Server-Origin")
	if origin == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "X-Server-Origin required")
		return
	}
	// main_only: only accept from Main
	if h.mainOnlyDomain != "" && origin != h.mainOnlyDomain {
		writeError(w, http.StatusForbidden, "main_only", "Only Main hub is allowed")
		return
	}
	// Body size limit
	maxBody := h.maxBody
	if maxBody <= 0 {
		maxBody = federation.MaxBodySize
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, int64(maxBody)))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid body")
		return
	}
	// Blocklist
	if h.security != nil && h.security.IsBlocked(origin) {
		recordFederationBlocklist(origin)
		SendFederationAlert(h.alertWebhookURL, "blocklist", origin)
		logjson.Log("federation_blocked", map[string]interface{}{"domain": origin})
		writeError(w, http.StatusForbidden, "blocked", "Domain is blocked")
		return
	}
	// Allowlist (manual mode blocks unknown domains)
	if h.security != nil {
		if err := h.security.CheckAllowlistForTransaction(origin); err != nil {
			writeError(w, http.StatusForbidden, "not_allowed", err.Error())
			return
		}
	}
	originKey, err := federation.FetchServerKey(r.Context(), origin)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid_origin", "Cannot fetch origin server key")
		return
	}
	if err := federation.VerifyRequest(r, body, originKey); err != nil {
		writeError(w, http.StatusUnauthorized, "invalid_signature", err.Error())
		return
	}
	var req federation.TransactionRequest
	if json.Unmarshal(body, &req) != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON")
		return
	}
	// Validation: schema and limits
	if h.security != nil {
		if err := h.security.ValidateTransaction(&req, time.Now().Unix()); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
	}
	if req.Destination != h.domain {
		// Relay: forward to destination (Main hub only)
		if h.fedClient != nil {
			txn := &federation.Transaction{
				TransactionID: req.TransactionID,
				Origin:        req.Origin,
				Destination:   req.Destination,
				Events:        make([]federation.TransactionEvent, len(req.Events)),
			}
			for i, e := range req.Events {
				txn.Events[i] = federation.TransactionEvent{
					EventID:   e.EventID,
					Type:      e.Type,
					Sender:    e.Sender,
					Recipient: e.Recipient,
					Timestamp: e.Timestamp,
				}
				txn.Events[i].Content.Ciphertext = e.Content.Ciphertext
				txn.Events[i].Content.SessionID = e.Content.SessionID
			}
			if err := h.fedClient.ForwardTransaction(r.Context(), req.Destination, txn); err != nil {
				writeError(w, http.StatusBadGateway, "relay_failed", err.Error())
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"accepted": []string{}, "relayed": true})
			return
		}
		writeError(w, http.StatusForbidden, "wrong_destination", "Transaction not for this server")
		return
	}
	events := make([]relay.FederatedEvent, len(req.Events))
	for i, e := range req.Events {
		ct, _ := base64.StdEncoding.DecodeString(e.Content.Ciphertext)
		events[i] = relay.FederatedEvent{
			EventID:      e.EventID,
			Type:         e.Type,
			Sender:       e.Sender,
			Recipient:    e.Recipient,
			SenderDevice: e.Sender, // S2S payload may omit; use sender as fallback
			Ciphertext:   ct,
			SessionID:    e.Content.SessionID,
			Timestamp:    e.Timestamp,
		}
	}
	// Auto allowlist: add domain after N successful transactions (trust threshold)
	if h.security != nil && !h.security.IsAllowed(origin) && h.security.RecordSuccessfulTransaction(origin) {
		_ = h.security.AddToAllowlist(origin)
	}
	accepted, rejected := h.relay.AcceptTransaction(r.Context(), req.TransactionID, events)
	if h.hub != nil {
		for i := range accepted {
			if i < len(events) {
				h.hub.Notify(events[i].Recipient, accepted[i])
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"accepted": accepted,
		"rejected": rejected,
	})
}
