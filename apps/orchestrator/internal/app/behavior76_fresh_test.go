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
	return behavior76HistoricalRowsEqualCount(ctx, tx, body, 174)
}

func behavior76HistoricalRowsEqualCount(ctx context.Context, tx pgx.Tx, body []byte, expected int) (bool, error) {
	lines := strings.Split(strings.TrimSpace(string(body)), "\n")
	array := "[" + strings.Join(lines, ",") + "]"
	if len(lines) != expected || !json.Valid([]byte(array)) {
		return false, fmt.Errorf("sealed complete attempt inventory unavailable")
	}
	var exact bool
	err := tx.QueryRow(ctx, `SELECT count(*)=$2 AND bool_and(j.value IS NOT NULL AND a.window_id IS NOT NULL AND j.value=to_jsonb(a)) FROM jsonb_array_elements($1::jsonb) AS j(value) FULL JOIN public.behavior76_attempts a ON a.window_id=j.value->>'window_id' AND a.n=(j.value->>'n')::integer`, array, expected).Scan(&exact)
	return exact, err
}

func TestBehavior76FreshHistoryBoundaryIntegration(t *testing.T) {
	if os.Getenv("BUTLER_TEST_MEMORY_POSTGRES_URL") == "" {
		t.Skip("isolated original local history required")
	}
	// This read-only audit hashes all retained windows on the Windows bind.
	// It grants no model/fixture authority; their original bounds are unchanged.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
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
	expected := 174
	historyPath := "/src20/.cache/behavior76/native-20261025c/behavior76_attempts.jsonl"
	if _, e := os.Stat("/src20/.cache/behavior76/native-fresh-v3/behavior76_attempts.jsonl"); e == nil {
		historyPath = "/src20/.cache/behavior76/native-fresh-v3/behavior76_attempts.jsonl"
		expected = 376
		if e = behavior76CheckRawFiles("/src20/.cache/behavior76/native-fresh-v3", "/src20/.cache/behavior76/native-fresh-v3-raw-sha256.json", "24f8595700ba3a496e8246ba7b7c062cf1183b9ba4f16e648b034672f2073105", 2465); e != nil {
			t.Fatal(e)
		}
	}
	if _, e := os.Stat("/src20/.cache/behavior76/native-local-v4-r2/behavior76_attempts.jsonl"); e == nil {
		historyPath = "/src20/.cache/behavior76/native-local-v4-r2/behavior76_attempts.jsonl"
		expected = 405
		if e = behavior76CheckRawFiles("/src20/.cache/behavior76/native-local-v4-r2", "/src20/experiments/behavior76/2026-10-25/native-local-v4-r2/raw-inventory.json", "fa0c4c82ff5b5fa2f850000bfadad4d5b89949ba391f8a527c0139f79110073e", 318); e != nil {
			t.Fatal(e)
		}
		manifest, e := os.ReadFile("/src20/.cache/behavior76/native-local-v4-r2/manifest.json")
		if e != nil || behavior76Hash(manifest) != "c308b02c887238d4203e434f3c8a69a33762ddfd524cf2f73c95d21f91872d37" {
			t.Fatal("sealed local-r2 manifest changed")
		}
		var closed bool
		if e = tx.QueryRow(ctx, `SELECT closed AND admitted=29 AND manifest=$1::jsonb FROM public.behavior76_windows WHERE id='issue76-fixture-local-20261025T214647'`, string(manifest)).Scan(&closed); e != nil || !closed {
			t.Fatal("sealed local-r2 window changed", e)
		}
	}
	if _, e := os.Stat("/src20/.cache/behavior76/native-stream-v5/behavior76_attempts.jsonl"); e == nil {
		if _, e = behavior76ClosedStreamHistory(ctx, tx, "/src20/.cache/behavior76"); e != nil {
			t.Fatal(e)
		}
		historyPath = "/src20/.cache/behavior76/native-stream-v5/behavior76_attempts.jsonl"
		expected = 506
	}
	if _, e := os.Stat("/src20/.cache/behavior76/native-day-v6/behavior76_attempts.jsonl"); e == nil {
		if _, e = behavior76ClosedDayHistory(ctx, tx, "/src20/.cache/behavior76"); e != nil {
			t.Fatal(e)
		}
		historyPath = "/src20/.cache/behavior76/native-day-v6/behavior76_attempts.jsonl"
		expected = 640
	}
	if _, e := os.Stat("/src20/.cache/behavior76/native-day-v7-r2/behavior76_attempts.jsonl"); e == nil {
		if _, e = behavior76ClosedEmptyHistory(ctx, tx, "/src20/.cache/behavior76"); e != nil {
			t.Fatal(e)
		}
		historyPath = "/src20/.cache/behavior76/native-day-v7-r2/behavior76_attempts.jsonl"
		expected = 643
	}
	if _, e := os.Stat("/src20/.cache/behavior76/native-local-model-v8/behavior76_attempts.jsonl"); e == nil {
		if _, e = behavior76ClosedModelHistory(ctx, tx, "/src20/.cache/behavior76"); e != nil {
			t.Fatal(e)
		}
		historyPath = "/src20/.cache/behavior76/native-local-model-v8/behavior76_attempts.jsonl"
		expected = 707
	}
	if _, e := os.Stat("/src20/.cache/behavior76/native-local-http-v9/behavior76_attempts.jsonl"); e == nil {
		if _, e = behavior76ClosedHTTPHistory(ctx, tx, "/src20/.cache/behavior76"); e != nil {
			t.Fatal(e)
		}
		historyPath = "/src20/.cache/behavior76/native-local-http-v9/behavior76_attempts.jsonl"
		expected = 723
	}
	if _, e := os.Stat("/src20/.cache/behavior76/native-local-response-v10-r2/behavior76_attempts.jsonl"); e == nil {
		if _, e = behavior76ClosedResponseHistory(ctx, tx, "/src20/.cache/behavior76"); e != nil {
			t.Fatal(e)
		}
		historyPath = "/src20/.cache/behavior76/native-local-response-v10-r2/behavior76_attempts.jsonl"
		expected = 774
	}
	if _, e := os.Stat("/src20/.cache/behavior76/native-local-response-v10-r3/behavior76_attempts.jsonl"); e == nil {
		if _, e = behavior76ClosedR3History(ctx, tx, "/src20/.cache/behavior76"); e != nil {
			t.Fatal(e)
		}
		historyPath = "/src20/.cache/behavior76/native-local-response-v10-r3/behavior76_attempts.jsonl"
		expected = 971
	}
	body, err := os.ReadFile(historyPath)
	if err != nil {
		t.Fatal("sealed history required")
	}
	if same, e := behavior76HistoricalRowsEqualCount(ctx, tx, body, expected); e != nil || !same {
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
	if same, e := behavior76HistoricalRowsEqualCount(ctx, tx, []byte(strings.Join(lines, "\n")), expected); e != nil || same {
		t.Fatal("changed row with unchanged count/token subtotal was accepted", e)
	}
	if same, e := behavior76HistoricalRowsEqualCount(ctx, tx, []byte(strings.Join(lines[1:], "\n")), expected); e == nil || same {
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
	behavior76CollectFreshConfigured(t, false)
}

func behavior76CollectFreshConfigured(t *testing.T, local bool) {
	behavior76CollectFreshMode(t, local, false)
}

func behavior76CollectFreshMode(t *testing.T, local, streaming bool) {
	behavior76CollectStudy(t, local, streaming, false)
}

func behavior76CollectStudy(t *testing.T, local, streaming, dayFit bool) {
	behavior76CollectStudyPlan(t, local, streaming, dayFit, false)
}

func behavior76CollectStudyPlan(t *testing.T, local, streaming, dayFit, literal bool) {
	if literal && (!local || !streaming || !dayFit || !behavior76LiteralBindingsFresh()) {
		t.Fatal("literal study requires the complete trusted profile and fresh bindings")
	}
	if dayFit && !streaming {
		t.Fatal("day-fit collection requires the unchanged stream protocol")
	}
	if streaming && !local {
		t.Fatal("streaming requires explicit fixture-local construction")
	}
	stimuliSHA, expectationsSHA := behavior76FreshStimuliSHA, behavior76FreshExpectationsSHA
	priorWindows, priorAdmissions, priorMeasured, priorTokens, priorUnknown := 3, 174, 172, 1274988, 2
	priorDirectory := "native-20261025c"
	version, prefix := "behavior76-fresh-independent-v3", "issue76-fresh-"
	if local {
		stimuliSHA, expectationsSHA = behavior76LocalStimuliSHA, behavior76LocalExpectationsSHA
		priorWindows, priorAdmissions, priorMeasured, priorTokens, priorUnknown = 4, 376, 372, 2867576, 4
		priorDirectory = "native-fresh-v3"
		version, prefix = "behavior76-fixture-local-v4", "issue76-fixture-local-"
		if streaming {
			stimuliSHA, expectationsSHA = behavior76StreamStimuliSHA, behavior76StreamExpectationsSHA
			priorWindows, priorAdmissions, priorMeasured, priorTokens, priorUnknown = 5, 405, 400, 3084472, 5
			priorDirectory = "native-local-v4-r2"
			version, prefix = "behavior76-fixture-stream-v5", "issue76-fixture-stream-"
			if dayFit {
				stimuliSHA, expectationsSHA = behavior76DayStimuliSHA, behavior76DayExpectationsSHA
				priorWindows, priorAdmissions, priorMeasured, priorTokens, priorUnknown = 11, 774, 762, 5948714, 12
				priorDirectory = "native-local-response-v10-r2"
				version, prefix = "behavior76-fixture-local-response-v10", "issue76-fixture-day-"
				if stimuliSHA == "078673c7da9df829f066faf1c3d1c3b21db716b43f74c4da8b80df2a3272b6a4" || expectationsSHA == "ba633aac4cb8bfb89e55bc16b4396296f0481f4c924fe3383612ac58a4ca9642" {
					t.Fatal("closed v10-r2 controls cannot be reused")
				}
				if stimuliSHA == "bb40fdc7ca4c22c15230364445d0c1e74384392cc9d55ab53ea8aa42a32e5686" || expectationsSHA == "fc202c9b27a025344156273c73ba2bb86c62d1ffed593f18c928668c8abd2c15" {
					t.Fatal("closed v9 controls cannot be reused")
				}
				if stimuliSHA == "ae86f66502969e359cebe7c814ed384be82e7685606dfae6044faa83bde18d00" || expectationsSHA == "2364bbe133f431d505fbf6a57de48764ec0cb3d12c6130d5bb389197f4124bf0" {
					t.Fatal("closed v8 controls cannot be reused")
				}
				if stimuliSHA == "a2d7ffa28a497a72246ba608d1a18351f316c6804a581784a1f76306975ba047" || expectationsSHA == "dabe70e665b96396bb1e3e9bfdbf7503ba2d07dead08482f6ce50d1d19708cc7" {
					t.Fatal("closed day-v7-r2 controls cannot be reused")
				}
				if stimuliSHA == "628eb9293577e9a37148367c8ae29ee8f77b2807d9adc5f92128f910fc7cd3ba" || expectationsSHA == "d6146f7c01ed7c120d1a0fdc7b435a9a52d07a8f15cda1823b20833e2014d0e9" {
					t.Fatal("closed day-v6 controls cannot be reused")
				}
				if stimuliSHA == behavior76StreamStimuliSHA || expectationsSHA == behavior76StreamExpectationsSHA {
					t.Fatal("closed stream controls cannot be reused")
				}
			}
			if stimuliSHA == behavior76LocalStimuliSHA || expectationsSHA == behavior76LocalExpectationsSHA {
				t.Fatal("closed local controls cannot be reused")
			}
		}
		if len(stimuliSHA) != 64 || len(expectationsSHA) != 64 {
			t.Fatal("new independent controls are not yet bound; no native admission")
		}
		if stimuliSHA == behavior76FreshStimuliSHA || stimuliSHA == behavior76StimuliSHA || expectationsSHA == behavior76FreshExpectationsSHA || expectationsSHA == behavior76ExpectationsSHA {
			t.Fatal("closed controls cannot become a new independent study")
		}
	}
	if literal {
		stimuliSHA, expectationsSHA = behavior76LiteralStimuliSHA, behavior76LiteralExpectationsSHA
		priorWindows, priorAdmissions, priorMeasured, priorTokens, priorUnknown = 12, 971, 959, 7700211, 12
		priorDirectory = "native-local-response-v10-r3"
		version, prefix = behavior76LiteralVersion, "issue76-fixture-literal-"
	}
	raw, err := os.ReadFile(os.Getenv("BUTLER_BEHAVIOR76_STIMULI"))
	if err != nil || behavior76Hash(raw) != stimuliSHA {
		t.Fatal("fresh independent stimulus commitment unavailable")
	}
	var episodes []behavior76Episode
	var literalEpisodes []behavior76LiteralEpisode
	if literal {
		literalEpisodes, err = behavior76ReadLiteralEpisodes(strings.NewReader(string(raw)))
		if err != nil {
			t.Fatal("complete literal independent interface unavailable")
		}
		for _, value := range literalEpisodes {
			episodes = append(episodes, value.Episode)
		}
	} else {
		episodes, err = behavior76ReadEpisodes(strings.NewReader(string(raw)))
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
	}
	dir, source := os.Getenv("BUTLER_BEHAVIOR76_OUTPUT"), os.Getenv("BUTLER_BEHAVIOR76_SOURCE")
	if dir == "" || source == "" {
		t.Fatal("explicit clean producer/output required")
	}
	var dayStart, dayDeadline, dayEnd time.Time
	if dayFit {
		dayStart = time.Now().UTC()
		dayDeadline, dayEnd, err = behavior76DayWindow(dayStart)
		if err != nil {
			t.Fatal("pre-entry owner-day admission rejected before output or native construction", err)
		}
	}
	if literal {
		if err = behavior76LiteralCertificateStart(dayStart, os.Getenv("BUTLER_BEHAVIOR76_LITERAL_NOT_BEFORE_UTC"), os.Getenv("BUTLER_BEHAVIOR76_LITERAL_LATEST_START_UTC")); err != nil {
			t.Fatal("actual literal window start outside finite certificate interval")
		}
		if err = behavior76LiteralBusinessDatesOutsideWindow(literalEpisodes, dayStart, dayDeadline); err != nil {
			t.Fatal("declared business dates overlap actual measured window")
		}
	}
	if err = os.Mkdir(dir, 0700); err != nil {
		t.Fatal("new fresh evidence directory required", err)
	}
	indexVersion := "behavior76-checker-index-v1"
	if local {
		indexVersion = "behavior76-checker-index-v2-local-tail"
	}
	if dayFit {
		indexVersion = "behavior76-checker-index-v5-local-response-tail"
	}
	if literal {
		indexVersion = behavior76LiteralIndexVersion
	}
	index := behavior76Index{Version: indexVersion, StimuliSHA256: stimuliSHA, Episodes: []behavior76IndexEpisode{}}
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
	if local {
		if err = behavior76CheckRawFiles(filepath.Join(root, "native-fresh-v3"), filepath.Join(root, "native-fresh-v3-raw-sha256.json"), "24f8595700ba3a496e8246ba7b7c062cf1183b9ba4f16e648b034672f2073105", 2465); err != nil {
			t.Fatal(err)
		}
	}
	if streaming {
		if err = behavior76CheckRawFiles(filepath.Join(root, "native-local-v4-r2"), "/src20/experiments/behavior76/2026-10-25/native-local-v4-r2/raw-inventory.json", "fa0c4c82ff5b5fa2f850000bfadad4d5b89949ba391f8a527c0139f79110073e", 318); err != nil {
			t.Fatal(err)
		}
	}
	started := time.Now().UTC()
	if dayFit {
		started = dayStart
	}
	deadline := started.Add(4 * time.Hour)
	if dayFit {
		deadline = dayDeadline
	}
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
	var localResources map[string]any
	var observerBuild map[string]any
	if local {
		observerBuild = behavior76PGXBuildMetadata(t)
		localResources, err = behavior76LocalResources(ctx, journal)
		if err != nil {
			t.Fatal("prospective resource observation failed before native window", behavior76SafeError(err))
		}
	}
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
	if local {
		body, e := os.ReadFile(filepath.Join(root, "native-fresh-v3", "manifest.json"))
		if e != nil || behavior76Hash(body) != "a5788ca6c47cc8c89fc930e6622136b1acb52444ac9d08b59b38dea50c69543a" {
			t.Fatal("closed fresh-v3 manifest changed")
		}
		var exact bool
		if e = transaction.QueryRow(ctx, `SELECT closed AND admitted=202 AND manifest=$1::jsonb AND (SELECT count(*)=202 FROM public.behavior76_attempts WHERE window_id='issue76-fresh-20261025T145320') FROM public.behavior76_windows WHERE id='issue76-fresh-20261025T145320'`, string(body)).Scan(&exact); e != nil || !exact {
			t.Fatal("closed fresh-v3 history changed", e)
		}
		prior = append(prior, map[string]any{"id": "issue76-fresh-20261025T145320", "admitted": 202, "closed": true, "manifest_sha256": behavior76Hash(body)})
	}
	if streaming {
		body, e := os.ReadFile(filepath.Join(root, "native-local-v4-r2", "manifest.json"))
		if e != nil || behavior76Hash(body) != "c308b02c887238d4203e434f3c8a69a33762ddfd524cf2f73c95d21f91872d37" {
			t.Fatal("closed local-r2 manifest changed")
		}
		var exact bool
		if e = transaction.QueryRow(ctx, `SELECT closed AND admitted=29 AND manifest=$1::jsonb AND (SELECT count(*)=29 FROM public.behavior76_attempts WHERE window_id='issue76-fixture-local-20261025T214647') FROM public.behavior76_windows WHERE id='issue76-fixture-local-20261025T214647'`, string(body)).Scan(&exact); e != nil || !exact {
			t.Fatal("closed local-r2 history changed", e)
		}
		prior = append(prior, map[string]any{"id": "issue76-fixture-local-20261025T214647", "admitted": 29, "closed": true, "manifest_sha256": behavior76Hash(body)})
	}
	var dayObserved time.Time
	if dayFit {
		body, e := behavior76ClosedStreamHistory(ctx, transaction, root)
		if e != nil {
			t.Fatal(e)
		}
		prior = append(prior, map[string]any{"id": "issue76-fixture-stream-20261025T231554", "admitted": 101, "closed": true, "manifest_sha256": behavior76Hash(body)})
		body, e = behavior76ClosedDayHistory(ctx, transaction, root)
		if e != nil {
			t.Fatal(e)
		}
		prior = append(prior, map[string]any{"id": "issue76-fixture-day-20261026T012812", "admitted": 134, "closed": true, "manifest_sha256": behavior76Hash(body)})
		body, e = behavior76ClosedEmptyHistory(ctx, transaction, root)
		if e != nil {
			t.Fatal(e)
		}
		prior = append(prior, map[string]any{"id": "issue76-fixture-day-20261026T050548", "admitted": 3, "closed": true, "manifest_sha256": behavior76Hash(body)})
		body, e = behavior76ClosedModelHistory(ctx, transaction, root)
		if e != nil {
			t.Fatal(e)
		}
		prior = append(prior, map[string]any{"id": "issue76-fixture-day-20261026T065844", "admitted": 64, "closed": true, "manifest_sha256": behavior76Hash(body)})
		body, e = behavior76ClosedHTTPHistory(ctx, transaction, root)
		if e != nil {
			t.Fatal(e)
		}
		prior = append(prior, map[string]any{"id": "issue76-fixture-day-20261026T092444", "admitted": 16, "closed": true, "manifest_sha256": behavior76Hash(body)})
		body, e = behavior76ClosedResponseHistory(ctx, transaction, root)
		if e != nil {
			t.Fatal(e)
		}
		prior = append(prior, map[string]any{"id": "issue76-fixture-day-20261026T181012", "admitted": 51, "closed": true, "manifest_sha256": behavior76Hash(body)})
		if literal {
			body, e = behavior76ClosedR3History(ctx, transaction, root)
			if e != nil {
				t.Fatal("closed r3 literal predecessor unavailable")
			}
			prior = append(prior, map[string]any{"id": "issue76-fixture-day-20261026T195902", "admitted": 197, "closed": true, "manifest_sha256": behavior76Hash(body)})
		}
		if e = transaction.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&dayObserved); e != nil {
			t.Fatal("owning database day unavailable")
		}
		actualEnd := dayObserved.UTC().Truncate(24 * time.Hour).Add(24 * time.Hour)
		if !dayEnd.Equal(actualEnd) || deadline.After(actualEnd) || !deadline.After(dayObserved) {
			t.Fatal("original whole window does not fit the actual owning database UTC day")
		}
	}
	var history bool
	if err = transaction.QueryRow(ctx, `SELECT (SELECT count(*)=$1 AND bool_and(closed) AND sum(admitted)=$2 FROM public.behavior76_windows) AND (SELECT count(*)=$2 AND count(total_tokens)=$3 AND sum(total_tokens)=$4 AND count(*) FILTER(WHERE total_tokens IS NULL)=$5 FROM public.behavior76_attempts)`, priorWindows, priorAdmissions, priorMeasured, priorTokens, priorUnknown).Scan(&history); err != nil || !history {
		t.Fatal("fresh-only admission requires exactly closed A/B/C and all spent/unknown usage")
	}
	oldAttempts, err := os.ReadFile(filepath.Join(root, priorDirectory, "behavior76_attempts.jsonl"))
	if err != nil {
		t.Fatal("sealed historical attempts unavailable")
	}
	if exactRows, e := behavior76HistoricalRowsEqualCount(ctx, transaction, oldAttempts, priorAdmissions); e != nil || !exactRows {
		t.Fatal("each historical attempt must exactly equal its sealed original JSONB row")
	}
	var entropy [33]byte
	if _, err = rand.Read(entropy[:]); err != nil {
		t.Fatal("fresh masking entropy unavailable")
	}
	arms := []string{"arm-" + hex.EncodeToString(entropy[:16]), "arm-" + hex.EncodeToString(entropy[16:32])}
	mapping := map[string]bool{arms[0]: entropy[32]&1 == 1, arms[1]: entropy[32]&1 == 0}
	if literal {
		index.LiteralPlan, err = behavior76LiteralDeclaredPlan(literalEpisodes, arms)
		if err != nil {
			t.Fatal("complete planned literal denominator unavailable")
		}
	}
	behavior76Write(t, filepath.Join(root, filepath.Base(dir)+"-private-arm-mapping.json"), mapping)
	schedule := []map[string]any{}
	for n, e := range episodes {
		schedule = append(schedule, map[string]any{"episode_id": e.ID, "arm_order": []string{arms[n%2], arms[(n+1)%2]}})
	}
	window := prefix + started.Format("20060102T150405")
	manifest := map[string]any{"version": version, "window": window, "source": source, "stimuli_sha256": stimuliSHA, "expectations_sha256": expectationsSHA, "mapping_sha256": behavior76Hash([]byte(behavior76JSON(mapping))), "schedule": schedule, "max_native_entries": behavior76FreshMaxEntries, "max_per_turn": 8, "max_per_run": 4, "wall_millis": (4 * time.Hour).Milliseconds(), "started_at": started.Format(time.RFC3339Nano), "absolute_deadline": deadline.Format(time.RFC3339Nano), "native": true, "model": antigravity.FlashHighModel, "baseline_prompt": prompt.DefaultBasePrompt, "candidate_addition": prompt.TaskContinuityProtocol, "output_contract": behavior76OutputContract, "prior_history": prior, "prior_admissions": priorAdmissions, "prior_measured": priorMeasured, "prior_known_tokens": priorTokens, "prior_unavailable_usage": priorUnknown, "cumulative_admission_ceiling": priorAdmissions + behavior76FreshMaxEntries, "separate_new_experiment": true, "prior_experiment_complete": false, "global_stop_unchanged": !local}
	if local {
		manifest["local_unknown_mode"] = "whole_fixture_after_exact_classified_effect_unknown"
		manifest["local_resource_profile"] = localResources
		manifest["observer_build"] = observerBuild
		protocolPath := "docs/testing/issue76-fixture-local-protocol.md"
		if streaming {
			protocolPath = "docs/testing/issue76-fixture-stream-protocol.md"
		}
		if dayFit {
			protocolPath = "docs/testing/issue76-local-response-protocol.md"
			manifest["local_model_failure_mode"] = behavior76LocalModelMode
			manifest["local_http_failure_mode"] = behavior76LocalHTTPMode
			manifest["local_response_failure_mode"] = behavior76LocalResponseMode
		}
		protocol, e := os.ReadFile(filepath.Join("/src20", protocolPath))
		if e != nil {
			t.Fatal("prospective public protocol unavailable")
		}
		manifest["protocol_sha256"] = behavior76Hash(protocol)
		manifest["global_priority"] = true
		manifest["planned_arms"] = 48
		manifest["planned_owner_opportunities"] = 96
		manifest["planned_families"] = 12
		if literal {
			manifest["planned_arms"], manifest["planned_owner_opportunities"], manifest["planned_families"], manifest["planned_dimensions"] = 32, 68, 6, 8
			manifest["literal_plan"] = index.LiteralPlan
			manifest["literal_observation_contract"] = "observed-business-date-and-manual-events-v1"
			manifest["business_dates_outside_entire_window"] = true
		}
		manifest["protocol"] = protocolPath
		if streaming {
			manifest["authority_observer"] = map[string]any{"version": "behavior76-authority-pg-trace-stream-v2", "maximum_buffered_events": behavior76StreamBuffer, "maximum_segments": behavior76StreamSegments, "maximum_compressed_bytes": behavior76StreamBytes, "maximum_header_bytes": behavior76StreamHeader, "maximum_line_bytes": behavior76StreamLine, "maximum_open_spans": behavior76StreamOpen}
		}
	}
	if dayFit {
		policyPath := "docs/testing/issue76-day-window-admission.md"
		policy, e := os.ReadFile(filepath.Join("/src20", policyPath))
		if e != nil {
			t.Fatal("public day admission policy unavailable")
		}
		manifest["owner_day_admission"] = map[string]any{"policy": policyPath, "policy_sha256": behavior76Hash(policy), "database_observed_at": dayObserved.UTC(), "original_window_started_at": started, "original_window_deadline": deadline, "owning_day_ends_at": dayEnd, "whole_original_window_fits": true, "scope_deadlines_extended": false}
	}
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
				var f *behavior76Fixture
				if literal {
					f = newBehavior76LiteralFixture(t, ctx, &sealed, behavior76ScopeBinding{Window: window, Arm: fmt.Sprintf("slot-%02d/%s", n+1, arm), ProtocolSHA256: manifest["protocol_sha256"].(string), Producer: source})
				} else if dayFit {
					f = newBehavior76ResponseFixture(t, ctx, &sealed, behavior76ScopeBinding{Window: window, Arm: fmt.Sprintf("slot-%02d/%s", n+1, arm), ProtocolSHA256: manifest["protocol_sha256"].(string), Producer: source})
				} else if streaming {
					f = newBehavior76StreamFixture(t, ctx, &sealed, behavior76ScopeBinding{Window: window, Arm: fmt.Sprintf("slot-%02d/%s", n+1, arm), ProtocolSHA256: manifest["protocol_sha256"].(string), Producer: source})
				} else if local {
					f = newBehavior76LocalFixture(t, ctx, &sealed, behavior76ScopeBinding{Window: window, Arm: fmt.Sprintf("slot-%02d/%s", n+1, arm), ProtocolSHA256: manifest["protocol_sha256"].(string), Producer: source})
				} else {
					f = newBehavior76Fixture(t, ctx)
				}
				if !streaming {
					f.seed(e)
				}
				relative := fmt.Sprintf("slot-%02d/%s", n+1, arm)
				caseDir := filepath.Join(dir, filepath.FromSlash(relative))
				if err = os.MkdirAll(caseDir, 0700); err != nil {
					t.Fatal(err)
				}
				if streaming {
					if err = f.authorityTrace.enableStream(caseDir, f.scope.binding, &sealed); err != nil {
						sealed.Store(true)
						t.Fatal("trusted stream startup failed", err)
					}
					if literal {
						f.seedLiteral(literalEpisodes[n])
					} else {
						f.seed(e)
					}
				}
				item := behavior76IndexEpisode{EpisodeID: e.ID, MaskedArmID: arm, AliasesPath: relative + "/aliases.json", TablesDir: relative + "/tables", Steps: []behavior76IndexStep{}}
				if literal {
					item.Dimension, item.Polarity = literalEpisodes[n].Dimension, literalEpisodes[n].Polarity
				}
				defer func() {
					defer f.retainOnEvidenceFailure()
					if local {
						cleanup, stop := context.WithTimeout(context.Background(), 30*time.Second)
						receipt, e := f.quiesce(cleanup)
						stop()
						if e != nil {
							sealed.Store(true)
							t.Error("fixture quiescence failed", behavior76SafeError(e))
						}
						f.streamQuiescent = e == nil
						behavior76Write(t, filepath.Join(caseDir, "quiescence.json"), receipt)
					}
					if !f.backupPrivate(fmt.Sprintf("%s-slot-%02d-%s", window, n+1, arm)) {
						sealed.Store(true)
					}
					f.export(filepath.Join(caseDir, "tables"))
					behavior76Write(t, filepath.Join(caseDir, "aliases.json"), f.aliases())
					if local {
						f.sealFixtureEvidence(caseDir)
					}
					index.Episodes = append(index.Episodes, item)
				}()
				for j, step := range e.Steps {
					if filepath.Base(step.ID) != step.ID || strings.Contains(step.ID, "..") {
						t.Fatal("invalid fresh step path")
					}
					stepDir := filepath.Join(caseDir, fmt.Sprintf("step-%02d", j+1))
					stepRel := relative + fmt.Sprintf("/step-%02d", j+1)
					if local && f.scope.localStopped() {
						outcome := behavior76Outcome{State: f.scope.disposition(true), RunIDs: []string{}, AutomaticRunIDs: []string{}, NativeEntryIDs: []int{}, FailurePhase: "fixture_local_stop", FixtureStopRef: relative + "/tables/behavior76_fixture_stops.jsonl"}
						behavior76Write(t, filepath.Join(stepDir, "outcome.json"), outcome)
						item.Steps = append(item.Steps, behavior76IndexStep{StepID: step.ID, Outcome: stepRel + "/outcome.json"})
						continue
					}
					behavior76Write(t, filepath.Join(stepDir, "before.json"), f.snapshot())
					var receipt any
					var eventErr error
					if literal {
						changes, e := f.applyLiteralServiceChanges(literalEpisodes[n], step.ID)
						eventErr = e
						if eventErr == nil {
							receipt, eventErr = f.apply(step)
						}
						receipt = map[string]any{"event": receipt, "business_service_changes": changes}
					} else {
						receipt, eventErr = f.apply(step)
					}
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
						if literal {
							var selections []behavior76LiteralTaskSelection
							for _, selection := range literalEpisodes[n].OwnerTaskSelections {
								if selection.StepID == step.ID {
									selections = append(selections, selection)
								}
							}
							outcome, eventErr = f.evaluateLiteral(provider, step, selections...)
						} else {
							outcome, eventErr = f.evaluate(provider, step)
						}
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
					if local && f.scope.localStopped() {
						outcome.FixtureStopRef = relative + "/tables/behavior76_fixture_stops.jsonl"
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
	plannedArms := 48
	if literal {
		plannedArms = 32
	}
	if streaming && (sealed.Load() || ctx.Err() != nil || len(index.Episodes) != plannedArms) {
		t.Fatal("stream window closeout failed; original partial evidence and all planned denominators retained")
	}
}
