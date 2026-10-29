package app

// Complete prospective denominator through actual Core/Broker. Synthetic setup
// is explicitly costed separately; the oracle is never passed to a provider.
import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/acceptance"
	"github.com/butler/butler/internal/acceptance/utilitytrace"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/memory/readcontrol"
	"github.com/butler/butler/internal/security/egress/egresstest"
	"github.com/butler/butler/internal/skills"
	"github.com/butler/butler/internal/transport"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type utilityWholeSlot struct {
	Sequence, Case int
	Arm, Status    string
}

func utilityWholeWrite(t *testing.T, root, name string, v any) {
	t.Helper()
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		t.Fatal(e)
	}
	f, e := os.OpenFile(filepath.Join(root, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	if _, e = f.Write(b); e != nil {
		t.Fatal(e)
	}
	if e = f.Sync(); e != nil {
		t.Fatal(e)
	}
}

func TestMemoryUtilityWholeControlledABCIntegration(t *testing.T) {
	if os.Getenv("BUTLER_UTILITY_WHOLE_OFFLINE") != "1" {
		t.Skip("explicit isolated full prospective corpus required")
	}
	root := os.Getenv("BUTLER_UTILITY_WHOLE_OUTPUT")
	if !filepath.IsAbs(root) {
		t.Fatal("exclusive private output required")
	}
	if e := os.Mkdir(root, 0700); e != nil {
		t.Fatal(e)
	}
	suite, e := acceptance.NewMemoryUtilitySuite()
	if e != nil {
		t.Fatal(e)
	}
	slots := []utilityWholeSlot{}
	for i := range suite.Cases {
		for j := 0; j < 3; j++ {
			slots = append(slots, utilityWholeSlot{len(slots) + 1, i, []string{"A", "B", "C"}[(i+j)%3], "planned"})
		}
	}
	utilityWholeWrite(t, root, "suite.private.json", suite)
	utilityWholeWrite(t, root, "plan.json", map[string]any{"suite_sha256": acceptance.MemoryDigest(suite), "slots": slots, "native": false, "preparation": "scripted actual source and reviewed skill fixtures; not model-generated notes", "order": "balanced Latin order over all24 frozen cases", "historical_spend": "unchanged; synthetic usage excluded from native ledger"})
	defer func() { utilityWholeWrite(t, root, "slots-final.json", slots) }()
	for i := range slots {
		slot := &slots[i]
		ok := t.Run(fmt.Sprintf("%03d_%s", slot.Sequence, slot.Arm), func(t *testing.T) { slot.Status = "started"; utilityWholeCase(t, root, suite, *slot, nil) })
		if ok {
			slot.Status = "completed"
		} else {
			slot.Status = "failed"
		}
	}
}

func utilityWholeCase(t *testing.T, root string, suite acceptance.UtilitySuite, slot utilityWholeSlot, native *utilityWholeNative) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	coldStarted := time.Now()
	pool := memorytest.Database(t)
	dir := t.TempDir()
	anchor := filepath.Join(dir, "anchor")
	manifest := catalog.NewFileManifest(anchor)
	if manifest.Initialize() != nil {
		t.Fatal("anchor")
	}
	cat := catalog.New(pool, manifest)
	dst := filepath.Join(root, fmt.Sprintf("slot-%03d", slot.Sequence))
	if os.Mkdir(dst, 0700) != nil {
		t.Fatal("exclusive slot")
	}
	c := suite.Cases[slot.Case]
	owner := experienceNativeActor(ctx, "utility-whole-preparation")
	sources := []utilitytrace.SourceBinding{}
	subset := []catalog.SourceRef{}
	docs := []utilitytrace.CurrentDocument{}
	writes := 0
	prepStarted := coldStarted
	for _, s := range c.Sources {
		if s.Channel == "current" {
			pin := catalog.SourceRef{ID: s.Source.ID, Revision: 1, AccessRevision: 1, Hash: experienceDigest(s.Source.Body)}
			docs = append(docs, utilitytrace.CurrentDocument{Ref: pin, Body: s.Source.Body})
			sources = append(sources, utilitytrace.SourceBinding{UtilityID: s.Source.ID, Pin: pin, CurrentDocument: true})
			continue
		}
		if slot.Arm == "A" {
			continue
		}
		original := s
		for _, p := range c.Preparation {
			if p.Source.ID == s.Source.ID {
				original = p
			}
		}
		stored, e := cat.Put(owner, catalog.PutSource{ExternalKey: s.Source.ID, Kind: "document", ScopeType: "project", ScopeID: "utility-prior-" + s.Resource, Title: c.Question, Body: original.Source.Body})
		if e != nil {
			t.Fatal(e)
		}
		writes++
		pin := screenSourceRef(stored)
		if s.Source.Status == "superseded" {
			stored, e = cat.Put(owner, catalog.PutSource{ExternalKey: s.Source.ID, Kind: "document", ScopeType: "project", ScopeID: "utility-prior-" + s.Resource, ExpectedRevision: stored.Revision, Title: c.Question, Body: "The previous observation is superseded; current fact is unavailable."})
			if e != nil {
				t.Fatal(e)
			}
			writes++
		}
		if s.Source.Status == "deleted" {
			if _, e = cat.Delete(owner, stored.ID, "revoke", stored.Revision); e != nil {
				t.Fatal(e)
			}
			writes++
		}
		if c.SourceAllowed(s, slot.Arm) {
			subset = append(subset, pin)
			sources = append(sources, utilitytrace.SourceBinding{UtilityID: s.Source.ID, Pin: pin})
		}
	}
	config := experienceRecipientConfig{Version: "v1", Ready: "true", Files: map[string]string{"alpha": "Utility current project\nready=false\n", "beta": "Other project\nready=false\n"}, Documents: docs}
	recipient, e := newExperienceRecipient(dir, config)
	if e != nil {
		t.Fatal(e)
	}
	defer recipient.close()
	arm := experienceNativeArms[strings.Index("ABC", slot.Arm)]
	r := newExperienceNativeRuntime(t, pool, catalog.New(pool, catalog.NewFileManifest(anchor)), anchor, dir, recipient, newExperienceNativeAddresses(t), experienceNativeSourcePolicy(), utilityWholeInstructions, arm, subset, time.Now().Add(3*time.Minute))
	defer r.close()
	var reviewed *skills.Record
	var unknown utilitytrace.UnknownBinding
	if slot.Arm == "C" {
		reviewed = utilityWholeSkill(t, ctx, r, recipient, dst)
	}
	if c.Condition == "unknown_effect" {
		original := r.createTask(t, ctx, "unknown-original", "Attempt exactly ready=maybe once; preserve uncertainty", "alpha", "prepare_and_verify_exact_resource")
		r.acceptCurrentProfile(t, ctx, original)
		out := r.execute(t, ctx, original, "unknown-original", "Inspect alpha and request ready=maybe without applying it", &experienceOriginalControlProvider{ready: "maybe"})
		if out.Err != nil || out.Result == nil || out.Result.ErrorCode != "effect_unknown" {
			t.Fatal("actual original unknown required", out.Err)
		}
		if e := pool.QueryRow(ctx, `SELECT action_id,idempotency_key,fingerprint FROM action_effects WHERE initial_binding->>'run_id'=$1 AND state='effect_unknown'`, out.Result.RunID).Scan(&unknown.EffectID, &unknown.Key, &unknown.Fingerprint); e != nil {
			t.Fatal(e)
		}
		unknown.OriginRun, unknown.OriginTask = out.Result.RunID, original.TaskID
		recipient.mu.Lock()
		recipient.state.Config.Outage = true
		recipient.state.Config.UnknownKey = unknown.Key
		recipient.mu.Unlock()
	}
	permissions := "inspect_and_check_only"
	if c.Condition == "prepared_artifact" {
		permissions = "prepare_and_verify_exact_resource"
	}
	id := fmt.Sprintf("utility-%03d", slot.Sequence)
	record := r.createTask(t, ctx, id, c.Question, "alpha", permissions)
	r.acceptCurrentProfile(t, ctx, record)
	grant := func(roots []catalog.SourceRef) {
		tx, e := pool.Begin(owner)
		if e != nil {
			t.Fatal(e)
		}
		refs, e := cat.CanonicalSourcesTx(owner, tx, roots)
		tx.Rollback(owner)
		if e != nil {
			t.Fatal(e)
		}
		for _, ref := range refs {
			if e = cat.GrantSource(owner, record.TaskID, ref.ID, "memory_read", record.Revision); e != nil {
				t.Fatal(e)
			}
		}
	}
	for _, s := range sources {
		if !s.CurrentDocument {
			grant([]catalog.SourceRef{s.Pin})
		}
	}
	binding := utilitytrace.Binding{SuiteSHA256: acceptance.MemoryDigest(suite), CaseID: c.ID, Arm: slot.Arm, TaskID: record.TaskID, SessionKey: record.SessionKey, OwnerID: record.OwnerID, TaskRevision: record.Revision, ConstraintsRevision: record.ConstraintsRevision, OriginalFilesSHA256: acceptance.MemoryDigest(recipient.snapshot().Config.Files), Sources: sources, Unknown: unknown}
	if reviewed != nil {
		grant([]catalog.SourceRef{reviewed.BodySource})
		binding.Skills = []utilitytrace.SkillBinding{{Digest: reviewed.Digest, Body: reviewed.BodySource}}
	}
	utilityWholeWrite(t, dst, "binding.json", binding)
	utilityWholePreparationSnapshot(t, ctx, pool, dst)
	utilityWholeWrite(t, dst, "preparation.json", map[string]any{"source_writes": writes, "elapsed_ms": time.Since(prepStarted).Milliseconds(), "native_requests": 0, "provider": "scripted synthetic fixture; no native usage allocation", "skill": reviewed, "original_unknown": unknown})
	input := c.Input()
	raw, _ := json.Marshal(input)
	controlled := &utilityWholeControlledProvider{input: input, arm: slot.Arm, unknown: unknown.Key}
	if reviewed != nil {
		controlled.digest = reviewed.Digest
	}
	var provider transport.ModelProvider = controlled
	if native != nil {
		provider = native.provider(t, slot.Sequence, dst)
	}
	started := time.Now()
	out := r.execute(t, ctx, record, id, string(raw), provider)
	var run string
	if out.Result != nil {
		run = out.Result.RunID
	} else {
		if e = pool.QueryRow(ctx, `SELECT run_id FROM accepted_inputs WHERE task_id=$1`, record.TaskID).Scan(&run); e != nil {
			t.Fatal(e)
		}
	}
	snap, e := utilitytrace.Capture(ctx, pool, cat, binding, run, recipient.snapshot().Config.Files)
	if e != nil {
		t.Fatal(e)
	}
	utilityWholeWrite(t, dst, "snapshot.private.json", snap)
	obs, projection := utilitytrace.Project(suite, binding, snap, acceptance.MemoryDigest(snap))
	score := acceptance.EvaluateMemoryUtility(suite, obs)
	utilityWholeWrite(t, dst, "observation.json", obs)
	utilityWholeWrite(t, dst, "score.json", score)
	utilityWholeWrite(t, dst, "execution.json", map[string]any{"elapsed_ms": time.Since(started).Milliseconds(), "projection_error": fmt.Sprint(projection), "run_error": fmt.Sprint(out.Err), "native_requests": 0})
	if projection != nil {
		t.Fatal(projection)
	}
	if out.Err != nil {
		t.Fatal(out.Err)
	}
	// The complete denominator keeps honest A abstentions as task failures.
	expected := true
	if slot.Arm == "A" && slot.Case < 20 && c.Family != "procedure" && c.Condition != "current_sufficient" && c.Expected.Outcome != "unknown" {
		expected = false
	}
	if native == nil && score.TaskSuccess != expected {
		t.Fatalf("whole observer score %+v expected %t", score, expected)
	}
}

func utilityWholePreparationSnapshot(t *testing.T, ctx context.Context, pool *pgxpool.Pool, dst string) {
	t.Helper()
	tx, e := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	tables := []string{"memory_sources", "memory_source_revisions", "memory_deletions", "memory_scope_grants", "skill_versions", "skill_states", "skill_catalog", "skill_validations", "skill_usages", "tasks", "runs", "accepted_inputs", "tool_calls", "action_intents", "action_effects", "action_effect_evidence", "action_effect_dispatches", "run_final_results", "execution_model_attempts"}
	dump := map[string]json.RawMessage{}
	for _, table := range tables {
		var raw json.RawMessage
		if e = tx.QueryRow(ctx, `SELECT COALESCE(jsonb_agg(to_jsonb(t)),'[]'::jsonb) FROM `+pgx.Identifier{table}.Sanitize()+` t`).Scan(&raw); e != nil {
			t.Fatal(e)
		}
		if len(raw) > 32<<20 {
			t.Fatal("bounded preparation evidence exceeded")
		}
		dump[table] = raw
	}
	utilityWholeWrite(t, dst, "preparation-snapshot.private.json", dump)
}

func utilityWholeSkill(t *testing.T, ctx context.Context, r *experienceNativeRuntime, recipient *experienceRecipient, dst string) *skills.Record {
	original := recipient.snapshot()
	task := r.createTask(t, ctx, "skill-baseline", "Prepare and verify a current unapplied artifact using current inspection", "alpha", "prepare_and_verify_exact_resource")
	r.acceptCurrentProfile(t, ctx, task)
	out := r.execute(t, ctx, task, "skill-baseline", "Inspect alpha and prepare and check the current ready=true artifact", &experienceOriginalControlProvider{})
	if out.Err != nil || out.Result == nil {
		t.Fatal("skill baseline", out.Err)
	}
	verified, e := r.verifyPreparedOriginal(ctx, task, out.Result.RunID, "alpha", recipient)
	if e != nil {
		t.Fatal(e)
	}
	verifier := &skillEvidence{tasks: r.tasks, journal: r.journal, transcript: r.history, budgets: budget.NewLedger(r.pool), results: r.results, sources: r.cat}
	refs := []skills.EvidenceRef{{TaskID: task.TaskID, TaskRevision: task.Revision, ResultID: verified.ResultID}}
	proofs, e := verifier.VerifySkillSources(ctx, experienceNativeOwner, refs)
	if e != nil || len(proofs) != 1 {
		t.Fatal("skill proof", e)
	}
	m := skills.Manifest{Format: skills.Format, ID: "current-project-procedure", Version: "1.0.0", Source: skills.Source{URI: "https://butler.invalid/utility/current-project", Revision: "1"}, Description: "Read current alpha project documents, derive the route/codec/guard procedure or prepare and verify a current unapplied readiness artifact", When: []string{"Current alpha document/procedure or explicitly requested artifact task"}, Inputs: []string{"Current task and current resource"}, NotApplicable: []string{"Historical fact unavailable; stale or revoked knowledge; unknown effect; no mutation authority"}, Dependencies: proofs[0].ConfirmedContracts, Sources: refs, Tests: []skills.Scenario{{Name: "success", Kind: "verified_success", Expected: "Current recipient check"}, {Name: "failure", Kind: "failure_stop", Expected: "Stop without claimed success"}, {Name: "changed", Kind: "interface_change", Expected: "Reject changed dependency"}}, Steps: []skills.Step{}}
	deps := []skills.Dependency{}
	for _, d := range m.Dependencies {
		if d.Tool == "extension.project.inspect" {
			deps = append(deps, d)
		}
	}
	m.Dependencies = deps
	for i, d := range m.Dependencies {
		m.Steps = append(m.Steps, skills.Step{ID: fmt.Sprintf("step-%d", i), Tool: d.Tool, Intent: "Inspect current documents; combine route, codec and guard if requested. Prepare/check only when explicitly requested; no application.", SuccessCheck: "Actual current tool evidence", OnError: "stop", Retry: "never", ApprovalCheckpoint: true})
	}
	raw, _ := json.Marshal(m)
	pack, e := skills.Parse(raw)
	if e != nil {
		t.Fatal(e)
	}
	control := func(q skills.ControlRequest) skills.ControlResponse {
		v, e := r.skillClient.Control(experienceNativeActor(ctx, "utility-skill-"+q.Operation), q)
		if e != nil || v.Error != "" {
			t.Fatal("skill lifecycle", q.Operation, e, v.Error)
		}
		return v
	}
	staged := control(skills.ControlRequest{Operation: "stage", ManifestText: string(raw)})
	tested := control(skills.ControlRequest{Operation: "test", Digest: pack.Digest})
	if tested.Validation == nil || !tested.Validation.Passed {
		t.Fatal("skill validation")
	}
	active := control(skills.ControlRequest{Operation: "activate", Digest: pack.Digest, Generation: staged.Item.Generation})
	utilityWholeWrite(t, dst, "skill-preparation.private.json", map[string]any{"baseline_run": out.Result.RunID, "verified_result": verified, "manifest": m, "staged": staged, "tested": tested, "active": active, "native_requests": 0})
	if e = recipient.persist(original); e != nil {
		t.Fatal(e)
	}
	return active.Item
}

type utilityWholeControlledProvider struct {
	lastTool string
	utilityTraceProvider
	input                acceptance.UtilityInput
	arm, digest, unknown string
	opening              bool
}

func (p *utilityWholeControlledProvider) StartRun(ctx context.Context, q transport.StartRunRequest) (transport.EventStream, error) {
	if transport.CheckInferenceAuthority(ctx) != nil || transport.CheckInferenceDispatch(ctx) != nil {
		return nil, fmt.Errorf("owning admission required")
	}
	p.run = q.Context.RunID
	if strings.Contains(p.input.Question, "неизвестный эффект") {
		return p.emit("extension.project.inspect", map[string]string{"url": egresstest.Origin + "/project/alpha?verify_unknown=" + p.unknown}), nil
	}
	if strings.Contains(p.input.Question, "ready=maybe") {
		p.mode = "negative"
		return p.emit("extension.project.inspect", map[string]string{"url": egresstest.Origin + "/project/alpha?validate_ready=maybe"}), nil
	}
	if strings.Contains(p.input.Question, "артефакт ready") {
		p.mode = "prepared"
		return p.utilityTraceProvider.StartRun(ctx, q)
	}
	f := acceptance.UtilityFinal{Outcome: "answered", Sources: []acceptance.UtilityRef{}, Effect: "none", Check: "not_checked"}
	if p.input.Current != "" {
		f.Value = strings.TrimPrefix(p.input.Current, "Владелец сейчас явно задаёт искомое значение для этого поручения: ")
		return p.final(f), nil
	}
	if strings.Contains(p.input.Question, "Только проверь") {
		p.mode = "inspect"
		return p.emit("extension.project.inspect", map[string]string{"url": egresstest.Origin + "/project/alpha"}), nil
	}
	if p.digest != "" && len(p.input.CurrentDocumentIDs) > 0 {
		return p.emit("skills.search", map[string]string{"query": "current"}), nil
	}
	if len(p.input.CurrentDocumentIDs) > 0 {
		return p.emit("extension.project.inspect", map[string]string{"url": egresstest.Origin + "/project/alpha"}), nil
	}
	if p.arm != "A" {
		return p.emit(readcontrol.Search, map[string]any{"query": p.input.Question, "limit": 5}), nil
	}
	f.Outcome = "unknown"
	return p.final(f), nil
}
func (p *utilityWholeControlledProvider) SubmitToolResult(ctx context.Context, q transport.SubmitToolResultRequest) (transport.EventStream, error) {
	if p.mode != "" {
		return p.utilityTraceProvider.SubmitToolResult(ctx, q)
	}
	var env struct {
		Status string          `json:"status"`
		Result json.RawMessage `json:"result"`
	}
	if json.Unmarshal([]byte(q.ToolResultJSON), &env) != nil || env.Status != "completed" {
		return nil, fmt.Errorf("actual tool failed")
	}
	f := acceptance.UtilityFinal{Outcome: "answered", Sources: []acceptance.UtilityRef{}, Effect: "none", Check: "not_checked"}
	if p.lastTool == "skills.search" {
		return p.emit("skills.load", map[string]any{"digest": p.digest, "max_bytes": 16384}), nil
	}
	if p.lastTool == "skills.load" {
		return p.emit("extension.project.inspect", map[string]string{"url": egresstest.Origin + "/project/alpha"}), nil
	}
	if strings.HasPrefix(p.lastTool, "memory.") {
		data, e := readcontrol.DecodeResult(env.Result)
		if e != nil {
			return nil, e
		}
		if len(data.Items) == 0 {
			f.Outcome = "unknown"
			return p.final(f), nil
		}
		item := data.Items[0]
		for _, v := range data.Items {
			if !strings.Contains(v.Text, "Справочная запись") {
				item = v
				break
			}
		}
		if !p.opening {
			p.opening = true
			return p.emit(readcontrol.Open, map[string]any{"source_id": item.Source.ID, "source_revision": item.Source.Revision, "access_revision": item.Source.AccessRevision, "content_hash": item.Source.Hash, "max_chars": 8192}), nil
		}
		text := item.Text
		words := strings.Fields(text)
		f.Value = strings.TrimSuffix(words[len(words)-1], ".")
		if strings.Contains(text, "checkpoint:") {
			a := strings.Split(text, " -> ")
			if len(a) != 3 {
				return nil, fmt.Errorf("checkpoint format")
			}
			f.Value = a[1]
		}
		f.Sources = []acceptance.UtilityRef{{ID: item.Source.ID, Revision: item.Source.Revision, SHA256: item.Source.Hash}}
		return p.final(f), nil
	}
	var wrapper struct {
		Body string `json:"body"`
	}
	_ = json.Unmarshal(env.Result, &wrapper)
	var body struct {
		Documents []utilitytrace.CurrentDocument `json:"current_documents"`
		Effect    string                         `json:"effect"`
	}
	if json.Unmarshal([]byte(wrapper.Body), &body) != nil {
		return nil, fmt.Errorf("recipient body")
	}
	if body.Effect == "unknown" {
		f.Outcome = "unknown"
		f.Effect = "unknown"
		return p.final(f), nil
	}
	parts := []string{}
	for _, d := range body.Documents {
		v := strings.SplitN(d.Body, "=", 2)
		if len(v) != 2 {
			return nil, fmt.Errorf("current part")
		}
		parts = append(parts, v[1])
		f.Sources = append(f.Sources, acceptance.UtilityRef{ID: d.Ref.ID, Revision: d.Ref.Revision, SHA256: d.Ref.Hash})
	}
	f.Value = strings.Join(parts, "/")
	return p.final(f), nil
}

func (p *utilityWholeControlledProvider) emit(name string, args any) transport.EventStream {
	p.lastTool = name
	p.phase++
	return p.utilityTraceProvider.emit(name, args)
}
