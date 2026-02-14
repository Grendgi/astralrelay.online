package vpn

type Config struct {
	WireGuard   WireGuardConfig
	OpenVPN     OpenVPNConfig
	Enabled     bool
}

type WireGuardConfig struct {
	Enabled      bool
	Endpoint     string // host:port
	ServerPubKey string
	ClientSubnet string // e.g. 10.66.66.0/24 for client addresses
}

type OpenVPNConfig struct {
	Enabled  bool
	Endpoint string // host:port
}
