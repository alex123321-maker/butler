package antigravity

// Separately frozen first-decision diagnostic on a historical synthetic input.
// The original native115 operator is never admitted or replayed. No proposed
// function is executed, including skills.load, and no product effect is claimed.
import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/config"
	"github.com/butler/butler/internal/providerauth"
	securecontent "github.com/butler/butler/internal/security/content"
	"github.com/butler/butler/internal/transport"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const contextProbeID = "skill-context-20261029-v1"
const contextRequestSHA = "894345aeccaa40dd352dddb37a6c1f04867e06f0c59bad18409a8ba44ea35c5f"
const contextManifestSHA = "5bfa834449648ad58b7fadaf89131b111db2e98c1d553bbf1dbac514778b9597"
const contextPilotSHA = "1929bff06e9ce83d5820ae6511424bdb1dc1c9bb5aff928006e97a99e20d0f5c"
const contextSkillDigest = "96f7e2ad1581d180dda1123a53d9e7ffc091bfec4cc2b7e123a74691c633adb5"

func contextRead(path, sha string) ([]byte, error) {
	b, e := os.ReadFile(path)
	h := sha256.Sum256(b)
	if e != nil || hex.EncodeToString(h[:]) != sha {
		return nil, errors.New("fixed fixture hash mismatch")
	}
	return b, nil
}
func contextRequest(raw, manifest []byte, arm string) (transport.StartRunRequest, error) {
	var req transport.StartRunRequest
	if json.Unmarshal(raw, &req) != nil {
		return req, errors.New("invalid frozen input")
	}
	if len(req.ToolDefinitions) != 9 || !json.Valid(manifest) {
		return req, errors.New("fixed tools and body required")
	}
	id := contextProbeID + "-" + arm
	req.Context = transport.TransportRunContext{RunID: id, SessionKey: id, ProviderName: Name, ModelName: FlashHighModel}
	if arm == "explicit_guidance" {
		req.InputItems = append(req.InputItems, transport.InputItem{Role: "system", Content: promptGuidance})
	}
	if arm == "preloaded_body" {
		item := securecontent.Data(map[string]any{"procedure": json.RawMessage(manifest), "digest": contextSkillDigest}, securecontent.Origin{Kind: securecontent.Document, Sensitivity: securecontent.Private, SourceRefs: []string{"synthetic:historical-reviewed-skill"}})
		n := len(req.InputItems)
		if n == 0 {
			return req, errors.New("missing owner input")
		}
		last := req.InputItems[n-1]
		req.InputItems = append(req.InputItems[:n-1], item, last)
	}
	if arm != "catalog_only" && arm != "explicit_guidance" && arm != "preloaded_body" {
		return req, errors.New("unknown arm")
	}
	return req, req.Validate()
}
func contextPlan() map[string]any {
	return map[string]any{"trial": contextProbeID, "arms": []string{"catalog_only", "explicit_guidance", "preloaded_body"}, "request_sha256": contextRequestSHA, "manifest_sha256": contextManifestSHA, "previous_pilot_sha256": contextPilotSHA, "source": "historical synthetic closed ABC6 initial C/control-01 request; tool definitions and input DATA preserved; fresh inference identity", "profile": FlashHighModel, "mode": "AUTO", "maximum_native": 3, "maximum_output_tokens": 4096, "request_seconds": 75, "window_seconds": 300, "guidance": promptGuidance, "measure": "First proposed tool only; every proposal is cancelled without execution", "limits": "One sample per arm; no full task, current source authority, skill load, production readiness or intelligence comparison is established; added preloaded body is procedural DATA; old closed histories and all diagnostics remain separate", "stop": "Any transport, admission, wire, accounting or storage error stops all. No retry, resume or fallback."}
}

func TestSkillContextProbeFixture(t *testing.T) {
	if os.Getenv("BUTLER_CONTEXT_OFFLINE") != "1" {
		t.Skip("frozen historical fixture mounted only on laba")
	}
	raw, e := contextRead("/probe/fixture/request.json", contextRequestSHA)
	if e != nil {
		t.Fatal(e)
	}
	manifest, e := contextRead("/probe/fixture/manifest.json", contextManifestSHA)
	if e != nil {
		t.Fatal(e)
	}
	for _, arm := range []string{"catalog_only", "explicit_guidance", "preloaded_body"} {
		req, e := contextRequest(raw, manifest, arm)
		if e != nil {
			t.Fatal(e)
		}
		if len(req.ToolDefinitions) != 9 {
			t.Fatal("tool set changed")
		}
		if arm == "catalog_only" {
			var old transport.StartRunRequest
			_ = json.Unmarshal(raw, &old)
			a, _ := json.Marshal(req.InputItems)
			b, _ := json.Marshal(old.InputItems)
			if !bytes.Equal(a, b) {
				t.Fatal("baseline context changed")
			}
		}
	}
	if _, e = contextRequest(raw, manifest, "foreign"); e == nil {
		t.Fatal("foreign arm accepted")
	}
	if e = promptWrite("/probe/plan.json", contextPlan()); e != nil {
		t.Fatal(e)
	}
}

func TestSkillContextProbeNative(t *testing.T) {
	if os.Getenv("BUTLER_CONTEXT_PROBE") != contextProbeID {
		t.Skip("explicit separately frozen first-decision diagnostic")
	}
	if os.Getenv("HTTPS_PROXY") != "http://127.0.0.1:18080" {
		t.Fatal("existing proxy required")
	}
	for _, k := range []string{"https_proxy", "HTTP_PROXY", "http_proxy", "ALL_PROXY", "all_proxy", "NO_PROXY", "no_proxy"} {
		if os.Getenv(k) != "" {
			t.Fatal("alternate route denied")
		}
	}
	for path, sha := range map[string]string{"/previous/cost-review.json": promptPriorSHA, "/previous/forced-native-result.json": promptForceReceiptSHA, "/previous/prompt-result.json": contextPilotSHA} {
		if _, e := contextRead(path, sha); e != nil {
			t.Fatal("previous receipt changed")
		}
	}
	raw, e := contextRead("/fixture/request.json", contextRequestSHA)
	if e != nil {
		t.Fatal(e)
	}
	manifest, e := contextRead("/fixture/manifest.json", contextManifestSHA)
	if e != nil {
		t.Fatal(e)
	}
	plan, e := os.ReadFile("/plan.json")
	expected, _ := json.MarshalIndent(contextPlan(), "", "  ")
	if e != nil || !bytes.Equal(bytes.TrimSpace(plan), expected) {
		t.Fatal("plan changed")
	}
	proof, e := os.ReadFile("/out/kernel-verified.json")
	executable, _ := os.Executable()
	binary, e2 := os.ReadFile(executable)
	bh := sha256.Sum256(binary)
	source := os.Getenv("BUTLER_CONTEXT_SOURCE")
	if e != nil || e2 != nil || !bytes.Contains(proof, []byte(hex.EncodeToString(bh[:]))) || len(source) != 40 {
		t.Fatal("bound containment required before auth")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()
	dsn := "postgres://postgres@127.0.0.1:55455/behavior76_test?sslmode=disable"
	control, e := pgx.Connect(ctx, dsn)
	if e != nil {
		t.Fatal("control unavailable")
	}
	defer control.Close(context.Background())
	var locked bool
	if control.QueryRow(ctx, `SELECT pg_try_advisory_lock(hashtextextended('issue115-laba-auth-owner-v1',0))`).Scan(&locked) != nil || !locked {
		t.Fatal("source busy")
	}
	var others int
	if control.QueryRow(ctx, `SELECT count(*) FROM pg_stat_activity WHERE backend_type='client backend' AND pid<>pg_backend_pid()`).Scan(&others) != nil || others != 0 {
		t.Fatal("other auth client")
	}
	_, e = control.Exec(ctx, `CREATE SCHEMA IF NOT EXISTS skill_context_diagnostics_v1; CREATE TABLE IF NOT EXISTS skill_context_diagnostics_v1.windows(trial text PRIMARY KEY,source text NOT NULL,plan jsonb NOT NULL,deadline timestamptz NOT NULL,admitted integer NOT NULL DEFAULT 0,finished_at timestamptz,result jsonb); CREATE TABLE IF NOT EXISTS skill_context_diagnostics_v1.turns(trial text NOT NULL,seq integer NOT NULL,arm text NOT NULL,body jsonb,PRIMARY KEY(trial,seq))`)
	if e != nil {
		t.Fatal("ledger unavailable")
	}
	_, e = control.Exec(ctx, `INSERT INTO skill_context_diagnostics_v1.windows(trial,source,plan,deadline)VALUES($1,$2,$3,$4)`, contextProbeID, source, plan, time.Now().Add(300*time.Second))
	if e != nil {
		t.Fatal("one-use diagnostic consumed or unavailable")
	}
	var turns []promptTurn
	defer func() {
		report := map[string]any{"trial": contextProbeID, "source": source, "turns": turns, "separate_diagnostic": true, "actual_tools_executed": 0, "production_changed": false, "invoice_cost": "unknown", "previous_pilot_native": 16, "previous_pilot_sha256": contextPilotSHA, "prior_abc_native": 503}
		encoded, _ := json.Marshal(report)
		persist, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		if _, err := control.Exec(persist, `UPDATE skill_context_diagnostics_v1.windows SET finished_at=clock_timestamp(),result=$2 WHERE trial=$1 AND finished_at IS NULL`, contextProbeID, encoded); err != nil {
			t.Error("final ledger retention failed")
		}
		if err := promptWrite("/out/result.json", report); err != nil {
			t.Error("final receipt retention failed")
		}
	}()
	info, e := os.Lstat("/private/settings.key")
	if e != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() < 32 || info.Size() > 1024 {
		t.Fatal("private reference invalid")
	}
	key, e := os.ReadFile("/private/settings.key")
	if e != nil {
		t.Fatal("private reference unavailable")
	}
	defer clear(key)
	pool, e := pgxpool.New(ctx, dsn)
	if e != nil {
		t.Fatal("auth store unavailable")
	}
	defer pool.Close()
	manager := providerauth.NewManager(config.NewPostgresSettingsStore(pool, config.WithSettingsCodec(config.StaticSecretValueCodec{Key: strings.TrimSpace(string(key))})))
	p, e := NewProvider(Config{Model: FlashHighModel, Timeout: 75 * time.Second, MaximumOutputTokens: 4096, AuthSource: manager})
	if e != nil {
		t.Fatal("provider rejected")
	}
	base := p.client.Transport
	wireCount := 0
	wireAUTO := false
	baseline, _ := contextRequest(raw, manifest, "catalog_only")
	declarations, _, _ := convertTools(baseline.ToolDefinitions)
	want, _ := json.Marshal([]functionTools{{FunctionDeclarations: declarations}})
	p.client.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		body, e := io.ReadAll(io.LimitReader(req.Body, 65537))
		if e != nil || len(body) > 65536 {
			return nil, errors.New("bounded historical fixture required")
		}
		var envelope cloudRequestEnvelope
		if json.Unmarshal(body, &envelope) != nil || envelope.Model != FlashHighModel {
			return nil, errors.New("fixed model required")
		}
		cfg, _ := json.Marshal(envelope.Request.ToolConfig)
		wireAUTO = string(cfg) == `{"functionCallingConfig":{"mode":"AUTO"}}`
		tools, _ := json.Marshal(envelope.Request.Tools)
		if !wireAUTO || !bytes.Equal(tools, want) {
			return nil, errors.New("AUTO or tools changed")
		}
		wireCount++
		if wireCount > 3 {
			return nil, errors.New("three requests only")
		}
		copy := req.Clone(req.Context())
		copy.Body = io.NopCloser(bytes.NewReader(body))
		return base.RoundTrip(copy)
	})
	admitted := 0
	currentArm := ""
	authority := func(c context.Context) error {
		var live bool
		e := control.QueryRow(c, `SELECT finished_at IS NULL AND deadline>clock_timestamp() AND admitted<3 FROM skill_context_diagnostics_v1.windows WHERE trial=$1`, contextProbeID).Scan(&live)
		if e != nil || !live {
			return errors.New("authority expired")
		}
		return nil
	}
	callCtx := transport.WithInferenceAuthority(ctx, authority)
	callCtx = transport.WithInferenceDispatch(callCtx, func(c context.Context) error {
		tx, e := control.Begin(c)
		if e != nil {
			return e
		}
		defer tx.Rollback(context.Background())
		if e = tx.QueryRow(c, `UPDATE skill_context_diagnostics_v1.windows SET admitted=admitted+1 WHERE trial=$1 AND finished_at IS NULL AND deadline>clock_timestamp() AND admitted<3 RETURNING admitted`, contextProbeID).Scan(&admitted); e != nil {
			return errors.New("dispatch denied")
		}
		if _, e = tx.Exec(c, `INSERT INTO skill_context_diagnostics_v1.turns(trial,seq,arm)VALUES($1,$2,$3)`, contextProbeID, admitted, currentArm); e != nil {
			return e
		}
		return tx.Commit(c)
	})
	for i, arm := range []string{"catalog_only", "explicit_guidance", "preloaded_body"} {
		currentArm = arm
		request, e := contextRequest(raw, manifest, arm)
		if e != nil {
			t.Fatal(e)
		}
		if e = promptWrite(filepath.Join("/out", fmt.Sprintf("request-%d.json", i+1)), request); e != nil {
			t.Fatal("request retention failed")
		}
		observation := promptTurn{Sequence: i + 1, Slot: i + 1, Turn: 1, Started: time.Now().UTC()}
		wireAUTO = false
		stream, err := p.StartRun(callCtx, request)
		if err != nil {
			observation.ErrorCode = "start_rejected"
		} else {
			for event := range stream {
				if event.Usage != nil {
					observation.Usage = event.Usage
				}
				if event.ProviderModelVersion != nil {
					observation.ModelVersion = event.ProviderModelVersion
				}
				if event.ToolCall != nil {
					v := *event.ToolCall
					observation.Proposal = &v
				}
				if event.AssistantFinal != nil {
					v := event.AssistantFinal.Content
					observation.Final = &v
					if event.AssistantFinal.Usage != nil {
						observation.Usage = event.AssistantFinal.Usage
					}
				}
				if event.TransportError != nil {
					observation.ErrorCode = string(event.TransportError.Type)
				}
			}
		}
		observation.ElapsedMillis = time.Since(observation.Started).Milliseconds()
		observation.WireAUTO = wireAUTO
		turns = append(turns, observation)
		_, _ = p.CancelRun(context.Background(), transport.CancelRunRequest{RunID: request.Context.RunID, Reason: "proposal-only diagnostic; never execute"})
		encoded, _ := json.Marshal(observation)
		if admitted != i+1 {
			t.Fatal("dispatch not accounted")
		}
		if _, e = control.Exec(ctx, `UPDATE skill_context_diagnostics_v1.turns SET body=$3 WHERE trial=$1 AND seq=$2 AND body IS NULL`, contextProbeID, admitted, encoded); e != nil {
			t.Fatal("ledger retention failed")
		}
		if e = promptWrite(filepath.Join("/out", fmt.Sprintf("turn-%d.json", i+1)), observation); e != nil {
			t.Fatal("turn retention failed")
		}
		if observation.ErrorCode != "" || observation.Usage == nil || !wireAUTO {
			t.Fatal("native error; stop all without retry")
		}
		tool := "text_final"
		if observation.Proposal != nil {
			tool = observation.Proposal.ToolName
		}
		t.Logf("arm=%s first_proposal=%s full_tokens=%d", arm, tool, observation.Usage.TotalTokens)
	}
	if admitted != 3 || wireCount != 3 || len(turns) != 3 {
		t.Fatal("incomplete diagnostic")
	}
}
