package skills

import (
	"context"
	"encoding/json"
	"testing"

	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/memory/catalog"
)

func TestReferencedAgentProposalRetainsOriginalDeletionParentIntegration(t *testing.T) {
	s, manifest, _, _, owner := skillFixture(t)
	p := packageOf(t, manifest)
	source, err := s.sources.Put(owner, catalog.PutSource{ScopeType: "user", ScopeID: "owner", Kind: "document", ExternalKey: "proposal-manifest", Title: "Candidate procedure", Body: string(p.Raw)})
	if err != nil {
		t.Fatal(err)
	}
	ref := catalog.SourceRef{ID: source.ID, Revision: source.Revision, AccessRevision: source.AccessRevision, Hash: source.Hash}
	routed := &RoutedExecutor{Store: s}
	var contract *toolbrokerv1.ToolContract
	for _, candidate := range Contracts() {
		if candidate.ToolName == "skills.propose" {
			contract = candidate
		}
	}
	args, _ := json.Marshal(map[string]any{"source_ref": ref})
	result, err := routed.Execute(context.Background(), &toolbrokerv1.ToolCall{RunId: "run", ToolCallId: "proposal", ToolName: "skills.propose", ArgsJson: string(args)}, contract, nil)
	if err != nil || result.Status != "completed" {
		t.Fatal("current referenced proposal rejected", err, result)
	}
	record, err := s.Get(owner, p.Digest)
	if err != nil || record.Status != "tested" || record.Active {
		t.Fatal("proposal created authority or lacked independent validation", err)
	}
	var edge bool
	if err = s.pool.QueryRow(owner, `SELECT EXISTS(SELECT 1 FROM memory_source_dependencies WHERE child_source_id=$1 AND parent_source_id=$2 AND parent_revision=$3)`, record.BodySource.ID, ref.ID, ref.Revision).Scan(&edge); err != nil || !edge {
		t.Fatal("proposal manifest source omitted from dependency graph", err)
	}
	oldArgs, _ := json.Marshal(map[string]string{"manifest_text": string(p.Raw)})
	result, err = routed.Execute(context.Background(), &toolbrokerv1.ToolCall{RunId: "run", ToolCallId: "raw-proposal", ToolName: "skills.propose", ArgsJson: string(oldArgs)}, contract, nil)
	if err != nil || result.Status != "failed" {
		t.Fatal("raw manifest entered immutable model action args", err)
	}
	if _, err = s.sources.Delete(owner, source.ID, "delete", source.Revision); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Get(owner, p.Digest); err == nil {
		t.Fatal("deleted original proposal source left a readable procedure")
	}
}
