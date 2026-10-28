package maintenance

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/rand"
	"debug/elf"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

var ErrBuild = errors.New("independent maintenance build unavailable")

// DockerBuilder is private checker configuration. Neither image nor module can
// come from a proposed patch. Submitted Dockerfiles, scripts and tests are not
// executed. The first bounded compiler supports dependency-free Go capsules;
// absent dependencies fail closed with networking and module downloads disabled.
type DockerBuilder struct {
	client *http.Client
	image  string
	module string
}

// BuiltExecutable measures the actual ELF. It is not a deployment image or a
// passing independent review and grants no authority to run or install code.
type BuiltExecutable struct {
	Commit          string      `json:"commit"`
	InventorySHA256 string      `json:"inventory_sha256"`
	CompilerSHA256  string      `json:"compiler_sha256"`
	Module          string      `json:"module"`
	ELFSHA256       string      `json:"elf_sha256"`
	Bytes           int         `json:"bytes"`
	Bounds          BuildBounds `json:"bounds"`
	elf             []byte
}

func (built BuiltExecutable) ELF() []byte { return bytes.Clone(built.elf) }

func NewDockerBuilder(socket, compilerSHA256, module string) (*DockerBuilder, error) {
	if !validDigest(compilerSHA256) || !validSourcePath(module) {
		return nil, ErrBuild
	}
	inspector, err := NewDockerInspector(socket, "maintenance-builder")
	if err != nil {
		return nil, ErrBuild
	}
	return &DockerBuilder{client: inspector.client, image: compilerSHA256, module: module}, nil
}

func (builder *DockerBuilder) request(ctx context.Context, method, target, contentType string, body []byte, limit int64, statuses ...int) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, method, "http://docker"+target, bytes.NewReader(body))
	if err != nil {
		return nil, ErrBuild
	}
	request.Header.Set("Content-Type", contentType)
	response, err := builder.client.Do(request)
	if err != nil {
		return nil, ErrBuild
	}
	defer response.Body.Close()
	accepted := false
	for _, status := range statuses {
		accepted = accepted || response.StatusCode == status
	}
	if !accepted {
		return nil, ErrBuild // Daemon error bodies/configuration stay private.
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil || int64(len(raw)) > limit {
		return nil, ErrBuild
	}
	return raw, nil
}

func (builder *DockerBuilder) json(ctx context.Context, method, target string, value any, statuses ...int) ([]byte, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, ErrBuild
	}
	return builder.request(ctx, method, target, "application/json", raw, 256<<10, statuses...)
}

// sourceArchive rechecks the independently prepared inventory before copying.
// It never follows a symlink or sends a host path to the daemon. Every submitted
// file, including author tests/configuration, remains in the measured source.
func sourceArchive(snapshot *SourceSnapshot) ([]byte, error) {
	if snapshot == nil || snapshot.directory == "" || !gitObjectID(snapshot.Commit) {
		return nil, ErrBuild
	}
	_, inventory, err := sourceInventory(snapshot.Files)
	if err != nil || inventory != snapshot.InventorySHA256 || len(snapshot.Files) > 8192 {
		return nil, ErrBuild
	}
	resolved, err := filepath.EvalSymlinks(snapshot.directory)
	if err != nil || resolved != snapshot.directory {
		return nil, ErrBuild
	}
	var buffer bytes.Buffer
	archive := tar.NewWriter(&buffer)
	var total int64
	for _, file := range snapshot.Files {
		if file.Bytes < 0 || file.Bytes > 64<<20 || total > (64<<20)-file.Bytes || (file.Mode != "100644" && file.Mode != "100755") {
			return nil, ErrBuild
		}
		total += file.Bytes
		name := filepath.Join(snapshot.directory, filepath.FromSlash(file.Path))
		// Every parent must still be a directory, with no symlink traversal.
		parent := filepath.Dir(name)
		for parent != snapshot.directory {
			info, err := os.Lstat(parent)
			if err != nil || !info.IsDir() || outside(snapshot.directory, parent) {
				return nil, ErrBuild
			}
			parent = filepath.Dir(parent)
		}
		info, err := os.Lstat(name)
		if err != nil || !info.Mode().IsRegular() || info.Size() != file.Bytes {
			return nil, ErrBuild
		}
		input, err := os.Open(name)
		if err != nil {
			return nil, ErrBuild
		}
		opened, statErr := input.Stat()
		raw, readErr := io.ReadAll(io.LimitReader(input, file.Bytes+1))
		closeErr := input.Close()
		if statErr != nil || !os.SameFile(info, opened) || readErr != nil || closeErr != nil {
			return nil, ErrBuild
		}
		if int64(len(raw)) != file.Bytes || Digest(raw) != file.SHA256 {
			return nil, ErrBuild
		}
		mode := int64(0444)
		if file.Mode == "100755" {
			mode = 0555
		}
		if archive.WriteHeader(&tar.Header{Name: file.Path, Mode: mode, Size: file.Bytes, Typeflag: tar.TypeReg, Uid: 65532, Gid: 65532}) != nil {
			return nil, ErrBuild
		}
		if _, err := archive.Write(raw); err != nil {
			return nil, ErrBuild
		}
	}
	if archive.Close() != nil {
		return nil, ErrBuild
	}
	return buffer.Bytes(), nil
}

func dockerBuildID(raw []byte, field string) (string, error) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return "", ErrBuild
	}
	var id string
	if json.Unmarshal(fields[field], &id) != nil || !validDigest(id) {
		return "", ErrBuild
	}
	return id, nil
}

const fixedBuildSupervisor = `n=0
while [ ! -f /out/build-authorized ]; do
 n=$((n+1))
 [ "$n" -le 30 ] || exit 124
 sleep 1
done
set +e
timeout -s KILL 120 /usr/local/go/bin/go build -trimpath -buildvcs=false -o /out/candidate . > /out/compiler.log 2>&1
code=$?
printf '%s\n' "$code" > /out/.build-exit.pending
mv /out/.build-exit.pending /out/build-exit
sleep 30
`

func buildContainer(image, name, source, module string, writable bool) map[string]any {
	mode := ":ro"
	if writable {
		mode = ":rw"
	}
	return map[string]any{
		"Image": "sha256:" + image, "User": "65532:65532", "Hostname": "maintenance-build",
		"WorkingDir": "/src/" + module, "Cmd": []string{"/bin/sh", "-ec", fixedBuildSupervisor},
		"Labels": map[string]string{"butler.maintenance.build": name},
		"Env":    []string{"PATH=/usr/local/go/bin:/usr/bin:/bin", "HOME=/work", "CGO_ENABLED=0", "GOOS=linux", "GOARCH=amd64", "GOMAXPROCS=2", "GOTOOLCHAIN=local", "GOENV=off", "GOWORK=off", "GOFLAGS=", "GOPROXY=off", "GOSUMDB=off", "GOCACHE=/work/cache", "GOTMPDIR=/work", "GOPATH=/work/gopath", "GOMODCACHE=/work/modules"},
		"HostConfig": map[string]any{
			"NetworkMode": "none", "ReadonlyRootfs": true, "CapDrop": []string{"ALL"},
			"SecurityOpt": []string{"no-new-privileges:true"}, "PidsLimit": 128,
			"Memory": 1 << 30, "MemorySwap": 1 << 30, "NanoCpus": 1_000_000_000,
			"Binds":     []string{source + ":/src" + mode},
			"Tmpfs":     map[string]string{"/work": "rw,nosuid,nodev,noexec,size=536870912,uid=65532,gid=65532,mode=0700", "/out": "rw,nosuid,nodev,noexec,size=33554432,uid=65532,gid=65532,mode=0700"},
			"LogConfig": map[string]any{"Type": "none"},
		},
	}
}

// Build compiles in a fixed, disposable, bounded container. It never executes
// the resulting ELF. Private daemon access remains exclusively in the checker.
func (builder *DockerBuilder) Build(ctx context.Context, snapshot *SourceSnapshot) (result BuiltExecutable, err error) {
	if builder == nil || builder.client == nil || !validDigest(builder.image) || !validSourcePath(builder.module) {
		return result, ErrBuild
	}
	archive, err := sourceArchive(snapshot)
	if err != nil {
		return result, ErrBuild
	}
	ctx, cancel := context.WithTimeout(ctx, 180*time.Second)
	defer cancel()
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return result, ErrBuild
	}
	name := "butler-build-" + hex.EncodeToString(nonce[:])
	volume := name + "-source"
	// Cleanup is narrow and best-effort; failure never creates a successful build.
	var containers []string
	volumeCreated := false
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		for _, id := range containers {
			if _, e := builder.request(cleanup, http.MethodDelete, "/containers/"+id+"?force=1&v=1", "", nil, 1024, 204); e != nil {
				err = ErrBuild
			}
		}
		if volumeCreated {
			if _, e := builder.request(cleanup, http.MethodDelete, "/volumes/"+volume, "", nil, 1024, 204); e != nil {
				err = ErrBuild
			}
		}
		if err != nil {
			result = BuiltExecutable{}
		}
	}()
	volumeRaw, err := builder.json(ctx, http.MethodPost, "/volumes/create", map[string]any{"Name": volume, "Driver": "local", "Labels": map[string]string{"butler.maintenance.build": name}}, 201)
	if err != nil {
		return result, ErrBuild
	}
	var createdVolume struct {
		Name, Driver string
		Labels       map[string]string
	}
	if json.Unmarshal(volumeRaw, &createdVolume) != nil || createdVolume.Name != volume || createdVolume.Driver != "local" || createdVolume.Labels["butler.maintenance.build"] != name {
		return result, ErrBuild
	}
	volumeCreated = true
	var worker string
	for _, stage := range []bool{true, false} {
		raw, e := builder.json(ctx, http.MethodPost, "/containers/create", buildContainer(builder.image, name, volume, builder.module, stage), 201)
		if e != nil {
			return result, ErrBuild
		}
		id, e := dockerBuildID(raw, "Id")
		if e != nil {
			return result, ErrBuild
		}
		containers = append(containers, id)
		if stage {
			if _, e = builder.request(ctx, http.MethodPut, "/containers/"+id+"/archive?path=/src", "application/x-tar", archive, 1024, 200); e != nil {
				return result, ErrBuild
			}
		} else {
			worker = id
		}
	}
	if _, err = builder.request(ctx, http.MethodPost, "/containers/"+worker+"/start", "", nil, 1024, 204); err != nil {
		return result, ErrBuild
	}
	bounds, err := builder.measureBuildBounds(ctx, worker)
	if err != nil {
		return result, ErrBuild
	}
	// Pin the actual private worker's tmpfs. Docker archive APIs do not expose
	// running tmpfs mounts on every daemon. os.Root confines all following access
	// to this verified process's /out, never an author-selected host path.
	output, err := os.OpenRoot("/proc/" + strconv.FormatInt(bounds.pid, 10) + "/root/out")
	if err != nil {
		return result, ErrBuild
	}
	defer output.Close()
	permit, err := output.OpenFile("build-authorized", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0444)
	if err != nil {
		return result, ErrBuild
	}
	if permit.Close() != nil {
		return result, ErrBuild
	}
	for {
		finished, code, err := buildExit(output)
		if err != nil {
			return result, ErrBuild
		}
		if finished {
			if code != "0\n" {
				return result, ErrBuild
			}
			break
		}
		select {
		case <-ctx.Done():
			return result, ErrBuild
		case <-time.After(250 * time.Millisecond):
		}
	}
	executable, err := readBuildOutput(output, "candidate", 31<<20)
	if err != nil {
		return result, ErrBuild
	}
	if validateBuiltELF(executable) != nil {
		return result, ErrBuild
	}
	return BuiltExecutable{Commit: snapshot.Commit, InventorySHA256: snapshot.InventorySHA256, CompilerSHA256: builder.image, Module: builder.module, ELFSHA256: Digest(executable), Bytes: len(executable), Bounds: bounds, elf: executable}, nil
}

func buildExit(output *os.Root) (bool, string, error) {
	code, err := readBuildOutput(output, "build-exit", 4)
	if os.IsNotExist(err) {
		return false, "", nil
	}
	if err != nil || len(code) < 2 {
		return false, "", ErrBuild
	}
	return true, string(code), nil
}

func readBuildOutput(output *os.Root, name string, limit int64) ([]byte, error) {
	if name != "candidate" && name != "build-exit" {
		return nil, ErrBuild
	}
	file, err := output.Open(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > limit {
		return nil, ErrBuild
	}
	raw, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil || int64(len(raw)) > limit {
		return nil, ErrBuild
	}
	return raw, nil
}

func builtELF(raw []byte) ([]byte, error) {
	reader := tar.NewReader(bytes.NewReader(raw))
	header, err := reader.Next()
	if err != nil || header.Name != "candidate" || header.Typeflag != tar.TypeReg || header.Size < 64 || header.Size > 31<<20 || header.Linkname != "" {
		return nil, ErrBuild
	}
	executable, err := io.ReadAll(io.LimitReader(reader, header.Size+1))
	if err != nil || int64(len(executable)) != header.Size {
		return nil, ErrBuild
	}
	if _, err = reader.Next(); err != io.EOF {
		return nil, ErrBuild
	}
	if validateBuiltELF(executable) != nil {
		return nil, ErrBuild
	}
	return executable, nil
}

func validateBuiltELF(executable []byte) error {
	if len(executable) < 64 || len(executable) > 31<<20 {
		return ErrBuild
	}
	image, err := elf.NewFile(bytes.NewReader(executable))
	if err != nil {
		return ErrBuild
	}
	defer image.Close()
	if image.Class != elf.ELFCLASS64 || image.Machine != elf.EM_X86_64 || image.Type != elf.ET_EXEC {
		return ErrBuild
	}
	for _, program := range image.Progs {
		if program.Type == elf.PT_INTERP {
			return ErrBuild
		}
	}
	return nil
}
