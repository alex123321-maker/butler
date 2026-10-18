package app

import (
	"context"
	"errors"
	"github.com/butler/butler/apps/orchestrator/internal/completion"
	durableinput "github.com/butler/butler/apps/orchestrator/internal/input"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"os"
	"sort"
	"time"
	"unicode/utf8"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/experience"
	"github.com/butler/butler/internal/memory/jobs"
	"github.com/butler/butler/internal/memory/sanitize"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"
	"github.com/butler/butler/internal/skills"
)

func configureExperience(sources *catalog.Store, tools experience.ToolCatalog) (*experience.Store, error) {
	if os.Getenv("BUTLER_MEMORY_EXPERIENCE_ENABLED") != "true" {
		return nil, nil
	}
	if !experience.ReleaseQualified {
		return nil, experience.ErrEvidence
	}
	return experience.New(sources, os.Getenv("BUTLER_MEMORY_EXPERIENCE_ENVIRONMENT_HASH"), tools)
}

type experiencePolicies struct {
	store    *experience.Store
	jobs     *jobs.Store
	evidence *skillEvidence
}

func configureExperiencePolicies(s *experience.Store, j *jobs.Store, t *task.Store, h *transcript.Store, r *completion.Coordinator, c *catalog.Store) experience.PolicyCreator {
	if s == nil || j == nil {
		return nil
	}
	return &experiencePolicies{store: s, jobs: j, evidence: &skillEvidence{tasks: t, journal: actionauth.NewPostgresStore(c.Pool()), transcript: h, budgets: budget.NewLedger(c.Pool()), results: r, sources: c, lessons: s}}
}

// clipObservation preserves an explicit truncation/unknown marker. The whole
// original still owns provenance; an excerpt is never counted as full evidence.
func clipObservation(s string, n int) (string, bool) {
	previous := s
	s = sanitize.Text(s)
	if len(s) <= n {
		return s, s != previous
	}
	s = s[:n]
	for !utf8.ValidString(s) {
		s = s[:len(s)-1]
	}
	return s, true
}

// SealObservation is a separate owner operation. It accepts an existing RunID,
// never caller-authored evidence, and creates no policy, job or model attempt.
func (p *experiencePolicies) SealObservation(ctx context.Context, run string) (catalog.SourceRef, error) {
	_, source, err := p.sealObservation(ctx, run)
	return source, err
}

func (p *experiencePolicies) sealObservation(ctx context.Context, run string) (experience.Observation, catalog.SourceRef, error) {
	a, ok := owneridentity.FromContext(ctx)
	if !ok || a.OwnerID == "" || a.DeliveryID == "" || p == nil || p.store == nil || p.evidence == nil || run == "" {
		return experience.Observation{}, catalog.SourceRef{}, experience.ErrEvidence
	}
	o, parents, err := p.evidence.taskObservation(ctx, a.OwnerID, run, p.store.EnvironmentHash)
	if err != nil {
		return o, catalog.SourceRef{}, err
	}
	source, err := p.store.Seal(ctx, o, parents)
	return o, source, err
}

func (p *experiencePolicies) Create(ctx context.Context, q experience.PolicyRequest) (jobs.ModelPolicy, error) {
	if p == nil || p.jobs == nil || len(q.Policy.Job.Sources) != 0 || q.Policy.Job.TaskID != "" || q.Policy.Job.OriginalRunID != "" {
		return jobs.ModelPolicy{}, experience.ErrEvidence
	}
	o, src, err := p.sealObservation(ctx, q.RunID)
	if err != nil {
		return jobs.ModelPolicy{}, err
	}
	q.Policy.Handler = experience.Version
	q.Policy.Trigger = "source_changed"
	q.Policy.MaxJobs = 1
	q.Policy.Job.TaskID = o.TaskID
	q.Policy.Job.OriginalRunID = o.RunID
	q.Policy.Job.Sources = []catalog.SourceRef{src}
	if o.TerminalAttempt != nil {
		return p.jobs.CreateTerminalExperiencePolicy(ctx, q.Policy, src.ID)
	}
	return p.jobs.CreateModelPolicy(ctx, q.Policy)
}

// taskObservation reads typed owning repositories. A model final is retained as
// an agent report only; owner verification and every effect remain separate.
func (v *skillEvidence) taskObservation(ctx context.Context, owner, run, environment string) (experience.Observation, []catalog.SourceRef, error) {
	o := experience.Observation{Format: experience.SnapshotFormat, OwnerID: owner, RunID: run, Environment: experience.Environment{Fingerprint: environment, Tools: []skills.Dependency{}}, Constraints: []string{}, VerifiedCriteria: []string{}, Actions: []experience.Action{}, Failures: []experience.FailedCall{}, Unknowns: []string{}, OwnerVerdict: "unknown"}
	if v == nil || v.tasks == nil || v.journal == nil || v.results == nil || v.transcript == nil || v.sources == nil || v.budgets == nil {
		return o, nil, experience.ErrEvidence
	}
	t, err := v.tasks.GetByRun(ctx, run)
	if err != nil || t.OwnerID != owner || t.ParentTaskID != "" {
		return o, nil, experience.ErrEvidence
	}
	final, err := v.results.GetByRun(ctx, run)
	o.TaskID = t.TaskID
	var parents []catalog.SourceRef
	if errors.Is(err, completion.ErrNotFound) {
		stopped, observeErr := durableinput.NewStore(v.sources.Pool()).ObserveStopped(ctx, run)
		if observeErr != nil || stopped.TaskID != t.TaskID || stopped.RunID != run {
			return o, nil, experience.ErrEvidence
		}
		o.TaskRevision, o.ConstraintsRevision = stopped.TaskRevision, stopped.ConstraintsRevision
		o.TerminalAttempt = &experience.TerminalAttempt{InputID: stopped.InputID, InputStatus: stopped.InputStatus, Attempts: stopped.Attempts, Checkpoint: stopped.Checkpoint, InputHash: stopped.InputHash, InputRevision: stopped.InputRevision, Reason: stopped.Reason, RunState: stopped.RunState}
		if o.TerminalAttempt.Validate() != nil {
			return o, nil, experience.ErrEvidence
		}
		o.Unknowns = append(o.Unknowns, "assistant_final_unavailable", "original_attempt_stopped")
		parents, err = v.sources.StoppedAttemptParents(ctx, run, stopped.InputID)
	} else {
		if err != nil || final.PrivacyRedacted || final.TaskID != t.TaskID || final.MessageID == "" || final.TaskRevision < 1 || final.ConstraintsRevision != t.ConstraintsRevision {
			return o, nil, experience.ErrEvidence
		}
		o.TaskRevision, o.ConstraintsRevision = final.TaskRevision, final.ConstraintsRevision
		o.ResultID = final.ResultID
		parents, err = v.sources.VerifiedResultParents(ctx, owner, final.MessageID, run)
	}
	if err != nil {
		return o, nil, err
	}
	var clipped bool
	o.Goal, clipped = clipObservation(t.Goal, 2048)
	if clipped {
		o.Unknowns = append(o.Unknowns, "goal_excerpt_only")
	}
	for _, s := range t.Constraints {
		c, clipped := clipObservation(s, 1024)
		if clipped {
			o.Unknowns = append(o.Unknowns, "constraint_excerpt_only")
		}
		o.Constraints = append(o.Constraints, c)
	}
	if o.TerminalAttempt == nil {
		o.AgentReport, clipped = clipObservation(final.Content, 4096)
		if clipped {
			o.Unknowns = append(o.Unknowns, "agent_report_excerpt_only")
		}
	}
	results, err := v.tasks.Results(ctx, t.TaskID)
	if err != nil {
		return o, nil, err
	}
	for _, r := range results {
		if o.TerminalAttempt == nil && r.RunID == run && r.TaskRevision == o.TaskRevision && r.Verifier == "owner" && r.Verdict == "verified" && len(r.EvidenceRefs) > 0 && len(r.VerifiedCriteria) > 0 {
			o.OwnerVerdict = "verified"
			o.OwnerResultID = r.ResultID
			o.VerifiedCriteria = append([]string{}, r.VerifiedCriteria...)
		}
	}
	if t.Revision != o.TaskRevision && (t.Status != "completed" || t.Revision != o.TaskRevision+1 || o.OwnerVerdict != "verified" || t.CompletionResultID != o.OwnerResultID) {
		return o, nil, experience.ErrEvidence
	}
	history, err := v.transcript.CaptureRunTools(ctx, run, 2*experience.MaxActions)
	if err != nil {
		return o, nil, err
	}
	o.SessionKey = history.SessionKey
	o.TranscriptRevision = history.SourceRevision
	o.ToolPins = []transcript.ToolPin{}
	type callResult struct{ status, result string }
	calls := map[string]callResult{}
	usedCalls := map[string]bool{}
	for _, c := range history.Calls {
		if c.RunID == run {
			calls[c.ToolCallID] = callResult{c.Status, c.ResultJSON}
			switch c.Status {
			case "failed", "effect_unknown", "execution_stopped":
				result, clipped := clipObservation(sanitize.JSON(c.ResultJSON), 2048)
				if clipped {
					o.Unknowns = append(o.Unknowns, "failed_result_excerpt_only")
				}
				o.Failures = append(o.Failures, experience.FailedCall{ToolCallID: c.ToolCallID, Tool: c.ToolName, Status: c.Status, Result: result})
				usedCalls[c.ToolCallID] = true
			}
		}
	}
	effects, err := v.journal.GetRunEvidence(ctx, run)
	if err != nil || len(effects) > experience.MaxActions {
		return o, nil, experience.ErrEvidence
	}
	deps := map[string]skills.Dependency{}
	for _, e := range effects {
		r, err := v.journal.GetEffect(ctx, e.ActionID)
		if err != nil || r.Binding.OwnerID != owner || r.Binding.TaskID != t.TaskID || r.Binding.TaskRevision != o.TaskRevision {
			return o, nil, experience.ErrEvidence
		}
		d := skills.Dependency{Tool: r.Binding.ToolName, ContractHash: r.Binding.ContractHash, ContractRevision: r.Binding.ContractRevision}
		if prior, ok := deps[d.Tool]; ok && prior != d {
			return o, nil, experience.ErrEvidence
		}
		deps[d.Tool] = d
		c, exists := calls[r.Binding.ToolCallID]
		if !exists {
			return o, nil, experience.ErrEvidence
		}
		usedCalls[r.Binding.ToolCallID] = true
		result, clipped := clipObservation(sanitize.JSON(c.result), 2048)
		if clipped {
			o.Unknowns = append(o.Unknowns, "tool_result_excerpt_only")
		}
		failure := ""
		switch c.status {
		case "failed", "effect_unknown", "execution_stopped":
			failure = c.status
		}
		o.Actions = append(o.Actions, experience.Action{ID: r.ActionID, ToolCallID: r.Binding.ToolCallID, Tool: d.Tool, ContractHash: d.ContractHash, ContractRevision: d.ContractRevision, ArgumentsHash: r.Binding.ArgsHash, Effect: r.State, Evidence: append([]string{}, r.EvidenceRefs...), Result: result, Failure: failure})
	}
	for _, pin := range history.Pins {
		if usedCalls[pin.ID] {
			o.ToolPins = append(o.ToolPins, pin)
		}
	}
	for _, d := range deps {
		o.Environment.Tools = append(o.Environment.Tools, d)
	}
	sort.Slice(o.Environment.Tools, func(i, j int) bool { return o.Environment.Tools[i].Tool < o.Environment.Tools[j].Tool })
	// Preserve all actual actions and failures; fail instead of dropping overflow.
	sort.Slice(o.Actions, func(i, j int) bool { return o.Actions[i].ID < o.Actions[j].ID })
	seen := map[string]bool{}
	unknowns := []string{}
	for _, s := range o.Unknowns {
		if !seen[s] {
			unknowns = append(unknowns, s)
			seen[s] = true
		}
	}
	o.Unknowns = unknowns
	u, err := v.budgets.ModelRunUsage(ctx, owner, run)
	if err != nil {
		return o, nil, err
	}
	o.Usage = &u
	if err = o.Validate(); err != nil {
		return o, nil, err
	}
	return o, parents, nil
}

type experienceRunSources struct {
	sources interface {
		PinTranscript(context.Context, string, int64) error
		CheckRunSources(context.Context, string) error
	}
	catalog *catalog.Store
	store   *experience.Store
}

func (s experienceRunSources) PinTranscript(ctx context.Context, session string, revision int64) error {
	return s.sources.PinTranscript(ctx, session, revision)
}

// Preserve an existing compound owning guard's explicit bound. Ordinary Catalog
// checks retain their unchanged 200ms deadline.
func (s experienceRunSources) MemorySourceCheckTimeout() time.Duration {
	if bounded, ok := s.sources.(interface{ MemorySourceCheckTimeout() time.Duration }); ok {
		return bounded.MemorySourceCheckTimeout()
	}
	return 200 * time.Millisecond
}
func (s experienceRunSources) CheckRunSources(ctx context.Context, run string) error {
	if err := s.sources.CheckRunSources(ctx, run); err != nil {
		return err
	}
	if s.store == nil {
		tx, err := s.catalog.Pool().Begin(ctx)
		if err != nil {
			return err
		}
		defer tx.Rollback(ctx)
		return s.store.CheckPinnedTx(ctx, tx, run)
	}
	return s.store.CheckPinned(ctx, run)
}

// Episode adapters already authorize the exact Catalog object. Additional
// eligibility runs with current Broker contracts and adds the final run pin.
func experienceObjectAllowed(ctx context.Context, sources *catalog.Store, s *experience.Store, id int64) (bool, error) {
	if sources == nil {
		return true, nil
	}
	var lesson bool
	if err := sources.Pool().QueryRow(ctx, `WITH RECURSIVE roots AS(SELECT source_id AS id FROM memory_object_sources WHERE memory_type='episodic' AND object_id=$1 UNION SELECT d.parent_source_id FROM memory_source_dependencies d JOIN roots r ON r.id=d.child_source_id)SELECT EXISTS(SELECT 1 FROM roots r JOIN memory_experience_lessons l ON l.source_id=r.id)`, id).Scan(&lesson); err != nil {
		return false, err
	}
	if !lesson {
		return true, nil
	}
	if s == nil {
		return false, nil
	}
	token, ok := ownership.TokenFromContext(ctx)
	if !ok {
		return false, nil
	}
	var current experience.Prepared
	var err error
	if s != nil {
		current, err = s.Prepare(ctx, token.RunID)
		if err != nil {
			return false, err
		}
	}
	tx, err := sources.Pool().Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	var ref catalog.SourceRef
	if tx.QueryRow(ctx, `SELECT source_id,source_revision FROM memory_object_sources WHERE memory_type='episodic' AND object_id=$1`, id).Scan(&ref.ID, &ref.Revision) != nil {
		return false, nil
	}
	ss, err := sources.AccessTx(ctx, tx, []catalog.SourceRef{ref}, "memory_read")
	if err != nil || len(ss) != 1 {
		return false, err
	}
	allowed, err := s.EligibleTx(ctx, tx, ss[0], current, true)
	if err != nil || !allowed {
		return false, err
	}
	return true, tx.Commit(ctx)
}
