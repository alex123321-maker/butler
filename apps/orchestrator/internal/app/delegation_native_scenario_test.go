package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	approvals "github.com/butler/butler/apps/orchestrator/internal/approval"
	"github.com/butler/butler/apps/orchestrator/internal/completion"
	"github.com/butler/butler/apps/orchestrator/internal/goal"
	durableinput "github.com/butler/butler/apps/orchestrator/internal/input"
	runservice "github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/delegation"
	runv1 "github.com/butler/butler/internal/gen/run/v1"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/jackc/pgx/v5/pgxpool"
)

const nativeFixtureOwner = "native-research-owner"

type nativeWidthEvidence struct {
	Width                      int                    `json:"width"`
	TaskID                     string                 `json:"task_id"`
	InputID                    string                 `json:"input_id"`
	ParentRunID                string                 `json:"parent_run_id"`
	ChildTaskIDs               []string               `json:"child_task_ids"`
	ChildRunIDs                []string               `json:"child_run_ids"`
	ChildResultIDs             []string               `json:"child_result_ids"`
	ReportSourceRefs           []delegation.SourceRef `json:"report_source_refs"`
	SourceRefs                 []delegation.SourceRef `json:"source_refs"`
	ObjectiveChecks            []bool                 `json:"objective_checks"`
	ParentWorkerKilled         bool                   `json:"parent_worker_killed"`
	ClaimExpiryWaitMillis      int64                  `json:"claim_expiry_wait_millis"`
	RecoveryAndChildMillis     int64                  `json:"recovery_and_child_millis"`
	ParentReceiptCheck         bool                   `json:"parent_receipt_check"`
	ElapsedMillis              int64                  `json:"elapsed_millis"`
	MaximumChildOverlap        int                    `json:"maximum_child_overlap"`
	SharedRootReservations     budget.Units           `json:"shared_root_reservations"`
	ParentTaskStillActive      bool                   `json:"parent_task_still_active"`
	IndependentConflictKept    bool                   `json:"independent_conflict_kept"`
	UnverifiedAcceptanceDenied bool                   `json:"unverified_acceptance_denied"`
}
type nativeScenarioEvidence struct {
	Status            string                  `json:"status"`
	ReleaseRef        string                  `json:"release_ref"`
	NativeAttempts    int                     `json:"native_attempts"`
	MaxAttempts       int                     `json:"max_attempts"`
	Widths            []nativeWidthEvidence   `json:"widths"`
	OwnerStopDenies   bool                    `json:"owner_stop_denies"`
	RevisionDenies    bool                    `json:"revision_denies"`
	DeletionDenies    bool                    `json:"deletion_denies"`
	CostBasis         string                  `json:"cost_basis"`
	SharedDayReserved budget.Units            `json:"shared_day_reserved"`
	Attempts          []nativeAttemptEvidence `json:"attempts"`
	Usage             []nativeUsageEvidence   `json:"usage"`
	Limitations       []string                `json:"limitations"`
}
type nativeAttemptEvidence struct {
	Attempt   int       `json:"attempt"`
	RunID     string    `json:"run_id"`
	Kind      string    `json:"kind"`
	InputHash string    `json:"input_hash"`
	CreatedAt time.Time `json:"created_at"`
}
type nativeUsageEvidence struct {
	RunID              string `json:"run_id"`
	ReservationID      string `json:"reservation_id"`
	ActualInputTokens  *int64 `json:"actual_input_tokens"`
	ActualOutputTokens *int64 `json:"actual_output_tokens"`
}

type nativeScenarioFixture struct {
	ctx                                              context.Context
	pool                                             *pgxpool.Pool
	sources                                          *catalog.Store
	tasks                                            *task.Store
	inputs                                           *durableinput.Store
	sessions                                         *session.PostgresRepository
	goals                                            *goal.Manager
	goalID, mandateID, manifest, private, corpusPath string
	corpus                                           nativeResearchCorpus
	args                                             []delegation.AdmissionArgs
}

func nativeOwner(ctx context.Context, delivery, sessionKey string) context.Context {
	return owneridentity.WithActor(ctx, owneridentity.Actor{OwnerID: nativeFixtureOwner, Channel: "web", DeliveryID: delivery, SessionKey: sessionKey})
}

func newNativeScenarioFixture(t *testing.T, ctx context.Context) *nativeScenarioFixture {
	t.Helper()
	pool := memorytest.Database(t)
	private := t.TempDir()
	manifestPath := filepath.Join(private, "deletions.log")
	anchor := catalog.NewFileManifest(manifestPath)
	if anchor.Initialize() != nil {
		t.Fatal("private retention anchor unavailable")
	}
	policy := nativeExperimentPolicy()
	if policy.Validate() != nil {
		t.Fatal("bounded subscription policy invalid")
	}
	bc := budget.NewController(pool, policy)
	bc.SetAuthorityResolver(actionauth.ResolveBudgetAuthority)
	tasks := task.NewStore(pool)
	tasks.SetBudgets(bc, "")
	f := &nativeScenarioFixture{ctx: ctx, pool: pool, sources: catalog.New(pool, anchor), tasks: tasks, inputs: durableinput.NewStore(pool), sessions: session.NewPostgresRepository(pool), private: private, manifest: manifestPath, corpusPath: os.Getenv("BUTLER_DELEGATION_NATIVE_CORPUS")}
	var err error
	f.corpus, err = readNativeCorpus(f.corpusPath)
	if err != nil {
		t.Fatal("immutable research corpus unavailable")
	}
	if _, err = newNativeDelegationLimiter(ctx, pool); err != nil {
		t.Fatal("durable attempt bound unavailable")
	}
	if _, err = pool.Exec(ctx, `CREATE TABLE fixture_native_sources(key text PRIMARY KEY,source_id text NOT NULL UNIQUE);
 CREATE TABLE fixture_native_gates(task_id text PRIMARY KEY,entered_at timestamptz NOT NULL);
 CREATE TABLE fixture_native_contexts(run_id text PRIMARY KEY,is_child boolean NOT NULL,input_hash text NOT NULL,source_ids jsonb NOT NULL,tool_count integer NOT NULL);`); err != nil {
		t.Fatal("fixture metadata unavailable")
	}
	f.goals = goal.New(pool, tasks, nil)
	expires := time.Now().Add(35 * time.Minute).UTC().Truncate(time.Microsecond)
	g, err := f.goals.PutGoal(nativeOwner(ctx, "research-goal", ""), goal.GoalRequest{Goal: actionauth.GoalPolicy{Title: "Сравнить два независимых наблюдения без повышения модельного вывода до истины", Kind: "objective", Status: "active", Purpose: "native_research", Constraints: []string{"Only two selected synthetic sources; no credentials, network tools or final-verdict promotion."}, ExpiresAt: expires}})
	if err != nil {
		t.Fatal("explicit owner objective denied")
	}
	f.goalID = g.SubjectID
	u := policy.Mandate
	m, err := f.goals.PutMandate(nativeOwner(ctx, "research-mandate", ""), goal.MandateRequest{Mandate: actionauth.Mandate{GoalID: g.SubjectID, GoalRevision: g.Revision, Status: "active", Purpose: "native_research", Tools: []string{delegation.AdmissionTool}, ResourceScopes: []actionauth.ResourceScope{{Origin: "tool:" + delegation.AdmissionTool}}, Effects: []actionauth.Effect{actionauth.EffectPrepare}, Privacy: "cloud_allowed", Budget: actionauth.MandateBudget{ModelTurns: u.ModelTurns, Tokens: u.Tokens, Children: u.Children, ExternalCalls: u.ExternalCalls, ActiveMillis: u.ActiveMillis, EgressBytes: u.EgressBytes, ProviderInputBytes: u.ProviderInputBytes}, ExpiresAt: expires, Intervention: []string{"scope_change", "effect_unknown", "budget_exhausted"}, Reporting: "exceptions"}})
	if err != nil {
		t.Fatal("finite explicit cloud research mandate denied")
	}
	f.mandateID = m.SubjectID
	deadline := time.Now().Add(30 * time.Minute).UTC().Truncate(time.Microsecond)
	for _, s := range f.corpus.Sources {
		source, err := f.sources.Put(nativeOwner(ctx, "source-"+s.Key, ""), catalog.PutSource{ExternalKey: s.Key, Kind: "document", ScopeType: "user", ScopeID: nativeFixtureOwner, Title: s.Title, Body: s.Body})
		if err != nil {
			t.Fatal("selected source publication denied")
		}
		ref := guardSource(source)
		body := f.corpus.Question + "\nВерни только JSON, без Markdown и пояснений. Поля: source_id, source_revision, source_hash, observed_value (целое), observed_date (YYYY-MM-DD), uncertainty (ровно source_only). Точная ссылка на проверяемый источник: " + string(nativeJSON(ref))
		contract, err := f.sources.Put(nativeOwner(ctx, "contract-"+s.Key, ""), catalog.PutSource{ExternalKey: "contract-" + s.Key, Kind: "model_candidate", ScopeType: "user", ScopeID: nativeFixtureOwner, Title: "Ограниченное поручение " + s.Key, Body: body, Parents: []catalog.SourceRef{{ID: ref.ID, Revision: ref.Revision, AccessRevision: ref.AccessRevision, Hash: ref.Hash}}})
		if err != nil {
			t.Fatal("bounded contract DATA publication denied")
		}
		if _, err = pool.Exec(ctx, `INSERT INTO fixture_native_sources VALUES($1,$2)`, s.Key, source.ID); err != nil {
			t.Fatal("source citation metadata unavailable")
		}
		f.args = append(f.args, delegation.AdmissionArgs{ContractSource: guardSource(contract), Sources: []delegation.SourceRef{ref}, Dependencies: []string{}, Budget: delegation.Limits{ModelTurns: 2, Tokens: 3 * 1048576, ActiveMillis: 240000, ProviderInputBytes: 2 * 131072}, Deadline: deadline})
	}
	return f
}
func nativeJSON(v any) []byte { b, _ := json.Marshal(v); return b }

func (f *nativeScenarioFixture) composition(t *testing.T, width int) (*DelegationAdmission, *actionContextReader) {
	t.Helper()
	limits := task.DefaultDelegationLimits()
	limits.MaxParallel = width
	admission, err := NewDelegationAdmission(f.pool, f.tasks, f.sessions, f.inputs, f.sources, limits)
	if err != nil || admission.CheckReady(f.ctx) != nil {
		t.Fatal("actual admission composition denied")
	}
	reader := &actionContextReader{runs: runservice.NewService(runservice.NewPostgresRepository(f.pool), slog.New(slog.NewTextHandler(io.Discard, nil))), sessions: f.sessions, leases: session.NewPostgresLeaseManager(f.pool), tasks: f.tasks, pool: f.pool, delegationReady: true}
	return admission, reader
}

// Zero-inference composition proof includes actual authenticated owner policy,
// scoped immutable sources and both production manager width configurations.
func TestNativeDelegationCompositionWithoutInferenceIntegration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	f := newNativeScenarioFixture(t, ctx)
	for _, width := range []int{1, 2} {
		f.composition(t, width)
	}
	var attempts int
	if f.pool.QueryRow(ctx, `SELECT attempts FROM fixture_native_limit`).Scan(&attempts) != nil || attempts != 0 {
		t.Fatal("composition performed inference")
	}
}

type nativeSubprocess struct {
	cmd     *exec.Cmd
	done    chan error
	stopped bool
}

func startNativeSubprocess(t *testing.T, binary, testName, dsn, private, label string, env []string) *nativeSubprocess {
	t.Helper()
	cmd := exec.Command(binary, "-test.run=^"+testName+"$", "-test.timeout=19m")
	cmd.Env = append(os.Environ(), append([]string{"BUTLER_DELEGATION_POSTGRES_URL=" + dsn}, env...)...)
	log, err := os.OpenFile(filepath.Join(private, label+".log"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal("private process log unavailable")
	}
	cmd.Stdout, cmd.Stderr = log, log
	if cmd.Start() != nil {
		log.Close()
		t.Fatal("private process start denied")
	}
	p := &nativeSubprocess{cmd: cmd, done: make(chan error, 1)}
	go func() { p.done <- cmd.Wait(); log.Close() }()
	t.Cleanup(func() { p.stop(t, false) })
	return p
}
func (p *nativeSubprocess) stop(t *testing.T, kill bool) {
	t.Helper()
	if p.stopped {
		return
	}
	p.stopped = true
	if kill {
		_ = p.cmd.Process.Kill()
	} else {
		_ = p.cmd.Process.Signal(os.Interrupt)
	}
	select {
	case err := <-p.done:
		if err != nil && !kill {
			t.Error("private subprocess did not stop cleanly")
		}
	case <-time.After(8 * time.Second):
		_ = p.cmd.Process.Kill()
		<-p.done
		t.Error("private subprocess shutdown deadline")
	}
}
func nativeWait(t *testing.T, ctx context.Context, timeout time.Duration, condition func() bool) {
	t.Helper()
	until := time.Now().Add(timeout)
	for time.Now().Before(until) && ctx.Err() == nil {
		if condition() {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("native scenario condition deadline; no automatic retry")
}

func TestNativeDelegationActualScenarioIntegration(t *testing.T) {
	if os.Getenv("BUTLER_DELEGATION_NATIVE_ACCEPTANCE") != "1" {
		t.Skip("native experiment requires explicit operator opt-in")
	}
	workerBinary, brokerBinary := os.Getenv("BUTLER_DELEGATION_NATIVE_WORKER_BINARY"), os.Getenv("BUTLER_DELEGATION_BROKER_BINARY")
	if workerBinary == "" || brokerBinary == "" || os.Getenv("BUTLER_DELEGATION_NATIVE_IMPORT_FILE") == "" {
		t.Fatal("reviewed private runtime references required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 18*time.Minute)
	defer cancel()
	f := newNativeScenarioFixture(t, ctx)
	report := nativeScenarioEvidence{Status: "failed", ReleaseRef: os.Getenv("BUTLER_DELEGATION_NATIVE_RELEASE"), MaxAttempts: 12, CostBasis: "operator_declared_subscription_no_overage", Limitations: []string{"gate_controlled_loss_after_parent_attempt_completed", "cloud_weights_not_observable", "no_local_generative_parity_claim", "blocked_attempt_report_bridge_pending", "metadata_barriers_do_not_claim_late_native_callback_injection"}}
	defer func() {
		auditCtx, auditCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer auditCancel()
		if f.pool.QueryRow(auditCtx, `SELECT attempts FROM fixture_native_limit`).Scan(&report.NativeAttempts) != nil {
			report.NativeAttempts = -1
			report.Status = "failed"
			report.Limitations = append(report.Limitations, "attempt_evidence_unavailable")
		}
		var raw []byte
		if f.pool.QueryRow(auditCtx, `SELECT used_json FROM execution_budget_scopes WHERE kind='owner_day' AND owner_id=$1`, nativeFixtureOwner).Scan(&raw) == nil {
			if json.Unmarshal(raw, &report.SharedDayReserved) != nil {
				report.Status = "failed"
			}
		} else {
			report.Status = "failed"
		}
		rows, e := f.pool.Query(auditCtx, `SELECT attempt,run_id,kind,input_hash,created_at FROM fixture_native_attempts ORDER BY attempt`)
		if e == nil {
			for rows.Next() {
				var a nativeAttemptEvidence
				if rows.Scan(&a.Attempt, &a.RunID, &a.Kind, &a.InputHash, &a.CreatedAt) != nil {
					report.Status = "failed"
					break
				}
				report.Attempts = append(report.Attempts, a)
			}
			if rows.Err() != nil {
				report.Status = "failed"
			}
			rows.Close()
		} else {
			report.Status = "failed"
		}
		rows, e = f.pool.Query(auditCtx, `SELECT run_id,reservation_id,actual_input_tokens,actual_output_tokens FROM execution_model_attempts ORDER BY created_at,operation_id`)
		if e == nil {
			for rows.Next() {
				var a nativeUsageEvidence
				if rows.Scan(&a.RunID, &a.ReservationID, &a.ActualInputTokens, &a.ActualOutputTokens) != nil {
					report.Status = "failed"
					break
				}
				report.Usage = append(report.Usage, a)
			}
			if rows.Err() != nil {
				report.Status = "failed"
			}
			rows.Close()
		} else {
			report.Status = "failed"
		}
		if len(report.Attempts) != report.NativeAttempts || len(report.Usage) != report.NativeAttempts {
			report.Status = "failed"
		}
		if report.Status != "passed" && !t.Failed() {
			t.Error("native experiment evidence incomplete")
		}
		if os.WriteFile(os.Getenv("BUTLER_DELEGATION_NATIVE_REPORT"), nativeJSON(report), 0600) != nil {
			t.Error("content-free scenario report unavailable")
		}
	}()
	u, err := url.Parse(f.pool.Config().ConnConfig.ConnString())
	if err != nil {
		t.Fatal("isolated DSN unavailable")
	}
	q := u.Query()
	q.Set("search_path", f.pool.Config().ConnConfig.RuntimeParams["search_path"])
	u.RawQuery = q.Encode()
	dsn := u.String()
	var random [32]byte
	if _, err = rand.Read(random[:]); err != nil {
		t.Fatal("private process proof unavailable")
	}
	token := hex.EncodeToString(random[:])
	tokenFile := filepath.Join(f.private, "authority-token")
	if os.WriteFile(tokenFile, []byte(token), 0600) != nil {
		t.Fatal("private proof file unavailable")
	}
	var current atomic.Pointer[DelegationAdmission]
	var currentReader atomic.Pointer[actionContextReader]
	mux := http.NewServeMux()
	mux.HandleFunc("/context", func(w http.ResponseWriter, r *http.Request) {
		reader := currentReader.Load()
		if reader == nil {
			http.Error(w, "not ready", 503)
			return
		}
		actionauth.NewContextHandler(token, reader).ServeHTTP(w, r)
	})
	mux.HandleFunc("/delegate", func(w http.ResponseWriter, r *http.Request) {
		service := current.Load()
		if service == nil {
			http.Error(w, "not ready", 503)
			return
		}
		h, e := delegation.NewAdmissionHandler(token, service)
		if e != nil {
			http.Error(w, "not ready", 503)
			return
		}
		h.ServeHTTP(w, r)
	})
	control := httptest.NewServer(mux)
	defer control.Close()
	address := freeAddress(t)
	policyJSON := string(nativeJSON(nativeExperimentPolicy()))
	broker := startNativeSubprocess(t, brokerBinary, "TestNativeDelegationBrokerHarness", dsn, f.private, "broker", []string{"BUTLER_DELEGATION_NATIVE_BROKER_HARNESS=1", "BUTLER_DELEGATION_TOKEN_FILE=" + tokenFile, "BUTLER_DELEGATION_CONTEXT_URL=" + control.URL + "/context", "BUTLER_DELEGATION_ADMISSION_URL=" + control.URL + "/delegate", "BUTLER_DELEGATION_GRPC_ADDRESS=" + address, "BUTLER_DELEGATION_MANIFEST_FILE=" + f.manifest, "BUTLER_EXECUTION_BUDGET_POLICY_JSON=" + policyJSON})
	defer broker.stop(t, false)
	nativeWait(t, ctx, 10*time.Second, func() bool {
		c, e := net.DialTimeout("tcp", address, 50*time.Millisecond)
		if e != nil {
			return false
		}
		c.Close()
		return true
	})
	for _, width := range []int{1, 2} {
		admission, reader := f.composition(t, width)
		current.Store(admission)
		currentReader.Store(reader)
		_ = f.runWidth(t, width, admission, address, tokenFile, dsn, workerBinary, &report)
	}
	// Real owner/control/source mutations, after the same native finals and
	// reports have been persisted. These are no-inference authority barriers,
	// separately labelled from an arbitrary late streaming model callback.
	first, second := report.Widths[0], report.Widths[1]
	admission, _ := f.composition(t, 2)
	leases := session.NewPostgresLeaseManager(f.pool)
	controls := task.NewController(f.tasks, leases.Authority(), leases, runservice.NewPostgresRepository(f.pool), approvals.NewPostgresRepository(f.pool), f.inputs, transcript.NewStore(f.pool))
	parent, err := f.tasks.Get(ctx, second.TaskID)
	if err != nil {
		t.Fatal("second parent metadata unavailable")
	}
	if _, err = controls.Apply(nativeOwner(ctx, "actual-owner-revision", parent.SessionKey), task.ControlRequest{TaskID: parent.TaskID, ExpectedRevision: parent.Revision, Kind: task.ControlAmend, Instruction: "Сохранить раздельные наблюдения; уточнение не принимает старый результат."}); err != nil {
		t.Fatal("actual owner amendment denied")
	}
	if _, err = admission.Manager.CollectGenerated(ctx, parent.TaskID, second.ChildTaskIDs[0], second.ChildResultIDs[0]); err == nil {
		t.Fatal("stale parent revision collected old report")
	}
	report.RevisionDenies = true
	var deletedChildRun, retainedChildRun string
	for i, target := range []*string{&deletedChildRun, &retainedChildRun} {
		if f.pool.QueryRow(ctx, `SELECT r.run_id FROM runs r JOIN tasks t ON t.task_id=r.task_id JOIN memory_run_sources s USING(run_id) WHERE t.parent_task_id=$1 AND s.source_id=$2 LIMIT 1`, first.TaskID, f.args[i].Sources[0].ID).Scan(target) != nil || checkRecordedInitiativeSources(ctx, f.pool, f.sources, *target) != nil {
			t.Fatal("current exact historical source release denied before mutation")
		}
	}
	if _, err = f.sources.Delete(nativeOwner(ctx, "actual-owner-delete", ""), f.args[0].Sources[0].ID, "delete", f.args[0].Sources[0].Revision); err != nil {
		t.Fatal("actual owner deletion denied")
	}
	if err = checkRecordedInitiativeSources(ctx, f.pool, f.sources, deletedChildRun); err == nil {
		t.Fatal("deleted original retained historical run access")
	}
	report.DeletionDenies = true
	if checkRecordedInitiativeSources(ctx, f.pool, f.sources, retainedChildRun) != nil {
		t.Fatal("unrelated selected source lost current release before stop")
	}
	parent, err = f.tasks.Get(ctx, first.TaskID)
	if err != nil {
		t.Fatal("parent metadata unavailable")
	}
	if _, err = controls.Apply(nativeOwner(ctx, "actual-owner-stop", parent.SessionKey), task.ControlRequest{TaskID: parent.TaskID, ExpectedRevision: parent.Revision, Kind: task.ControlStop}); err != nil {
		t.Fatal("actual owner stop denied")
	}
	if checkRecordedInitiativeSources(ctx, f.pool, f.sources, retainedChildRun) == nil {
		t.Fatal("stopped branch retained current source release")
	}
	if _, err = admission.Manager.Advance(ctx, parent.TaskID); err == nil {
		t.Fatal("stopped parent continued child lifecycle")
	}
	report.OwnerStopDenies = true
	var attempts int
	var dayRaw []byte
	if f.pool.QueryRow(ctx, `SELECT used_json FROM execution_budget_scopes WHERE kind='owner_day' AND owner_id=$1`, nativeFixtureOwner).Scan(&dayRaw) != nil || json.Unmarshal(dayRaw, &report.SharedDayReserved) != nil {
		t.Fatal("shared day reservation evidence unavailable")
	}
	if f.pool.QueryRow(ctx, `SELECT attempts FROM fixture_native_limit`).Scan(&attempts) != nil || attempts > 12 || report.SharedDayReserved.ModelTurns > 12 {
		t.Fatal("experiment exceeded shared attempt bound")
	}
	report.Status = "passed"
}

func (f *nativeScenarioFixture) runWidth(t *testing.T, width int, admission *DelegationAdmission, broker, tokenFile, dsn, binary string, report *nativeScenarioEvidence) nativeWidthEvidence {
	t.Helper()
	started := time.Now()
	sessionKey := "native-root-width-" + string(rune('0'+width))
	if _, _, err := f.sessions.CreateSession(f.ctx, session.CreateSessionParams{SessionKey: sessionKey, UserID: nativeFixtureOwner, Channel: "web", MetadataJSON: `{}`}); err != nil {
		t.Fatal("actual parent session denied")
	}
	prompt := "Создай ровно два независимых поручения: по одному последовательному вызову task.delegate для каждого следующего JSON. Используй аргументы без изменений, не подменяй ссылки, бюджет и срок. Не отвечай на исследовательский вопрос сам. После двух успешных receipts заверши свой ответ JSON с полем admitted_task_ids, содержащим ровно две фактические task_id из receipts. Не вызывай другие инструменты и не создавай третье поручение.\n" + string(nativeJSON(f.args[0])) + "\n" + string(nativeJSON(f.args[1]))
	root, err := f.goals.CreateOwnerTask(nativeOwner(f.ctx, "root-"+sessionKey, sessionKey), goal.TaskRequest{SessionKey: sessionKey, GoalID: f.goalID, MandateID: f.mandateID, Purpose: "native_research", Goal: "Два независимых ограниченных исследования", Constraints: []string{"Create only the two exact source-bound context-only children; retain disagreements."}, CompletionCriteria: []string{"Independent immutable-source checker, no model consensus"}, InitiativeReason: "explicit lab owner experiment", ObservationRef: "fixture:native-research"})
	if err != nil {
		t.Fatal("actual assigned parent task denied")
	}
	event, err := ingress.NormalizeEvent(ingress.NormalizeRequest{EventType: runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE, EventID: sessionKey, SessionKey: sessionKey, Source: "web", Payload: map[string]any{"user_id": nativeFixtureOwner, "task_id": root.TaskID, "text": prompt}, DeliveryID: sessionKey})
	if err != nil {
		t.Fatal("bounded owner input invalid")
	}
	receipt, _, err := f.inputs.Accept(nativeOwner(f.ctx, sessionKey, sessionKey), event, nativeFixtureOwner, root.TaskID)
	if err != nil {
		t.Fatal("actual ordered parent input denied")
	}
	v := nativeWidthEvidence{Width: width, TaskID: root.TaskID, InputID: receipt.InputID, SourceRefs: []delegation.SourceRef{f.args[0].Sources[0], f.args[1].Sources[0]}}
	index := len(report.Widths)
	report.Widths = append(report.Widths, v)
	defer func() { v.ElapsedMillis = time.Since(started).Milliseconds(); report.Widths[index] = v }()
	config := nativeWorkerConfig{Width: width, Gate: true, Broker: broker, TokenFile: tokenFile, ManifestFile: f.manifest, ImportFile: os.Getenv("BUTLER_DELEGATION_NATIVE_IMPORT_FILE"), KeyFile: filepath.Join(f.private, "encrypted-settings-key"), CorpusFile: f.corpusPath}
	start := func(stage string) *nativeSubprocess {
		p := filepath.Join(f.private, sessionKey+"-"+stage+".json")
		if os.WriteFile(p, nativeJSON(config), 0600) != nil {
			t.Fatal("private worker config unavailable")
		}
		return startNativeSubprocess(t, binary, "TestNativeDelegationWorkerHarness", dsn, f.private, sessionKey+"-"+stage, []string{"BUTLER_DELEGATION_NATIVE_WORKER_HARNESS=1", "BUTLER_DELEGATION_NATIVE_WORKER_CONFIG=" + p})
	}
	worker := start("gated")
	nativeWait(t, f.ctx, 5*time.Minute, func() bool {
		var n int
		var status, runID string
		if len(worker.done) > 0 {
			t.Fatal("parent worker stopped before durable receipt boundary")
		}
		if f.pool.QueryRow(f.ctx, `SELECT status FROM accepted_inputs WHERE input_id=$1`, receipt.InputID).Scan(&status) == nil && (status == "blocked" || status == "failed" || status == "expired" || status == "waiting_for_user") {
			t.Fatal("native parent stopped before two admissions; no retry")
		}
		if f.pool.QueryRow(f.ctx, `SELECT count(*) FROM delegation_admission_snapshots s JOIN task_delegations d USING(task_id) WHERE d.parent_task_id=$1`, root.TaskID).Scan(&n) != nil || n != 2 {
			return false
		}
		if f.pool.QueryRow(f.ctx, `SELECT status,COALESCE(run_id,'') FROM accepted_inputs WHERE input_id=$1`, receipt.InputID).Scan(&status, &runID) != nil || status != "completed" {
			return false
		}
		v.ParentRunID = runID
		if f.pool.QueryRow(f.ctx, `SELECT count(*) FROM fixture_native_gates g JOIN tasks t USING(task_id) WHERE t.parent_task_id=$1`, root.TaskID).Scan(&n) != nil || n != width {
			return false
		}
		return true
	})
	var childAttempts int
	if f.pool.QueryRow(f.ctx, `SELECT count(*) FROM fixture_native_attempts a JOIN runs r USING(run_id) JOIN tasks t ON t.task_id=r.task_id WHERE t.parent_task_id=$1`, root.TaskID).Scan(&childAttempts) != nil || childAttempts != 0 {
		t.Fatal("scheduling gate ran child inference before crash")
	}
	worker.stop(t, true)
	v.ParentWorkerKilled = true
	var oldClaimExpires time.Time
	if f.pool.QueryRow(f.ctx, `SELECT max(i.claim_expires_at) FROM accepted_inputs i JOIN tasks t ON t.task_id=i.task_id WHERE t.parent_task_id=$1 AND i.claim_id IS NOT NULL`, root.TaskID).Scan(&oldClaimExpires) != nil {
		t.Fatal("actual gated input claim absent")
	}
	config.Gate = false
	restartedAt := time.Now()
	v.ClaimExpiryWaitMillis = max(int64(0), oldClaimExpires.Sub(restartedAt).Milliseconds())
	worker = start("restarted")
	nativeWait(t, f.ctx, 5*time.Minute, func() bool {
		var n int
		if len(worker.done) > 0 {
			t.Fatal("restarted worker stopped before child reports")
		}
		var denied bool
		if f.pool.QueryRow(f.ctx, `SELECT EXISTS(SELECT 1 FROM accepted_inputs i JOIN tasks t ON t.task_id=i.task_id WHERE t.parent_task_id=$1 AND i.status IN ('blocked','failed','expired','waiting_for_user'))`, root.TaskID).Scan(&denied) == nil && denied {
			t.Fatal("native child stopped; no retry")
		}
		return f.pool.QueryRow(f.ctx, `SELECT count(*) FROM task_delegations WHERE parent_task_id=$1 AND state='result_ready'`, root.TaskID).Scan(&n) == nil && n == 2
	})
	v.RecoveryAndChildMillis = time.Since(restartedAt).Milliseconds()
	if time.Now().Before(oldClaimExpires) {
		t.Fatal("accepted child resumed before real claim expiry")
	}
	worker.stop(t, false)
	rows, err := f.pool.Query(f.ctx, `SELECT d.task_id,d.contract_json,d.decision_evidence_json,f.run_id,f.result_id FROM task_delegations d JOIN run_final_results f ON f.task_id=d.task_id WHERE d.parent_task_id=$1 ORDER BY d.contract_source_id`, root.TaskID)
	if err != nil {
		t.Fatal("actual child finals unavailable")
	}
	type childFinal struct {
		taskID, runID, resultID string
		c                       delegation.Contract
		r                       delegation.Report
		reportRef               struct {
			Report *delegation.SourceRef `json:"report_source"`
		}
	}
	var finals []childFinal
	for rows.Next() {
		var x childFinal
		var contract, report []byte
		if rows.Scan(&x.taskID, &contract, &report, &x.runID, &x.resultID) != nil || json.Unmarshal(contract, &x.c) != nil || json.Unmarshal(report, &x.reportRef) != nil || x.reportRef.Report == nil {
			rows.Close()
			t.Fatal("durable child projection invalid")
		}
		finals = append(finals, x)
	}
	if rows.Err() != nil {
		rows.Close()
		t.Fatal("actual child evidence read incomplete")
	}
	rows.Close()
	if len(finals) != 2 {
		t.Fatal("independent finals missing")
	}
	for _, x := range finals {
		storedReport, err := f.sources.Get(nativeOwner(f.ctx, "read-report-"+x.taskID, ""), x.reportRef.Report.ID)
		if err != nil {
			t.Fatal("current report source unavailable")
		}
		x.r, err = delegation.ParseReport([]byte(storedReport.Body))
		if err != nil {
			t.Fatal("actual report projection invalid")
		}
		index := -1
		for i, a := range f.args {
			if a.ContractSource.ID == x.c.ContractSource.ID {
				index = i
			}
		}
		if index < 0 || len(x.c.Sources) != 1 || x.c.Sources[0] != f.args[index].Sources[0] || len(x.c.Permissions) != 0 {
			t.Fatal("child contract context widened")
		}
		final, err := completion.New(f.pool).GetByRun(f.ctx, x.runID)
		if err != nil || final.Verdict != "unverified" || final.AnswerState != "final" || final.PrivacyRedacted {
			t.Fatal("native final became verification or was incomplete")
		}
		check := verifyNativeResearchObservation(final.Content, x.c.Sources[0], f.corpus.Sources[index].ExpectedValue, f.corpus.Sources[index].ExpectedDate) == nil
		v.ObjectiveChecks = append(v.ObjectiveChecks, check)
		v.ChildTaskIDs = append(v.ChildTaskIDs, x.taskID)
		v.ChildRunIDs = append(v.ChildRunIDs, x.runID)
		v.ChildResultIDs = append(v.ChildResultIDs, x.resultID)
		v.ReportSourceRefs = append(v.ReportSourceRefs, *x.reportRef.Report)
		if !check {
			t.Fatal("native source observation failed objective fixture; no retry")
		}
		if len(x.r.Accomplished) != 0 || len(x.r.Uncertainty) == 0 || x.r.ResultID != x.resultID {
			t.Fatal("report promoted unverified model output")
		}
		if _, err = admission.Manager.Decide(nativeOwner(f.ctx, "reject-unverified-"+x.taskID, root.SessionKey), root.TaskID, x.taskID, task.DelegationDecision{Accept: true, Report: *x.reportRef.Report, Evidence: []delegation.SourceRef{x.c.Sources[0]}}); err == nil {
			t.Fatal("numeric model agreement promoted unverified task result")
		}
	}
	parentFinal, err := completion.New(f.pool).GetByRun(f.ctx, v.ParentRunID)
	if err != nil || parentFinal.Verdict != "unverified" || parentFinal.PrivacyRedacted || verifyNativeAdmissionReceipts(parentFinal.Content, v.ChildTaskIDs) != nil {
		t.Fatal("native parent final failed exact receipt projection; no retry")
	}
	v.ParentReceiptCheck = true
	v.UnverifiedAcceptanceDenied = true
	if f.pool.QueryRow(f.ctx, `SELECT max(overlap) FROM (SELECT (SELECT count(*) FROM fixture_native_attempts a JOIN runs r USING(run_id) JOIN tasks t ON t.task_id=r.task_id JOIN run_final_results f USING(run_id) WHERE t.parent_task_id=$1 AND a.created_at<=p.created_at AND f.created_at>=p.created_at) overlap FROM fixture_native_attempts p JOIN runs r USING(run_id) JOIN tasks t ON t.task_id=r.task_id WHERE t.parent_task_id=$1) spans`, root.TaskID).Scan(&v.MaximumChildOverlap) != nil || v.MaximumChildOverlap != width {
		t.Fatal("actual child execution did not demonstrate configured width")
	}
	var raw []byte
	if f.pool.QueryRow(f.ctx, `SELECT used_json FROM execution_budget_scopes WHERE scope_id=$1`, budget.ScopeID("task", nativeFixtureOwner, root.TaskID)).Scan(&raw) != nil || json.Unmarshal(raw, &v.SharedRootReservations) != nil || v.SharedRootReservations.Children != 2 || v.SharedRootReservations.ModelTurns < 5 || v.SharedRootReservations.ModelTurns > 6 {
		t.Fatal("shared root conservative reservation evidence invalid")
	}
	current, err := f.tasks.Get(f.ctx, root.TaskID)
	if err != nil || current.Status != "active" {
		t.Fatal("parent completed from model consensus")
	}
	v.ParentTaskStillActive = true
	v.IndependentConflictKept = true
	v.ElapsedMillis = time.Since(started).Milliseconds()
	return v
}
