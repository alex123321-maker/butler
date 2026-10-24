package communication

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/butler/butler/internal/owneridentity"
	"github.com/jackc/pgx/v5"
)

// DiagnosticProfile describes installed components, not authorization or live
// channel/inference qualification. Only trusted composition supplies it.
type DiagnosticProfile struct {
	OwnerID   string
	DrainOnly bool
	Methods   []string
	Bindings  []Binding
	ToolProbe func(context.Context) (map[string]bool, error)
}

type DiagnosticCheck struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Reason string `json:"reason"`
}
type DiagnosticBinding struct {
	ID      string   `json:"binding_id"`
	Channel string   `json:"channel"`
	Version int64    `json:"version"`
	Enabled bool     `json:"enabled"`
	Current bool     `json:"current"`
	Methods []string `json:"methods"`
}
type DiagnosticCount struct {
	Channel       string  `json:"channel,omitempty"`
	State         string  `json:"state"`
	Count         int64   `json:"count"`
	OldestSeconds float64 `json:"oldest_seconds"`
}
type DiagnosticInput struct {
	InputID      string `json:"input_ref"`
	RunID        string `json:"run_ref"`
	State        string `json:"state"`
	Obligation   string `json:"obligation"`
	Publications int    `json:"publications"`
	Corrections  int    `json:"corrections"`
}
type DiagnosticReport struct {
	Status            string              `json:"status"`
	CheckedAt         time.Time           `json:"checked_at"`
	DrainOnly         bool                `json:"drain_only"`
	FullV1Qualified   bool                `json:"full_v1_qualified"`
	OpenQuestionLimit int                 `json:"open_question_limit"`
	LimitVersion      int64               `json:"limit_version"`
	Checks            []DiagnosticCheck   `json:"checks"`
	Bindings          []DiagnosticBinding `json:"bindings"`
	Deliveries        []DiagnosticCount   `json:"deliveries"`
	Obligations       []DiagnosticCount   `json:"obligations"`
	Interactions      []DiagnosticCount   `json:"interactions"`
	Inputs            []DiagnosticInput   `json:"inputs"`
	InputsTruncated   bool                `json:"inputs_truncated"`
	LegacyDeliveries  []DiagnosticCount   `json:"legacy_deliveries"`
}

// Diagnostics reads metadata only. It never loads payload, ACK, recipient,
// source/artifact refs, answers, error text, auth state or grant material.
// Owner-scoped metadata remains available when a binding/source is revoked.
func (s *Store) Diagnostics(ctx context.Context, p DiagnosticProfile) (DiagnosticReport, error) {
	out := DiagnosticReport{Status: "unknown", CheckedAt: time.Now().UTC(), DrainOnly: p.DrainOnly,
		Checks: []DiagnosticCheck{}, Bindings: []DiagnosticBinding{}, Deliveries: []DiagnosticCount{},
		Obligations: []DiagnosticCount{}, Interactions: []DiagnosticCount{}, Inputs: []DiagnosticInput{}, LegacyDeliveries: []DiagnosticCount{}}
	a, ok := owneridentity.FromContext(ctx)
	if !ok || p.OwnerID == "" || a.OwnerID != p.OwnerID || s == nil || s.Pool == nil {
		return out, ErrDenied
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	tx, err := s.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return out, errors.New("communication_observation_unavailable")
	}
	defer tx.Rollback(context.Background())
	// Missing schema is a visible readiness failure, never a healthy empty queue.
	var schema bool
	if err = tx.QueryRow(ctx, `SELECT to_regclass('communication_admissions') IS NOT NULL AND to_regclass('communication_interactions') IS NOT NULL AND to_regclass('communication_widget_views') IS NOT NULL AND to_regclass('communication_owner_limits') IS NOT NULL AND EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema=current_schema() AND table_name='accepted_inputs' AND column_name='communication_ingress_epoch')`).Scan(&schema); err != nil {
		return out, errors.New("communication_observation_unavailable")
	}
	if !schema {
		out.Checks = append(out.Checks, DiagnosticCheck{"schema", "unhealthy", "migrations_required"})
		out.Status = "unhealthy"
		return out, nil
	}
	out.Checks = append(out.Checks, DiagnosticCheck{"schema", "healthy", "metadata_readable"})
	out.Status = "healthy"
	check := func(name string, ready bool, reason string) {
		status := "healthy"
		if !ready {
			status = "degraded"
			out.Status = "degraded"
		}
		out.Checks = append(out.Checks, DiagnosticCheck{name, status, reason})
	}
	check("source_policy", s.SourceGuard != nil && s.ChannelGuard != nil, "installed_guards_required")
	err = tx.QueryRow(ctx, `SELECT max_open_interactions,version FROM communication_owner_limits WHERE owner_id=$1`, a.OwnerID).Scan(&out.OpenQuestionLimit, &out.LimitVersion)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return out, errors.New("communication_observation_unavailable")
	}
	if errors.Is(err, pgx.ErrNoRows) {
		out.OpenQuestionLimit = 16
	}
	expectedLimit := s.MaxOpenInteractions
	if expectedLimit == 0 {
		expectedLimit = 16
	}
	check("owner_question_limit", out.OpenQuestionLimit == expectedLimit, "current_owner_policy_required")
	check("worker_epoch", s.Inputs != nil && s.Inputs.CommunicationEpoch == 1, "epoch_1_required")
	check("web", s.WebAvailable, "authorized_reader_required")
	for _, method := range []string{"communication.message", "communication.photo", "communication.report", "communication.widget"} {
		ready := false
		for _, m := range p.Methods {
			ready = ready || m == method
		}
		if method == "communication.photo" || method == "communication.report" {
			ready = ready && s.PlanDelivery != nil
		}
		if method == "communication.widget" {
			ready = ready && s.WidgetProjections
		}
		check(method, ready, "composed_method_required")
	}
	rows, err := tx.Query(ctx, `SELECT binding_id,channel,version,enabled,capabilities FROM communication_bindings WHERE owner_id=$1 ORDER BY binding_id LIMIT 101`, a.OwnerID)
	if err != nil {
		return out, errors.New("communication_observation_unavailable")
	}
	for rows.Next() {
		var b DiagnosticBinding
		if err = rows.Scan(&b.ID, &b.Channel, &b.Version, &b.Enabled, &b.Methods); err != nil {
			break
		}
		for _, expected := range p.Bindings {
			b.Current = b.Current || b.ID == expected.ID && b.Channel == expected.Channel && b.Version == expected.Version && b.Enabled
		}
		out.Bindings = append(out.Bindings, b)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return out, errors.New("communication_observation_unavailable")
	}
	if len(out.Bindings) > 100 {
		out.Bindings = out.Bindings[:100]
		check("bindings", false, "binding_limit")
	}
	for _, expected := range p.Bindings {
		ready := false
		capabilities := false
		for _, b := range out.Bindings {
			ready = ready || b.ID == expected.ID && b.Current
			if b.ID == expected.ID && b.Current {
				capabilities = true
				for _, method := range p.Methods {
					capabilities = capabilities && slices.Contains(b.Methods, strings.TrimPrefix(method, "communication."))
				}
			}
		}
		check("binding."+expected.Channel, ready, "pinned_binding_required")
		check("binding."+expected.Channel+".methods", capabilities, "binding_capability_required")
	}
	check("adapter_profile", len(p.Bindings) >= 2, "web_and_telegram_required")
	queries := []struct {
		sql string
		dst *[]DiagnosticCount
	}{
		{`SELECT d.channel,d.status,count(*),COALESCE(EXTRACT(EPOCH FROM clock_timestamp()-min(d.created_at)),0)::float8 FROM delivery_outbox d JOIN communication_publications p USING(publication_id) WHERE p.owner_id=$1 GROUP BY d.channel,d.status ORDER BY d.channel,d.status`, &out.Deliveries},
		{`SELECT '',obligation_state,count(*),COALESCE(EXTRACT(EPOCH FROM clock_timestamp()-min(created_at)),0)::float8 FROM communication_admissions WHERE owner_id=$1 GROUP BY obligation_state ORDER BY obligation_state`, &out.Obligations},
		{`SELECT '',state,count(*),COALESCE(EXTRACT(EPOCH FROM clock_timestamp()-min(created_at)),0)::float8 FROM communication_interactions WHERE owner_id=$1 GROUP BY state ORDER BY state`, &out.Interactions},
		{`SELECT d.channel,d.status,count(*),COALESCE(EXTRACT(EPOCH FROM clock_timestamp()-min(d.created_at)),0)::float8 FROM delivery_outbox d JOIN sessions s USING(session_key) WHERE s.user_id=$1 AND d.publication_id IS NULL GROUP BY d.channel,d.status ORDER BY d.channel,d.status`, &out.LegacyDeliveries},
	}
	for _, q := range queries {
		rows, err = tx.Query(ctx, q.sql, a.OwnerID)
		if err != nil {
			return out, errors.New("communication_observation_unavailable")
		}
		for rows.Next() {
			var v DiagnosticCount
			if err = rows.Scan(&v.Channel, &v.State, &v.Count, &v.OldestSeconds); err != nil {
				break
			}
			*q.dst = append(*q.dst, v)
		}
		if err == nil {
			err = rows.Err()
		}
		rows.Close()
		if err != nil {
			return out, errors.New("communication_observation_unavailable")
		}
	}
	rows, err = tx.Query(ctx, `SELECT i.input_id,COALESCE(i.run_id,''),i.status,a.obligation_state,a.publication_count,a.correction_count FROM communication_admissions a JOIN accepted_inputs i USING(input_id) WHERE a.owner_id=$1 AND i.owner_id=$1 AND (a.obligation_state IN ('open','waiting_for_user','closed_with_failure') OR i.status IN ('failed','blocked','expired')) ORDER BY a.created_at DESC,i.input_id LIMIT 101`, a.OwnerID)
	if err != nil {
		return out, errors.New("communication_observation_unavailable")
	}
	for rows.Next() {
		var v DiagnosticInput
		if err = rows.Scan(&v.InputID, &v.RunID, &v.State, &v.Obligation, &v.Publications, &v.Corrections); err != nil {
			break
		}
		out.Inputs = append(out.Inputs, v)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return out, errors.New("communication_observation_unavailable")
	}
	out.InputsTruncated = len(out.Inputs) > 100
	if out.InputsTruncated {
		out.Inputs = out.Inputs[:100]
	}
	for _, v := range out.Deliveries {
		if v.Count > 0 && (v.State == "failed" || v.State == "delivery_unknown") {
			out.Status = "degraded"
		}
	}
	for _, v := range out.Obligations {
		if v.Count > 0 && v.State == "closed_with_failure" {
			out.Status = "degraded"
		}
	}
	if tx.Commit(ctx) != nil {
		return out, errors.New("communication_observation_unavailable")
	}
	// A composed flag cannot establish that Broker exposes the pinned schemas.
	// Probe only fixed read-only contracts after releasing the DB transaction.
	observed := map[string]bool{}
	var probeErr error
	if p.ToolProbe != nil {
		observed, probeErr = p.ToolProbe(ctx)
	}
	for i := range out.Checks {
		c := &out.Checks[i]
		if c.Status != "healthy" || (c.Name != "communication.message" && c.Name != "communication.photo" && c.Name != "communication.report" && c.Name != "communication.widget") {
			continue
		}
		if p.ToolProbe == nil || probeErr != nil {
			c.Status = "unknown"
			c.Reason = "broker_not_observed"
			if out.Status == "healthy" {
				out.Status = "unknown"
			}
		} else if !observed[c.Name] {
			c.Status = "degraded"
			c.Reason = "broker_contract_mismatch"
			out.Status = "degraded"
		} else {
			c.Reason = "pinned_broker_contract"
		}
	}
	return out, nil
}
