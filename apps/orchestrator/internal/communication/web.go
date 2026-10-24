package communication

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/artifacts"
	"github.com/butler/butler/internal/communicationcontrol"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/jackc/pgx/v5"
)

// WebReader serves only publications already pinned to this exact owner binding.
// It neither creates destinations for old history nor grants artifact capability.
type WebReader struct {
	Core    *Store
	Binding Binding
	Resolve func(context.Context, pgx.Tx, string, Binding, string) (artifacts.Media, error)
}

type WebFile struct {
	Ref    string `json:"artifact_ref"`
	Name   string `json:"name"`
	MIME   string `json:"mime"`
	SHA256 string `json:"sha256"`
	Size   int    `json:"size"`
}
type WebPart struct {
	Channel        string          `json:"channel"`
	BindingID      string          `json:"binding_id"`
	BindingVersion int64           `json:"binding_version"`
	Index          int             `json:"index"`
	State          string          `json:"state"`
	ACK            string          `json:"ack,omitempty"`
	Error          string          `json:"error,omitempty"`
	Manifest       json.RawMessage `json:"manifest,omitempty"`
	Projection     string          `json:"projection_state,omitempty"`
}
type WebPublication struct {
	Ref          string                     `json:"publication_ref"`
	Sequence     int64                      `json:"sequence"`
	Session      string                     `json:"session_ref,omitempty"`
	Task         string                     `json:"task_ref,omitempty"`
	TaskRevision int64                      `json:"task_revision,omitempty"`
	Kind         string                     `json:"kind,omitempty"`
	CreatedAt    time.Time                  `json:"created_at"`
	Available    bool                       `json:"available"`
	Payload      *communicationcontrol.Args `json:"payload,omitempty"`
	Interaction  *WidgetView                `json:"interaction,omitempty"`
	Files        []WebFile                  `json:"files"`
	Parts        []WebPart                  `json:"parts"`
}
type WebPage struct {
	Items    []WebPublication `json:"items"`
	Messages []WebInput       `json:"messages"`
	Cursor   int64            `json:"cursor"`
	HasMore  bool             `json:"has_more"`
}

type WebInput struct {
	ID        string    `json:"message_id"`
	Session   string    `json:"session_ref"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

func (w *WebReader) bindingTx(ctx context.Context, tx pgx.Tx) error {
	a, ok := owneridentity.FromContext(ctx)
	b := w.Binding
	if !ok || a.Channel != "web" || b.Channel != "web" || a.SessionKey != b.Recipient || b.ID == "" || b.Version < 1 || w.Core == nil || !w.Core.WebAvailable || w.Core.SourceGuard == nil || w.Core.ChannelGuard == nil || w.Resolve == nil {
		return ErrDenied
	}
	var yes bool
	if err := tx.QueryRow(ctx, `SELECT TRUE FROM communication_bindings WHERE binding_id=$1 AND version=$2 AND channel='web' AND owner_id=$3 AND recipient=$4 AND enabled AND verified FOR SHARE`, b.ID, b.Version, a.OwnerID, b.Recipient).Scan(&yes); err != nil {
		return ErrDenied
	}
	return nil
}

func mediaRefs(a communicationcontrol.Args) []string {
	refs := append([]string(nil), a.Attachments...)
	for _, im := range a.Images {
		refs = append(refs, im.ArtifactRef)
	}
	for _, block := range a.Blocks {
		if block.ArtifactRef != "" {
			refs = append(refs, block.ArtifactRef)
		}
	}
	if a.Input != nil {
		for _, option := range a.Input.Options {
			if option.ImageRef != "" {
				refs = append(refs, option.ImageRef)
			}
		}
	}
	seen := map[string]bool{}
	out := []string{}
	for _, ref := range refs {
		if !seen[ref] {
			seen[ref] = true
			out = append(out, ref)
		}
	}
	return out
}

// Plan validates every required attachment before any publication/outbox commit.
// The Web representation is the full canonical typed document, never a TG part.
func (w *WebReader) Plan(ctx context.Context, tx pgx.Tx, run string, b Binding, a communicationcontrol.Args) ([]string, error) {
	if b.ID != w.Binding.ID || b.Version != w.Binding.Version || b.Channel != "web" || b.Recipient != w.Binding.Recipient || w.Resolve == nil {
		return nil, ErrDenied
	}
	for _, block := range a.Blocks {
		if !communicationcontrol.ValidReportBlock(block) {
			return nil, ErrDenied
		}
	}
	for _, ref := range mediaRefs(a) {
		m, err := w.Resolve(ctx, tx, run, b, ref)
		if err != nil || m.Ref != ref || m.Validate() != nil {
			return nil, ErrDenied
		}
		if a.Input != nil {
			for _, option := range a.Input.Options {
				if option.ImageRef == ref && m.MIME != "image/png" && m.MIME != "image/jpeg" {
					return nil, ErrDenied
				}
			}
		}
	}
	raw, err := json.Marshal(a)
	if err != nil || len(raw) > 256*1024 {
		return nil, ErrDenied
	}
	return []string{string(raw)}, nil
}

func (w *WebReader) loadTx(ctx context.Context, tx pgx.Tx, id string) (WebPublication, string, error) {
	out := WebPublication{Ref: id, Files: []WebFile{}, Parts: []WebPart{}}
	actor, _ := owneridentity.FromContext(ctx)
	var run, kind string
	var raw []byte
	err := tx.QueryRow(ctx, `SELECT p.publication_sequence,p.session_key,COALESCE(p.task_id,''),p.task_revision,p.kind,p.created_at,p.payload,p.run_id FROM communication_publications p JOIN delivery_outbox d USING(publication_id) JOIN communication_bindings b ON b.binding_id=d.binding_id AND b.version=d.binding_version JOIN sessions s ON s.session_key=p.session_key WHERE p.publication_id=$1 AND p.owner_id=$2 AND s.user_id=$2 AND b.binding_id=$3 AND b.version=$4 AND b.channel='web' AND b.owner_id=$2 AND b.recipient=$5 AND d.recipient_snapshot=b.recipient AND b.enabled AND b.verified AND NOT p.privacy_redacted AND NOT d.privacy_redacted AND d.status='result_ready' FOR SHARE OF p,d,b,s`, id, actor.OwnerID, w.Binding.ID, w.Binding.Version, w.Binding.Recipient).Scan(&out.Sequence, &out.Session, &out.Task, &out.TaskRevision, &kind, &out.CreatedAt, &raw, &run)
	if err != nil {
		return out, "", err
	}
	a, err := communicationcontrol.DecodeArgs("communication."+kind, string(raw))
	if err != nil || w.Core.SourceGuard(ctx, tx, run) != nil || w.Core.ChannelGuard(ctx, tx, run, w.Binding, a) != nil {
		return out, "", ErrDenied
	}
	for _, ref := range mediaRefs(a) {
		m, e := w.Resolve(ctx, tx, run, w.Binding, ref)
		if e != nil || m.Ref != ref || m.Validate() != nil {
			return out, "", ErrDenied
		}
		out.Files = append(out.Files, WebFile{Ref: ref, Name: m.Name, MIME: m.MIME, SHA256: m.SHA256, Size: len(m.Data)})
	}
	rows, err := tx.Query(ctx, `SELECT d.channel,d.binding_id,d.binding_version,d.chunk_index,d.status,d.channel_ack,d.error_code,d.content,COALESCE((SELECT op.status FROM communication_widget_projection_ops op WHERE op.notification_id=d.notification_id ORDER BY op.created_at DESC,op.revision DESC LIMIT 1),'') FROM delivery_outbox d JOIN communication_bindings b ON b.binding_id=d.binding_id AND b.version=d.binding_version WHERE d.publication_id=$1 AND b.owner_id=$2 AND b.enabled AND b.verified AND b.recipient=d.recipient_snapshot AND NOT d.privacy_redacted ORDER BY d.binding_id,d.chunk_index`, id, actor.OwnerID)
	if err != nil {
		return out, "", err
	}
	for rows.Next() {
		var p WebPart
		var content string
		if err = rows.Scan(&p.Channel, &p.BindingID, &p.BindingVersion, &p.Index, &p.State, &p.ACK, &p.Error, &content, &p.Projection); err != nil {
			rows.Close()
			return out, "", err
		}
		if p.Channel == "web" && p.State == "result_ready" {
			p.State = "available"
		}
		if content != "" {
			if json.Valid([]byte(content)) {
				p.Manifest = json.RawMessage(content)
			} else {
				p.Manifest, _ = json.Marshal(map[string]string{"text": content})
			}
		}
		out.Parts = append(out.Parts, p)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, "", err
	}
	// Repeat after media reads/lock waits; locks remain held until response data
	// has been assembled. Availability never records a human read or a send ACK.
	if w.bindingTx(ctx, tx) != nil || w.Core.SourceGuard(ctx, tx, run) != nil || w.Core.ChannelGuard(ctx, tx, run, w.Binding, a) != nil {
		return out, "", ErrDenied
	}
	out.Kind = kind
	out.Payload = &a
	if kind == "widget" {
		view, e := widgetViewTx(ctx, tx, id)
		if e != nil {
			return out, "", e
		}
		out.Interaction = &view
	}
	out.Available = true
	return out, run, nil
}

func (w *WebReader) Feed(ctx context.Context, after int64, limit int) (WebPage, error) {
	out := WebPage{Items: []WebPublication{}, Messages: []WebInput{}, Cursor: after}
	if w == nil || w.Core == nil || w.Core.Pool == nil || after < 0 || limit < 1 || limit > 100 {
		return out, ErrDenied
	}
	tx, err := w.Core.Pool.Begin(ctx)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(context.Background())
	if err = w.bindingTx(ctx, tx); err != nil {
		return out, err
	}
	a, _ := owneridentity.FromContext(ctx)
	rows, err := tx.Query(ctx, `SELECT DISTINCT p.publication_id,p.publication_sequence FROM communication_publications p JOIN delivery_outbox d USING(publication_id) WHERE p.owner_id=$1 AND d.binding_id=$2 AND d.binding_version=$3 AND p.publication_sequence>$4 ORDER BY p.publication_sequence LIMIT $5`, a.OwnerID, w.Binding.ID, w.Binding.Version, after, limit+1)
	if err != nil {
		return out, err
	}
	type ref struct {
		id  string
		seq int64
	}
	refs := []ref{}
	for rows.Next() {
		var r ref
		if err = rows.Scan(&r.id, &r.seq); err != nil {
			rows.Close()
			return out, err
		}
		refs = append(refs, r)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	out.HasMore = len(refs) > limit
	if out.HasMore {
		refs = refs[:limit]
	}
	runs := []string{}
	for _, r := range refs {
		p, run, e := w.loadTx(ctx, tx, r.id)
		if e != nil && !errors.Is(e, ErrDenied) && !errors.Is(e, pgx.ErrNoRows) {
			return out, e
		}
		if e != nil {
			p = WebPublication{Ref: r.id, Sequence: r.seq, Files: []WebFile{}, Parts: []WebPart{}}
		}
		if p.Available {
			runs = append(runs, run)
		}
		out.Items = append(out.Items, p)
		out.Cursor = r.seq
	}
	rows, err = tx.Query(ctx, `SELECT message_id,session_key,content,created_at FROM messages WHERE run_id=ANY($1::text[]) AND role='user' AND NOT privacy_redacted AND COALESCE(metadata->>'kind','')<>'interaction_response' ORDER BY created_at,message_id`, runs)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var m WebInput
		if err = rows.Scan(&m.ID, &m.Session, &m.Content, &m.CreatedAt); err != nil {
			rows.Close()
			return out, err
		}
		out.Messages = append(out.Messages, m)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	return out, tx.Commit(ctx)
}

// RefreshCursor releases no publication data or source/artifact identity. SSE
// only invalidates REST pages, whose full current export proof remains mandatory.
// Avoid replaying every historical source graph and retaining transcript locks
// while an owning writer is publishing the answer that triggers that refresh.
func (w *WebReader) RefreshCursor(ctx context.Context, after int64) (int64, error) {
	if w == nil || w.Core == nil || w.Core.Pool == nil || after < 0 {
		return 0, ErrDenied
	}
	tx, err := w.Core.Pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(context.Background())
	if err = w.bindingTx(ctx, tx); err != nil {
		return 0, err
	}
	a, _ := owneridentity.FromContext(ctx)
	var cursor int64
	if err = tx.QueryRow(ctx, `SELECT COALESCE(max(p.publication_sequence),$4) FROM communication_publications p JOIN delivery_outbox d USING(publication_id) WHERE p.owner_id=$1 AND d.binding_id=$2 AND d.binding_version=$3 AND p.publication_sequence>$4`, a.OwnerID, w.Binding.ID, w.Binding.Version, after).Scan(&cursor); err != nil {
		return 0, err
	}
	return cursor, tx.Commit(ctx)
}

func (w *WebReader) Detail(ctx context.Context, id string) (WebPublication, error) {
	if w == nil || w.Core == nil || w.Core.Pool == nil || len(id) > 128 {
		return WebPublication{}, ErrDenied
	}
	tx, err := w.Core.Pool.Begin(ctx)
	if err != nil {
		return WebPublication{}, err
	}
	defer tx.Rollback(context.Background())
	if err = w.bindingTx(ctx, tx); err != nil {
		return WebPublication{}, err
	}
	p, _, err := w.loadTx(ctx, tx, id)
	if err != nil {
		return WebPublication{}, err
	}
	return p, tx.Commit(ctx)
}

func (w *WebReader) Media(ctx context.Context, id, ref string) (artifacts.Media, error) {
	if w == nil || w.Core == nil || w.Core.Pool == nil || len(id) > 128 || len(ref) > 512 {
		return artifacts.Media{}, ErrDenied
	}
	tx, err := w.Core.Pool.Begin(ctx)
	if err != nil {
		return artifacts.Media{}, err
	}
	defer tx.Rollback(context.Background())
	if err = w.bindingTx(ctx, tx); err != nil {
		return artifacts.Media{}, err
	}
	p, run, err := w.loadTx(ctx, tx, id)
	if err != nil {
		return artifacts.Media{}, err
	}
	allowed := false
	for _, f := range p.Files {
		allowed = allowed || f.Ref == ref
	}
	if !allowed {
		return artifacts.Media{}, ErrDenied
	}
	m, err := w.Resolve(ctx, tx, run, w.Binding, ref)
	if err != nil {
		return artifacts.Media{}, err
	}
	if w.bindingTx(ctx, tx) != nil || w.Core.SourceGuard(ctx, tx, run) != nil || w.Core.ChannelGuard(ctx, tx, run, w.Binding, *p.Payload) != nil {
		return artifacts.Media{}, ErrDenied
	}
	return m, tx.Commit(ctx)
}
