package maintenance

import (
	"archive/tar"
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Diagnostic metadata only: never log request bodies, headers, configurations
// or daemon error bodies from an independent build.
type publicBuildTrace struct {
	transport http.RoundTripper
	t         *testing.T
}

func (trace publicBuildTrace) RoundTrip(request *http.Request) (*http.Response, error) {
	response, err := trace.transport.RoundTrip(request)
	if err != nil {
		trace.t.Logf("private Docker %s %s transport unavailable", request.Method, request.URL.Path)
	} else {
		trace.t.Logf("private Docker %s %s status=%d", request.Method, request.URL.Path, response.StatusCode)
	}
	return response, err
}

func publicBuildSnapshot(t *testing.T) *SourceSnapshot {
	t.Helper()
	source, git := publicGitSource(t)
	directory := filepath.Join(source.RepositoryRoot, "modules/public")
	if err := os.MkdirAll(directory, 0700); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"go.mod":       "module example.invalid/public\n\ngo 1.27.2\n",
		"main.go":      "package main\nimport \"fmt\"\n//go:generate sh -c 'touch /out/author-generate-ran'\nfunc main(){fmt.Println(\"synthetic functional output\")}\n",
		"main_test.go": "package main\nfunc init(){panic(\"author tests must not execute\")}\n",
		"Dockerfile":   "FROM forbidden.invalid/image\nRUN touch /out/author-dockerfile-ran\n",
	} {
		if err := os.WriteFile(filepath.Join(directory, name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	git("", "add", ".")
	git("", "commit", "-m", "synthetic independent build fixture")
	snapshot, err := source.Snapshot(t.Context(), git("", "rev-parse", "HEAD"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := snapshot.Close(); err != nil {
			t.Error(err)
		}
	})
	return snapshot
}

func TestDockerBuildSourceArchiveBindsWholeInventory(t *testing.T) {
	snapshot := publicBuildSnapshot(t)
	raw, err := sourceArchive(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	reader := tar.NewReader(bytes.NewReader(raw))
	seen := map[string]bool{}
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil || header.Typeflag != tar.TypeReg || header.Uid != 65532 || header.Gid != 65532 || header.Mode&0222 != 0 {
			t.Fatal("source staging identity/mode lost", err)
		}
		if seen[header.Name] {
			t.Fatal("ambiguous archive member")
		}
		seen[header.Name] = true
		actual, err := io.ReadAll(reader)
		if err != nil {
			t.Fatal(err)
		}
		matched := false
		for _, file := range snapshot.Files {
			if file.Path == header.Name && Digest(actual) == file.SHA256 && int64(len(actual)) == file.Bytes {
				matched = true
			}
		}
		if !matched {
			t.Fatal("unmeasured bytes reached compiler staging")
		}
	}
	if len(seen) != len(snapshot.Files) || !seen["modules/public/main_test.go"] || !seen["modules/public/Dockerfile"] {
		t.Fatal("author tests/config were omitted from complete inventory")
	}
	name := filepath.Join(snapshot.Directory(), "modules/public/main.go")
	if err := os.Chmod(name, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte("changed after snapshot"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := sourceArchive(snapshot); err == nil {
		t.Fatal("snapshot mutation accepted")
	}
}

func TestDockerBuildConfigurationDoesNotExecuteAuthorInstructions(t *testing.T) {
	config := buildContainer(strings.Repeat("1", 64), "public-nonce", "owned-volume", "modules/public", false)
	command := config["Cmd"].([]string)
	if len(command) != 3 || command[0] != "/bin/sh" || command[1] != "-ec" || !strings.Contains(command[2], "/out/build-authorized") || strings.Contains(command[2], "go test") || strings.Contains(command[2], "go generate") || strings.Contains(command[2], "Dockerfile") || config["User"] != "65532:65532" || config["WorkingDir"] != "/src/modules/public" {
		t.Fatal("source controls trusted build supervisor")
	}
	host := config["HostConfig"].(map[string]any)
	if host["NetworkMode"] != "none" || host["ReadonlyRootfs"] != true || host["Memory"] != 1<<30 || host["PidsLimit"] != 128 {
		t.Fatal("declared sandbox bounds changed")
	}
	binds := host["Binds"].([]string)
	if len(binds) != 1 || binds[0] != "owned-volume:/src:ro" {
		t.Fatal("compiler received writable source or host mount")
	}
	for _, invalid := range []string{"../author", "/host", "modules/../author", "modules\\author"} {
		if _, err := NewDockerBuilder("/unavailable", strings.Repeat("1", 64), invalid); err == nil {
			t.Fatal("invalid fixed module accepted")
		}
	}
	if _, err := (&DockerBuilder{}).Build(context.Background(), nil); err == nil {
		t.Fatal("unconfigured compiler admitted")
	}
}

func TestDockerBuildRejectsNonExecutableArchive(t *testing.T) {
	for _, name := range []string{"candidate", "../candidate", "out/candidate"} {
		var buffer bytes.Buffer
		writer := tar.NewWriter(&buffer)
		body := []byte(strings.Repeat("not-an-ELF", 20))
		if err := writer.WriteHeader(&tar.Header{Name: name, Size: int64(len(body)), Mode: 0555, Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		_, _ = writer.Write(body)
		_ = writer.Close()
		if _, err := builtELF(buffer.Bytes()); err == nil {
			t.Fatal("non-executable or unsafe archive accepted")
		}
	}
}

func TestDockerBuilderOwnedDaemonIntegration(t *testing.T) {
	if os.Getenv("BUTLER_TEST_MAINTENANCE_BUILD") != "private-local-build-only" {
		t.Skip("explicit private owned compiler/daemon required")
	}
	compiler := os.Getenv("BUTLER_TEST_MAINTENANCE_COMPILER_SHA256")
	if !validDigest(compiler) {
		t.Fatal("actual immutable compiler image ID required")
	}
	builder, err := NewDockerBuilder("/run/butler21-docker.sock", compiler, "modules/public")
	if err != nil {
		t.Fatal(err)
	}
	builder.client.Transport = publicBuildTrace{transport: builder.client.Transport, t: t}
	snapshot := publicBuildSnapshot(t)
	result, err := builder.Build(t.Context(), snapshot)
	if err != nil {
		t.Fatal("actual isolated build failed", err)
	}
	if result.Commit != snapshot.Commit || result.InventorySHA256 != snapshot.InventorySHA256 || result.CompilerSHA256 != compiler || result.Module != "modules/public" || result.ELFSHA256 != Digest(result.ELF()) || result.Bytes != len(result.ELF()) || result.Bytes < 4096 {
		t.Fatal("actual executable/source/compiler binding lost")
	}
	if result.Bounds.CompilerSHA256 != compiler || result.Bounds.UID != 65532 || result.Bounds.MemoryBytes != 1<<30 || result.Bounds.PIDs != 128 || result.Bounds.CPU != "100000 100000" || result.Bounds.EffectiveSwapMax != 0 || !result.Bounds.NoNewPrivileges || result.Bounds.EffectiveCaps != "0000000000000000" || !validDigest(result.Bounds.ContainerID) || result.Bounds.MeasuredAt.IsZero() {
		t.Fatal("actual kernel bounds unavailable before compiler execution")
	}
	copy := result.ELF()
	copy[0] = 0
	if Digest(result.ELF()) != result.ELFSHA256 {
		t.Fatal("external bytes can rewrite measured result")
	}
	image, err := builder.Package(t.Context(), result)
	if err != nil || !validDigest(image.ImageSHA256) || image.ELFSHA256 != result.ELFSHA256 || image.InventorySHA256 != result.InventorySHA256 || image.CompilerSHA256 != compiler || image.PackagingSHA256 != Digest([]byte(fixedELFPackage)) {
		t.Fatal("fixed independent scratch packaging failed", err)
	}
	t.Logf("actual bounded compiler produced ELF sha256=%s bytes=%d; author tests/generate/Dockerfile not executed; ELF not run or deployed", result.ELFSHA256, result.Bytes)
	t.Logf("actual imported scratch image sha256=%s; compilation/packaging grants no review or deployment permission", image.ImageSHA256)
}
