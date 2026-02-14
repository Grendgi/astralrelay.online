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
			log.Printf("DATABASE_FEDERATION_URL: %v (using main pool)", err)
			federationDB = nil
		}
	}
	if federationDB != nil {
		defer federationDB.Close()
	}

	dbEnc, err := dbenc.New(cfg.Database.EncryptionKey)
	if err != nil {
		log.Printf("DB_ENCRYPTION_KEY invalid (%v), running without DB encryption", err)
		dbEnc = nil
	}
	if dbEnc != nil {
		if err := dbenc.EncryptExisting(ctx, database.Pool, dbEnc); err != nil {
			log.Fatalf("encrypt existing data: %v", err)
		}
	}

	// JWT secret - in prod use env var
	jwtSecret := []byte(os.Getenv("JWT_SECRET"))
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
		log.Printf("vpn nodes seed: %v", err)
	}

	pushSvc := push.New(cfg.Push.VAPIDPublicKey, cfg.Push.VAPIDPrivateKey, database)

	fedSecCfg := federation.SecurityConfig{
		RateLimit:       cfg.Federation.Security.RateLimit,
		MaxBodySize:     cfg.Federation.Security.MaxBodySize,
		AllowlistMode:   cfg.Federation.Security.AllowlistMode,
		AllowlistPath:   cfg.Federation.Security.AllowlistPath,
		BlocklistPath:   cfg.Federation.Security.BlocklistPath,
		BlocklistURL:    cfg.Federation.Security.BlocklistURL,
		BlocklistReload: time.Duration(cfg.Federation.Security.BlocklistReload) * time.Hour,
	}
	if fedSecCfg.MaxBodySize <= 0 {
		fedSecCfg.MaxBodySize = federation.MaxBodySize
	}
	if fedSecCfg.AllowlistMode == "" {
		fedSecCfg.AllowlistMode = "auto"
	}
	fedSecurity := federation.NewSecurityService(fedSecCfg)

	router := api.NewRouter(authSvc, keydirSvc, relaySvc, roomsSvc, mediaSvc, fedClient, fedKeys, cfg.Server.Domain, streamHub, vpnSvc, pushSvc, database, dbEnc, fedSecurity, cfg.Federation.Security.RateLimit, fedMainOnlyDomain, cfg.Federation.Security.AlertWebhookURL)

	port := cfg.Server.Port
	if port <= 0 {
		port = 8080
	}
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: router,
	}

	go func() {
		log.Printf("server listening on %s", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("shutting down...")
	if err := srv.Shutdown(context.Background()); err != nil {
		log.Printf("shutdown: %v", err)
	}
}
