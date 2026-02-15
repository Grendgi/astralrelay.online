package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/messenger/server/internal/api"
	"github.com/messenger/server/internal/logjson"
	"github.com/messenger/server/internal/auth"
	"github.com/messenger/server/internal/config"
	"github.com/messenger/server/internal/db"
	"github.com/messenger/server/internal/dbenc"
	"github.com/messenger/server/internal/federation"
	"github.com/messenger/server/internal/keydir"
	"github.com/messenger/server/internal/stream"
	"github.com/messenger/server/internal/media"
	"github.com/messenger/server/internal/push"
	"github.com/messenger/server/internal/relay"
	"github.com/messenger/server/internal/rooms"
	"github.com/messenger/server/internal/vpn"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	ctx := context.Background()
	database, err := db.New(ctx, cfg.Database.URL)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer database.Close()

	if err := db.Migrate(cfg.Database.URL); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	var federationDB *db.DB
	if cfg.Database.FederationURL != "" {
		federationDB, err = db.New(ctx, cfg.Database.FederationURL)
		if err != nil {
			logjson.Log("server", map[string]interface{}{"error": err.Error(), "fallback": "main pool"})
			federationDB = nil
		}
	}
	if federationDB != nil {
		defer federationDB.Close()
	}

	dbEnc, err := dbenc.New(cfg.Database.EncryptionKey)
	if err != nil {
		logjson.Log("server", map[string]interface{}{"error": err.Error(), "fallback": "no DB encryption"})
		dbEnc = nil
	}
	if dbEnc != nil {
		if err := dbenc.EncryptExisting(ctx, database.Pool, dbEnc); err != nil {
			log.Fatalf("encrypt existing data: %v", err)
		}
	}

	// JWT secret: fail-fast in production if missing
	jwtSecretStr := os.Getenv("JWT_SECRET")
	env := os.Getenv("ENV")
	if env == "" {
		env = os.Getenv("APP_ENV")
	}
	isProd := env == "production" || env == "prod"
	if isProd && (jwtSecretStr == "" || len(jwtSecretStr) < 16) {
		log.Fatalf("JWT_SECRET required and must be at least 16 bytes in production (ENV=%s)", env)
	}
	jwtSecret := []byte(jwtSecretStr)
	if len(jwtSecret) == 0 {
		jwtSecret = []byte("dev-secret-change-in-production")
	}

	authSvc := auth.New(database, cfg.Server.Domain, jwtSecret, dbEnc)
	keydirSvc := keydir.New(database, cfg.Server.Domain, dbEnc)
	roomsSvc := rooms.New(database, cfg.Server.Domain)
	relaySvc := relay.New(database, cfg.Server.Domain, roomsSvc)
	if federationDB != nil {
		relaySvc.SetFederationPool(federationDB.Pool)
	}
	mediaSvc, err := media.New(&cfg.S3)
	if err != nil {
		log.Fatalf("media: %v", err)
	}

	fedKeys, err := federation.LoadServerKeys(&cfg.Server)
	if err != nil {
		log.Fatalf("federation keys: %v", err)
	}
	var fedClient *federation.Client
	mtlsCert := cfg.Federation.MTLS.ClientCert
	mtlsKey := cfg.Federation.MTLS.ClientKey
	mainOnly := cfg.Federation.Mode == "main_only" && cfg.Federation.MainDomain != ""
	if mainOnly && mtlsCert != "" && mtlsKey != "" {
		fedClient = federation.NewClientWithMainAndMTLS(cfg.Server.Domain, cfg.Federation.MainDomain, fedKeys, mtlsCert, mtlsKey)
	} else if mainOnly {
		fedClient = federation.NewClientWithMain(cfg.Server.Domain, cfg.Federation.MainDomain, fedKeys)
	} else if mtlsCert != "" && mtlsKey != "" {
		fedClient = federation.NewClientWithMTLS(cfg.Server.Domain, fedKeys, mtlsCert, mtlsKey)
	} else {
		fedClient = federation.NewClient(cfg.Server.Domain, fedKeys)
	}
	fedMainOnlyDomain := ""
	if cfg.Federation.Mode == "main_only" && cfg.Federation.MainDomain != "" {
		fedMainOnlyDomain = cfg.Federation.MainDomain
	}

	var streamHub *stream.Hub
	if cfg.Redis.Disabled || cfg.Redis.URL == "" {
		streamHub = stream.NewHub()
	} else {
		streamHub = stream.NewHubWithRedis(cfg.Redis.URL)
	}
	go streamHub.Run()
	defer streamHub.Close()

	vpnSvc := vpn.New(&cfg.VPN, database)
	if err := vpnSvc.Nodes().SeedFromJSON(ctx, cfg.VPN.NodesJSON); err != nil {
		logjson.Log("server", map[string]interface{}{"error": err.Error(), "op": "vpn_nodes_seed"})
	}

	pushSvc := push.New(cfg.Push.VAPIDPublicKey, cfg.Push.VAPIDPrivateKey, database)

	fedSecCfg := federation.SecurityConfig{
		RateLimit:               cfg.Federation.Security.RateLimit,
		MaxBodySize:             cfg.Federation.Security.MaxBodySize,
		AllowlistMode:           cfg.Federation.Security.AllowlistMode,
		AllowlistPath:           cfg.Federation.Security.AllowlistPath,
		AllowlistTrustThreshold: cfg.Federation.Security.AllowlistTrustThreshold,
		BlocklistPath:           cfg.Federation.Security.BlocklistPath,
		BlocklistURL:            cfg.Federation.Security.BlocklistURL,
		BlocklistReload:         time.Duration(cfg.Federation.Security.BlocklistReload) * time.Hour,
	}
	if fedSecCfg.MaxBodySize <= 0 {
		fedSecCfg.MaxBodySize = federation.MaxBodySize
	}
	if fedSecCfg.AllowlistMode == "" {
		fedSecCfg.AllowlistMode = "auto"
	}
	fedSecurity := federation.NewSecurityService(fedSecCfg)

	router := api.NewRouter(authSvc, keydirSvc, relaySvc, roomsSvc, mediaSvc, fedClient, fedKeys, cfg.Server.Domain, streamHub, vpnSvc, pushSvc, database, dbEnc, fedSecurity, cfg.Federation.Security.RateLimit, fedMainOnlyDomain, cfg.Federation.Security.AlertWebhookURL, cfg.Server.E2EEStrictOnly)

	redisURL := ""
	if !cfg.Redis.Disabled && cfg.Redis.URL != "" {
		redisURL = cfg.Redis.URL
	}
	go api.StartLatencyProbe(context.Background(), database.Pool, redisURL)

	// Periodic cleanup: tokens + consumed OTPK (every 6h + once at startup)
	go func() {
		run := func() {
			if n, err := authSvc.CleanupExpiredTokens(ctx); err != nil {
				logjson.Log("server", map[string]interface{}{"op": "cleanup_tokens", "error": err.Error()})
			} else if n > 0 {
				logjson.Log("server", map[string]interface{}{"op": "cleanup_tokens", "deleted": n})
			}
			if n, err := keydirSvc.CleanupOldConsumedPrekeys(ctx); err != nil {
				logjson.Log("server", map[string]interface{}{"op": "cleanup_otpk", "error": err.Error()})
			} else if n > 0 {
				logjson.Log("server", map[string]interface{}{"op": "cleanup_otpk", "deleted": n})
			}
		}
		run() // once at startup
		ticker := time.NewTicker(6 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			run()
		}
	}()

	port := cfg.Server.Port
	if port <= 0 {
		port = 8080
	}
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: router,
	}

	go func() {
		logjson.Log("server", map[string]interface{}{"listen": srv.Addr})
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	logjson.Log("server", map[string]interface{}{"event": "shutting down"})
	if err := srv.Shutdown(context.Background()); err != nil {
		logjson.Log("server", map[string]interface{}{"error": err.Error(), "op": "shutdown"})
	}
}
