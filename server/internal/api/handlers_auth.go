package api

import (
	"encoding/base64"
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"github.com/messenger/server/internal/auth"
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
	Username            string `json:"username"`
	Password            string `json:"password"`
	DeviceID            string `json:"device_id"`
	RequestKeysRestore  bool   `json:"request_keys_restore"`
	Keys                *struct {
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

	deviceID, err := uuid.Parse(req.DeviceID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid device_id")
		return
	}

	in := auth.LoginInput{
		Username:           req.Username,
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
	if err == auth.ErrInvalidCredentials {
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "Invalid credentials")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

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
}
