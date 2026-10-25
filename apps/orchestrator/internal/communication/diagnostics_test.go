package communication_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/api"
	"github.com/butler/butler/apps/orchestrator/internal/communication"
	"github.com/butler/butler/apps/orchestrator/internal/input"
	"github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/internal/communicationcontrol"
	runv1 "github.com/butler/butler/internal/gen/run/v1"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/metrics"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"
	"github.com/jackc/pgx/v5"
)

func diagnosticProfile(f *fixture) communication.DiagnosticProfile {
	return communication.DiagnosticProfile{OwnerID: f.p.OwnerID, Methods: []string{"communication.message", "communication.widget"}, Bindings: []communication.Binding{
		{ID: f.bindings["web"], Channel: "web", Version: 1}, {ID: f.bindings["telegram"], Channel: "telegram", Version: 1},
	}}
}

func TestCommunicationDiagnosticsIndependentDeliveryAndPrivacyIntegration(t *testing.T) {
	f := setup(t)
	p, err := f.publish(t, "diagnostic", "communication.message", message)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.pool.Exec(f.ctx, `UPDATE delivery_outbox SET status=CASE WHEN channel='web' THEN 'result_ready' ELSE 'delivery_unknown' END,error_code='PRIVATE_ERROR',channel_ack='PRIVATE_ACK' WHERE publication_id=$1`, p.PublicationRef); err != nil {
		t.Fatal(err)
	}
	r, err := f.s.Diagnostics(f.actor("web", "diagnostic"), diagnosticProfile(f))
	if err != nil || r.Status != "degraded" || r.FullV1Qualified {
		t.Fatal(r, err)
	}
	found := map[string]int64{}
	for _, d := range r.Deliveries {
		found[d.Channel+":"+d.State] = d.Count
	}
	if found["web:result_ready"] != 1 || found["telegram:delivery_unknown"] != 1 {
		t.Fatal(found)
	}
	raw, _ := json.Marshal(r)
	for _, secret := range []string{"PRIVATE_ERROR", "PRIVATE_ACK", "telegram:verified", "web:verified", "Choose a preference", "payload", "source_refs", "response_json"} {
		if strings.Contains(string(raw), secret) {
			t.Fatal("private field in diagnostics", secret)
		}
	}
	// Source/binding revocation never conceals retained failures behind Web.
	f.revoked.Store(true)
	if _, err = f.pool.Exec(f.ctx, `UPDATE communication_bindings SET enabled=FALSE WHERE binding_id=$1`, f.bindings["telegram"]); err != nil {
		t.Fatal(err)
	}
	r, err = f.s.Diagnostics(f.actor("web", "revoked"), diagnosticProfile(f))
	if err != nil || len(r.Deliveries) != 2 {
		t.Fatal("revocation hid durable metadata", r, err)
	}
	for _, b := range r.Bindings {
		if b.Channel == "telegram" && (b.Enabled || b.Current) {
			t.Fatal("disabled adapter called ready", b)
		}
	}
	foreign := owneridentity.WithActor(f.ctx, owneridentity.Actor{OwnerID: "foreign", Channel: "web", SessionKey: "web:foreign"})
	if _, err = f.s.Diagnostics(foreign, diagnosticProfile(f)); err == nil {
		t.Fatal("foreign owner read diagnostics")
	}
	if _, err = f.s.Diagnostics(f.ctx, diagnosticProfile(f)); err == nil {
		t.Fatal("worker token became owner proof")
	}
	// Scraping uses PostgreSQL observations, not Doctor's last snapshot.
	m := metrics.New()
	if err = communication.RegisterMetrics(m, f.pool); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	m.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/metrics", nil))
	if !strings.Contains(w.Body.String(), `butler_communication_deliveries{channel="telegram",state="delivery_unknown"} 1`) {
		t.Fatal(w.Body.String())
	}
	for _, secret := range []string{f.p.OwnerID, f.p.InputID, "PRIVATE_ERROR", "PRIVATE_ACK", "telegram:verified"} {
		if strings.Contains(w.Body.String(), secret) {
			t.Fatal("metric leaked private label", secret)
		}
	}
}

func TestCommunicationDiagnosticsWaitingAndFailureRemainDistinctIntegration(t *testing.T) {
	f := setup(t)
	if _, err := f.publish(t, "diagnostic-question", "communication.widget", widget); err != nil {
		t.Fatal(err)
	}
	r, err := f.s.Diagnostics(f.actor("web", "diagnostic"), diagnosticProfile(f))
	if err != nil || len(r.Inputs) != 1 || r.Inputs[0].Obligation != "waiting_for_user" || len(r.Interactions) != 1 || r.Interactions[0].State != "open" {
		t.Fatal(r, err)
	}
	if _, err = f.pool.Exec(f.ctx, `UPDATE communication_admissions SET obligation_state='closed_with_failure' WHERE input_id=$1`, f.p.InputID); err != nil {
		t.Fatal(err)
	}
	r, err = f.s.Diagnostics(f.actor("web", "diagnostic"), diagnosticProfile(f))
	if err != nil || r.Status != "degraded" || r.Inputs[0].Obligation != "closed_with_failure" || r.Interactions[0].State != "open" {
		t.Fatal("failure erased accepted question", r, err)
	}
}

func TestCommunicationDiagnosticsMissingSchemaIsNotHealthyIntegration(t *testing.T) {
	f := setup(t)
	if _, err := f.pool.Exec(f.ctx, `ALTER TABLE communication_widget_views RENAME TO missing_widget_views`); err != nil {
		t.Fatal(err)
	}
	r, err := f.s.Diagnostics(f.actor("web", "diagnostic"), diagnosticProfile(f))
	if err != nil || r.Status != "unhealthy" || len(r.Checks) != 1 || r.Checks[0].Reason != "migrations_required" {
		t.Fatal(r, err)
	}
}

func TestCommunicationOwnerQuestionLimitIsAtomicIntegration(t *testing.T) {
	for _, limit := range []int{-1, 65} {
		f := setup(t)
		f.s.MaxOpenInteractions = limit
		if _, err := f.publish(t, "question", "communication.widget", widget); err == nil {
			t.Fatal("invalid question limit admitted")
		}
		if f.count(t, "communication_publications") != 0 || f.count(t, "delivery_outbox") != 0 {
			t.Fatal("quota denial partially committed")
		}
	}
	f := setup(t)
	tx, err := f.pool.BeginTx(f.ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if _, err = f.s.ApplyTx(f.ctx, tx, f.p, communicationcontrol.Envelope{RunID: f.p.RunID, ToolCallID: "snapshot", EffectID: "snapshot", Tool: "communication.widget", ArgsJSON: widget}); err == nil {
		t.Fatal("snapshot isolation could oversubscribe owner quota")
	}
}

func TestCommunicationDiagnosticReadinessNeedsBrokerObservationIntegration(t *testing.T) {
	f := setup(t)
	f.s.WidgetProjections = true
	p := diagnosticProfile(f)
	p.ToolProbe = func(context.Context) (map[string]bool, error) {
		return map[string]bool{"communication.message": true, "communication.widget": false}, nil
	}
	r, err := f.s.Diagnostics(f.actor("web", "probe"), p)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range r.Checks {
		if c.Name == "communication.widget" && c.Reason == "broker_contract_mismatch" {
			found = true
		}
	}
	if !found {
		t.Fatal("composed flag bypassed actual contract check", r)
	}
	p.ToolProbe = func(context.Context) (map[string]bool, error) { return nil, fmt.Errorf("PRIVATE_BROKER_BODY") }
	r, err = f.s.Diagnostics(f.actor("web", "probe-unavailable"), p)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(r)
	if strings.Contains(string(raw), "PRIVATE_BROKER_BODY") || !strings.Contains(string(raw), "broker_not_observed") {
		t.Fatal("unknown Broker observation leaked or became ready", string(raw))
	}
}

type backedDiagnosticReporter struct {
	store   *communication.Store
	profile communication.DiagnosticProfile
}

func (r backedDiagnosticReporter) CommunicationReport(ctx context.Context) (communication.DiagnosticReport, error) {
	return r.store.Diagnostics(ctx, r.profile)
}

func TestCommunicationDiagnosticHTTPUsesDurableOwnerMetadataIntegration(t *testing.T) {
	f := setup(t)
	if _, err := f.publish(t, "owner-http", "communication.message", message); err != nil {
		t.Fatal(err)
	}
	token := strings.Repeat("h", 32)
	h := api.CommunicationDiagnostics(api.NewOwnerControlAuth(token, f.p.OwnerID), backedDiagnosticReporter{f.s, diagnosticProfile(f)})
	r := httptest.NewRequest("GET", "/api/v3/communications/diagnostics", nil)
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	var out communication.DiagnosticReport
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &out) != nil || len(out.Deliveries) != 2 || out.FullV1Qualified {
		t.Fatal(w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "Published answer") || strings.Contains(w.Body.String(), "telegram:verified") {
		t.Fatal("metadata endpoint loaded private content", w.Body.String())
	}
}

func diagnosticSibling(t *testing.T, f *fixture) *fixture {
	t.Helper()
	ctx := context.Background()
	id := fmt.Sprintf("diagnostic-sibling:%d", time.Now().UnixNano())
	key := "web:" + id
	if _, _, err := session.NewPostgresRepository(f.pool).CreateSession(ctx, session.CreateSessionParams{SessionKey: key, UserID: f.p.OwnerID, Channel: "web", MetadataJSON: `{}`}); err != nil {
		t.Fatal(err)
	}
	if _, err := run.NewPostgresRepository(f.pool).CreateRun(ctx, run.Record{RunID: id, SessionKey: key, InputEventID: id, Status: "tool_running", CurrentState: "tool_running", AutonomyMode: "mode_1", ModelProvider: "fixture", StartedAt: time.Now(), UpdatedAt: time.Now(), MetadataJSON: `{}`}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.s.Inputs.Accept(ctx, ingress.InputEvent{SessionKey: key, EventID: id, Source: "fixture", IdempotencyKey: id, EventType: runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE, PayloadJSON: `{"text":"ask"}`, CreatedAt: time.Now()}, f.p.OwnerID, ""); err != nil {
		t.Fatal(err)
	}
	claim, err := f.s.Inputs.Claim(ctx, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := f.leases.AcquireLease(ctx, session.AcquireLeaseParams{SessionKey: key, RunID: id, OwnerID: "diagnostic-worker", TTL: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.pool.Exec(ctx, `UPDATE runs SET lease_id=$2 WHERE run_id=$1`, id, lease.LeaseID); err != nil {
		t.Fatal(err)
	}
	ctx = ownership.WithToken(input.WithClaim(ctx, claim), f.leases.Authority(), ownership.Token{SessionKey: key, RunID: id, LeaseID: lease.LeaseID, FenceEpoch: lease.FenceEpoch})
	if err = f.s.Inputs.SaveCheckpoint(ctx, id, "tool_dispatching", `{"tool_call_id":"call"}`); err != nil {
		t.Fatal(err)
	}
	g := &fixture{ctx: ctx, pool: f.pool, s: f.s, p: communication.Principal{OwnerID: f.p.OwnerID, SessionKey: key, RunID: id, InputID: claim.InputID}, claim: claim, leases: f.leases, lease: lease, bindings: f.bindings}
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if err = f.s.AdmitTx(ctx, tx, g.p, false, []string{"communication.message", "communication.widget"}); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return g
}

func TestCommunicationOwnerQuestionLimitAcrossSessionsIntegration(t *testing.T) {
	f := setup(t)
	g := diagnosticSibling(t, f)
	f.s.MaxOpenInteractions = 1
	if err := f.s.ConfigureQuestionLimit(f.ctx, f.p.OwnerID, 1); err != nil {
		t.Fatal(err)
	}
	type result struct {
		f       *fixture
		receipt communicationcontrol.Receipt
		err     error
	}
	results := make(chan result, 2)
	for _, candidate := range []*fixture{f, g} {
		go func(c *fixture) {
			r, e := c.publish(t, "quota-question", "communication.widget", widget)
			results <- result{c, r, e}
		}(candidate)
	}
	a, b := <-results, <-results
	if (a.err == nil) == (b.err == nil) {
		t.Fatal("concurrent owner ceiling not enforced", a.err, b.err)
	}
	if a.err != nil {
		a, b = b, a
	}
	var publications, open int
	if err := f.pool.QueryRow(f.ctx, `SELECT (SELECT count(*) FROM communication_publications),(SELECT count(*) FROM communication_interactions WHERE state='open')`).Scan(&publications, &open); err != nil || publications != 1 || open != 1 {
		t.Fatal(publications, open, err)
	}
	if again, err := a.f.publish(t, "quota-question", "communication.widget", widget); err != nil || again.PublicationRef != a.receipt.PublicationRef {
		t.Fatal("quota broke receipt replay", again, err)
	}
	if _, err := f.s.Respond(f.actor("web", "quota-answer"), a.receipt.InteractionRef, f.bindings["web"], 1, 1, communicationcontrol.Answer{OptionIDs: []string{"a"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := b.f.publish(t, "quota-question", "communication.widget", widget); err != nil {
		t.Fatal("resolved question did not release capacity", err)
	}
}

func TestCommunicationOwnerQuestionLimitFencesOlderSQLWritersIntegration(t *testing.T) {
	f := setup(t)
	g := diagnosticSibling(t, f)
	if err := f.s.ConfigureQuestionLimit(f.ctx, f.p.OwnerID, 1); err != nil {
		t.Fatal(err)
	}
	if err := f.s.ConfigureQuestionLimit(f.actor("web", "not-operator"), f.p.OwnerID, 64); err == nil {
		t.Fatal("owner HTTP identity changed operator ceiling")
	}
	type pending struct {
		f   *fixture
		pub string
	}
	var pendingInserts []pending
	for _, c := range []*fixture{f, g} {
		r, err := c.publish(t, "existing-publication", "communication.message", message)
		if err != nil {
			t.Fatal(err)
		}
		pendingInserts = append(pendingInserts, pending{c, r.PublicationRef})
	}
	// Earlier writer SQL has neither the new Go check nor its advisory lock.
	const oldSQL = `INSERT INTO communication_interactions(interaction_id,publication_id,input_id,owner_id,session_key,run_id,task_revision,constraints_revision,question_json,state) SELECT 'old:'||publication_id,publication_id,input_id,owner_id,session_key,run_id,task_revision,constraints_revision,$2::jsonb,'open' FROM communication_publications WHERE publication_id=$1`
	first, err := f.pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Rollback(context.Background())
	if _, err = first.Exec(f.ctx, oldSQL, pendingInserts[0].pub, widget); err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	result := make(chan error, 1)
	go func() {
		second, err := f.pool.Begin(g.ctx)
		close(started)
		if err == nil {
			_, err = second.Exec(g.ctx, oldSQL, pendingInserts[1].pub, widget)
			if err == nil {
				err = second.Commit(g.ctx)
			} else {
				_ = second.Rollback(context.Background())
			}
		}
		result <- err
	}()
	<-started
	select {
	case err := <-result:
		t.Fatal("old writer did not wait for the uncommitted owner slot", err)
	case <-time.After(75 * time.Millisecond):
	}
	if err = first.Commit(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err = <-result; err == nil {
		t.Fatal("old writer used a pre-lock snapshot to bypass the owner ceiling")
	}
	var n int
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM communication_interactions WHERE owner_id=$1 AND state='open'`, f.p.OwnerID).Scan(&n); err != nil || n != 1 {
		t.Fatal(n, err)
	}
	if err := f.s.ConfigureQuestionLimit(f.ctx, f.p.OwnerID, 2); err != nil {
		t.Fatal(err)
	}
	rr, err := f.pool.BeginTx(f.ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = rr.Exec(f.ctx, oldSQL, pendingInserts[1].pub, widget); err == nil {
		t.Fatal("old snapshot writer bypassed supported isolation")
	}
	_ = rr.Rollback(context.Background())
	r, err := f.s.Diagnostics(f.actor("web", "policy-version"), diagnosticProfile(f))
	if err != nil || r.OpenQuestionLimit != 2 || r.LimitVersion != 2 {
		t.Fatal("policy version was not retained", r, err)
	}
	tx, err := f.pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(f.ctx, migration(t, "100_communication_owner_limits.down.sql")); err == nil {
		t.Fatal("downgrade erased accepted owner policy/history")
	}
}
