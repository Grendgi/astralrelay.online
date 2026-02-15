// Migrate-only binary. Usage: DATABASE_URL=... go run ./server/cmd/migrate
package main

import (
	"fmt"
	"log"
	"os"

	"github.com/messenger/server/internal/config"
	"github.com/messenger/server/internal/db"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	if cfg.Database.URL == "" {
		cfg.Database.URL = os.Getenv("DATABASE_URL")
	}
	if cfg.Database.URL == "" {
		log.Fatal("DATABASE_URL required")
	}
	if err := db.Migrate(cfg.Database.URL); err != nil {
		log.Fatalf("migrate: %v", err)
	}
	fmt.Println("migrations OK")
}
