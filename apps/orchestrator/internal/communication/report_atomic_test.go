package communication_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/butler/butler/apps/orchestrator/internal/artifacts"
	"github.com/butler/butler/internal/communicationcontrol"
)

func TestCommunicationReportArtifactAndAllPartsAtomicBoundariesIntegration(t *testing.T) {
	a := communicationcontrol.Args{SchemaVersion: 1, Purpose: "result", Title: "Canonical report", Summary: "Atomic parts"}
	for n := 0; n < 55; n++ {
		a.Blocks = append(a.Blocks, communicationcontrol.Block{Type: "paragraph", Text: fmt.Sprint("Block ", n)})
	}
	raw, _ := json.Marshal(a)
	apply := func(f *fixture, stop int) ([]string, error) {
		tx, err := f.pool.Begin(f.ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(context.Background())
		b := &boundaryTx{Tx: tx, stop: stop}
		_, err = f.s.ApplyTx(f.ctx, b, f.p, communicationcontrol.Envelope{RunID: f.p.RunID, ToolCallID: "atomic-report", EffectID: "atomic-report:" + f.p.RunID, Tool: "communication.report", ArgsJSON: string(raw)})
		return b.labels, err
	}
	probe, _ := mediaFixture(t)
	labels, err := apply(probe, 0)
	if err != nil || len(labels) < 7 {
		t.Fatal("report matrix did not observe all writes", labels, err)
	}
	for n, label := range labels {
		t.Run(fmt.Sprint(n+1, "_", label), func(t *testing.T) {
			f, _ := mediaFixture(t)
			seen, err := apply(f, n+1)
			if err == nil || len(seen) != n+1 {
				t.Fatal("write failure missed", seen, err)
			}
			assertPublicationAbsent(t, f)
			var count int
			if err = f.pool.QueryRow(f.ctx, `SELECT count(*) FROM artifacts WHERE run_id=$1 AND artifact_type='communication_report'`, f.p.RunID).Scan(&count); err != nil || count != 0 {
				t.Fatal("canonical artifact escaped rollback", count, err)
			}
			r, err := f.publish(t, "atomic-report", "communication.report", string(raw))
			if err != nil {
				t.Fatal("clean recovery failed", err)
			}
			if _, err = f.publish(t, "atomic-report", "communication.report", string(raw)); err != nil {
				t.Fatal("lost commit ACK recovery", err)
			}
			var saved []byte
			if err = f.pool.QueryRow(f.ctx, `SELECT content_json FROM artifacts WHERE artifact_id=$1`, artifacts.ReportID(r.PublicationRef)).Scan(&saved); err != nil {
				t.Fatal(err)
			}
			var got communicationcontrol.Args
			if json.Unmarshal(saved, &got) != nil || len(got.Blocks) != 55 || f.count(t, "delivery_outbox") != 2 || f.count(t, "communication_publications") != 1 {
				t.Fatal("report replay lost or duplicated parts")
			}
		})
	}
}
