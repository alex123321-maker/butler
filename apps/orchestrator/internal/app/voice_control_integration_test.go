package app

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/butler/butler/apps/orchestrator/internal/approval"
	"github.com/butler/butler/apps/orchestrator/internal/input"
	"github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/audio"
	"github.com/butler/butler/internal/budget"
	pb "github.com/butler/butler/internal/gen/localmodels/v1"
	"github.com/butler/butler/internal/localmodels"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/jobs"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/voice"
	"github.com/jackc/pgx/v5"
)

type voiceControlBackend struct{}

func (voiceControlBackend) Describe(context.Context, *pb.Empty) (*pb.Catalog, error) {
	return &pb.Catalog{Models: []*pb.ModelProfile{
		{Id: "silero-vad-pcm-cpu-v1", Model: localmodels.VADModel, Revision: localmodels.VADRevision, Runtime: "torch-voice", Enabled: true, Capabilities: []pb.Capability{pb.Capability_CAPABILITY_VAD}},
		{Id: "qwen3-asr-ru-bf16-v1", Model: localmodels.ASRModel, Revision: localmodels.ASRRevision, Runtime: "torch-voice", Enabled: true, Capabilities: []pb.Capability{pb.Capability_CAPABILITY_ASR}},
	}}, nil
}
func (voiceControlBackend) Infer(_ context.Context, r *pb.InferRequest) (*pb.InferResponse, error) {
	out := &pb.InferResponse{RequestId: r.RequestId, ProfileId: r.ProfileId, Device: "cpu"}
	if r.GetVad() != nil {
		out.Revision = localmodels.VADRevision
		out.Output = &pb.InferResponse_Vad{Vad: &pb.VADOutput{SampleRate: 16000, Regions: []*pb.SpeechRegion{{StartSample: 0, EndSample: 16000}}}}
	} else {
		out.Revision = localmodels.ASRRevision
		out.Output = &pb.InferResponse_Asr{Asr: &pb.ASROutput{Text: "стоп", DecoderComplete: true}}
	}
	return out, nil
}

type voiceControlFault struct {
	inner  *VoiceControlAdapter
	fail   bool
	before func()
}

func (f *voiceControlFault) ApplyReviewed(ctx context.Context, c voice.ReviewedCommand, g func(context.Context, pgx.Tx) error) (string, error) {
	if f.before != nil {
		f.before()
	}
	id, e := f.inner.ApplyReviewed(ctx, c, g)
	if e == nil && f.fail {
		return "", errors.New("lost acknowledgement")
	}
	return id, e
}

func TestReviewedVoiceUsesCommonStopAndLostAckReceiptIntegration(t *testing.T) {
	for _, mode := range []string{"normal", "lost_ack", "deleted_before_commit"} {
		t.Run(mode, func(t *testing.T) {
			pool := memorytest.Database(t)
			ctx := owneridentity.WithActor(context.Background(), owneridentity.Actor{OwnerID: "voice-owner", Channel: "web", SessionKey: "web:voice-owner", DeliveryID: "upload"})
			manifest := catalog.NewFileManifest(filepath.Join(t.TempDir(), "anchor"))
			if e := manifest.Initialize(); e != nil {
				t.Fatal(e)
			}
			cat := catalog.New(pool, manifest)
			limits := budget.NewController(pool, budget.DefaultPolicy())
			limits.SetAuthorityResolver(actionauth.ResolveBudgetAuthority)
			js := jobs.New(pool, cat, limits, &jobs.LocalTarget{Endpoint: "local-models:9090", Identity: "voice-fixture"})
			store := &voice.Store{Catalog: cat, Jobs: js}
			if _, e := pool.Exec(ctx, `INSERT INTO sessions(session_key,user_id,channel) VALUES('web:voice-owner','voice-owner','web')`); e != nil {
				t.Fatal(e)
			}
			tasks := task.NewStore(pool)
			record, e := tasks.Create(ctx, task.CreateRequest{SessionKey: "web:voice-owner", OwnerID: "voice-owner", Goal: "prepare", CompletionCriteria: []string{"reviewed"}})
			if e != nil {
				t.Fatal(e)
			}
			leases := session.NewPostgresLeaseManager(pool)
			controls := task.NewController(tasks, leases.Authority(), leases, run.NewPostgresRepository(pool), approval.NewPostgresRepository(pool), input.NewStore(pool), transcript.NewStore(pool))
			target, _ := budget.NewLocalTarget("local-models:9090", "voice-fixture")
			processor, e := voice.NewProcessor(store, voiceControlBackend{}, target, budget.ModelPrice{Basis: "local_no_billing"})
			if e != nil {
				t.Fatal(e)
			}
			wav, _ := audio.Encode(make([]byte, 32000), 16000)
			clip, e := store.Upload(ctx, voice.Upload{WAV: wav, Role: voice.OwnerCommand, RetentionHours: 1})
			if e != nil {
				t.Fatal(e)
			}
			if _, e = store.Enqueue(ctx, clip.ID, budget.Units{ModelTurns: 2, Tokens: 5000, ActiveMillis: 120000, ProviderInputBytes: 2 << 20}); e != nil {
				t.Fatal(e)
			}
			if _, e = processor.ProcessOne(ctx); e != nil {
				t.Fatal(e)
			}
			clip, e = store.Get(ctx, clip.ID)
			if e != nil {
				t.Fatal(e)
			}
			review := voice.Review{ClipID: clip.ID, ExpectedRevision: clip.Revision, Text: "стоп", CheckedCriticalDetails: true, Target: voice.Target{SessionKey: record.SessionKey, TaskID: record.TaskID, ExpectedTaskRevision: record.Revision}}
			adapter := &VoiceControlAdapter{Pool: pool, Controller: controls}
			fault := &voiceControlFault{inner: adapter, fail: mode == "lost_ack"}
			if mode == "deleted_before_commit" {
				fault.before = func() {
					if e := store.Delete(ctx, clip.ID); e != nil {
						t.Fatal(e)
					}
				}
			}
			result, e := store.ConfirmControl(ctx, review, fault)
			if mode == "deleted_before_commit" {
				if e == nil {
					t.Fatal("deleted review controlled task")
				}
				current, e := tasks.Get(ctx, record.TaskID)
				if e != nil || current.Status != "active" {
					t.Fatal("deleted source changed authority", e)
				}
				return
			}
			if mode == "lost_ack" {
				if e == nil {
					t.Fatal("missing ACK reported success")
				}
				pending, readError := store.Get(ctx, clip.ID)
				if readError != nil || pending.State != "control_pending" {
					t.Fatal("lost ACK not recoverable", readError)
				}
				review.ExpectedRevision = pending.Revision
				fault.fail = false
				result, e = store.ConfirmControl(ctx, review, fault)
			}
			if e != nil || result.State != "submitted" || result.InputID == "" {
				t.Fatal("review control receipt", result, e)
			}
			current, e := tasks.Get(ctx, record.TaskID)
			if e != nil || current.Status != "cancelled" || current.Revision != 2 {
				t.Fatal("not common Stop", current, e)
			}
			var n int
			if e = pool.QueryRow(ctx, `SELECT count(*) FROM task_control_events WHERE task_id=$1`, record.TaskID).Scan(&n); e != nil || n != 1 {
				t.Fatal("duplicate voice control", n, e)
			}
		})
	}
}
