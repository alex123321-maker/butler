package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/transport"
)

func TestMemoryCoherentLateWithdrawalAfterRestartIntegration(t *testing.T) {
	if os.Getenv("BUTLER_MEMORY_COHERENT_PILOT") != "1" {
		t.Skip("explicit coherent timeline followed by late privacy withdrawal required")
	}
	p := runMemoryCoherentPilotOrdinaryTimeline(t, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	p.ctx = owneridentity.WithActor(ctx, owneridentity.Actor{OwnerID: "owner", Channel: "web", DeliveryID: "coherent-late-withdrawal"})
	body := p.provider.source.Body
	var copies int
	if err := p.pool.QueryRow(ctx, `SELECT count(*) FROM tool_calls WHERE result::text LIKE '%'||$1||'%'`, body).Scan(&copies); err != nil || copies < 1 {
		t.Fatal("actual earlier Broker original copies absent", copies, err)
	}
	journalDigest := func() string {
		t.Helper()
		var hash string
		if err := p.pool.QueryRow(ctx, `SELECT encode(sha256(convert_to(jsonb_build_object('effects',(SELECT jsonb_agg(to_jsonb(e) ORDER BY action_id) FROM action_effects e),'evidence',(SELECT jsonb_agg(to_jsonb(e) ORDER BY evidence_id) FROM action_effect_evidence e))::text,'UTF8')),'hex')`).Scan(&hash); err != nil {
			t.Fatal(err)
		}
		return hash
	}
	before := journalDigest()
	late := p.newTask("coherent-late-withdrawal", "Проверить актуальность ранее принятого предпочтения", []string{"только чтение", "память не даёт полномочий"})
	p.provider.mode = "late_private"
	withdrew := false
	p.provider.beforeFinal = func(_ context.Context, request transport.StartRunRequest) error {
		visible, _ := json.Marshal(request.InputItems)
		if !strings.Contains(string(visible), body) {
			t.Error("late withdrawal did not follow actual authorized model preparation")
		}
		_, err := p.cat.Delete(p.ctx, p.provider.source.ID, "delete", p.provider.source.Revision)
		withdrew = err == nil
		return err
	}
	receipt, execution := p.execute(late, "Проверь принятое предпочтение и сохрани действующие ограничения.", "", p.b, p.cat, 65536)
	p.provider.beforeFinal = nil
	if !withdrew || execution.err == nil || receipt.Status == "completed" {
		t.Fatal("source withdrawal after inference preparation released the private answer", withdrew, receipt.Status)
	}
	var known, leaked, summaries int
	if err := p.pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM execution_model_attempts WHERE run_id=$1 AND observed_at IS NOT NULL AND actual_input_tokens=80 AND actual_output_tokens=30),(SELECT count(*) FROM tool_calls WHERE result::text LIKE '%'||$2||'%')+(SELECT count(*) FROM run_final_results WHERE content LIKE '%'||$2||'%')+(SELECT count(*) FROM messages WHERE content LIKE '%'||$2||'%'),(SELECT count(*) FROM session_summaries)`, receipt.RunID, body).Scan(&known, &leaked, &summaries); err != nil || known != 1 || leaked != 0 || summaries != 0 || journalDigest() != before {
		t.Fatal("withdrawal lost known charge, retained private copies or changed effect truth", known, leaked, summaries, err)
	}
	p.record("late_owner_delete_blocks_prepared_private_final_erases_prior_copies_preserves_effects_usage", &receipt, map[string]any{"known_usage_observations": known, "remaining_private_copies": leaked, "remaining_summaries": summaries, "effect_journal_unchanged": true})
	raw, _ := json.MarshalIndent(map[string]any{"controlled_only": true, "pass": !t.Failed(), "native_calls": 0, "steps": p.steps, "late_input": receipt, "known_usage": known, "remaining_private_copies": leaked, "effect_journal_unchanged": true, "limits": "partial ordered timeline plus late deletion; verified experience, consolidation, actual custom restore and ENOSPC remain separate"}, "", "  ")
	if err := os.WriteFile(filepath.Join(os.Getenv("BUTLER_MEMORY_COHERENT_PROOF"), "timeline-withdrawn.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
}
