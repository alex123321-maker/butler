package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/prompt"
	"github.com/butler/butler/internal/transport/antigravity"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Only these independent corpus bindings may change after fresh authorship.
// Shared fixture/candidate/provider semantics were frozen at22e834fb.
const behavior76FreshStimuliSHA = "8d064bab832f2b3e71fe61260b7e918f413c47fb6df0f96675375f999de2160b"
const behavior76FreshExpectationsSHA = "7479b3d977b413a96fa7d37fb8dc1a899b428ccaaabac2343c367bc605947e89"
const behavior76FreshMaxEntries = 768

func behavior76HistoricalRowsEqual(ctx context.Context, tx pgx.Tx, body []byte) (bool, error) {
	lines := strings.Split(strings.TrimSpace(string(body)), "\n")
	array := "[" + strings.Join(lines, ",") + "]"
	if len(lines) != 174 || !json.Valid([]byte(array)) {
		return false, fmt.Errorf("sealed complete attempt inventory unavailable")
	}
	var exact bool
	err := tx.QueryRow(ctx, `SELECT count(*)=174 AND bool_and(j.value IS NOT NULL AND a.window_id IS NOT NULL AND j.value=to_jsonb(a)) FROM jsonb_array_elements($1::jsonb) AS j(value) FULL JOIN public.behavior76_attempts a ON a.window_id=j.value->>'window_id' AND a.n=(j.value->>'n')::integer`, array).Scan(&exact)
	return exact, err
}

func TestBehavior76FreshHistoryBoundaryIntegration(t *testing.T) {
	if os.Getenv("BUTLER_TEST_MEMORY_POSTGRES_URL") == "" {
		t.Skip("isolated original local history required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	config, err := pgxpool.ParseConfig(os.Getenv("BUTLER_TEST_MEMORY_POSTGRES_URL"))
	if err != nil || config.ConnConfig.Database != "behavior76_test" || config.ConnConfig.Host != "127.0.0.1" {
		t.Fatal("isolated loopback history required")
	}
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	body, err := os.ReadFile("/src20/.cache/behavior76/native-20261025c/behavior76_attempts.jsonl")
	if err != nil {
		t.Fatal("sealed history required")
	}
	if same, e := behavior76HistoricalRowsEqual(ctx, tx, body); e != nil || !same {
		t.Fatal("original history equality failed", e)
	}
	lines := strings.Split(strings.TrimSpace(string(body)), "\n")
	var first map[string]json.RawMessage
	if err = json.Unmarshal([]byte(lines[0]), &first); err != nil {
		t.Fatal(err)
	}
	first["stage"] = json.RawMessage(`"tampered-public-test-stage"`)
	changed, _ := json.Marshal(first)
	lines[0] = string(changed)
	if same, e := behavior76HistoricalRowsEqual(ctx, tx, []byte(strings.Join(lines, "\n"))); e != nil || same {
		t.Fatal("changed row with unchanged count/token subtotal was accepted", e)
	}
	if same, e := behavior76HistoricalRowsEqual(ctx, tx, []byte(strings.Join(lines[1:], "\n"))); e == nil || same {
		t.Fatal("truncated historical inventory accepted")
	}
}

func TestBehavior76NativeFreshLocal(t *testing.T) {
	if os.Getenv("BUTLER_BEHAVIOR76_NATIVE_ENABLE") != "fresh-synthetic-local-only" {
		t.Skip("explicit separate fresh native experiment required")
	}
	if len(behavior76FreshStimuliSHA) != 64 || len(behavior76FreshExpectationsSHA) != 64 {
		t.Fatal("independent fresh corpus is not bound")
	}
	behavior76CollectFresh(t)
}

func behavior76CollectFresh(t *testing.T) {
	raw, err := os.ReadFile(os.Getenv("BUTLER_BEHAVIOR76_STIMULI"))
	if err != nil || behavior76Hash(raw) != behavior76FreshStimuliSHA {
		t.Fatal("fresh independent stimulus commitment unavailable")
	}
	episodes, err := behavior76ReadEpisodes(strings.NewReader(string(raw)))
	if err != nil || len(episodes) != 24 {
		t.Fatal("fresh independent interface unavailable")
	}
	families := map[string]int{}
	for _, e := range episodes {
		evaluated := 0
		for _, step := range e.Steps {
			if step.Evaluate {
				evaluated++
			}
		}
		if evaluated != 2 {
			t.Fatal("each fresh episode requires exactly two evaluated owner opportunities")
		}
		families[e.Family]++
	}
	if len(families) != 12 {
		t.Fatal("fresh full family coverage required")
	}
	for _, count := range families {
		if count != 2 {
			t.Fatal("two fresh episodes per family required")
		}
	}
	dir, source := os.Getenv("BUTLER_BEHAVIOR76_OUTPUT"), os.Getenv("BUTLER_BEHAVIOR76_SOURCE")
	if dir == "" || source == "" {
		t.Fatal("explicit clean producer/output required")
	}
	if err = os.Mkdir(dir, 0700); err != nil {
		t.Fatal("new fresh evidence directory required", err)
	}
	index := behavior76Index{Version: "behavior76-checker-index-v1", StimuliSHA256: behavior76FreshStimuliSHA, Episodes: []behavior76IndexEpisode{}}
	defer func() { behavior76Write(t, filepath.Join(dir, "index.json"), index) }()
	root := filepath.Dir(dir)
	for _, old := range []struct {
		directory, inventory, hash string
		count                      int
	}{
		{"native-20261025b", "native-b-raw-sha256.json", "c028dfbb5e782fa78bc35824d6e149ab751adabe982168f5a589cf167fabb848", 1492},
		{"native-20261025c", "native-c-raw-sha256.json", "0fce72bbfdf54299890e527a261e599ba0dbc36ad5b783b3eb177f5d293f99c2", 50},
	} {
		if err = behavior76CheckRawFiles(filepath.Join(root, old.directory), filepath.Join(root, old.inventory), old.hash, old.count); err != nil {
			t.Fatal(err)
		}
	}
	started := time.Now().UTC()
	deadline := started.Add(4 * time.Hour)
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()
	config, err := pgxpool.ParseConfig(os.Getenv("BUTLER_TEST_MEMORY_POSTGRES_URL"))
	if err != nil || config.ConnConfig.Database != "behavior76_test" || config.ConnConfig.Host != "127.0.0.1" {
		t.Fatal("isolated local original control database required")
	}
	config.MaxConns = 2
	journal, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer journal.Close()
	var settingsKey bool
	if err = journal.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='public.system_settings'::regclass AND contype='p' AND conkey=ARRAY[(SELECT attnum FROM pg_attribute WHERE attrelid='public.system_settings'::regclass AND attname='key')]::smallint[])`).Scan(&settingsKey); err != nil || !settingsKey {
		t.Fatal("original encrypted settings primary key required")
	}
	transaction, err := journal.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer transaction.Rollback(context.Background())
	if _, err = transaction.Exec(ctx, `LOCK TABLE public.behavior76_windows IN SHARE ROW EXCLUSIVE MODE;LOCK TABLE public.behavior76_attempts IN SHARE MODE`); err != nil {
		t.Fatal(err)
	}
	prior := []map[string]any{}
	for _, old := range []struct {
		id, directory, hash string
		admitted            int
	}{
		{"issue76-local-20261025T104513", "native-20261025a", "712670def12bb6ab11d7d689ed39c3a46bbd6455988594f10aaf9c421e9834ae", 1},
		{behavior76BWindow, "native-20261025b", behavior76BManifestSHA, 171},
		{"issue76-local-20261025T123250", "native-20261025c", "c464f701c7dad25cfc27519692af2b12a7e89e23e4824eb9b9e280cfee802d52", 2},
	} {
		body, e := os.ReadFile(filepath.Join(root, old.directory, "manifest.json"))
		if e != nil || behavior76Hash(body) != old.hash {
			t.Fatal("original predecessor manifest changed")
		}
		var exact bool
		if e = transaction.QueryRow(ctx, `SELECT closed AND admitted=$2 AND manifest=$3::jsonb AND (SELECT count(*)=$2 FROM public.behavior76_attempts WHERE window_id=$1) FROM public.behavior76_windows WHERE id=$1`, old.id, old.admitted, string(body)).Scan(&exact); e != nil || !exact {
			t.Fatal("original predecessor history cannot be replaced")
		}
		prior = append(prior, map[string]any{"id": old.id, "admitted": old.admitted, "manifest_sha256": old.hash, "closed": true})
	}
	var history bool
	if err = transaction.QueryRow(ctx, `SELECT (SELECT count(*)=3 AND bool_and(closed) AND sum(admitted)=174 FROM public.behavior76_windows) AND (SELECT count(*)=174 AND count(total_tokens)=172 AND sum(total_tokens)=1274988 AND count(*) FILTER(WHERE total_tokens IS NULL)=2 FROM public.behavior76_attempts)`).Scan(&history); err != nil || !history {
		t.Fatal("fresh-only admission requires exactly closed A/B/C and all spent/unknown usage")
	}
	oldAttempts, err := os.ReadFile(filepath.Join(root, "native-20261025c", "behavior76_attempts.jsonl"))
	if err != nil {
		t.Fatal("sealed historical attempts unavailable")
	}
	if exactRows, e := behavior76HistoricalRowsEqual(ctx, transaction, oldAttempts); e != nil || !exactRows {
		t.Fatal("each historical attempt must exactly equal its sealed original JSONB row")
	}
	var entropy [33]byte
	if _, err = rand.Read(entropy[:]); err != nil {
		t.Fatal("fresh masking entropy unavailable")
	}
	arms := []string{"arm-" + hex.EncodeToString(entropy[:16]), "arm-" + hex.EncodeToString(entropy[16:32])}
	mapping := map[string]bool{arms[0]: entropy[32]&1 == 1, arms[1]: entropy[32]&1 == 0}
	behavior76Write(t, filepath.Join(root, filepath.Base(dir)+"-private-arm-mapping.json"), mapping)
	schedule := []map[string]any{}
	for n, e := range episodes {
		schedule = append(schedule, map[string]any{"episode_id": e.ID, "arm_order": []string{arms[n%2], arms[(n+1)%2]}})
	}
	window := "issue76-fresh-" + started.Format("20060102T150405")
	manifest := map[string]any{"version": "behavior76-fresh-independent-v3", "window": window, "source": source, "stimuli_sha256": behavior76FreshStimuliSHA, "expectations_sha256": behavior76FreshExpectationsSHA, "mapping_sha256": behavior76Hash([]byte(behavior76JSON(mapping))), "schedule": schedule, "max_native_entries": behavior76FreshMaxEntries, "max_per_turn": 8, "max_per_run": 4, "wall_millis": (4 * time.Hour).Milliseconds(), "started_at": started.Format(time.RFC3339Nano), "absolute_deadline": deadline.Format(time.RFC3339Nano), "native": true, "model": antigravity.FlashHighModel, "baseline_prompt": prompt.DefaultBasePrompt, "candidate_addition": prompt.TaskContinuityProtocol, "output_contract": behavior76OutputContract, "prior_history": prior, "prior_admissions": 174, "prior_measured": 172, "prior_known_tokens": 1274988, "prior_unavailable_usage": 2, "cumulative_admission_ceiling": 942, "separate_new_experiment": true, "prior_experiment_complete": false, "global_stop_unchanged": true}
	behavior76Write(t, filepath.Join(dir, "manifest.json"), manifest)
	if _, err = transaction.Exec(ctx, `INSERT INTO public.behavior76_windows(id,manifest,expires_at) VALUES($1,$2,$3)`, window, behavior76JSON(manifest), deadline); err != nil {
		t.Fatal("fresh window cannot reopen", err)
	}
	if err = transaction.Commit(ctx); err != nil {
		t.Fatal("fresh window admission unavailable", err)
	}
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 20*time.Second)
		defer stop()
		if tag, err := journal.Exec(cleanup, `UPDATE public.behavior76_windows SET closed=true WHERE id=$1`, window); err != nil || tag.RowsAffected() != 1 {
			t.Error("fresh window closure unavailable")
		}
		converterLocalNativeExport(t, journal, dir, "public", []string{"behavior76_windows", "behavior76_attempts"})
	}()
	var sealed atomic.Bool
	for n, e := range episodes {
		for _, arm := range []string{arms[n%2], arms[(n+1)%2]} {
			if sealed.Load() || ctx.Err() != nil {
				t.Fatal("fresh window stopped; original partial/failed/missing outcomes retained")
			}
			ok := t.Run(fmt.Sprintf("slot-%02d-%s", n+1, arm), func(t *testing.T) {
				base := prompt.DefaultBasePrompt
				if mapping[arm] {
					base += "\n\n" + prompt.TaskContinuityProtocol
				}
				t.Setenv(prompt.SettingKeyBasePrompt, base)
				t.Setenv(prompt.SettingKeyBasePromptEnabled, "true")
				f := newBehavior76Fixture(t, ctx)
				f.seed(e)
				relative := fmt.Sprintf("slot-%02d/%s", n+1, arm)
				caseDir := filepath.Join(dir, filepath.FromSlash(relative))
				if err = os.MkdirAll(caseDir, 0700); err != nil {
					t.Fatal(err)
				}
				item := behavior76IndexEpisode{EpisodeID: e.ID, MaskedArmID: arm, AliasesPath: relative + "/aliases.json", TablesDir: relative + "/tables", Steps: []behavior76IndexStep{}}
				defer func() {
					defer f.retainOnEvidenceFailure()
					if !f.backupPrivate(fmt.Sprintf("%s-slot-%02d-%s", window, n+1, arm)) {
						sealed.Store(true)
					}
					f.export(filepath.Join(caseDir, "tables"))
					behavior76Write(t, filepath.Join(caseDir, "aliases.json"), f.aliases())
					index.Episodes = append(index.Episodes, item)
				}()
				for j, step := range e.Steps {
					if filepath.Base(step.ID) != step.ID || strings.Contains(step.ID, "..") {
						t.Fatal("invalid fresh step path")
					}
					stepDir := filepath.Join(caseDir, fmt.Sprintf("step-%02d", j+1))
					stepRel := relative + fmt.Sprintf("/step-%02d", j+1)
					behavior76Write(t, filepath.Join(stepDir, "before.json"), f.snapshot())
					receipt, eventErr := f.apply(step)
					outcome := behavior76Outcome{State: "not_evaluated", RunIDs: []string{}, AutomaticRunIDs: []string{}, NativeEntryIDs: []int{}, EventReceipt: receipt}
					if eventErr != nil {
						outcome.State = "failed"
						outcome.ErrorCode = behavior76SafeError(eventErr)
						outcome.FailurePhase = "fixture_event"
						sealed.Store(true)
					} else if step.Evaluate {
						provider := behavior76NativeFactory(t, journal, f, window, relative+"/"+step.ID, &sealed)
						native, ok := provider.(*behavior76NativeProvider)
						if !ok {
							t.Fatal("ordinary native collector required")
						}
						native.entryLimit = behavior76FreshMaxEntries
						outcome, eventErr = f.evaluate(provider, step)
						outcome.EventReceipt = receipt
						rows, e := journal.Query(ctx, `SELECT n FROM public.behavior76_attempts WHERE window_id=$1 AND slot=$2 ORDER BY n`, window, relative+"/"+step.ID)
						if e != nil {
							t.Fatal("fresh entry correlation unavailable")
						}
						for rows.Next() {
							var id int
							if e = rows.Scan(&id); e != nil {
								t.Fatal(e)
							}
							outcome.NativeEntryIDs = append(outcome.NativeEntryIDs, id)
						}
						readErr := rows.Err()
						rows.Close()
						if readErr != nil {
							sealed.Store(true)
							t.Fatal("fresh entry correlation ended before complete read")
						}
						if eventErr != nil {
							outcome.State = "failed"
							outcome.ErrorCode = behavior76SafeError(eventErr)
							outcome.FailurePhase = "collector"
							sealed.Store(true)
						}
					}
					behavior76Write(t, filepath.Join(stepDir, "after.json"), f.snapshot())
					behavior76Write(t, filepath.Join(stepDir, "outcome.json"), outcome)
					item.Steps = append(item.Steps, behavior76IndexStep{StepID: step.ID, Before: stepRel + "/before.json", After: stepRel + "/after.json", Outcome: stepRel + "/outcome.json"})
					if sealed.Load() {
						t.Fatal("fresh window sealed; preserve partial state and unknown usage")
					}
				}
			})
			if !ok {
				sealed.Store(true)
			}
		}
	}
}
