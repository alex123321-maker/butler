package extraction

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/budget"
	pb "github.com/butler/butler/internal/gen/localmodels/v1"
	"github.com/butler/butler/internal/localmodels"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/jobs"
	"github.com/butler/butler/internal/memory/semantic"
	"google.golang.org/grpc/status"
)

type extractionCase struct {
	ID     string              `json:"id"`
	Split  string              `json:"split"`
	Text   string              `json:"text"`
	Fields map[string][]string `json:"fields"`
}
type corpus struct {
	Version   string           `json:"version"`
	Threshold float64          `json:"threshold"`
	Zone      string           `json:"timezone"`
	Event     string           `json:"event_time"`
	Cases     []extractionCase `json:"cases"`
}
type measurement struct {
	Case      string `json:"case"`
	Mode      string `json:"mode"`
	QueueMS   uint64 `json:"queue_ms"`
	ExecuteMS uint64 `json:"execution_ms"`
	PeakRAM   uint32 `json:"peak_ram_mib"`
	PeakGPU   uint32 `json:"peak_gpu_mib"`
}
type recordingBackend struct {
	Backend
	Measurements           []measurement
	Case, Mode, LastStatus string
	Attempts               int
}

func (b *recordingBackend) Infer(ctx context.Context, r *pb.InferRequest) (*pb.InferResponse, error) {
	b.Attempts++
	out, e := b.Backend.Infer(ctx, r)
	b.LastStatus = status.Code(e).String()
	if e == nil && out != nil {
		b.Measurements = append(b.Measurements, measurement{b.Case, b.Mode, out.QueueMs, out.ExecutionMs, out.PeakRamMib, out.PeakDeviceMib})
	}
	return out, e
}

type metric struct {
	TP        int     `json:"tp"`
	FP        int     `json:"fp"`
	FN        int     `json:"fn"`
	Precision float64 `json:"precision"`
	Recall    float64 `json:"recall"`
}

func (m *metric) finish() {
	if m.TP+m.FP > 0 {
		m.Precision = float64(m.TP) / float64(m.TP+m.FP)
	}
	if m.TP+m.FN > 0 {
		m.Recall = float64(m.TP) / float64(m.TP+m.FN)
	}
}
func expected(c extractionCase) map[string]bool {
	m := map[string]bool{}
	for name, values := range c.Fields {
		for _, v := range values {
			m[name+"\x00"+v] = true
		}
	}
	return m
}
func compare(want, got map[string]bool) metric {
	m := metric{}
	for key := range got {
		if want[key] {
			m.TP++
		} else {
			m.FP++
		}
	}
	for key := range want {
		if !got[key] {
			m.FN++
		}
	}
	m.finish()
	return m
}
func accumulate(a *metric, b metric) { a.TP += b.TP; a.FP += b.FP; a.FN += b.FN; a.finish() }
func modelFields(claims []catalog.Claim) (map[string]bool, int) {
	m := map[string]bool{}
	grouped := 0
	for _, c := range claims {
		var r Record
		_ = json.Unmarshal(c.Value, &r)
		if r.Kind == "constraint" {
			grouped++
		}
		for _, f := range r.Fields {
			m[f.Type+"\x00"+f.Text] = true
		}
	}
	return m, grouped
}

// Literal fragment evidence is checked separately from model precision/recall.
// It preserves the entire acceptance restriction even if the model omits its
// prohibition label. It never turns an exception cue into action permission.
func protectedAcceptance(claims []catalog.Claim) bool {
	for _, c := range claims {
		var r Record
		if json.Unmarshal(c.Value, &r) != nil || c.Status != "candidate" || !r.CandidateOnly || r.Kind != "constraint_fragment" || r.QuoteClipped || r.Quote != "До конца месяца не обновляй рабочий сервер, тестовый можно" || !r.Cues.Negated || !r.Cues.Exception || r.Cues.Scope != "quoted_fragment" {
			continue
		}
		fields := map[string]bool{}
		for _, f := range r.Fields {
			fields[f.Type+":"+f.Text] = f.Method == "model_span"
		}
		if !fields["action:обновляй"] || !fields["resource:рабочий сервер"] || !fields["deadline:До конца месяца"] {
			continue
		}
		found := map[string]bool{}
		for _, f := range r.Cues.Evidence {
			if string([]rune(r.Quote)[f.Start-r.QuoteStart:f.End-r.QuoteStart]) == f.Text {
				found[f.Type+":"+f.Text] = true
			}
		}
		if found["negation:не"] && found["exception:можно"] {
			return true
		}
	}
	return false
}

var rulePatterns = map[string]*regexp.Regexp{
	"action":   regexp.MustCompile(`(?i)(обновляй|обнови|перезапусти|перезапускай|удаляй|удалить|проверь|исправь|подготовь|выкати|оставь|запускай)`),
	"resource": regexp.MustCompile(`(?i)(рабочий сервер|рабочую базу данных|рабочей базе|тестовый сервер|резервные копии|диски лаборатории|server-[ab]|ClaimGrant)`),
	"deadline": regexp.MustCompile(`(?i)(до конца месяца|до завтра|завтра|через [1-9][0-9]? дня|[0-9]{4}-[0-9]{2}-[0-9]{2})`),
	"project":  regexp.MustCompile(`Butler`), "component": regexp.MustCompile(`Tool Broker|PostgreSQL`),
}

func ruleFields(text string) map[string]bool {
	m := map[string]bool{}
	for name, re := range rulePatterns {
		for _, v := range re.FindAllString(text, -1) {
			m[name+"\x00"+v] = true
		}
	}
	for _, p := range cuesPatterns {
		if p.label != "negation" {
			continue
		}
		for _, match := range p.re.FindAllStringSubmatch(text, -1) {
			m["prohibition\x00"+match[2]] = true
		}
	}
	return m
}

// Fixed public fixture only. Calls the same persistent internal service through
// actual owned-job admission, never a second tensor worker or cloud fallback.
func TestActualStructuredExtractionQuality(t *testing.T) {
	tokenFile := os.Getenv("BUTLER_TEST_LOCAL_MODELS_TOKEN_FILE")
	if tokenFile == "" {
		t.Skip("same persistent local service private token file required")
	}
	path := filepath.Join("..", "..", "..", "deploy", "local-models", "russian-extraction.json")
	raw, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	var suite corpus
	if json.Unmarshal(raw, &suite) != nil || suite.Threshold != .5 || len(suite.Cases) != 24 {
		t.Fatal("fixed public corpus invalid")
	}
	at, e := time.Parse(time.RFC3339, suite.Event)
	if e != nil {
		t.Fatal(e)
	}
	f := newFixture(t)
	target, e := budget.NewLocalTarget("local-models:9090", "pinned-local-service")
	if e != nil {
		t.Fatal(e)
	}
	client, closeClient, e := semantic.NewPinnedBackend(target, tokenFile)
	if e != nil {
		t.Fatal("private composed client unavailable")
	}
	defer closeClient()
	recording := &recordingBackend{Backend: client}
	service, e := New(f.catalog, f.jobs, recording, target, budget.ModelPrice{Basis: "local_no_billing"}, suite.Zone)
	if e != nil {
		t.Fatal(e)
	}
	ctx, stop := context.WithTimeout(context.Background(), 5*time.Minute)
	defer stop()
	type caseResult struct {
		ID              string `json:"id"`
		Split           string `json:"split"`
		Structured      metric `json:"structured"`
		Generic         metric `json:"current_generic_adapter"`
		Rules           metric `json:"rules"`
		Grouped         int    `json:"grouped_records"`
		NegationMiss    bool   `json:"negation_missing"`
		Cached          bool   `json:"repeat_cached"`
		StructuredError string `json:"structured_error,omitempty"`
		GenericError    string `json:"generic_error,omitempty"`
	}
	modelErrors, negationMissing := 0, 0
	acceptanceProtected := false
	var results []caseResult
	totals := map[string]map[string]*metric{"dev": {}, "test": {}}
	for _, m := range totals {
		for _, mode := range []string{"structured", "generic", "rules"} {
			m[mode] = &metric{}
		}
	}
	for _, c := range suite.Cases {
		src, e := f.catalog.Put(f.owner, catalog.PutSource{ExternalKey: "quality:" + c.ID, Kind: "document", ScopeType: "user", ScopeID: "owner", Author: "public synthetic fixture", Body: c.Text, EventTime: &at})
		if e != nil {
			t.Fatal(e)
		}
		ref := catalog.SourceRef{ID: src.ID, Revision: src.Revision, AccessRevision: src.AccessRevision, Hash: src.Hash}
		if _, e = f.jobs.Enqueue(f.owner, jobs.Request{ID: "extract-quality-" + c.ID, Kind: "extract", Purpose: "memory_extract", Sources: []catalog.SourceRef{ref}, Allowance: budget.Units{ModelTurns: 2, Tokens: 10000, ActiveMillis: 120000, ProviderInputBytes: 12000}, Deadline: time.Now().Add(time.Hour)}); e != nil {
			t.Fatal(e)
		}
		job, ok, e := f.jobs.Claim(ctx)
		if e != nil || !ok {
			t.Fatal(e)
		}
		recording.Case = c.ID
		recording.Mode = "structured"
		recording.LastStatus = ""
		claims, e := service.Run(ctx, job, ref)
		structuredError := ""
		cachedOK := false
		if e != nil {
			structuredError = recording.LastStatus
			if structuredError == "" || structuredError == "OK" {
				structuredError = "CallerRejected"
			}
			modelErrors++
			claims = nil
		} else {
			before := recording.Attempts
			cached, ce := service.Run(ctx, job, ref)
			cachedOK = ce == nil && len(cached) == len(claims) && before == recording.Attempts
			if !cachedOK {
				structuredError = "CacheRejected"
				modelErrors++
			}
		}
		if c.ID == "dev-constraint" {
			acceptanceProtected = protectedAcceptance(claims)
		}
		got, grouped := modelFields(claims)
		want := expected(c)
		sm := compare(want, got)
		op, e := budget.NewOperationID()
		if e != nil {
			t.Fatal(e)
		}
		recording.Mode = "current_generic_adapter"
		request := &pb.InferRequest{RequestId: op, CallerId: "quality:" + c.ID, ProfileId: Profile, TimeoutMs: 60000, CpuOnly: true, Input: &pb.InferRequest_Extract{Extract: &pb.ExtractInput{Text: c.Text, EntityTypes: localmodels.MemoryFields(), SchemaVersion: "generic-entity-baseline-v1"}}}
		bounded, release, e := f.jobs.Admit(ctx, job, jobs.Operation{ID: op, Kind: "extract", Endpoint: target.Endpoint(), ServiceIdentity: target.Identity(), InputBytes: int64(len(c.Text) + 512), Tokens: 1024, Timeout: time.Minute, Origin: src.Origin})
		if e != nil {
			t.Fatal(e)
		}
		out, e := recording.Infer(bounded, request)
		release()
		genericError := ""
		baseline := map[string]bool{}
		if e != nil || out == nil || out.GetExtract() == nil {
			genericError = recording.LastStatus
			if genericError == "" || genericError == "OK" {
				genericError = "CallerRejected"
			}
			modelErrors++
		} else {
			for _, field := range out.GetExtract().Spans {
				baseline[field.Type+"\x00"+field.Text] = true
			}
		}
		gm := compare(want, baseline)
		rm := compare(want, ruleFields(c.Text))
		negMissing := false
		for key := range want {
			if strings.HasPrefix(key, "prohibition\x00") && !got[key] {
				negMissing = true
			}
		}
		if negMissing {
			negationMissing++
		}
		results = append(results, caseResult{c.ID, c.Split, sm, gm, rm, grouped, negMissing, cachedOK, structuredError, genericError})
		accumulate(totals[c.Split]["structured"], sm)
		accumulate(totals[c.Split]["generic"], gm)
		accumulate(totals[c.Split]["rules"], rm)
		if e = f.jobs.Finish(ctx, job, "", false); e != nil {
			t.Fatal(e)
		}
	}
	sort.Slice(results, func(i, j int) bool { return results[i].ID < results[j].ID })
	sum := sha256.Sum256(raw)
	gate := modelErrors == 0 && negationMissing == 0 && totals["test"]["structured"].Precision >= .9 && totals["test"]["structured"].Recall >= .8
	report := map[string]any{"generated_at": time.Now().UTC(), "corpus": suite.Version, "corpus_sha256": hex.EncodeToString(sum[:]), "threshold": .5, "threshold_tuned_on_test": false, "model": Model, "revision": Revision, "runtime": Runtime, "schema": Schema, "normalizer": Normalizer, "mode": "candidate_only", "network": "persistent internal-only service", "held_out_counts": map[string]int{"dev": 6, "test": 18}, "totals": totals, "cases": results, "measurements": recording.Measurements, "actual_calls": recording.Attempts, "runtime_error_count": modelErrors, "negation_missing_cases": negationMissing, "candidate_suggestion_gate": map[string]any{"precision_min": .9, "recall_min": .8, "runtime_errors_max": 0, "negation_missing_max": 0, "passed": gate, "set_before_final_test": true}, "production_enabled": false, "cloud_baseline": "legacy cloud extractor disabled; no authorized unbudgeted comparison", "score_is_calibrated_probability": false, "owner_acceptance_required": true}
	report["acceptance_source_constraint_preserved"] = acceptanceProtected
	report["metrics_include_literal_rules"] = false
	reportRaw, e := json.MarshalIndent(report, "", "  ")
	if e != nil {
		t.Fatal(e)
	}
	reportPath := os.Getenv("BUTLER_TEST_EXTRACTION_REPORT")
	if reportPath == "" {
		reportPath = filepath.Join("..", "..", "..", "deploy", "local-models", "verification-structured-extraction.json")
	}
	if e = os.WriteFile(reportPath, append(reportRaw, '\n'), 0644); e != nil {
		t.Fatal(e)
	}
	t.Logf("actual CPU calls=%d test structured precision=%.3f recall=%.3f acceptanceProtected=%v", recording.Attempts, totals["test"]["structured"].Precision, totals["test"]["structured"].Recall, acceptanceProtected)
	if !acceptanceProtected {
		t.Error("acceptance restriction was not preserved as source-aware candidate")
	}
}
