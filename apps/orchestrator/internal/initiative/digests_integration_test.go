package initiative

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/deliveryoutbox"
)

func digestFixture(t *testing.T, channel string) *fixture {
	t.Helper()
	f := setup(t)
	if channel != "web" {
		if _, err := f.pool.Exec(f.ctx, `UPDATE sessions SET channel=$2 WHERE session_key=$1`, f.source.SessionKey, channel); err != nil {
			t.Fatal(err)
		}
	}
	f.source.DigestChannel = channel
	if _, err := f.store.SaveSource(f.actor("digest-config"), f.source, 1); err != nil {
		t.Fatal(err)
	}
	f.source.Revision = 2
	c := f.errors(t)
	if _, err := f.promoter.Promote(f.ctx, c.ID, f.source.OwnerID); err != nil {
		t.Fatal(err)
	}
	return f
}

func TestConcurrentDigestsGroupChangesWithoutRawContentIntegration(t *testing.T) {
	f := digestFixture(t, "web")
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for range 8 {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- f.store.QueueDigests(f.ctx) }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var count, events int
	var content, status string
	if err := f.pool.QueryRow(f.ctx, `SELECT (SELECT count(*) FROM initiative_digest_outbox),(SELECT count(*) FROM initiative_digest_events),content,status FROM initiative_digest_outbox`).Scan(&count, &events, &content, &status); err != nil {
		t.Fatal(err)
	}
	if count != 1 || events != 1 || status != "result_ready" || !strings.Contains(content, "начато поручений — 1") || strings.Contains(content, f.source.TaskGoal) || strings.Contains(content, f.source.ProjectRef) {
		t.Fatal("incorrect digest scope", count, events, status, content)
	}
	v, err := f.store.View(f.ctx, f.source.OwnerID)
	if err != nil || len(v.Digests) != 1 || v.Digests[0].Status != "result_ready" {
		t.Fatal("digest missing from owner view", err)
	}
	v, err = f.store.View(f.ctx, "foreign-owner")
	if err != nil || len(v.Digests) != 0 {
		t.Fatal("foreign owner saw digest", err)
	}
	if err := f.store.QueueDigests(f.ctx); err != nil {
		t.Fatal(err)
	}
}

func TestDigestUnknownRecoveryDoesNotRepeatNetworkSendIntegration(t *testing.T) {
	f := digestFixture(t, "telegram")
	if err := f.store.QueueDigests(f.ctx); err != nil {
		t.Fatal(err)
	}
	d := DigestStore{Store: f.store}
	first, err := d.Claim(f.ctx, time.Minute)
	if err != nil || first == nil {
		t.Fatal(err)
	}
	if second, e := d.Claim(f.ctx, time.Minute); e != nil || second != nil {
		t.Fatal("active sending claim duplicated", e)
	}
	if _, err = f.pool.Exec(f.ctx, `UPDATE initiative_digest_outbox SET claim_until=clock_timestamp()-interval '1 second'`); err != nil {
		t.Fatal(err)
	}
	if next, e := d.Claim(f.ctx, time.Minute); e != nil || next != nil {
		t.Fatal("uncertain delivery automatically repeated", e)
	}
	var status string
	if err = f.pool.QueryRow(f.ctx, `SELECT status FROM initiative_digest_outbox`).Scan(&status); err != nil || status != "delivery_unknown" {
		t.Fatal("lost ACK not recorded", err, status)
	}
	if err = d.Finish(f.ctx, *first, deliveryoutbox.Outcome{Status: "sent", ACK: "late"}); err != deliveryoutbox.ErrClaimLost {
		t.Fatal("late claim rewrote uncertainty", err)
	}
}

func TestDigestChecksSourceAuthorityAndActiveHoursBeforeDeliveryIntegration(t *testing.T) {
	for _, kind := range []string{"revoked", "source_changed", "outside_active"} {
		t.Run(kind, func(t *testing.T) {
			f := digestFixture(t, "telegram")
			if err := f.store.QueueDigests(f.ctx); err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "revoked":
				if _, err := f.pool.Exec(f.ctx, `UPDATE action_mandates SET status='revoked',revoked_at=clock_timestamp() WHERE mandate_id=$1`, f.source.MandateID); err != nil {
					t.Fatal(err)
				}
			case "source_changed":
				f.source.Enabled = false
				if _, err := f.store.SaveSource(f.actor("disable-source"), f.source, 2); err != nil {
					t.Fatal(err)
				}
			case "outside_active":
				// Simulate queue recovery in the next inactive local hour, retaining the
				// pinned config revision. This fixture-only SQL is not a public mutation.
				if _, err := f.pool.Exec(f.ctx, `UPDATE initiative_sources SET source_json=jsonb_set(jsonb_set(source_json,'{active_start_hour}',to_jsonb(((extract(hour FROM clock_timestamp() AT TIME ZONE 'UTC')::int+1)%24)::int)),'{active_end_hour}',to_jsonb(((extract(hour FROM clock_timestamp() AT TIME ZONE 'UTC')::int+2)%24)::int))`); err != nil {
					t.Fatal(err)
				}
			}
			d := DigestStore{Store: f.store}
			m, err := d.Claim(f.ctx, time.Minute)
			if err != nil || m != nil {
				t.Fatal("stale/inactive digest sent", err)
			}
			var status string
			var attempts int
			if err = f.pool.QueryRow(f.ctx, `SELECT status,attempt_count FROM initiative_digest_outbox`).Scan(&status, &attempts); err != nil {
				t.Fatal(err)
			}
			expected := "failed"
			if kind == "outside_active" {
				expected = "pending"
			}
			if status != expected || attempts != 0 {
				t.Fatal("invalid pre-delivery outcome", status, attempts)
			}
		})
	}
}

func TestEmptyObservationsNeverCreateDigestIntegration(t *testing.T) {
	f := setup(t)
	f.source.DigestChannel = "web"
	if _, err := f.store.SaveSource(f.actor("digest-config"), f.source, 1); err != nil {
		t.Fatal(err)
	}
	e := testEvent(time.Now().UTC())
	e.Kind = "periodic"
	if _, _, err := f.store.Observe(f.ctx, f.source.OwnerID, e, 2, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if err := f.store.QueueDigests(f.ctx); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM initiative_digest_outbox`).Scan(&count); err != nil || count != 0 {
		t.Fatal("empty check produced notification", err)
	}
}
