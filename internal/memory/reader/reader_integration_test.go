package reader

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/memory/readcontrol"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"
)

func TestSearchOpenNeighborsCurrentSourceIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	m := catalog.NewFileManifest(filepath.Join(t.TempDir(), "anchor"))
	if e := m.Initialize(); e != nil {
		t.Fatal(e)
	}
	s := catalog.New(pool, m)
	own := owneridentity.WithActor(context.Background(), owneridentity.Actor{OwnerID: "owner", Channel: "web", DeliveryID: "read-fixture"})
	_, e := pool.Exec(own, `INSERT INTO sessions(session_key,user_id,channel) VALUES('read-session','owner','web');INSERT INTO runs(run_id,session_key,input_event_id,status,current_state,autonomy_mode,model_provider,started_at,updated_at) VALUES('read-run','read-session','input','model_running','model_running','mode_0','fixture',clock_timestamp(),clock_timestamp());INSERT INTO session_ownership(session_key,run_id,lease_id,fence_epoch,expires_at) VALUES('read-session','read-run','lease',1,clock_timestamp()+INTERVAL '1 hour')`)
	if e != nil {
		t.Fatal(e)
	}
	live := ownership.WithToken(context.Background(), ownership.NewAuthority(pool), ownership.Token{SessionKey: "read-session", RunID: "read-run", LeaseID: "lease", FenceEpoch: 1})
	src, e := s.Put(own, catalog.PutSource{ExternalKey: "large", Kind: "document", ScopeType: "user", ScopeID: "owner", Title: "Регламент", Body: "Правила калибровки"})
	if e != nil {
		t.Fatal(e)
	}
	ref := catalog.SourceRef{ID: src.ID, Revision: src.Revision, AccessRevision: src.AccessRevision, Hash: src.Hash}
	text := strings.Repeat("я", 6000) + " Калибровка: код МАЯК-742."
	_, e = s.PublishFragments(own, ref, []catalog.Fragment{{Key: "before", Text: "Перед настройкой отключите питание", Locator: json.RawMessage(`{"paragraph":1}`)}, {Key: "target", Title: "калибровка", Text: text, Locator: json.RawMessage(`{"paragraph":2}`)}, {Key: "after", Text: "После настройки проверьте журнал", Locator: json.RawMessage(`{"paragraph":3}`)}})
	if e != nil {
		t.Fatal(e)
	}
	read := func(tool string, a readcontrol.Args) readcontrol.Result {
		t.Helper()
		tx, e := pool.Begin(live)
		if e != nil {
			t.Fatal(e)
		}
		defer tx.Rollback(live)
		out, e := (Reader{Sources: s}).ReadTx(live, tx, tool, a)
		if e != nil {
			t.Fatal(e)
		}
		if e = tx.Commit(live); e != nil {
			t.Fatal(e)
		}
		return out
	}
	found := read(readcontrol.Search, readcontrol.Args{Query: "калибровка", Limit: 5})
	if found.Status != "ok" || len(found.Items) != 1 || found.Items[0].FragmentKey != "target" || strings.Contains(found.Items[0].Text, "МАЯК-742") {
		t.Fatalf("compact search: %+v", found)
	}
	a := readcontrol.Args{SourceID: src.ID, Revision: src.Revision, AccessRevision: src.AccessRevision, Hash: src.Hash, FragmentKey: "target", Offset: 6000, MaxChars: 100}
	opened := read(readcontrol.Open, a)
	if opened.Status != "ok" || !strings.Contains(opened.Items[0].Text, "МАЯК-742") || !utf8.ValidString(opened.Items[0].Text) || opened.Trust != "DATA" {
		t.Fatal("second bounded read did not provide exact evidence", opened)
	}
	a.Offset = 0
	a.MaxChars = 32
	neighbors := read(readcontrol.Neighbors, a)
	if neighbors.Status != "ok" || len(neighbors.Items) != 2 || utf8.RuneCountInString(neighbors.Items[0].Text)+utf8.RuneCountInString(neighbors.Items[1].Text) > 32 {
		t.Fatal(neighbors)
	}
	a.FragmentKey = "missing"
	if out := read(readcontrol.Open, a); out.Status != "not_found" {
		t.Fatal(out)
	}
	// Catalog IDs are opaque: messages and generated derivatives do not share
	// Put's deterministic source_ +64hex shape. Exact search handles must open.
	_, e = pool.Exec(own, `INSERT INTO messages(message_id,session_key,run_id,role,content) VALUES('reader-evidence-message','read-session','read-run','user','СообщениеОригинал: русский первоисточник')`)
	if e != nil {
		t.Fatal(e)
	}
	tx, e := pool.Begin(own)
	if e != nil {
		t.Fatal(e)
	}
	derived, e := s.CreateDerivedTx(own, tx, []catalog.SourceRef{ref}, "memory_extract", "ПроизводнаяЗапись", "Проверяемое производное DATA", json.RawMessage(`{}`))
	if e != nil {
		tx.Rollback(own)
		t.Fatal(e)
	}
	if e = tx.Commit(own); e != nil {
		t.Fatal(e)
	}
	for _, query := range []string{"СообщениеОригинал", "ПроизводнаяЗапись"} {
		out := read(readcontrol.Search, readcontrol.Args{Query: query, Limit: 5})
		if out.Status != "ok" || len(out.Items) != 1 {
			t.Fatal(query, out)
		}
		h := out.Items[0]
		raw, _ := json.Marshal(map[string]any{"source_id": h.Source.ID, "source_revision": h.Source.Revision, "access_revision": h.Source.AccessRevision, "content_hash": h.Source.Hash})
		args, e := readcontrol.DecodeArgs(readcontrol.Open, string(raw))
		if e != nil {
			t.Fatal("catalog-generated ID cannot open", h.Source.ID, e)
		}
		if opened := read(readcontrol.Open, args); opened.Status != "ok" || len(opened.Items) != 1 {
			t.Fatal("search handle failed open", opened)
		}
	}
	if !strings.HasPrefix(derived.ID, "source_") {
		t.Log("generated opaque ID", derived.ID)
	}
	// An accepted large source must not make tsvector exceed PostgreSQL's1MiB
	// limit and poison otherwise unrelated results. Original scanning is bounded.
	var huge strings.Builder
	for i := 0; i < 100000; i++ {
		fmt.Fprintf(&huge, "token%06dxxxxxxxxxxxxxxxxxxxxxxxx ", i)
	}
	if _, e = s.Put(own, catalog.PutSource{ExternalKey: "huge", Kind: "document", ScopeType: "user", ScopeID: "owner", Title: "Большой источник", Body: huge.String()}); e != nil {
		t.Fatal(e)
	}
	if out := read(readcontrol.Search, readcontrol.Args{Query: "калибровка", Limit: 5}); out.Status != "ok" || !out.Truncated {
		t.Fatal("large original poisoned search or hid incomplete scan", out)
	}
	project, e := s.Put(own, catalog.PutSource{ExternalKey: "other-project", Kind: "document", ScopeType: "project", ScopeID: "not-granted", Body: "ПроектЗакрыт: чужая область"})
	if e != nil {
		t.Fatal(e)
	}
	if out := read(readcontrol.Search, readcontrol.Args{Query: "ПроектЗакрыт", Limit: 5}); out.Status != "empty" {
		t.Fatal("cross-scope source in search", out)
	}
	if out := read(readcontrol.Open, readcontrol.Args{SourceID: project.ID, Revision: project.Revision, AccessRevision: project.AccessRevision, Hash: project.Hash, MaxChars: 32}); out.Status != "denied" {
		t.Fatal("cross-scope source open", out)
	}
	foreign := owneridentity.WithActor(context.Background(), owneridentity.Actor{OwnerID: "other", Channel: "web", DeliveryID: "x"})
	other, e := s.Put(foreign, catalog.PutSource{ExternalKey: "secret", Kind: "document", ScopeType: "user", ScopeID: "other", Body: "калибровка FOREIGN_PRIVATE"})
	if e != nil {
		t.Fatal(e)
	}
	a = readcontrol.Args{SourceID: other.ID, Revision: other.Revision, AccessRevision: other.AccessRevision, Hash: other.Hash, MaxChars: 20}
	if out := read(readcontrol.Open, a); out.Status != "denied" || len(out.Items) != 0 {
		t.Fatal("cross-owner read", out)
	}
	_, e = s.Put(own, catalog.PutSource{ExternalKey: "large", Kind: "document", ScopeType: "user", ScopeID: "owner", ExpectedRevision: src.Revision, Body: "Исправлено"})
	if e != nil {
		t.Fatal(e)
	}
	a = readcontrol.Args{SourceID: src.ID, Revision: src.Revision, AccessRevision: src.AccessRevision, Hash: src.Hash, MaxChars: 20}
	if out := read(readcontrol.Open, a); out.Status != "stale" || len(out.Items) != 0 {
		t.Fatal("search handle survived correction", out)
	}
}
