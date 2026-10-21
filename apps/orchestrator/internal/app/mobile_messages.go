package app

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/readcontrol"
	"github.com/butler/butler/internal/memory/sanitize"
	"github.com/butler/butler/internal/mobile"
	"github.com/jackc/pgx/v5"
)

type messageRow struct {
	id, state string
	part      int
	source    catalog.SourceRef
}

func messageItem(src catalog.Source, id string, text string, loc any) readcontrol.Item {
	raw, _ := json.Marshal(loc)
	r := []rune(sanitize.MaskCredentials(text))
	clipped := len(r) > 2048
	if clipped {
		r = r[:2048]
	}
	out := readcontrol.Item{Source: readcontrol.SourceRef{ID: src.ID, Revision: src.Revision, AccessRevision: src.AccessRevision, Hash: src.Hash}, Kind: src.Kind, ScopeType: src.ScopeType, Title: src.Title, Text: string(r), Locator: raw, Freshness: "source_backed_observation", Truncated: clipped, NextOffset: len(r)}
	if src.EventTime != nil {
		out.EventTime = src.EventTime.UTC().Format(time.RFC3339Nano)
	}
	return out
}

func (s *MemoryReads) messagesTx(ctx context.Context, tx pgx.Tx, e readcontrol.Envelope, a readcontrol.Args, own, task string) (readcontrol.Result, error) {
	out := readcontrol.Empty("empty")
	var enabled bool
	if tx.QueryRow(ctx, `SELECT enabled FROM mobile_agent_access WHERE owner_id=$1 FOR SHARE`, own).Scan(&enabled) != nil || !enabled {
		return out, readcontrol.ErrDenied
	}
	var current bool
	if tx.QueryRow(ctx, `SELECT COALESCE(t.source_json->>'mobile_batch_id','')='' OR EXISTS(SELECT 1 FROM mobile_attention_batches b JOIN initiative_sources s ON s.source_id=b.source_id WHERE b.batch_id=t.source_json->>'mobile_batch_id' AND b.owner_id=$2 AND b.source_revision=s.revision AND s.enabled) FROM tasks t WHERE t.task_id=$1 AND t.owner_id=$2`, task, own).Scan(&current) != nil || !current {
		return out, readcontrol.ErrDenied
	}
	if e.Tool == readcontrol.MessageProcess {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,19))`, own); err != nil {
			return out, err
		}
		return s.processMessageTx(ctx, tx, e, a, own)
	}
	batch := ""
	if e.Tool == readcontrol.MessageInbox {
		if tx.QueryRow(ctx, `SELECT COALESCE(source_json->>'mobile_batch_id','') FROM tasks WHERE task_id=$1 AND owner_id=$2`, task, own).Scan(&batch) != nil || batch == "" {
			return out, readcontrol.ErrDenied
		}
	}
	rows, err := tx.Query(ctx, `SELECT m.message_id,m.part,m.state,s.source_id,s.head_revision,s.access_revision,r.content_hash
 FROM mobile_messages m JOIN memory_sources s ON s.source_id=m.source_id
 JOIN memory_source_revisions r ON r.source_id=s.source_id AND r.revision=s.head_revision
 JOIN mobile_events ev ON ev.source_id=s.source_id
 WHERE m.owner_id=$1 AND s.owner_id=$1 AND s.status='active' AND r.origin->>'sensitivity'<>'secret'
 AND ($2='' OR m.batch_id=$2 AND m.state='pending')
 AND ($3='' OR position(lower($3) in lower(COALESCE(CASE WHEN m.part<0 THEN COALESCE(NULLIF(r.body::jsonb->'payload'->>'big_text',''),r.body::jsonb->'payload'->>'text') ELSE r.body::jsonb->'payload'->'messages'->m.part->>'text' END,'')))>0)
 AND ($4='' OR ev.package_name=$4)
 AND ($5='' OR position(lower($5) in lower(COALESCE(CASE WHEN m.part<0 THEN r.body::jsonb->'payload'->>'title' ELSE r.body::jsonb->'payload'->'messages'->m.part->>'sender' END,'')))>0)
 AND ($6::timestamptz IS NULL OR ev.observed_at >= $6) AND ($7::timestamptz IS NULL OR ev.observed_at < $7)
 ORDER BY CASE WHEN $2<>'' THEN m.received_at END ASC,CASE WHEN $2='' THEN m.received_at END DESC,m.message_id LIMIT $8 OFFSET $9`, own, batch, a.Query, a.Package, a.Sender, optionalTime(a.EventFrom), optionalTime(a.EventUntil), a.Limit+1, a.Page*a.Limit)
	if err != nil {
		return out, err
	}
	found := []messageRow{}
	for rows.Next() {
		var m messageRow
		if err = rows.Scan(&m.id, &m.part, &m.state, &m.source.ID, &m.source.Revision, &m.source.AccessRevision, &m.source.Hash); err != nil {
			rows.Close()
			return out, err
		}
		found = append(found, m)
	}
	rows.Close()
	if rows.Err() != nil {
		return out, rows.Err()
	}
	if len(found) > a.Limit {
		out.Truncated = true
		found = found[:a.Limit]
	}
	textBytes := 0
	for _, m := range found {
		sources, err := s.sources.AccessTx(ctx, tx, []catalog.SourceRef{m.source}, "memory_read")
		if err != nil {
			return out, err
		}
		src := sources[0]
		var event mobile.Event
		if json.Unmarshal([]byte(src.Body), &event) != nil {
			return out, readcontrol.ErrDenied
		}
		part, ok := mobile.MessageParts(event)[m.part]
		if !ok {
			continue
		}
		item := messageItem(src, m.id, part.Text, map[string]any{"message_id": m.id, "package": event.Package, "sender": clipMessageLabel(part.Sender), "conversation": clipMessageLabel(event.Payload.Conversation), "state": m.state, "posted_at_epoch_ms": part.Timestamp})
		if textBytes+len(item.Text) > 8192 && len(out.Items) > 0 {
			out.Truncated = true
			break
		}
		textBytes += len(item.Text)
		out.Items = append(out.Items, item)
	}
	if len(out.Items) > 0 {
		out.Status = "ok"
	}
	return out, nil
}
func clipMessageLabel(raw *string) *string {
	if raw == nil {
		return nil
	}
	r := []rune(*raw)
	if len(r) > 128 {
		r = r[:128]
	}
	safe := sanitize.MaskCredentials(string(r))
	return &safe
}
func optionalTime(raw string) any {
	if raw == "" {
		return nil
	}
	t, _ := time.Parse(time.RFC3339Nano, raw)
	return t
}

func (s *MemoryReads) processMessageTx(ctx context.Context, tx pgx.Tx, e readcontrol.Envelope, a readcontrol.Args, own string) (readcontrol.Result, error) {
	out := readcontrol.Empty("ok")
	if sanitize.Text(a.Note) != a.Note || sanitize.Text(a.NoiseContains) != a.NoiseContains {
		return out, readcontrol.ErrDenied
	}
	// The exact source must already have been read through the owning run.
	var pinned bool
	if tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM memory_run_sources WHERE run_id=$1 AND source_id=$2 AND source_revision=$3)`, e.RunID, a.SourceID, a.Revision).Scan(&pinned) != nil || !pinned {
		return out, readcontrol.ErrDenied
	}
	refs := []catalog.SourceRef{{ID: a.SourceID, Revision: a.Revision, AccessRevision: a.AccessRevision, Hash: a.Hash}}
	srcs, err := s.sources.AccessTx(ctx, tx, refs, "memory_read")
	if err != nil {
		return out, err
	}
	var state, batch string
	var part int
	err = tx.QueryRow(ctx, `SELECT state,part,COALESCE(batch_id,'') FROM mobile_messages WHERE message_id=$1 AND owner_id=$2 AND source_id=$3 FOR UPDATE`, a.MessageID, own, a.SourceID).Scan(&state, &part, &batch)
	if err != nil {
		return out, readcontrol.ErrDenied
	}
	if state != "pending" {
		out.Diagnostics = []string{"already_processed"}
		return out, nil
	}
	resultID := ""
	switch a.Disposition {
	case "remember":
		observed := ""
		if srcs[0].EventTime != nil {
			observed = srcs[0].EventTime.UTC().Format(time.RFC3339Nano)
		}
		locator, _ := json.Marshal(map[string]any{"adapter": "android_message_note_v1", "message_id": a.MessageID, "original_source": refs[0], "observed_at": observed})
		body := "Наблюдение из уведомления от " + observed + ":\n" + a.Note
		derived, err := s.sources.CreateDerivedTx(ctx, tx, refs, "memory_read", "Message memory", body, locator)
		if err != nil {
			return out, err
		}
		resultID = derived.ID
		if err = s.sources.RetainMobileNoteTx(ctx, tx, catalog.SourceRef{ID: derived.ID, Revision: derived.Revision, AccessRevision: derived.AccessRevision, Hash: derived.Hash}); err != nil {
			return out, err
		}
		if _, err = s.sources.PublishFragmentsTx(ctx, tx, catalog.SourceRef{ID: derived.ID, Revision: derived.Revision, AccessRevision: derived.AccessRevision, Hash: derived.Hash}, []catalog.Fragment{{Key: "note", Title: derived.Title, Text: body, Locator: locator}}); err != nil {
			return out, err
		}
		state = "remembered"
		out.Items = append(out.Items, messageItem(derived, a.MessageID, body, map[string]any{"message_id": a.MessageID, "original_source": refs[0], "observed_at": observed}))
	case "noise":
		var event mobile.Event
		if json.Unmarshal([]byte(srcs[0].Body), &event) != nil {
			return out, readcontrol.ErrDenied
		}
		m, ok := mobile.MessageParts(event)[part]
		if !ok {
			return out, readcontrol.ErrDenied
		}
		rule := mobile.NoiseRule{Package: event.Package, Contains: strings.TrimSpace(a.NoiseContains)}
		if event.Channel != nil {
			rule.Channel = *event.Channel
		}
		if !rule.Matches(event, m) {
			return out, readcontrol.ErrDenied
		}
		// Serialized owner bound: concurrent tasks cannot grow an unlimited ruleset.
		if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,19))`, own); err != nil {
			return out, err
		}
		var n int
		if err = tx.QueryRow(ctx, `SELECT count(*) FROM mobile_noise_rules WHERE owner_id=$1 AND enabled`, own).Scan(&n); err != nil {
			return out, err
		}
		if n >= 32 {
			return out, readcontrol.ErrDenied
		}
		raw, _ := json.Marshal(rule)
		derived, err := s.sources.CreateDerivedTx(ctx, tx, refs, "memory_read", "Notification attention rule", string(raw), json.RawMessage(`{"adapter":"android_noise_rule_v1"}`))
		if err != nil {
			return out, err
		}
		resultID = derived.ID
		state = "noise"
		if _, err = tx.Exec(ctx, `INSERT INTO mobile_noise_rules(rule_id,owner_id) VALUES($1,$2)`, derived.ID, own); err != nil {
			return out, err
		}
		// Apply only to the current bounded batch. Matching uses this message
		// part, not the whole card (which may contain unrelated conversation).
		if batch != "" {
			_, err = tx.Exec(ctx, `UPDATE mobile_messages m SET state='noise',result_source_id=$2 FROM mobile_events ev,memory_sources s,memory_source_revisions r WHERE m.batch_id=$1 AND m.owner_id=$3 AND m.state='pending' AND ev.source_id=m.source_id AND ev.package_name=$4 AND s.source_id=m.source_id AND s.status='active' AND r.source_id=s.source_id AND r.revision=s.head_revision AND ($5='' OR r.body::jsonb->>'channel_id'=$5) AND position(lower($6) in lower(COALESCE(CASE WHEN m.part<0 THEN COALESCE(NULLIF(r.body::jsonb->'payload'->>'big_text',''),r.body::jsonb->'payload'->>'text') ELSE r.body::jsonb->'payload'->'messages'->m.part->>'text' END,'')))>0`, batch, derived.ID, own, rule.Package, rule.Channel, rule.Contains)
			if err != nil {
				return out, err
			}
		}
		out.Items = append(out.Items, messageItem(derived, a.MessageID, string(raw), map[string]string{"message_id": a.MessageID, "rule_id": derived.ID}))
	default:
		state = "ignored"
	}
	_, err = tx.Exec(ctx, `UPDATE mobile_messages SET state=$2,result_source_id=NULLIF($3,'') WHERE message_id=$1`, a.MessageID, state, resultID)
	if err != nil {
		return out, err
	}
	if batch != "" {
		_, err = tx.Exec(ctx, `UPDATE mobile_attention_batches b SET finished_at=clock_timestamp() WHERE batch_id=$1 AND NOT EXISTS(SELECT 1 FROM mobile_messages m WHERE m.batch_id=b.batch_id AND m.state='pending')`, batch)
	}
	return out, err
}
