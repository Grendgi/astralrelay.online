package api

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/messenger/server/internal/auth"
	"github.com/messenger/server/internal/db"
	"github.com/messenger/server/internal/federation"
	"github.com/messenger/server/internal/keydir"
	"github.com/messenger/server/internal/logjson"
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
	db              *db.DB
	auth            *auth.Service
}

// authVerifyRequest is the body for federation auth/verify (login on home server from another server).
type authVerifyRequest struct {
	UserID            string `json:"user_id"`
	Password          string `json:"password"`
	DeviceID          string `json:"device_id"`
	RequestKeysRestore bool  `json:"request_keys_restore"`
	Keys              *struct {
		IdentityKey  string `json:"identity_key"`
		SignedPrekey struct {
			Key       string `json:"key"`
			Signature string `json:"signature"`
			KeyID     int64  `json:"key_id"`
		} `json:"signed_prekey"`
		OneTimePrekeys []struct {
			Key   string `json:"key"`
			KeyID int64  `json:"key_id"`
		} `json:"one_time_prekeys"`
	} `json:"keys,omitempty"`
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

// federationServers returns GET /.well-known/federation-servers — list of known federation servers (self + federation_peers from DB).
func (h *federationHandler) federationServers(w http.ResponseWriter, r *http.Request) {
	servers := []string{strings.ToLower(h.domain)}
	if h.db != nil {
		rows, err := h.db.Pool.Query(r.Context(), `SELECT domain FROM federation_peers WHERE allowed = TRUE ORDER BY domain`)
		if err == nil {
			defer rows.Close()
			seen := map[string]struct{}{strings.ToLower(h.domain): {}}
			for rows.Next() {
				var d string
				if rows.Scan(&d) == nil {
					d = strings.ToLower(strings.TrimSpace(d))
					if d != "" && d != strings.ToLower(h.domain) {
						if _, ok := seen[d]; !ok {
							seen[d] = struct{}{}
							servers = append(servers, d)
						}
					}
				}
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"servers": servers})
}

// register handles POST /federation/v1/register — add self as federation peer (signed request).
func (h *federationHandler) register(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "POST required")
		return
	}
	origin := r.Header.Get("X-Server-Origin")
	if origin == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "X-Server-Origin required")
		return
	}
	origin = strings.ToLower(strings.TrimSpace(origin))
	if origin == strings.ToLower(h.domain) {
		writeError(w, http.StatusBadRequest, "invalid_request", "Cannot register self")
		return
	}
	body, _ := io.ReadAll(io.LimitReader(r.Body, int64(256)))
	originKey, err := federation.FetchServerKey(r.Context(), origin)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid_origin", "Cannot fetch origin server key")
		return
	}
	if err := federation.VerifyRequest(r, body, originKey); err != nil {
		writeError(w, http.StatusUnauthorized, "invalid_signature", err.Error())
		return
	}
	if h.db != nil {
		endpoint := "https://" + origin + "/federation/v1"
		_, _ = h.db.Pool.Exec(r.Context(),
			`INSERT INTO federation_peers (domain, endpoint, allowed, updated_at) VALUES ($1, $2, TRUE, NOW())
			 ON CONFLICT (domain) DO UPDATE SET endpoint = $2, allowed = TRUE, updated_at = NOW()`,
			origin, endpoint)
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *federationHandler) getKeys(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "userID")
	deviceID := chi.URLParam(r, "deviceID")
	if userID == "" || deviceID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "userID and deviceID required")
		return
	}
	if u, err := url.PathUnescape(userID); err == nil {
		userID = u
	}
	if d, err := url.PathUnescape(deviceID); err == nil {
		deviceID = d
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

// getDevices handles GET /federation/v1/keys/devices/{userID} — returns device list for a local user (for remote keys lookup).
func (h *federationHandler) getDevices(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "userID")
	if userID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "userID required")
		return
	}
	if u, err := url.PathUnescape(userID); err == nil {
		userID = u
	}
	// Only return devices for users on this server
	if idx := strings.Index(userID, ":"); idx >= 0 {
		if domain := strings.ToLower(strings.TrimSpace(userID[idx+1:])); domain != strings.ToLower(h.domain) {
			writeError(w, http.StatusForbidden, "wrong_domain", "User not on this server")
			return
		}
	}
	deviceIDs, err := h.keydir.ListDevicesForUser(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "User not found")
		return
	}
	out := make([]map[string]interface{}, 0, len(deviceIDs))
	for _, id := range deviceIDs {
		d := map[string]interface{}{"device_id": id, "status": "active"}
		if dev, err := uuid.Parse(id); err == nil {
			d["signal_device_id"] = keydir.UUIDToSignalDeviceID(dev)
		}
		out = append(out, d)
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"devices": out})
}

func (h *federationHandler) authVerify(w http.ResponseWriter, r *http.Request) {
	if h.auth == nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Auth not configured")
		return
	}
	origin := r.Header.Get("X-Server-Origin")
	if origin == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "X-Server-Origin required")
		return
	}
	maxBody := h.maxBody
	if maxBody <= 0 {
		maxBody = federation.MaxBodySize
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, int64(maxBody)))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid body")
		return
	}
	if h.security != nil && h.security.IsBlocked(origin) {
		writeError(w, http.StatusForbidden, "blocked", "Domain is blocked")
		return
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
	var req authVerifyRequest
	if json.Unmarshal(body, &req) != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON")
		return
	}
	req.UserID = strings.TrimSpace(req.UserID)
	if req.UserID == "" || !strings.HasPrefix(req.UserID, "@") {
		writeError(w, http.StatusBadRequest, "invalid_request", "user_id required (e.g. @user:domain)")
		return
	}
	parts := strings.SplitN(req.UserID[1:], ":", 2)
	if len(parts) != 2 {
		writeError(w, http.StatusBadRequest, "invalid_request", "user_id must be @username:domain")
		return
	}
	username, domain := strings.TrimSpace(parts[0]), strings.ToLower(strings.TrimSpace(parts[1]))
	if domain != h.domain {
		writeError(w, http.StatusForbidden, "wrong_domain", "user_id domain does not match this server")
		return
	}
	deviceID, err := uuid.Parse(req.DeviceID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid device_id")
		return
	}
	in := auth.LoginInput{
		Username:           username,
		Password:           req.Password,
		DeviceID:           deviceID,
		RequestKeysRestore: req.RequestKeysRestore,
	}
	if req.Keys != nil {
		identityKey, _ := base64.StdEncoding.DecodeString(req.Keys.IdentityKey)
		signedPrekey, _ := base64.StdEncoding.DecodeString(req.Keys.SignedPrekey.Key)
		signedPrekeySig, _ := base64.StdEncoding.DecodeString(req.Keys.SignedPrekey.Signature)
		prekeys := make([]auth.PrekeyItem, len(req.Keys.OneTimePrekeys))
		for i, pk := range req.Keys.OneTimePrekeys {
			key, _ := base64.StdEncoding.DecodeString(pk.Key)
			prekeys[i] = auth.PrekeyItem{Key: key, KeyID: pk.KeyID}
		}
		in.IdentityKey = identityKey
		in.SignedPrekey = signedPrekey
		in.SignedPrekeySig = signedPrekeySig
		in.SignedPrekeyID = req.Keys.SignedPrekey.KeyID
		in.OneTimePrekeys = prekeys
	}
	result, err := h.auth.Login(r.Context(), in)
	if err == auth.ErrInvalidCredentials {
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "Invalid credentials")
		return
	}
	if err != nil {
		logjson.Log("federation_auth_verify", map[string]interface{}{"error": err.Error(), "user_id": req.UserID})
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	out := map[string]interface{}{
		"access_token": result.Token,
		"expires_in":   86400,
		"user_id":      result.UserID,
		"device_id":    result.DeviceID.String(),
	}
	if result.KeysBackup != nil {
		out["keys_backup"] = map[string]interface{}{
			"salt": base64.StdEncoding.EncodeToString(result.KeysBackup.Salt),
			"blob": base64.StdEncoding.EncodeToString(result.KeysBackup.Blob),
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// usersLookup handles GET /federation/v1/users/lookup?username=... — returns user_id and home_domain if this server is the user's home (for federated login discovery).
func (h *federationHandler) usersLookup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "GET required")
		return
	}
	username := strings.TrimSpace(r.URL.Query().Get("username"))
	if username == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "username query required")
		return
	}
	origin := r.Header.Get("X-Server-Origin")
	if origin == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "X-Server-Origin required")
		return
	}
	if h.security != nil && h.security.IsBlocked(origin) {
		writeError(w, http.StatusForbidden, "blocked", "Domain is blocked")
		return
	}
	originKey, err := federation.FetchServerKey(r.Context(), origin)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid_origin", "Cannot fetch origin server key")
		return
	}
	if err := federation.VerifyRequest(r, nil, originKey); err != nil {
		writeError(w, http.StatusUnauthorized, "invalid_signature", err.Error())
		return
	}
	if h.db == nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "DB not configured")
		return
	}
	var id int64
	var domain string
	err = h.db.Pool.QueryRow(r.Context(),
		`SELECT id, domain FROM users WHERE username = $1 AND domain = $2`,
		username, h.domain,
	).Scan(&id, &domain)
	if err != nil || id == 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"not_found"}`))
		return
	}
	userID := "@" + username + ":" + domain
	writeJSON(w, http.StatusOK, map[string]string{
		"user_id":      userID,
		"home_domain":  domain,
	})
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
	// Record origin as federation peer for stats (known servers)
	if h.db != nil {
		originLower := strings.ToLower(strings.TrimSpace(origin))
		endpoint := "https://" + originLower + "/federation/v1"
		_, _ = h.db.Pool.Exec(r.Context(),
			`INSERT INTO federation_peers (domain, endpoint, allowed, updated_at) VALUES ($1, $2, TRUE, NOW())
			 ON CONFLICT (domain) DO UPDATE SET endpoint = $2, updated_at = NOW()`,
			originLower, endpoint)
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
