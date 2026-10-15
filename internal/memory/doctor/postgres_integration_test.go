package doctor

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/metrics"
)

func TestDurableMemoryObservationsRecoverAndFailWithoutZeros(t *testing.T) {
	pool := memorytest.Database(t)
	ctx := context.Background()
	_, err := pool.Exec(ctx, `INSERT INTO memory_aux_jobs(job_id,owner_id,kind,purpose,sources,source_fingerprint,allowance,deadline,created_at) VALUES('job-owner','PRIVATE_OWNER','index','test','[]','fingerprint','{}',clock_timestamp()+INTERVAL '1 hour',clock_timestamp()-INTERVAL '2 minutes'),('job-other','OTHER_OWNER','index','test','[]','other','{}',clock_timestamp()+INTERVAL '1 hour',clock_timestamp());INSERT INTO semantic_index_generations(id,space_id,space_data,dimensions,scope_type,scope_id) VALUES('generation','space','{}',768,'user','PRIVATE_OWNER')`)
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		reader := &postgresReader{pool: pool}
		j, err := reader.Jobs(ctx, "PRIVATE_OWNER")
		if err != nil || j.Pending != 1 || j.OldestPendingSeconds == nil || *j.OldestPendingSeconds < 119 {
			t.Fatal(j, err)
		}
		g, truncated, err := reader.Indexes(ctx, "PRIVATE_OWNER")
		if err != nil || truncated || len(g) != 1 || g[0].State != "building" || g[0].Active || g[0].VerifiedAt != nil {
			t.Fatal(g, err)
		}
		m := metrics.New()
		if err = RegisterMetrics(m, pool); err != nil {
			t.Fatal(err)
		}
		w := httptest.NewRecorder()
		m.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/metrics", nil))
		out := w.Body.String()
		if !strings.Contains(out, "butler_memory_durable_pending 2") || strings.Contains(out, "PRIVATE_OWNER") || strings.Contains(out, "job-owner") {
			t.Fatal("durable metrics missing or private")
		}
	}
	if _, err = pool.Exec(ctx, `UPDATE semantic_index_generations SET retired_at=clock_timestamp(),retired_by='PRIVATE_OWNER' WHERE id='generation'`); err != nil {
		t.Fatal(err)
	}
	retired, _, err := (&postgresReader{pool: pool}).Indexes(ctx, "PRIVATE_OWNER")
	if err != nil || len(retired) != 1 || retired[0].State != "retired" || retired[0].Active {
		t.Fatal("retired derivative misreported as reusable", retired, err)
	}
	if _, err = pool.Exec(ctx, `ALTER TABLE memory_aux_jobs RENAME TO temporarily_unavailable_jobs`); err != nil {
		t.Fatal(err)
	}
	m := metrics.New()
	_ = RegisterMetrics(m, pool)
	w := httptest.NewRecorder()
	m.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/metrics", nil))
	out := w.Body.String()
	if !strings.Contains(out, "butler_memory_durable_observation_success 0") || strings.Contains(out, "butler_memory_durable_pending ") {
		t.Fatal("missing observation became zero backlog", out)
	}
}
