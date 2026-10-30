package browsercontrol

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func newPageTestID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		panic("test randomness unavailable")
	}
	return hex.EncodeToString(value[:])
}

func pageTestMigration(t *testing.T, pool *pgxpool.Pool, name string) error {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "migrations", name))
	if err != nil {
		t.Fatal("migration file unavailable")
	}
	_, err = pool.Exec(t.Context(), string(raw))
	return err
}

func pageDB(t *testing.T) *Store {
	t.Helper()
	dsn := os.Getenv("BUTLER_TEST_BROWSER_PAGES_POSTGRES_URL")
	if dsn == "" {
		t.Skip("requires isolated browser_pages_test PostgreSQL")
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil || !strings.HasSuffix(cfg.ConnConfig.Database, "_test") {
		t.Fatal("disposable *_test PostgreSQL required")
	}
	cfg.MaxConns = 8
	admin, err := pgxpool.NewWithConfig(t.Context(), cfg)
	if err != nil {
		t.Fatal("test PostgreSQL unavailable")
	}
	schema := "pages_test_" + newPageTestID()
	if _, err = admin.Exec(t.Context(), `CREATE SCHEMA `+schema); err != nil {
		admin.Close()
		t.Fatal("cannot create isolated page schema")
	}
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_, _ = admin.Exec(cleanup, `DROP SCHEMA `+schema+` CASCADE`)
		admin.Close()
	})
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	cfg.ConnConfig.RuntimeParams["application_name"] = schema
	pool, err := pgxpool.NewWithConfig(t.Context(), cfg)
	if err != nil {
		t.Fatal("page test pool unavailable")
	}
	t.Cleanup(pool.Close)
	// Exercise the real owning control schema; unrelated Run columns are unused.
	if _, err = pool.Exec(t.Context(), `CREATE TABLE runs(run_id TEXT PRIMARY KEY); INSERT INTO runs VALUES('run'),('run2')`); err != nil {
		t.Fatal("Run fixture unavailable")
	}
	if pageTestMigration(t, pool, "086_server_browser_control.up.sql") != nil || pageTestMigration(t, pool, "107_browser_pages.up.sql") != nil {
		t.Fatal("page/control migrations failed")
	}
	return &Store{Pool: pool}
}

func pageSession(t *testing.T, s *Store) PageContext {
	t.Helper()
	p := PageContext{SessionID: newPageTestID(), RunID: "run", OwnerID: "owner", TaskID: "task"}
	if _, err := s.Pool.Exec(t.Context(), `INSERT INTO server_browser_sessions(browser_session_id,run_id,owner_id,task_id,expires_at) VALUES($1,$2,$3,$4,clock_timestamp()+interval '15 minutes')`, p.SessionID, p.RunID, p.OwnerID, p.TaskID); err != nil {
		t.Fatal("cannot create owning context")
	}
	return p
}

func readyPage(t *testing.T, s *Store, p PageContext, parent *PageRef, target string) PageState {
	t.Helper()
	ref, err := s.RegisterPage(t.Context(), p, newPageTestID(), parent)
	if err != nil {
		t.Fatal("cannot register page")
	}
	state, err := s.ObservePage(t.Context(), p, ref, newPageTestID(), target)
	if err != nil {
		t.Fatal("cannot record page observation")
	}
	return state
}

func TestPageBindingLifecycleIntegration(t *testing.T) {
	s := pageDB(t)
	p := pageSession(t, s)
	root, err := s.RegisterPage(t.Context(), p, newPageTestID(), nil)
	if err != nil {
		t.Fatal("cannot register root")
	}
	if _, err = s.ResolvePage(t.Context(), p, root); err == nil {
		t.Fatal("unobserved page became usable")
	}
	first, err := s.ObservePage(t.Context(), p, root, newPageTestID(), "https://rp.example/callback?code=private-code-canary&state=private-state-canary#private-fragment-canary")
	if err != nil || first.URL != "https://rp.example/callback" {
		t.Fatal("failed sanitized root observation")
	}
	popup := readyPage(t, s, p, &first.PageRef, "https://idp.example/login")
	if popup.OpenerID != first.ID {
		t.Fatal("lost opener identity")
	}
	// A fresh Store must resolve the exact page, never borrow the opener URL.
	reopened := &Store{Pool: s.Pool}
	observed, err := reopened.ResolvePage(t.Context(), p, popup.PageRef)
	if err != nil || observed.URL != "https://idp.example/login" {
		t.Fatal("page metadata did not survive Store recreation")
	}
	swapped := popup.PageRef
	swapped.DocumentID = first.DocumentID
	if _, err = s.ResolvePage(t.Context(), p, swapped); err == nil {
		t.Fatal("another page's document was accepted")
	}
	navigated, err := s.ObservePage(t.Context(), p, first.PageRef, newPageTestID(), "https://rp.example/account")
	if err != nil || navigated.Version != first.Version+1 {
		t.Fatal("navigation did not advance observation")
	}
	if _, err = s.ResolvePage(t.Context(), p, first.PageRef); err == nil {
		t.Fatal("old document remains current")
	}
	if _, err = s.ResolvePage(t.Context(), p, popup.PageRef); err != nil {
		t.Fatal("independent page invalidated by another document")
	}
	if s.RetirePage(t.Context(), p, popup.PageRef, "closed") != nil || s.RetirePage(t.Context(), p, navigated.PageRef, "lost") != nil {
		t.Fatal("page lifecycle receipt failed")
	}
	for _, ref := range []PageRef{popup.PageRef, navigated.PageRef} {
		if _, err = s.ResolvePage(t.Context(), p, ref); err == nil {
			t.Fatal("terminal page accepted")
		}
		if _, err = s.RegisterPage(t.Context(), p, ref.ID, nil); err == nil {
			t.Fatal("retired identity reused")
		}
	}
	var leaked bool
	if s.Pool.QueryRow(t.Context(), `SELECT EXISTS(SELECT 1 FROM server_browser_pages WHERE observed_url LIKE '%private-%' OR observed_url LIKE '%?%' OR observed_url LIKE '%#%')`).Scan(&leaked) != nil || leaked {
		t.Fatal("callback private material reached PostgreSQL")
	}
}

func TestPageBindingContextFencesIntegration(t *testing.T) {
	s := pageDB(t)
	for _, kind := range []string{"foreign_owner", "foreign_run", "foreign_task", "foreign_context", "old_epoch", "owner_mode", "expired", "closed"} {
		t.Run(kind, func(t *testing.T) {
			p := pageSession(t, s)
			root := readyPage(t, s, p, nil, "https://rp.example/")
			bad := p
			switch kind {
			case "foreign_owner":
				bad.OwnerID = "other"
			case "foreign_run":
				bad.RunID = "run2"
			case "foreign_task":
				bad.TaskID = "other"
			case "foreign_context":
				bad.SessionID = newPageTestID()
			case "old_epoch":
				bad.Epoch++
			case "owner_mode":
				_, _ = s.Pool.Exec(t.Context(), `UPDATE server_browser_sessions SET mode='owner' WHERE browser_session_id=$1`, p.SessionID)
			case "expired":
				_, _ = s.Pool.Exec(t.Context(), `UPDATE server_browser_sessions SET expires_at=clock_timestamp()-interval '1 second' WHERE browser_session_id=$1`, p.SessionID)
			case "closed":
				_, _ = s.Pool.Exec(t.Context(), `UPDATE server_browser_sessions SET mode='closed' WHERE browser_session_id=$1`, p.SessionID)
			}
			if _, err := s.ResolvePage(t.Context(), bad, root.PageRef); err == nil {
				t.Fatal("foreign/stale context resolved a page")
			}
			if _, err := s.ObservePage(t.Context(), bad, root.PageRef, newPageTestID(), "https://other.example/"); err == nil {
				t.Fatal("foreign/stale context updated a page")
			}
			if _, err := s.RegisterPage(t.Context(), bad, newPageTestID(), &root.PageRef); err == nil {
				t.Fatal("foreign/stale context registered a page")
			}
			if s.RetirePage(t.Context(), bad, root.PageRef, "closed") == nil {
				t.Fatal("foreign/stale context retired a page")
			}
		})
	}
	t.Run("return_requires_each_page_fresh", func(t *testing.T) {
		p := pageSession(t, s)
		root := readyPage(t, s, p, nil, "https://rp.example/")
		popup := readyPage(t, s, p, &root.PageRef, "https://idp.example/login")
		if _, err := s.Pool.Exec(t.Context(), `UPDATE server_browser_sessions SET epoch=epoch+2,fresh_needed=true WHERE browser_session_id=$1`, p.SessionID); err != nil {
			t.Fatal("take/return fixture failed")
		}
		if _, err := s.ResolvePage(t.Context(), p, root.PageRef); err == nil {
			t.Fatal("old context epoch accepted")
		}
		p.Epoch += 2
		if _, err := s.ResolvePage(t.Context(), p, root.PageRef); err == nil {
			t.Fatal("fresh context epoch revived old observation")
		}
		fresh, err := s.ObservePage(t.Context(), p, root.PageRef, newPageTestID(), "https://rp.example/")
		if err != nil {
			t.Fatal("fresh root observation failed")
		}
		if _, err = s.ResolvePage(t.Context(), p, fresh.PageRef); err != nil {
			t.Fatal("fresh root denied")
		}
		if _, err = s.ResolvePage(t.Context(), p, popup.PageRef); err == nil {
			t.Fatal("refreshing opener revived stale popup")
		}
		if _, err = s.RegisterPage(t.Context(), p, newPageTestID(), &popup.PageRef); err == nil {
			t.Fatal("stale popup admitted child")
		}
		var needed bool
		if s.Pool.QueryRow(t.Context(), `SELECT fresh_needed FROM server_browser_sessions WHERE browser_session_id=$1`, p.SessionID).Scan(&needed) != nil || !needed {
			t.Fatal("page ledger cleared legacy freshness fence")
		}
	})
}

func TestPageBindingConcurrentCapsAndCASIntegration(t *testing.T) {
	s := pageDB(t)
	p := pageSession(t, s)
	root := readyPage(t, s, p, nil, "https://rp.example/")
	var wg sync.WaitGroup
	var registered, observed atomic.Int32
	for range 8 {
		wg.Go(func() {
			if _, err := s.RegisterPage(t.Context(), p, newPageTestID(), &root.PageRef); err == nil {
				registered.Add(1)
			}
		})
	}
	wg.Wait()
	if registered.Load() != MaxLivePages-1 {
		t.Fatal("concurrent page cap was bypassed")
	}
	for range 8 {
		wg.Go(func() {
			if _, err := s.ObservePage(t.Context(), p, root.PageRef, newPageTestID(), "https://rp.example/changed"); err == nil {
				observed.Add(1)
			}
		})
	}
	wg.Wait()
	if observed.Load() != 1 {
		t.Fatal("observation compare-and-swap admitted multiple writers")
	}
	// Read the winning receipt only for further fixture setup.
	if s.Pool.QueryRow(t.Context(), `SELECT document_id,observation_version FROM server_browser_pages WHERE browser_session_id=$1 AND page_id=$2`, p.SessionID, root.ID).Scan(&root.DocumentID, &root.Version) != nil {
		t.Fatal("winning receipt unavailable")
	}
	rows, err := s.Pool.Query(t.Context(), `SELECT page_id FROM server_browser_pages WHERE browser_session_id=$1 AND state='pending'`, p.SessionID)
	if err != nil {
		t.Fatal("pending pages unavailable")
	}
	var pending []string
	for rows.Next() {
		var id string
		if rows.Scan(&id) != nil {
			t.Fatal("pending row invalid")
		}
		pending = append(pending, id)
	}
	rows.Close()
	for _, id := range pending {
		if s.RetirePage(t.Context(), p, PageRef{ID: id}, "closed") != nil {
			t.Fatal("pending retirement failed")
		}
	}
	for total := MaxLivePages; total < MaxLifetimePages; total++ {
		ref, err := s.RegisterPage(t.Context(), p, newPageTestID(), &root.PageRef)
		if err != nil || s.RetirePage(t.Context(), p, ref, "closed") != nil {
			t.Fatal("bounded lifetime page setup failed")
		}
	}
	if _, err := s.RegisterPage(t.Context(), p, newPageTestID(), &root.PageRef); err == nil {
		t.Fatal("close/open loop bypassed lifetime limit")
	}
}

func TestPageBindingOwnerTransitionRaceIntegration(t *testing.T) {
	s := pageDB(t)
	p := pageSession(t, s)
	root := readyPage(t, s, p, nil, "https://rp.example/")
	owner, err := s.Pool.Begin(t.Context())
	if err != nil {
		t.Fatal("owner transaction unavailable")
	}
	defer owner.Rollback(context.Background())
	if _, err = owner.Exec(t.Context(), `UPDATE server_browser_sessions SET mode='owner',epoch=epoch+1,fresh_needed=true WHERE browser_session_id=$1`, p.SessionID); err != nil {
		t.Fatal("owner fence failed")
	}
	results := make(chan error, 3)
	go func() { _, e := s.ResolvePage(t.Context(), p, root.PageRef); results <- e }()
	go func() {
		_, e := s.ObservePage(t.Context(), p, root.PageRef, newPageTestID(), "https://rp.example/changed")
		results <- e
	}()
	go func() { _, e := s.RegisterPage(t.Context(), p, newPageTestID(), &root.PageRef); results <- e }()
	// Require actual PostgreSQL lock contention, not a scheduling sleep assumption.
	deadline := time.Now().Add(3 * time.Second)
	blocked := 0
	for time.Now().Before(deadline) {
		err = s.Pool.QueryRow(t.Context(), `SELECT count(*) FROM pg_stat_activity WHERE application_name=current_setting('application_name') AND pid<>pg_backend_pid() AND wait_event_type='Lock'`).Scan(&blocked)
		if err != nil || blocked == 3 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err != nil || blocked != 3 {
		_ = owner.Rollback(context.Background())
		for range 3 {
			<-results
		}
		t.Fatal("page operations did not contend on the common owner fence")
	}
	if owner.Commit(t.Context()) != nil {
		t.Fatal("owner transition commit failed")
	}
	for range 3 {
		if <-results == nil {
			t.Fatal("old page operation crossed owner takeover")
		}
	}
	var pages int
	var version int64
	if s.Pool.QueryRow(t.Context(), `SELECT count(*),max(observation_version) FROM server_browser_pages WHERE browser_session_id=$1`, p.SessionID).Scan(&pages, &version) != nil || pages != 1 || version != 1 {
		t.Fatal("denied operation modified page state")
	}
}

func TestPageBindingRetentionAndDowngradeIntegration(t *testing.T) {
	s := pageDB(t)
	if pageTestMigration(t, s.Pool, "107_browser_pages.down.sql") != nil || pageTestMigration(t, s.Pool, "107_browser_pages.up.sql") != nil {
		t.Fatal("empty migration down/up failed")
	}
	p := pageSession(t, s)
	root := readyPage(t, s, p, nil, "https://rp.example/")
	if pageTestMigration(t, s.Pool, "107_browser_pages.down.sql") == nil {
		t.Fatal("downgrade erased retained page bindings")
	}
	for _, sql := range []string{`DELETE FROM server_browser_pages`, `TRUNCATE server_browser_pages`, `UPDATE server_browser_pages SET page_id='11111111111111111111111111111111'`, `UPDATE server_browser_pages SET observation_version=observation_version`} {
		if _, err := s.Pool.Exec(t.Context(), sql); err == nil {
			t.Fatal("retained identity/version was mutable")
		}
	}
	if s.RetirePage(t.Context(), p, root.PageRef, "lost") != nil {
		t.Fatal("lost receipt failed")
	}
	if _, err := s.Pool.Exec(t.Context(), `UPDATE server_browser_pages SET state='ready',observation_version=observation_version+1`); err == nil {
		t.Fatal("lost page revived")
	}
}
