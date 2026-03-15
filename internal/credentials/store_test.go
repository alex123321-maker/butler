package credentials

import (
	"errors"
	"testing"
	"time"
)

type stubRow struct {
	err    error
	values []any
}

func (s stubRow) Scan(dest ...any) error {
	if s.err != nil {
		return s.err
	}
	for i := range dest {
		switch typed := dest[i].(type) {
		case *int64:
			*typed = s.values[i].(int64)
		case *string:
			*typed = s.values[i].(string)
		case *time.Time:
			*typed = s.values[i].(time.Time)
		}
	}
	return nil
}

func TestValidateRecord(t *testing.T) {
	t.Parallel()
	valid := Record{Alias: "github", SecretType: "api_token", TargetType: "http", ApprovalPolicy: ApprovalPolicyAlwaysConfirm, SecretRef: "secret://github/token", Status: StatusActive, MetadataJSON: `{"owner":"me"}`}
	if err := validateRecord(valid); err != nil {
		t.Fatalf("validateRecord returned error: %v", err)
	}

	invalid := valid
	invalid.ApprovalPolicy = "bad"
	if err := validateRecord(invalid); err == nil {
		t.Fatal("expected invalid approval policy to fail validation")
	}
}

func TestScanRecord(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	record, err := scanRecord(stubRow{values: []any{int64(1), "github", "api_token", "http", `["api.github.com"]`, `["http.request"]`, ApprovalPolicyAlwaysConfirm, "secret://github/token", StatusActive, `{"owner":"me"}`, now, now}})
	if err != nil {
		t.Fatalf("scanRecord returned error: %v", err)
	}
	if len(record.AllowedDomains) != 1 || record.AllowedDomains[0] != "api.github.com" {
		t.Fatalf("unexpected allowed domains: %+v", record.AllowedDomains)
	}
	if len(record.AllowedTools) != 1 || record.AllowedTools[0] != "http.request" {
		t.Fatalf("unexpected allowed tools: %+v", record.AllowedTools)
	}
	if record.SecretRef != "secret://github/token" {
		t.Fatalf("unexpected secret ref %q", record.SecretRef)
	}
}

func TestScanRecordInvalidJSON(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	_, err := scanRecord(stubRow{values: []any{int64(1), "github", "api_token", "http", `{`, `[]`, ApprovalPolicyAlwaysConfirm, "secret://github/token", StatusActive, `{}`, now, now}})
	if err == nil {
		t.Fatal("expected invalid allowed_domains JSON to fail")
	}
}

func TestNormalizeMetadata(t *testing.T) {
	t.Parallel()
	if got := normalizeMetadata(""); got != "{}" {
		t.Fatalf("normalizeMetadata empty = %q, want {}", got)
	}
	if got := normalizeMetadata(`{"x":1}`); got != `{"x":1}` {
		t.Fatalf("normalizeMetadata preserved = %q", got)
	}
}

func TestScanRecordReturnsRowError(t *testing.T) {
	t.Parallel()
	boom := errors.New("boom")
	if _, err := scanRecord(stubRow{err: boom}); !errors.Is(err, boom) {
		t.Fatalf("scanRecord error = %v, want %v", err, boom)
	}
}
