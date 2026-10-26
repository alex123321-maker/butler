package maintenance

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// BuildBounds is measured by the private host before executing the compiler.
// Docker configuration alone cannot prove limits were applied by the kernel.
// No environment, host pathname, signing material or author verdict is exported.
type BuildBounds struct {
	ContainerID      string    `json:"container_id"`
	CompilerSHA256   string    `json:"compiler_sha256"`
	UID              int64     `json:"uid"`
	MemoryBytes      int64     `json:"memory_bytes"`
	PIDs             int64     `json:"pids"`
	CPU              string    `json:"cpu"`
	EffectiveSwapMax int64     `json:"effective_swap_max"`
	EffectiveCaps    string    `json:"effective_caps"`
	NoNewPrivileges  bool      `json:"no_new_privileges"`
	MeasuredAt       time.Time `json:"measured_at"`
	pid              int64
}

func boundedHostRead(name string, limit int64) ([]byte, error) {
	file, err := os.Open(name)
	if err != nil {
		return nil, ErrBuild
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil || int64(len(raw)) > limit {
		return nil, ErrBuild
	}
	return raw, nil
}

// effectiveZeroSwap accepts a real zero at the container or any ancestor.
// This supports the isolated host where Docker ignores MemorySwap but its
// private ancestor cgroup actually forbids swap. It never treats "max" as zero.
func effectiveZeroSwap(root, group string) error {
	if !path.IsAbs(group) || path.Clean(group) != group || strings.ContainsAny(group, "\\\x00") {
		return ErrBuild
	}
	for depth := 0; depth < 32; depth++ {
		raw, err := boundedHostRead(filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(group, "/")), "memory.swap.max"), 128)
		if err != nil {
			return ErrBuild
		}
		value := strings.TrimSpace(string(raw))
		if value == "0" {
			return nil
		}
		if value != "max" {
			limit, err := strconv.ParseInt(value, 10, 64)
			if err != nil || limit < 0 {
				return ErrBuild
			}
		}
		if group == "/" {
			break
		}
		group = path.Dir(group)
	}
	return ErrBuild
}

// This initial compiler profile requires Linux cgroup v2/cgroupfs and host PID
// visibility. Unsupported or hidden resource state fails before source build.
func (builder *DockerBuilder) measureBuildBounds(ctx context.Context, id string) (BuildBounds, error) {
	var bounds BuildBounds
	if runtime.GOOS != "linux" || !validDigest(id) {
		return bounds, ErrBuild
	}
	raw, err := builder.request(ctx, http.MethodGet, "/containers/"+id+"/json", "", nil, 256<<10, 200)
	var inspected struct {
		ID    string `json:"Id"`
		Image string
		State struct {
			Pid     int64
			Running bool
		}
		Config     struct{ User string }
		HostConfig struct {
			NetworkMode    string
			ReadonlyRootfs bool
			Privileged     bool
		}
	}
	if err != nil || json.Unmarshal(raw, &inspected) != nil || inspected.ID != id || inspected.Image != "sha256:"+builder.image || !inspected.State.Running || inspected.State.Pid < 1 || inspected.Config.User != "65532:65532" || inspected.HostConfig.NetworkMode != "none" || !inspected.HostConfig.ReadonlyRootfs || inspected.HostConfig.Privileged {
		return bounds, ErrBuild
	}
	proc := "/proc/" + strconv.FormatInt(inspected.State.Pid, 10)
	raw, err = boundedHostRead(proc+"/cgroup", 4096)
	if err != nil {
		return bounds, ErrBuild
	}
	line := strings.TrimSpace(string(raw))
	if strings.ContainsRune(line, '\n') || !strings.HasPrefix(line, "0::/") {
		return bounds, ErrBuild
	}
	group := strings.TrimPrefix(line, "0::")
	if path.Base(group) != id || !path.IsAbs(group) || path.Clean(group) != group || strings.ContainsAny(group, "\\\x00") {
		return bounds, ErrBuild
	}
	root := "/sys/fs/cgroup"
	for field, expected := range map[string]string{"memory.max": "1073741824", "pids.max": "128", "cpu.max": "100000 100000"} {
		actual, err := boundedHostRead(filepath.Join(root, strings.TrimPrefix(group, "/"), field), 128)
		if err != nil || strings.TrimSpace(string(actual)) != expected {
			return bounds, ErrBuild
		}
	}
	if effectiveZeroSwap(root, group) != nil {
		return bounds, ErrBuild
	}
	raw, err = boundedHostRead(proc+"/status", 64<<10)
	if err != nil {
		return bounds, ErrBuild
	}
	fields := map[string]string{}
	for _, line := range strings.Split(string(raw), "\n") {
		key, value, ok := strings.Cut(line, ":")
		if ok {
			fields[key] = strings.TrimSpace(value)
		}
	}
	if strings.Join(strings.Fields(fields["Uid"]), " ") != "65532 65532 65532 65532" || fields["CapEff"] != "0000000000000000" || fields["NoNewPrivs"] != "1" {
		return bounds, ErrBuild
	}
	routes, err := boundedHostRead(proc+"/net/route", 16<<10)
	if err != nil || len(bytes.Split(bytes.TrimSpace(routes), []byte("\n"))) != 1 {
		return bounds, ErrBuild
	}
	return BuildBounds{ContainerID: id, CompilerSHA256: builder.image, UID: 65532, MemoryBytes: 1 << 30, PIDs: 128, CPU: "100000 100000", EffectiveSwapMax: 0, EffectiveCaps: fields["CapEff"], NoNewPrivileges: true, MeasuredAt: time.Now().UTC().Truncate(time.Microsecond), pid: inspected.State.Pid}, nil
}
