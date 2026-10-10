package localmodels

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ProcMemory sums the complete resident processes in the service PID namespace,
// conservatively counting shared pages twice. Host MemAvailable and a finite
// cgroup remaining allowance prevent CPU fallback from consuming host reserves.
// Run in its dedicated container, never in the host PID namespace.
type ProcMemory struct{ Root string }

func (p ProcMemory) SnapshotRAM(ctx context.Context) (RAMStatus, error) {
	root := p.Root
	if root == "" {
		root = "/proc"
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return RAMStatus{}, fault(Resources)
	}
	var used uint64
	for _, entry := range entries {
		if ctx.Err() != nil {
			return RAMStatus{}, ctx.Err()
		}
		if _, err := strconv.ParseUint(entry.Name(), 10, 32); err != nil {
			continue
		}
		data, err := os.ReadFile(filepath.Join(root, entry.Name(), "status"))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return RAMStatus{}, fault(Resources)
		}
		used += procKiB(string(data), "VmRSS:")
	}
	data, err := os.ReadFile(filepath.Join(root, "meminfo"))
	if err != nil {
		return RAMStatus{}, fault(Resources)
	}
	available := procKiB(string(data), "MemAvailable:")
	if available == 0 {
		return RAMStatus{}, fault(Resources)
	}
	if root == "/proc" {
		maxData, maxErr := os.ReadFile("/sys/fs/cgroup/memory.max")
		current, currentErr := os.ReadFile("/sys/fs/cgroup/memory.current")
		if maxErr == nil && currentErr == nil && strings.TrimSpace(string(maxData)) != "max" {
			maximum, err1 := strconv.ParseUint(strings.TrimSpace(string(maxData)), 10, 64)
			now, err2 := strconv.ParseUint(strings.TrimSpace(string(current)), 10, 64)
			if err1 != nil || err2 != nil || now >= maximum {
				return RAMStatus{}, fault(Resources)
			}
			available = min(available, (maximum-now)/1024)
		}
	}
	return RAMStatus{UsedMiB: uint32((used + 1023) / 1024), AvailableMiB: uint32(available / 1024)}, nil
}
func procKiB(data, key string) uint64 {
	for _, line := range strings.Split(data, "\n") {
		if strings.HasPrefix(line, key) {
			fields := strings.Fields(line)
			if len(fields) == 3 && fields[2] == "kB" {
				n, _ := strconv.ParseUint(fields[1], 10, 64)
				return n
			}
		}
	}
	return 0
}
