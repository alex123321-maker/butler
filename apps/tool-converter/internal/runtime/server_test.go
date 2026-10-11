package runtime

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"strings"
	"testing"

	converterv1 "github.com/butler/butler/internal/gen/converter/v1"
	"github.com/butler/butler/internal/generatedtools"
	"github.com/butler/butler/internal/security/content"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/proto"
)

func fixture(t *testing.T) (generatedtools.Need, generatedtools.Package, generatedtools.Profile, *converterv1.ExecutionProof) {
	t.Helper()
	n := generatedtools.Need{ID: "original-inventory", ConverterID: "inventory", Class: generatedtools.Class, Input: generatedtools.InputSchema{MediaType: "text/csv", Delimiter: ",", Header: true, Columns: []string{"name", "count", "enabled"}}, Output: generatedtools.OutputSchema{MediaType: "application/json", Fields: []generatedtools.OutputField{{Name: "name", Type: "string", SourceIndex: 0}, {Name: "count", Type: "integer", SourceIndex: 1}, {Name: "enabled", Type: "boolean", SourceIndex: 2}}}, Limits: generatedtools.Limits{InputBytes: 8192, OutputBytes: 32768, Rows: 128, FieldBytes: 1024, Millis: 1000}}
	pin := generatedtools.Profile{Dependency: generatedtools.Dependency{ID: "butler-converter-ir", Version: "1.0.0", SHA256: strings.Repeat("a", 64)}, Checker: generatedtools.Checker}
	p, e := generatedtools.Build(n, "1.0.0", pin)
	if e != nil {
		t.Fatal(e)
	}
	return n, p, pin, &converterv1.ExecutionProof{RunId: "current-run", ToolCallId: "exact-call", ActionId: "effect-exact", ActionFingerprint: strings.Repeat("b", 64)}
}
func data(id string, b []byte) *converterv1.SourceData {
	origin, _ := json.Marshal(content.Origin{Kind: content.Document, SourceRefs: []string{id}, Sensitivity: content.Private})
	return &converterv1.SourceData{SourceId: id, Revision: 1, AccessRevision: 1, ContentHash: strings.Repeat("c", 64), Body: b, OriginJson: string(origin), BodyHash: generatedtools.HashBytes(b)}
}
func client(t *testing.T, temp string, p generatedtools.Profile) converterv1.ConverterRuntimeServiceClient {
	t.Helper()
	l := bufconn.Listen(256 * 1024)
	s := grpc.NewServer(grpc.UnaryInterceptor(generatedtools.RuntimeAuthentication("synthetic-converter-runtime-token-000000000000")), grpc.MaxRecvMsgSize(192*1024))
	converterv1.RegisterConverterRuntimeServiceServer(s, NewServer(temp, p))
	go func() { _ = s.Serve(l) }()
	t.Cleanup(s.Stop)
	c, e := grpc.NewClient("passthrough:///converter", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return l.Dial() }))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = c.Close() })
	return converterv1.NewConverterRuntimeServiceClient(c)
}
func TestPrivateRuntimeDevelopTestAndOriginalInputConversion(t *testing.T) {
	n, p, profile, proof := fixture(t)
	needJSON, _ := json.Marshal(n)
	temp := t.TempDir()
	c := client(t, temp, profile)
	ctx := metadata.AppendToOutgoingContext(context.Background(), "authorization", "Bearer synthetic-converter-runtime-token-000000000000")
	develop := &converterv1.DevelopRequest{Proof: proof, NeedSource: data("need", needJSON), NeedHash: generatedtools.Hash(n), ProfilePin: generatedtools.Hash(profile), Version: "1.0.0"}
	if _, e := c.Develop(context.Background(), develop); status.Code(e) != codes.Unauthenticated {
		t.Fatalf("unauth: %v", e)
	}
	built, e := c.Develop(ctx, develop)
	if e != nil || built.GetPackageHash() != p.Digest {
		t.Fatalf("develop: %v", e)
	}
	tested, e := c.TestPackage(ctx, &converterv1.TestPackageRequest{Proof: proof, PackageSource: data("package", built.GetPackage()), NeedSource: data("need", needJSON), PackageHash: p.Digest, NeedHash: generatedtools.Hash(n), ProfilePin: generatedtools.Hash(profile)})
	var report generatedtools.Report
	if e != nil || json.Unmarshal([]byte(tested.GetReportJson()), &report) != nil || !report.Passed || report.Checker != generatedtools.Checker {
		t.Fatalf("independent report: %v %#v", e, report)
	}
	request := &converterv1.ConvertRequest{Proof: proof, PackageSource: data("package", built.GetPackage()), NeedSource: data("need", needJSON), InputSource: data("original", []byte("name,count,enabled\n\"Original, Document\",5,true\n")), PackageHash: p.Digest, NeedHash: generatedtools.Hash(n), ProfilePin: generatedtools.Hash(profile)}
	result, e := c.Convert(ctx, request)
	if e != nil || result.GetOutputHash() != generatedtools.HashBytes(result.GetOutput()) || string(result.GetOutput()) != `[{"count":5,"enabled":true,"name":"Original, Document"}]` {
		t.Fatalf("original convert: %s %v", result.GetOutput(), e)
	}
	entries, _ := os.ReadDir(temp)
	if len(entries) != 0 {
		t.Fatal("temporary developer workcopy retained")
	}
}
func TestRuntimeRejectsSecretStaleShapeAndUntestedProgramBeforeWork(t *testing.T) {
	n, p, profile, proof := fixture(t)
	needJSON, _ := json.Marshal(n)
	temp := t.TempDir()
	s := NewServer(temp, profile)
	original := &converterv1.ConvertRequest{Proof: proof, PackageSource: data("package", p.Raw), NeedSource: data("need", needJSON), InputSource: data("original", []byte("name,count,enabled\nprivate-marker,5,true\n")), PackageHash: p.Digest, NeedHash: generatedtools.Hash(n), ProfilePin: generatedtools.Hash(profile)}
	for _, tc := range []struct {
		name   string
		change func(*converterv1.ConvertRequest)
	}{
		{"secret", func(r *converterv1.ConvertRequest) {
			r.InputSource.OriginJson = `{"kind":"document","source_refs":["original"],"sensitivity":"secret"}`
		}},
		{"instruction", func(r *converterv1.ConvertRequest) {
			r.InputSource.OriginJson = `{"kind":"owner_policy","source_refs":["original"],"sensitivity":"private"}`
		}},
		{"missing-source", func(r *converterv1.ConvertRequest) { r.InputSource.SourceId = "" }},
		{"missing-proof", func(r *converterv1.ConvertRequest) { r.Proof = nil }},
		{"stale-package", func(r *converterv1.ConvertRequest) { r.PackageHash = strings.Repeat("d", 64) }},
		{"stale-need", func(r *converterv1.ConvertRequest) { r.NeedHash = strings.Repeat("d", 64) }},
		{"foreign-runtime", func(r *converterv1.ConvertRequest) { r.ProfilePin = strings.Repeat("d", 64) }},
		{"body-corruption", func(r *converterv1.ConvertRequest) { r.InputSource.Body[0] = 'x' }},
		{"credential", func(r *converterv1.ConvertRequest) {
			r.InputSource = data("original", []byte("name,count,enabled\nBearer synthetic-private-marker,5,true\n"))
		}},
		{"unknown-origin-key", func(r *converterv1.ConvertRequest) {
			r.InputSource.OriginJson = `{"kind":"document","source_refs":["original"],"sensitivity":"private","permission":"all"}`
		}},
		{"origin-duplicate", func(r *converterv1.ConvertRequest) {
			r.InputSource.OriginJson = `{"kind":"document","source_refs":["original"],"sensitivity":"secret","sensitivity":"private"}`
		}},
		{"untested-mapping", func(r *converterv1.ConvertRequest) {
			m := p.Manifest
			m.Code.Columns = append([]generatedtools.Column(nil), m.Code.Columns...)
			m.Code.Columns[0].InputIndex = 1
			raw, _ := json.Marshal(m)
			r.PackageSource = data("package", raw)
			r.PackageHash = generatedtools.HashBytes(raw)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := proto.Clone(original).(*converterv1.ConvertRequest)
			tc.change(r)
			got, e := s.Convert(context.Background(), r)
			if got != nil || e == nil || strings.Contains(e.Error(), "private-marker") {
				t.Fatalf("unsafe request: %v %v", got, e)
			}
			entries, _ := os.ReadDir(temp)
			if len(entries) != 0 {
				t.Fatal("rejected input reached developer workspace")
			}
		})
	}
}
func TestFailedIndependentAuthorCandidateReportsFalse(t *testing.T) {
	n, p, profile, proof := fixture(t)
	needJSON, _ := json.Marshal(n)
	s := NewServer(t.TempDir(), profile)
	m := p.Manifest
	m.Code.Header = false
	raw, _ := json.Marshal(m)
	result, e := s.TestPackage(context.Background(), &converterv1.TestPackageRequest{Proof: proof, PackageSource: data("package", raw), NeedSource: data("need", needJSON), PackageHash: generatedtools.HashBytes(raw), NeedHash: generatedtools.Hash(n), ProfilePin: generatedtools.Hash(profile)})
	var report generatedtools.Report
	if e != nil || json.Unmarshal([]byte(result.GetReportJson()), &report) != nil || report.Passed || report.PackageHash != generatedtools.HashBytes(raw) {
		t.Fatalf("failed candidate: %#v %v", report, e)
	}
}
