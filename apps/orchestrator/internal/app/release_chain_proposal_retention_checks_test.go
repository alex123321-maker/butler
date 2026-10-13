package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/generatedtools"
	"github.com/butler/butler/internal/transport"
	"github.com/butler/butler/internal/transport/antigravity"
)

func TestReleaseChainV8ProductActionsRetainArgumentHashIntegration(t *testing.T) {
	f := newDeveloperFixture(t)
	f.channel = &releaseChainChannel{}
	f.projectDirectory = t.TempDir()
	_, prepared, _ := f.dispatch(t, f.parentRun, "converters.discover", map[string]any{"source_ref": f.needRef})
	w, err := createReleaseChainV5Window(f.f.ctx, f.f.pool, strings.Repeat("e", 64), strings.Repeat("f", 64), 7, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	w.policy = releaseChainV8Policy()
	if recordReleaseChainV5ProductEvidence(f.f.ctx, w, f, "explicit_absence", true, nil) != nil {
		t.Fatal("owning product evidence unavailable")
	}
	var actual string
	if f.f.pool.QueryRow(f.f.ctx, `SELECT payload_json->'postgres'->'actions'->0->>'args_hash' FROM `+w.table("fixture_chain_v5_high_case_evidence")).Scan(&actual) != nil || actual != prepared.Binding.ArgsHash || len(actual) != 64 {
		t.Fatal("retained argument fingerprint differs from actual Broker binding")
	}
}

func TestReleaseChainV8ProposalExactBytesAndNoMetadataLeak(t *testing.T) {
	dir := t.TempDir()
	marker := "PRIVATE_SYNTHETIC_ARGUMENT_VALUE"
	call := transport.ToolCallRequest{ToolCallRef: "native-synthetic-ref", ToolName: generatedtools.DeveloperAdmissionTool, SequenceNo: 4, ArgsJSON: ` { "unexpected_PRIVATE_KEY": "` + marker + `", "deadline":"not_a_timestamp", "budget":{"tokens":1.0} } `}
	sink := releaseChainProposalSinkFor(true, dir, "normal_sent")
	if err := sink.retain(542, "submit", "synthetic-run", &call); err != nil {
		t.Fatal("private proposal not retained", err)
	}
	path := filepath.Join(dir, "tool-proposal-000542.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var proof releaseChainPrivateProposal
	if json.Unmarshal(raw, &proof) != nil || !reflect.DeepEqual(proof.ToolCall, call) || string(proof.ArgumentsBytes) != call.ArgsJSON {
		t.Fatal("proposal bytes or identity changed")
	}
	info, _ := os.Stat(path)
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
		t.Fatal("proposal was not private")
	}
	meta, err := os.ReadFile(filepath.Join(dir, "tool-proposal-000542.meta.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(meta), marker) || strings.Contains(string(meta), "unexpected_PRIVATE_KEY") || strings.Contains(string(meta), "not_a_timestamp") || strings.Contains(string(meta), "synthetic-run") || strings.Contains(string(meta), call.ToolCallRef) {
		t.Fatal("original values entered public-safe metadata")
	}
	var decoded map[string]any
	if json.Unmarshal(meta, &decoded) != nil || decoded["developer_decode_status"] != "rejected" || decoded["unexpected_key_count"] != float64(1) || decoded["deadline_rfc3339"] != false {
		t.Fatal("diagnostic shapes not actual")
	}
	if sink.retain(542, "submit", "synthetic-run", &call) == nil {
		t.Fatal("existing private proof overwritten")
	}
	after, _ := os.ReadFile(path)
	if string(raw) != string(after) {
		t.Fatal("old proposal changed")
	}
	// Invalid UTF8 cannot be silently repaired by the outer JSON encoder. Its
	// original bytes are still preserved independently by the base64 byte field.
	call.ArgsJSON = string([]byte{'{', '"', 'x', '"', ':', '"', 0xff, '"', '}'})
	if sink.retain(543, "start", "synthetic-run", &call) != nil {
		t.Fatal("original byte retention failed")
	}
	raw, _ = os.ReadFile(filepath.Join(dir, "tool-proposal-000543.json"))
	_ = json.Unmarshal(raw, &proof)
	if string(proof.ArgumentsBytes) != call.ArgsJSON {
		t.Fatal("invalid original bytes were rewritten")
	}
}
func TestReleaseChainV8ProposalBoundAndLegacyDefault(t *testing.T) {
	if releaseChainProposalSinkFor(false, "/private", "normal_sent") != nil {
		t.Fatal("sealed legacy scope acquired retention")
	}
	dir := t.TempDir()
	sink := releaseChainProposalSinkFor(true, dir, "normal_sent")
	call := transport.ToolCallRequest{ToolCallRef: "synthetic-ref", ToolName: "synthetic.read", ArgsJSON: strings.Repeat("x", releaseChainProposalMaxBytes)}
	if sink.retain(542, "start", "run", &call) == nil {
		t.Fatal("unbounded private proposal accepted")
	}
	files, _ := os.ReadDir(dir)
	if len(files) != 0 {
		t.Fatal("oversized proof left a file")
	}
	sink.directory = "relative-untrusted-path"
	call.ArgsJSON = `{}`
	if sink.retain(542, "start", "run", &call) == nil {
		t.Fatal("relative artifact path accepted")
	}
}
func TestReleaseChainV8ProposalRetainedBeforeCoreIntegration(t *testing.T) {
	for _, deny := range []bool{false, true} {
		name := "retained_before_release"
		if deny {
			name = "write_denied_blocks_release"
		}
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			if deny {
				if os.WriteFile(filepath.Join(dir, "tool-proposal-000538.json"), []byte("OLD_IMMUTABLE_PROOF"), 0600) != nil {
					t.Fatal("collision fixture failed")
				}
			}
			version := "gemini-3.8-flash-n"
			usage := transport.Usage{InputTokens: 3, OutputTokens: 2, TotalTokens: 205}
			call := transport.ToolCallRequest{ToolCallRef: "synthetic-ref", ToolName: "synthetic.read", ArgsJSON: ` {"x":1.0,"private":"SYNTHETIC_ORIGINAL_BYTES"} `}
			event := transport.NewToolCallRequestedEvent("chain-run", antigravity.Name, call)
			event.Usage, event.ProviderModelVersion = &usage, &version
			spy := &releaseChainV6StageSpy{releaseChainV5StreamSpy: &releaseChainV5StreamSpy{converterWorkerProvider: &converterWorkerProvider{}, events: []transport.TransportEvent{event}}, stage: transport.ProviderStageStream}
			p, live, req := releaseChainV6OwnedProviderFixture(t, spy)
			p.proposals = releaseChainProposalSinkFor(true, dir, "normal_sent")
			stream, err := p.StartRun(live, req)
			if err != nil {
				t.Fatal(err)
			}
			tools, errors := 0, 0
			for out := range stream {
				switch out.EventType {
				case transport.EventTypeToolCallRequested:
					tools++
					raw, e := os.ReadFile(filepath.Join(dir, "tool-proposal-000538.json"))
					var proof releaseChainPrivateProposal
					if e != nil || json.Unmarshal(raw, &proof) != nil || proof.ToolCall.ArgsJSON != call.ArgsJSON || string(proof.ArgumentsBytes) != call.ArgsJSON {
						t.Fatal("Core saw a proposal before its exact private proof")
					}
				case transport.EventTypeTransportError:
					errors++
				}
			}
			var measured bool
			if p.window.pool.QueryRow(context.Background(), `SELECT outcome='completed' AND input_tokens=3 AND output_tokens=2 AND total_tokens=205 FROM `+p.window.table("fixture_chain_v5_high_requests")+` WHERE ordinal=538`).Scan(&measured) != nil || !measured || spy.calls.Load() != 1 {
				t.Fatal("retention changed spent measured native request")
			}
			if !deny && (tools != 1 || errors != 0) || deny && (tools != 0 || errors != 1) {
				t.Fatal("retention failure released or replayed a tool", tools, errors)
			}
		})
	}
}

func TestReleaseChainV8RetentionScopeBinding(t *testing.T) {
	policy := releaseChainV8Policy()
	if !policy.valid() || !policy.ProposalRetention || policy.MaximumPrivateProposalBytes != releaseChainProposalMaxBytes || policy.attemptMillis() != 300000 {
		t.Fatal("unfixed retention scope")
	}
	changed := policy
	changed.MaximumPrivateProposalBytes++
	if changed.valid() {
		t.Fatal("unbounded capture scope accepted")
	}
	cfg := releaseChainHighConfigurationFor(policy)
	if cfg.ChildBudget.ActiveMillis != 1320000 || cfg.Policy.ModelMillis != 300000 {
		t.Fatal("V8 resource profile changed")
	}
	dir := t.TempDir()
	sink := releaseChainProposalSinkFor(true, dir, "normal_sent")
	call := transport.ToolCallRequest{ToolCallRef: "opaque-synthetic", ToolName: "synthetic.read", ArgsJSON: `{}`}
	if sink.retain(543, "start", "run", &call) != nil || sink.retain(542, "submit", "run", &call) != nil {
		t.Fatal("fixture proof write failed")
	}
	if os.WriteFile(filepath.Join(dir, "encrypted-settings-key"), []byte("UNREAD_SYNTHETIC_PRIVATE_OTHER_FILE"), 0600) != nil {
		t.Fatal("unrelated fixture setup failed")
	}
	items, err := readReleaseChainProposalMetadata(dir, 541, 74)
	if err != nil || len(items) != 2 || items[0]["ordinal"] != float64(542) || items[1]["ordinal"] != float64(543) {
		t.Fatal("own namespace or native ordering lost", err)
	}
	if _, err = readReleaseChainProposalMetadata(dir, 542, 74); err == nil {
		t.Fatal("old proof imported into a new owning range")
	}
}
