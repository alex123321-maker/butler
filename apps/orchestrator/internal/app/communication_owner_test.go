package app

import (
	"errors"
	"path/filepath"
	"testing"

	approvals "github.com/butler/butler/apps/orchestrator/internal/approval"
	"github.com/butler/butler/apps/orchestrator/internal/channel/telegramrich"
	"github.com/butler/butler/apps/orchestrator/internal/communication"
	"github.com/butler/butler/apps/orchestrator/internal/input"
	"github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/owneridentity"
)

func TestCommunicationTelegramOwnerControlsUseExistingControllerIntegration(t *testing.T) {
	for _, scenario := range []string{"pause", "stop", "prepare", "amend", "resume", "ambiguous", "revoked"} {
		t.Run(scenario, func(t *testing.T) {
			f := newDelegationGuardFixtureWithManifest(t, filepath.Join(t.TempDir(), "deletions.log"))
			_ = f.leasedWorker(t, f.parent, "controlled-run")
			inputs := input.NewStore(f.pool)
			core := &communication.Store{Pool: f.pool, Inputs: inputs}
			binding := communication.Binding{ID: "test-bot", Version: 1, Channel: "telegram", Recipient: "telegram:42", Enabled: true, Capabilities: []string{"message"}}
			actor := owneridentity.WithActor(f.ctx, owneridentity.Actor{OwnerID: f.parent.OwnerID, Channel: "telegram", SessionKey: binding.Recipient, DeliveryID: "telegram-rich:77:5"})
			if err := core.Bind(actor, binding, 0); err != nil {
				t.Fatal(err)
			}
			controls := task.NewController(f.store, f.leases.Authority(), f.leases, run.NewPostgresRepository(f.pool), approvals.NewPostgresRepository(f.pool), inputs, transcript.NewStore(f.pool))
			controls.SetInteractionControls(core)
			wantRevision := f.parent.Revision + 1
			wantControls := 1
			if scenario == "resume" {
				seed := owneridentity.WithActor(f.ctx, owneridentity.Actor{OwnerID: f.parent.OwnerID, Channel: "telegram", DeliveryID: "seed-pause"})
				if _, err := controls.Apply(seed, task.ControlRequest{TaskID: f.parent.TaskID, Kind: task.ControlPause}); err != nil {
					t.Fatal(err)
				}
				wantRevision++
				wantControls++
			}
			c := &telegramCommunication{store: core, binding: telegramrich.OwnerBinding{OwnerID: f.parent.OwnerID, BindingID: binding.ID, Version: 1, BotID: 77, ChatID: 42, UserID: 42}, policy: telegramExportPolicy{binding: binding}, controls: controls}
			c.installInputHooks()
			text := "/pause " + f.parent.TaskID
			switch scenario {
			case "stop":
				text = "/stop " + f.parent.TaskID
			case "prepare":
				text = "/prepare " + f.parent.TaskID + " только черновик"
			case "amend":
				text = "/amend " + f.parent.TaskID + " уточнённые требования"
			case "resume":
				text = "/resume " + f.parent.TaskID
			case "ambiguous":
				if _, err := f.store.Create(f.ctx, task.CreateRequest{OwnerID: f.parent.OwnerID, SessionKey: f.parent.SessionKey, Goal: "Another active task", CompletionCriteria: []string{"Explicit owner result"}}); err != nil {
					t.Fatal(err)
				}
				text = "стоп"
			case "revoked":
				if _, err := f.pool.Exec(f.ctx, `UPDATE communication_bindings SET enabled=FALSE WHERE binding_id=$1`, binding.ID); err != nil {
					t.Fatal(err)
				}
			}
			u := telegramrich.Update{ID: 5, Message: &telegramrich.Message{Text: text}}
			err := c.conversation(actor, u)
			if scenario == "ambiguous" {
				if !errors.Is(err, communication.ErrAmbiguous) {
					t.Fatal(err)
				}
				return
			}
			if scenario == "revoked" {
				if err == nil {
					t.Fatal("revoked binding changed task")
				}
				saved, e := f.store.Get(f.ctx, f.parent.TaskID)
				if e != nil || saved.Status != "active" || saved.Revision != f.parent.Revision {
					t.Fatal(saved, e)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if err = c.conversation(actor, u); err != nil {
				t.Fatal("lost cursor ACK replay repeated/failed control", err)
			}
			saved, err := f.store.Get(f.ctx, f.parent.TaskID)
			if err != nil {
				t.Fatal(err)
			}
			if saved.Revision != wantRevision {
				t.Fatal("control replay changed revision", saved.Revision)
			}
			if scenario == "pause" && saved.Status != "paused" || scenario == "stop" && saved.Status != "cancelled" || scenario == "prepare" && saved.ExecutionPolicy != task.PolicyPrepareOnly || scenario == "resume" && saved.Status != "active" {
				t.Fatal("control not applied", saved.Status, saved.ExecutionPolicy)
			}
			var n int
			if err = f.pool.QueryRow(f.ctx, `SELECT count(*) FROM task_control_events`).Scan(&n); err != nil || n != wantControls {
				t.Fatal("control duplicated", n, err)
			}
			if scenario == "resume" {
				if err = f.pool.QueryRow(f.ctx, `SELECT count(*) FROM accepted_inputs WHERE task_id=$1 AND communication_ingress_epoch=1 AND communication_ingress_json->>'Kind'='owner_control'`, f.parent.TaskID).Scan(&n); err != nil || n != 1 {
					t.Fatal("resume lost private ingress proof", n, err)
				}
			}
		})
	}
}
