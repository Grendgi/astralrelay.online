package api

import (
	"net/http"

	"github.com/messenger/server/internal/db"
)

type statsHandler struct {
	db *db.DB
}

func (h *statsHandler) stats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var users, servers int
	if err := h.db.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM users`).Scan(&users); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get stats")
		return
	}
	if err := h.db.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM federation_peers`).Scan(&servers); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get stats")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"users":   users,
		"servers": servers,
	})
}
