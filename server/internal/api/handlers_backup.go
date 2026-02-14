package api

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"

	"github.com/messenger/server/internal/auth"
	"github.com/messenger/server/internal/db"
	"github.com/messenger/server/internal/dbenc"
)

type backupHandler struct {
	db   *db.DB
	enc  *dbenc.Cipher
	auth *auth.Service
}

func (h *backupHandler) prepare(w http.ResponseWriter, r *http.Request) {
	userID := getUserID(r.Context())
	if userID == 0 {
		writeError(w, http.StatusUnauthorized, "unauthorized", "auth required")
		return
	}

	salt := make([]byte, 32)
	if _, err := rand.Read(salt); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to generate salt")
		return
	}
	saltStored := salt
	if h.enc != nil {
		var err error
		saltStored, err = h.enc.Encrypt(salt)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to encrypt salt")
			return
		}
	}

	_, err := h.db.Pool.Exec(r.Context(),
		`INSERT INTO backup_salts (user_id, salt) VALUES ($1, $2)
		 ON CONFLICT (user_id) DO UPDATE SET salt = $2`,
		userID, saltStored,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"salt": base64.StdEncoding.EncodeToString(salt)})
}

func (h *backupHandler) syncKeys(w http.ResponseWriter, r *http.Request) {
	userID := getUserID(r.Context())
	if userID == 0 {
		writeError(w, http.StatusUnauthorized, "unauthorized", "auth required")
		return
	}
	var req struct {
		Salt string `json:"salt"`
		Blob string `json:"blob"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON")
		return
	}
	if req.Salt == "" || req.Blob == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "salt and blob required")
		return
	}
	salt, err := base64.StdEncoding.DecodeString(req.Salt)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid salt base64")
		return
	}
	blob, err := base64.StdEncoding.DecodeString(req.Blob)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid blob base64")
		return
	}
	if err := h.auth.StoreKeyBackup(r.Context(), userID, salt, blob); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *backupHandler) getSalt(w http.ResponseWriter, r *http.Request) {
	userID := getUserID(r.Context())
	if userID == 0 {
		writeError(w, http.StatusUnauthorized, "unauthorized", "auth required")
		return
	}

	var salt []byte
	err := h.db.Pool.QueryRow(r.Context(),
		`SELECT salt FROM backup_salts WHERE user_id = $1`,
		userID,
	).Scan(&salt)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "no backup salt for this device")
		return
	}
	if h.enc != nil {
		salt, err = h.enc.Decrypt(salt)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to decrypt salt")
			return
		}
	}

	writeJSON(w, http.StatusOK, map[string]string{"salt": base64.StdEncoding.EncodeToString(salt)})
}
