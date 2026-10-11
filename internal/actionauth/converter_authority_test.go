package actionauth

import (
	"strings"
	"testing"
	"time"
)

func TestConverterPermissionCannotBroadenShapeOrPower(t *testing.T) {
	now := time.Now()
	p := ConverterPermission{ConverterID: "inventory", Class: "delimited_json_v1", InputSchemaHash: strings.Repeat("a", 64), OutputSchemaHash: strings.Repeat("b", 64), RuntimePin: strings.Repeat("c", 64), InputBytes: 8192, OutputBytes: 32768, Rows: 128, FieldBytes: 1024, Millis: 1000, Trials: 2}
	m := Mandate{ScopeKind: "owner_goal", Status: "active", ExpiresAt: now.Add(time.Hour), Converters: []ConverterPermission{p}}
	if p.Validate() != nil || !AllowsConverterAttachment(m, p, now) {
		t.Fatal("exact permission denied")
	}
	for _, f := range []func(*ConverterPermission){
		func(p *ConverterPermission) { p.ConverterID = "other" }, func(p *ConverterPermission) { p.ConverterID = "*" },
		func(p *ConverterPermission) { p.Class = "shell" }, func(p *ConverterPermission) { p.InputSchemaHash = strings.Repeat("d", 64) },
		func(p *ConverterPermission) { p.OutputSchemaHash = strings.Repeat("d", 64) }, func(p *ConverterPermission) { p.RuntimePin = strings.Repeat("d", 64) },
		func(p *ConverterPermission) { p.InputBytes++ }, func(p *ConverterPermission) { p.OutputBytes++ }, func(p *ConverterPermission) { p.Rows++ },
		func(p *ConverterPermission) { p.FieldBytes++ }, func(p *ConverterPermission) { p.Millis++ }, func(p *ConverterPermission) { p.Trials++ },
	} {
		changed := p
		f(&changed)
		if AllowsConverterAttachment(m, changed, now) {
			t.Fatalf("broadened: %#v", changed)
		}
	}
	m.Status = "revoked"
	if AllowsConverterAttachment(m, p, now) {
		t.Fatal("revoked admitted")
	}
	m.Status = "active"
	if AllowsConverterAttachment(m, p, m.ExpiresAt) {
		t.Fatal("expired admitted")
	}
}
