package app

import (
	"context"
	"github.com/jackc/pgx/v5"
	"sync/atomic"
	"testing"
	"time"
)

func TestBehavior76LocalUnknownWaitsForExactBrokerBackendIntegration(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	var global atomic.Bool
	f := newBehavior76LocalFixture(t, ctx, &global)
	if !f.brokerObserved || len(f.brokerApplication) > 63 {
		t.Fatal("broker identity missing or truncated")
	}
	config := f.pool.Config().ConnConfig.Copy()
	config.RuntimeParams["application_name"] = f.brokerApplication
	// A real extra backend with the exact observed application identity delays
	// teardown proof. It grants nothing and changes no fixture state.
	connection, err := pgx.ConnectConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close(context.Background())
	var actualName string
	if err = connection.QueryRow(ctx, `SELECT current_setting('application_name')`).Scan(&actualName); err != nil || actualName != f.brokerApplication {
		t.Fatal("server did not retain exact bounded identity", err)
	}
	brokerReaped := make(chan struct{})
	stopBroker := f.stopBroker
	var observed atomic.Bool
	f.stopBroker = func(ctx context.Context) error {
		err := stopBroker(ctx)
		if observed.CompareAndSwap(false, true) {
			close(brokerReaped)
		}
		return err
	}
	type result struct {
		receipt behavior76Quiescence
		err     error
	}
	finished := make(chan result, 1)
	go func() { receipt, err := f.quiesce(ctx); finished <- result{receipt, err} }()
	select {
	case <-brokerReaped:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	select {
	case r := <-finished:
		t.Fatal("quiescence passed while exact backend remained", r.err)
	case <-time.After(80 * time.Millisecond):
	}
	if err = connection.Close(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case r := <-finished:
		if r.err != nil || !r.receipt.BrokerObserved || r.receipt.BrokerBackends != 0 {
			t.Fatal("exact backend teardown receipt missing", r.err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	t.Log("actual bounded PostgreSQL application name read back exactly; process Wait alone could not pass while its exact backend identity remained; native/auth=0")
}
