package doctor

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/owneridentity"
)

func TestFeatureCompositionNeverGrantsAdmissionOrQuality(t *testing.T) {
	for _, tc := range []struct {
		name          string
		config        FeatureConfiguration
		state, reason string
	}{
		{"default", FeatureConfiguration{}, "disabled", "not_requested"},
		{"unqualified", FeatureConfiguration{Requested: true, Wired: true, ProfileReady: true}, "blocked", "release_not_qualified"},
		{"profile", FeatureConfiguration{Requested: true, Wired: true, Qualified: true}, "blocked", "profile_not_supported"},
		{"wiring", FeatureConfiguration{Requested: true, Qualified: true, ProfileReady: true}, "blocked", "composition_unavailable"},
		{"composed", FeatureConfiguration{Requested: true, Wired: true, Qualified: true, ProfileReady: true}, "composed", "current_authority_required"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.config.Name = "task_compaction"
			r := NewReporter(nil, nil)
			r.Configure(Options{Features: []FeatureConfiguration{tc.config}})
			report, err := r.Report(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			observed := report["effective_features"].(map[string]any)
			feature := observed["features"].([]map[string]any)[0]
			if feature["state"] != tc.state || feature["reason"] != tc.reason || observed["quality"] != "unverified" {
				t.Fatal(observed)
			}
			policies := observed["owner_policies"].(map[string]any)
			if policies["status"] != "unknown" || policies["reason"] != "owner_required" {
				t.Fatal(policies)
			}
			if _, ok := policies["observation"]; ok {
				t.Fatal("missing owner became empty policy set")
			}
			if report["answer_quality"].(map[string]any)["status"] != "unverified" {
				t.Fatal("composition became quality")
			}
		})
	}
}

func TestFeatureDigestUsesStableActualComposition(t *testing.T) {
	r := NewReporter(nil, nil)
	a := FeatureConfiguration{Name: "source_reads", Qualified: true, ProfileReady: true}
	b := FeatureConfiguration{Name: "candidate_jobs", Qualified: true}
	r.Configure(Options{Features: []FeatureConfiguration{a, b}})
	first, _ := r.Report(context.Background())
	r.Configure(Options{Features: []FeatureConfiguration{b, a, {Name: "PRIVATE_UNKNOWN_FEATURE"}}})
	second, _ := r.Report(context.Background())
	digest := func(report map[string]any) any { return report["answer_quality"].(map[string]any)["configuration"] }
	if digest(first) != digest(second) {
		t.Fatal("ordering or unknown labels changed supported composition")
	}
	a.Wired = true
	r.Configure(Options{Features: []FeatureConfiguration{a, b}})
	third, _ := r.Report(context.Background())
	if digest(first) == digest(third) {
		t.Fatal("actual composition missing from quality identity")
	}
}

func TestFeaturePoliciesObserveOwnerAndLiveControlsIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	ctx := owneridentity.WithActor(context.Background(), owneridentity.Actor{OwnerID: "doctor-owner", Channel: "web"})
	_, err := pool.Exec(ctx, `INSERT INTO memory_model_policies(policy_id,owner_id,policy,state,jobs_created) VALUES
	('private-active','doctor-owner','{"handler":"task-summary-v1","max_jobs":2,"job":{"deadline":"2099-01-01T00:00:00Z"}}','active',0),
	('private-expired','doctor-owner','{"handler":"task-summary-v1","max_jobs":1,"job":{"deadline":"2000-01-01T00:00:00Z"}}','paused',1),
	('private-other-handler','doctor-owner','{"handler":"PRIVATE_HANDLER","max_jobs":2,"job":{"deadline":"2099-01-01T00:00:00Z"}}','active',0),
	('private-foreign','foreign-owner','{"handler":"task-summary-v1","max_jobs":2,"job":{"deadline":"2099-01-01T00:00:00Z"}}','active',0)`)
	if err != nil {
		t.Fatal(err)
	}
	r := NewReporter(nil, nil)
	r.storage = &postgresReader{pool: pool}
	r.Configure(Options{Features: []FeatureConfiguration{{Name: "task_compaction", Requested: true, Wired: true, Qualified: true, ProfileReady: true, Handler: "task-summary-v1"}}})
	for _, state := range []string{"disabled", "enabled", "paused"} {
		if _, err = pool.Exec(ctx, `UPDATE memory_model_control SET state=$1 WHERE singleton`, state); err != nil {
			t.Fatal(err)
		}
		observed := r.featureReport(ctx, time.Now().UTC())
		policies := observed["owner_policies"].(map[string]any)
		if policies["status"] != "observed" || policies["reason"] != "metadata_only_not_admission" {
			t.Fatal(policies)
		}
		p := policies["observation"].(FeaturePolicies)
		c := p.Handlers["task-summary-v1"]
		if p.WorkerControl != state || len(p.Handlers) != 1 || c.Active != 1 || c.Paused != 1 || c.Expired != 1 || c.Exhausted != 1 {
			t.Fatal(p)
		}
		raw, _ := json.Marshal(observed)
		if strings.Contains(string(raw), "private-") || strings.Contains(string(raw), "foreign-owner") || strings.Contains(string(raw), "PRIVATE_HANDLER") {
			t.Fatal("private identity leaked")
		}
	}
	if _, err = pool.Exec(ctx, `ALTER TABLE memory_model_control RENAME TO unavailable_control`); err != nil {
		t.Fatal(err)
	}
	failed := r.featureReport(ctx, time.Now().UTC())["owner_policies"].(map[string]any)
	if failed["status"] != "failed" || failed["reason"] != "backend" {
		t.Fatal(failed)
	}
	if _, ok := failed["observation"]; ok {
		t.Fatal("backend failure became zeros")
	}
}
