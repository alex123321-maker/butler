package communication_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/butler/butler/apps/orchestrator/internal/communication"
	"github.com/butler/butler/internal/communicationcontrol"
)

func widgetFault(t *testing.T, op string) (*fixture, communicationcontrol.Receipt, communication.WidgetAction) {
	t.Helper()
	f := setup(t)
	pub, e := f.publish(t, "widget", "communication.widget", multiWidget)
	if e != nil {
		t.Fatal(e)
	}
	q := widgetAction(f, "web", "draft", 1, "a")
	if op == "submit" {
		if _, e = f.s.WidgetAction(f.actor("web", "seed"), pub.InteractionRef, q); e != nil {
			t.Fatal(e)
		}
		q = widgetAction(f, "web", "submit", 2)
	}
	return f, pub, q
}

func TestCommunicationWidgetEveryControlWriteFailureAndProcessCrashIntegration(t *testing.T) {
	for _, op := range []string{"draft", "submit"} {
		t.Run(op, func(t *testing.T) {
			probe, pub, q := widgetFault(t, op)
			tx, e := probe.pool.Begin(probe.ctx)
			if e != nil {
				t.Fatal(e)
			}
			log := &boundaryTx{Tx: tx}
			if _, e = probe.s.WidgetActionTx(probe.actor("web", "fault-answer"), log, pub.InteractionRef, q); e != nil {
				t.Fatal(e)
			}
			_ = tx.Rollback(context.Background())
			if log.writes < 3 {
				t.Fatal("control write boundary not exercised", log.labels)
			}
			for stop, label := range log.labels {
				for _, death := range []bool{false, true} {
					t.Run(fmt.Sprintf("%02d/%s/process_death=%t", stop+1, label, death), func(t *testing.T) {
						f, pub, q := widgetFault(t, op)
						if death {
							runCrash(t, f, "widget:"+op, pub.InteractionRef, stop+1, false)
						} else {
							tx, e := f.pool.Begin(f.ctx)
							if e != nil {
								t.Fatal(e)
							}
							_, e = f.s.WidgetActionTx(f.actor("web", "fault-answer"), &boundaryTx{Tx: tx, stop: stop + 1}, pub.InteractionRef, q)
							_ = tx.Rollback(context.Background())
							if !errors.Is(e, boundaryFailure) {
								t.Fatal("missed failed response", e)
							}
						}
						expected := int64(1)
						actions := 0
						if op == "submit" {
							expected = 2
							actions = 1
						}
						var version int64
						var state, checkpoint string
						if e = f.pool.QueryRow(f.ctx, `SELECT w.draft_version,x.state,i.checkpoint FROM communication_widget_views w JOIN communication_interactions x USING(interaction_id) JOIN accepted_inputs i USING(input_id) WHERE x.interaction_id=$1`, pub.InteractionRef).Scan(&version, &state, &checkpoint); e != nil || version != expected || state != "open" || checkpoint != "awaiting_interaction" || widgetCount(t, f, "communication_widget_actions") != actions {
							t.Fatal("partial control survived", version, state, checkpoint, e)
						}
						if _, e = f.s.WidgetAction(f.actor("web", "fault-answer"), pub.InteractionRef, q); e != nil {
							t.Fatal("safe retry", e)
						}
						if _, e = f.s.WidgetAction(f.actor("web", "fault-answer"), pub.InteractionRef, q); e != nil {
							t.Fatal("ACK replay", e)
						}
					})
				}
			}
			t.Run("commit_before_lost_ack", func(t *testing.T) {
				f, pub, q := widgetFault(t, op)
				runCrash(t, f, "widget:"+op, pub.InteractionRef, 0, true)
				if _, e = f.s.WidgetAction(f.actor("web", "fault-answer"), pub.InteractionRef, q); e != nil {
					t.Fatal("lost committed ACK replay", e)
				}
				expected := 1
				if op == "submit" {
					expected = 2
				}
				if widgetCount(t, f, "communication_widget_actions") != expected {
					t.Fatal("committed action repeated")
				}
			})
		})
	}
}
