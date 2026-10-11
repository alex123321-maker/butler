package generatedtools

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"time"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/delegation"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/jackc/pgx/v5"
)

var ErrPrerequisite = errors.New("missing_prerequisite: current confirmed parent converter discovery required")

// This is a content-free accepted capability, not a model-proposed permission.
// The original receipt stays task-bound; only this distinct immutable bridge can
// admit its exact selected Need into the developer child's three local tools.
type DeveloperSnapshot struct {
	Version                  int                            `json:"version"`
	OwnerID                  string                         `json:"owner_id"`
	ChildTaskID              string                         `json:"child_task_id"`
	ChildTaskRevision        int64                          `json:"child_task_revision"`
	ChildConstraintsRevision int64                          `json:"child_constraints_revision"`
	Binding                  actionauth.ActionBinding       `json:"binding"`
	EffectID                 string                         `json:"effect_id"`
	AuthorizationID          string                         `json:"authorization_id"`
	Args                     DeveloperArgs                  `json:"args"`
	ArgsJSON                 string                         `json:"args_json"`
	NeedID                   string                         `json:"need_id"`
	NeedHash                 string                         `json:"need_hash"`
	Spec                     DeveloperSpec                  `json:"spec"`
	Permission               actionauth.ConverterPermission `json:"permission"`
	DiscoveryHash            string                         `json:"discovery_hash"`
	DiscoveryActionID        string                         `json:"discovery_action_id"`
	Permissions              []delegation.Permission        `json:"permissions"`
}

func DeveloperPermissions() ([]delegation.Permission, error) {
	result := []delegation.Permission{}
	for _, c := range Contracts() {
		if c.ToolName != "converters.discover" && c.ToolName != "converters.develop" && c.ToolName != "converters.test" {
			continue
		}
		hash, err := actionauth.ContractHash(c)
		if err != nil {
			return nil, err
		}
		effect := "prepare"
		if c.ToolName == "converters.discover" {
			effect = "read"
		}
		result = append(result, delegation.Permission{Tool: c.ToolName, ContractHash: hash, Resources: []string{"tool:" + c.ToolName}, Effects: []string{effect}, Credentials: []delegation.CredentialRef{}})
	}
	return result, nil
}

type DeveloperSources interface {
	AccessTx(context.Context, pgx.Tx, []catalog.SourceRef, string) ([]catalog.Source, error)
	CheckDelegationSourcesTx(context.Context, pgx.Tx, string, []catalog.SourceRef, string) error
}

// Caller has locked parent authority before entering source/effect rows. This
// never performs discovery as a hidden admission action or borrows a connection.
func ReadDeveloperPrerequisiteTx(ctx context.Context, tx pgx.Tx, sources DeveloperSources, b actionauth.ActionBinding, args DeveloperArgs, profile Profile) (DeveloperSnapshot, error) {
	hash, err := actionauth.ContractHash(DeveloperAdmissionContract())
	if err != nil || profile.Validate() != nil || b.ToolName != DeveloperAdmissionTool || b.ContractHash != hash || b.Resource != "tool:"+DeveloperAdmissionTool || b.Effect != actionauth.EffectPrepare || len(b.CredentialRefs) != 0 || b.OwnerID == "" || b.TaskID == "" {
		return DeveloperSnapshot{}, ErrAccess
	}
	if err = checkDeveloperPolicyTx(ctx, tx, b); err != nil {
		return DeveloperSnapshot{}, err
	}
	rows, err := sources.AccessTx(ctx, tx, []catalog.SourceRef{args.NeedSource, args.ContractSource}, "memory_read")
	if err != nil || len(rows) != 2 {
		return DeveloperSnapshot{}, ErrAccess
	}
	data := sourcesByID(rows)
	n, err := needFrom(data[args.NeedSource.ID])
	if err != nil {
		return DeveloperSnapshot{}, err
	}
	spec, err := DecodeDeveloperSpec([]byte(data[args.ContractSource.ID].Body), Hash(n))
	if err != nil {
		return DeveloperSnapshot{}, err
	}
	var now time.Time
	if tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now) != nil || !args.Deadline.After(now) {
		return DeveloperSnapshot{}, ErrAccess
	}
	a, err := actionauth.CheckTaskAuthorityTx(ctx, tx, b.TaskID, b.OwnerID, nil, now)
	if err != nil || a.TaskID == "" {
		return DeveloperSnapshot{}, ErrAccess
	}
	m, _, err := actionauth.CheckAssignmentTx(ctx, tx, a, now)
	if err != nil {
		return DeveloperSnapshot{}, err
	}
	var permission actionauth.ConverterPermission
	for _, p := range m.Converters {
		if p == ExpectedPermission(n, profile, p.Trials) && actionauth.AllowsConverterAttachment(m, p, now) {
			permission = p
			break
		}
	}
	if permission.ConverterID == "" {
		return DeveloperSnapshot{}, ErrAccess
	}
	var saved []byte
	var ref catalog.SourceRef
	var needRef []byte
	var needHash, taskID string
	var revision, constraints int64
	if tx.QueryRow(ctx, `SELECT need_hash,source_ref_json,task_id,task_revision,constraints_revision,discovery_json FROM converter_needs WHERE owner_id=$1 AND need_id=$2`, b.OwnerID, n.ID).Scan(&needHash, &needRef, &taskID, &revision, &constraints, &saved) != nil || json.Unmarshal(needRef, &ref) != nil || ref != args.NeedSource || needHash != Hash(n) || taskID != b.TaskID || revision != b.TaskRevision || constraints != b.ConstraintsRevision {
		return DeveloperSnapshot{}, ErrPrerequisite
	}
	var d Discovery
	if json.Unmarshal(saved, &d) != nil {
		return DeveloperSnapshot{}, ErrPrerequisite
	}
	d, err = normalizeDiscovery(d, n)
	if err != nil || len(d.Compatible) != 0 {
		return DeveloperSnapshot{}, ErrPrerequisite
	}
	if err = CheckCompatibleInstalledTx(ctx, tx, b.OwnerID, n); err != nil {
		return DeveloperSnapshot{}, err
	}
	discoveryAction, err := confirmedParentDiscoveryTx(ctx, tx, b, args.NeedSource, n, d)
	if err != nil {
		return DeveloperSnapshot{}, err
	}
	permissions, err := DeveloperPermissions()
	if err != nil {
		return DeveloperSnapshot{}, err
	}
	return DeveloperSnapshot{Version: 1, OwnerID: b.OwnerID, Binding: b, Args: args, NeedID: n.ID, NeedHash: Hash(n), Spec: spec, Permission: permission, DiscoveryHash: Hash(d), DiscoveryActionID: discoveryAction, Permissions: permissions}, nil
}

func checkDeveloperPolicyTx(ctx context.Context, tx pgx.Tx, b actionauth.ActionBinding) error {
	var policy string
	if tx.QueryRow(ctx, `SELECT execution_policy FROM tasks WHERE task_id=$1 AND owner_id=$2 AND revision=$3 AND constraints_revision=$4 AND status='active' FOR SHARE`, b.TaskID, b.OwnerID, b.TaskRevision, b.ConstraintsRevision).Scan(&policy) != nil || actionauth.CheckExecutionPolicy(actionauth.ContextSnapshot{ExecutionPolicy: policy}, actionauth.EffectPrepare) != nil {
		return ErrAccess
	}
	var now time.Time
	if tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now) != nil {
		return ErrAccess
	}
	if err := actionauth.CheckTaskActionTx(ctx, tx, b, now); err != nil {
		return err
	}
	permissions, err := DeveloperPermissions()
	if err != nil {
		return err
	}
	for _, p := range permissions {
		action := b
		action.ToolName = p.Tool
		action.ContractHash = p.ContractHash
		action.Resource = p.Resources[0]
		action.Effect = actionauth.Effect(p.Effects[0])
		action.DataSources = nil
		if err = actionauth.CheckTaskActionTx(ctx, tx, action, now); err != nil {
			return err
		}
	}
	return nil
}

// Search absence is bounded metadata, never a semantic conclusion. Any currently
// installed exact schema converter wins over generating a new implementation.
func CheckCompatibleInstalledTx(ctx context.Context, tx pgx.Tx, owner string, n Need) error {
	var exists bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM converter_catalog c JOIN converter_versions v ON v.owner_id=c.owner_id AND v.digest=c.active_digest JOIN converter_states s ON s.owner_id=v.owner_id AND s.digest=v.digest WHERE c.owner_id=$1 AND s.status='active' AND v.permission_shape_json->>'class'=$2 AND v.permission_shape_json->>'input_schema_hash'=$3 AND v.permission_shape_json->>'output_schema_hash'=$4)`, owner, n.Class, Hash(n.Input), Hash(n.Output)).Scan(&exists)
	if err != nil {
		return err
	}
	if exists {
		return ErrPrerequisite
	}
	return nil
}

func confirmedParentDiscoveryTx(ctx context.Context, tx pgx.Tx, parent actionauth.ActionBinding, ref catalog.SourceRef, n Need, d Discovery) (string, error) {
	resultHash, err := observationHash(map[string]any{"need_id": n.ID, "need_hash": Hash(n), "discovery": d, "instruction_authority": false})
	if err != nil {
		return "", err
	}
	var contractHash string
	for _, c := range Contracts() {
		if c.ToolName == "converters.discover" {
			contractHash, _ = actionauth.ContractHash(c)
		}
	}
	rows, err := tx.Query(ctx, `SELECT i.binding,i.effect_id,i.action_hash FROM action_intents i JOIN action_effects e ON e.action_id=i.effect_id JOIN action_authorizations a ON a.action_id=i.action_id JOIN action_effect_dispatches d ON d.action_id=e.action_id AND d.authorization_id=a.authorization_id WHERE i.binding->>'owner_id'=$1 AND i.binding->>'task_id'=$2 AND i.binding->>'tool_name'='converters.discover' AND e.state='effect_confirmed' AND a.state='dispatch_completed' AND d.outcome='dispatch_completed' AND EXISTS(SELECT 1 FROM action_effect_evidence x WHERE x.action_id=e.action_id AND x.source_action_id=i.action_id AND x.evidence->>'source'='runtime_response' AND x.evidence->>'verdict'='effect_confirmed' AND x.evidence->>'method'='read_result' AND x.evidence->>'result_digest'=$3 AND x.evidence->>'source_run_id'=i.run_id AND x.evidence->>'source_tool_call_id'=i.tool_call_id) ORDER BY i.created_at DESC LIMIT 16`, parent.OwnerID, parent.TaskID, resultHash)
	if err != nil {
		return "", err
	}
	type candidate struct {
		raw          []byte
		effect, hash string
	}
	candidates := []candidate{}
	for rows.Next() {
		var c candidate
		if err = rows.Scan(&c.raw, &c.effect, &c.hash); err != nil {
			rows.Close()
			return "", err
		}
		candidates = append(candidates, c)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return "", err
	}
	for _, c := range candidates {
		var b actionauth.ActionBinding
		if json.Unmarshal(c.raw, &b) != nil || b.TaskRevision != parent.TaskRevision || b.ConstraintsRevision != parent.ConstraintsRevision || b.ContractHash != contractHash || b.Effect != actionauth.EffectRead || b.Resource != "tool:converters.discover" || len(b.CredentialRefs) != 0 || b.GoalID != parent.GoalID || b.GoalRevision != parent.GoalRevision || b.MandateID != parent.MandateID || b.MandateRevision != parent.MandateRevision || b.Purpose != parent.Purpose || b.OwnerPolicyRevision != parent.OwnerPolicyRevision || actionauth.BindCredentialMetadata(b, b.CredentialMetadataHash).ActionHash != c.hash || b.ActionHash != c.hash || len(b.DataSources) != 1 {
			continue
		}
		r := b.DataSources[0]
		if r.ID == ref.ID && r.Revision == ref.Revision && r.AccessRevision == ref.AccessRevision && r.Hash == ref.Hash {
			return c.effect, nil
		}
	}
	return "", ErrPrerequisite
}

// Read accepted lifecycle without reviving the old parent lease. Caller must
// have taken task/goal locks; Catalog repeats full selected source grant ancestry.
func ReadDeveloperSnapshotTx(ctx context.Context, tx pgx.Tx, sources DeveloperSources, owner, childID string) (*DeveloperSnapshot, error) {
	var raw []byte
	var taskID, own, effect, auth, actionHash, argsHash, contractHash, snapshotHash string
	var revision, constraints int64
	err := tx.QueryRow(ctx, `SELECT owner_id,parent_task_id,parent_revision,parent_constraints_revision,effect_id,authorization_id,action_hash,args_hash,contract_hash,snapshot_hash,accepted_json FROM converter_developer_snapshots WHERE task_id=$1 FOR SHARE`, childID).Scan(&own, &taskID, &revision, &constraints, &effect, &auth, &actionHash, &argsHash, &contractHash, &snapshotHash, &raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var s DeveloperSnapshot
	permissions, err := DeveloperPermissions()
	hash, he := actionauth.ContractHash(DeveloperAdmissionContract())
	if err != nil || he != nil || json.Unmarshal(raw, &s) != nil || s.Version != 1 || s.OwnerID != owner || own != owner || s.ChildTaskID != childID || s.Binding.TaskID != taskID || s.Binding.TaskRevision != revision || s.Binding.ConstraintsRevision != constraints || s.EffectID != effect || s.AuthorizationID != auth || s.Binding.ActionHash != actionHash || s.Binding.ArgsHash != argsHash || s.Binding.ContractHash != contractHash || contractHash != hash || Hash(s) != snapshotHash || !reflect.DeepEqual(s.Permissions, permissions) {
		return nil, ErrAccess
	}
	args, e := DecodeDeveloperArgs(s.ArgsJSON)
	var current bool
	if s.Binding.OwnerID != owner || s.Binding.ToolName != DeveloperAdmissionTool || s.Binding.Resource != "tool:"+DeveloperAdmissionTool || s.Binding.Effect != actionauth.EffectPrepare || len(s.Binding.CredentialRefs) != 0 || s.ChildTaskRevision < 1 || s.ChildConstraintsRevision < 1 || tx.QueryRow(ctx, `SELECT TRUE FROM tasks WHERE task_id=$1 AND parent_task_id=$2 AND owner_id=$3 AND revision=$4 AND constraints_revision=$5 AND status='active' FOR SHARE`, childID, taskID, owner, s.ChildTaskRevision, s.ChildConstraintsRevision).Scan(&current) != nil {
		return nil, ErrAccess
	}
	actual, ae := actionauth.ArgumentsHash(s.ArgsJSON)
	if e != nil || ae != nil || actual != argsHash || !reflect.DeepEqual(args, s.Args) || actionauth.BindCredentialMetadata(s.Binding, s.Binding.CredentialMetadataHash).ActionHash != actionHash {
		return nil, ErrAccess
	}
	c, e := delegation.LoadTx(ctx, tx, childID)
	if e != nil || c == nil || c.ParentTaskID != taskID || c.ParentRevision != revision || c.ConstraintsRevision != constraints || !reflect.DeepEqual(c.Permissions, permissions) || catalog.SourceRef(c.ContractSource) != args.ContractSource || len(c.Sources) != 1 || catalog.SourceRef(c.Sources[0]) != args.NeedSource || c.Budget != delegation.Limits(args.Budget) || !c.Deadline.Equal(args.Deadline) {
		return nil, ErrAccess
	}
	if e = checkDeveloperPolicyTx(ctx, tx, s.Binding); e != nil {
		return nil, e
	}
	if e = sources.CheckDelegationSourcesTx(ctx, tx, childID, []catalog.SourceRef{args.NeedSource, args.ContractSource}, "memory_read"); e != nil {
		return nil, e
	}
	var now time.Time
	if tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now) != nil || !args.Deadline.After(now) {
		return nil, ErrAccess
	}
	a, e := actionauth.CheckTaskAuthorityTx(ctx, tx, taskID, owner, nil, now)
	if e != nil {
		return nil, e
	}
	m, _, e := actionauth.CheckAssignmentTx(ctx, tx, a, now)
	if e != nil || !actionauth.AllowsConverterAttachment(m, s.Permission, now) {
		return nil, ErrAccess
	}
	return &s, nil
}
