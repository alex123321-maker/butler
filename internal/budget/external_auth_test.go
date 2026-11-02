package budget

import (
	"context"
	"encoding/json"
	"testing"
)

func TestExternalAuthenticationUnknownFencesNewTaskInConversation(t *testing.T) {
	pool := controllerDB(t)
	ctx := context.Background()
	if _, e := pool.Exec(ctx, `INSERT INTO tasks(task_id,owner_id,session_key) VALUES('new-task','owner','session');INSERT INTO runs VALUES('new-run','session','new-task',1,'model_running');INSERT INTO sessions VALUES('other-session','owner'),('foreign-session','foreign');INSERT INTO tasks(task_id,owner_id,session_key) VALUES('other-task','owner','other-session'),('foreign-task','foreign','foreign-session');INSERT INTO runs VALUES('other-run','other-session','other-task',1,'model_running'),('foreign-run','foreign-session','foreign-task',1,'model_running');`); e != nil {
		t.Fatal(e)
	}
	b, _ := json.Marshal(map[string]string{"owner_id": "owner", "task_id": "root", "run_id": "run", "effect": "commit"})
	if _, e := pool.Exec(ctx, `INSERT INTO action_effects(action_id,fingerprint,initial_binding,policy,state) VALUES('external-effect','fixture',$1,'{"verification_method":"external_google_rp_v1"}','effect_unknown')`, b); e != nil {
		t.Fatal(e)
	}
	for _, test := range []struct {
		name, run, owner, root, effect string
		blocked                        bool
	}{
		{"new-task", "new-run", "owner", "new-task", "commit", true},
		{"credential-prepare", "new-run", "owner", "new-task", "prepare", true},
		{"read", "new-run", "owner", "new-task", "read", false},
		{"separate-conversation", "other-run", "owner", "other-task", "commit", false},
		{"foreign-owner", "foreign-run", "foreign", "foreign-task", "commit", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			tx, e := pool.Begin(ctx)
			if e != nil {
				t.Fatal(e)
			}
			defer tx.Rollback(ctx)
			e = externalAuthUnknownGuardTx(ctx, tx, ToolIdentity{RunID: test.run, Effect: test.effect, ToolName: "browser.page_click"}, test.owner, test.root)
			if (e != nil) != test.blocked {
				t.Fatal("wrong cross-Input fence", e)
			}
		})
	}
	if _, e := pool.Exec(ctx, `UPDATE action_effects SET state='effect_absent' WHERE action_id='external-effect'`); e != nil {
		t.Fatal(e)
	}
	tx, e := pool.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	if e = externalAuthUnknownGuardTx(ctx, tx, ToolIdentity{RunID: "new-run", Effect: "commit"}, "owner", "new-task"); e != nil {
		t.Fatal("settled history blocked unrelated new task", e)
	}
}
