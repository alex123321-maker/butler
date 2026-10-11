package actionauth

import (
	"regexp"
	"time"
)

// ConverterPermission is an owner-reviewed allowance for one pure IR shape,
// not permission to attach arbitrary code, files, network or a new account.
// Package identity is unknown before development; its exact tested hash is
// additionally pinned by catalog admission and each prepared action contract.
type ConverterPermission struct {
	ConverterID      string `json:"converter_id"`
	Class            string `json:"class"`
	InputSchemaHash  string `json:"input_schema_hash"`
	OutputSchemaHash string `json:"output_schema_hash"`
	RuntimePin       string `json:"runtime_pin"`
	InputBytes       int64  `json:"input_bytes"`
	OutputBytes      int64  `json:"output_bytes"`
	Rows             int64  `json:"rows"`
	FieldBytes       int64  `json:"field_bytes"`
	Millis           int64  `json:"millis"`
	Trials           int64  `json:"trials"`
}

var converterID = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)
var converterDigest = regexp.MustCompile(`^[a-f0-9]{64}$`)

func (p ConverterPermission) Validate() error {
	if !converterID.MatchString(p.ConverterID) || p.Class != "delimited_json_v1" || !converterDigest.MatchString(p.InputSchemaHash) || !converterDigest.MatchString(p.OutputSchemaHash) || !converterDigest.MatchString(p.RuntimePin) || p.InputBytes < 1 || p.InputBytes > 64*1024 || p.OutputBytes < 1 || p.OutputBytes > 256*1024 || p.Rows < 1 || p.Rows > 512 || p.FieldBytes < 1 || p.FieldBytes > 4096 || p.Millis < 1 || p.Millis > 2000 || p.Trials < 1 || p.Trials > 8 {
		return ErrOwnerAuthority
	}
	return nil
}

// AllowsConverterAttachment receives a trusted expected shape from the owned
// catalog. It never consumes a model-selected mandate or author pass flag.
func AllowsConverterAttachment(m Mandate, expected ConverterPermission, at time.Time) bool {
	if expected.Validate() != nil || m.ScopeKind != "owner_goal" || m.Status != "active" || m.RevokedAt != nil || !m.ExpiresAt.After(at) || !WithinActiveHours(m.Hours, at) || contains(m.DenyTools, "converter."+expected.ConverterID+".convert") {
		return false
	}
	for _, p := range m.Converters {
		if p == expected {
			return true
		}
	}
	return false
}
