package api

import (
	"context"
	"encoding/json"
	"github.com/butler/butler/internal/memory/memorytest"
	"sync"
	"testing"
)

func TestDoctorStorageKeepsHistoryScopedAndOneReportPerRequest(t *testing.T) {
	pool := memorytest.Database(t)
	s := NewDoctorServer(pool, nil, nil, nil, nil)
	ctx := context.Background()
	var wg sync.WaitGroup
	ids := make(chan int64, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := s.storeReport(ctx, "unknown", json.RawMessage(`{"owner_id":"a","input_id":"request"}`))
			if err != nil {
				t.Error(err)
				return
			}
			ids <- r.ID
		}()
	}
	wg.Wait()
	close(ids)
	var first int64
	for id := range ids {
		if first == 0 {
			first = id
		}
		if first != id {
			t.Fatal("polling duplicated report")
		}
	}
	_, err := s.storeReport(ctx, "healthy", json.RawMessage(`{"status":"healthy","checks":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.storeReport(ctx, "unknown", json.RawMessage(`{"owner_id":"b","input_id":"other"}`))
	if err != nil {
		t.Fatal(err)
	}
	rows, err := s.listReports(ctx, "a", 50)
	if err != nil || len(rows) != 2 {
		t.Fatal(len(rows), err)
	}
}

func TestDoctorPendingReportSurvivesApprovalNavigation(t *testing.T) {
	pool := memorytest.Database(t)
	s := NewDoctorServer(pool, nil, nil, nil, nil)
	ctx := context.Background()
	pending, err := s.storeReport(ctx, "unknown", json.RawMessage(`{"owner_id":"a","input_id":"request","execution":{"status":"waiting_for_user"}}`))
	if err != nil {
		t.Fatal(err)
	}
	history, err := s.listReports(ctx, "a", 50)
	if err != nil || len(history) != 1 || history[0].ID != pending.ID {
		t.Fatal(history, err)
	}
	final, err := s.storeReport(ctx, "healthy", json.RawMessage(`{"owner_id":"a","input_id":"request","execution":{"status":"completed"}}`))
	if err != nil || final.ID != pending.ID || final.Status != "healthy" {
		t.Fatal(final, err)
	}
	repeated, err := s.storeReport(ctx, "unhealthy", json.RawMessage(`{"owner_id":"a","input_id":"request","execution":{"status":"completed"}}`))
	if err != nil || repeated.Status != "healthy" || repeated.ID != final.ID {
		t.Fatal("final snapshot changed", repeated, err)
	}
}
