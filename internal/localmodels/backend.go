// Package localmodels owns auxiliary model resources, not runs, memory or tools.
package localmodels

import (
	"context"
	"errors"

	pb "github.com/butler/butler/internal/gen/localmodels/v1"
)

type ErrorKind string

const (
	Invalid     ErrorKind = "invalid_input"
	Unavailable ErrorKind = "model_unavailable"
	Resources   ErrorKind = "insufficient_resources"
	OOM         ErrorKind = "out_of_memory"
	Protocol    ErrorKind = "invalid_model_output"
	Closed      ErrorKind = "manager_stopped"
	QueueFull   ErrorKind = "queue_full"
)

// Fault deliberately carries no provider response, input text or secret.
type Fault struct{ Kind ErrorKind }

func (e *Fault) Error() string   { return string(e.Kind) }
func fault(kind ErrorKind) error { return &Fault{Kind: kind} }
func isFault(err error, kind ErrorKind) bool {
	var f *Fault
	return errors.As(err, &f) && f.Kind == kind
}

type Resident struct {
	Model   string
	VRAMMiB uint32
	RAMMiB  uint32
}

type RAMStatus struct{ UsedMiB, AvailableMiB uint32 }
type RAMTelemetry interface {
	SnapshotRAM(context.Context) (RAMStatus, error)
}
type Backend interface {
	Healthy(context.Context) bool
	Verify(context.Context, *pb.ModelProfile) error
	Resident(context.Context) ([]Resident, error)
	Infer(context.Context, *pb.ModelProfile, *pb.InferRequest, string, uint32) (*pb.InferResponse, error)
	Unload(context.Context, string) error
}
type Telemetry interface {
	Snapshot(context.Context) (*pb.GPUStatus, error)
}
