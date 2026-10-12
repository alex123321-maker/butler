package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"testing"
	"time"

	"github.com/butler/butler/internal/config"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/modelprovider"
	"github.com/butler/butler/internal/providerauth"
	"github.com/butler/butler/internal/transport"
	"github.com/butler/butler/internal/transport/antigravity"
	"github.com/jackc/pgx/v5/pgxpool"
)

// A concrete native provider is created only after promotion. The trusted task
// ID, current persisted initiative binding and the one declared session fence
// prevent this experiment's finite allowance becoming a general model route.
type initiativeHighProvider struct {
	*antigravity.Provider
	limit  *initiativeHighLimiter
	taskID string
	mu     sync.Mutex
	runID  string
}

func newInitiativeHighProvider(ctx context.Context, pool *pgxpool.Pool, taskID, importPath, keyPath string) (*initiativeHighProvider, error) {
	if taskID == "" || len(taskID) > 256 {
		return nil, errNativeFixtureDenied
	}
	limit, err := newInitiativeHighLimiter(ctx, pool)
	if err != nil {
		return nil, errNativeFixtureDenied
	}
	key, err := privateNativeFixtureKey(keyPath)
	if err != nil {
		return nil, errNativeFixtureDenied
	}
	store := config.NewPostgresSettingsStore(pool, config.WithSettingsCodec(config.StaticSecretValueCodec{Key: key}))
	manager := providerauth.NewManager(store)
	state, err := manager.State(ctx, modelprovider.ProviderAntigravity)
	if err != nil {
		return nil, errNativeFixtureDenied
	}
	if !state.Connected {
		file, err := privateNativeFixtureFile(importPath, 64*1024)
		if err != nil {
			return nil, errNativeFixtureDenied
		}
		err = manager.ImportAntigravity(ctx, file, "isolated-native-initiative-fixture")
		_ = file.Close()
		if err != nil {
			return nil, errNativeFixtureDenied
		}
	}
	provider, err := antigravity.NewProvider(antigravity.Config{Model: antigravity.FlashHighModel, Timeout: 2 * time.Minute, AuthSource: manager})
	if err != nil {
		return nil, errNativeFixtureDenied
	}
	return &initiativeHighProvider{Provider: provider, limit: limit, taskID: taskID}, nil
}

func (p *initiativeHighProvider) ownsRun(ctx context.Context, runID string) bool {
	if p == nil || p.limit == nil || p.limit.pool == nil || p.taskID == "" || runID == "" {
		return false
	}
	var owned bool
	err := p.limit.pool.QueryRow(ctx, `SELECT EXISTS (
	 SELECT 1 FROM runs r JOIN tasks t ON t.task_id=r.task_id
	 JOIN initiatives i ON i.task_id=t.task_id
	 JOIN initiative_sources s ON s.source_id=i.source_id
	 JOIN task_authority_bindings b ON b.task_id=t.task_id
	 JOIN owner_goals g ON g.goal_id=b.goal_id
	 JOIN action_mandates m ON m.mandate_id=b.mandate_id
	 WHERE r.run_id=$1 AND t.task_id=$2 AND t.owner_id='owner'
	 AND r.session_key='telegram:chat:424242' AND t.session_key=r.session_key
	 AND r.model_provider='antigravity' AND r.task_revision=t.revision
	 AND t.status='active' AND t.source_json->>'type'='initiative'
	 AND i.status='promoted' AND i.owner_id=t.owner_id AND i.source_revision=s.revision
	 AND s.enabled AND s.owner_id=t.owner_id AND s.session_key=t.session_key
	 AND b.owner_id=t.owner_id AND b.goal_id=i.goal_id AND b.mandate_id=i.mandate_id
	 AND g.owner_id=t.owner_id AND g.revision=b.goal_revision AND g.status='active'
	 AND g.expires_at>clock_timestamp() AND m.owner_id=t.owner_id
	 AND m.revision=b.mandate_revision AND m.status='active' AND m.revoked_at IS NULL
	 AND m.expires_at>clock_timestamp())`, runID, p.taskID).Scan(&owned)
	return err == nil && owned
}

func (p *initiativeHighProvider) StartRun(ctx context.Context, r transport.StartRunRequest) (transport.EventStream, error) {
	if p == nil || p.Provider == nil || r.Validate() != nil ||
		r.Context.ProviderName != antigravity.Name || r.Context.ModelName != antigravity.FlashHighModel ||
		r.Context.SessionKey != "telegram:chat:424242" || r.Context.ProviderSessionRef != nil ||
		len(r.TransportOptions) != 0 || r.TransportOptionsJSON != "" {
		return nil, errNativeFixtureDenied
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.runID != "" || !p.ownsRun(ctx, r.Context.RunID) || p.limit.reserve(ctx, "start", r.Context.RunID, r) != nil {
		return nil, errNativeFixtureDenied
	}
	// Retain the identity even if the provider fails; a failure grants no retry.
	p.runID = r.Context.RunID
	return p.Provider.StartRun(ctx, r)
}

func (p *initiativeHighProvider) ContinueRun(ctx context.Context, r transport.ContinueRunRequest) (transport.EventStream, error) {
	if p == nil || p.Provider == nil || r.Validate() != nil || len(r.TransportOptions) != 0 || r.TransportOptionsJSON != "" {
		return nil, errNativeFixtureDenied
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.runID != r.RunID || !p.ownsRun(ctx, r.RunID) || p.limit.reserve(ctx, "continue", r.RunID, r) != nil {
		return nil, errNativeFixtureDenied
	}
	return p.Provider.ContinueRun(ctx, r)
}

func (p *initiativeHighProvider) SubmitToolResult(ctx context.Context, r transport.SubmitToolResultRequest) (transport.EventStream, error) {
	if p == nil || p.Provider == nil || r.Validate() != nil || len(r.TransportOptions) != 0 || r.TransportOptionsJSON != "" {
		return nil, errNativeFixtureDenied
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.runID != r.RunID || !p.ownsRun(ctx, r.RunID) || p.limit.reserve(ctx, "tool_result", r.RunID, r) != nil {
		return nil, errNativeFixtureDenied
	}
	return p.Provider.SubmitToolResult(ctx, r)
}

// No credentials, provider requests, model final or raw tool observations are
// exported. NULL usage remains unknown, separately from conservative admission.
type initiativeHighEvidence struct {
	Status       string                   `json:"status"`
	Model        string                   `json:"model"`
	ReleaseRef   string                   `json:"release_ref"`
	Historical   nativeHistoricalEvidence `json:"historical"`
	NativeTotal  int                      `json:"native_attempts"`
	NewAttempts  int                      `json:"new_native_attempts"`
	MaxTotal     int                      `json:"maximum_total_attempts"`
	MaxNew       int                      `json:"maximum_new_attempts"`
	Attempts     []nativeAttemptEvidence  `json:"attempts"`
	Usage        []nativeUsageEvidence    `json:"usage"`
	TaskID       string                   `json:"promoted_task_id,omitempty"`
	RunID        string                   `json:"ordinary_run_id,omitempty"`
	CheapChecks  int                      `json:"actual_cheap_broker_checks"`
	CorePassed   bool                     `json:"ordinary_core_artifact_channel_acceptance"`
	ModelVerdict string                   `json:"model_final_verdict,omitempty"`
	TaskStatus   string                   `json:"owner_verified_task_status,omitempty"`
	DigestStatus string                   `json:"count_only_digest_status,omitempty"`
	DigestACK    string                   `json:"synthetic_channel_ack,omitempty"`
}

func TestInitiativeHighActualAcceptanceE2E(t *testing.T) {
	if os.Getenv("BUTLER_INITIATIVE_HIGH_ACCEPTANCE") != "1" {
		t.Skip("separately sealed native initiative window")
	}
	history, err := readInitiativeHighHistory(initiativeHighHistoryPaths())
	if err != nil || !regexp.MustCompile(`^[a-f0-9]{40}$`).MatchString(os.Getenv("BUTLER_INITIATIVE_HIGH_RELEASE")) {
		t.Fatal("native initiative source/history prerequisite denied")
	}
	report := initiativeHighEvidence{Status: "failed", Model: antigravity.FlashHighModel,
		ReleaseRef: os.Getenv("BUTLER_INITIATIVE_HIGH_RELEASE"), Historical: history,
		NativeTotal: 41, MaxTotal: 45, MaxNew: 4, Attempts: []nativeAttemptEvidence{}, Usage: []nativeUsageEvidence{}}
	// Register before the fixture cleanup, then register the database exporter
	// inside its factory. LIFO exports the journal before the fixture closes PG.
	t.Cleanup(func() {
		if !t.Failed() && report.CorePassed {
			report.Status = "passed"
		}
		raw, marshalErr := json.MarshalIndent(report, "", "  ")
		path := os.Getenv("BUTLER_INITIATIVE_HIGH_REPORT")
		if marshalErr != nil || !filepath.IsAbs(path) || os.WriteFile(path, append(raw, '\n'), 0600) != nil {
			t.Error("native initiative safe report persistence failed")
		}
	})
	runInitiativeProjectAcceptance(t, func(ctx context.Context, pool *pgxpool.Pool, taskID string) (transport.ModelProvider, error) {
		report.TaskID, report.CheapChecks = taskID, 3
		provider, err := newInitiativeHighProvider(ctx, pool, taskID,
			os.Getenv("BUTLER_INITIATIVE_HIGH_IMPORT_FILE"), filepath.Join(os.Getenv("TMPDIR"), "initiative-encrypted-settings-key"))
		if err != nil {
			return nil, err
		}
		t.Cleanup(func() {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			if pool.QueryRow(ctx, `SELECT attempts FROM fixture_initiative_native_limit WHERE singleton`).Scan(&report.NativeTotal) != nil {
				t.Error("native initiative counter unavailable")
				return
			}
			report.NewAttempts = report.NativeTotal - 41
			rows, err := pool.Query(ctx, `SELECT attempt,run_id,kind,input_hash,created_at FROM fixture_initiative_native_attempts ORDER BY attempt`)
			if err != nil {
				t.Error("native initiative attempts unavailable")
				return
			}
			for rows.Next() {
				var a nativeAttemptEvidence
				if rows.Scan(&a.Attempt, &a.RunID, &a.Kind, &a.InputHash, &a.CreatedAt) != nil {
					t.Error("native initiative attempt projection failed")
					break
				}
				report.Attempts = append(report.Attempts, a)
				report.RunID = a.RunID
			}
			rowErr := rows.Err()
			rows.Close()
			if rowErr != nil {
				t.Error("native initiative attempt projection incomplete")
			}
			rows, err = pool.Query(ctx, `SELECT a.run_id,a.reservation_id,a.actual_input_tokens,a.actual_output_tokens
			 FROM execution_model_attempts a WHERE EXISTS (SELECT 1 FROM fixture_initiative_native_attempts f WHERE f.run_id=a.run_id)
			 AND a.provider='antigravity' AND a.model='gemini-3.8-flash-high' ORDER BY a.created_at,a.operation_id`)
			if err != nil {
				t.Error("native initiative usage unavailable")
				return
			}
			for rows.Next() {
				var u nativeUsageEvidence
				if rows.Scan(&u.RunID, &u.ReservationID, &u.ActualInputTokens, &u.ActualOutputTokens) != nil {
					t.Error("native initiative usage projection failed")
					break
				}
				report.Usage = append(report.Usage, u)
			}
			rowErr = rows.Err()
			rows.Close()
			if rowErr != nil {
				t.Error("native initiative usage projection incomplete")
			}
			_ = pool.QueryRow(ctx, `SELECT status FROM tasks WHERE task_id=$1`, taskID).Scan(&report.TaskStatus)
			_ = pool.QueryRow(ctx, `SELECT verdict FROM run_final_results WHERE run_id=$1`, report.RunID).Scan(&report.ModelVerdict)
			_ = pool.QueryRow(ctx, `SELECT status,channel_ack FROM initiative_digest_outbox`).Scan(&report.DigestStatus, &report.DigestACK)
			if report.CorePassed && (report.NewAttempts != 4 || len(report.Attempts) != 4 || len(report.Usage) != 4 ||
				report.TaskStatus != "completed" || report.ModelVerdict != "unverified" || report.DigestStatus != "sent" || report.DigestACK != "101") {
				t.Error("native initiative journal and acceptance projections differ")
			}
		})
		return provider, nil
	}, antigravity.Name, antigravity.FlashHighModel)
	report.CorePassed, report.CheapChecks = true, 3
}

func TestInitiativeHighProviderRejectsUnownedRunIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	ctx := context.Background()
	limit, err := newInitiativeHighLimiter(ctx, pool)
	if err != nil {
		t.Fatal("native initiative counter unavailable")
	}
	p := &initiativeHighProvider{Provider: new(antigravity.Provider), limit: limit, taskID: "promoted-task"}
	request := transport.StartRunRequest{Context: transport.TransportRunContext{RunID: "unowned", SessionKey: "telegram:chat:424242", ProviderName: antigravity.Name, ModelName: antigravity.FlashHighModel}}
	if _, err = p.StartRun(ctx, request); err != errNativeFixtureDenied {
		t.Fatal("unowned task/run entered native provider")
	}
	request.Context.ModelName = antigravity.DefaultModel
	if _, err = p.StartRun(ctx, request); err != errNativeFixtureDenied {
		t.Fatal("undeclared profile entered native provider")
	}
	p.runID = "unowned"
	if _, err = p.ContinueRun(ctx, transport.ContinueRunRequest{RunID: "unowned"}); err != errNativeFixtureDenied {
		t.Fatal("continuation without current owning provenance accepted")
	}
	if _, err = p.SubmitToolResult(ctx, transport.SubmitToolResultRequest{RunID: "unowned", ToolCallRef: "call", ToolResultJSON: `{}`}); err != errNativeFixtureDenied {
		t.Fatal("tool result without current owning provenance accepted")
	}
	var count, entries int
	if pool.QueryRow(ctx, `SELECT attempts FROM fixture_initiative_native_limit`).Scan(&count) != nil ||
		pool.QueryRow(ctx, `SELECT count(*) FROM fixture_initiative_native_attempts`).Scan(&entries) != nil || count != 41 || entries != 0 {
		t.Fatal("routing denial spent native allowance")
	}
}
