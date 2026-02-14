package api

import (
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	federationRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "federation_requests_total",
			Help: "Total federation requests by domain and status",
		},
		[]string{"domain", "path", "status"},
	)
	federationRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "federation_request_duration_seconds",
			Help:    "Federation request duration in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"path"},
	)
	federationBlocklistHits = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "federation_blocklist_hits_total",
			Help: "Total blocklist matches by domain",
		},
		[]string{"domain"},
	)
)

func recordFederationRequest(domain, path string, status int, durationSeconds float64) {
	federationRequestsTotal.WithLabelValues(domain, path, strconv.Itoa(status)).Inc()
	federationRequestDuration.WithLabelValues(path).Observe(durationSeconds)
}

func recordFederationBlocklist(domain string) {
	federationBlocklistHits.WithLabelValues(domain).Inc()
}
