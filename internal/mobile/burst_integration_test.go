package mobile

import (
	"fmt"
	"testing"
	"time"
)

func TestMobileThousandEventBurstIntegration(t *testing.T) {
	s, ctx, id, token := fixture(t)
	started := time.Now()
	for batch := 0; batch < 20; batch++ {
		b := testBatch()
		template := b.Events[0]
		b.ID = fmt.Sprintf("burst_batch_%08d", batch)
		b.Events = nil
		for i := 0; i < 50; i++ {
			event := template
			event.ID = fmt.Sprintf("burst_event_%08d", batch*50+i)
			event.Key = fmt.Sprintf("card_%d", i)
			b.Events = append(b.Events, event)
		}
		a, err := s.Accept(ctx, id, token, b)
		if err != nil || len(a.Accepted) != 50 {
			t.Fatal("burst commit", err)
		}
		a, err = s.Accept(ctx, id, token, b)
		if err != nil || len(a.Duplicate) != 50 {
			t.Fatal("burst replay", err)
		}
	}
	var count, runs int
	if err := s.Pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM mobile_events),(SELECT count(*) FROM runs)`).Scan(&count, &runs); err != nil || count != 1000 || runs != 0 {
		t.Fatal(count, runs, err)
	}
	t.Logf("1000 durable events plus 1000 replays in %s; zero model runs", time.Since(started))
}
