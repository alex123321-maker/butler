package communication_test

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/api"
	"github.com/butler/butler/apps/orchestrator/internal/communication"
	"github.com/butler/butler/internal/communicationcontrol"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/jackc/pgx/v5"
)

// A content-free SSE pulse must not reread source graphs or wait for a
// transcript writer. REST remains responsible for releasing current content.
func TestCommunicationWebSSEDoesNotRetainTranscriptLocksIntegration(t *testing.T) {
	f, w, image, document := webFixture(t)
	if _, err := f.publish(t, "sse-retained-report", "communication.report", webReport(image, document)); err != nil {
		t.Fatal(err)
	}
	var sourceReads atomic.Int32
	guard := f.s.SourceGuard
	f.s.SourceGuard = func(ctx context.Context, tx pgx.Tx, run string) error {
		sourceReads.Add(1)
		return guard(ctx, tx, run)
	}
	tx, err := f.pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(f.ctx, `SELECT 1 FROM sessions WHERE session_key=$1 FOR UPDATE`, f.p.SessionKey); err != nil {
		t.Fatal(err)
	}
	token := strings.Repeat("s", 40)
	server := httptest.NewServer(api.CommunicationWeb(api.NewOwnerControlAuth(token, f.p.OwnerID), w, nil, nil))
	defer server.Close()
	ctx, cancel := context.WithTimeout(f.ctx, time.Second)
	defer cancel()
	request, _ := http.NewRequestWithContext(ctx, "GET", server.URL+"/api/v3/communications/events", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal("SSE waited for a transcript writer", err)
	}
	defer response.Body.Close()
	buffer := bufio.NewReader(response.Body)
	var frame strings.Builder
	for {
		line, err := buffer.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		frame.WriteString(line)
		if line == "\n" {
			break
		}
	}
	if response.StatusCode != 200 || !strings.Contains(frame.String(), "event: refresh") || sourceReads.Load() != 0 {
		t.Fatal("SSE read private content instead of scoped invalidation", response.StatusCode, sourceReads.Load())
	}
	if strings.Contains(frame.String(), "publication:") || strings.Contains(frame.String(), "artifact:") || strings.Contains(frame.String(), "Полный") {
		t.Fatal("SSE exposed retained data")
	}
	cancel()
}

func TestCommunicationWebCursorPageDoesNotWaitForLaterHistoryIntegration(t *testing.T) {
	f, w, _, _ := webFixture(t)
	wanted := map[string]bool{}
	for i, text := range []string{"first bounded publication", "slow later publication", "last bounded publication"} {
		raw, _ := json.Marshal(communicationcontrol.Args{SchemaVersion: 1, Purpose: "result", Text: text})
		r, err := f.publish(t, "bounded-page-"+strconv.Itoa(i), "communication.message", string(raw))
		if err != nil {
			t.Fatal(err)
		}
		wanted[r.PublicationRef] = true
	}
	var firstPage atomic.Bool
	firstPage.Store(true)
	guard := f.s.ChannelGuard
	f.s.ChannelGuard = func(ctx context.Context, tx pgx.Tx, run string, b communication.Binding, a communicationcontrol.Args) error {
		if firstPage.Load() && a.Text == "slow later publication" {
			<-ctx.Done()
			return ctx.Err()
		}
		return guard(ctx, tx, run, b, a)
	}
	token := strings.Repeat("s", 40)
	handler := api.CommunicationWeb(api.NewOwnerControlAuth(token, f.p.OwnerID), w, nil, nil)
	after := int64(0)
	seen := map[string]bool{}
	for n := range 3 {
		ctx, cancel := context.WithTimeout(f.ctx, time.Second)
		request := httptest.NewRequest("GET", "/api/v3/communications?cursor="+strconv.FormatInt(after, 10), nil).WithContext(ctx)
		request.Header.Set("Authorization", "Bearer "+token)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		cancel()
		if response.Code != 200 {
			t.Fatal("a later history proof blocked the first cursor page", response.Code)
		}
		firstPage.Store(false)
		var page communication.WebPage
		if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil || len(page.Items) != 1 || page.Cursor <= after || page.HasMore != (n < 2) {
			t.Fatal("bounded cursor lost history or progress", n, page, err)
		}
		p := page.Items[0]
		if !wanted[p.Ref] || seen[p.Ref] || !p.Available || p.Payload == nil {
			t.Fatal("missing, repeated or unavailable publication", p.Ref)
		}
		seen[p.Ref] = true
		after = page.Cursor
		// Returned pages must release their complete proof; a later request
		// cannot keep this session's owning writer behind historical locks.
		ctx, cancel = context.WithTimeout(f.ctx, time.Second)
		tx, err := f.pool.Begin(ctx)
		if err == nil {
			_, err = tx.Exec(ctx, `SELECT 1 FROM sessions WHERE session_key=$1 FOR UPDATE`, f.p.SessionKey)
			_ = tx.Rollback(context.Background())
		}
		cancel()
		if err != nil {
			t.Fatal("history page retained the owning writer lock", err)
		}
	}
	if len(seen) != len(wanted) {
		t.Fatal("history truncated by server page size")
	}
}

func TestCommunicationWebRefreshCursorScopeAndRevocationIntegration(t *testing.T) {
	for _, kind := range []string{"current", "future-cursor", "wrong-owner", "wrong-recipient", "binding-disabled", "binding-rebound", "source-revoked"} {
		t.Run(kind, func(t *testing.T) {
			f, w, image, document := webFixture(t)
			if _, err := f.publish(t, "cursor-report", "communication.report", webReport(image, document)); err != nil {
				t.Fatal(err)
			}
			ctx := f.actor("web", "cursor")
			after := int64(0)
			var err error
			switch kind {
			case "future-cursor":
				after = 1 << 50
			case "wrong-owner":
				ctx = owneridentity.WithActor(ctx, owneridentity.Actor{OwnerID: "other-owner", Channel: "web", SessionKey: w.Binding.Recipient})
			case "wrong-recipient":
				ctx = owneridentity.WithActor(ctx, owneridentity.Actor{OwnerID: f.p.OwnerID, Channel: "web", SessionKey: "web:other"})
			case "binding-disabled":
				_, err = f.pool.Exec(ctx, `UPDATE communication_bindings SET enabled=FALSE WHERE binding_id=$1`, w.Binding.ID)
			case "binding-rebound":
				_, err = f.pool.Exec(ctx, `UPDATE communication_bindings SET version=version+1 WHERE binding_id=$1`, w.Binding.ID)
			case "source-revoked":
				f.revoked.Store(true)
			}
			if err != nil {
				t.Fatal(err)
			}
			cursor, err := w.RefreshCursor(ctx, after)
			denied := kind == "wrong-owner" || kind == "wrong-recipient" || kind == "binding-disabled" || kind == "binding-rebound"
			if denied {
				if err == nil || cursor != 0 {
					t.Fatal("cursor crossed current owner binding", cursor, err)
				}
				return
			}
			if err != nil || cursor < after || after == 0 && cursor == 0 {
				t.Fatal("scoped cursor did not advance monotonically", cursor, err)
			}
			if kind == "source-revoked" {
				page, err := w.Feed(ctx, 0, 50)
				if err != nil || len(page.Items) != 1 || page.Items[0].Available || page.Items[0].Payload != nil || len(page.Items[0].Parts) != 0 {
					t.Fatal("invalidation authorized revoked content", err)
				}
			}
		})
	}
}
