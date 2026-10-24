// Operator-only research importer. It is not installed in Butler bootstrap.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/channel/telegramrich"
	"github.com/jackc/pgx/v5"
)

type entry struct {
	Pack           string                    `json:"pack"`
	Sticker        telegramrich.EmojiSticker `json:"sticker"`
	Status         string                    `json:"status"`
	SHA256         string                    `json:"sha256,omitempty"`
	MIME           string                    `json:"mime,omitempty"`
	Bytes          int                       `json:"bytes,omitempty"`
	DuplicateGroup string                    `json:"exact_duplicate_group,omitempty"`
	Split          string                    `json:"split"`
	Duration       *float64                  `json:"duration_seconds"`
	DecodeStatus   string                    `json:"decode_status"`
	DownloadMS     int64                     `json:"download_ms,omitempty"`
	RetrievedAt    string                    `json:"retrieved_at,omitempty"`
}
type pack struct {
	Name           string                 `json:"name"`
	Status         string                 `json:"status"`
	SnapshotSHA256 string                 `json:"snapshot_sha256,omitempty"`
	Metadata       *telegramrich.EmojiSet `json:"metadata,omitempty"`
}
type manifest struct {
	Version      string  `json:"version"`
	Run          string  `json:"run"`
	SourceCommit string  `json:"source_commit"`
	Started      string  `json:"started_at"`
	Finished     string  `json:"finished_at"`
	Status       string  `json:"status"`
	Selection    string  `json:"selection"`
	Packs        []pack  `json:"packs"`
	Entries      []entry `json:"entries"`
	Selected     int     `json:"selected_downloads"`
	Downloaded   int     `json:"downloaded"`
	Failed       int     `json:"failed_downloads"`
}

func digest(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "emoji_research_failed")
		os.Exit(1)
	}
}
func run() error {
	token := flag.String("token-file", "", "private token reference")
	proxy := flag.String("proxy-file", "", "current private proxy reference (required)")
	dbfile := flag.String("database-file", "", "private URL of isolated butler_emoji140_test database")
	out := flag.String("out", "", "new private artifact directory")
	names := flag.String("packs", "AnimatedEmoji,CrayonsEmoji,FaceEmoji,Catz5Emoji,ABCEmoji,AIActions,Strawberry_hot_by_fStikBot", "explicit public pack names")
	limit := flag.Int("max-downloads", 450, "unique IDs to download, 0..450; discovery only, not a study split")
	commit := flag.String("source-commit", "", "source base revision")
	flag.Parse()
	if *token == "" || *proxy == "" || *dbfile == "" || !filepath.IsAbs(*out) || *limit < 0 || *limit > 450 || len(*commit) != 40 {
		return fmt.Errorf("configuration")
	}
	raw, err := os.ReadFile(*dbfile)
	if err != nil {
		return err
	}
	cfg, err := pgx.ParseConfig(strings.TrimSpace(string(raw)))
	if err != nil || cfg.Database != "butler_emoji140_test" {
		return fmt.Errorf("database")
	}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	ctx, timeout := context.WithTimeout(ctx, 30*time.Minute)
	defer timeout()
	db, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		return err
	}
	defer db.Close(context.Background())
	_, err = db.Exec(ctx, `CREATE TABLE IF NOT EXISTS emoji140_runs(id text PRIMARY KEY, payload jsonb NOT NULL);
 CREATE TABLE IF NOT EXISTS emoji140_entries(run_id text NOT NULL REFERENCES emoji140_runs(id),ordinal integer NOT NULL,payload jsonb NOT NULL,PRIMARY KEY(run_id,ordinal));`)
	if err != nil {
		return err
	}
	if err = os.Mkdir(*out, 0700); err != nil {
		return err
	}
	if err = os.Mkdir(filepath.Join(*out, "media"), 0700); err != nil {
		return err
	}
	c, err := telegramrich.NewClientFromTokenFileWithProxyFile(*token, *proxy)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	m := manifest{Version: "emoji140-inventory-v1", Run: now.Format("20060102T150405.000000000Z"), SourceCommit: *commit, Started: now.Format(time.RFC3339Nano), Status: "running", Selection: "round-robin packs; SHA256(custom_emoji_id) order; discovery pool only; no holdout or semantic quotas assigned", Packs: []pack{}, Entries: []entry{}}
	save := func() error {
		b, e := json.Marshal(m)
		if e != nil {
			return e
		}
		_, e = db.Exec(ctx, "INSERT INTO emoji140_runs VALUES($1,$2) ON CONFLICT(id) DO UPDATE SET payload=excluded.payload", m.Run, b)
		return e
	}
	saveEntry := func(i int) error {
		b, e := json.Marshal(m.Entries[i])
		if e != nil {
			return e
		}
		_, e = db.Exec(ctx, "INSERT INTO emoji140_entries VALUES($1,$2,$3) ON CONFLICT(run_id,ordinal) DO UPDATE SET payload=excluded.payload", m.Run, i, b)
		return e
	}
	if err = save(); err != nil {
		return err
	}
	groups := [][]int{}
	for _, name := range strings.Split(*names, ",") {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		s, e := c.GetCustomEmojiSet(ctx, name)
		p := pack{Name: name, Status: "unavailable"}
		group := []int{}
		if e == nil {
			b, _ := json.Marshal(s)
			p.Status = "inventoried"
			p.SnapshotSHA256 = digest(b)
			p.Metadata = &s
			for _, st := range s.Stickers {
				group = append(group, len(m.Entries))
				m.Entries = append(m.Entries, entry{Pack: s.Name, Sticker: st, Status: "not_selected", Split: "unassigned", DecodeStatus: "not_run"})
			}
			sort.Slice(group, func(i, j int) bool {
				return digest([]byte(m.Entries[group[i]].Sticker.CustomEmojiID)) < digest([]byte(m.Entries[group[j]].Sticker.CustomEmojiID))
			})
		}
		m.Packs = append(m.Packs, p)
		groups = append(groups, group)
		if err = save(); err != nil {
			return err
		}
		for _, i := range group {
			if err = saveEntry(i); err != nil {
				return err
			}
		}
		fmt.Printf("pack=%s status=%s items=%d\n", name, p.Status, len(group))
	}
	seen := map[string]int{}
	selected := []int{}
	for n := 0; len(selected) < *limit; n++ {
		available := false
		for _, g := range groups {
			if n >= len(g) {
				continue
			}
			available = true
			i := g[n]
			id := m.Entries[i].Sticker.CustomEmojiID
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = i
			selected = append(selected, i)
			if len(selected) == *limit {
				break
			}
		}
		if !available {
			break
		}
	}
	m.Selected = len(selected)
	if err = save(); err != nil {
		return err
	}
	for _, i := range selected {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		row := &m.Entries[i]
		start := time.Now()
		row.Status = "download_started"
		if err = saveEntry(i); err != nil {
			return err
		}
		b, mime, e := c.DownloadCustomEmoji(ctx, row.Sticker)
		row.DownloadMS = time.Since(start).Milliseconds()
		row.RetrievedAt = time.Now().UTC().Format(time.RFC3339Nano)
		if e != nil {
			row.Status = "download_failed"
			m.Failed++
		} else {
			row.SHA256 = digest(b)
			row.DuplicateGroup = row.SHA256
			row.MIME = mime
			row.Bytes = len(b)
			row.Status = "downloaded"
			dest := filepath.Join(*out, "media", row.SHA256)
			if old, e := os.ReadFile(dest); e == nil {
				if digest(old) != row.SHA256 {
					return fmt.Errorf("media_integrity")
				}
			} else if !os.IsNotExist(e) {
				return e
			} else if e = os.WriteFile(dest, b, 0600); e != nil {
				return e
			}
			m.Downloaded++
		}
		if err = saveEntry(i); err != nil {
			return err
		}
		if (m.Downloaded+m.Failed)%25 == 0 {
			if err = save(); err != nil {
				return err
			}
			fmt.Printf("downloaded=%d failed=%d selected=%d\n", m.Downloaded, m.Failed, m.Selected)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	// Metadata aliases remain explicit; a repeated ID shares the actual download.
	for i := range m.Entries {
		row := &m.Entries[i]
		if row.Status != "not_selected" {
			continue
		}
		if j, ok := seen[row.Sticker.CustomEmojiID]; ok && m.Entries[j].Status == "downloaded" {
			src := m.Entries[j]
			row.Status = "duplicate_id"
			row.SHA256 = src.SHA256
			row.DuplicateGroup = src.SHA256
			row.MIME = src.MIME
			row.Bytes = src.Bytes
			if err = saveEntry(i); err != nil {
				return err
			}
		}
	}
	m.Status = "inventory_complete"
	m.Finished = time.Now().UTC().Format(time.RFC3339Nano)
	if err = save(); err != nil {
		return err
	}
	// Export is a derivative of the durable database, not an alternative source.
	var exported []byte
	if err = db.QueryRow(ctx, "SELECT payload FROM emoji140_runs WHERE id=$1", m.Run).Scan(&exported); err != nil {
		return err
	}
	var stored manifest
	if err = json.Unmarshal(exported, &stored); err != nil {
		return err
	}
	exported, err = json.MarshalIndent(stored, "", "  ")
	if err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(*out, "manifest.json"), exported, 0600); err != nil {
		return err
	}
	fmt.Printf("run=%s catalog_entries=%d downloaded=%d failed=%d manifest_sha256=%s\n", m.Run, len(m.Entries), m.Downloaded, m.Failed, digest(exported))
	return nil
}
