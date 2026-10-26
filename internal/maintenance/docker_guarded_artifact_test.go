package maintenance

import (
	"archive/tar"
	"bytes"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
)

func TestGuardedArchiveRequiresExactRegularMember(t *testing.T) {
	body := bytes.Repeat([]byte{1}, 128)
	archive := func(name string, mode int64, link string, extra bool) []byte {
		t.Helper()
		var buffer bytes.Buffer
		writer := tar.NewWriter(&buffer)
		header := &tar.Header{Name: name, Mode: mode, Uid: 65532, Gid: 65532, Size: int64(len(body)), Typeflag: tar.TypeReg, Linkname: link}
		if writer.WriteHeader(header) != nil {
			t.Fatal("archive header")
		}
		if _, err := writer.Write(body); err != nil {
			t.Fatal(err)
		}
		if extra {
			if writer.WriteHeader(&tar.Header{Name: "extra", Mode: 0555, Typeflag: tar.TypeReg}) != nil {
				t.Fatal("extra archive header")
			}
		}
		if writer.Close() != nil {
			t.Fatal("archive close")
		}
		return buffer.Bytes()
	}
	good := archive("maintenance-guard", 0555, "", false)
	if exactGuardedArchiveMember(good, "maintenance-guard", Digest(body)) != nil {
		t.Fatal("exact member rejected")
	}
	for _, raw := range [][]byte{archive("candidate", 0555, "", false), archive("maintenance-guard", 0755, "", false), archive("maintenance-guard", 0555, "", true), good[:512]} {
		if exactGuardedArchiveMember(raw, "maintenance-guard", Digest(body)) == nil {
			t.Fatal("wrong member/mode/tail/truncation accepted")
		}
	}
	if exactGuardedArchiveMember(good, "maintenance-guard", strings.Repeat("0", 64)) == nil {
		t.Fatal("wrong actual bytes accepted")
	}
	if _, err := NewGuardedPackager(&DockerBuilder{client: &http.Client{}}, body, Digest(body)); err == nil {
		t.Fatal("non-ELF guard accepted")
	}
}

type guardedPackageTrace struct {
	base http.RoundTripper
	t    *testing.T
	seen []string
}

func (trace *guardedPackageTrace) RoundTrip(request *http.Request) (*http.Response, error) {
	if strings.HasSuffix(request.URL.Path, "/start") || strings.Contains(request.URL.Path, "/exec") || strings.HasSuffix(request.URL.Path, "/kill") || strings.HasSuffix(request.URL.Path, "/stop") {
		trace.t.Error("packaging attempted process execution")
		return nil, ErrBuild
	}
	trace.seen = append(trace.seen, request.Method+" "+request.URL.Path)
	return trace.base.RoundTrip(request)
}

func TestDockerGuardedPackagerOwnedDaemonIntegration(t *testing.T) {
	if os.Getenv("BUTLER_TEST_MAINTENANCE_BUILD") != "private-local-build-only" || os.Getenv("BUTLER_TEST_MAINTENANCE_GUARDED_PACKAGE") != "private-local-no-execution" {
		t.Skip("explicit private daemon and operator guard required")
	}
	guard, err := os.ReadFile(os.Getenv("BUTLER_TEST_MAINTENANCE_GUARD_ELF"))
	guardID := os.Getenv("BUTLER_TEST_MAINTENANCE_GUARD_SHA256")
	if err != nil || !validDigest(guardID) || Digest(guard) != guardID {
		t.Fatal("exact private trusted guard unavailable")
	}
	builder, err := NewDockerBuilder("/run/butler21-docker.sock", os.Getenv("BUTLER_TEST_MAINTENANCE_COMPILER_SHA256"), "modules/public")
	if err != nil {
		t.Fatal(err)
	}
	built, err := builder.Build(t.Context(), publicBuildSnapshot(t))
	if err != nil {
		t.Fatal("actual independent candidate build failed", err)
	}
	packager, err := NewGuardedPackager(builder, guard, guardID)
	if err != nil {
		t.Fatal("fixed guard configuration failed", err)
	}
	guard[0] = 0 // The private configuration must not retain caller-owned bytes.
	archive, err := packager.archive(built)
	if err != nil {
		t.Fatal(err)
	}
	reader := tar.NewReader(bytes.NewReader(archive))
	for _, expected := range []struct{ name, sha string }{{"candidate", built.ELFSHA256}, {"maintenance-guard", guardID}} {
		header, err := reader.Next()
		if err != nil || header.Name != expected.name {
			t.Fatal("fixed archive order lost")
		}
		raw, err := io.ReadAll(reader)
		if err != nil || Digest(raw) != expected.sha {
			t.Fatal("fixed archive bytes lost")
		}
	}
	if _, err = reader.Next(); err != io.EOF {
		t.Fatal("unexpected archive member")
	}
	trace := &guardedPackageTrace{base: builder.client.Transport, t: t}
	builder.client.Transport = trace
	image, err := packager.Package(t.Context(), built)
	if err != nil || !validDigest(image.ImageSHA256) || image.GuardELFSHA256 != guardID || image.ELFSHA256 != built.ELFSHA256 || image.PackagingSHA256 != Digest([]byte(fixedGuardedPackage+"guard-sha256="+guardID+"\n")) {
		t.Fatal("actual guarded package/content verification failed", err)
	}
	if len(trace.seen) != 6 {
		t.Fatalf("expected import/inspect/create/two exact file reads/delete, got%d", len(trace.seen))
	}
	t.Logf("actual guarded image=%s guard=%s candidate=%s; exact cold-image bytes/modes verified; no candidate process started", image.ImageSHA256, guardID, built.ELFSHA256)
}
