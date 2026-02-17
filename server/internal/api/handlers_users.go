package api

import (
	"net/http"
	"strings"

	"github.com/messenger/server/internal/db"
	"github.com/messenger/server/internal/federation"
)

type usersHandler struct {
	db        *db.DB
	fedClient *federation.Client
	domain    string
	fedPeers  []string // FEDERATION_PEERS: bootstrap domains for search
}

// search handles GET /api/v1/users/search?q=... — returns users matching the query (local + federated peers). No domain needed.
func (h *usersHandler) search(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(q) == 0 {
		writeJSON(w, http.StatusOK, map[string]interface{}{"users": []map[string]string{}})
		return
	}
	if len(q) > 128 {
		q = q[:128]
	}
	seen := make(map[string]struct{})
	var users []map[string]string

	// Local users: prefix or exact match
	if h.db != nil {
		rows, err := h.db.Pool.Query(r.Context(),
			`SELECT username, domain FROM users WHERE domain = $1 AND (username = $2 OR username ILIKE $2 || '%') ORDER BY username LIMIT 20`,
			h.domain, q,
		)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var username, domain string
				if rows.Scan(&username, &domain) == nil {
					userID := "@" + username + ":" + domain
					if _, ok := seen[userID]; !ok {
						seen[userID] = struct{}{}
						users = append(users, map[string]string{"user_id": userID})
					}
				}
			}
		}
	}

	// Federated: DB peers + FEDERATION_PEERS (bootstrap)
	peers := make(map[string]struct{})
	for _, p := range h.fedPeers {
		if d := strings.ToLower(strings.TrimSpace(p)); d != "" && d != strings.ToLower(h.domain) {
			peers[d] = struct{}{}
		}
	}
	if h.db != nil {
		rows, err := h.db.Pool.Query(r.Context(),
			`SELECT domain FROM federation_peers WHERE allowed = TRUE AND domain != $1`,
			h.domain,
		)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var peer string
				if rows.Scan(&peer) == nil {
					peers[strings.ToLower(strings.TrimSpace(peer))] = struct{}{}
				}
			}
		}
	}
	for peer := range peers {
		if h.fedClient != nil {
			if userID, _, found := h.fedClient.UserLookup(r.Context(), peer, q); found && userID != "" {
				if _, ok := seen[userID]; !ok {
					seen[userID] = struct{}{}
					users = append(users, map[string]string{"user_id": userID})
				}
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"users": users})
}
