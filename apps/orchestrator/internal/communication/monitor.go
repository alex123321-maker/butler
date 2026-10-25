package communication

import (
	"context"
	"encoding/json"
	"errors"
	"time"
	"unicode/utf8"

	"github.com/butler/butler/internal/communicationcontrol"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/research"
	"github.com/jackc/pgx/v5"
)

func runPagesTx(ctx context.Context, tx pgx.Tx, run string) ([]research.Page, bool, error) {
	rows, err := tx.Query(ctx, `SELECT tool_name,status,args::text,COALESCE(result::text,'{}'),finished_at FROM tool_calls WHERE run_id=$1 AND tool_name IN ('web.search','web.fetch','web.fetch_batch','web.extract','http.request','browser.read','browser.open','browser.navigate','browser.snapshot','browser.extract_text') ORDER BY started_at LIMIT 65`, run)
	if err != nil {
		return nil, false, err
	}
	var calls []transcript.ToolCall
	for rows.Next() {
		c := transcript.ToolCall{RunID: run}
		if err = rows.Scan(&c.ToolName, &c.Status, &c.ArgsJSON, &c.ResultJSON, &c.FinishedAt); err != nil {
			rows.Close()
			return nil, false, err
		}
		calls = append(calls, c)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, false, err
	}
	if len(calls) > 64 {
		return nil, false, ErrDenied
	}
	pages, _ := research.Pages(calls, run)
	attempted := false
	for _, c := range calls {
		attempted = attempted || research.IsReadAttempt(c)
	}
	return pages, attempted, nil
}

// monitorTx compares source observations under the owning session/job locks.
// Its receipt commits together with the canonical publication/outbox, or with
// the explicit unchanged decision. Restart never needs to replay a send.
func (s *Store) monitorTx(ctx context.Context, tx pgx.Tx, p Principal, e communicationcontrol.Envelope, a communicationcontrol.Args, digest string, systemFailure bool) (*communicationcontrol.Receipt, error) {
	var occurrence, schedule string
	var revision int64
	var enabled bool
	err := tx.QueryRow(ctx, `SELECT o.occurrence_id,j.schedule_id,j.revision,COALESCE((j.spec_json->>'monitor')::boolean,FALSE) FROM scheduled_occurrences o JOIN scheduled_jobs j USING(schedule_id) JOIN accepted_inputs i ON i.input_id=o.input_id WHERE o.input_id=$1 AND o.task_id=$2 AND o.schedule_revision=j.revision AND j.owner_id=$3 AND j.session_key=$4 AND i.run_id=$5 AND j.state NOT IN ('paused','blocked','cancelled') AND j.expires_at>clock_timestamp() FOR SHARE OF j`, p.InputID, p.TaskID, p.OwnerID, p.SessionKey, p.RunID).Scan(&occurrence, &schedule, &revision, &enabled)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	if err != nil || !enabled {
		if a.Observation != nil {
			return nil, ErrDenied
		}
		return nil, nil
	}
	if systemFailure {
		return nil, nil
	}
	if a.Observation == nil || e.Tool != "communication.message" || (a.Purpose != "result" && a.Purpose != "blocked") {
		return nil, ErrDenied
	}
	var receiptRaw []byte
	err = tx.QueryRow(ctx, `SELECT receipt_json FROM schedule_observations WHERE effect_id=$1 AND input_id=$2 AND run_id=$3 AND tool_call_id=$4 AND payload_hash=$5`, e.EffectID, p.InputID, p.RunID, e.ToolCallID, digest).Scan(&receiptRaw)
	if err == nil {
		var out communicationcontrol.Receipt
		if json.Unmarshal(receiptRaw, &out) != nil {
			return nil, ErrDenied
		}
		return &out, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	o := a.Observation
	if utf8.RuneCountInString(a.Text) > 1000 {
		return nil, ErrDenied
	}
	pages, attempted, err := runPagesTx(ctx, tx, p.RunID)
	if err != nil {
		return nil, err
	}
	if !attempted {
		return nil, ErrDenied
	}
	if o.Status != "unknown" {
		if research.Check(a.Text, a.Evidence, pages, time.Now().UTC(), false) != "" {
			return nil, ErrDenied
		}
	}
	stateHash := hash(schedule + ":" + o.EventKey + ":" + o.Status + ":" + string(mustJSONMonitor(o.ExpectedAt)))
	if o.Status == "unknown" {
		stateHash = hash(schedule + ":unknown")
	}
	notify := o.Status != "pending"
	var seen bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM schedule_observations WHERE schedule_id=$1 AND schedule_revision=$2 AND state_hash=$3 AND notified)`, schedule, revision, stateHash).Scan(&seen); err != nil {
		return nil, err
	}
	notify = notify && !seen
	out := communicationcontrol.Receipt{Status: "unchanged", PublicationRef: "", Delivery: []communicationcontrol.Delivery{}}
	if notify {
		out.Status = "accepted"
		out.PublicationRef = "publication:" + hash(e.EffectID)
	}
	raw, _ := json.Marshal(out)
	if _, err = tx.Exec(ctx, `INSERT INTO schedule_observations(occurrence_id,schedule_id,schedule_revision,input_id,owner_id,run_id,effect_id,tool_call_id,payload_hash,event_key_hash,state,state_hash,notified,receipt_json) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`, occurrence, schedule, revision, p.InputID, p.OwnerID, p.RunID, e.EffectID, e.ToolCallID, digest, hash(o.EventKey), o.Status, stateHash, notify, raw); err != nil {
		return nil, err
	}
	if !notify {
		return &out, nil
	}
	return nil, nil // Continue through ordinary source/export checks and outbox.
}
func mustJSONMonitor(v any) []byte { b, _ := json.Marshal(v); return b }
