package budget

import (
	"context"
	"testing"
)

func TestStoppedAttemptIsolationCannotAuthorizeAnotherWriteIntegration(t *testing.T) {
	for _, name := range []string{"read", "write", "fresh_owner_reply", "duplicate_owner_reply", "forged_reply_flag", "new_task", "other_session", "known_stop"} {
		t.Run(name, func(t *testing.T) {
			pool := conversationLoopDB(t)
			ctx := context.Background()
			if _, err := pool.Exec(ctx, `INSERT INTO accepted_inputs(input_id,run_id,owner_id,session_key) VALUES('stopped-input','conversation-a','owner','session'); INSERT INTO input_stopped_attempts(input_id,run_id,owner_id,session_key,classification,reason) VALUES('stopped-input','conversation-a','owner','session','uncertain','journal_uncertain'); INSERT INTO action_effects(action_id,fingerprint,initial_binding,policy,state) VALUES('stopped-effect','f','{"owner_id":"owner","run_id":"conversation-a","tool_name":"communication.message","args_hash":"old-args","effect":"prepare"}','{}','effect_unknown')`); err != nil {
				t.Fatal(err)
			}
			i := conversationLoopCall("conversation-b", "new-effect")
			i.Effect, i.ToolName, i.ArgsHash = "commit", "different.write", "new-args"
			root := "session"
			switch name {
			case "read":
				i.Effect = "read"
			case "fresh_owner_reply", "duplicate_owner_reply":
				i.OwnerCommunication = true
				i.Effect = "prepare"
				i.ToolName = "communication.message"
				if name == "duplicate_owner_reply" {
					i.ArgsHash = "old-args"
				}
			case "forged_reply_flag":
				i.OwnerCommunication = true
				i.Effect = "prepare"
			case "new_task":
				if _, err := pool.Exec(ctx, `INSERT INTO tasks(task_id,owner_id,session_key) VALUES('fresh-root','owner','session'); UPDATE runs SET task_id='fresh-root',task_revision=1 WHERE run_id='conversation-b'`); err != nil {
					t.Fatal(err)
				}
				root = "fresh-root"
			case "other_session":
				if _, err := pool.Exec(ctx, `INSERT INTO sessions VALUES('other-session','owner'); UPDATE runs SET session_key='other-session' WHERE run_id='conversation-b'`); err != nil {
					t.Fatal(err)
				}
				root = "other-session"
			case "known_stop":
				if _, err := pool.Exec(ctx, `UPDATE input_stopped_attempts SET classification='known'`); err != nil {
					t.Fatal(err)
				}
			}
			tx, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			err = stoppedAttemptGuardTx(ctx, tx, i, "owner", root)
			allowed := name == "read" || name == "fresh_owner_reply" || name == "other_session" || name == "known_stop"
			if allowed && err != nil {
				t.Fatal("independent permitted work denied", err)
			}
			if !allowed {
				assertConversationLoopStopped(t, err, "stopped_attempt_uncertain")
			}
		})
	}
}
