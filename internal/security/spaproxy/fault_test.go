package spaproxy

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/butler/butler/internal/security/egress"
)

func relayClient(t *testing.T, p *Proxy) *http.Client {
	t.Helper()
	u, _ := url.Parse(p.URL())
	c := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(u), TLSClientConfig: pinnedTLS(p.SPKI), DisableKeepAlives: true}, Timeout: 3 * time.Second}
	t.Cleanup(c.CloseIdleConnections)
	return c
}

type changingResolver struct{ calls atomic.Int32 }

func (r *changingResolver) LookupIPAddr(context.Context, string) ([]net.IPAddr, error) {
	if r.calls.Add(1) == 1 {
		return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
	}
	return []net.IPAddr{{IP: net.ParseIP("169.254.169.254")}}, nil
}

func TestRelayDNSRebindingDoesNotReachSecondRecipient(t *testing.T) {
	var effects atomic.Int32
	base, _, _ := fixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { effects.Add(1); _, _ = io.WriteString(w, "ok") }))
	o := base.options
	base.Close()
	o.Policy.Resolver = &changingResolver{}
	p, err := New(context.Background(), o)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	client := relayClient(t, p)
	r, err := client.Get("https://external.test/first")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, r.Body)
	r.Body.Close()
	r, _ = client.Get("https://external.test/rebound")
	if r != nil {
		r.Body.Close()
	}
	_, failure := p.Snapshot()
	if failure == "" || effects.Load() != 1 {
		t.Fatal("rebinding reached recipient", failure, effects.Load())
	}
}

func TestRelaySPKIDoesNotBypassRemoteTLS(t *testing.T) {
	var effects atomic.Int32
	base, _, _ := fixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { effects.Add(1) }))
	o := base.options
	base.Close()
	o.Policy.TLSRoots = nil
	p, err := New(context.Background(), o)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	r, err := relayClient(t, p).Get("https://external.test/private")
	if r != nil {
		r.Body.Close()
	}
	_, failure := p.Snapshot()
	if err == nil || failure == "" || effects.Load() != 0 {
		t.Fatal("local trust pin disabled remote verification")
	}
}

func TestRelayPrivateForeignAndMetadataRedirectsStayDenied(t *testing.T) {
	for _, destination := range []string{"https://127.0.0.1/effect", "https://169.254.169.254/latest/meta-data/", "https://foreign.test/effect"} {
		t.Run(destination, func(t *testing.T) {
			var effects atomic.Int32
			p, client, _ := fixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				effects.Add(1)
				http.Redirect(w, r, destination, http.StatusFound)
			}))
			client.CheckRedirect = nil
			r, _ := client.Get("https://external.test/redirect")
			if r != nil {
				r.Body.Close()
			}
			_, failure := p.Snapshot()
			if effects.Load() != 1 || failure != "destination_denied" {
				t.Fatal("redirect bypassed relay", effects.Load(), failure)
			}
		})
	}
}

func TestRelayOversizeAndCompressedExpansionAreTerminal(t *testing.T) {
	for _, mode := range []string{"oversize", "compressed"} {
		t.Run(mode, func(t *testing.T) {
			p, client, _ := fixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Accept-Encoding") != "identity" {
					t.Error("relay requested an unbounded decompressor")
				}
				if mode == "compressed" {
					w.Header().Set("Content-Encoding", "br")
					_, _ = io.WriteString(w, "encoded")
					return
				}
				_, _ = io.Copy(w, strings.NewReader(strings.Repeat("x", 5*1024*1024)))
			}))
			r, err := client.Get("https://external.test/large")
			if r != nil {
				_, err = io.Copy(io.Discard, r.Body)
				r.Body.Close()
			}
			stats, failure := p.Snapshot()
			if err == nil || failure == "" || stats.MeteredBytes > egress.MaxRequestBytes {
				t.Fatal("unbounded response accepted", failure)
			}
			r, err = client.Get("https://external.test/retry")
			if r != nil {
				r.Body.Close()
			}
			if err == nil {
				t.Fatal("exhausted relay resumed")
			}
		})
	}
}

func TestRelayIdleInfiniteStreamExpiresWithoutFollowup(t *testing.T) {
	closed := make(chan struct{})
	base, _, _ := fixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: first\n\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
		close(closed)
	}))
	o := base.options
	base.Close()
	o.Deadline = time.Now().Add(250 * time.Millisecond)
	p, err := New(context.Background(), o)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	r, err := relayClient(t, p).Get("https://external.test/infinite")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Body.Close()
	if _, err = io.Copy(io.Discard, r.Body); err == nil {
		t.Fatal("deadline-truncated stream had a clean EOF")
	}
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("idle stream survived independent expiry")
	}
}
