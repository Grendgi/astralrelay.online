// Backup-receiver — принимает pg_dump и медиа-бэкапы от peer-узлов mesh-сети.
package main

import (
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const defaultDir = "/data/backups"

func main() {
	dir := os.Getenv("BACKUP_DIR")
	if dir == "" {
		dir = defaultDir
	}
	secret := os.Getenv("BACKUP_SECRET")
	addr := os.Getenv("BACKUP_RECEIVER_ADDR")
	if addr == "" {
		addr = ":9100"
	}

	http.HandleFunc("/backup/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if secret != "" {
			if tok := r.Header.Get("X-Backup-Token"); tok != secret {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}
		// Path: /backup/{domain} — сохраняем в dir/from-{domain}/
		domain := strings.TrimPrefix(r.URL.Path, "/backup/")
		domain = strings.Trim(domain, "/")
		if domain == "" {
			domain = "unknown"
		}
		domain = strings.ReplaceAll(domain, "..", "")
		domain = strings.ReplaceAll(domain, "/", "_")

		saveDir := filepath.Join(dir, "from-"+domain)
		if err := os.MkdirAll(saveDir, 0700); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		filename := time.Now().Format("20060102-150405") + ".dump.gz"
		if h := r.Header.Get("X-Backup-Filename"); h != "" {
			filename = filepath.Base(h)
		}
		path := filepath.Join(saveDir, filename)
		f, err := os.Create(path)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer f.Close()
		n, err := io.Copy(f, r.Body)
		if err != nil {
			os.Remove(path)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		log.Printf("backup from %s: %s (%d bytes)", domain, filename, n)
		w.WriteHeader(http.StatusOK)
	})

	http.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) })

	log.Printf("Backup receiver on %s, dir=%s", addr, dir)
	log.Fatal(http.ListenAndServe(addr, nil))
}
