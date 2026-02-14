package vpn

import (
	"fmt"
	"strings"
)

// BuildWireGuardConf generates a WireGuard client .conf content.
func BuildWireGuardConf(clientPrivKey, clientPubKey, serverPubKey, endpoint, clientAddress string) string {
	var b strings.Builder
	b.WriteString("[Interface]\n")
	b.WriteString("PrivateKey = ")
	b.WriteString(clientPrivKey)
	b.WriteString("\n")
	b.WriteString("Address = ")
	b.WriteString(clientAddress)
	b.WriteString("\n")
	b.WriteString("DNS = 1.1.1.1, 8.8.8.8\n\n")
	b.WriteString("[Peer]\n")
	b.WriteString("PublicKey = ")
	b.WriteString(serverPubKey)
	b.WriteString("\n")
	b.WriteString("Endpoint = ")
	b.WriteString(endpoint)
	b.WriteString("\n")
	b.WriteString("AllowedIPs = 0.0.0.0/0, ::/0\n")
	return b.String()
}

// NextClientAddress returns the next free address in subnet 10.66.66.0/24.
// In MVP we use a simple counter; production would check DB for existing addresses.
func NextClientAddress(used []string) string {
	usedSet := make(map[string]bool)
	for _, a := range used {
		usedSet[a] = true
	}
	for i := 2; i < 254; i++ {
		addr := fmt.Sprintf("10.66.66.%d/32", i)
		if !usedSet[addr] {
			return addr
		}
	}
	return "10.66.66.2/32"
}
