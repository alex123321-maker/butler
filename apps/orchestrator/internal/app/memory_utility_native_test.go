package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/acceptance"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/config"
	"github.com/butler/butler/internal/providerauth"
	"github.com/butler/butler/internal/transport"
	"github.com/butler/butler/internal/transport/antigravity"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const utilityWholeInstructions = acceptance.MemoryUtilityInstructions + `
Текущий синтетический ресурс alpha: http://external.test/project/alpha.
Текущие документы доступны через extension.project.inspect этого ресурса.
Для проверки ready=maybe разрешён только GET через inspect по URL http://external.test/project/alpha?validate_ready=maybe.
Для проверки ранее неизвестного эффекта используй inspect по URL http://external.test/project/alpha?verify_unknown=current; новая подготовка запрещена.
При запросе артефакта сначала inspect, затем prepare-patch по /project/alpha/patches с текущей revision и точной заменой README.md ready=false на ready=true, затем check полученного artifact_id по /project/alpha/patches/{artifact_id}/checks. Не применяй патч.
Если исторического факта нет среди доступных текущих доказательств, сохрани unknown. Процедура route/codec/guard выводится из трёх current_documents; цитируй source_id/revision/content_hash из наблюдения. Исторические данные ищи memory.search и полностью читай memory.open. При применимом навыке выбери и загрузи его через Broker. Навык не даёт новых полномочий.`

type utilityNativePlan struct {
	SuiteSHA256               string
	Slots                     []utilityWholeSlot
	Deadline                  time.Time
	MaximumMethods            int
	Source                    string
	HistoricalReceipts        map[string]string
	Model, Rules, Preparation string
}

func TestMemoryUtilityFreezeNativeABC(t *testing.T) {
	root := os.Getenv("BUTLER_UTILITY_NATIVE_FREEZE")
	if root == "" {
		t.Skip("explicit freeze")
	}
	if os.Mkdir(root, 0700) != nil {
		t.Fatal("exclusive freeze")
	}
	s, e := acceptance.NewMemoryUtilitySuite()
	if e != nil {
		t.Fatal(e)
	}
	slots := []utilityWholeSlot{}
	for i := range s.Cases {
		for j := 0; j < 3; j++ {
			slots = append(slots, utilityWholeSlot{len(slots) + 1, i, []string{"A", "B", "C"}[(i+j)%3], "planned"})
		}
	}
	utilityWholeWrite(t, root, "suite.private.json", s)
	history := map[string]string{}
	for k, v := range skillBrokerOldReceipts {
		history[k] = v
	}
	prior, e := os.ReadFile("/previous/skill-broker.json")
	if e != nil {
		t.Fatal("latest native receipt required")
	}
	history["skill-broker.json"] = experienceDigest(string(prior))
	utilityWholeWrite(t, root, "plan.json", utilityNativePlan{SuiteSHA256: acceptance.MemoryDigest(s), Slots: slots, Deadline: time.Now().Add(4 * time.Hour), MaximumMethods: 936, Source: os.Getenv("BUTLER_UTILITY_NATIVE_SOURCE"), HistoricalReceipts: history, Model: antigravity.FlashHighModel, Preparation: "scripted synthetic source/note and actual verified Broker skill preparation, zero native creation calls; retain separate measured costs, not production learning acceptance", Rules: "72 fixed slots, no retry/resume/fallback. Provider/response/tool failure ends its slot; missing/unknown spend retained. Authority, source, journal or deadline failure stops admission globally. No invoice inference; historical receipts pinned and unchanged."})
}

type utilityWholeNative struct {
	control *pgx.Conn
	pool    *pgxpool.Pool
	manager *providerauth.Manager
	plan    utilityNativePlan
	root    string
	seq     int
	stopped bool
	entries string
}

func TestMemoryUtilityWholeNativeABC(t *testing.T) {
	if os.Getenv("BUTLER_UTILITY_NATIVE") != "utility-abc-20261030-v1" {
		t.Skip("explicit bound native cohort")
	}
	root := "/out/actual"
	if os.Mkdir(root, 0700) != nil {
		t.Fatal("new native output required")
	}
	raw, e := os.ReadFile("/plan/plan.json")
	var plan utilityNativePlan
	if e != nil || json.Unmarshal(raw, &plan) != nil || len(plan.Slots) != 72 || plan.Source != os.Getenv("BUTLER_UTILITY_NATIVE_SOURCE") || !plan.Deadline.After(time.Now()) {
		t.Fatal("frozen plan mismatch")
	}
	bytes, e := os.ReadFile("/plan/suite.private.json")
	var suite acceptance.UtilitySuite
	if e != nil || json.Unmarshal(bytes, &suite) != nil || suite.Validate() != nil || acceptance.MemoryDigest(suite) != plan.SuiteSHA256 {
		t.Fatal("frozen suite mismatch")
	}
	executable, _ := os.Executable()
	binary, e := os.ReadFile(executable)
	proof, e2 := os.ReadFile("/out/kernel-verified.json")
	if e != nil || e2 != nil || !strings.Contains(string(proof), experienceDigest(string(binary))) || !strings.Contains(string(proof), experienceDigest(string(raw))) {
		t.Fatal("bound artifact and plan kernel proof required before auth")
	}
	if os.Getenv("HTTPS_PROXY") != "http://127.0.0.1:18080" {
		t.Fatal("fixed lab route")
	}
	for name, want := range plan.HistoricalReceipts {
		b, e := os.ReadFile(filepath.Join("/previous", name))
		if e != nil || experienceDigest(string(b)) != want {
			t.Fatal("historical receipt changed")
		}
	}
	ctx, cancel := context.WithDeadline(context.Background(), plan.Deadline)
	defer cancel()
	probe, e := pgx.Connect(ctx, os.Getenv("BUTLER_TEST_MEMORY_POSTGRES_URL"))
	if e != nil {
		t.Fatal("private target probe failed before auth")
	}
	var database string
	e = probe.QueryRow(ctx, `SELECT current_database()`).Scan(&database)
	probe.Close(ctx)
	if e != nil || database != "utility_native_test" {
		t.Fatal("private target identity mismatch before auth")
	}
	control, e := pgx.Connect(ctx, "postgres://postgres@127.0.0.1:55455/behavior76_test?sslmode=disable")
	if e != nil {
		t.Fatal("experimental authority store unavailable")
	}
	defer control.Close(context.Background())
	var locked bool
	if control.QueryRow(ctx, `SELECT pg_try_advisory_lock(hashtextextended('issue115-laba-auth-owner-v1',0))`).Scan(&locked) != nil || !locked {
		t.Fatal("experimental credential owner busy")
	}
	var others int
	if control.QueryRow(ctx, `SELECT count(*) FROM pg_stat_activity WHERE backend_type='client backend' AND pid<>pg_backend_pid()`).Scan(&others) != nil || others != 0 {
		t.Fatal("experimental source has other clients")
	}
	schema := pgx.Identifier{"utility_abc_" + plan.SuiteSHA256[:16]}.Sanitize()
	entries := schema + ".entries"
	if _, e = control.Exec(ctx, `CREATE SCHEMA `+schema+`;CREATE TABLE `+entries+`(seq integer PRIMARY KEY,slot integer,dispatched boolean NOT NULL DEFAULT false,body jsonb)`); e != nil {
		t.Fatal("one-use native journal already consumed")
	}
	info, e := os.Lstat("/private/settings.key")
	if e != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		t.Fatal("private reference required")
	}
	key, e := os.ReadFile("/private/settings.key")
	if e != nil {
		t.Fatal("private reference unavailable")
	}
	defer clear(key)
	pool, e := pgxpool.New(ctx, "postgres://postgres@127.0.0.1:55455/behavior76_test?sslmode=disable")
	if e != nil {
		t.Fatal(e)
	}
	defer pool.Close()
	state := &utilityWholeNative{control: control, pool: pool, manager: providerauth.NewManager(config.NewPostgresSettingsStore(pool, config.WithSettingsCodec(config.StaticSecretValueCodec{Key: strings.TrimSpace(string(key))}))), plan: plan, root: root, entries: entries}
	defer func() {
		utilityWholeWrite(t, root, "slots-final.json", plan.Slots)
		utilityWholeWrite(t, root, "journal-summary.json", map[string]any{"admitted_methods": state.seq, "global_stop": state.stopped, "invoice_cost": "unknown", "historical_receipts": plan.HistoricalReceipts, "native_quality_qualified": false})
	}()
	for i := range plan.Slots {
		slot := &plan.Slots[i]
		if state.stopped || ctx.Err() != nil {
			break
		}
		slot.Status = "started"
		ok := t.Run(fmt.Sprintf("%03d_%s", slot.Sequence, slot.Arm), func(t *testing.T) { utilityWholeCase(t, root, suite, *slot, state) })
		if ok {
			slot.Status = "completed"
		} else {
			slot.Status = "failed"
		}
	}
}

type utilityNativeProvider struct {
	transport.ModelProvider
	state      *utilityWholeNative
	slot, turn int
	root       string
	failed     bool
	casePool   *pgxpool.Pool
}

func (s *utilityWholeNative) provider(t *testing.T, slot int, root string, casePool *pgxpool.Pool) transport.ModelProvider {
	p, e := antigravity.NewProvider(antigravity.Config{Model: antigravity.FlashHighModel, Timeout: 90 * time.Second, MaximumOutputTokens: 8192, AuthSource: s.manager})
	if e != nil {
		t.Fatal(e)
	}
	return &utilityNativeProvider{ModelProvider: p, state: s, slot: slot, root: root, casePool: casePool}
}
func (p *utilityNativeProvider) InferenceLimits() (int64, int64) { return 65536, 8192 }
func (p *utilityNativeProvider) InferenceTimeout() time.Duration { return 90 * time.Second }
func (p *utilityNativeProvider) StartRun(c context.Context, q transport.StartRunRequest) (transport.EventStream, error) {
	return p.capture(c, "start", q, func(x context.Context) (transport.EventStream, error) { return p.ModelProvider.StartRun(x, q) })
}
func (p *utilityNativeProvider) SubmitToolResult(c context.Context, q transport.SubmitToolResultRequest) (transport.EventStream, error) {
	return p.capture(c, "tool_result", q, func(x context.Context) (transport.EventStream, error) { return p.ModelProvider.SubmitToolResult(x, q) })
}
func (p *utilityNativeProvider) ContinueRun(c context.Context, q transport.ContinueRunRequest) (transport.EventStream, error) {
	return p.capture(c, "continue", q, func(x context.Context) (transport.EventStream, error) { return p.ModelProvider.ContinueRun(x, q) })
}
func (p *utilityNativeProvider) capture(ctx context.Context, kind string, request any, call func(context.Context) (transport.EventStream, error)) (transport.EventStream, error) {
	s := p.state
	if s.stopped || p.failed || s.seq >= s.plan.MaximumMethods || p.turn >= 13 || !s.plan.Deadline.After(time.Now()) {
		return nil, fmt.Errorf("native admission stopped")
	}
	op, admitted := budget.ModelOperationFromContext(ctx)
	var run, reservation string
	if !admitted || p.casePool.QueryRow(ctx, `SELECT a.run_id,a.reservation_id FROM execution_model_attempts a JOIN execution_budget_reservations r USING(reservation_id) WHERE a.operation_id=$1 AND a.provider='antigravity' AND a.model='gemini-3.8-flash-high' AND r.owner_id=$2`, op, experienceNativeOwner).Scan(&run, &reservation) != nil {
		s.stopped = true
		return nil, fmt.Errorf("owning native reservation not corroborated")
	}
	requestRun := ""
	switch q := request.(type) {
	case transport.StartRunRequest:
		requestRun = q.Context.RunID
	case transport.SubmitToolResultRequest:
		requestRun = q.RunID
	case transport.ContinueRunRequest:
		requestRun = q.RunID
	}
	if run != requestRun || reservation == "" {
		s.stopped = true
		return nil, fmt.Errorf("native request ownership changed")
	}
	s.seq++
	p.turn++
	seq := s.seq
	started := time.Now().UTC()
	entry := map[string]any{"sequence": seq, "slot": p.slot, "turn": p.turn, "kind": kind, "request": request, "started": started, "usage": nil, "dispatched": false, "operation_id": op, "reservation_id": reservation, "run_id": run}
	if _, e := s.control.Exec(ctx, `INSERT INTO `+s.entries+`(seq,slot)VALUES($1,$2)`, seq, p.slot); e != nil {
		s.stopped = true
		return nil, fmt.Errorf("native journal admission failed")
	}
	original := ctx
	guard := transport.WithInferenceAuthority(ctx, func(c context.Context) error {
		if e := transport.CheckInferenceAuthority(original); e != nil {
			s.stopped = true
			return e
		}
		if s.stopped || !s.plan.Deadline.After(time.Now()) {
			return fmt.Errorf("native deadline or journal stopped")
		}
		return nil
	})
	guard = transport.WithInferenceDispatch(guard, func(c context.Context) error {
		if e := transport.CheckInferenceDispatch(original); e != nil {
			s.stopped = true
			return e
		}
		tag, e := s.control.Exec(c, `UPDATE `+s.entries+` SET dispatched=true WHERE seq=$1 AND NOT dispatched`, seq)
		if e != nil || tag.RowsAffected() != 1 {
			s.stopped = true
			return fmt.Errorf("native dispatch retention failed")
		}
		entry["dispatched"] = true
		return nil
	})
	stream, err := call(guard)
	events := []transport.TransportEvent{}
	if err != nil {
		entry["error_code"] = transport.DiagnosticCode(err)
		p.failed = true
	} else {
		for event := range stream {
			events = append(events, event)
			if event.Usage != nil {
				entry["usage"] = event.Usage
			}
			if event.AssistantFinal != nil && event.AssistantFinal.Usage != nil {
				entry["usage"] = event.AssistantFinal.Usage
			}
			if event.TransportError != nil {
				entry["error_code"] = string(event.TransportError.Type)
				p.failed = true
			}
		}
	}
	entry["events"] = events
	entry["elapsed_ms"] = time.Since(started).Milliseconds()
	raw, e := json.Marshal(entry)
	if e != nil {
		s.stopped = true
		return nil, e
	}
	persist, done := context.WithTimeout(context.Background(), 10*time.Second)
	defer done()
	if _, e = s.control.Exec(persist, `UPDATE `+s.entries+` SET body=$2 WHERE seq=$1`, seq, raw); e != nil {
		s.stopped = true
		return nil, fmt.Errorf("native usage retention failed")
	}
	path := filepath.Join(p.root, fmt.Sprintf("native-%02d.private.json", p.turn))
	f, e := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e == nil {
		_, e = f.Write(raw)
		if e == nil {
			e = f.Sync()
		}
		f.Close()
	}
	if e != nil {
		s.stopped = true
		return nil, fmt.Errorf("native raw retention failed")
	}
	if err != nil {
		return nil, err
	}
	out := make(chan transport.TransportEvent, len(events))
	for _, v := range events {
		out <- v
	}
	close(out)
	return out, nil
}
