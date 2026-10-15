package orchestrator

import (
	"context"
	"testing"

	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/actionauth"
	memoryservice "github.com/butler/butler/internal/memory/service"
)

type memoryPolicyTasks struct {
	task.Manager
	record task.Record
}

func (t *memoryPolicyTasks) Get(context.Context, string) (task.Record, error) { return t.record, nil }

type memoryPolicyAuthority struct{ privacy string }

func (a *memoryPolicyAuthority) ReadTaskAuthority(context.Context, string) (actionauth.Assignment, actionauth.Mandate, actionauth.GoalPolicy, error) {
	return actionauth.Assignment{TaskID: "task", MandateRevision: 1}, actionauth.Mandate{ID: "mandate", Revision: 1, Privacy: a.privacy}, actionauth.GoalPolicy{}, nil
}

type memoryPolicyBundles struct {
	request memoryservice.BundleRequest
	after   func()
}

func (b *memoryPolicyBundles) BuildBundle(_ context.Context, r memoryservice.BundleRequest) (memoryservice.Bundle, error) {
	b.request = r
	if b.after != nil {
		b.after()
	}
	return memoryservice.Bundle{Items: map[string]any{"chunks": []map[string]any{{"summary": "allowed local evidence"}}}}, nil
}

func TestAssignedLocalOnlyTaskRetainsLocalQueryAndRechecksAuthority(t *testing.T) {
	for _, privacy := range []string{"local_only", "private", "cloud_allowed"} {
		t.Run(privacy, func(t *testing.T) {
			tasks := &memoryPolicyTasks{record: task.Record{TaskID: "task", SessionKey: "session", OwnerID: "owner", Status: "active", Revision: 1, ConstraintsRevision: 1}}
			a := &memoryPolicyAuthority{privacy: privacy}
			b := &memoryPolicyBundles{}
			s := &Service{config: Config{Tasks: tasks, OwnerGoals: a, MemoryBundles: b}}
			prepared := &preparedRun{InputPayload: map[string]any{"task_id": "task"}, MemoryBundle: map[string]any{}}
			if err := s.attachMemoryContext(context.Background(), "session", "owner", "query", prepared); err != nil {
				t.Fatal(err)
			}
			if b.request.QueryPolicy == nil || !b.request.QueryPolicy.Local || b.request.QueryPolicy.External != (privacy == "cloud_allowed") {
				t.Fatal("local and release policy conflated", b.request)
			}
			b.after = func() { tasks.record.Revision++ }
			if err := s.attachMemoryContext(context.Background(), "session", "owner", "query", prepared); err == nil {
				t.Fatal("task amendment during search accepted")
			}
			b.after = func() { a.privacy = "local_only" }
			if privacy == "cloud_allowed" {
				if err := s.attachMemoryContext(context.Background(), "session", "owner", "query", prepared); err == nil {
					t.Fatal("release withdrawal during search accepted")
				}
			}
		})
	}
}

func TestOptionalContextTrimsOneItemAndPreservesDocument(t *testing.T) {
	b := map[string]any{"profile": []map[string]any{{"key": "language", "summary": "ru"}, {"key": "style", "summary": "long"}}, "chunks": []map[string]any{{"summary": "pickup code"}}, "working": map[string]any{"goal": "do not send"}}
	key, ok := removeOptionalMemoryItem(b)
	if !ok || key != "profile" || len(b["profile"].([]map[string]any)) != 1 || b["working"] == nil || b["chunks"] == nil {
		t.Fatal("section-wide loss or mandatory trim", b)
	}
}
