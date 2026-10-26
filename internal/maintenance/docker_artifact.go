package maintenance

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"time"
)

// BuiltImage binds the independently measured executable to the actual Docker
// image ID produced by a fixed scratch package. No author Dockerfile, image
// reference, shell fragment, mount or configuration participates in packaging.
// It is retained evidence, not approval or deployment authority.
type BuiltImage struct {
	ImageSHA256     string `json:"image_sha256"`
	ELFSHA256       string `json:"elf_sha256"`
	Commit          string `json:"commit"`
	InventorySHA256 string `json:"inventory_sha256"`
	CompilerSHA256  string `json:"compiler_sha256"`
	PackagingSHA256 string `json:"packaging_sha256"`
}

const fixedELFPackage = "butler.maintenance.scratch-elf.v1\n/candidate;uid=65532;gid=65532;mode=0555;entrypoint=/candidate;workdir=/\n"

func executableArchive(built BuiltExecutable) ([]byte, error) {
	if built.Bytes < 64 || built.Bytes > 31<<20 || len(built.elf) != built.Bytes || Digest(built.elf) != built.ELFSHA256 || !gitObjectID(built.Commit) || !validDigest(built.InventorySHA256) || !validDigest(built.CompilerSHA256) {
		return nil, ErrBuild
	}
	var buffer bytes.Buffer
	writer := tar.NewWriter(&buffer)
	if writer.WriteHeader(&tar.Header{Name: "candidate", Mode: 0555, Uid: 65532, Gid: 65532, Size: int64(built.Bytes), Typeflag: tar.TypeReg}) != nil {
		return nil, ErrBuild
	}
	if _, err := writer.Write(built.elf); err != nil || writer.Close() != nil {
		return nil, ErrBuild
	}
	// The same bounded ELF validator also rejects extra members and links.
	if _, err := builtELF(buffer.Bytes()); err != nil {
		return nil, ErrBuild
	}
	return buffer.Bytes(), nil
}

// Package imports a fixed root filesystem and then measures its actual image
// identity. The image stays in this private daemon for independent checks and
// an explicitly admitted node action. Packaging never starts a container.
func (builder *DockerBuilder) Package(ctx context.Context, built BuiltExecutable) (BuiltImage, error) {
	if builder == nil || builder.client == nil || builder.image != built.CompilerSHA256 || builder.module != built.Module {
		return BuiltImage{}, ErrBuild
	}
	archive, err := executableArchive(built)
	if err != nil {
		return BuiltImage{}, ErrBuild
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	query := url.Values{"fromSrc": {"-"}, "changes": {"ENTRYPOINT [\"/candidate\"]", "USER 65532:65532", "WORKDIR /"}}
	raw, err := builder.request(ctx, http.MethodPost, "/images/create?"+query.Encode(), "application/x-tar", archive, 64<<10, 200)
	if err != nil {
		return BuiltImage{}, ErrBuild
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	imageID := ""
	for {
		var event struct {
			Status      string          `json:"status"`
			Error       string          `json:"error"`
			ErrorDetail json.RawMessage `json:"errorDetail"`
		}
		if err := decoder.Decode(&event); err == io.EOF {
			break
		} else if err != nil || event.Error != "" || (len(event.ErrorDetail) != 0 && string(event.ErrorDetail) != "null") {
			return BuiltImage{}, ErrBuild
		}
		if len(event.Status) == 71 && event.Status[:7] == "sha256:" && validDigest(event.Status[7:]) {
			imageID = event.Status[7:]
		}
	}
	if imageID == "" {
		return BuiltImage{}, ErrBuild
	}
	raw, err = builder.request(ctx, http.MethodGet, "/images/sha256:"+imageID+"/json", "", nil, 256<<10, 200)
	if err != nil {
		return BuiltImage{}, ErrBuild
	}
	var image struct {
		ID     string `json:"Id"`
		Config struct {
			Entrypoint []string
			User       string
			WorkingDir string
			Env        []string
			Cmd        []string
			Volumes    map[string]json.RawMessage
		}
	}
	if json.Unmarshal(raw, &image) != nil || image.ID != "sha256:"+imageID || len(image.Config.Entrypoint) != 1 || image.Config.Entrypoint[0] != "/candidate" || image.Config.User != "65532:65532" || image.Config.WorkingDir != "/" || len(image.Config.Env) != 0 || len(image.Config.Cmd) != 0 || len(image.Config.Volumes) != 0 {
		return BuiltImage{}, ErrBuild
	}
	return BuiltImage{ImageSHA256: imageID, ELFSHA256: built.ELFSHA256, Commit: built.Commit, InventorySHA256: built.InventorySHA256, CompilerSHA256: built.CompilerSHA256, PackagingSHA256: Digest([]byte(fixedELFPackage))}, nil
}
