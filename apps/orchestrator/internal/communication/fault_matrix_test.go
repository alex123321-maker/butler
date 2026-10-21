package communication_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/communication"
	"github.com/butler/butler/apps/orchestrator/internal/input"
	"github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/internal/communicationcontrol"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// The wrapper executes actual PostgreSQL statements, then simulates a failed
// storage response or kills the worker process. There are no product fault hooks.
type boundaryTx struct {
	pgx.Tx
	writes, stop int
	kill         bool
	labels       []string
}

var boundaryFailure = errors.New("fixture storage response lost")

func (b *boundaryTx) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	tag, err := b.Tx.Exec(ctx, sql, args...)
	if err != nil {
		return tag, err
	}
	b.writes++
	words := strings.Fields(sql)
	label := strings.Join(words[:min(3, len(words))], " ")
	b.labels = append(b.labels, label)
	if b.writes == b.stop {
		if b.kill {
			os.Exit(93)
		}
		return tag, boundaryFailure
	}
	return tag, nil
}

type crashRequest struct {
	ClaimID                   string
	Principal                 communication.Principal
	Claim                     input.Record
	Token                     ownership.Token
	Kind, Schema, Interaction string
	Stop                      int
	Commit                    bool
}

func TestCrashCommunicationWorker(t *testing.T) {
	if os.Getenv("BUTLER_COMMUNICATION_CRASH_CHILD") != "1" {
		return
	}
	var q crashRequest
	if json.Unmarshal([]byte(os.Getenv("BUTLER_COMMUNICATION_CRASH_REQUEST")), &q) != nil {
		t.Fatal("invalid crash fixture")
	}
	q.Claim.ClaimID = q.ClaimID
	cfg, e := pgxpool.ParseConfig(os.Getenv("BUTLER_COMMUNICATION_CRASH_DSN"))
	if e != nil || !strings.HasSuffix(cfg.ConnConfig.Database, "_test") || !strings.HasPrefix(q.Schema, "memory_") {
		t.Fatal("isolated DB required")
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = q.Schema
	cfg.ConnConfig.RuntimeParams["application_name"] = "comm-crash-" + q.Principal.RunID
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	pool, e := pgxpool.NewWithConfig(ctx, cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer pool.Close()
	s := &communication.Store{Pool: pool, Inputs: input.NewStore(pool), SourceGuard: func(context.Context, pgx.Tx, string) error { return nil }, ChannelGuard: func(context.Context, pgx.Tx, string, communication.Binding, communicationcontrol.Args) error {
		return nil
	}}
	leas := session.NewPostgresLeaseManager(pool)
	ctx = ownership.WithToken(input.WithClaim(ctx, q.Claim), leas.Authority(), q.Token)
	if strings.HasPrefix(q.Kind, "handoff:") {
		s.Inputs.CommunicationEpoch = 1
		claim, e := s.Inputs.Claim(ctx, 2*time.Second)
		if e != nil {
			t.Fatal(e)
		}
		if q.Kind == "handoff:fresh_claim_before_lease" {
			os.Exit(93)
		}
		lease, e := leas.AcquireLease(ctx, session.AcquireLeaseParams{SessionKey: q.Principal.SessionKey, RunID: q.Principal.RunID, OwnerID: "crash-worker", TTL: 2 * time.Second})
		if e != nil {
			t.Fatal(e)
		}
		if q.Kind == "handoff:fresh_lease_before_run_refresh" {
			os.Exit(93)
		}
		live := ownership.WithToken(input.WithClaim(context.Background(), claim), leas.Authority(), ownership.Token{SessionKey: q.Principal.SessionKey, RunID: q.Principal.RunID, LeaseID: lease.LeaseID, FenceEpoch: lease.FenceEpoch})
		if _, e = run.NewService(run.NewPostgresRepository(pool), nil).RefreshExecutionLease(live, q.Principal.RunID, lease.LeaseID); e != nil {
			t.Fatal(e)
		}
		if q.Kind == "handoff:run_refresh_before_activation" {
			os.Exit(93)
		}
		if q.Kind != "handoff:task_activation_before_provider" {
			t.Fatal("unknown crash handoff")
		}
		if _, e = s.ResumeInteraction(live, q.Principal.RunID, q.Interaction); e != nil {
			t.Fatal(e)
		}
		os.Exit(93)
	}
	tx, e := pool.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	b := &boundaryTx{Tx: tx, stop: q.Stop, kill: true}
	if q.Kind == "response" {
		actor := owneridentity.WithActor(ctx, owneridentity.Actor{OwnerID: q.Principal.OwnerID, Channel: "web", SessionKey: "web:verified", DeliveryID: "fault-answer"})
		_, e = s.RespondTx(actor, b, q.Interaction, q.Principal.RunID+":web", 1, 1, communicationcontrol.Answer{OptionIDs: []string{"a"}})
	} else {
		_, e = s.ApplyTx(ctx, b, q.Principal, communicationcontrol.Envelope{RunID: q.Principal.RunID, ToolCallID: "fault", EffectID: "fault:" + q.Principal.RunID, Tool: q.Kind, ArgsJSON: map[string]string{"communication.message": message, "communication.widget": widget}[q.Kind]})
	}
	if e != nil {
		t.Fatal("child did not reach intended boundary", e)
	}
	if q.Commit {
		if e = tx.Commit(ctx); e != nil {
			t.Fatal(e)
		}
		os.Exit(93)
	}
	t.Fatal("crash boundary not reached")
}

func runCrash(t *testing.T, f *fixture, kind, id string, stop int, commit bool) {
	t.Helper()
	token, _ := ownership.TokenFromContext(f.ctx)
	q := crashRequest{Principal: f.p, Claim: f.claim, ClaimID: f.claim.ClaimID, Token: token, Kind: kind, Interaction: id, Stop: stop, Commit: commit, Schema: f.pool.Config().ConnConfig.RuntimeParams["search_path"]}
	raw, _ := json.Marshal(q)
	exe, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	u, e := url.Parse(f.pool.Config().ConnConfig.ConnString())
	if e != nil {
		t.Fatal(e)
	}
	cmd := exec.CommandContext(f.ctx, exe, "-test.run=^TestCrashCommunicationWorker$", "-test.timeout=20s")
	cmd.Env = append(os.Environ(), "BUTLER_COMMUNICATION_CRASH_CHILD=1", "BUTLER_COMMUNICATION_CRASH_REQUEST="+string(raw), "BUTLER_COMMUNICATION_CRASH_DSN="+u.String())
	out, e := cmd.CombinedOutput()
	var exit *exec.ExitError
	if !errors.As(e, &exit) || exit.ExitCode() != 93 {
		t.Fatalf("not the intended process death: %v %s", e, out)
	}
	// Do not mistake an uncommitted invisible row for completed crash recovery.
	for until := time.Now().Add(3 * time.Second); ; {
		var n int
		e = f.pool.QueryRow(f.ctx, `SELECT count(*) FROM pg_stat_activity WHERE application_name=$1`, "comm-crash-"+f.p.RunID).Scan(&n)
		if e != nil {
			t.Fatal(e)
		}
		if n == 0 {
			break
		}
		if time.Now().After(until) {
			t.Fatal("dead process DB transaction still present")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func applyBoundary(t *testing.T, f *fixture, kind string, stop int) ([]string, error) {
	t.Helper()
	tx, e := f.pool.Begin(f.ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(context.Background())
	b := &boundaryTx{Tx: tx, stop: stop}
	_, e = f.s.ApplyTx(f.ctx, b, f.p, communicationcontrol.Envelope{RunID: f.p.RunID, ToolCallID: "fault", EffectID: "fault:" + f.p.RunID, Tool: kind, ArgsJSON: map[string]string{"communication.message": message, "communication.widget": widget}[kind]})
	return b.labels, e
}

func assertPublicationAbsent(t *testing.T, f *fixture) {
	t.Helper()
	for _, table := range []string{"communication_publications", "messages", "delivery_outbox", "communication_interactions", "communication_events"} {
		if f.count(t, table) != 0 {
			t.Fatal("partial publication", table)
		}
	}
	r, e := f.s.Inputs.Get(f.ctx, f.p.InputID)
	if e != nil || r.Checkpoint != "tool_dispatching" {
		t.Fatal("partial wait checkpoint", r, e)
	}
	var count int
	var state string
	if e = f.pool.QueryRow(f.ctx, `SELECT publication_count,obligation_state FROM communication_admissions WHERE input_id=$1`, f.p.InputID).Scan(&count, &state); e != nil || count != 0 || state != "open" {
		t.Fatal("partial obligation", count, state, e)
	}
	if e = f.pool.QueryRow(f.ctx, `SELECT current_state FROM runs WHERE run_id=$1`, f.p.RunID).Scan(&state); e != nil || state != "tool_running" {
		t.Fatal("partial run wait", state, e)
	}
	if f.p.TaskID != "" {
		if e = f.pool.QueryRow(f.ctx, `SELECT status FROM tasks WHERE task_id=$1`, f.p.TaskID).Scan(&state); e != nil || state != "active" {
			t.Fatal("partial task wait", state, e)
		}
	}
}

func TestCommunicationEveryPublicationWriteFailureAndProcessCrashIntegration(t *testing.T) {
	for _, kind := range []string{"communication.message", "communication.widget"} {
		t.Run(kind, func(t *testing.T) {
			probe := setup(t, fixtureOptions{Task: true})
			labels, e := applyBoundary(t, probe, kind, 0)
			if e != nil {
				t.Fatal(e)
			}
			if len(labels) < 5 {
				t.Fatal("matrix failed to observe owning writes", labels)
			}
			for i, label := range labels {
				for _, mode := range []string{"storage_response_lost", "worker_killed"} {
					t.Run(fmt.Sprintf("%02d_%s_%s", i+1, strings.ReplaceAll(label, " ", "_"), mode), func(t *testing.T) {
						f := setup(t, fixtureOptions{Task: true})
						if mode == "worker_killed" {
							runCrash(t, f, kind, "", i+1, false)
						} else {
							observed, e := applyBoundary(t, f, kind, i+1)
							if e == nil || len(observed) != i+1 {
								t.Fatal("failure boundary missed", e)
							}
						}
						assertPublicationAbsent(t, f)
					})
				}
			}
			t.Run("commit_then_worker_killed_receipt_survives", func(t *testing.T) {
				f := setup(t, fixtureOptions{Task: true})
				runCrash(t, f, kind, "", 0, true)
				if f.count(t, "communication_publications") != 1 || f.count(t, "delivery_outbox") != 2 {
					t.Fatal("committed publication lost")
				}
				if kind == "communication.message" {
					r, e := f.publish(t, "fault", kind, message)
					if e != nil || r.PublicationRef == "" || f.count(t, "communication_publications") != 1 {
						t.Fatal("lost ACK replay duplicated", r, e)
					}
				} else {
					if id, e := f.s.PendingInteraction(f.ctx, f.p.RunID); e != nil || id == "" {
						t.Fatal("committed wait lost", id, e)
					}
				}
			})
		})
	}
}

func responseBoundary(t *testing.T, f *fixture, id string, stop int) ([]string, error) {
	t.Helper()
	tx, e := f.pool.Begin(f.ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(context.Background())
	b := &boundaryTx{Tx: tx, stop: stop}
	_, e = f.s.RespondTx(f.actor("web", "fault-answer"), b, id, f.bindings["web"], 1, 1, communicationcontrol.Answer{OptionIDs: []string{"a"}})
	return b.labels, e
}
func TestCommunicationEveryResponseWriteFailureAndProcessCrashIntegration(t *testing.T) {
	probe := setup(t, fixtureOptions{Task: true})
	p, e := probe.publish(t, "question", "communication.widget", widget)
	if e != nil {
		t.Fatal(e)
	}
	labels, e := responseBoundary(t, probe, p.InteractionRef, 0)
	if e != nil || len(labels) < 5 {
		t.Fatal(labels, e)
	}
	for i, label := range labels {
		for _, mode := range []string{"storage_response_lost", "worker_killed"} {
			t.Run(strconv.Itoa(i+1)+"_"+strings.ReplaceAll(label, " ", "_")+"_"+mode, func(t *testing.T) {
				f := setup(t, fixtureOptions{Task: true})
				p, e := f.publish(t, "question", "communication.widget", widget)
				if e != nil {
					t.Fatal(e)
				}
				if mode == "worker_killed" {
					runCrash(t, f, "response", p.InteractionRef, i+1, false)
				} else {
					if _, e = responseBoundary(t, f, p.InteractionRef, i+1); !errors.Is(e, boundaryFailure) {
						t.Fatal("response fault missed", e)
					}
				}
				var state string
				var response []byte
				if e = f.pool.QueryRow(f.ctx, `SELECT state,response_json FROM communication_interactions WHERE interaction_id=$1`, p.InteractionRef).Scan(&state, &response); e != nil || state != "open" || len(response) != 0 {
					t.Fatal("partial response", state, e)
				}
				if e = f.pool.QueryRow(f.ctx, `SELECT obligation_state FROM communication_admissions WHERE input_id=$1`, f.p.InputID).Scan(&state); e != nil || state != "waiting_for_user" {
					t.Fatal("partial response obligation", state, e)
				}
				if f.count(t, "communication_events") != 1 {
					t.Fatal("partial response event")
				}
				r, e := f.s.Inputs.Get(f.ctx, f.p.InputID)
				if e != nil || r.Checkpoint != "awaiting_interaction" || f.count(t, "messages") != 1 {
					t.Fatal("partial ready/transcript", r, e)
				}
				if _, e = f.s.Respond(f.actor("web", "fault-answer"), p.InteractionRef, f.bindings["web"], 1, 1, communicationcontrol.Answer{OptionIDs: []string{"a"}}); e != nil {
					t.Fatal("restart lost question", e)
				}
			})
		}
	}
	t.Run("commit_then_lost_response_ack", func(t *testing.T) {
		f := setup(t)
		p, e := f.publish(t, "question", "communication.widget", widget)
		if e != nil {
			t.Fatal(e)
		}
		runCrash(t, f, "response", p.InteractionRef, 0, true)
		if _, e = f.s.Respond(f.actor("web", "fault-answer"), p.InteractionRef, f.bindings["web"], 1, 1, communicationcontrol.Answer{OptionIDs: []string{"a"}}); e != nil {
			t.Fatal(e)
		}
		if f.count(t, "messages") != 2 {
			t.Fatal("response ACK replay duplicated transcript")
		}
	})
}
