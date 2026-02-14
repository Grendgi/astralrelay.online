package federation

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"github.com/messenger/server/internal/config"
)

type ServerKeys struct {
	PublicKey  ed25519.PublicKey
	PrivateKey ed25519.PrivateKey
}

var (
	keys     *ServerKeys
	keysOnce sync.Once
)

func LoadServerKeys(cfg *config.ServerConfig) (*ServerKeys, error) {
	var err error
	keysOnce.Do(func() {
		baseDir := os.Getenv("FEDERATION_KEYS_DIR")
		if baseDir == "" {
			baseDir = "."
		}
		path := filepath.Join(baseDir, ".federation_key_"+cfg.Domain)
		data, readErr := os.ReadFile(path)
		if readErr == nil {
			var stored struct {
				Pub  string `json:"public"`
				Priv string `json:"private"`
			}
			if json.Unmarshal(data, &stored) == nil {
				pub, _ := base64.StdEncoding.DecodeString(stored.Pub)
				priv, _ := base64.StdEncoding.DecodeString(stored.Priv)
				if len(pub) == ed25519.PublicKeySize && len(priv) == ed25519.PrivateKeySize {
					keys = &ServerKeys{PublicKey: pub, PrivateKey: priv}
					return
				}
			}
		}
		pub, priv, genErr := ed25519.GenerateKey(rand.Reader)
		if genErr != nil {
			err = genErr
			return
		}
		keys = &ServerKeys{PublicKey: pub, PrivateKey: priv}
		stored := map[string]string{
			"public":  base64.StdEncoding.EncodeToString(pub),
			"private": base64.StdEncoding.EncodeToString(priv),
		}
		b, _ := json.Marshal(stored)
		_ = os.WriteFile(path, b, 0600)
	})
	return keys, err
}

func (k *ServerKeys) SignPayload(payload []byte) []byte {
	return ed25519.Sign(k.PrivateKey, payload)
}

func (k *ServerKeys) PublicKeyBase64() string {
	return base64.StdEncoding.EncodeToString(k.PublicKey)
}
