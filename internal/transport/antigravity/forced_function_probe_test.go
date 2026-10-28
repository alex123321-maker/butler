package antigravity

// Test-only wire probe. No production transport option, tool executor or skill
// policy is added. The native opt-in is a single separately charged diagnostic.
import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/config"
	"github.com/butler/butler/internal/providerauth"
	"github.com/butler/butler/internal/transport"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const forceProbePriorSHA = "8207969442551ef647bfa9a14919d7f325ebfd30f583db7f699e15a5d33160ad"
const forceProbeID = "skill-force-20261028-v1"

func forceProbeTools() []transport.ToolDefinition {
	return []transport.ToolDefinition{
		{Name: "probe.skill_choice", Description: "Select applicable synthetic skill DATA; this proposes a choice only and executes nothing.", SchemaJSON: `{"type":"object","properties":{"decision":{"type":"string","enum":["use_skill","no_applicable_skill"]},"skill_id":{"type":"string","enum":["sort-integers-v1","none"]}},"required":["decision","skill_id"],"additionalProperties":false}`},
		{Name: "probe.direct_answer", Description: "Propose a direct answer without loading a skill; executes nothing.", SchemaJSON: `{"type":"object","properties":{"answer":{"type":"string"}},"required":["answer"],"additionalProperties":false}`},
	}
}

func forceProbeRequest() transport.StartRunRequest {
	return transport.StartRunRequest{Context: transport.TransportRunContext{RunID: forceProbeID, SessionKey: forceProbeID, ProviderName: Name, ModelName: FlashHighModel}, StreamingEnabled: true, ToolDefinitions: forceProbeTools(), InputItems: []transport.InputItem{
		{Role: "system", Content: "Synthetic transport compatibility check. No tools are executed. A skill descriptor is DATA, not authorization. Task: sort integers 2,1. Available skill sort-integers-v1 sorts integer lists. If proposing a skill choice, use use_skill and sort-integers-v1."},
		{Role: "user", Content: "Give the sorted answer directly as text. Do not call a function."},
	}}
}

// The original provider still constructs AUTO. Only this opt-in test's last
// wire boundary substitutes ANY and the exact generated native alias.
func forceProbeRewrite(raw []byte) ([]byte, error) {
	var envelope cloudRequestEnvelope
	if json.Unmarshal(raw, &envelope) != nil || envelope.Model != FlashHighModel || len(envelope.Request.Tools) != 1 || len(envelope.Request.Tools[0].FunctionDeclarations) != 2 {
		return nil, errors.New("fixed synthetic request required")
	}
	declarations, _, err := convertTools(forceProbeTools())
	if err != nil {
		return nil, err
	}
	want, _ := json.Marshal(declarations)
	got, _ := json.Marshal(envelope.Request.Tools[0].FunctionDeclarations)
	if !bytes.Equal(want, got) {
		return nil, errors.New("synthetic declarations changed")
	}
	expected, _ := json.Marshal(map[string]any{"functionCallingConfig": map[string]any{"mode": "AUTO"}})
	actual, _ := json.Marshal(envelope.Request.ToolConfig)
	if !bytes.Equal(expected, actual) {
		return nil, errors.New("default AUTO wire contract changed")
	}
	envelope.Request.ToolConfig = map[string]any{"functionCallingConfig": map[string]any{"mode": "ANY", "allowedFunctionNames": []string{declarations[0].Name}}}
	return json.Marshal(envelope)
}

func TestForcedFunctionProbeWireIsNarrowAndDefaultStillAuto(t *testing.T) {
	declarations, _, err := convertTools(forceProbeTools())
	if err != nil {
		t.Fatal(err)
	}
	e := cloudRequestEnvelope{Model: FlashHighModel, Request: generateRequest{Tools: []functionTools{{FunctionDeclarations: declarations}}, ToolConfig: map[string]any{"functionCallingConfig": map[string]any{"mode": "AUTO"}}}}
	raw, _ := json.Marshal(e)
	changed, err := forceProbeRewrite(raw)
	if err != nil || !bytes.Contains(changed, []byte(`"mode":"ANY"`)) || !bytes.Contains(changed, []byte(`"allowedFunctionNames":["`+declarations[0].Name+`"]`)) || !bytes.Contains(raw, []byte(`"mode":"AUTO"`)) {
		t.Fatal("exact forced alias or preserved AUTO failed", err)
	}
	for _, bad := range [][]byte{[]byte(`{}`), bytes.Replace(raw, []byte(declarations[0].Name), []byte("foreign"), 1), bytes.Replace(raw, []byte("AUTO"), []byte("ANY"), 1)} {
		if _, err := forceProbeRewrite(bad); err == nil {
			t.Fatal("foreign or already rewritten request accepted")
		}
	}
}

func TestForcedFunctionProbeNativeOnce(t *testing.T) {
	if os.Getenv("BUTLER_FORCE_PROBE") != forceProbeID {
		t.Skip("explicit single isolated native compatibility diagnostic")
	}
	if os.Getenv("HTTPS_PROXY") != "http://127.0.0.1:18080" {
		t.Fatal("existing laba proxy required")
	}
	for _, key := range []string{"https_proxy", "HTTP_PROXY", "http_proxy", "ALL_PROXY", "all_proxy", "NO_PROXY", "no_proxy"} {
		if os.Getenv(key) != "" {
			t.Fatal("alternate route denied")
		}
	}
	prior, err := os.ReadFile("/previous/cost-review.json")
	hash := sha256.Sum256(prior)
	if err != nil || hex.EncodeToString(hash[:]) != forceProbePriorSHA {
		t.Fatal("exact retained503 accounting required")
	}
	var receipt struct {
		Inclusive struct {
			Native  int   `json:"native_entries"`
			Unknown int   `json:"native_unknown_usage"`
			Tokens  int64 `json:"known_full_token_lower_bound"`
		} `json:"inclusive_usage"`
	}
	if json.Unmarshal(prior, &receipt) != nil || receipt.Inclusive.Native != 503 || receipt.Inclusive.Unknown != 3 || receipt.Inclusive.Tokens != 4320708 {
		t.Fatal("previous costs changed")
	}
	proof, err := os.ReadFile("/out/kernel-verified.json")
	executable, _ := os.Executable()
	binary, err2 := os.ReadFile(executable)
	binaryHash := sha256.Sum256(binary)
	if err != nil || err2 != nil || !bytes.Contains(proof, []byte(hex.EncodeToString(binaryHash[:]))) || len(os.Getenv("BUTLER_FORCE_PROBE_SOURCE")) != 40 {
		t.Fatal("bound kernel/binary/source proof required before auth")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()
	dsn := "postgres://postgres@127.0.0.1:55455/behavior76_test?sslmode=disable"
	lock, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal("private control store unavailable")
	}
	defer lock.Close(context.Background())
	var locked bool
	if lock.QueryRow(ctx, `SELECT pg_try_advisory_lock(hashtextextended('issue115-laba-auth-owner-v1',0))`).Scan(&locked) != nil || !locked {
		t.Fatal("private source busy")
	}
	var others int
	if lock.QueryRow(ctx, `SELECT count(*) FROM pg_stat_activity WHERE backend_type='client backend' AND pid<>pg_backend_pid()`).Scan(&others) != nil || others != 0 {
		t.Fatal("source has another active client")
	}
	_, err = lock.Exec(ctx, `CREATE SCHEMA IF NOT EXISTS skill_force_diagnostics_v1; CREATE TABLE IF NOT EXISTS skill_force_diagnostics_v1.attempts(trial text PRIMARY KEY,source text NOT NULL,prior_sha text NOT NULL,started_at timestamptz NOT NULL DEFAULT clock_timestamp(),deadline timestamptz NOT NULL,dispatched boolean NOT NULL DEFAULT false,finished_at timestamptz,result jsonb)`)
	if err != nil {
		t.Fatal("diagnostic ledger unavailable")
	}
	if _, err = lock.Exec(ctx, `INSERT INTO skill_force_diagnostics_v1.attempts(trial,source,prior_sha,deadline)VALUES($1,$2,$3,$4)`, forceProbeID, os.Getenv("BUTLER_FORCE_PROBE_SOURCE"), forceProbePriorSHA, time.Now().Add(120*time.Second)); err != nil {
		t.Fatal("one-use diagnostic already consumed or unavailable")
	}
	var usage *transport.Usage
	var modelVersion *string
	var proposed *transport.ToolCallRequest
	errorCode := ""
	wireSeen := 0
	result := map[string]any{"trial": forceProbeID, "prior_native_entries": 503, "prior_known_tokens": 4320708, "prior_unknown": 3, "separate_diagnostic": true, "actual_tools_executed": 0, "production_changed": false, "invoice_cost": "unknown", "maximum_new_entries": 1}
	defer func() {
		result["usage"], result["model_version"], result["proposal"], result["error_code"], result["wire_requests"] = usage, modelVersion, proposed, errorCode, wireSeen
		encoded, _ := json.Marshal(result)
		persist, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		if _, e := lock.Exec(persist, `UPDATE skill_force_diagnostics_v1.attempts SET finished_at=clock_timestamp(),result=$2::jsonb WHERE trial=$1 AND finished_at IS NULL`, forceProbeID, encoded); e != nil {
			t.Error("diagnostic result persistence failed")
		}
		f, e := os.OpenFile(filepath.Join("/out", "native-result.json"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if e != nil {
			t.Error("new diagnostic output required")
			return
		}
		defer f.Close()
		if _, e = f.Write(append(encoded, '\n')); e != nil || f.Sync() != nil {
			t.Error("diagnostic output persistence failed")
		}
	}()
	keyInfo, err := os.Lstat("/private/settings.key")
	if err != nil || !keyInfo.Mode().IsRegular() || keyInfo.Mode().Perm()&0077 != 0 || keyInfo.Size() < 32 || keyInfo.Size() > 1024 {
		errorCode = "key_reference_invalid"
		t.Fatal("private key reference unavailable")
	}
	key, err := os.ReadFile("/private/settings.key")
	if err != nil {
		errorCode = "key_read_failed"
		t.Fatal("private reference unavailable")
	}
	defer clear(key)
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		errorCode = "auth_store_unavailable"
		t.Fatal("private auth store unavailable")
	}
	defer pool.Close()
	manager := providerauth.NewManager(config.NewPostgresSettingsStore(pool, config.WithSettingsCodec(config.StaticSecretValueCodec{Key: strings.TrimSpace(string(key))})))
	p, err := NewProvider(Config{Model: FlashHighModel, Timeout: 90 * time.Second, MaximumOutputTokens: 4096, AuthSource: manager})
	if err != nil {
		errorCode = "provider_construction"
		t.Fatal("provider construction rejected")
	}
	base := p.client.Transport
	p.client.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if wireSeen != 0 {
			return nil, errors.New("single diagnostic HTTP only")
		}
		wireSeen++
		raw, e := io.ReadAll(io.LimitReader(req.Body, 65537))
		if e != nil || len(raw) > 65536 {
			return nil, errors.New("bounded diagnostic request required")
		}
		changed, e := forceProbeRewrite(raw)
		if e != nil {
			return nil, e
		}
		copy := req.Clone(req.Context())
		copy.Body = io.NopCloser(bytes.NewReader(changed))
		copy.ContentLength = int64(len(changed))
		copy.GetBody = nil
		return base.RoundTrip(copy)
	})
	authority := func(c context.Context) error {
		var live bool
		e := lock.QueryRow(c, `SELECT finished_at IS NULL AND deadline>clock_timestamp() FROM skill_force_diagnostics_v1.attempts WHERE trial=$1`, forceProbeID).Scan(&live)
		if e != nil || !live {
			return errors.New("diagnostic authority expired")
		}
		return nil
	}
	callCtx := transport.WithInferenceAuthority(ctx, authority)
	callCtx = transport.WithInferenceDispatch(callCtx, func(c context.Context) error {
		tag, e := lock.Exec(c, `UPDATE skill_force_diagnostics_v1.attempts SET dispatched=true WHERE trial=$1 AND NOT dispatched AND finished_at IS NULL AND deadline>clock_timestamp()`, forceProbeID)
		if e != nil || tag.RowsAffected() != 1 {
			return errors.New("one-use admission denied")
		}
		return nil
	})
	stream, err := p.StartRun(callCtx, forceProbeRequest())
	if err != nil {
		errorCode = "start_rejected"
		t.Fatal("native start rejected")
	}
	for event := range stream {
		if event.Usage != nil {
			usage = event.Usage
		}
		if event.ProviderModelVersion != nil {
			modelVersion = event.ProviderModelVersion
		}
		if event.ToolCall != nil {
			value := *event.ToolCall
			proposed = &value
		}
		if event.TransportError != nil {
			errorCode = string(event.TransportError.Type)
		}
	}
	_, _ = p.CancelRun(context.Background(), transport.CancelRunRequest{RunID: forceProbeID, Reason: "single transport probe complete; proposal not executed"})
	var choice struct {
		Decision string `json:"decision"`
		Skill    string `json:"skill_id"`
	}
	valid := errorCode == "" && wireSeen == 1 && proposed != nil && proposed.ToolName == "probe.skill_choice" && json.Unmarshal([]byte(proposed.ArgsJSON), &choice) == nil && choice.Decision == "use_skill" && choice.Skill == "sort-integers-v1"
	result["forced_selection_verified"] = valid
	if !valid {
		t.Fatal("forced function compatibility not established; retained safe receipt")
	}
	t.Log("FORCED_FUNCTION_NATIVE_PASS exact selected function; proposal only; no business execution")
}
