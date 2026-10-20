package catalog

import (
	"context"
	"encoding/json"

	"github.com/butler/butler/internal/memory/sanitize"
	"github.com/butler/butler/internal/security/content"
	"github.com/jackc/pgx/v5"
)

type mobileNoteLocator struct {
	Adapter  string    `json:"adapter"`
	Message  string    `json:"message_id"`
	Original SourceRef `json:"original_source"`
}

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
	var loc mobileNoteLocator
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
	if len(originals) != 1 {
		return ErrDenied
	}
	if err = s.retainMobileEvidenceTx(ctx, tx, saved, originals[0], loc); err != nil {
		return err
	}
	_, err = s.AccessTx(ctx, tx, []SourceRef{note}, "memory_ingest")
	return err
}

// The normal caller has current Catalog access. Expiry/restore callers instead
// prove an existing remembered decision under the serialized erasure transaction;
// this helper never publishes model inputs, accepts claims or grants authority.
func (s *Store) retainMobileEvidenceTx(ctx context.Context, tx pgx.Tx, saved, original Source, loc mobileNoteLocator) error {
	note := SourceRef{ID: saved.ID, Revision: saved.Revision, AccessRevision: saved.AccessRevision, Hash: saved.Hash}
	if original.OwnerID != saved.OwnerID || original.Kind != "document" || saved.Kind != "model_candidate" || saved.Status != "active" || original.Status != "active" || original.Origin.Sensitivity == content.Secret || saved.Origin.Sensitivity == content.Secret || saved.ScopeType != "user" || saved.ScopeID != saved.OwnerID || original.ScopeType != "user" || original.ScopeID != saved.OwnerID || loc.Original != (SourceRef{ID: original.ID, Revision: original.Revision, AccessRevision: original.AccessRevision, Hash: original.Hash}) {
		return ErrDenied
	}
	var adapter struct {
		Adapter string `json:"adapter"`
	}
	if json.Unmarshal(original.Locator, &adapter) != nil || adapter.Adapter != "android_notification_v1" {
		return ErrDenied
	}
	var part int
	if err := tx.QueryRow(ctx, `SELECT part FROM mobile_messages WHERE message_id=$1 AND source_id=$2 AND owner_id=$3 AND (state='pending' OR state='remembered' AND result_source_id=$4) FOR SHARE`, loc.Message, loc.Original.ID, saved.OwnerID, note.ID).Scan(&part); err != nil {
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
	if json.Unmarshal([]byte(original.Body), &event) != nil {
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
	if err := tx.QueryRow(ctx, `SELECT count(*)=1 AND COALESCE(bool_and(parent_source_id=$2 AND parent_revision=$3),FALSE) FROM memory_source_dependencies WHERE child_source_id=$1`, note.ID, loc.Original.ID, loc.Original.Revision).Scan(&linked); err != nil {
		return err
	}
	if !linked {
		return ErrDenied
	}
	if _, err := tx.Exec(ctx, `INSERT INTO memory_retained_mobile_evidence(source_id,original_source_id,original_revision,original_access_revision,original_hash,message_id,evidence) VALUES($1,$2,$3,$4,$5,$6,$7)`, note.ID, loc.Original.ID, loc.Original.Revision, loc.Original.AccessRevision, loc.Original.Hash, loc.Message, evidence); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM memory_source_dependencies WHERE child_source_id=$1 AND parent_source_id=$2`, note.ID, loc.Original.ID); err != nil {
		return err
	}
	return nil
}

// An archive tombstone must also replay against a backup made before evidence
// migration. Preserve only its already-committed remembered decisions, while the
// original body is still available. No owner actor or access bypass is exported.
// Callers hold the deletion advisory fence and current device policy (cleanup),
// or the independent operator manifest/restore fence (reconciliation).
func (s *Store) prepareMobileArchiveExpiryTx(ctx context.Context, tx pgx.Tx, sourceID, own string) error {
	if err := lockDeletionAuthoritiesTx(ctx, tx, own); err != nil {
		return err
	}
	// Lock the complete pre-conversion privacy graph in ordinary erasure order.
	if _, err := lockDeletionSourceIDsTx(ctx, tx, sourceID, own, "archive_expire"); err != nil {
		return err
	}
	return s.retainExpiredMobileNotesTx(ctx, tx, sourceID, own)
}

func (s *Store) retainExpiredMobileNotesTx(ctx context.Context, tx pgx.Tx, sourceID, own string) error {
	rows, err := tx.Query(ctx, `SELECT n.source_id FROM mobile_messages m JOIN memory_sources n ON n.source_id=m.result_source_id WHERE m.source_id=$1 AND m.owner_id=$2 AND m.state='remembered' AND n.owner_id=$2 AND n.status='active' AND NOT EXISTS(SELECT 1 FROM memory_retained_mobile_evidence e WHERE e.source_id=n.source_id) ORDER BY n.source_id`, sourceID, own)
	if err != nil {
		return err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil || len(ids) == 0 {
		return err
	}
	original, err := getSourceTx(ctx, tx, sourceID, false)
	if err != nil || original.OwnerID != own {
		return ErrDenied
	}
	for _, id := range ids {
		note, err := getSourceTx(ctx, tx, id, true)
		if err != nil {
			return err
		}
		var exists bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM memory_retained_mobile_evidence WHERE source_id=$1)`, id).Scan(&exists); err != nil {
			return err
		}
		if exists {
			continue
		}
		var loc mobileNoteLocator
		if json.Unmarshal(note.Locator, &loc) != nil || loc.Adapter != "android_message_note_v1" || loc.Original.ID != sourceID {
			return ErrDenied
		}
		if err = s.retainMobileEvidenceTx(ctx, tx, note, original, loc); err != nil {
			return err
		}
	}
	return nil
}

// RetainedEvidenceTx releases only after the same current Catalog guard as body
// reads. Callers cannot use a stored ID to bypass source ownership or privacy.
func (s *Store) RetainedEvidenceTx(ctx context.Context, tx pgx.Tx, ref SourceRef) (json.RawMessage, error) {
	sources, err := s.AccessTx(ctx, tx, []SourceRef{ref}, "memory_read")
	if err != nil {
		return nil, err
	}
	for _, source := range sources {
		if source.ID == ref.ID {
			return source.RetainedEvidence, nil
		}
	}
	return nil, ErrDenied
}
