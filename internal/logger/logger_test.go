package logger

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"testing"
)

func TestNewProducesJSONWithRequiredFields(t *testing.T) {
	var buf bytes.Buffer

	log := New(Options{
		Service:   "orchestrator",
		Component: "session",
		Writer:    &buf,
	})

	log.Info("session resolved")

	entry := decodeEntry(t, buf.Bytes())
	assertStringField(t, entry, "level", "INFO")
	assertStringField(t, entry, "msg", "session resolved")
	assertStringField(t, entry, FieldService, "orchestrator")
	assertStringField(t, entry, FieldComponent, "session")

	if _, ok := entry["time"].(string); !ok {
		t.Fatalf("expected time field to be present as string, got %#v", entry["time"])
	}
}

func TestChildHelpersInjectRunAndToolFields(t *testing.T) {
	var buf bytes.Buffer

	log := New(Options{Service: "orchestrator", Writer: &buf})
	log = WithComponent(log, "executor")
	log = WithRunID(log, "run-123")
	log = WithToolCallID(log, "tool-456")

	log.Warn("tool waiting")

	entry := decodeEntry(t, buf.Bytes())
	assertStringField(t, entry, FieldService, "orchestrator")
	assertStringField(t, entry, FieldComponent, "executor")
	assertStringField(t, entry, FieldRunID, "run-123")
	assertStringField(t, entry, FieldToolCallID, "tool-456")
	assertStringField(t, entry, "level", "WARN")
	assertStringField(t, entry, "msg", "tool waiting")
}

func TestNewHonorsConfiguredLevel(t *testing.T) {
	var buf bytes.Buffer

	log := New(Options{
		Writer: &buf,
		Level:  slog.LevelWarn,
	})

	log.Info("hidden")
	log.Warn("visible")

	if bytes.Contains(buf.Bytes(), []byte("hidden")) {
		t.Fatal("expected info message to be filtered out")
	}
	if !bytes.Contains(buf.Bytes(), []byte("visible")) {
		t.Fatal("expected warn message to be written")
	}
}

func TestMaskSecret(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "empty", input: "", want: ""},
		{name: "short", input: "abc", want: "****"},
		{name: "four chars", input: "abcd", want: "****"},
		{name: "longer", input: "secret-token", want: "se********en"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MaskSecret(tt.input)
			if got != tt.want {
				t.Fatalf("MaskSecret(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func decodeEntry(t *testing.T, raw []byte) map[string]any {
	t.Helper()

	var entry map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(raw), &entry); err != nil {
		t.Fatalf("failed to decode JSON log entry: %v", err)
	}

	return entry
}

func assertStringField(t *testing.T, entry map[string]any, key, want string) {
	t.Helper()

	got, ok := entry[key].(string)
	if !ok {
		t.Fatalf("expected %q to be a string, got %#v", key, entry[key])
	}
	if got != want {
		t.Fatalf("expected %q = %q, got %q", key, want, got)
	}
}
