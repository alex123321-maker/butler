package maintenance

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestMaintenanceDurableGuardsIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	ctx := context.Background()
	_, file, _, _ := runtime.Caller(0)
	dir := filepath.Join(filepath.Dir(file), "..", "..", "migrations")
	for _, suffix := range []string{"down", "up"} {
		raw, err := os.ReadFile(filepath.Join(dir, "105_independent_maintenance."+suffix+".sql"))
		if err != nil {
			t.Fatal(err)
		}
		if _, err = pool.Exec(ctx, string(raw)); err != nil {
			t.Fatal("empty maintenance migration rollback/up failed", suffix, err)
		}
	}
	execute := func(query string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	denied := func(code, query string, args ...any) {
		t.Helper()
		_, err := pool.Exec(ctx, query, args...)
		var pgerr *pgconn.PgError
		if !errors.As(err, &pgerr) || pgerr.Code != code {
			t.Fatal("durable guard missing", code, err)
		}
	}
	profile := []byte(`{"synthetic_profile":"a"}`)
	profileID := Digest(profile)
	execute(`INSERT INTO maintenance_profiles(profile_sha256,owner_id,target_id,raw) VALUES($1,'owner-a','target-a',$2)`, profileID, profile)
	execute(`INSERT INTO maintenance_targets(owner_id,target_id,profile_sha256) VALUES('owner-a','target-a',$1)`, profileID)
	var stopped bool
	var epoch int64
	if err := pool.QueryRow(ctx, `SELECT stopped,epoch FROM maintenance_targets WHERE owner_id='owner-a'`).Scan(&stopped, &epoch); err != nil || !stopped || epoch != 1 {
		t.Fatal("new maintenance target was active", err)
	}
	binding := []byte(`{"synthetic_binding":"a"}`)
	stageID := "mstage_" + Digest(binding)
	execute(`INSERT INTO maintenance_stages(stage_id,owner_id,target_id,profile_sha256,binding,source_inventory) VALUES($1,'owner-a','target-a',$2,$3,'[]')`, stageID, profileID, binding)
	review := []byte(`{"synthetic_review":"a"}`)
	reviewID := Digest(review)
	// These synthetic SQL-only bytes exercise storage constraints. They are not
	// a cryptographic PASS or deployment authority; VerifyReview tests are separate.
	signature := make([]byte, 64)
	execute(`INSERT INTO maintenance_reviews(review_sha256,stage_id,raw,signature) VALUES($1,$2,$3,$4)`, reviewID, stageID, review, signature)
	permission := []byte(`{"synthetic_permission":"a"}`)
	permissionID := "mpermit_" + Digest(permission)
	execute(`INSERT INTO maintenance_permissions(permission_id,owner_id,target_id,profile_sha256,kind,raw,signature,limit_claims,expires_at) VALUES($1,'owner-a','target-a',$2,'ordinary_mandate',$3,$4,2,clock_timestamp()+INTERVAL '1 hour')`, permissionID, profileID, permission, signature)
	attemptID := "mdeploy_" + Digest([]byte("synthetic attempt a"))
	execute(`INSERT INTO maintenance_attempts(attempt_id,owner_id,target_id,profile_sha256,stage_id,review_sha256,permission_id,purpose,fence,epoch,base_artifact_sha256,artifact_sha256,claim,signature,expires_at) VALUES($1,'owner-a','target-a',$2,$3,$4,$5,'promote',1,1,$6,$7,'{}',$8,clock_timestamp()+INTERVAL '1 minute')`, attemptID, profileID, stageID, reviewID, permissionID, Digest([]byte("base")), Digest([]byte("candidate")), signature)
	denied("55000", `UPDATE maintenance_attempts SET state='completed',dispatched_at=clock_timestamp(),ended_at=clock_timestamp() WHERE attempt_id=$1`, attemptID)
	execute(`UPDATE maintenance_attempts SET state='dispatching',dispatched_at=clock_timestamp() WHERE attempt_id=$1`, attemptID)
	execute(`UPDATE maintenance_attempts SET state='unknown',ended_at=clock_timestamp() WHERE attempt_id=$1`, attemptID)
	denied("55000", `UPDATE maintenance_attempts SET state='pending',dispatched_at=NULL,ended_at=NULL WHERE attempt_id=$1`, attemptID)
	denied("55000", `UPDATE maintenance_attempts SET state='completed' WHERE attempt_id=$1`, attemptID)
	denied("55000", `DELETE FROM maintenance_attempts WHERE attempt_id=$1`, attemptID)
	execute(`UPDATE maintenance_targets SET unresolved_attempt=$1 WHERE owner_id='owner-a'`, attemptID)
	denied("55000", `UPDATE maintenance_permissions SET used_claims=2 WHERE permission_id=$1`, permissionID)
	execute(`UPDATE maintenance_permissions SET used_claims=1 WHERE permission_id=$1`, permissionID)
	denied("55000", `UPDATE maintenance_permissions SET used_claims=0 WHERE permission_id=$1`, permissionID)
	execute(`UPDATE maintenance_permissions SET revoked=true WHERE permission_id=$1`, permissionID)
	denied("55000", `UPDATE maintenance_permissions SET revoked=false WHERE permission_id=$1`, permissionID)
	denied("55000", `UPDATE maintenance_permissions SET kind='exact_protected' WHERE permission_id=$1`, permissionID)
	denied("55000", `UPDATE maintenance_targets SET epoch=0 WHERE owner_id='owner-a'`)
	denied("55000", `UPDATE maintenance_targets SET next_fence=0 WHERE owner_id='owner-a'`)
	denied("55000", `UPDATE maintenance_targets SET stopped=false WHERE owner_id='owner-a'`)
	execute(`UPDATE maintenance_targets SET stopped=false,epoch=epoch+1 WHERE owner_id='owner-a'`)
	execute(`UPDATE maintenance_targets SET stopped=true,epoch=epoch+1 WHERE owner_id='owner-a'`)
	execute(`INSERT INTO maintenance_events(owner_id,target_id,kind,reference,epoch) VALUES('owner-a','target-a','stop','synthetic owner stop',3)`)
	for _, query := range []string{
		`UPDATE maintenance_profiles SET raw='{}'`, `DELETE FROM maintenance_profiles`,
		`UPDATE maintenance_stages SET source_inventory='{}'`, `DELETE FROM maintenance_stages`,
		`UPDATE maintenance_reviews SET raw='{}'`, `DELETE FROM maintenance_reviews`,
		`UPDATE maintenance_events SET reference='rewritten'`, `DELETE FROM maintenance_events`,
	} {
		denied("55000", query)
	}
	otherProfile := []byte(`{"synthetic_profile":"b"}`)
	otherID := Digest(otherProfile)
	execute(`INSERT INTO maintenance_profiles(profile_sha256,owner_id,target_id,raw) VALUES($1,'owner-b','target-b',$2)`, otherID, otherProfile)
	execute(`INSERT INTO maintenance_targets(owner_id,target_id,profile_sha256) VALUES('owner-b','target-b',$1)`, otherID)
	denied("23503", `UPDATE maintenance_targets SET unresolved_attempt=$1 WHERE owner_id='owner-b'`, attemptID)
	denied("55000", `UPDATE maintenance_targets SET owner_id='owner-b' WHERE owner_id='owner-a'`)
	down, err := os.ReadFile(filepath.Join(dir, "105_independent_maintenance.down.sql"))
	if err != nil {
		t.Fatal(err)
	}
	denied("55000", string(down))
	var state string
	if err = pool.QueryRow(ctx, `SELECT state FROM maintenance_attempts WHERE attempt_id=$1`, attemptID).Scan(&state); err != nil || state != "unknown" {
		t.Fatal("failed schema removal erased unknown history", err)
	}
}
