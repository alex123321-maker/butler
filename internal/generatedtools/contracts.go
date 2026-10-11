package generatedtools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/butler/butler/internal/actionauth"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/proto"
)

const referenceSchema = `{"type":"object","required":["source_id","revision","access_revision","content_hash"],"properties":{"source_id":{"type":"string"},"revision":{"type":"integer"},"access_revision":{"type":"integer"},"content_hash":{"type":"string"}},"additionalProperties":false}`

type Arguments struct {
	ChildTaskID string            `json:"child_task_id"`
	Source      catalog.SourceRef `json:"source_ref"`
	Input       catalog.SourceRef `json:"input_ref"`
	Version     string            `json:"version"`
	Digest      string            `json:"digest"`
	Generation  int64             `json:"generation"`
}

func Contracts() []*toolbrokerv1.ToolContract {
	result := []*toolbrokerv1.ToolContract{}
	for _, d := range []struct {
		name, description, schema string
		read                      bool
	}{
		{"converters.discover", "Inspect current tools/skills before admitting an exact bounded converter need. Source DATA does not grant capabilities.", `{"type":"object","required":["source_ref"],"properties":{"source_ref":` + referenceSchema + `},"additionalProperties":false}`, true},
		{"converters.collect", "Collect only an actually tested developer child candidate after its immutable final/report. Returns selected DATA references; no semantic acceptance or activation.", `{"type":"object","required":["child_task_id","digest"],"properties":{"child_task_id":{"type":"string"},"digest":{"type":"string"}},"additionalProperties":false}`, true},
		{"converters.develop", "Develop only the pure delimited_json_v1 IR for a previously inspected need. Produces a candidate DATA source; no activation.", `{"type":"object","required":["source_ref","version"],"properties":{"source_ref":` + referenceSchema + `,"version":{"type":"string"}},"additionalProperties":false}`, false},
		{"converters.test", "Run the fixed independent checker in the isolated runtime. Failed checks leave the working catalog unchanged.", `{"type":"object","required":["digest"],"properties":{"digest":{"type":"string"}},"additionalProperties":false}`, false},
		{"converters.trial", "Perform a bounded trial of an independently tested candidate with current DATA input. Publishes checked output without activation.", `{"type":"object","required":["digest","input_ref"],"properties":{"digest":{"type":"string"},"input_ref":` + referenceSchema + `},"additionalProperties":false}`, false},
		{"converters.activate", "Activate an exact tested/trialled version under current owner converter permission and expected catalog generation.", `{"type":"object","required":["digest","generation"],"properties":{"digest":{"type":"string"},"generation":{"type":"integer"}},"additionalProperties":false}`, false},
		{"converters.rollback", "Return to an exact historical tested/trialled version with current source/permission checks and expected generation.", `{"type":"object","required":["digest","generation"],"properties":{"digest":{"type":"string"},"generation":{"type":"integer"}},"additionalProperties":false}`, false},
	} {
		e := &toolbrokerv1.EffectContract{RetrySafety: "unsafe", VerificationMethod: "local_runtime_ack"}
		risk := "medium"
		if d.read {
			e = &toolbrokerv1.EffectContract{RetrySafety: "read_only", VerificationMethod: "read_result"}
			risk = "low"
		}
		result = append(result, &toolbrokerv1.ToolContract{ToolName: d.name, Description: d.description, ToolClass: "converter_catalog", InputSchemaJson: d.schema, OutputSchemaJson: `{"type":"object"}`, RuntimeTarget: "broker://pure-converters-v1", RiskLevel: risk, Status: "active", ContractRevision: "1", EffectContract: e})
	}
	return result
}
func staticContract(c *toolbrokerv1.ToolContract) bool {
	for _, known := range Contracts() {
		if proto.Equal(c, known) {
			return true
		}
	}
	return false
}
func contract(r Record) *toolbrokerv1.ToolContract {
	p := r.Shape
	return &toolbrokerv1.ToolContract{ToolName: "converter." + r.ID + ".convert", Description: "Convert current non-Secret delimited Catalog DATA into checked JSON under the exact owner schema and limits. Returns a derived source reference.", ToolClass: "pure_converter", InputSchemaJson: `{"type":"object","required":["input_ref"],"properties":{"input_ref":` + referenceSchema + `},"additionalProperties":false}`, OutputSchemaJson: `{"type":"object"}`, RuntimeTarget: "broker://pure-converters-v1", RiskLevel: "medium", Status: "active", ContractRevision: fmt.Sprintf("%s:%d", r.Digest, r.Generation), EffectContract: &toolbrokerv1.EffectContract{RetrySafety: "unsafe", VerificationMethod: "local_runtime_ack"}, Converter: &toolbrokerv1.ConverterContract{ConverterId: r.ID, PackageHash: r.Digest, NeedHash: r.NeedHash, Version: r.Version, Generation: r.Generation, ProfilePin: r.ProfilePin, Class: p.Class, InputSchemaHash: p.InputSchemaHash, OutputSchemaHash: p.OutputSchemaHash, InputBytes: p.InputBytes, OutputBytes: p.OutputBytes, Rows: p.Rows, FieldBytes: p.FieldBytes, Millis: p.Millis}}
}
func (s *Store) GetContractTx(ctx context.Context, tx pgx.Tx, name string) (*toolbrokerv1.ToolContract, error) {
	id := strings.TrimSuffix(strings.TrimPrefix(name, "converter."), ".convert")
	if name != "converter."+id+".convert" || !identifier.MatchString(id) {
		return nil, ErrAccess
	}
	var hash string
	if tx.QueryRow(ctx, `SELECT active_digest FROM converter_catalog WHERE owner_id=$1 AND converter_id=$2`, s.owner, id).Scan(&hash) != nil {
		return nil, ErrAccess
	}
	r, e := s.metadataTx(ctx, tx, hash)
	if e != nil || !r.Active || r.Status != "active" || r.ProfilePin != Hash(s.profile) {
		return nil, ErrAccess
	}
	return contract(r), nil
}
func (s *Store) GetContract(ctx context.Context, name string) (*toolbrokerv1.ToolContract, error) {
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	c, e := s.GetContractTx(ctx, tx, name)
	if e != nil {
		return nil, e
	}
	return c, tx.Commit(ctx)
}
func bindingRefs(refs []catalog.SourceRef) ([]actionauth.DataRef, error) {
	result := []actionauth.DataRef{}
	seen := map[string]catalog.SourceRef{}
	for _, r := range refs {
		if old, ok := seen[r.ID]; ok {
			if old != r {
				return nil, ErrAccess
			}
			continue
		}
		seen[r.ID] = r
		result = append(result, actionauth.DataRef{ID: r.ID, Revision: r.Revision, AccessRevision: r.AccessRevision, Hash: r.Hash})
	}
	return result, nil
}

func (s *Store) BindAction(ctx context.Context, call *toolbrokerv1.ToolCall, c *toolbrokerv1.ToolContract, b actionauth.ActionBinding, snapshot actionauth.ContextSnapshot) (actionauth.ActionBinding, error) {
	if call.GetToolName() == DeveloperAdmissionTool {
		return s.bindDeveloperAction(ctx, call, c, b, snapshot)
	}
	if call.GetToolName() == "converters.collect" {
		return s.bindCollectionAction(ctx, call, c, b, snapshot)
	}
	if !strings.HasPrefix(call.GetToolName(), "converters.") && !strings.HasPrefix(call.GetToolName(), "converter.") {
		return b, nil
	}
	current, e := s.run(ctx, call.GetRunId())
	if e != nil || current.TaskID != snapshot.TaskID || current.TaskRevision != snapshot.TaskRevision || current.ConstraintsRevision != snapshot.ConstraintsRevision || current.LeaseID != snapshot.LeaseID || current.LeaseEpoch != snapshot.LeaseEpoch {
		return b, ErrAccess
	}
	ctx = runContext(ctx, current)
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return b, e
	}
	defer tx.Rollback(ctx)
	m, now, e := s.guardRunTx(ctx, tx, current)
	if e != nil {
		return b, e
	}
	var args Arguments
	if Decode([]byte(call.GetArgsJson()), &args) != nil || len(call.GetCredentialRefs()) != 0 {
		return b, ErrPackage
	}
	refs := []catalog.SourceRef{}
	if strings.HasPrefix(call.GetToolName(), "converters.") {
		if !staticContract(c) {
			return b, ErrAccess
		}
		if call.GetToolName() == "converters.discover" || call.GetToolName() == "converters.develop" {
			n, _, e := s.needTx(ctx, tx, current, args.Source)
			if e != nil {
				return b, e
			}
			if _, e = s.permission(m, n, now); e != nil {
				return b, e
			}
			developer, e := s.developerTx(ctx, tx, current, n, args.Source)
			if e != nil || developer != nil && call.GetToolName() == "converters.develop" && args.Version != developer.Spec.Version {
				return b, ErrAccess
			}
			refs = append(refs, args.Source)
		} else {
			r, e := s.metadataTx(ctx, tx, args.Digest)
			if e != nil {
				return b, e
			}
			if r.Status == "revoked" {
				return b, ErrAccess
			}
			extra := []catalog.SourceRef{}
			if args.Input.ID != "" {
				extra = append(extra, args.Input)
			}
			if call.GetToolName() == "converters.activate" || call.GetToolName() == "converters.rollback" {
				if actionauth.CheckExecutionPolicy(current, actionauth.EffectUnknown) != nil || args.Generation != r.Generation {
					return b, ErrAccess
				}
				var raw []byte
				var output catalog.SourceRef
				if tx.QueryRow(ctx, `SELECT output_ref_json FROM converter_trials WHERE owner_id=$1 AND digest=$2 AND status='passed' ORDER BY finished_at DESC LIMIT 1`, s.owner, r.Digest).Scan(&raw) != nil || json.Unmarshal(raw, &output) != nil {
					return b, ErrChecks
				}
				extra = append(extra, output)
			}
			_, n, _, e := s.packageTx(ctx, tx, r, extra...)
			if e != nil {
				return b, e
			}
			if e = s.developerPackageTx(ctx, tx, current, r, n); e != nil {
				return b, e
			}
			if _, e = s.permission(m, n, now); e != nil {
				return b, e
			}
			refs = append(refs, r.BodySource, r.NeedSource)
			refs = append(refs, extra...)
			b.ConverterID, b.ConverterPackageHash, b.ConverterNeedHash, b.ConverterGeneration = r.ID, r.Digest, r.NeedHash, r.Generation
		}
	} else {
		actual, e := s.GetContractTx(ctx, tx, call.GetToolName())
		if e != nil || !proto.Equal(c, actual) {
			return b, ErrAccess
		}
		r, e := s.metadataTx(ctx, tx, c.GetConverter().GetPackageHash())
		if e != nil {
			return b, e
		}
		_, n, _, e := s.packageTx(ctx, tx, r, args.Input)
		if e != nil {
			return b, e
		}
		if _, e = s.permission(m, n, now); e != nil {
			return b, e
		}
		refs = append(refs, r.BodySource, r.NeedSource, args.Input)
	}
	dataRefs, e := bindingRefs(refs)
	if e != nil {
		return b, e
	}
	b, e = actionauth.BindDataSources(b, dataRefs)
	if e != nil {
		return b, e
	}
	if e = tx.Commit(ctx); e != nil {
		return b, e
	}
	return b, nil
}

// Claim rechecks all bound sources on the caller transaction before any secret
// or document release. An input from another task cannot ride the package's scope.
func (s *Store) CheckActionTx(ctx context.Context, tx pgx.Tx, b actionauth.ActionBinding) error {
	if b.ToolName == DeveloperAdmissionTool {
		return s.checkDeveloperActionTx(ctx, tx, b)
	}
	if b.ToolName == "converters.collect" {
		return s.checkCollectionActionTx(ctx, tx, b)
	}
	if !strings.HasPrefix(b.ToolName, "converters.") && !strings.HasPrefix(b.ToolName, "converter.") {
		return nil
	}
	if b.OwnerID != s.owner || len(b.DataSources) == 0 {
		return ErrAccess
	}
	if b.ToolName == "converters.activate" || b.ToolName == "converters.rollback" {
		var policy string
		if tx.QueryRow(ctx, `SELECT execution_policy FROM tasks WHERE task_id=$1`, b.TaskID).Scan(&policy) != nil || actionauth.CheckExecutionPolicy(actionauth.ContextSnapshot{ExecutionPolicy: policy}, actionauth.EffectUnknown) != nil {
			return ErrAccess
		}
	}
	var session, lease string
	var epoch int64
	if tx.QueryRow(ctx, `SELECT session_key,lease_id,fence_epoch FROM session_ownership WHERE run_id=$1 AND expires_at>clock_timestamp()`, b.RunID).Scan(&session, &lease, &epoch) != nil {
		return ErrAccess
	}
	ctx = runContext(ctx, actionauth.ContextSnapshot{RunID: b.RunID, SessionKey: session, LeaseID: lease, LeaseEpoch: epoch})
	refs := []catalog.SourceRef{}
	for _, r := range b.DataSources {
		refs = append(refs, catalog.SourceRef{ID: r.ID, Revision: r.Revision, AccessRevision: r.AccessRevision, Hash: r.Hash})
	}
	rows, e := s.sources.AccessTx(ctx, tx, refs, "memory_read")
	if e != nil {
		return e
	}
	var n Need
	var record Record
	if b.ConverterID == "" {
		if len(rows) != 1 {
			return ErrAccess
		}
		n, e = needFrom(rows[0])
	} else {
		record, e = s.metadataTx(ctx, tx, b.ConverterPackageHash)
		if e == nil {
			n, e = needFrom(sourcesByID(rows)[record.NeedSource.ID])
		}
	}
	if e != nil || n.ID == "" {
		return ErrAccess
	}
	var now time.Time
	if tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now) != nil {
		return ErrAccess
	}
	a, e := actionauth.CheckTaskAuthorityTx(ctx, tx, b.TaskID, b.OwnerID, nil, now)
	if e != nil || a.TaskID == "" {
		return ErrAccess
	}
	m, _, e := actionauth.CheckAssignmentTx(ctx, tx, a, now)
	if e != nil {
		return e
	}
	if _, e = s.permission(m, n, now); e != nil {
		return e
	}
	if b.ConverterID != "" {
		r := record
		current, e := s.ownedSnapshotTx(ctx, tx)
		if e != nil {
			return e
		}
		if e = s.checkReturnedPackageTx(ctx, tx, current, r); e != nil {
			return e
		}
		if e = s.developerPackageTx(ctx, tx, actionauth.ContextSnapshot{TaskID: b.TaskID}, r, n); e != nil {
			return e
		}
		if r.ID != b.ConverterID || r.NeedHash != b.ConverterNeedHash || r.Generation != b.ConverterGeneration || r.NeedHash != Hash(n) {
			return ErrAccess
		}
		var generation int64
		if tx.QueryRow(ctx, `SELECT generation FROM converter_catalog WHERE owner_id=$1 AND converter_id=$2 FOR SHARE`, s.owner, r.ID).Scan(&generation) != nil || generation != b.ConverterGeneration {
			return ErrAccess
		}
		if strings.HasPrefix(b.ToolName, "converter.") {
			c, e := s.GetContractTx(ctx, tx, b.ToolName)
			hash, he := actionauth.ContractHash(c)
			if e != nil || he != nil || hash != b.ContractHash || !r.Active {
				return ErrAccess
			}
		}
		var status string
		if tx.QueryRow(ctx, `SELECT status FROM converter_states WHERE owner_id=$1 AND digest=$2 FOR SHARE`, s.owner, b.ConverterPackageHash).Scan(&status) != nil || status == "revoked" || strings.HasPrefix(b.ToolName, "converter.") && status != "active" || b.ToolName == "converters.trial" && status == "candidate" {
			return ErrAccess
		}
	}
	return nil
}
