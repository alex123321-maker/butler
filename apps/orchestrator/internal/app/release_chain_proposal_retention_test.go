package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/generatedtools"
	"github.com/butler/butler/internal/transport"
)

const releaseChainProposalMaxBytes = 256 * 1024

// Kernel fixture configuration only. The caller already requires literal
// /private, and ordinal/scenario come from the owning native admission.
type releaseChainProposalSink struct{ directory, scenario string }

func releaseChainProposalSinkFor(enabled bool, directory, scenario string) *releaseChainProposalSink {
	if !enabled {
		return nil
	}
	return &releaseChainProposalSink{directory: directory, scenario: scenario}
}

type releaseChainPrivateProposal struct {
	Version        string                    `json:"version"`
	Ordinal        int                       `json:"ordinal"`
	Scenario       string                    `json:"scenario"`
	Kind           string                    `json:"kind"`
	RunID          string                    `json:"run_id"`
	ToolCall       transport.ToolCallRequest `json:"tool_call"`
	ArgumentsBytes []byte                    `json:"arguments_bytes_base64"`
}

func (s *releaseChainProposalSink) retain(ordinal int, kind, run string, call *transport.ToolCallRequest) error {
	if s == nil || !filepath.IsAbs(s.directory) || !releaseChainV5ScenarioKnown(s.scenario) || ordinal <= 0 || run == "" || call == nil || call.ToolCallRef == "" || call.ToolName == "" || (kind != "start" && kind != "submit" && kind != "continue") {
		return errNativeFixtureDenied
	}
	info, err := os.Lstat(s.directory)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errNativeFixtureDenied
	}
	raw, err := json.Marshal(releaseChainPrivateProposal{Version: "butler-native-tool-proposal-v1", Ordinal: ordinal, Scenario: s.scenario, Kind: kind, RunID: run, ToolCall: *call, ArgumentsBytes: []byte(call.ArgsJSON)})
	if err != nil || len(raw) > releaseChainProposalMaxBytes {
		return errNativeFixtureDenied
	}
	basename := fmt.Sprintf("tool-proposal-%06d", ordinal)
	if privateReleaseChainProposalWrite(filepath.Join(s.directory, basename+".json"), raw) != nil {
		return errNativeFixtureDenied
	}
	// Public-safe metadata contains types and fixed statuses only. Original
	// values, unexpected keys and decoder error strings never leave private proof.
	metadata := releaseChainProposalDiagnostics(call)
	hash := sha256.Sum256(raw)
	metadata["private_proposal_sha256"] = hex.EncodeToString(hash[:])
	metadata["private_proposal_bytes"] = len(raw)
	metadata["version"] = "butler-native-tool-proposal-diagnostics-v1"
	metadata["ordinal"] = ordinal
	metadata["scenario"] = s.scenario
	metadata["kind"] = kind
	encoded, err := json.Marshal(metadata)
	if err != nil || len(encoded) > 8192 || privateReleaseChainProposalWrite(filepath.Join(s.directory, basename+".meta.json"), encoded) != nil {
		return errNativeFixtureDenied
	}
	return nil
}
func privateReleaseChainProposalWrite(path string, body []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return errNativeFixtureDenied
	}
	defer file.Close()
	n, err := file.Write(body)
	if err != nil || n != len(body) || file.Sync() != nil {
		return errNativeFixtureDenied
	}
	return file.Close()
}
func releaseChainProposalJSONType(raw json.RawMessage) string {
	if len(raw) == 0 {
		return "missing"
	}
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return "invalid_json"
	}
	switch value.(type) {
	case nil:
		return "null"
	case bool:
		return "boolean"
	case string:
		return "string"
	case float64:
		return "number"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	}
	return "invalid_json"
}
func releaseChainProposalDiagnostics(call *transport.ToolCallRequest) map[string]any {
	meta := map[string]any{"developer_decode_status": "not_applicable"}
	if call == nil {
		meta["developer_decode_status"] = "no_tool_call"
		return meta
	}
	bytesHash := sha256.Sum256([]byte(call.ArgsJSON))
	meta["arguments_sha256"] = hex.EncodeToString(bytesHash[:])
	meta["arguments_bytes"] = len(call.ArgsJSON)
	authHash, err := actionauth.ArgumentsHash(call.ArgsJSON)
	meta["strict_arguments_json"] = err == nil
	if err == nil {
		meta["arguments_authorization_sha256"] = authHash
	}
	if call.ToolName != generatedtools.DeveloperAdmissionTool {
		return meta
	}
	meta["tool_class"] = "converter_developer_admission"
	meta["developer_decode_status"] = "rejected"
	if _, err := generatedtools.DecodeDeveloperArgs(call.ArgsJSON); err == nil {
		meta["developer_decode_status"] = "valid_syntax"
	}
	object := map[string]json.RawMessage{}
	if json.Unmarshal([]byte(call.ArgsJSON), &object) != nil {
		meta["top_level_type"] = releaseChainProposalJSONType(json.RawMessage(call.ArgsJSON))
		return meta
	}
	meta["top_level_key_count"] = len(object)
	fields := map[string]string{}
	allowed := map[string]bool{}
	for _, name := range []string{"need_source", "contract_source", "budget", "deadline", "dependencies"} {
		fields[name] = releaseChainProposalJSONType(object[name])
		allowed[name] = true
	}
	meta["field_types"] = fields
	unknown := 0
	for key := range object {
		if !allowed[key] {
			unknown++
		}
	}
	meta["unexpected_key_count"] = unknown
	var deadline string
	deadlineValid := false
	if json.Unmarshal(object["deadline"], &deadline) == nil {
		at, e := time.Parse(time.RFC3339Nano, deadline)
		deadlineValid = e == nil && !at.IsZero()
	}
	meta["deadline_rfc3339"] = deadlineValid
	for _, name := range []string{"need_source", "contract_source"} {
		ref := map[string]json.RawMessage{}
		_ = json.Unmarshal(object[name], &ref)
		types := map[string]string{}
		for _, key := range []string{"source_id", "revision", "access_revision", "content_hash"} {
			types[key] = releaseChainProposalJSONType(ref[key])
		}
		meta[name+"_field_types"] = types
	}
	units := map[string]json.RawMessage{}
	_ = json.Unmarshal(object["budget"], &units)
	budgetTypes := map[string]string{}
	budgetIntegers := map[string]bool{}
	for _, name := range []string{"model_turns", "tokens", "cost_micros", "external_calls", "children", "active_millis", "egress_bytes", "provider_input_bytes"} {
		budgetTypes[name] = releaseChainProposalJSONType(units[name])
		var integer int64
		budgetIntegers[name] = len(units[name]) > 0 && strings.TrimSpace(string(units[name])) != "null" && json.Unmarshal(units[name], &integer) == nil && integer >= 0
	}
	meta["budget_field_types"] = budgetTypes
	meta["budget_nonnegative_go_int64"] = budgetIntegers
	var dependencies []json.RawMessage
	if json.Unmarshal(object["dependencies"], &dependencies) == nil {
		meta["dependency_count"] = len(dependencies)
	}
	return meta
}

func readReleaseChainProposalMetadata(directory string, baseline, maximum int) ([]map[string]any, error) {
	return readReleaseChainProposalMetadataWithDeclaredLimit(directory, baseline, maximum, 80)
}

func readReleaseChainProposalMetadata120(directory string, baseline int) ([]map[string]any, error) {
	return readReleaseChainProposalMetadataWithDeclaredLimit(directory, baseline, 120, 120)
}

func readReleaseChainProposalMetadataWithDeclaredLimit(directory string, baseline, maximum, declaredLimit int) ([]map[string]any, error) {
	items := []map[string]any{}
	if (declaredLimit != 80 && declaredLimit != 120) || !filepath.IsAbs(directory) || baseline <= 0 || maximum <= 0 || maximum > declaredLimit {
		return nil, errNativeFixtureDenied
	}
	paths, err := filepath.Glob(filepath.Join(directory, "tool-proposal-*.meta.json"))
	if err != nil || len(paths) > maximum {
		return nil, errNativeFixtureDenied
	}
	for _, path := range paths {
		info, e := os.Lstat(path)
		if e != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() > 8192 {
			return nil, errNativeFixtureDenied
		}
		raw, e := os.ReadFile(path)
		var m map[string]any
		if e != nil || json.Unmarshal(raw, &m) != nil || m["version"] != "butler-native-tool-proposal-diagnostics-v1" {
			return nil, errNativeFixtureDenied
		}
		ordinal, ok := m["ordinal"].(float64)
		if !ok || ordinal <= float64(baseline) || ordinal > float64(baseline+maximum) || ordinal != float64(int(ordinal)) || filepath.Base(path) != fmt.Sprintf("tool-proposal-%06d.meta.json", int(ordinal)) {
			return nil, errNativeFixtureDenied
		}
		items = append(items, m)
	}
	sort.Slice(items, func(i, j int) bool { return items[i]["ordinal"].(float64) < items[j]["ordinal"].(float64) })
	return items, nil
}
