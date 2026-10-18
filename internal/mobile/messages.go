package mobile

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/jackc/pgx/v5"
)

// MessageParts projects only distinct message content. Updating a progress bar,
// dismissing a card or replaying a MessagingStyle history cannot trigger work.
func MessageParts(e Event) map[int]Message {
	out := map[int]Message{}
	if e.Payload == nil || e.Change == "REMOVED" || e.GroupSummary || e.Omitted || e.OTPRedacted {
		return out
	}
	if e.Payload.Progress != nil && len(e.Payload.Messages) == 0 {
		return out
	}
	for i, m := range e.Payload.Messages {
		if strings.TrimSpace(m.Text) != "" {
			out[i] = m
		}
	}
	if len(out) > 0 {
		return out
	}
	p := e.Payload
	text := ""
	if p.BigText != nil {
		text = *p.BigText
	}
	if text == "" && p.Text != nil {
		text = *p.Text
	}
	if strings.TrimSpace(text) != "" {
		out[-1] = Message{Sender: p.Title, Text: text, Timestamp: e.Posted}
	}
	return out
}

func IndexMessagesTx(ctx context.Context, tx pgx.Tx, own, device, source string, e Event) error {
	for part, m := range MessageParts(e) {
		raw, _ := json.Marshal([]any{own, device, e.Package, e.Key, m.Sender, m.Text, m.Timestamp})
		_, err := tx.Exec(ctx, `INSERT INTO mobile_messages(message_id,owner_id,device_id,source_id,part) VALUES($1,$2,$3,$4,$5) ON CONFLICT DO NOTHING`, digest(raw), own, device, source, part)
		if err != nil {
			return err
		}
	}
	_, err := tx.Exec(ctx, `UPDATE mobile_events SET messages_indexed=true WHERE source_id=$1`, source)
	return err
}

// Bounded restartable upgrade of receipts created before message projection.
func (s *Store) IndexPendingMessages(ctx context.Context) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err = s.Catalog.CheckRestoreTx(ctx, tx); err != nil {
		return err
	}
	rows, err := tx.Query(ctx, `SELECT d.owner_id,e.device_id,e.source_id,r.body FROM mobile_events e JOIN mobile_devices d USING(device_id) JOIN memory_sources s USING(source_id) JOIN memory_source_revisions r ON r.source_id=s.source_id AND r.revision=s.head_revision WHERE NOT e.messages_indexed ORDER BY e.received_at LIMIT 100 FOR UPDATE OF e SKIP LOCKED`)
	if err != nil {
		return err
	}
	type item struct{ owner, device, source, body string }
	items := []item{}
	for rows.Next() {
		var p item
		if err = rows.Scan(&p.owner, &p.device, &p.source, &p.body); err != nil {
			rows.Close()
			return err
		}
		items = append(items, p)
	}
	rows.Close()
	if rows.Err() != nil {
		return rows.Err()
	}
	for _, p := range items {
		var event Event
		_ = json.Unmarshal([]byte(p.body), &event)
		if err = IndexMessagesTx(ctx, tx, p.owner, p.device, p.source, event); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// ConfigureAgentAccessTx is called only by the authenticated initiative source
// configuration boundary. It permits DATA reads, not instructions from sources.
func ConfigureAgentAccessTx(ctx context.Context, tx pgx.Tx, own string, enabled bool) error {
	_, err := tx.Exec(ctx, `INSERT INTO mobile_agent_access(owner_id,enabled) VALUES($1,$2) ON CONFLICT(owner_id) DO UPDATE SET enabled=$2,revision=mobile_agent_access.revision+1`, own, enabled)
	if err != nil {
		return err
	}
	status, previous := "quarantined", "active"
	if enabled {
		status, previous = previous, status
	}
	_, err = tx.Exec(ctx, `UPDATE memory_sources s SET status=$2,access_revision=access_revision+1 WHERE owner_id=$1 AND status=$3 AND EXISTS(SELECT 1 FROM mobile_events e WHERE e.source_id=s.source_id)`, own, status, previous)
	return err
}

type NoiseRule struct {
	Package  string `json:"package"`
	Channel  string `json:"channel"`
	Contains string `json:"contains"`
}

func (r NoiseRule) Matches(e Event, m Message) bool {
	return r.Package == e.Package && (r.Channel == "" || e.Channel != nil && r.Channel == *e.Channel) && len([]rune(r.Contains)) >= 4 && strings.Contains(strings.ToLower(m.Text), strings.ToLower(r.Contains))
}
