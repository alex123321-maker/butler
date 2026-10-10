package localmodels

import (
	"context"
	"encoding/csv"
	pb "github.com/butler/butler/internal/gen/localmodels/v1"
	"os/exec"
	"strconv"
	"strings"
)

// NvidiaSMI reports the whole selected device, including unrelated processes.
// Failure means GPU unavailable; it must never be interpreted as zero usage.
type NvidiaSMI struct {
	Binary string
	Device uint32
}

func (t NvidiaSMI) Snapshot(ctx context.Context) (*pb.GPUStatus, error) {
	binary := t.Binary
	if binary == "" {
		binary = "nvidia-smi"
	}
	output, err := exec.CommandContext(ctx, binary, "--id="+strconv.FormatUint(uint64(t.Device), 10), "--query-gpu=memory.total,memory.used,memory.free,name,driver_version", "--format=csv,noheader,nounits").Output()
	if err != nil {
		return &pb.GPUStatus{}, fault(Unavailable)
	}
	return parseGPU(string(output))
}
func parseGPU(data string) (*pb.GPUStatus, error) {
	rows, err := csv.NewReader(strings.NewReader(data)).ReadAll()
	if err != nil || len(rows) != 1 || len(rows[0]) != 5 {
		return &pb.GPUStatus{}, fault(Protocol)
	}
	values := make([]uint32, 3)
	for i := range values {
		n, err := strconv.ParseUint(strings.TrimSpace(rows[0][i]), 10, 32)
		if err != nil {
			return &pb.GPUStatus{}, fault(Protocol)
		}
		values[i] = uint32(n)
	}
	if values[0] == 0 || values[1] > values[0] || values[2] > values[0] {
		return &pb.GPUStatus{}, fault(Protocol)
	}
	return &pb.GPUStatus{Available: true, TotalMib: values[0], UsedMib: values[1], FreeMib: values[2], Name: strings.TrimSpace(rows[0][3]), Driver: strings.TrimSpace(rows[0][4])}, nil
}
