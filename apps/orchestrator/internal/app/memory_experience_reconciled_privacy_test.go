package app

import (
	"context"
	"testing"

	"github.com/butler/butler/internal/memory/catalog"
)

func TestExperienceWitnessOnlyDeletionCascadesReconciledCopiesIntegration(t *testing.T) {
	for _, action := range []string{"delete", "revoke"} {
		t.Run(action, func(t *testing.T) { experienceOriginalOwnerResumeRead(t, "privacy_witness_"+action) })
	}
}

func experienceAssertReconciledCopiesErased(t *testing.T, ctx context.Context, cat *catalog.Store, p experienceReconciledBackupReceipt, originalDeleted bool) {
	t.Helper()
	owner := experienceNativeActor(ctx, "reconciled-privacy-assert-owner")
	pool := cat.Pool()
	refs := []catalog.SourceRef{p.Witness, p.Snapshot, p.Lesson, p.Descendant, p.ConsumerFinal}
	if originalDeleted {
		refs = append(refs, p.Original)
	}
	for _, ref := range refs {
		if _, err := cat.Get(owner, ref.ID); err == nil {
			t.Fatal("deleted reconciled copy remained readable", ref.ID)
		}
		var retained int
		if err := pool.QueryRow(ctx, `SELECT COALESCE(sum(octet_length(body)+octet_length(title)+octet_length(author)),0) FROM memory_source_revisions WHERE source_id=$1`, ref.ID).Scan(&retained); err != nil || retained != 0 {
			t.Fatal("copied source bytes retained", ref.ID, retained, err)
		}
	}
	for _, id := range []int64{p.EpisodeID, p.DescendantEpisodeID} {
		var count int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM memory_episodes WHERE id=$1`, id).Scan(&count); err != nil || count != 0 {
			t.Fatal("accepted reconciled episode or descendant survived", id, err)
		}
	}
	for _, id := range []string{p.ClaimID, p.DescendantClaimID} {
		var status, value, summary string
		if err := pool.QueryRow(ctx, `SELECT status,value_json::text,summary FROM memory_claims WHERE claim_id=$1`, id).Scan(&status, &value, &summary); err != nil || status != "deleted" || value != "{}" || summary != "" {
			t.Fatal("reconciled claim retained copied text", err)
		}
		if _, err := cat.Decide(owner, catalog.Decision{ClaimID: id}); err == nil {
			t.Fatal("deleted reconciled note could be reaccepted")
		}
	}
	var consumerErased bool
	if err := pool.QueryRow(ctx, `SELECT (SELECT count(*)>0 AND bool_and(args='{}'::jsonb AND error='{}'::jsonb AND result->>'privacy_redacted'='true') FROM tool_calls WHERE run_id=$1) AND EXISTS(SELECT 1 FROM run_final_results WHERE run_id=$1 AND content='' AND privacy_redacted)`, p.ConsumerRun).Scan(&consumerErased); err != nil || !consumerErased {
		t.Fatal("actual Broker cached copy or foreground final survived", err)
	}
	if originalDeleted {
		var erased bool
		if err := pool.QueryRow(ctx, `SELECT count(*)>0 AND bool_and(args='{}'::jsonb AND error='{}'::jsonb AND result->>'privacy_redacted'='true') FROM tool_calls WHERE run_id=$1`, p.OriginalRun).Scan(&erased); err != nil || !erased {
			t.Fatal("actual original cached tool payload survived deletion/anchor replay", err)
		}
	} else {
		original, err := cat.Get(owner, p.Original.ID)
		if err != nil || experienceDigest(original.Body) != p.OriginalBodyHash {
			t.Fatal("witness-only privacy action erased an independent primary parent", err)
		}
		var payload string
		if err = pool.QueryRow(ctx, `SELECT jsonb_agg(jsonb_build_array(tool_call_id,args,result,error)ORDER BY tool_call_id)::text FROM tool_calls WHERE run_id=$1`, p.OriginalRun).Scan(&payload); err != nil || experienceDigest(payload) != p.OriginalPayloadHash {
			t.Fatal("witness-only deletion changed independent original tool evidence", err)
		}
	}
}
