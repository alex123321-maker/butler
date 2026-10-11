package runtime

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/butler/butler/internal/actionauth"
	converterv1 "github.com/butler/butler/internal/gen/converter/v1"
	"github.com/butler/butler/internal/generatedtools"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

// The dedicated agent helper starts the actual production binary in a read-only
// network-none container. This client is an extra read-only test-only mount;
// no core, provider, owner, PG or original document credentials are supplied.
func TestContainerConverterSmoke(t *testing.T) {
	if os.Getenv("BUTLER_CONVERTER_CONTAINER_E2E") != "1" {
		t.Skip("explicit isolated container harness required")
	}
	profile, e := generatedtools.DecodeProfile([]byte(os.Getenv("BUTLER_CONVERTER_PROFILE_JSON")))
	if e != nil {
		t.Fatal(e)
	}
	token, e := actionauth.ReadServiceToken(os.Getenv("BUTLER_CONVERTER_RUNTIME_TOKEN_FILE"))
	if e != nil {
		t.Fatal("synthetic runtime token unavailable")
	}
	c, e := grpc.NewClient("127.0.0.1:9090", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	client := converterv1.NewConverterRuntimeServiceClient(c)
	n, _, _, proof := fixture(t)
	p, e := generatedtools.Build(n, "1.0.0", profile)
	if e != nil {
		t.Fatal(e)
	}
	needJSON, _ := json.Marshal(n)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+token)
	d, e := client.Develop(ctx, &converterv1.DevelopRequest{Proof: proof, NeedSource: data("synthetic-need", needJSON), NeedHash: generatedtools.Hash(n), ProfilePin: generatedtools.Hash(profile), Version: "1.0.0"})
	if e != nil || d.GetPackageHash() != p.Digest {
		t.Fatal("actual developer failed", e)
	}
	r, e := client.TestPackage(ctx, &converterv1.TestPackageRequest{Proof: proof, PackageSource: data("synthetic-package", d.GetPackage()), NeedSource: data("synthetic-need", needJSON), PackageHash: p.Digest, NeedHash: generatedtools.Hash(n), ProfilePin: generatedtools.Hash(profile)})
	var report generatedtools.Report
	if e != nil || json.Unmarshal([]byte(r.GetReportJson()), &report) != nil || !report.Passed {
		t.Fatal("actual fixed checker failed", e)
	}
	converted, e := client.Convert(ctx, &converterv1.ConvertRequest{Proof: proof, PackageSource: data("synthetic-package", p.Raw), NeedSource: data("synthetic-need", needJSON), InputSource: data("synthetic-document", []byte("name,count,enabled\ncontainer-proof,2,true\n")), PackageHash: p.Digest, NeedHash: generatedtools.Hash(n), ProfilePin: generatedtools.Hash(profile)})
	if e != nil || string(converted.GetOutput()) != `[{"count":2,"enabled":true,"name":"container-proof"}]` {
		t.Fatal("actual conversion failed", e)
	}
	secret := data("synthetic-secret", []byte("Bearer synthetic-never-sent"))
	if _, e = client.Convert(ctx, &converterv1.ConvertRequest{Proof: proof, PackageSource: data("synthetic-package", p.Raw), NeedSource: data("synthetic-need", needJSON), InputSource: secret, PackageHash: p.Digest, NeedHash: generatedtools.Hash(n), ProfilePin: generatedtools.Hash(profile)}); e == nil {
		t.Fatal("recognized credential accepted")
	}
}
