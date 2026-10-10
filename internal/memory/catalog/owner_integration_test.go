package catalog

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestCorrectionExportAndOriginalParentPinsIntegration(t *testing.T) {
	s, owner := fixture(t)
	live := seedRun(t, s)
	parent := put(t, s, owner, "original", "исходный текст")
	child, e := s.Put(owner, PutSource{ExternalKey: "child", Kind: "model_candidate", ScopeType: "user", ScopeID: "owner", Body: "false child report", Parents: []SourceRef{ref(parent)}})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Get(live, child.ID); e != nil {
		t.Fatal(e)
	}
	var count int
	if e = s.pool.QueryRow(owner, `SELECT count(*) FROM memory_run_sources WHERE run_id='run'`).Scan(&count); e != nil || count != 2 {
		t.Fatal("original source was not pinned", count, e)
	}
	corrected, e := s.Correct(owner, Correction{SourceID: parent.ID, ExpectedRevision: 1, Title: parent.Title, Body: "исправленный текст", Author: "owner"})
	if e != nil || corrected.Revision != 2 {
		t.Fatal(corrected, e)
	}
	if _, e = s.Get(owner, child.ID); !errors.Is(e, ErrStale) {
		t.Fatal("false derived report survived source correction", e)
	}
	tx, e := s.pool.Begin(live)
	if e != nil {
		t.Fatal(e)
	}
	e = s.CheckRunSourcesTx(live, tx, "run")
	_ = tx.Rollback(context.Background())
	if e == nil {
		t.Fatal("late child proposal used corrected original")
	}
	var data bytes.Buffer
	if e = s.Export(owner, json.NewEncoder(&data).Encode); e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(data.String(), "исходный текст") || !strings.Contains(data.String(), "исправленный текст") {
		t.Fatal("revision history missing")
	}
	if _, e = s.Delete(owner, parent.ID, "delete", 2); e != nil {
		t.Fatal(e)
	}
	data.Reset()
	if e = s.Export(owner, json.NewEncoder(&data).Encode); e != nil {
		t.Fatal(e)
	}
	if strings.Contains(data.String(), "исходный текст") || strings.Contains(data.String(), "исправленный текст") || strings.Contains(data.String(), "false child report") {
		t.Fatal("deleted original or derivative export survived")
	}
}
