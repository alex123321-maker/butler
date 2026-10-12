package app

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/providerauth"
	"github.com/butler/butler/internal/transport"
	"github.com/butler/butler/internal/transport/antigravity"
)

type fullHighExpiryAuth struct{ entries atomic.Int32 }

func (a *fullHighExpiryAuth) ResolveAntigravity(context.Context) (providerauth.AntigravityAuth, error) {
	a.entries.Add(1)
	return providerauth.AntigravityAuth{AccessToken: "synthetic-expiry-test-only", ProjectID: "synthetic-expiry-project", ExpiresAt: time.Now().Add(time.Hour)}, nil
}

func (a *fullHighExpiryAuth) ResolveAntigravityCloudCode(ctx context.Context) (providerauth.AntigravityAuth, error) {
	return a.ResolveAntigravity(ctx)
}

// NewProvider clones DefaultTransport. Replace it only during construction and
// restore it before starting any goroutine; this test is deliberately serial.
// The cloned transport handles HTTPS over net.Pipe, so even the unfixed limiter
// can only reach this synthetic response, never a subscription or network host.
func fullHighExpiryProvider(t *testing.T, auth *fullHighExpiryAuth, httpEntries *atomic.Int32) *antigravity.Provider {
	t.Helper()
	synthetic := &http.Transport{DialTLSContext: func(context.Context, string, string) (net.Conn, error) {
		httpEntries.Add(1)
		client, server := net.Pipe()
		go func() {
			defer server.Close()
			_ = server.SetDeadline(time.Now().Add(5 * time.Second))
			request, err := http.ReadRequest(bufio.NewReader(server))
			if err != nil {
				return
			}
			request.Body.Close()
			const body = "data: {\"response\":{\"candidates\":[{\"index\":0,\"content\":{\"role\":\"model\",\"parts\":[{\"text\":\"synthetic\"}]},\"finishReason\":\"STOP\"}]}}\n\n"
			_, _ = fmt.Fprintf(server, "HTTP/1.1 200 OK\r\nContent-Type: text/event-stream\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s", len(body), body)
		}()
		return client, nil
	}}
	original := http.DefaultTransport
	http.DefaultTransport = synthetic
	provider, err := antigravity.NewProvider(antigravity.Config{Model: antigravity.FlashHighModel, Timeout: 5 * time.Second, AuthSource: auth})
	http.DefaultTransport = original
	if err != nil {
		t.Fatal("synthetic expiry provider unavailable")
	}
	return provider
}

func TestFullHighDelegationExpiryAfterUnchangedRowWaitIntegration(t *testing.T) {
	t.Setenv("BUTLER_DELEGATION_HIGH_ACCEPTANCE_WINDOW", "1")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	pool := memorytest.Database(t)
	limit, err := newNativeDelegationLimiter(ctx, pool)
	if err != nil {
		t.Fatal("expiry fixture limiter unavailable")
	}
	// Shorten only this isolated schema's deadline before the race. Neither the
	// deadline nor the locked row changes while the admission is waiting.
	if _, err = pool.Exec(ctx, `ALTER TABLE fixture_native_limit DISABLE TRIGGER fixture_full_high_guard; UPDATE fixture_native_limit SET expires_at=clock_timestamp()+interval '4 seconds'; ALTER TABLE fixture_native_limit ENABLE TRIGGER fixture_full_high_guard`); err != nil {
		t.Fatal("isolated expiry fixture unavailable")
	}
	blocker, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal("expiry row locker unavailable")
	}
	defer blocker.Rollback(context.Background())
	var unchanged int
	if blocker.QueryRow(ctx, `SELECT attempts FROM fixture_native_limit WHERE singleton FOR UPDATE`).Scan(&unchanged) != nil || unchanged != 27 {
		t.Fatal("unchanged expiry row lock unavailable")
	}
	auth := &fullHighExpiryAuth{}
	var httpEntries atomic.Int32
	provider := &limitedNativeDelegationProvider{Provider: fullHighExpiryProvider(t, auth, &httpEntries), limit: limit}
	request := transport.StartRunRequest{Context: transport.TransportRunContext{RunID: "synthetic-expiry-run", SessionKey: "synthetic-expiry-session", ProviderName: antigravity.Name, ModelName: antigravity.FlashHighModel}, InputItems: []transport.InputItem{{Role: "user", Content: "Synthetic expiry guard fixture"}}, StreamingEnabled: true}
	if request.Validate() != nil {
		t.Fatal("synthetic expiry request invalid")
	}
	done := make(chan error, 1)
	go func() {
		stream, entryErr := provider.StartRun(ctx, request)
		if stream != nil {
			for range stream {
			}
		}
		done <- entryErr
	}()
	// Pool size is two: inspect from the actual locker, not a third connection.
	// Refresh the transaction's statistics snapshot to observe the real wait.
	for {
		if _, err = blocker.Exec(ctx, `SELECT pg_stat_clear_snapshot()`); err != nil {
			t.Fatal("expiry wait observation unavailable")
		}
		var waiting, expired bool
		if blocker.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND pid<>pg_backend_pid() AND wait_event_type='Lock' AND pg_backend_pid()=ANY(pg_blocking_pids(pid)) AND query LIKE 'UPDATE fixture_native_limit%'), clock_timestamp()>=expires_at FROM fixture_native_limit WHERE singleton`).Scan(&waiting, &expired) != nil {
			t.Fatal("expiry lock evidence unavailable")
		}
		if waiting {
			if expired {
				t.Fatal("expiry admission did not begin before the deadline")
			}
			break
		}
		select {
		case <-done:
			t.Fatal("expiry admission did not wait on the actual row")
		case <-ctx.Done():
			t.Fatal("expiry row wait timed out")
		case <-time.After(10 * time.Millisecond):
		}
	}
	for {
		var expired bool
		if blocker.QueryRow(ctx, `SELECT clock_timestamp()>=expires_at FROM fixture_native_limit WHERE singleton`).Scan(&expired) != nil {
			t.Fatal("natural expiry observation unavailable")
		}
		if expired {
			break
		}
		select {
		case <-done:
			t.Fatal("expiry admission escaped the unchanged row lock")
		case <-ctx.Done():
			t.Fatal("natural expiry timed out")
		case <-time.After(10 * time.Millisecond):
		}
	}
	if blocker.Rollback(ctx) != nil {
		t.Fatal("unchanged expiry row release unavailable")
	}
	select {
	case entryErr := <-done:
		if entryErr != errNativeFixtureDenied {
			t.Errorf("post-wait expiry admission was not denied")
		}
	case <-ctx.Done():
		t.Fatal("post-wait expiry admission timed out")
	}
	var attempts, rows int
	if pool.QueryRow(ctx, `SELECT attempts,(SELECT count(*) FROM fixture_native_attempts) FROM fixture_native_limit WHERE singleton`).Scan(&attempts, &rows) != nil {
		t.Fatal("expiry spending evidence unavailable")
	}
	if attempts != 27 || rows != 0 || auth.entries.Load() != 0 || httpEntries.Load() != 0 {
		t.Errorf("expired admission changed isolated evidence: attempts=%d rows=%d auth_entries=%d http_entries=%d", attempts, rows, auth.entries.Load(), httpEntries.Load())
	}
}
