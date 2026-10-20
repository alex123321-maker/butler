package catalog

import (
	"context"
	"encoding/json"

	"github.com/butler/butler/internal/memory/sanitize"
	"github.com/jackc/pgx/v5"
)

// RetainMobileNoteTx converts a copied-body dependency into an immutable typed
// privacy edge. Only an existing message decision or its invisible publication
// can be converted. The caller's current source authority is checked throughout.
// Evidence is extracted from the authorized original, never supplied by a model.
func (s *Store) RetainMobileNoteTx(ctx context.Context, tx pgx.Tx, note SourceRef) error {
	notes, err := s.AccessTx(ctx, tx, []SourceRef{note}, "memory_ingest")
	if err != nil {
		return err
	}
	var saved Source
	for _, n := range notes {
		if n.ID == note.ID {
			saved = n
		}
	}
	if saved.ID == "" {
		return ErrInvalid
	}
	var loc struct {
		Adapter  string    `json:"adapter"`
		Message  string    `json:"message_id"`
		Original SourceRef `json:"original_source"`
	}
	if json.Unmarshal(saved.Locator, &loc) != nil || loc.Adapter != "android_message_note_v1" || loc.Message == "" {
		return ErrInvalid
	}
	var exists bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM memory_retained_mobile_evidence WHERE source_id=$1)`, note.ID).Scan(&exists); err != nil || exists {
		return err
	}
	originals, err := s.AccessTx(ctx, tx, []SourceRef{loc.Original}, "memory_ingest")
	if err != nil {
		return err
	}
	if len(originals) != 1 || originals[0].OwnerID != saved.OwnerID || originals[0].Kind != "document" {
		return ErrDenied
	}
	var adapter struct {
		Adapter string `json:"adapter"`
	}
	if json.Unmarshal(originals[0].Locator, &adapter) != nil || adapter.Adapter != "android_notification_v1" {
		return ErrDenied
	}
	var part int
	if err = tx.QueryRow(ctx, `SELECT part FROM mobile_messages WHERE message_id=$1 AND source_id=$2 AND owner_id=$3 AND (state='pending' OR state='remembered' AND result_source_id=$4) FOR SHARE`, loc.Message, loc.Original.ID, saved.OwnerID, note.ID).Scan(&part); err != nil {
		return ErrDenied
	}
	var event struct {
		Package string `json:"package_name"`
		Posted  int64  `json:"posted_at_epoch_ms"`
		Payload struct {
			Title        string `json:"title"`
			Text         string `json:"text"`
			BigText      string `json:"big_text"`
			Conversation string `json:"conversation_title"`
			Messages     []struct {
				Sender    string `json:"sender"`
				Text      string `json:"text"`
				Timestamp int64  `json:"timestamp_epoch_ms"`
			} `json:"messages"`
		} `json:"payload"`
	}
	if json.Unmarshal([]byte(originals[0].Body), &event) != nil {
		return ErrInvalid
	}
	text, sender, at := event.Payload.BigText, event.Payload.Title, event.Posted
	if part >= 0 {
		if part >= len(event.Payload.Messages) {
			return ErrInvalid
		}
		m := event.Payload.Messages[part]
		text, sender, at = m.Text, m.Sender, m.Timestamp
	} else if text == "" {
		text = event.Payload.Text
	}
	if text == "" {
		return ErrInvalid
	}
	// Keep a bounded exact prefix; truncation is explicit and never paraphrased.
	quote := []rune(text)
	truncated := len(quote) > 2048
	if truncated {
		quote = quote[:2048]
	}
	clip := func(v string) string {
		r := []rune(v)
		if len(r) > 128 {
			r = r[:128]
		}
		return sanitize.MaskCredentials(string(r))
	}
	evidence, _ := json.Marshal(map[string]any{"original_source": loc.Original, "message_id": loc.Message, "package": clip(event.Package), "sender": clip(sender), "conversation": clip(event.Payload.Conversation), "posted_at_epoch_ms": at, "quote": sanitize.MaskCredentials(string(quote)), "truncated": truncated, "retention": "indefinite_until_explicit_forget", "trust": "DATA"})
	// A canonical ordinary edge must exist before conversion; no orphan adoption.
	var linked bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM memory_source_dependencies WHERE child_source_id=$1 AND parent_source_id=$2 AND parent_revision=$3)`, note.ID, loc.Original.ID, loc.Original.Revision).Scan(&linked); err != nil {
		return err
	}
	if !linked {
		return ErrDenied
	}
	if _, err = tx.Exec(ctx, `INSERT INTO memory_retained_mobile_evidence(source_id,original_source_id,original_revision,original_access_revision,original_hash,message_id,evidence) VALUES($1,$2,$3,$4,$5,$6,$7)`, note.ID, loc.Original.ID, loc.Original.Revision, loc.Original.AccessRevision, loc.Original.Hash, loc.Message, evidence); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM memory_source_dependencies WHERE child_source_id=$1 AND parent_source_id=$2`, note.ID, loc.Original.ID); err != nil {
		return err
	}
	_, err = s.AccessTx(ctx, tx, []SourceRef{note}, "memory_ingest")
	return err
}

// RetainedEvidenceTx releases only after the same current Catalog guard as body
// reads. Callers cannot use a stored ID to bypass source ownership or privacy.
func (s *Store) RetainedEvidenceTx(ctx context.Context, tx pgx.Tx, ref SourceRef) (json.RawMessage, error) {
	if _, err := s.AccessTx(ctx, tx, []SourceRef{ref}, "memory_read"); err != nil {
		return nil, err
	}
	var raw json.RawMessage
	err := tx.QueryRow(ctx, `SELECT evidence FROM memory_retained_mobile_evidence WHERE source_id=$1`, ref.ID).Scan(&raw)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return raw, err
}
