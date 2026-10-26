package maintenance

import (
	"archive/tar"
	"bytes"
	"context"
	"debug/elf"
	"io"
	"net/http"
	"time"
)

const fixedGuardedPackage = "butler.maintenance.guarded-scratch-elf.v1\n/candidate;/maintenance-guard;uid=65532;gid=65532;mode=0555;entrypoint=/maintenance-guard;workdir=/\n"

// GuardedPackager is private checker configuration with one immutable trusted
// guard. The author submits only candidate source, never guard bytes or paths.
type GuardedPackager struct {
	builder *DockerBuilder
	guard   []byte
	guardID string
}

type GuardedImage struct {
	BuiltImage
	GuardELFSHA256 string `json:"guard_elf_sha256"`
}

func NewGuardedPackager(builder *DockerBuilder, trustedGuard []byte, expectedSHA256 string) (*GuardedPackager, error) {
	if builder == nil || builder.client == nil || !validDigest(expectedSHA256) || len(trustedGuard) < 64 || len(trustedGuard) > 31<<20 || Digest(trustedGuard) != expectedSHA256 {
		return nil, ErrBuild
	}
	guard := bytes.Clone(trustedGuard)
	image, err := elf.NewFile(bytes.NewReader(guard))
	if err != nil {
		return nil, ErrBuild
	}
	defer image.Close()
	if image.Class != elf.ELFCLASS64 || image.Data != elf.ELFDATA2LSB || image.Machine != elf.EM_X86_64 || image.Type != elf.ET_EXEC {
		return nil, ErrBuild
	}
	for _, program := range image.Progs {
		if program.Type == elf.PT_INTERP || program.Type == elf.PT_DYNAMIC {
			return nil, ErrBuild
		}
	}
	return &GuardedPackager{builder: builder, guard: guard, guardID: expectedSHA256}, nil
}

func (packager *GuardedPackager) archive(built BuiltExecutable) ([]byte, error) {
	if packager == nil || packager.builder == nil || Digest(packager.guard) != packager.guardID {
		return nil, ErrBuild
	}
	// Reuse every original candidate binding/ELF validation, without executing it.
	validated, err := executableArchive(built)
	if err != nil {
		return nil, ErrBuild
	}
	candidate, err := builtELF(validated)
	if err != nil {
		return nil, ErrBuild
	}
	var buffer bytes.Buffer
	writer := tar.NewWriter(&buffer)
	for _, file := range []struct {
		name string
		raw  []byte
	}{{"candidate", candidate}, {"maintenance-guard", packager.guard}} {
		if writer.WriteHeader(&tar.Header{Name: file.name, Mode: 0555, Uid: 65532, Gid: 65532, Size: int64(len(file.raw)), Typeflag: tar.TypeReg}) != nil {
			return nil, ErrBuild
		}
		if _, err := writer.Write(file.raw); err != nil {
			return nil, ErrBuild
		}
	}
	if writer.Close() != nil || buffer.Len() > 64<<20 {
		return nil, ErrBuild
	}
	return buffer.Bytes(), nil
}

// Package never starts a container or grants launch authority. The later fixed
// executor must verify this guard, actual isolation and current one-use dispatch
// before publishing its marker. The legacy direct-entry image is not guarded.
func (packager *GuardedPackager) Package(ctx context.Context, built BuiltExecutable) (result GuardedImage, err error) {
	if packager == nil || packager.builder == nil || packager.builder.image != built.CompilerSHA256 || packager.builder.module != built.Module {
		return GuardedImage{}, ErrBuild
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	archive, err := packager.archive(built)
	if err != nil {
		return GuardedImage{}, ErrBuild
	}
	id, err := packager.builder.importFixedArchive(ctx, archive, "/maintenance-guard")
	if err != nil {
		return GuardedImage{}, ErrBuild
	}
	// Inspect actual packaged files through a never-started disposable container.
	// Import/config success alone does not establish their bytes and modes.
	raw, err := packager.builder.json(ctx, http.MethodPost, "/containers/create", map[string]any{
		"Image": "sha256:" + id, "User": "65532:65532", "Hostname": "maintenance-package",
		"HostConfig": map[string]any{"NetworkMode": "none", "ReadonlyRootfs": true, "CapDrop": []string{"ALL"}, "SecurityOpt": []string{"no-new-privileges:true"}, "PidsLimit": 32, "Memory": 128 << 20, "MemorySwap": 128 << 20, "NanoCpus": 1_000_000_000, "LogConfig": map[string]any{"Type": "none"}},
	}, 201)
	if err != nil {
		return GuardedImage{}, ErrBuild
	}
	container, err := dockerBuildID(raw, "Id")
	if err != nil {
		return GuardedImage{}, ErrBuild
	}
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		if _, cleanupErr := packager.builder.request(cleanup, http.MethodDelete, "/containers/"+container+"?v=1", "", nil, 1024, 204); cleanupErr != nil {
			result, err = GuardedImage{}, ErrBuild
		}
	}()
	for _, file := range []struct{ name, sha string }{{"candidate", built.ELFSHA256}, {"maintenance-guard", packager.guardID}} {
		raw, err = packager.builder.request(ctx, http.MethodGet, "/containers/"+container+"/archive?path=/"+file.name, "", nil, 32<<20, 200)
		if err != nil || exactGuardedArchiveMember(raw, file.name, file.sha) != nil {
			return GuardedImage{}, ErrBuild
		}
	}
	return GuardedImage{BuiltImage: BuiltImage{ImageSHA256: id, ELFSHA256: built.ELFSHA256, Commit: built.Commit, InventorySHA256: built.InventorySHA256, CompilerSHA256: built.CompilerSHA256, PackagingSHA256: Digest([]byte(fixedGuardedPackage + "guard-sha256=" + packager.guardID + "\n"))}, GuardELFSHA256: packager.guardID}, nil
}

// Keep tar parsing explicit for tests and later fixed-node image-content checks.
func exactGuardedArchiveMember(raw []byte, name, digest string) error {
	reader := tar.NewReader(bytes.NewReader(raw))
	header, err := reader.Next()
	if err != nil || header.Name != name || header.Typeflag != tar.TypeReg || header.Size < 64 || header.Size > 31<<20 || header.Linkname != "" || header.Mode != 0555 || header.Uid != 65532 || header.Gid != 65532 {
		return ErrBuild
	}
	body, err := io.ReadAll(io.LimitReader(reader, header.Size+1))
	if err != nil || int64(len(body)) != header.Size || Digest(body) != digest {
		return ErrBuild
	}
	if _, err = reader.Next(); err != io.EOF {
		return ErrBuild
	}
	return nil
}
