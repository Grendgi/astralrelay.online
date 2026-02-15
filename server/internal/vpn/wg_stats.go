package vpn

import (
	"context"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// WGPeerStats holds rx/tx bytes for a WireGuard peer (by public key).
type WGPeerStats struct {
	RxBytes int64
	TxBytes int64
}

// FetchWGStats runs `wg show <iface>` and returns traffic per public key.
// Returns nil map if wg is unavailable or interface not set.
func (s *Service) FetchWGStats(ctx context.Context) map[string]WGPeerStats {
	iface := s.cfg.WireGuard.StatsInterface
	if iface == "" {
		return nil
	}
	out, err := exec.CommandContext(ctx, "wg", "show", iface).CombinedOutput()
	if err != nil {
		return nil
	}
	return parseWGShow(string(out))
}

// parseWGShow parses "wg show" output. Format:
//
//	peer: <base64_pubkey>
//	  transfer: 1.23 MiB received, 4.56 MiB sent
func parseWGShow(out string) map[string]WGPeerStats {
	result := make(map[string]WGPeerStats)
	lines := strings.Split(out, "\n")
	var currentPeer string
	peerRe := regexp.MustCompile(`^\s*peer:\s*(.+)$`)
	transferRe := regexp.MustCompile(`^\s*transfer:\s*(.+)\s+received,\s*(.+)\s+sent`)
	for _, line := range lines {
		if m := peerRe.FindStringSubmatch(line); m != nil {
			currentPeer = strings.TrimSpace(m[1])
			continue
		}
		if m := transferRe.FindStringSubmatch(line); m != nil && currentPeer != "" {
			rx := parseWGTransfer(m[1])
			tx := parseWGTransfer(m[2])
			result[currentPeer] = WGPeerStats{RxBytes: rx, TxBytes: tx}
			currentPeer = ""
		}
	}
	return result
}

// RemoveWGPeer removes a peer from the WireGuard interface. No-op if iface empty or wg unavailable.
func (s *Service) RemoveWGPeer(ctx context.Context, pubkey string) {
	iface := s.cfg.WireGuard.StatsInterface
	if iface == "" || pubkey == "" {
		return
	}
	_ = exec.CommandContext(ctx, "wg", "set", iface, "peer", pubkey, "remove").Run()
}

// parseWGTransfer parses "1.23 MiB" or "123 B" etc. into bytes.
func parseWGTransfer(s string) int64 {
	s = strings.TrimSpace(s)
	mult := int64(1)
	if strings.HasSuffix(s, " GiB") {
		mult = 1024 * 1024 * 1024
		s = strings.TrimSuffix(s, " GiB")
	} else if strings.HasSuffix(s, " MiB") {
		mult = 1024 * 1024
		s = strings.TrimSuffix(s, " MiB")
	} else if strings.HasSuffix(s, " KiB") {
		mult = 1024
		s = strings.TrimSuffix(s, " KiB")
	} else if strings.HasSuffix(s, " B") {
		s = strings.TrimSuffix(s, " B")
	}
	f, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return int64(f * float64(mult))
}
