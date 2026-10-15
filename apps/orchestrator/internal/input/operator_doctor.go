package input

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/jackc/pgx/v5"
)

// AcceptDoctorTx is an owner-control admission, never a model-visible input
// field. The task and receipt are created in the caller's single transaction.
func (s *Store) AcceptDoctorTx(ctx context.Context, tx pgx.Tx, event ingress.InputEvent, owner, taskID string) (Record, error) {
	a, ok := owneridentity.FromContext(ctx)
	if !ok || a.OwnerID != owner || a.Channel != "web" || a.SessionKey != event.SessionKey || taskID == "" {
		return Record{}, fmt.Errorf("authenticated diagnostic owner required")
	}
	r, created, err := s.AcceptTx(ctx, tx, event, owner, taskID)
	if err != nil {
		return r, err
	}
	if !created {
		return r, fmt.Errorf("diagnostic delivery already accepted")
	}
	_, err = tx.Exec(ctx, `UPDATE accepted_inputs SET checkpoint_json=checkpoint_json||'{"operator_operation":"doctor.check_system"}'::jsonb WHERE input_id=$1`, r.InputID)
	return r, err
}

func doctorAdmission(raw string) bool {
	var v struct {
		Operation string `json:"operator_operation"`
	}
	return json.Unmarshal([]byte(raw), &v) == nil && v.Operation == "doctor.check_system"
}

func (s *Store) GetDoctor(ctx context.Context, id string) (Record, error) {
	a, ok := owneridentity.FromContext(ctx)
	if !ok {
		return Record{}, fmt.Errorf("authenticated owner required")
	}
	r, err := s.Get(ctx, id)
	if err != nil {
		return r, err
	}
	if r.OwnerID != a.OwnerID || !doctorAdmission(r.CheckpointJSON) {
		return Record{}, fmt.Errorf("diagnostic request unavailable")
	}
	return r, nil
}
