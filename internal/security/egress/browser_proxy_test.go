package egress

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"
)

func TestBrowserProxyPublicOriginsRetainPrivateAndOperatorCaps(t *testing.T) {
	var dials atomic.Int32
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, r.Host) }))
	defer site.Close()
	target, _ := url.Parse(site.URL)
	for _, capped := range []bool{false, true} {
		policy := proxyFixturePolicy(t, target.Host, &dials)
		if capped {
			policy.AllowedDomains = []string{"first.test"}
		}
		proxy, err := NewBrowserProxy(policy, "http://first.test", time.Now().Add(time.Minute), MaxRequestBytes)
		if err != nil {
			t.Fatal(err)
		}
		proxyURL, _ := url.Parse(proxy.URL())
		client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)}}
		for _, destination := range []string{"http://first.test/", "http://second.test/", "http://127.0.0.1/private", "http://169.254.169.254/metadata", "http://[::1]/private"} {
			before := dials.Load()
			resp, err := client.Get(destination)
			if err != nil {
				t.Fatal(err)
			}
			resp.Body.Close()
			allowed := destination == "http://first.test/" || destination == "http://second.test/" && !capped
			if (resp.StatusCode == 200) != allowed || !allowed && dials.Load() != before {
				t.Fatalf("destination %s cap=%v status=%d", destination, capped, resp.StatusCode)
			}
		}
		proxy.Freeze()
		resp, err := client.Get("http://first.test/")
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != 403 {
			t.Fatal("freeze lost across origins")
		}
		client.CloseIdleConnections()
		proxy.Close()
	}
}
