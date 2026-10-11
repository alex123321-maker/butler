package app

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/input"
	"github.com/butler/butler/internal/audio"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"
	"github.com/butler/butler/internal/voice"
)

func TestReviewedVoiceQueuesOnlyRefsAndPinsCurrentSourceIntegration(t *testing.T) {
	for _, mode := range []string{"current", "deleted_before_prepare", "expired", "control_rejected"} {
		t.Run(mode, func(t *testing.T) {
			pool := memorytest.Database(t)
			owner := owneridentity.WithActor(context.Background(), owneridentity.Actor{OwnerID: "voice-owner", Channel: "web", DeliveryID: "clip"})
			manifest := catalog.NewFileManifest(filepath.Join(t.TempDir(), "anchor"))
			if e := manifest.Initialize(); e != nil {
				t.Fatal(e)
			}
			cat := catalog.New(pool, manifest)
			store := &voice.Store{Catalog: cat}
			if _, e := pool.Exec(owner, `INSERT INTO sessions(session_key,user_id,channel) VALUES('web:voice-owner','voice-owner','web')`); e != nil {
				t.Fatal(e)
			}
			wav, _ := audio.Encode(make([]byte, 32000), 16000)
			clip, e := store.Upload(owner, voice.Upload{WAV: wav, Role: voice.OwnerCommand, RetentionHours: 1})
			if e != nil {
				t.Fatal(e)
			}
			tx, e := pool.Begin(owner)
			if e != nil {
				t.Fatal(e)
			}
			child, e := cat.CreateDerivedTx(owner, tx, []catalog.SourceRef{clip.Source}, "voice_owner_review", "mocked ASR preview", "PRIVATE VOICE BODY", json.RawMessage(`{}`))
			if e != nil {
				t.Fatal(e)
			}
			transcript := catalog.SourceRef{ID: child.ID, Revision: child.Revision, AccessRevision: child.AccessRevision, Hash: child.Hash}
			raw, _ := json.Marshal(transcript)
			if _, e = tx.Exec(owner, `UPDATE voice_clips SET state='review',transcript_ref=$2 WHERE clip_id=$1`, clip.ID, raw); e != nil {
				t.Fatal(e)
			}
			if e = tx.Commit(owner); e != nil {
				t.Fatal(e)
			}
			adapter := &VoiceInputAdapter{Pool: pool, Sources: cat, Inputs: input.NewStore(pool)}
			text := "PRIVATE VOICE BODY"
			if mode == "control_rejected" {
				text = "не отправляй письмо"
			}
			reviewed, e := store.Confirm(owner, voice.Review{ClipID: clip.ID, ExpectedRevision: clip.Revision, Text: text, CheckedCriticalDetails: true, Target: voice.Target{SessionKey: "web:voice-owner"}}, adapter)
			if mode == "control_rejected" {
				if e == nil {
					t.Fatal("restrictive owner control bypassed controller")
				}
				return
			}
			if e != nil {
				t.Fatal(e)
			}
			receipt, e := adapter.Inputs.Get(owner, reviewed.InputID)
			if e != nil {
				t.Fatal(e)
			}
			if strings.Contains(receipt.Event.PayloadJSON, "PRIVATE VOICE BODY") {
				t.Fatal("queue copied transcript")
			}
			claim, e := adapter.Inputs.Claim(context.Background(), time.Minute)
			if e != nil {
				t.Fatal(e)
			}
			_, e = pool.Exec(owner, `INSERT INTO runs(run_id,session_key,input_event_id,status,autonomy_mode,current_state,model_provider,started_at,updated_at) VALUES('voice-run','web:voice-owner',$1,'running','mode_1','preparing','fixture',clock_timestamp(),clock_timestamp())`, receipt.Event.EventID)
			if e != nil {
				t.Fatal(e)
			}
			if _, e = pool.Exec(owner, `INSERT INTO session_ownership(session_key,run_id,lease_id,fence_epoch,expires_at) VALUES('web:voice-owner','voice-run','lease',1,clock_timestamp()+INTERVAL '1 hour')`); e != nil {
				t.Fatal(e)
			}
			if e = adapter.Inputs.SaveCheckpoint(input.WithClaim(context.Background(), claim), "voice-run", "run_created", "{}"); e != nil {
				t.Fatal(e)
			}
			if mode == "deleted_before_prepare" {
				if e = store.Delete(owner, clip.ID); e != nil {
					t.Fatal(e)
				}
			}
			if mode == "expired" {
				if _, e = pool.Exec(owner, `UPDATE voice_clips SET expires_at=clock_timestamp()-INTERVAL '1 second'`); e != nil {
					t.Fatal(e)
				}
			}
			live := ownership.WithToken(context.Background(), ownership.NewAuthority(pool), ownership.Token{SessionKey: "web:voice-owner", RunID: "voice-run", LeaseID: "lease", FenceEpoch: 1})
			resolved, e := adapter.Prepare(live, receipt.Event)
			if mode != "current" {
				if e == nil {
					t.Fatal("deleted/expired input released")
				}
				return
			}
			if e != nil || !strings.Contains(resolved.PayloadJSON, "PRIVATE VOICE BODY") {
				t.Fatal("source body unavailable", e)
			}
			var n int
			if e = pool.QueryRow(owner, `SELECT count(*) FROM memory_run_sources WHERE run_id='voice-run'`).Scan(&n); e != nil || n != 3 {
				t.Fatal("complete source graph not pinned", n, e)
			}
			if _, e = pool.Exec(owner, `UPDATE runs SET metadata_json=$1 WHERE run_id='voice-run'`, resolved.PayloadJSON); e != nil {
				t.Fatal(e)
			}
			if e = store.Delete(owner, clip.ID); e != nil {
				t.Fatal(e)
			}
			var durable string
			if e = pool.QueryRow(owner, `SELECT metadata_json::text FROM runs WHERE run_id='voice-run'`).Scan(&durable); e != nil || strings.Contains(durable, "PRIVATE VOICE BODY") {
				t.Fatal("prepared copy survived deletion", e)
			}
			if _, e = adapter.Prepare(live, receipt.Event); e == nil {
				t.Fatal("late voice callback after delete")
			}
		})
	}
}
