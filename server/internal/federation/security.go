package federation

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	MaxBodySize     = 1 << 20  // 1MB
	MaxEvents       = 100
	MaxEventIDLen   = 128
	MaxTransactionIDLen = 128
	MaxUserIDLen    = 256
	MaxCiphertextLen = 64 << 10  // 64KB
	MaxSessionIDLen = 64
	MaxJSONDepth    = 10
	TimestampWindow = 5 * 60 // 5 min
)

// SecurityConfig holds federation security settings.
type SecurityConfig struct {
	RateLimit       int    // requests per window per domain
	MaxBodySize     int
	AllowlistMode   string // auto, manual, open
	AllowlistPath   string // file for allowlist (auto: append on first valid; manual: read)
	BlocklistPath   string // local blocklist file
	BlocklistURL    string // optional: fetch blocklist from URL
	BlocklistReload time.Duration
}

// DefaultSecurityConfig returns default values.
func DefaultSecurityConfig() SecurityConfig {
	return SecurityConfig{
		RateLimit:       100,
		MaxBodySize:     MaxBodySize,
		AllowlistMode:   "auto",
		BlocklistReload: 6 * time.Hour,
	}
}

// SecurityService manages blocklist, allowlist and validation.
type SecurityService struct {
	cfg       SecurityConfig
	blocklist map[string]struct{}
	allowlist map[string]struct{}
	mu        sync.RWMutex
}

// NewSecurityService creates a SecurityService and loads lists.
func NewSecurityService(cfg SecurityConfig) *SecurityService {
	s := &SecurityService{cfg: cfg, blocklist: make(map[string]struct{}), allowlist: make(map[string]struct{})}
	s.loadBlocklist()
	s.loadAllowlist()
	if cfg.BlocklistURL != "" && cfg.BlocklistReload > 0 {
		go s.reloadBlocklistLoop()
	}
	return s
}

func (s *SecurityService) loadBlocklist() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.blocklist = make(map[string]struct{})
	if s.cfg.BlocklistPath != "" {
		data, err := os.ReadFile(s.cfg.BlocklistPath)
		if err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				d := strings.TrimSpace(strings.ToLower(line))
				if d != "" && !strings.HasPrefix(d, "#") {
					s.blocklist[d] = struct{}{}
				}
			}
		}
	}
	if s.cfg.BlocklistURL != "" {
		resp, err := http.Get(s.cfg.BlocklistURL)
		if err == nil {
			defer resp.Body.Close()
			data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
			var list []string
			if json.Unmarshal(data, &list) == nil {
				for _, d := range list {
					s.blocklist[strings.ToLower(strings.TrimSpace(d))] = struct{}{}
				}
			} else {
				for _, line := range strings.Split(string(data), "\n") {
					d := strings.TrimSpace(strings.ToLower(line))
					if d != "" && !strings.HasPrefix(d, "#") {
						s.blocklist[d] = struct{}{}
					}
				}
			}
		}
	}
}

func (s *SecurityService) reloadBlocklistLoop() {
	for range time.Tick(s.cfg.BlocklistReload) {
		s.loadBlocklist()
	}
}

func (s *SecurityService) loadAllowlist() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.allowlist = make(map[string]struct{})
	if s.cfg.AllowlistPath != "" && (s.cfg.AllowlistMode == "manual" || s.cfg.AllowlistMode == "auto") {
		data, err := os.ReadFile(s.cfg.AllowlistPath)
		if err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				d := strings.TrimSpace(strings.ToLower(line))
				if d != "" && !strings.HasPrefix(d, "#") {
					s.allowlist[d] = struct{}{}
				}
			}
		}
	}
}

// MaxBodySize returns configured max body size (bytes).
func (s *SecurityService) MaxBodySize() int {
	if s.cfg.MaxBodySize > 0 {
		return s.cfg.MaxBodySize
	}
	return MaxBodySize
}

// IsBlocked returns true if domain is in blocklist.
func (s *SecurityService) IsBlocked(domain string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.blocklist[strings.ToLower(strings.TrimSpace(domain))]
	return ok
}

// IsAllowed returns true if domain is in allowlist (for manual mode) or if mode is open.
func (s *SecurityService) IsAllowed(domain string) bool {
	if s.cfg.AllowlistMode == "open" {
		return true
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.allowlist[strings.ToLower(strings.TrimSpace(domain))]
	return ok
}

// AddToAllowlist adds domain to allowlist and persists to file (auto mode).
// CheckAllowlistForTransaction returns nil if domain may proceed. For auto mode, adds domain on first valid call.
func (s *SecurityService) CheckAllowlistForTransaction(domain string) error {
	if s.cfg.AllowlistMode == "open" {
		return nil
	}
	if s.IsAllowed(domain) {
		return nil
	}
	if s.cfg.AllowlistMode == "manual" {
		return fmt.Errorf("domain not in allowlist")
	}
	// auto: trust-on-first-contact - caller adds after validation
	return nil
}

// AddToAllowlist adds domain to allowlist and persists to file (auto mode).
// No-op if already in allowlist.
func (s *SecurityService) AddToAllowlist(domain string) error {
	d := strings.ToLower(strings.TrimSpace(domain))
	if d == "" {
		return fmt.Errorf("empty domain")
	}
	s.mu.Lock()
	if _, exists := s.allowlist[d]; exists {
		s.mu.Unlock()
		return nil
	}
	s.allowlist[d] = struct{}{}
	s.mu.Unlock()
	if s.cfg.AllowlistPath != "" && s.cfg.AllowlistMode == "auto" {
		_ = os.MkdirAll(filepath.Dir(s.cfg.AllowlistPath), 0755)
		f, err := os.OpenFile(s.cfg.AllowlistPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return err
		}
		defer f.Close()
		_, _ = f.WriteString(d + "\n")
	}
	return nil
}

// ValidateTransaction validates request structure and limits.
func (s *SecurityService) ValidateTransaction(req *TransactionRequest, now int64) error {
	if len(req.TransactionID) > MaxTransactionIDLen {
		return fmt.Errorf("transaction_id too long")
	}
	if req.TransactionID == "" {
		return fmt.Errorf("transaction_id required")
	}
	if len(req.Events) > MaxEvents {
		return fmt.Errorf("too many events")
	}
	if len(req.Events) == 0 {
		return fmt.Errorf("events required")
	}
	for i, e := range req.Events {
		if len(e.EventID) > MaxEventIDLen {
			return fmt.Errorf("event[%d].event_id too long", i)
		}
		if len(e.Sender) > MaxUserIDLen {
			return fmt.Errorf("event[%d].sender too long", i)
		}
		if len(e.Recipient) > MaxUserIDLen {
			return fmt.Errorf("event[%d].recipient too long", i)
		}
		if len(e.Content.Ciphertext) > MaxCiphertextLen*2 { // base64 ~2x
			return fmt.Errorf("event[%d].content.ciphertext too long", i)
		}
		if len(e.Content.SessionID) > MaxSessionIDLen {
			return fmt.Errorf("event[%d].content.session_id too long", i)
		}
		// Timestamp check
		diff := now - e.Timestamp
		if diff < -60 || diff > TimestampWindow+60 {
			return fmt.Errorf("event[%d].timestamp out of window", i)
		}
	}
	return nil
}
