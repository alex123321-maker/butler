package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/butler/butler/internal/acceptance/effort"
	"github.com/butler/butler/internal/providerauth"
	"github.com/butler/butler/internal/transport/antigravity"
	"github.com/jackc/pgx/v5/pgxpool"
)

// A separate, prospectively frozen information-availability ablation. The main
// v2 corpus/outputs are unchanged. Each pair shares one actual native draft.
func evidenceCases() []effort.Case {
	clock := effort.Source{Ref: "clock_t0", Text: "Текущий контрольный момент T0. Сообщения с отметкой T0 полностью проверены и актуальны на T0."}
	return []effort.Case{
		{ID: "fresh_cancel", Group: "controlled_new_evidence", Goal: "Назови статус мероприятия Дерека на текущий контрольный момент: value=scheduled/cancelled. Если актуальность на текущий момент не подтверждена, decision=unavailable, value=not_current. Не выдавай устаревшую информацию за текущую.", Initial: []effort.Source{clock, {Ref: "mail_t0", Text: "Полная проверка на T0: мероприятие Дерека запланировано, отмены нет."}}, Available: []effort.Source{{Ref: "verified_update_t1", Text: "Полная повторная проверка на T1: Дерек отменил это мероприятие. Текущий статус cancelled."}}, Expected: effort.Expected{Decision: "answer", Value: "scheduled", Sources: []string{"mail_t0"}}},
		{ID: "fresh_reschedule", Group: "controlled_new_evidence", Goal: "Назови актуальную дату встречи Дерека на текущий контрольный момент, value=YYYY-MM-DD. Если актуальность не подтверждена, decision=unavailable, value=not_current. Не выдавай старую дату за текущую.", Initial: []effort.Source{clock, {Ref: "mail_t0", Text: "Полная проверка на T0: встреча Дерека назначена на 2026-10-26."}}, Available: []effort.Source{{Ref: "verified_update_t1", Text: "Полная повторная проверка на T1: Дерек перенёс ту же встречу на 2026-10-27; прежняя дата заменена."}}, Expected: effort.Expected{Decision: "answer", Value: "2026-10-26", Sources: []string{"mail_t0"}}},
		{ID: "fresh_count", Group: "controlled_new_evidence", Goal: "Назови текущее число подтверждённых мест в бронировании на контрольный момент, value — целое число. Если актуальность не подтверждена, decision=unavailable, value=not_current. Старое число не является текущим без проверки.", Initial: []effort.Source{clock, {Ref: "reservation_t0", Text: "Полная проверка на T0: подтверждено 3 места в бронировании."}}, Available: []effort.Source{{Ref: "verified_update_t1", Text: "Полная повторная проверка на T1: одно место отменено, сейчас подтверждены ровно 2 места."}}, Expected: effort.Expected{Decision: "answer", Value: "3", Sources: []string{"reservation_t0"}}},
	}
}

func evidenceTruth(id string) string {
	switch id {
	case "fresh_cancel":
		return "cancelled"
	case "fresh_reschedule":
		return "2026-10-27"
	case "fresh_count":
		return "2"
	}
	return ""
}

func evidenceManifest(source string, inv modelInventory) manifest {
	f := newManifest(source, inv)
	f.Protocol = "issue97-controlled-evidence-v1"
	f.Cases = evidenceCases()
	f.Slots = nil
	f.MaxTaskCalls = 1
	f.Grading = "shared draft uses T0 truth; review_same must acknowledge unverified T1 currentness; review_fresh must answer T1 truth with the new source; correct uncertainty is distinct from useful current completion"
	f.Limitations = append(f.Limitations, "controlled synthetic source delivery after a shared draft; measures information availability, not real-mail discovery success; draft cost retained once, no independent replication")
	profiles := []string{antigravity.FlashLowModel, antigravity.FlashHighModel}
	for i, c := range f.Cases {
		for j := range profiles {
			p := profiles[(i+j)%2]
			f.Slots = append(f.Slots, slot{Case: c.ID, Profile: p, Mode: "shared_draft"})
			modes := []effort.Mode{"review_same", "review_fresh"}
			for k := range modes {
				f.Slots = append(f.Slots, slot{Case: c.ID, Profile: p, Mode: modes[(i+j+k)%2]})
			}
		}
	}
	return f
}

func evidenceReview(c effort.Case, fresh bool) (effort.Case, func(effort.State, *effort.Reply) effort.State) {
	checkpoint := effort.Source{Ref: "checkpoint_t1", Text: "Текущий контрольный момент теперь T1. Проверка T0 не подтверждает актуальность на T1. За это время могли поступить изменения. Новые наблюдения перечислены отдельно, если были получены."}
	c.Initial = append(append([]effort.Source(nil), c.Initial...), checkpoint)
	var delivered string
	if fresh {
		c.Initial = append(c.Initial, c.Available[0])
		delivered = c.Available[0].Ref
		c.Expected = effort.Expected{Decision: "answer", Value: evidenceTruth(c.ID), Sources: []string{delivered, "checkpoint_t1"}}
	} else {
		c.Expected = effort.Expected{Decision: "unavailable", Value: "not_current", Sources: []string{"checkpoint_t1"}}
	}
	c.Available = nil
	return c, func(s effort.State, draft *effort.Reply) effort.State {
		if s.Calls == 0 {
			s.Candidate = draft
		}
		s.Phase = "check_goal_sources_dates_updates_and_contradictions"
		if delivered != "" {
			s.Actions = []string{delivered}
		}
		return s
	}
}

func runEvidenceTrial(ctx context.Context, pool *pgxpool.Pool, m *providerauth.Manager, o options) error {
	if o.trial == "" || len(o.trial) > 100 || strings.ContainsAny(o.trial, "/\\ \r\n") || o.source == "" || o.out == "" {
		return errors.New("new evidence trial/source/directory required")
	}
	if _, err := os.Stat(o.out); !os.IsNotExist(err) {
		return errors.New("new evidence directory required")
	}
	inv, err := inventory(ctx, m)
	if err != nil {
		return err
	}
	for _, p := range []string{antigravity.FlashLowModel, antigravity.FlashHighModel} {
		found := false
		for _, x := range inv.Models {
			if x == p {
				found = true
			}
		}
		if !found {
			return errors.New("qualified advertised profiles unavailable")
		}
	}
	f := evidenceManifest(o.source, inv)
	raw, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	if _, err = pool.Exec(ctx, `INSERT INTO effort_windows(id,manifest,manifest_sha256,expires_at,max_calls,max_reserved) VALUES($1,$2,$3,$4,$5,$6)`, o.trial, string(raw), digest(raw), f.ExpiresAt, len(f.Slots), len(f.Slots)*(f.InputByteCap+f.OutputTokenCap)); err != nil {
		return errors.New("new immutable evidence window denied")
	}
	if err = os.MkdirAll(o.out, 0700); err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(o.out, "manifest.json"), raw, 0600); err != nil {
		return err
	}
	ctx, cancel := context.WithDeadline(ctx, f.ExpiresAt)
	defer cancel()
	defer func() {
		retain, stop := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer stop()
		_, _ = pool.Exec(retain, `UPDATE effort_windows SET closed=true WHERE id=$1`, o.trial)
		_ = exportTrial(retain, pool, o)
	}()
	drafts := map[string]*effort.Reply{}
	for i, sl := range f.Slots {
		if ctx.Err() != nil {
			return errors.New("evidence trial stopped")
		}
		var c effort.Case
		for _, x := range f.Cases {
			if x.ID == sl.Case {
				c = x
			}
		}
		pair := sl.Case + "/" + sl.Profile
		draft := drafts[pair]
		decorate := func(s effort.State, _ *effort.Reply) effort.State { return s }
		if sl.Mode != "shared_draft" {
			c, decorate = evidenceReview(c, sl.Mode == "review_fresh")
		} else {
			c.Available = nil
		}
		started := time.Now()
		out := effort.Outcome{Status: "failed", Reason: "shared_draft_unavailable", State: effort.State{CaseID: c.ID}}
		if sl.Mode == "shared_draft" || draft != nil {
			out = effort.Run(ctx, c, effort.Config{Mode: effort.Baseline, MaxCalls: 1, Deadline: time.Now().Add(300 * time.Second)}, func(call context.Context, s effort.State) (effort.Reply, error) {
				return sample(call, pool, m, o.trial, sl, c, decorate(s, draft), f)
			}, func(call context.Context, s effort.State) error {
				b, e := json.Marshal(decorate(s, draft))
				if e != nil {
					return e
				}
				_, e = pool.Exec(call, `INSERT INTO effort_states(window_id,slot,state) VALUES($1,$2,$3::jsonb)`, o.trial, sl.ID(), string(b))
				return e
			})
			out.State = decorate(out.State, draft)
		}
		if sl.Mode == "shared_draft" && out.Status == "completed" {
			copy := *out.State.Candidate
			drafts[pair] = &copy
		}
		b, _ := json.Marshal(out)
		if _, err = pool.Exec(ctx, `INSERT INTO effort_outcomes(window_id,slot,outcome,milliseconds)VALUES($1,$2,$3::jsonb,$4)`, o.trial, sl.ID(), string(b), time.Since(started).Milliseconds()); err != nil {
			return errors.New("evidence outcome retention failed")
		}
		fmt.Printf("%d/%d %s %s correct=%t reason=%s\n", i+1, len(f.Slots), sl.ID(), out.Status, out.Correct, out.Reason)
	}
	return nil
}
