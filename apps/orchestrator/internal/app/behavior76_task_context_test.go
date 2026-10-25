package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/prompt"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/ownership"
	"github.com/butler/butler/internal/transport"
	"github.com/jackc/pgx/v5/pgconn"
)

// The ordinary Core establishes the fresh run/lease/task binding before this
// test-only metadata write. Never write through a stale previous run, clear a
// source fence, replace a deletion anchor, or restore an old working snapshot.
// Embedding the concrete Store preserves all its optional owning interfaces.
type behavior76PreparedTasks struct {
	*task.Store
	fixture  *behavior76Fixture
	mu       sync.Mutex
	enriched map[string]bool
}

func newBehavior76PreparedTasks(f *behavior76Fixture) *behavior76PreparedTasks {
	return &behavior76PreparedTasks{Store: f.tasks, fixture: f, enriched: map[string]bool{}}
}

var _ task.Manager = (*behavior76PreparedTasks)(nil)
var _ interface {
	ValidateRunAuthority(context.Context, string) error
} = (*behavior76PreparedTasks)(nil)
var _ interface {
	BlockRun(context.Context, string, string) error
} = (*behavior76PreparedTasks)(nil)

func (s *behavior76PreparedTasks) AttachRun(ctx context.Context, id string, revision int64, runID, sessionKey string) error {
	token, ok := ownership.TokenFromContext(ctx)
	if !ok || token.RunID != runID || token.SessionKey != sessionKey {
		return fmt.Errorf("owned execution context required for fixture metadata")
	}
	if err := s.Store.AttachRun(ctx, id, revision, runID, sessionKey); err != nil {
		return err
	}
	record, err := s.Store.Get(ctx, id)
	if err != nil {
		return err
	}
	if record.LastRunID != runID || record.SessionKey != sessionKey || record.Revision != revision {
		return fmt.Errorf("current fixture task binding required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.enriched[runID] {
		return nil
	}
	var key string
	if err = s.fixture.pool.QueryRow(ctx, `SELECT key FROM behavior76_aliases WHERE kind='task' AND value=$1`, id).Scan(&key); err != nil {
		return err
	}
	fresh := *s.fixture
	fresh.ctx = ctx
	if err = fresh.enrichWorking(key); err != nil {
		return err
	}
	// This local once-per-Core cache only avoids a second metadata write. It
	// grants no authority; every call above still performs actual AttachRun.
	s.enriched[runID] = true
	return nil
}

type behavior76SetupProvider struct {
	*initiativeProjectScriptedProvider
	requests []transport.StartRunRequest
	fixture  *behavior76Fixture
}

func (p *behavior76SetupProvider) StartRun(ctx context.Context, q transport.StartRunRequest) (transport.EventStream, error) {
	if p.fixture != nil {
		current, err := p.fixture.tasks.GetByRun(ctx, q.Context.RunID)
		hasTask := err == nil
		if err != nil && !errors.Is(err, task.ErrNotFound) {
			return nil, err
		}
		if err = p.fixture.sources.CheckRunSources(ctx, q.Context.RunID); err != nil {
			return nil, err
		}
		found := !hasTask
		for _, item := range q.InputItems {
			var payload struct {
				Data struct {
					Working *task.WorkingState `json:"task_working"`
				} `json:"data"`
			}
			if json.Unmarshal([]byte(item.Content), &payload) != nil || payload.Data.Working == nil {
				continue
			}
			if !hasTask {
				return nil, fmt.Errorf("conversation acquired a task working snapshot")
			}
			found = true
			for _, key := range []string{"synthetic_fixture_task_inventory", "synthetic_fixture_observations", "synthetic_fixture_resource_refs"} {
				if behavior76JSON(payload.Data.Working.Scratch[key]) != behavior76JSON(current.Working.Scratch[key]) {
					return nil, fmt.Errorf("prepared fixture metadata differs from durable state")
				}
			}
		}
		if !found {
			return nil, fmt.Errorf("actual task working model input missing")
		}
	}
	p.requests = append(p.requests, q)
	return initiativeProjectStream(
		transport.NewAssistantFinalEvent(q.Context.RunID, p.Name(), transport.AssistantFinal{Content: `{"answer":"Public setup-only response","task_key":"A","next_action":{"kind":"answer","resource_key":null,"expected_version":null,"body":null,"reason":"No tool effects"},"needs_clarification":false,"used_source_refs":[],"proposed_profile_update":{"kind":"none","source_refs":[],"summary":""},"completion_claim":"not_complete"}`, Usage: &transport.Usage{InputTokens: 1, OutputTokens: 1, TotalTokens: 2}}),
		transport.NewTerminalEvent(q.Context.RunID, transport.EventTypeRunCompleted, p.Name())), nil
}

func TestBehavior76LifecyclePreparationIntegration(t *testing.T) {
	t.Setenv(prompt.SettingKeyBasePrompt, prompt.DefaultBasePrompt)
	t.Setenv(prompt.SettingKeyBasePromptEnabled, "true")
	for _, operation := range []string{"correct", "revoke", "delete"} {
		t.Run(operation, func(t *testing.T) {
			f := newBehavior76Fixture(t)
			e := behavior76DevelopmentEpisode()
			e.Initial.Tasks[0].ExecutionPolicy = "normal"
			f.seed(e)
			provider := &behavior76SetupProvider{initiativeProjectScriptedProvider: &initiativeProjectScriptedProvider{}, fixture: f}
			first, err := f.evaluate(provider, e.Steps[0])
			if err != nil || first.State != "completed" || len(provider.requests) != 1 {
				t.Fatal("public first input preparation failed", err)
			}
			oldRun := first.OwnerRunID
			changed := "Use one exact compact status sentence for this public fixture."
			change := behavior76Step{ID: "public-lifecycle", Event: behavior76Event{Type: "profile_change", SourceKey: "public-pref", Operation: operation, Summary: &changed}}
			if _, err = f.apply(change); err != nil {
				t.Fatal("public source lifecycle failed", err)
			}
			// The old write must STILL be fenced. This is the original failure,
			// not a reason to weaken a migration or reissue old authority.
			var pgerr *pgconn.PgError
			if err = f.enrichWorking("A"); !errors.As(err, &pgerr) || pgerr.Code != "40001" {
				t.Fatal("old-context metadata write was not fenced", err)
			}
			if _, err = f.apply(behavior76Step{ID: "public-restart", Event: behavior76Event{Type: "service_restart"}}); err != nil {
				t.Fatal(err)
			}
			second, err := f.evaluate(provider, e.Steps[1])
			if err != nil || second.State != "completed" || len(provider.requests) != 2 {
				t.Fatal("fresh actual input failed after lifecycle/restart", err)
			}
			if second.OwnerRunID == "" || second.OwnerRunID == oldRun {
				t.Fatal("fresh run identity missing")
			}
			if f.sources.CheckRunSources(f.ctx, oldRun) == nil {
				t.Fatal("old source context became current")
			}
			var input strings.Builder
			for _, item := range provider.requests[1].InputItems {
				input.WriteString(item.Content)
			}
			if strings.Contains(input.String(), e.Initial.Profiles[0].Summary) {
				t.Fatal("old profile body entered the fresh model input")
			}
			if operation == "correct" && !strings.Contains(input.String(), changed) {
				t.Fatal("current accepted corrected profile absent from actual preparation")
			}
			if operation != "correct" && strings.Contains(input.String(), changed) {
				t.Fatal("deleted/revoked profile fabricated")
			}
			current, err := f.tasks.Get(f.ctx, f.lookup("task", "A"))
			if err != nil {
				t.Fatal(err)
			}
			if operation != "correct" && (current.Working.Scratch["privacy_redacted"] != true || len(current.Working.Plan) != 0 || len(current.Working.PendingActions) != 0) {
				t.Fatal("fresh preparation restored redacted working fields")
			}
			var effects int
			if err = f.pool.QueryRow(f.ctx, `SELECT count(*) FROM action_effects`).Scan(&effects); err != nil || effects != 0 {
				t.Fatal("setup-only provider created a tool effect", err)
			}
		})
	}
}

func TestBehavior76PreparedTasksOnceIntegration(t *testing.T) {
	f := newBehavior76Fixture(t)
	f.seed(behavior76DevelopmentEpisode())
	mandate := f.mandate("A")
	var expiry time.Time
	if err := f.pool.QueryRow(f.ctx, `SELECT expires_at FROM action_mandates WHERE mandate_id=$1`, mandate).Scan(&expiry); err != nil {
		t.Fatal(err)
	}
	deadline, ok := f.ctx.Deadline()
	if !ok || expiry.After(deadline) {
		t.Fatal("fresh fixture mandate outlives its actual parent/fixture deadline")
	}
	id, key := f.lookup("task", "A"), f.lookup("session", "A")
	record, err := f.tasks.Get(f.ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	const runID = "public-preparation-owned-run"
	if _, err = f.pool.Exec(f.ctx, `INSERT INTO runs(run_id,session_key,input_event_id,status,current_state,autonomy_mode,model_provider,task_id,task_revision,started_at,updated_at) VALUES($1,$2,'public-preparation','model_running','model_running','mode_1','fixture',$3,$4,clock_timestamp(),clock_timestamp())`, runID, key, id, record.Revision); err != nil {
		t.Fatal(err)
	}
	lease, err := f.leases.AcquireLease(f.ctx, session.AcquireLeaseParams{SessionKey: key, RunID: runID, OwnerID: "public-fixture-worker", TTL: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	token := ownership.Token{SessionKey: key, RunID: runID, LeaseID: lease.LeaseID, FenceEpoch: lease.FenceEpoch}
	ctx := ownership.WithToken(f.ctx, f.leases.Authority(), token)
	prepared := newBehavior76PreparedTasks(f)
	if err = prepared.AttachRun(ctx, id, record.Revision, runID, key); err != nil {
		t.Fatal(err)
	}
	first, err := f.tasks.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if first.WorkingRevision != record.WorkingRevision+1 {
		t.Fatal("first enrichment did not write exactly once")
	}
	if err = prepared.AttachRun(ctx, id, record.Revision, runID, key); err != nil {
		t.Fatal(err)
	}
	second, err := f.tasks.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if second.WorkingRevision != first.WorkingRevision || second.LastRunID != runID {
		t.Fatal("second attachment rewrote prepared working revision")
	}
	if err = prepared.ValidateRunAuthority(ctx, runID); err != nil {
		t.Fatal("optional owning authority interface lost", err)
	}
	if err = prepared.AttachRun(f.ctx, id, record.Revision, runID, key); err == nil {
		t.Fatal("missing execution token accepted")
	}
	wrong := token
	wrong.RunID = "foreign-run"
	if err = prepared.AttachRun(ownership.WithToken(f.ctx, f.leases.Authority(), wrong), id, record.Revision, runID, key); err == nil {
		t.Fatal("mismatched execution token accepted")
	}
	if err = prepared.AttachRun(ctx, id, record.Revision+1, runID, key); err == nil {
		t.Fatal("stale task revision accepted on duplicate attachment")
	}
	final, err := f.tasks.Get(ctx, id)
	if err != nil || final.WorkingRevision != second.WorkingRevision {
		t.Fatal("denied attachment changed durable working state", err)
	}
}
