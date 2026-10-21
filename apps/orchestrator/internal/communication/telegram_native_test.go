package communication_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/artifacts"
	"github.com/butler/butler/apps/orchestrator/internal/channel/telegram"
	"github.com/butler/butler/apps/orchestrator/internal/channel/telegramrich"
	"github.com/butler/butler/apps/orchestrator/internal/communication"
	"github.com/butler/butler/apps/orchestrator/internal/completion"
	"github.com/butler/butler/apps/orchestrator/internal/deliveryoutbox"
	durableinput "github.com/butler/butler/apps/orchestrator/internal/input"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/internal/communicationcontrol"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type nativeDelivery struct {
	DSN, Schema, Owner, Run, Binding, Bot string
	Chat                                  int64
}

// This process really dies after persisting one native channel ACK. A second
// fresh process can only send the next saved part, never repeat the task/model.
func TestNativeTelegramRichDeliveryChild(t *testing.T) {
	if os.Getenv("BUTLER_NATIVE_TELEGRAM_CHILD") != "1" {
		return
	}
	var q nativeDelivery
	if json.Unmarshal([]byte(os.Getenv("BUTLER_NATIVE_TELEGRAM_REQUEST")), &q) != nil {
		t.Fatal("invalid native fixture")
	}
	cfg, err := pgxpool.ParseConfig(q.DSN)
	if err != nil || !strings.HasSuffix(cfg.ConnConfig.Database, "_test") || !strings.HasPrefix(q.Schema, "memory_") {
		t.Fatal("isolated fixture required")
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = q.Schema + ",public"
	cfg.MaxConns = 4
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	client, err := telegramrich.NewClientFromTokenFile(os.Getenv("BUTLER_TEST_TELEGRAM_RICH_TOKEN_FILE"))
	if err != nil {
		t.Fatal(err)
	}
	me, err := client.GetMe(ctx)
	if err != nil || me.Username != q.Bot {
		t.Fatal("wrong test bot")
	}
	core := &communication.Store{Pool: pool, Inputs: durableinput.NewStore(pool)}
	core.SourceGuard = func(ctx context.Context, tx pgx.Tx, run string) error {
		var ok bool
		if run != q.Run || tx.QueryRow(ctx, `SELECT TRUE FROM runs r JOIN sessions s USING(session_key) WHERE r.run_id=$1 AND s.user_id=$2`, run, q.Owner).Scan(&ok) != nil {
			return communication.ErrDenied
		}
		return nil
	}
	core.ChannelGuard = func(_ context.Context, _ pgx.Tx, run string, b communication.Binding, _ communicationcontrol.Args) error {
		if run != q.Run || b.ID != q.Binding || b.Channel != "telegram" || b.Recipient != "telegram:"+strconv.FormatInt(q.Chat, 10) {
			return communication.ErrDenied
		}
		return nil
	}
	d := &communication.DeliveryStore{Core: core}
	sender := &telegramrich.Sender{Client: client, Authorize: d.AuthorizeSend, Resolve: func(ctx context.Context, m deliveryoutbox.Message, file telegramrich.File) (artifacts.Media, error) {
		tx, err := pool.Begin(ctx)
		if err != nil {
			return artifacts.Media{}, err
		}
		defer tx.Rollback(context.Background())
		return artifacts.ResolveMediaTx(ctx, tx, m.RunID, file.Ref, func(ctx context.Context, tx pgx.Tx, run, ref string) error { return core.SourceGuard(ctx, tx, run) })
	}}
	binding := telegramrich.OwnerBinding{OwnerID: q.Owner, BindingID: q.Binding, Version: 2, ChatID: q.Chat, UserID: q.Chat, BotID: me.ID}
	controls := replyController(&fixture{pool: pool, s: core, leases: session.NewPostgresLeaseManager(pool)})
	poller := &telegramrich.Poller{Pool: pool, Client: client, Binding: binding, Acceptor: &telegramrich.Ingress{Store: core, Controls: controls, Binding: binding}}
	adapter, err := telegram.NewExplicitAdapter(poller, d, sender)
	if err != nil {
		t.Fatal(err)
	}
	w := deliveryoutbox.Worker{Store: d, Sender: adapter}
	if sent, err := w.ProcessOne(ctx); err != nil || !sent {
		t.Fatal("native stored-part delivery failed", err)
	}
	var n int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM delivery_outbox WHERE run_id=$1 AND status='sent'`, q.Run).Scan(&n); err != nil || n < 1 {
		t.Fatal("native ACK not durable")
	}
	os.Exit(93)
}

func TestNativeTelegramRichOutboxProcessRestartIntegration(t *testing.T) {
	if os.Getenv("BUTLER_TEST_TELEGRAM_RICH_TOKEN_FILE") == "" {
		t.Skip("explicit isolated test bot required")
	}
	chat, err := strconv.ParseInt(os.Getenv("BUTLER_TEST_TELEGRAM_RICH_CHAT_ID"), 10, 64)
	bot := os.Getenv("BUTLER_TEST_TELEGRAM_RICH_BOT")
	if err != nil || chat <= 0 || bot == "" {
		t.Fatal("explicit test owner and bot required")
	}
	f, photo := mediaFixture(t)
	if _, err = f.pool.Exec(f.ctx, `UPDATE communication_bindings SET recipient=$2 WHERE binding_id=$1`, f.bindings["telegram"], "telegram:"+strconv.FormatInt(chat, 10)); err != nil {
		t.Fatal(err)
	}
	doc, err := artifacts.NewService(artifacts.NewPostgresRepository(f.pool)).SaveMedia(f.ctx, f.p.RunID, f.p.SessionKey, "restart-check.txt", "text/plain", []byte("Butler: isolated durable native report, no production data.\n"))
	if err != nil {
		t.Fatal(err)
	}
	args := communicationcontrol.Args{SchemaVersion: 1, Purpose: "result", Title: "Butler — проверка восстановления", Summary: "Отчёт из сохранённого outbox. Каждый фрагмент отправляет новый процесс.", Blocks: []communicationcontrol.Block{{Type: "table", Rows: [][]string{{"Проверка", "Состояние"}, {"Хранение", "PostgreSQL"}, {"Доставка", "Новый процесс на каждую часть"}}}, {Type: "image", ArtifactRef: photo, Alt: "Синтетическое изображение"}, {Type: "document", ArtifactRef: doc, Alt: "Проверочный файл"}}}
	for n := 1; n <= 52; n++ {
		args.Blocks = append(args.Blocks, communicationcontrol.Block{Type: "paragraph", Text: "Проверка сохранённого блока " + strconv.Itoa(n) + ": границы частей и порядок доставки."})
	}
	raw, _ := json.Marshal(args)
	receipt, err := f.publish(t, "native-restart-report", "communication.report", string(raw))
	if err != nil {
		t.Fatal(err)
	}
	var saved []byte
	if err = f.pool.QueryRow(f.ctx, `SELECT content_json FROM artifacts WHERE artifact_id=$1 AND artifact_type='communication_report'`, artifacts.ReportID(receipt.PublicationRef)).Scan(&saved); err != nil {
		t.Fatal("canonical report missing", err)
	}
	var canonical communicationcontrol.Args
	if json.Unmarshal(saved, &canonical) != nil || len(canonical.Blocks) != 55 {
		t.Fatal("canonical report lost blocks")
	}
	var parts int
	if err = f.pool.QueryRow(f.ctx, `SELECT count(*) FROM delivery_outbox WHERE publication_id=$1`, receipt.PublicationRef).Scan(&parts); err != nil || parts != 2 {
		t.Fatal("native report did not split", parts, err)
	}
	q := nativeDelivery{DSN: f.pool.Config().ConnConfig.ConnString(), Schema: strings.Split(f.pool.Config().ConnConfig.RuntimeParams["search_path"], ",")[0], Owner: f.p.OwnerID, Run: f.p.RunID, Binding: f.bindings["telegram"], Bot: bot, Chat: chat}
	request, _ := json.Marshal(q)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for n := 0; n < parts; n++ {
		cmd := exec.CommandContext(f.ctx, exe, "-test.run=^TestNativeTelegramRichDeliveryChild$", "-test.timeout=25s")
		cmd.Env = append(os.Environ(), "BUTLER_NATIVE_TELEGRAM_CHILD=1", "BUTLER_NATIVE_TELEGRAM_REQUEST="+string(request))
		output, err := cmd.CombinedOutput()
		var exit *exec.ExitError
		if !errors.As(err, &exit) || exit.ExitCode() != 93 {
			t.Fatalf("native child did not reach durable ACK: %v %s", err, output)
		}
	}
	rows, err := f.pool.Query(f.ctx, `SELECT chunk_index,channel_ack,status FROM delivery_outbox WHERE publication_id=$1 ORDER BY chunk_index`, receipt.PublicationRef)
	if err != nil {
		t.Fatal(err)
	}
	last := int64(0)
	n := 0
	for rows.Next() {
		var index int
		var ack, state string
		if rows.Scan(&index, &ack, &state) != nil {
			t.Fatal("bad saved part")
		}
		id, err := strconv.ParseInt(ack, 10, 64)
		if err != nil || id <= last || index != n || state != "sent" {
			t.Fatal("native parts reordered or lost", index, ack, state)
		}
		last = id
		n++
		t.Logf("native report part=%d ACK=%s", index, ack)
	}
	rows.Close()
	if rows.Err() != nil || n != parts {
		t.Fatal("native report incomplete")
	}
	if _, err = f.pool.Exec(f.ctx, `UPDATE runs SET current_state='finalizing',status='finalizing' WHERE run_id=$1`, f.p.RunID); err != nil {
		t.Fatal(err)
	}
	c := completion.New(f.pool)
	c.Communication = f.s
	if _, err = c.Save(f.ctx, completion.Request{RunID: f.p.RunID, SessionKey: f.p.SessionKey, Content: "Другой внутренний итог", TerminalObserved: true}); err != nil {
		t.Fatal(err)
	}
	if f.count(t, "delivery_outbox") != parts || f.count(t, "communication_publications") != 1 {
		t.Fatal("internal final duplicated the native report")
	}
}
