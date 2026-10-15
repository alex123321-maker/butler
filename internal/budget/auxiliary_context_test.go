package budget

import (
	"context"
	"errors"
	"testing"
)

func TestAuxiliaryCrossContextRejectsAmbiguousTopMandate(t *testing.T) {
	c := NewController(nil, DefaultPolicy())
	_, err := c.ReserveAuxiliaryTx(context.Background(), nil, AuxiliaryIdentity{OwnerID: "owner", JobID: "job", OperationID: "operation", InputBytes: 1, Tokens: 1, ActiveMillis: 1, MandateID: "ambiguous-top-level", AdditionalTasks: []AuxiliaryTaskIdentity{{TaskID: "actual-root", MandateID: "actual-assignment"}}})
	if !errors.Is(err, ErrInvalid) {
		t.Fatal("ambiguous top-level scope accepted", err)
	}
}
