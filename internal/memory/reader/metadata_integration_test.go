package reader

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/ingestion"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/memory/readcontrol"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"
)

func TestCredentialFragmentMetadataIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	manifest := catalog.NewFileManifest(filepath.Join(t.TempDir(), "anchor"))
	if err := manifest.Initialize(); err != nil {
		t.Fatal(err)
	}
	s := catalog.New(pool, manifest)
	owner := owneridentity.WithActor(context.Background(), owneridentity.Actor{OwnerID: "owner", Channel: "web", DeliveryID: "metadata-fixture"})
	if _, err := pool.Exec(owner, `INSERT INTO sessions(session_key,user_id,channel) VALUES('metadata-session','owner','web');INSERT INTO runs(run_id,session_key,input_event_id,status,current_state,autonomy_mode,model_provider,started_at,updated_at) VALUES('metadata-run','metadata-session','input','model_running','model_running','mode_0','fixture',clock_timestamp(),clock_timestamp());INSERT INTO session_ownership(session_key,run_id,lease_id,fence_epoch,expires_at) VALUES('metadata-session','metadata-run','metadata-lease',1,clock_timestamp()+INTERVAL '1 hour')`); err != nil {
		t.Fatal(err)
	}
	live := ownership.WithToken(context.Background(), ownership.NewAuthority(pool), ownership.Token{SessionKey: "metadata-session", RunID: "metadata-run", LeaseID: "metadata-lease", FenceEpoch: 1})
	read := func(tool string, args readcontrol.Args) readcontrol.Result {
		t.Helper()
		tx, err := pool.Begin(live)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(live)
		out, err := (Reader{Sources: s}).ReadTx(live, tx, tool, args)
		if err != nil {
			t.Fatal(err)
		}
		if err = tx.Commit(live); err != nil {
			t.Fatal(err)
		}
		return out
	}
	const token = "Bearer SYNTHETIC_METADATA_TOKEN_123456789"
	assertSafe := func(out readcontrol.Result) {
		t.Helper()
		encoded, err := json.Marshal(out)
		if err != nil || strings.Contains(string(encoded), "SYNTHETIC_METADATA") {
			t.Fatal("credential escaped through read metadata")
		}
	}

	// Exercise the ordinary parse -> original asset -> fragment publication
	// pipeline. Message IDs are omitted from normalized original text but become
	// fragment handles, so body-only credential filtering cannot protect them.
	ingest := ingestion.Store{Catalog: s}
	upload := func(key, id string) (ingestion.Receipt, error) {
		raw, err := json.Marshal([]map[string]string{{"id": id, "author": "Автор", "time": "2026-10-15T18:00:00Z", "text": "ОбычныйМаркер безопасного сообщения"}})
		if err != nil {
			t.Fatal(err)
		}
		return ingest.Ingest(owner, ingestion.Upload{ExternalKey: key, ScopeType: "user", ScopeID: "owner", Format: "messages", Path: "messages.json", Raw: raw})
	}
	bad, err := upload("dirty-message", token)
	if !errors.Is(err, catalog.ErrInvalid) || bad.Status != "parsed" || bad.Source.ID == "" || strings.Contains(bad.Source.Body, token) {
		t.Fatal("message credential handle was not rejected at publication", err)
	}
	var fragments int
	if err = pool.QueryRow(owner, `SELECT count(*) FROM memory_object_sources WHERE memory_type='chunk' AND source_id=$1`, bad.Source.ID).Scan(&fragments); err != nil || fragments != 0 {
		t.Fatal("failed ingestion published a credential handle", err)
	}
	clean, err := upload("clean-message", "ordinary-id-1")
	if err != nil || clean.Status != "ready" {
		t.Fatal("ordinary ingestion failed", err)
	}
	cleanSearch := read(readcontrol.Search, readcontrol.Args{Query: "ОбычныйМаркер", Limit: 5})
	assertSafe(cleanSearch)
	var cleanHandle *readcontrol.Item
	for i := range cleanSearch.Items {
		if cleanSearch.Items[i].Source.ID == clean.Source.ID && cleanSearch.Items[i].FragmentKey == "message:ordinary-id-1:0" {
			cleanHandle = &cleanSearch.Items[i]
		}
	}
	if cleanHandle == nil {
		t.Fatal("ordinary exact fragment handle lost")
	}
	cleanOpen := read(readcontrol.Open, readcontrol.Args{SourceID: cleanHandle.Source.ID, Revision: cleanHandle.Source.Revision, AccessRevision: cleanHandle.Source.AccessRevision, Hash: cleanHandle.Source.Hash, FragmentKey: cleanHandle.FragmentKey, MaxChars: 100})
	if cleanOpen.Status != "ok" || len(cleanOpen.Items) != 1 || cleanOpen.Items[0].Text != "ОбычныйМаркер безопасного сообщения" {
		t.Fatal("ordinary ingestion search handle cannot open")
	}

	src, err := s.Put(owner, catalog.PutSource{ExternalKey: "metadata-source", Kind: "document", ScopeType: "user", ScopeID: "owner", Title: "Метаданные", Body: "Безопасный оригинал"})
	if err != nil {
		t.Fatal(err)
	}
	ref := catalog.SourceRef{ID: src.ID, Revision: src.Revision, AccessRevision: src.AccessRevision, Hash: src.Hash}
	for _, fragment := range []catalog.Fragment{
		{Key: token, Text: "Безопасный текст", Locator: json.RawMessage(`{}`)},
		{Key: "long-title", Title: strings.Repeat("x", 2049), Text: "Безопасный текст", Locator: json.RawMessage(`{}`)},
	} {
		if _, err = s.PublishFragments(owner, ref, []catalog.Fragment{fragment}); !errors.Is(err, catalog.ErrInvalid) {
			t.Fatal("unbounded or credential-bearing fragment admitted", err)
		}
	}
	if _, err = s.PublishFragments(owner, ref, []catalog.Fragment{
		{Key: "before", Title: token, Text: "TitleMetadataMarker", Locator: json.RawMessage(`{}`)},
		{Key: "target", Title: strings.Repeat("я", 1024), Text: "Целевой фрагмент", Locator: json.RawMessage(`{}`)},
		{Key: "after", Text: "СоседБезопасный", Locator: json.RawMessage(`{}`)},
		{Key: "legacy", Text: "LegacyMetadataMarker", Locator: json.RawMessage(`{}`)},
	}); err != nil {
		t.Fatal(err)
	}
	a := readcontrol.Args{SourceID: src.ID, Revision: src.Revision, AccessRevision: src.AccessRevision, Hash: src.Hash, FragmentKey: "before", MaxChars: 32}
	titleSearch := read(readcontrol.Search, readcontrol.Args{Query: "TitleMetadataMarker", Limit: 5})
	titleOpen := read(readcontrol.Open, a)
	a.FragmentKey = "target"
	titleNeighbors := read(readcontrol.Neighbors, a)
	for _, out := range []readcontrol.Result{titleSearch, titleOpen, titleNeighbors} {
		assertSafe(out)
		if out.Status != "ok" || len(out.Items) == 0 || !out.Items[0].Redacted {
			t.Fatal("title-only credential masking lost its redaction flag")
		}
	}

	// Simulate a pre-fix stored projection, without passing the new publication
	// gate. Read defense must protect existing databases as well as new uploads.
	beforeKey, legacyKey := "message:"+token+":0", "legacy:"+token
	if _, err = pool.Exec(owner, `UPDATE memory_object_sources SET fragment_key=CASE WHEN fragment_key='before' THEN $2 ELSE $3 END WHERE source_id=$1 AND memory_type='chunk' AND fragment_key IN ('before','legacy')`, src.ID, beforeKey, legacyKey); err != nil {
		t.Fatal(err)
	}
	legacySearch := read(readcontrol.Search, readcontrol.Args{Query: "LegacyMetadataMarker", Limit: 5})
	assertSafe(legacySearch)
	if legacySearch.Status != "empty" || len(legacySearch.Items) != 0 || !legacySearch.Truncated {
		t.Fatal("unsafe legacy key released or incomplete search hidden")
	}
	a.FragmentKey = legacyKey
	for _, tool := range []string{readcontrol.Open, readcontrol.Neighbors} {
		denied := read(tool, a)
		assertSafe(denied)
		if denied.Status != "denied" || len(denied.Items) != 0 {
			t.Fatal("unsafe incoming fragment handle accepted")
		}
	}
	a.FragmentKey = "target"
	legacyNeighbors := read(readcontrol.Neighbors, a)
	assertSafe(legacyNeighbors)
	if legacyNeighbors.Status != "ok" || len(legacyNeighbors.Items) != 1 || legacyNeighbors.Items[0].FragmentKey != "after" || !legacyNeighbors.Truncated {
		t.Fatal("unsafe legacy neighbor key released or masked into a false handle")
	}
}
