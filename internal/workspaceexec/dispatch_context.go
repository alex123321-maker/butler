package workspaceexec

import (
	"bytes"
	"encoding/json"
	"io"

	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/workspace"
)

// This wire exists only in immutable kernel PostgreSQL rows. It is never an
// argument, runtime result, model context or caller-supplied ownership shortcut.
type storedContext struct {
	WorkspaceID         string              `json:"workspace_id"`
	OwnerID             string              `json:"owner_id"`
	SessionKey          string              `json:"session_key"`
	TaskID              string              `json:"task_id"`
	RunID               string              `json:"run_id"`
	TaskRevision        int64               `json:"task_revision"`
	ConstraintsRevision int64               `json:"constraints_revision"`
	Previous            int64               `json:"previous"`
	Parents             []catalog.SourceRef `json:"parents"`
}

func encodeContext(c SourceContext) ([]byte, error) {
	b := c.ref.Binding
	if !opaqueID(c.ref.ID) || c.run == "" || b.OwnerID == "" || b.SessionKey == "" || b.TaskID == "" || b.TaskRevision < 1 || b.ConstraintsRevision < 1 || c.previous < 0 || len(c.parents) < 1 || len(c.parents) > 64 {
		return nil, ErrDenied
	}
	return json.Marshal(storedContext{c.ref.ID, b.OwnerID, b.SessionKey, b.TaskID, c.run, b.TaskRevision, b.ConstraintsRevision, c.previous, c.parents})
}
func decodeContext(raw []byte) (SourceContext, error) {
	if len(raw) > 64*1024 {
		return SourceContext{}, ErrDenied
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	var c storedContext
	if d.Decode(&c) != nil {
		return SourceContext{}, ErrDenied
	}
	if _, err := d.Token(); err != io.EOF {
		return SourceContext{}, ErrDenied
	}
	result := SourceContext{ref: workspace.Ref{ID: c.WorkspaceID, Binding: workspace.Binding{OwnerID: c.OwnerID, SessionKey: c.SessionKey, TaskID: c.TaskID, TaskRevision: c.TaskRevision, ConstraintsRevision: c.ConstraintsRevision}}, parents: c.Parents, previous: c.Previous, run: c.RunID}
	if _, err := encodeContext(result); err != nil {
		return SourceContext{}, ErrDenied
	}
	seen := map[string]bool{}
	for _, r := range result.parents {
		if r.ID == "" || r.Revision < 1 || r.AccessRevision < 1 || !sha256Digest(r.Hash) || seen[r.ID] {
			return SourceContext{}, ErrDenied
		}
		seen[r.ID] = true
	}
	return result, nil
}
