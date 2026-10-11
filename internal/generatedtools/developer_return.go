package generatedtools

import (
	"context"
	"encoding/json"
	"reflect"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/jackc/pgx/v5"
)

// DeveloperReturn is a technical, reference-only projection. It grants neither
// task completion nor the child's report/session/narrative as parent context.
type DeveloperReturn struct {
	Version        int               `json:"version"`
	SnapshotHash   string            `json:"snapshot_hash"`
	OwnerID        string            `json:"owner_id"`
	ParentTaskID   string            `json:"parent_task_id"`
	ChildTaskID    string            `json:"child_task_id"`
	ChildRunID     string            `json:"child_run_id"`
	ResultID       string            `json:"result_id"`
	FinalHash      string            `json:"final_hash"`
	ReportSource   catalog.SourceRef `json:"report_source"`
	FinalSource    catalog.SourceRef `json:"final_source"`
	ContractSource catalog.SourceRef `json:"contract_source"`
	Candidate      Record            `json:"candidate"`
	TestEffectID   string            `json:"test_effect_id"`
	TestReport     Report            `json:"test_report"`
}

// ReadTestedDeveloperCandidateTx reads only owning immutable package/check
// records. Catalog currentness and full privacy ancestry are separate mandatory
// guards, composed by the publication/read caller on this same transaction.
func ReadTestedDeveloperCandidateTx(ctx context.Context, tx pgx.Tx, snapshot DeveloperSnapshot, runID string) (Record, string, Report, error) {
	var digest string
	rows, err := tx.Query(ctx, `SELECT digest FROM converter_versions WHERE owner_id=$1 AND created_by_run=$2 AND converter_id=$3 AND version=$4 AND need_id=$5 AND need_hash=$6 AND profile_pin=$7 ORDER BY digest LIMIT 2`, snapshot.OwnerID, runID, snapshot.Permission.ConverterID, snapshot.Spec.Version, snapshot.NeedID, snapshot.NeedHash, snapshot.Permission.RuntimePin)
	if err != nil {
		return Record{}, "", Report{}, err
	}
	count := 0
	for rows.Next() {
		count++
		if err = rows.Scan(&digest); err != nil {
			break
		}
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil || count != 1 {
		return Record{}, "", Report{}, ErrAccess
	}
	s := Store{owner: snapshot.OwnerID}
	r, err := s.metadataTx(ctx, tx, digest)
	if err != nil || r.NeedSource != snapshot.Args.NeedSource || r.Shape != snapshot.Permission || r.Status != "tested" && r.Status != "active" && r.Status != "obsolete" {
		return Record{}, "", Report{}, ErrAccess
	}
	var raw []byte
	var effect string
	if tx.QueryRow(ctx, `SELECT v.action_id,v.report_json FROM converter_validations v JOIN action_effects e ON e.action_id=v.action_id WHERE v.owner_id=$1 AND v.digest=$2 AND v.run_id=$3 AND e.state='effect_confirmed' AND v.report_json->>'passed'='true' ORDER BY v.validation_id DESC LIMIT 1`, r.OwnerID, r.Digest, runID).Scan(&effect, &raw) != nil {
		return Record{}, "", Report{}, ErrChecks
	}
	var report Report
	if json.Unmarshal(raw, &report) != nil || !report.Passed || report.PackageHash != r.Digest || report.NeedHash != r.NeedHash || report.ProfileHash != r.ProfilePin || report.Checker != Checker {
		return Record{}, "", Report{}, ErrChecks
	}
	var confirmed bool
	reportHash, err := observationHash(report)
	if err != nil {
		return Record{}, "", Report{}, ErrChecks
	}
	if tx.QueryRow(ctx, `SELECT TRUE FROM action_intents i JOIN action_effect_dispatches d ON d.execution_action_id=i.action_id JOIN action_authorizations a ON a.authorization_id=d.authorization_id WHERE i.run_id=$1 AND i.effect_id=$2 AND i.binding->>'tool_name'='converters.test' AND i.binding->>'converter_package_hash'=$3 AND d.outcome='dispatch_completed' AND a.state='dispatch_completed' AND EXISTS(SELECT 1 FROM action_effect_evidence x WHERE x.action_id=$2 AND x.source_action_id=i.action_id AND x.evidence->>'source'='runtime_response' AND x.evidence->>'method'='local_runtime_ack' AND x.evidence->>'verdict'='effect_confirmed' AND x.evidence->>'result_digest'=$4 AND x.evidence->>'source_run_id'=$1 AND x.evidence->>'source_tool_call_id'=i.tool_call_id)`, runID, effect, r.Digest, reportHash).Scan(&confirmed) != nil {
		return Record{}, "", Report{}, ErrChecks
	}
	return r, effect, report, nil
}

func ReadDeveloperReturnTx(ctx context.Context, tx pgx.Tx, snapshot DeveloperSnapshot) (DeveloperReturn, error) {
	var p DeveloperReturn
	var raw []byte
	var hash string
	if tx.QueryRow(ctx, `SELECT projection_json,projection_hash FROM converter_developer_returns WHERE child_task_id=$1 FOR SHARE`, snapshot.ChildTaskID).Scan(&raw, &hash) != nil || json.Unmarshal(raw, &p) != nil || Hash(p) != hash || p.Version != 1 || p.SnapshotHash != Hash(snapshot) || p.OwnerID != snapshot.OwnerID || p.ParentTaskID != snapshot.Binding.TaskID || p.ChildTaskID != snapshot.ChildTaskID || p.ContractSource != snapshot.Args.ContractSource {
		return p, ErrAccess
	}
	var finalHash, reportID, messageID string
	var reportRef catalog.SourceRef
	var evidence []byte
	if tx.QueryRow(ctx, `SELECT f.content_hash,d.decision_evidence_json,f.message_id FROM task_delegations d JOIN run_final_results f ON f.result_id=d.result_id JOIN runs r ON r.run_id=f.run_id JOIN task_results t ON t.result_id=f.result_id WHERE d.task_id=$1 AND d.state='result_ready' AND d.result_id=$2 AND f.run_id=$3 AND f.task_id=$1 AND f.task_revision=$4 AND f.constraints_revision=$5 AND f.answer_state='final' AND NOT f.privacy_redacted AND r.current_state='completed' AND NOT t.privacy_redacted`, p.ChildTaskID, p.ResultID, p.ChildRunID, snapshot.ChildTaskRevision, snapshot.ChildConstraintsRevision).Scan(&finalHash, &evidence, &messageID) != nil || finalHash != p.FinalHash || p.FinalSource.ID != "message:"+messageID {
		return p, ErrAccess
	}
	var saved struct {
		Report *catalog.SourceRef `json:"report_source"`
	}
	if json.Unmarshal(evidence, &saved) != nil || saved.Report == nil {
		return p, ErrAccess
	}
	reportRef = *saved.Report
	reportID = reportRef.ID
	if reportID == "" || reportRef != p.ReportSource {
		return p, ErrAccess
	}
	r, effect, report, err := ReadTestedDeveloperCandidateTx(ctx, tx, snapshot, p.ChildRunID)
	// Catalog generation/status may change through separately authorized parent
	// activation/rollback. The returned immutable package identity stays exact.
	r.Generation, r.Status, r.Active = p.Candidate.Generation, p.Candidate.Status, p.Candidate.Active
	if err != nil || Hash(r) != Hash(p.Candidate) || effect != p.TestEffectID || !reflect.DeepEqual(report, p.TestReport) {
		return p, ErrAccess
	}
	return p, nil
}

func developerReturnSources(p DeveloperReturn) []catalog.SourceRef {
	return []catalog.SourceRef{p.Candidate.BodySource, p.Candidate.NeedSource, p.ContractSource}
}

// Validate only the three actual retained child actions. The immutable final's
// matching effect journal is checked by the owning report projection as well.
func CheckDeveloperFinalActionsTx(ctx context.Context, tx pgx.Tx, snapshot DeveloperSnapshot, runID string) error {
	rows, err := tx.Query(ctx, `SELECT i.binding,e.state FROM action_intents i JOIN action_effects e ON e.action_id=i.effect_id WHERE i.run_id=$1 ORDER BY i.action_id`, runID)
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for rows.Next() {
		var raw []byte
		var state string
		var b actionauth.ActionBinding
		if err = rows.Scan(&raw, &state); err != nil {
			break
		}
		if json.Unmarshal(raw, &b) != nil || state != string(actionauth.StateConfirmed) || b.TaskID != snapshot.ChildTaskID || b.TaskRevision != snapshot.ChildTaskRevision || b.ConstraintsRevision != snapshot.ChildConstraintsRevision || b.OwnerID != snapshot.OwnerID || len(b.CredentialRefs) != 0 {
			err = ErrAccess
			break
		}
		valid := false
		for _, p := range snapshot.Permissions {
			if b.ToolName == p.Tool && b.ContractHash == p.ContractHash && b.Resource == p.Resources[0] && string(b.Effect) == p.Effects[0] {
				valid = true
				seen[p.Tool] = true
				break
			}
		}
		if !valid {
			err = ErrAccess
			break
		}
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return err
	}
	if len(seen) != 3 {
		return ErrAccess
	}
	return nil
}
