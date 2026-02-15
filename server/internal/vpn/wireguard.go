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

// Address allocation is done transactionally via vpn_wg_addr_seq (migration 016).
// client_address is stored as inet in vpn_peers (migration 017); use GIST index
// and network operators (&&, >>=, etc.) for subnet/overlap queries.
