package projectfixture

import (
	"bytes"
	"encoding/json"
	"github.com/butler/butler/internal/extensions"
)

// Manifest uses an operator-measured runtime artifact digest, not a self-attested
// hash supplied by executable package code. Source revision remains review data.
func Manifest(runtimeDigest, sourceRevision string) (extensions.Package, error) {
	output := json.RawMessage(`{"type":"object","required":["status_code","body"],"properties":{"status_code":{"type":"integer"},"body":{"type":"string"}},"additionalProperties":false}`)
	read := json.RawMessage(`{"type":"object","required":["url"],"properties":{"url":{"type":"string"}},"additionalProperties":false}`)
	write := json.RawMessage(`{"type":"object","required":["url","body"],"properties":{"url":{"type":"string"},"body":{"type":"object","required":["project_revision","changes"],"properties":{"project_revision":{"type":"string"},"changes":{"type":"array","items":{"type":"object","required":["path","before","after"],"properties":{"path":{"type":"string","enum":["README.md"]},"before":{"type":"string"},"after":{"type":"string"}},"additionalProperties":false}}},"additionalProperties":false}},"additionalProperties":false}`)
	root := "https://github.com/alex123321-maker/butler/tree/" + sourceRevision
	m := extensions.Manifest{Format: extensions.ManifestFormat, ID: "project", Version: "1.0.0", Source: extensions.Source{URI: root + "/internal/extensions/projectfixture", Revision: sourceRevision}, Description: "Inspect a disposable project and prepare a bounded README patch. No patch is applied and no process/build command is executed.", Runtime: extensions.Runtime{ABI: extensions.RuntimeABI, Profile: "reference-http"}, Compatibility: extensions.Compatibility{CoreContract: "1", ToolContract: "1"}, Configuration: []extensions.Setting{{Name: "origin", Type: "public_origin", Required: true}}, Resources: []extensions.Resource{{OriginSetting: "origin", PathPrefix: "/project"}}, Operations: []extensions.Operation{
		{Name: "extension.project.inspect", Description: "Read the synthetic project revision and current failed ready_flag check.", Method: "GET", Effect: "read", InputSchema: read, OutputSchema: output, Tags: []string{"project", "inspect", "checks"}},
		{Name: "extension.project.prepare-patch", Description: "Create one immutable external patch artifact for README.md, leaving the project unapplied.", Method: "POST", Effect: "commit", InputSchema: write, OutputSchema: output, Tags: []string{"project", "patch"}, Receipt: &extensions.Receipt{Protocol: "external_receipt_v1", IdempotencyHeader: "Idempotency-Key", VerificationPath: "/project/effects/{idempotency_key}"}},
		{Name: "extension.project.check", Description: "Read checks on the stored prepared patch artifact; the original project remains unchanged.", Method: "GET", Effect: "read", InputSchema: read, OutputSchema: output, Tags: []string{"project", "checks"}},
	}, Readiness: extensions.Readiness{Method: "GET", Path: "/project/health"}, Dependencies: []extensions.Dependency{{ID: "reviewed-http-runtime", Version: "1.0.0", Source: root + "/apps/tool-extension", SHA256: runtimeDigest}}, Limits: extensions.Limits{TimeoutMillis: 5000, EgressBytes: 1024 * 1024, OutputBytes: 65536}, Rollback: extensions.Rollback{Supported: true, StateCompatibility: "stateless"}}
	raw, err := json.Marshal(m)
	if err != nil {
		return extensions.Package{}, err
	}
	return extensions.Parse(bytes.NewReader(raw))
}
