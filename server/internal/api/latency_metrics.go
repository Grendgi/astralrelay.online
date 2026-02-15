package api

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/redis/go-redis/v9"
)

var (
	dbPingDuration = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "db_ping_duration_seconds",
			Help:    "PostgreSQL ping latency in seconds",
			Buckets: []float64{.001, .0025, .005, .01, .025, .05, .1, .25, .5, 1},
		},
	)
	redisPingDuration = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "redis_ping_duration_seconds",
			Help:    "Redis ping latency in seconds",
			Buckets: []float64{.0005, .001, .0025, .005, .01, .025, .05, .1, .25, .5},
		},
	)
)

// StartLatencyProbe runs periodic DB and Redis ping, recording latency to Prometheus.
func StartLatencyProbe(ctx context.Context, dbPool *pgxpool.Pool, redisURL string) {
	if dbPool == nil {
		return
	}
	var rdb *redis.Client
	if redisURL != "" {
		opts, err := redis.ParseURL(redisURL)
		if err == nil {
			rdb = redis.NewClient(opts)
			defer rdb.Close()
		}
	}
	probeDB(ctx, dbPool)
	if rdb != nil {
		probeRedis(ctx, rdb)
	}
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			probeDB(ctx, dbPool)
			if rdb != nil {
				probeRedis(ctx, rdb)
			}
		}
	}
}

func probeDB(ctx context.Context, pool *pgxpool.Pool) {
	start := time.Now()
	if err := pool.Ping(ctx); err == nil {
		dbPingDuration.Observe(time.Since(start).Seconds())
	}
}

func probeRedis(ctx context.Context, rdb *redis.Client) {
	start := time.Now()
	if err := rdb.Ping(ctx).Err(); err == nil {
		redisPingDuration.Observe(time.Since(start).Seconds())
	}
}
