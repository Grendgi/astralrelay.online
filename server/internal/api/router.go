package api

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/messenger/server/internal/logjson"
	"github.com/go-chi/cors"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/messenger/server/internal/auth"
	"github.com/messenger/server/internal/db"
	"github.com/messenger/server/internal/dbenc"
	"github.com/messenger/server/internal/federation"
	"github.com/messenger/server/internal/keydir"
	"github.com/messenger/server/internal/media"
	"github.com/messenger/server/internal/push"
	"github.com/messenger/server/internal/relay"
	"github.com/messenger/server/internal/rooms"
	"github.com/messenger/server/internal/stream"
	"github.com/messenger/server/internal/vpn"
)

func NewRouter(
	authSvc *auth.Service,
	keydirSvc *keydir.Service,
	relaySvc *relay.Service,
	roomsSvc *rooms.Service,
	mediaSvc *media.Service,
	fedClient *federation.Client,
	fedKeys *federation.ServerKeys,
	domain string,
	streamHub *stream.Hub,
	vpnSvc *vpn.Service,
	pushSvc *push.Service,
	database *db.DB,
	dbEnc *dbenc.Cipher,
	fedSecurity *federation.SecurityService,
	fedRateLimit int,
	fedMainOnlyDomain string,
	fedAlertWebhook string,
	e2eeStrictOnly bool,
) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)
	r.Use(anonymityLogger)
	r.Use(cors.Handler(corsForDomain(domain)))

	authH := &authHandler{auth: authSvc}
	keysH := &keysHandler{keydir: keydirSvc, fed: fedClient}
	relayH := &relayHandler{relay: relaySvc, rooms: roomsSvc, fed: fedClient, domain: domain, hub: streamHub, push: pushSvc, auth: authSvc, e2eeStrictOnly: e2eeStrictOnly}
	roomsH := &roomsHandler{rooms: roomsSvc}
	mediaH := &mediaHandler{media: mediaSvc}
	maxBody := 1048576
	if fedSecurity != nil {
		maxBody = fedSecurity.MaxBodySize()
	}
	fedH := &federationHandler{
		keydir:          keydirSvc,
		relay:           relaySvc,
		keys:            fedKeys,
		domain:          domain,
		hub:             streamHub,
		security:        fedSecurity,
		maxBody:         maxBody,
		fedClient:       fedClient,
		mainOnlyDomain:  fedMainOnlyDomain,
		alertWebhookURL: fedAlertWebhook,
	}
	streamH := &streamHandler{hub: streamHub, auth: authSvc, domain: domain}
	vpnH := &vpnHandler{vpn: vpnSvc, domain: domain}
	pushH := &pushHandler{push: pushSvc}
	backupH := &backupHandler{db: database, enc: dbEnc, auth: authSvc}

	r.Get("/.well-known/federation", fedH.wellKnown)
	r.Route("/federation/v1", func(r chi.Router) {
		r.Use(federationLogger)
		r.Use(federationRecover)
		r.Get("/keys/bundle/{userID}/{deviceID}", fedH.getKeys)
		limit := fedRateLimit
		if limit <= 0 {
			limit = 100
		}
		r.With(
			LimitByFederationDomain(limit, time.Minute, fedAlertWebhook),
			timeoutHandler(10*time.Second),
		).Post("/transaction", fedH.transaction)
	})

	r.Route("/api/v1", func(r chi.Router) {
		// Public (rate limited by IP)
		r.With(LimitByIP(5, time.Minute)).Post("/auth/register", authH.register)
		r.With(LimitByIP(10, time.Minute)).Post("/auth/login", authH.login)

		// Protected
		// WebSocket: ws_token only (no access_token in URL)
		r.Get("/messages/stream", streamH.serveWs)

		r.Group(func(r chi.Router) {
			r.Use(AuthMiddleware(authSvc))
			r.Post("/auth/ws-token", authH.wsToken)
			r.Post("/auth/logout", authH.logout)
			r.Get("/auth/devices", authH.listDevices)
			r.Post("/auth/devices/{deviceID}/revoke", authH.revokeDevice)
			r.With(LimitByUser(60, time.Minute)).Get("/auth/keys/status", keysH.keysStatus)
			r.With(LimitByUser(20, time.Minute)).Put("/auth/keys", keysH.updateKeys)
			r.With(LimitByUser(120, time.Minute)).Get("/keys/bundle/{userID}", keysH.getBundleForUser)
			r.With(LimitByUser(120, time.Minute)).Get("/keys/bundle/{userID}/{deviceID}", keysH.getBundle)
			r.With(LimitByUser(60, time.Minute)).Get("/keys/devices/{userID}", keysH.listDevicesForUser)
			r.With(LimitByUser(60, time.Minute)).Post("/messages/send", relayH.send)
			r.Post("/messages/typing", relayH.typing)
			r.Post("/messages/read", relayH.read)
			r.Get("/messages/sync", relayH.sync)
			r.Post("/rooms", roomsH.create)
			r.Get("/rooms", roomsH.list)
			r.Get("/rooms/{roomID}", roomsH.get)
			r.Post("/rooms/{roomID}/invite", roomsH.invite)
			r.Post("/rooms/{roomID}/leave", roomsH.leave)
			r.Post("/rooms/{roomID}/transfer", roomsH.transferCreator)
			r.Post("/rooms/{roomID}/remove", roomsH.removeMember)
			r.Get("/rooms/{roomID}/members", roomsH.members)
			r.Post("/media/upload", mediaH.upload)
			r.Get("/media/*", mediaH.download)
			r.Get("/vpn/protocols", vpnH.listProtocols)
			r.Get("/vpn/nodes", vpnH.listNodes)
			r.Get("/vpn/my-configs", vpnH.myConfigs)
			r.Post("/vpn/revoke", vpnH.revoke)
			r.Get("/vpn/config/{protocol}", vpnH.getConfig)
			r.With(LimitByUser(10, time.Minute)).Post("/backup/prepare", backupH.prepare)
			r.With(LimitByUser(30, time.Minute)).Get("/backup/salt", backupH.getSalt)
			r.With(LimitByUser(10, time.Minute)).Post("/keys/sync", backupH.syncKeys)
			r.Get("/push/vapid-public", pushH.vapidPublic)
			r.Post("/push/subscribe", pushH.subscribe)
			r.Post("/push/unsubscribe", pushH.unsubscribe)
		})
	})

	// Health
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	})

	// Metrics (Prometheus)
	r.Handle("/metrics", promhttp.Handler())

	return r
}

// anonymityLogger logs method, path, status, latency — без IP и user_id (анонимность).
func anonymityLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)
		logjson.Log("api", map[string]interface{}{
			"method": r.Method, "path": r.URL.Path, "status": ww.Status(), "duration_ms": time.Since(start).Milliseconds(),
		})
	})
}

func timeoutHandler(d time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.TimeoutHandler(next, d, "request timeout")
	}
}

func corsForDomain(domain string) cors.Options {
	origins := []string{"http://localhost:3000", "http://127.0.0.1:3000"}
	if domain != "" && domain != "localhost" {
		origins = append(origins, "http://"+domain, "https://"+domain)
	}
	return cors.Options{
		AllowedOrigins:   origins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Protocol-Version", "X-Idempotency-Key"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	}
}
