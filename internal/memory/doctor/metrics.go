package doctor

import (
	"context"
	"time"

	"github.com/butler/butler/internal/metrics"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
)

type durableCollector struct {
	pool *pgxpool.Pool
	desc map[string]*prometheus.Desc
}

func RegisterMetrics(registry *metrics.Registry, pool *pgxpool.Pool) error {
	c := &durableCollector{pool: pool, desc: map[string]*prometheus.Desc{}}
	for _, name := range []string{"observation_success", "observed_at_seconds", "pending", "running", "completed", "failed", "retries", "oldest_pending_seconds", "active_generations", "verified_generations", "building_generations", "model_attempts", "model_unknown_usage", "model_reserved_tokens", "model_reserved_cost_micros"} {
		c.desc[name] = prometheus.NewDesc("butler_memory_durable_"+name, "Read-only PostgreSQL memory observation: "+name, nil, nil)
	}
	return registry.RegisterCollector(c)
}
func (c *durableCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, d := range c.desc {
		ch <- d
	}
}
func (c *durableCollector) Collect(ch chan<- prometheus.Metric) {
	emit := func(name string, value float64) {
		ch <- prometheus.MustNewConstMetric(c.desc[name], prometheus.GaugeValue, value)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	j, err := (&postgresReader{pool: c.pool}).Jobs(ctx, "")
	var active, verified, building float64
	if err == nil {
		err = c.pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM semantic_index_active),count(*) FILTER(WHERE verified_at IS NOT NULL),count(*) FILTER(WHERE state='building') FROM semantic_index_generations`).Scan(&active, &verified, &building)
	}
	if err != nil {
		emit("observation_success", 0)
		return
	}
	emit("observation_success", 1)
	emit("observed_at_seconds", float64(time.Now().Unix()))
	emit("pending", float64(j.Pending))
	emit("running", float64(j.Running))
	emit("completed", float64(j.Completed))
	emit("failed", float64(j.Failed))
	emit("retries", float64(j.Retries))
	emit("model_attempts", float64(j.ModelAttempts))
	emit("model_unknown_usage", float64(j.ModelUnknownUsage))
	emit("model_reserved_tokens", float64(j.ModelReservedTokens))
	emit("model_reserved_cost_micros", float64(j.ModelReservedCostMicros))
	if j.OldestPendingSeconds != nil {
		emit("oldest_pending_seconds", *j.OldestPendingSeconds)
	} else {
		emit("oldest_pending_seconds", 0)
	}
	emit("active_generations", active)
	emit("verified_generations", verified)
	emit("building_generations", building)
}
