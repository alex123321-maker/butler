package telegramrich

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/communication"
	"github.com/butler/butler/apps/orchestrator/internal/deliveryoutbox"
)

func renderWidget(v communication.WidgetView, base string) (string, any) {
	empty := map[string]any{"inline_keyboard": [][]any{}}
	// Bot API10.3 permits forced reply on an inline keyboard. Unlike standalone
	// ForceReply, that representation remains editable after the answer. Its
	// force_reply flag is immutable and must also be retained on terminal edits.
	if v.Question.Input != nil && v.Question.Input.Type == "text" {
		empty["force_reply"] = true
	}
	if v.State != "open" {
		state := map[string]string{"resolved": "Ответ принят. Вопрос закрыт.", "expired": "Срок ответа истёк.", "cancelled": "Вопрос отменён.", "superseded": "Вопрос заменён новым уточнением."}[v.State]
		if state == "" {
			state = "Вопрос закрыт."
		}
		return base + "\n\n" + state, empty
	}
	if v.Question.Input == nil {
		return base + "\n\nВопрос недоступен.", empty
	}
	q := v.Question.Input
	if q.Type == "text" {
		return base + "\n\nОтветьте на это сообщение текстом.", empty
	}
	prefix := "cw:" + v.Token + ":" + strconv.FormatInt(v.Version, 36) + ":" + strconv.FormatInt(v.DraftVersion, 36) + ":"
	rows := [][]map[string]string{}
	for index, o := range q.Options {
		label := []rune(o.Label)
		if len(label) > 48 {
			label = append(label[:48], '…')
		}
		op := "o"
		check := ""
		if q.Type == "multi_choice" {
			op = "t"
			check = "□ "
			if slices.Contains(v.Draft.OptionIDs, o.ID) {
				check = "☑ "
			}
		}
		rows = append(rows, []map[string]string{{"text": fmt.Sprintf("%s%d. %s", check, index+1, string(label)), "callback_data": prefix + op + strconv.Itoa(index)}})
	}
	if q.Type == "multi_choice" {
		rows = append(rows, []map[string]string{{"text": "Отправить ответ", "callback_data": prefix + "s"}})
		base += "\n\nВыбор — черновик. Нажмите «Отправить ответ», когда закончите."
	}
	if q.AllowText {
		base += "\n\nМожно ответить на это сообщение своим текстом."
	}
	return base, map[string]any{"inline_keyboard": rows}
}

func widgetRevision(v communication.WidgetView) string {
	return fmt.Sprintf("%d:%d:%s", v.Version, v.DraftVersion, v.State)
}

// RefreshWidgets journals edits of already acknowledged messages separately from
// publication delivery. An interrupted edit becomes UNKNOWN, never a new send.
// Only this adapter's numeric bot/binding can touch these exact message IDs.
func (p *Poller) RefreshWidgets(ctx context.Context) error {
	in, ok := p.Acceptor.(*Ingress)
	if !ok || !in.Widgets {
		return nil
	}
	if err := in.Store.Review(ctx); err != nil {
		return err
	}
	client, ok := p.Client.(*Client)
	if !ok {
		return ErrUnavailable
	}
	b := p.Binding
	_, err := p.Pool.Exec(ctx, `UPDATE communication_widget_projection_ops SET status='delivery_unknown' WHERE status='sending' AND created_at<clock_timestamp()-INTERVAL '30 seconds' AND notification_id IN (SELECT notification_id FROM delivery_outbox WHERE binding_id=$1 AND binding_version=$2)`, b.BindingID, b.Version)
	if err != nil {
		return err
	}
	rows, err := p.Pool.Query(ctx, `SELECT d.notification_id,d.publication_id,d.run_id,d.binding_id,d.binding_version,d.recipient_snapshot,d.content,d.channel_ack FROM delivery_outbox d JOIN communication_interactions x USING(publication_id) JOIN communication_widget_views w USING(interaction_id) WHERE d.binding_id=$1 AND d.binding_version=$2 AND d.channel='telegram' AND d.status='sent' AND d.channel_ack<>'' AND d.content::jsonb->>'widget'='true' AND NOT d.privacy_redacted AND NOT x.privacy_redacted AND (x.state<>'open' OR w.draft_version>1 OR x.expires_at<=clock_timestamp()) AND NOT EXISTS(SELECT 1 FROM communication_widget_projection_ops op WHERE op.notification_id=d.notification_id AND op.revision=x.version::text||':'||w.draft_version::text||':'||CASE WHEN x.state='open' AND x.expires_at<=clock_timestamp() THEN 'expired' ELSE x.state END) ORDER BY d.created_at,d.notification_id LIMIT 30`, b.BindingID, b.Version)
	if err != nil {
		return err
	}
	type item struct {
		m   deliveryoutbox.Message
		ack string
	}
	items := []item{}
	for rows.Next() {
		var it item
		it.m.Channel = "telegram"
		err = rows.Scan(&it.m.NotificationID, &it.m.PublicationID, &it.m.RunID, &it.m.BindingID, &it.m.BindingVersion, &it.m.Recipient, &it.m.Content, &it.ack)
		if err != nil {
			break
		}
		items = append(items, it)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return err
	}
	for _, it := range items {
		if err := p.refreshWidget(ctx, in, client, it.m, it.ack); err != nil {
			return err
		}
	}
	return nil
}

func (p *Poller) refreshWidget(ctx context.Context, in *Ingress, client *Client, message deliveryoutbox.Message, ack string) error {
	b := p.Binding
	// Serialise edits per known message across workers without blocking owner
	// responses on the outbox row. A newer projection cannot finish before an
	// older in-flight edit and then be overwritten by it.
	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	serial, e := p.Pool.Begin(ctx)
	if e != nil {
		return e
	}
	defer serial.Rollback(context.Background())
	var locked bool
	if e = serial.QueryRow(ctx, `SELECT pg_try_advisory_xact_lock(hashtextextended($1,104))`, message.NotificationID).Scan(&locked); e != nil {
		return e
	}
	if !locked {
		return nil
	}
	it := struct {
		m   deliveryoutbox.Message
		ack string
	}{message, ack}
	var part Part
	if communicationJSON(it.m.Content, &part) != nil || !part.Widget {
		return nil
	}
	view, e := in.Store.DeliveryWidget(ctx, it.m)
	if e != nil {
		return nil
	}
	revision := widgetRevision(view)
	tag, e := p.Pool.Exec(ctx, `INSERT INTO communication_widget_projection_ops(notification_id,revision,status) VALUES($1,$2,'sending') ON CONFLICT DO NOTHING`, it.m.NotificationID, revision)
	if e != nil {
		return e
	}
	if tag.RowsAffected() == 0 {
		return nil
	}
	status := "failed"
	current, e := in.Store.DeliveryWidget(ctx, it.m)
	if e == nil && widgetRevision(current) == revision {
		text, markup := renderWidget(current, part.Text)
		body, _ := json.Marshal(map[string]any{"chat_id": b.ChatID, "message_id": it.ack, "text": text, "reply_markup": markup})
		response, httpStatus, callErr := client.call(ctx, "editMessageText", body, "application/json")
		status = "delivery_unknown"
		if callErr == nil {
			if httpStatus == 200 && *response.OK {
				var m Message
				if json.Unmarshal(response.Result, &m) == nil && strconv.FormatInt(m.ID, 10) == it.ack && m.Chat.ID == b.ChatID {
					status = "sent"
				}
			}
			if !*response.OK && httpStatus >= 400 && httpStatus < 600 && response.Code >= 400 && response.Code < 600 {
				status = "failed"
			}
		}
	}
	finish, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	_, e = p.Pool.Exec(finish, `UPDATE communication_widget_projection_ops SET status=$3 WHERE notification_id=$1 AND revision=$2 AND status='sending'`, it.m.NotificationID, revision, status)
	cancel()
	if e != nil {
		return e
	}
	return nil
}

func communicationJSON(raw string, out any) error { return json.Unmarshal([]byte(raw), out) }
