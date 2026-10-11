package egress

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestScopedProxyExpiryTimerInitializationIsSynchronized(t *testing.T) {
	var dials atomic.Int32
	proxy, err := NewScopedProxy(proxyFixturePolicy(t, "127.0.0.1:1", &dials), "http://external.test", time.Now().Add(30*time.Millisecond), 100)
	if err != nil {
		t.Fatal(err)
	}
	defer proxy.Close()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		proxy.mu.Lock()
		closed := proxy.closed
		proxy.mu.Unlock()
		if closed {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("independent expiry timer did not close proxy")
}
