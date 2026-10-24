package communication

import (
	"context"
	"time"

	"github.com/butler/butler/internal/metrics"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
)

type diagnosticCollector struct {
	pool                                                      *pgxpool.Pool
	ready, deliveries, obligations, interactions, corrections *prometheus.Desc
	backlogAge, retries, interactionAge                       *prometheus.Desc
}

// RegisterMetrics reads durable counts at scrape time; no text, ref, owner,
// recipient, provider error or unbounded label can leave the owning database.
func RegisterMetrics(registry *metrics.Registry, pool *pgxpool.Pool) error {
	return registry.RegisterCollector(&diagnosticCollector{pool: pool,
		ready:          prometheus.NewDesc("butler_communication_observation_success", "Whether durable communication counts could be read.", nil, nil),
		deliveries:     prometheus.NewDesc("butler_communication_deliveries", "Retained publication delivery parts, not human reads or task completion.", []string{"channel", "state"}, nil),
		obligations:    prometheus.NewDesc("butler_communication_obligations", "Retained reply obligations.", []string{"state"}, nil),
		interactions:   prometheus.NewDesc("butler_communication_interactions", "Retained owner questions.", []string{"state"}, nil),
		corrections:    prometheus.NewDesc("butler_communication_corrections", "Retained spent missing-response continuations.", nil, nil),
		backlogAge:     prometheus.NewDesc("butler_communication_oldest_pending_seconds", "Age of the oldest pending or sending publication part.", []string{"channel"}, nil),
		retries:        prometheus.NewDesc("butler_communication_delivery_retries", "Retained delivery attempts after the first attempt.", []string{"channel"}, nil),
		interactionAge: prometheus.NewDesc("butler_communication_oldest_open_question_seconds", "Age of the oldest outstanding question, including no-expiry waits.", nil, nil)})
}
func (c *diagnosticCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, d := range []*prometheus.Desc{c.ready, c.deliveries, c.obligations, c.interactions, c.corrections, c.backlogAge, c.retries, c.interactionAge} {
		ch <- d
	}
}
func (c *diagnosticCollector) Collect(ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	collected := []prometheus.Metric{}
	for _, q := range []struct {
		sql    string
		desc   *prometheus.Desc
		labels int
	}{
		{`SELECT channel,status,count(*) FROM delivery_outbox WHERE publication_id IS NOT NULL AND channel IN ('web','telegram') AND status IN ('pending','sending','sent','delivery_unknown','failed','result_ready') GROUP BY channel,status`, c.deliveries, 2},
		{`SELECT obligation_state,count(*) FROM communication_admissions WHERE obligation_state IN ('open','waiting_for_user','response_committed','closed_with_failure','cancelled') GROUP BY obligation_state`, c.obligations, 1},
		{`SELECT state,count(*) FROM communication_interactions WHERE state IN ('open','resolved','cancelled','expired','superseded') GROUP BY state`, c.interactions, 1},
		{`SELECT channel,COALESCE(EXTRACT(EPOCH FROM clock_timestamp()-min(created_at) FILTER(WHERE status IN ('pending','sending'))),0)::float8 FROM delivery_outbox WHERE publication_id IS NOT NULL AND channel IN ('web','telegram') GROUP BY channel`, c.backlogAge, 1},
		{`SELECT channel,sum(GREATEST(attempt_count-1,0))::float8 FROM delivery_outbox WHERE publication_id IS NOT NULL AND channel IN ('web','telegram') GROUP BY channel`, c.retries, 1},
	} {
		rows, err := c.pool.Query(ctx, q.sql)
		if err != nil {
			ch <- prometheus.MustNewConstMetric(c.ready, prometheus.GaugeValue, 0)
			return
		}
		for rows.Next() {
			var a, b string
			var n float64
			if q.labels == 2 {
				err = rows.Scan(&a, &b, &n)
			} else {
				err = rows.Scan(&a, &n)
			}
			if err != nil {
				break
			}
			labels := []string{a}
			if q.labels == 2 {
				labels = append(labels, b)
			}
			collected = append(collected, prometheus.MustNewConstMetric(q.desc, prometheus.GaugeValue, n, labels...))
		}
		if err == nil {
			err = rows.Err()
		}
		rows.Close()
		if err != nil {
			ch <- prometheus.MustNewConstMetric(c.ready, prometheus.GaugeValue, 0)
			return
		}
	}
	var corrections float64
	if c.pool.QueryRow(ctx, `SELECT COALESCE(sum(correction_count),0) FROM communication_admissions`).Scan(&corrections) != nil {
		ch <- prometheus.MustNewConstMetric(c.ready, prometheus.GaugeValue, 0)
		return
	}
	var oldestOpen float64
	if c.pool.QueryRow(ctx, `SELECT COALESCE(EXTRACT(EPOCH FROM clock_timestamp()-min(created_at)),0)::float8 FROM communication_interactions WHERE state='open'`).Scan(&oldestOpen) != nil {
		ch <- prometheus.MustNewConstMetric(c.ready, prometheus.GaugeValue, 0)
		return
	}
	ch <- prometheus.MustNewConstMetric(c.ready, prometheus.GaugeValue, 1)
	ch <- prometheus.MustNewConstMetric(c.corrections, prometheus.GaugeValue, corrections)
	ch <- prometheus.MustNewConstMetric(c.interactionAge, prometheus.GaugeValue, oldestOpen)
	for _, m := range collected {
		ch <- m
	}
}
