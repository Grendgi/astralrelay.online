package api

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	vpnConfigIssuedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "vpn_config_issued_total",
			Help: "Total VPN configs issued by protocol",
		},
		[]string{"protocol"},
	)
	vpnConfigRevokedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "vpn_config_revoked_total",
			Help: "Total VPN configs revoked by protocol",
		},
		[]string{"protocol"},
	)
)

func recordVPNConfigIssued(protocol string) {
	vpnConfigIssuedTotal.WithLabelValues(protocol).Inc()
}

func recordVPNConfigRevoked(protocol string) {
	vpnConfigRevokedTotal.WithLabelValues(protocol).Inc()
}
