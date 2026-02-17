package api

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/messenger/server/internal/db"
	"github.com/messenger/server/internal/federation"
)

const serversCacheTTL = 1 * time.Hour

var (
	serversCacheMu   sync.RWMutex
	serversCacheHub  string
	serversCacheList []string
	serversCacheExp  time.Time
)

// getFederationPeers returns merged list of federation peer domains: FEDERATION_PEERS + DB federation_peers + fetched from discovery hub.
func getFederationPeers(ctx context.Context, database *db.DB, fedClient *federation.Client, fedPeers []string, discoveryHub, myDomain string) map[string]struct{} {
	peers := make(map[string]struct{})
	myLower := strings.ToLower(strings.TrimSpace(myDomain))
	for _, p := range fedPeers {
		d := strings.ToLower(strings.TrimSpace(p))
		if d != "" && d != myLower {
			peers[d] = struct{}{}
		}
	}
	if database != nil {
		rows, err := database.Pool.Query(ctx, `SELECT domain FROM federation_peers WHERE allowed = TRUE AND domain != $1`, myDomain)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var d string
				if rows.Scan(&d) == nil {
					d = strings.ToLower(strings.TrimSpace(d))
					if d != "" && d != myLower {
						peers[d] = struct{}{}
					}
				}
			}
		}
	}
	hub := strings.ToLower(strings.TrimSpace(discoveryHub))
	if hub != "" && hub != myLower && fedClient != nil {
		serversCacheMu.Lock()
		if serversCacheHub == hub && time.Now().Before(serversCacheExp) {
			for _, s := range serversCacheList {
				if s != myLower {
					peers[s] = struct{}{}
				}
			}
			serversCacheMu.Unlock()
		} else {
			serversCacheMu.Unlock()
			list, err := fedClient.FetchServersList(ctx, hub)
			if err == nil {
				serversCacheMu.Lock()
				serversCacheHub = hub
				serversCacheList = list
				serversCacheExp = time.Now().Add(serversCacheTTL)
				serversCacheMu.Unlock()
				for _, s := range list {
					if s != myLower {
						peers[s] = struct{}{}
					}
				}
			}
		}
	}
	return peers
}
