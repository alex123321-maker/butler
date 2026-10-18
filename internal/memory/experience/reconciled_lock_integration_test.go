package experience

import (
	"context"
	"testing"
	"time"

	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/transcript"
)

// This is a controlled lock-order fixture, not reconciled outcome evidence. A
// current stopped observation is an ancestor of another proposed observation;
// the new primary tool sorts before the ancestor's already captured tool.
func TestExperienceReconciledSealingLocksNewAndAncestorToolsBeforeInputsIntegration(t *testing.T) {
	f, ancestor := terminalFixture(t)
	ctx, cancel := context.WithTimeout(f.ctx, 10*time.Second)
	defer cancel()
	const ownTool = "000-new-observation-tool"
	if _, err := f.pool.Exec(ctx, `INSERT INTO tool_calls(tool_call_id,run_id,tool_name,args,status,runtime_target,started_at,result)VALUES($1,'run-1','http.request','{}','completed','fixture',clock_timestamp(),'{}')`, ownTool); err != nil {
		t.Fatal(err)
	}
	o := Observation{ToolPins: []transcript.ToolPin{{ID: ownTool}}, TerminalAttempt: &TerminalAttempt{InputID: "own-input"}, ReconciledAttempt: &ReconciledAttempt{OriginalSnapshotID: f.parent.ID, ContinuationInputID: "own-continuation", LookupPin: transcript.ToolPin{ID: ownTool}, Heads: []ReconciledHeadPin{}}}
	parents := []catalog.SourceRef{ancestor}
	writer, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Rollback(ctx)
	if _, err = writer.Exec(ctx, `SELECT tool_call_id FROM tool_calls WHERE tool_call_id=$1 FOR UPDATE`, ownTool); err != nil {
		t.Fatal(err)
	}
	pid, done := make(chan uint32, 1), make(chan error, 1)
	go func() {
		reader, err := f.pool.Begin(ctx)
		if err != nil {
			done <- err
			return
		}
		defer reader.Rollback(ctx)
		pid <- reader.Conn().PgConn().PID()
		if err = lockReconciledSealingRowsTx(ctx, reader, o, parents); err == nil {
			_, err = f.s.Sources.AccessTx(ctx, reader, parents, "memory_job_create")
		}
		if err == nil {
			err = reader.Commit(ctx)
		}
		done <- err
	}()
	var readerPID uint32
	select {
	case readerPID = <-pid:
	case err = <-done:
		t.Fatal("reader admission", err)
	case <-ctx.Done():
		t.Fatal("reader admission deadline")
	}
	waiting := false
	for until := time.Now().Add(2 * time.Second); time.Now().Before(until); {
		if err = writer.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE pid=$1 AND wait_event_type='Lock')`, readerPID).Scan(&waiting); err != nil || waiting {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if err != nil || !waiting {
		t.Fatal("reader did not wait for the exact new primary tool", err)
	}
	// With the previous parent AccessTx-before-own-tools seam, these ancestor
	// locks would already be held by the blocked reader, forming an inversion.
	if _, err = writer.Exec(ctx, `SELECT tool_call_id FROM tool_calls WHERE tool_call_id=$1 FOR UPDATE NOWAIT`, f.observation.ToolPins[0].ID); err != nil {
		t.Fatal("ancestor tool was locked before the earlier new primary tool", err)
	}
	var inputAvailable bool
	if err = writer.QueryRow(ctx, `SELECT pg_try_advisory_xact_lock(hashtextextended('butler-terminal-input:stopped-input',0))`).Scan(&inputAvailable); err != nil || !inputAvailable {
		t.Fatal("ancestor input fence was taken before the complete primary tool union", err)
	}
	if err = writer.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case err = <-done:
		if err != nil {
			t.Fatal("reader after primary-tool release", err)
		}
	case <-ctx.Done():
		t.Fatal("reader did not complete after exact writer release")
	}
}
