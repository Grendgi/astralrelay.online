package api

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/messenger/server/internal/auth"
	"github.com/messenger/server/internal/logjson"
)

type registerRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	DeviceID string `json:"device_id"`
	Keys     struct {
		IdentityKey   string `json:"identity_key"`
		SignedPrekey  struct {
			Key       string `json:"key"`
			Signature string `json:"signature"`
			KeyID     int64  `json:"key_id"`
		} `json:"signed_prekey"`
		OneTimePrekeys []struct {
			Key   string `json:"key"`
			KeyID int64  `json:"key_id"`
		} `json:"one_time_prekeys"`
	} `json:"keys"`
	KeysBackup *struct {
		SaltB64 string `json:"salt"`
		BlobB64 string `json:"blob"`
	} `json:"keys_backup,omitempty"`
}

type loginRequest struct {
	Username           string `json:"username"`
	Password           string `json:"password"`
	DeviceID           string `json:"device_id"`
	Domain             string `json:"domain,omitempty"` // optional: home server domain for federated login
	RequestKeysRestore bool   `json:"request_keys_restore"`
	Keys               *struct {
		IdentityKey   string `json:"identity_key"`
		SignedPrekey  struct {
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

func (h *authHandler) register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON")
		return
	}

	deviceID, err := uuid.Parse(req.DeviceID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid device_id")
		return
	}

	identityKey, _ := base64.StdEncoding.DecodeString(req.Keys.IdentityKey)
	signedPrekey, _ := base64.StdEncoding.DecodeString(req.Keys.SignedPrekey.Key)
	signedPrekeySig, _ := base64.StdEncoding.DecodeString(req.Keys.SignedPrekey.Signature)

	prekeys := make([]auth.PrekeyItem, len(req.Keys.OneTimePrekeys))
	for i, pk := range req.Keys.OneTimePrekeys {
		key, _ := base64.StdEncoding.DecodeString(pk.Key)
		prekeys[i] = auth.PrekeyItem{Key: key, KeyID: pk.KeyID}
	}

	if len(req.Password) < 6 {
		writeError(w, http.StatusBadRequest, "invalid_request", "Password must be at least 6 characters")
		return
	}

	in := auth.RegisterInput{
		Username:        req.Username,
		Password:        req.Password,
		DeviceID:        deviceID,
		IdentityKey:     identityKey,
		SignedPrekey:    signedPrekey,
		SignedPrekeySig: signedPrekeySig,
		SignedPrekeyID:  req.Keys.SignedPrekey.KeyID,
		OneTimePrekeys:  prekeys,
	}
	if req.KeysBackup != nil && req.KeysBackup.SaltB64 != "" && req.KeysBackup.BlobB64 != "" {
		in.KeysBackupSalt, _ = base64.StdEncoding.DecodeString(req.KeysBackup.SaltB64)
		in.KeysBackupBlob, _ = base64.StdEncoding.DecodeString(req.KeysBackup.BlobB64)
	}

	userID, devID, token, err := h.auth.Register(r.Context(), in)
	if err == auth.ErrUsernameTaken {
		writeError(w, http.StatusBadRequest, "username_taken", "Username already taken")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"user_id":       userID,
		"device_id":     devID.String(),
		"access_token":  token,
		"expires_in":    86400,
	})
}

func (h *authHandler) login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON")
		return
	}

	username := strings.TrimSpace(req.Username)
	domain := strings.ToLower(strings.TrimSpace(req.Domain))
	if strings.Contains(username, "@") {
		parts := strings.SplitN(username, "@", 2)
		if len(parts) == 2 {
			username = strings.TrimSpace(parts[0])
			if domain == "" {
				domain = strings.ToLower(strings.TrimSpace(parts[1]))
			}
		}
	}
	if domain == "" {
		domain = h.domain
	}

	deviceID, err := uuid.Parse(req.DeviceID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid device_id")
		return
	}

	// Federated login: user's home server is another domain
	if domain != h.domain && h.fedClient != nil {
		userID := "@" + username + ":" + domain
		body := map[string]interface{}{
			"user_id":             userID,
			"password":            req.Password,
			"device_id":           req.DeviceID,
			"request_keys_restore": req.RequestKeysRestore,
		}
		if req.Keys != nil {
			body["keys"] = req.Keys
		}
		bodyBytes, _ := json.Marshal(body)
		fedRes, err := h.fedClient.AuthVerify(r.Context(), domain, bodyBytes)
		if err != nil {
			logjson.Log("federation_login", map[string]interface{}{"error": err.Error(), "domain": domain})
			writeError(w, http.StatusUnauthorized, "invalid_credentials", "Invalid credentials or federation error")
			return
		}
		localToken, err := h.auth.CreateProxySession(r.Context(), domain, fedRes.AccessToken, fedRes.UserID, fedRes.DeviceID, 24*time.Hour)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
			return
		}
		out := map[string]interface{}{
			"user_id":      fedRes.UserID,
			"device_id":    fedRes.DeviceID,
			"access_token": localToken,
			"expires_in":   86400,
		}
		if fedRes.KeysBackup != nil {
			out["keys_backup"] = fedRes.KeysBackup
		}
		writeJSON(w, http.StatusOK, out)
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

	res, err := h.auth.Login(r.Context(), in)
	if err == nil {
		out := map[string]interface{}{
			"user_id":      res.UserID,
			"device_id":    res.DeviceID.String(),
			"access_token": res.Token,
			"expires_in":   86400,
		}
		if res.KeysBackup != nil {
			out["keys_backup"] = map[string]string{
				"salt": base64.StdEncoding.EncodeToString(res.KeysBackup.Salt),
				"blob": base64.StdEncoding.EncodeToString(res.KeysBackup.Blob),
			}
		}
		writeJSON(w, http.StatusOK, out)
		return
	}
	if err != auth.ErrInvalidCredentials {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	// Local user not found or wrong password — try to discover home server via federation
	if h.fedClient != nil && h.db != nil {
		rows, qErr := h.db.Pool.Query(r.Context(), `SELECT domain FROM federation_peers WHERE allowed = TRUE AND domain != $1`, h.domain)
		if qErr == nil {
			defer rows.Close()
			for rows.Next() {
				var peerDomain string
				if rows.Scan(&peerDomain) == nil {
					if _, homeDomain, found := h.fedClient.UserLookup(r.Context(), peerDomain, username); found {
						domain = homeDomain
						break
					}
				}
			}
		}
		if domain != h.domain {
			userID := "@" + username + ":" + domain
			body := map[string]interface{}{
				"user_id":             userID,
				"password":            req.Password,
				"device_id":           req.DeviceID,
				"request_keys_restore": req.RequestKeysRestore,
			}
			if req.Keys != nil {
				body["keys"] = req.Keys
			}
			bodyBytes, _ := json.Marshal(body)
			fedRes, fedErr := h.fedClient.AuthVerify(r.Context(), domain, bodyBytes)
			if fedErr == nil {
				localToken, createErr := h.auth.CreateProxySession(r.Context(), domain, fedRes.AccessToken, fedRes.UserID, fedRes.DeviceID, 24*time.Hour)
				if createErr == nil {
					out := map[string]interface{}{
						"user_id":      fedRes.UserID,
						"device_id":    fedRes.DeviceID,
						"access_token": localToken,
						"expires_in":   86400,
					}
					if fedRes.KeysBackup != nil {
						out["keys_backup"] = fedRes.KeysBackup
					}
					writeJSON(w, http.StatusOK, out)
					return
				}
			}
		}
	}

	writeError(w, http.StatusUnauthorized, "invalid_credentials", "Invalid credentials")
}

func (h *authHandler) wsToken(w http.ResponseWriter, r *http.Request) {
	userID := getUserID(r.Context())
	deviceIDStr := getDeviceID(r.Context())
	if userID == 0 || deviceIDStr == "" {
		writeError(w, http.StatusUnauthorized, "missing_context", "Auth context required")
		return
	}
	deviceID, err := uuid.Parse(deviceIDStr)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Invalid device")
		return
	}
	wsToken, err := h.auth.IssueWSToken(userID, deviceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ws_token":   wsToken,
		"expires_in": auth.WSTokenExpirySec(),
	})
}

func (h *authHandler) listDevices(w http.ResponseWriter, r *http.Request) {
	userID := getUserID(r.Context())
	deviceIDStr := getDeviceID(r.Context())
	if userID == 0 || deviceIDStr == "" {
		writeError(w, http.StatusUnauthorized, "missing_context", "Auth required")
		return
	}
	deviceID, err := uuid.Parse(deviceIDStr)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Invalid device")
		return
	}
	devices, err := h.auth.ListDevices(r.Context(), userID, deviceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	type dev struct {
		DeviceID  string `json:"device_id"`
		Name      string `json:"name,omitempty"`
		CreatedAt string `json:"created_at"`
		IsCurrent bool   `json:"is_current"`
	}
	out := make([]dev, len(devices))
	for i, d := range devices {
		out[i] = dev{DeviceID: d.DeviceID, Name: d.Name, CreatedAt: d.CreatedAt.Format("2006-01-02 15:04"), IsCurrent: d.IsCurrent}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"devices": out})
}

func (h *authHandler) revokeDevice(w http.ResponseWriter, r *http.Request) {
	userID := getUserID(r.Context())
	deviceIDStr := chi.URLParam(r, "deviceID")
	if userID == 0 || deviceIDStr == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "deviceID required")
		return
	}
	deviceID, err := uuid.Parse(deviceIDStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid device_id")
		return
	}
	if err := h.auth.RevokeDevice(r.Context(), userID, deviceID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	logjson.Log("audit", map[string]interface{}{"action": "device_revoke", "user_id": userID, "device_id": deviceID.String()})
	writeJSON(w, http.StatusOK, map[string]interface{}{"status": "ok"})
}

func (h *authHandler) renameDevice(w http.ResponseWriter, r *http.Request) {
	userID := getUserID(r.Context())
	deviceIDStr := chi.URLParam(r, "deviceID")
	if userID == 0 || deviceIDStr == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "deviceID required")
		return
	}
	deviceID, err := uuid.Parse(deviceIDStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid device_id")
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON")
		return
	}
	if err := h.auth.RenameDevice(r.Context(), userID, deviceID, req.Name); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	logjson.Log("audit", map[string]interface{}{"action": "device_rename", "user_id": userID, "device_id": deviceID.String()})
	writeJSON(w, http.StatusOK, map[string]interface{}{"status": "ok"})
}

func (h *authHandler) logout(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	token = strings.TrimSpace(token)
	if token == "" {
		writeError(w, http.StatusUnauthorized, "missing_token", "Authorization required")
		return
	}
	err := h.auth.RevokeToken(r.Context(), token)
	if err == auth.ErrInvalidToken {
		writeError(w, http.StatusUnauthorized, "invalid_token", "Invalid or expired token")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"status": "ok"})
}
