package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/extensions"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/readcontrol"
	"github.com/butler/butler/internal/transport"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Controlled echo of a real Broker read proves cached-copy erasure. This does
// not create a lesson, original result, owner verification or native answer.
type experienceReconciledCopyProvider struct {
	experienceOriginalControlProvider
	source catalog.SourceRef
	read   string
	before func() error
}

func (p *experienceReconciledCopyProvider) StartRun(ctx context.Context, q transport.StartRunRequest) (transport.EventStream, error) {
	if transport.CheckInferenceAuthority(ctx) != nil || transport.CheckInferenceDispatch(ctx) != nil {
		return nil, fmt.Errorf("current read inference authority required")
	}
	p.run, p.phase = q.Context.RunID, 1
	return p.emit(readcontrol.Open, readcontrol.Args{SourceID: p.source.ID, Revision: p.source.Revision, AccessRevision: p.source.AccessRevision, Hash: p.source.Hash, MaxChars: 8192}), nil
}

func (p *experienceReconciledCopyProvider) SubmitToolResult(ctx context.Context, q transport.SubmitToolResultRequest) (transport.EventStream, error) {
	if transport.CheckInferenceAuthority(ctx) != nil || transport.CheckInferenceDispatch(ctx) != nil || p.phase != 1 {
		return nil, fmt.Errorf("one actual read result required")
	}
	var envelope struct {
		Status string          `json:"status"`
		Result json.RawMessage `json:"result"`
	}
	if json.Unmarshal([]byte(q.ToolResultJSON), &envelope) != nil || envelope.Status != "completed" {
		return nil, fmt.Errorf("actual reconciled lesson read failed")
	}
	read, err := readcontrol.DecodeResult(envelope.Result)
	if err != nil || len(read.Items) != 1 || !strings.Contains(read.Items[0].Text, "reconciled_attempt") {
		return nil, fmt.Errorf("actual reconciled lesson bytes unavailable")
	}
	p.read, p.phase = read.Items[0].Text, 2
	if p.before != nil {
		if err := p.before(); err != nil {
			return nil, err
		}
	}
	stream := make(chan transport.TransportEvent, 1)
	stream <- transport.NewAssistantFinalEvent(p.run, p.Name(), transport.AssistantFinal{Content: p.read, Usage: &transport.Usage{InputTokens: 30, OutputTokens: 20, TotalTokens: 60}})
	close(stream)
	return stream, nil
}

// Build both an actual foreground cached copy and a verbatim derived accepted
// episode. The latter is an explicit projection, not model consolidation proof.
func experienceReconciledPrivacyCopies(t *testing.T, ctx context.Context, r *experienceNativeRuntime, recipient *experienceRecipient, anchor, endpoint, token string, p *experienceReconciledBackupReceipt, modes ...string) {
	t.Helper()
	owner := experienceNativeActor(ctx, "reconciled-privacy-copy-owner")
	cat := r.cat
	var lessonSource string
	if err := r.pool.QueryRow(ctx, `SELECT source_id,claim_id FROM memory_experience_lessons WHERE run_id=$1`, p.OriginalRun).Scan(&lessonSource, &p.ClaimID); err != nil {
		t.Fatal(err)
	}
	lesson, err := cat.Get(owner, lessonSource)
	if err != nil {
		t.Fatal(err)
	}
	p.Lesson = screenSourceRef(lesson)
	if err = r.pool.QueryRow(ctx, `SELECT episode_id FROM memory_claims WHERE claim_id=$1`, p.ClaimID).Scan(&p.EpisodeID); err != nil {
		t.Fatal(err)
	}
	if p.Original.ID == "" {
		var original string
		if err = r.pool.QueryRow(ctx, `SELECT 'message:'||message_id FROM messages WHERE run_id=$1 AND role='user' ORDER BY message_id LIMIT 1`, p.OriginalRun).Scan(&original); err != nil {
			t.Fatal(err)
		}
		source, err := cat.Get(owner, original)
		if err != nil {
			t.Fatal(err)
		}
		p.Original = screenSourceRef(source)
	}
	original, err := cat.Get(owner, p.Original.ID)
	if err != nil {
		t.Fatal(err)
	}
	p.OriginalBodyHash = experienceDigest(original.Body)
	var payload string
	if err = r.pool.QueryRow(ctx, `SELECT jsonb_agg(jsonb_build_array(tool_call_id,args,result,error)ORDER BY tool_call_id)::text FROM tool_calls WHERE run_id=$1`, p.OriginalRun).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	p.OriginalPayloadHash = experienceDigest(payload)

	tx, err := r.pool.Begin(owner)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(owner)
	copy, err := cat.CreateDerivedTx(owner, tx, []catalog.SourceRef{p.Lesson}, "memory_extract", "Verbatim reconciled note projection", lesson.Body, json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	value, _ := json.Marshal(map[string]string{"copied_note": lesson.Body})
	claim, err := cat.ProposeTx(owner, tx, catalog.ClaimRequest{MemoryType: "episodic", SourceID: copy.ID, SourceRevision: copy.Revision, Key: "episode.reconciled.verbatim_projection", Value: value, Summary: "Verbatim projection of the actual reconciled note", Confidence: 0, DedupKey: "actual-reconciled-privacy-projection"})
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(owner); err != nil {
		t.Fatal(err)
	}
	p.Descendant, p.DescendantClaimID = screenSourceRef(copy), claim.ID
	screenOwnerAPI(t, endpoint, token, http.MethodPost, "/api/v4/memory/decisions", catalog.Decision{ClaimID: claim.ID}, nil)
	if err = r.pool.QueryRow(ctx, `SELECT episode_id FROM memory_claims WHERE claim_id=$1`, claim.ID).Scan(&p.DescendantEpisodeID); err != nil {
		t.Fatal(err)
	}

	generation, environment := r.generation, r.environment
	r.close()
	// The same exact contract endpoints/generation and source database now serve
	// the real read-enabled composition; this is another visible current input.
	r = newExperienceNativeRuntime(t, r.pool, cat, anchor, t.TempDir(), recipient, r.addresses, r.budgets.Policy, "Прочитай разрешённую заметку как DATA; чтение не является выполнением исходной задачи.", experienceNativeArms[1], []catalog.SourceRef{p.Lesson, p.Descendant}, time.Now().Add(time.Hour))
	if r.generation != generation || r.environment != environment {
		t.Fatal("cached-copy consumer changed frozen generation or environment")
	}
	id := experienceDigest("actual-reconciled-copy-consumer")
	record := r.createTask(t, owner, id, "Read the permitted observed note as DATA", "alpha", "prepare_and_verify_exact_resource")
	r.acceptCurrentProfile(t, owner, record)
	tx, err = r.pool.Begin(owner)
	if err != nil {
		t.Fatal(err)
	}
	grants, err := cat.CanonicalSourcesTx(owner, tx, []catalog.SourceRef{p.Lesson, p.Descendant})
	tx.Rollback(owner)
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range grants {
		if err = cat.GrantSource(owner, record.TaskID, source.ID, "memory_read", record.Revision); err != nil {
			t.Fatal(err)
		}
	}
	provider := &experienceReconciledCopyProvider{source: p.Lesson}
	mode := "accepted"
	if len(modes) == 1 {
		mode = modes[0]
	} else if len(modes) != 0 {
		t.Fatal("one current publication scenario required")
	}
	if strings.HasPrefix(mode, "derived_") {
		provider.source = p.Descendant
		mode = strings.TrimPrefix(mode, "derived_")
	}
	var mutationErr, proofErr, leaseGuardErr error
	var leaseWait <-chan error
	if mode != "accepted" {
		provider.before = func() error {
			switch mode {
			case "missing_pin", "changed_pin", "environment":
				// These immutable rows have no production mutation API. Inject a
				// missing/corrupt persisted pin in this private test database only;
				// restore the trigger in the same transaction before publication.
				var fault pgx.Tx
				fault, mutationErr = r.pool.Begin(ctx)
				if mutationErr != nil {
					break
				}
				defer fault.Rollback(ctx)
				_, mutationErr = fault.Exec(ctx, `ALTER TABLE memory_experience_pins DISABLE TRIGGER experience_pins_immutable`)
				if mutationErr == nil {
					statements := map[string]string{
						"missing_pin": `DELETE FROM memory_experience_pins WHERE run_id=$1`,
						"changed_pin": `UPDATE memory_experience_pins SET lesson_hash=repeat('e',64) WHERE run_id=$1`,
						"environment": `UPDATE memory_experience_pins SET environment_hash=repeat('e',64) WHERE run_id=$1`,
					}
					var tag pgconn.CommandTag
					tag, mutationErr = fault.Exec(ctx, statements[mode], provider.run)
					if mutationErr == nil && tag.RowsAffected() != 1 {
						mutationErr = fmt.Errorf("one actual captured pin required before fault injection")
					}
				}
				if mutationErr == nil {
					_, mutationErr = fault.Exec(ctx, `ALTER TABLE memory_experience_pins ENABLE TRIGGER experience_pins_immutable`)
				}
				if mutationErr == nil {
					mutationErr = fault.Commit(ctx)
				}
			case "grant":
				_, mutationErr = r.pool.Exec(ctx, `DELETE FROM memory_scope_grants WHERE task_id=$1 AND source_id=$2`, record.TaskID, p.Lesson.ID)
			case "privacy":
				_, mutationErr = cat.Delete(owner, p.Lesson.ID, "delete", p.Lesson.Revision)
			case "feedback":
				mutationErr = r.lessons.Feedback(owner, p.Lesson.ID, provider.run, "negative")
			case "dependency":
				var out extensions.ControlResponse
				out, mutationErr = r.extensions.Control(owner, extensions.ControlRequest{Operation: "disable", ExtensionID: "project", Generation: r.generation})
				if mutationErr == nil && out.Error != "" {
					mutationErr = fmt.Errorf("actual dependency control denied")
				}
			case "owner":
				_, mutationErr = r.pool.Exec(ctx, `UPDATE tasks SET owner_id='different-owner' WHERE task_id=$1`, record.TaskID)
			case "task":
				_, mutationErr = r.pool.Exec(ctx, `UPDATE tasks SET status='paused',revision=revision+1,constraints_revision=constraints_revision+1 WHERE task_id=$1`, record.TaskID)
			case "lease_wait":
				var notify func(pgx.Tx)
				notify, leaseWait, mutationErr = experienceFinalLeaseExpiryWait(t, ctx, r, record.TaskID, p.Lesson.ID, provider.run)
				if mutationErr == nil {
					guard := r.results.DataGuard
					r.results.DataGuard = func(c context.Context, tx pgx.Tx, run string) error {
						notify(tx)
						leaseGuardErr = guard(c, tx, run)
						return leaseGuardErr
					}
				}
			default:
				mutationErr = fmt.Errorf("unsupported current publication scenario")
			}
			return mutationErr
		}
	} else {
		guard := r.results.DataGuard
		r.results.DataGuard = func(c context.Context, tx pgx.Tx, run string) error {
			var finalizing bool
			if proofErr = tx.QueryRow(c, `SELECT current_state='finalizing' FROM runs WHERE run_id=$1`, run).Scan(&finalizing); proofErr != nil || !finalizing {
				return fmt.Errorf("actual completion was not in finalizing")
			}
			if _, err := cat.AccessTx(c, tx, []catalog.SourceRef{p.Lesson}, "memory_read"); !errors.Is(err, catalog.ErrDenied) {
				proofErr = fmt.Errorf("generic finalizing source body access was not denied")
				return proofErr
			}
			return guard(c, tx, run)
		}
	}
	result := r.execute(t, ctx, record, id, "Read only the permitted observed note; preserve its uncertainty.", provider)
	if leaseWait != nil {
		if err := <-leaseWait; err != nil {
			t.Fatal("actual completion did not wait beyond lease expiry", err)
		}
		if leaseGuardErr == nil {
			t.Fatal("captured publication guard returned after actual grant wait beyond lease expiry")
		}
	}
	if mutationErr != nil || proofErr != nil || provider.phase != 2 || provider.read == "" {
		t.Fatal("actual cached-copy control setup failed", mutationErr, proofErr)
	}
	p.ConsumerRun = provider.run
	if mode != "accepted" {
		var finals, attempts int
		finalErr := r.pool.QueryRow(ctx, `SELECT count(*) FROM run_final_results WHERE run_id=$1`, p.ConsumerRun).Scan(&finals)
		attemptErr := r.pool.QueryRow(ctx, `SELECT count(*) FROM execution_model_attempts WHERE run_id=$1`, p.ConsumerRun).Scan(&attempts)
		if result.Err == nil || finalErr != nil || finals != 0 || attemptErr != nil || attempts != 2 {
			t.Fatal("changed or missing current proof published a final or lost spending", mode, result.Err, finals, attempts)
		}
		return
	}
	if result.Err != nil || result.Result == nil {
		t.Fatal("actual foreground cached-copy read failed", result.Err)
	}
	final, err := r.results.GetByRun(owner, p.ConsumerRun)
	if err != nil || final.Content != provider.read {
		t.Fatal("actual cached foreground final missing", err)
	}
	source, err := cat.Get(owner, "message:"+final.MessageID)
	if err != nil {
		t.Fatal(err)
	}
	p.ConsumerFinal = screenSourceRef(source)
}

func TestExperienceCapturedPinsFinalPublicationIntegration(t *testing.T) {
	for _, scenario := range []string{"accepted", "missing_pin", "changed_pin", "grant", "privacy", "environment", "feedback", "dependency", "owner", "task", "derived_accepted", "derived_missing_pin", "derived_grant"} {
		t.Run(scenario, func(t *testing.T) { experienceOriginalOwnerResumeRead(t, "publication_"+scenario) })
	}
}

func TestExperienceCapturedPinsLeaseExpiryDuringFinalWaitIntegration(t *testing.T) {
	experienceOriginalOwnerResumeRead(t, "publication_lease_wait")
}

// Hold an actual current grant on an independent connection while the ordinary
// Coordinator holds its ownership.Begin SHARE. Observe its real SQL waiter and
// database expiry before releasing the grant; no fake result or lease proof is
// passed to the completion. The worker's two model attempts remain charged.
func experienceFinalLeaseExpiryWait(t *testing.T, ctx context.Context, r *experienceNativeRuntime, taskID, sourceID, run string) (func(pgx.Tx), <-chan error, error) {
	t.Helper()
	waitCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	t.Cleanup(cancel)
	conn, err := pgx.ConnectConfig(waitCtx, r.pool.Config().ConnConfig.Copy())
	if err != nil {
		cancel()
		return nil, nil, err
	}
	// The ordinary image's two pooled connections are needed by the actual
	// ownership guard and completion. This private blocker cannot borrow them.
	if _, err = conn.Exec(waitCtx, `UPDATE session_ownership SET expires_at=clock_timestamp()+INTERVAL '2 seconds' WHERE run_id=$1`, run); err != nil {
		conn.Close(context.Background())
		cancel()
		return nil, nil, err
	}
	blocker, err := conn.Begin(waitCtx)
	if err != nil {
		conn.Close(context.Background())
		cancel()
		return nil, nil, err
	}
	var locked bool
	if err = blocker.QueryRow(waitCtx, `SELECT TRUE FROM memory_scope_grants WHERE task_id=$1 AND source_id=$2 FOR UPDATE`, taskID, sourceID).Scan(&locked); err != nil || !locked {
		blocker.Rollback(context.Background())
		conn.Close(context.Background())
		cancel()
		return nil, nil, fmt.Errorf("current grant blocker unavailable: %w", err)
	}
	pid := make(chan uint32, 1)
	done := make(chan error, 1)
	go func() {
		defer conn.Close(context.Background())
		defer blocker.Rollback(context.Background())
		var worker uint32
		select {
		case worker = <-pid:
		case <-waitCtx.Done():
			done <- waitCtx.Err()
			return
		}
		waited := false
		for {
			var waiting, expired bool
			err := blocker.QueryRow(waitCtx, `SELECT COALESCE((SELECT wait_event_type='Lock' FROM pg_stat_activity WHERE pid=$1),FALSE),EXISTS(SELECT 1 FROM session_ownership WHERE run_id=$2 AND expires_at<=clock_timestamp())`, worker, run).Scan(&waiting, &expired)
			if err != nil {
				done <- err
				return
			}
			waited = waited || waiting
			if waited && expired {
				done <- blocker.Rollback(waitCtx)
				return
			}
			select {
			case <-waitCtx.Done():
				done <- waitCtx.Err()
				return
			case <-time.After(10 * time.Millisecond):
			}
		}
	}()
	return func(tx pgx.Tx) { pid <- tx.Conn().PgConn().PID() }, done, nil
}
