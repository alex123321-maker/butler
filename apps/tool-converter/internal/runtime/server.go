// Package runtime executes a fixed pure converter IR. It has no catalog,
// ownership, orchestration, model, credential or external network client.
package runtime

import (
	"context"
	"encoding/json"
	"time"

	converterv1 "github.com/butler/butler/internal/gen/converter/v1"
	"github.com/butler/butler/internal/generatedtools"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Server struct {
	converterv1.UnimplementedConverterRuntimeServiceServer
	developer generatedtools.Developer
}

func NewServer(tempDir string, profile generatedtools.Profile) *Server {
	return &Server{developer: generatedtools.Developer{TempDir: tempDir, Profile: profile}}
}
func rejected() error {
	return status.Error(codes.FailedPrecondition, "bounded converter request rejected")
}
func (s *Server) Develop(ctx context.Context, r *converterv1.DevelopRequest) (*converterv1.DevelopResponse, error) {
	var n generatedtools.Need
	if r == nil || s == nil || generatedtools.ValidateProof(r.GetProof()) != nil || generatedtools.ValidateData(r.GetNeedSource(), generatedtools.MaxPackageBytes) != nil || r.GetProfilePin() != generatedtools.Hash(s.developer.Profile) || generatedtools.Decode(r.GetNeedSource().GetBody(), &n) != nil || generatedtools.Hash(n) != r.GetNeedHash() {
		return nil, rejected()
	}
	if e := ctx.Err(); e != nil {
		return nil, rejected()
	}
	p, e := generatedtools.Build(n, r.GetVersion(), s.developer.Profile)
	if e != nil {
		return nil, rejected()
	}
	// A workcopy round-trip verifies only bounded package serialization. The
	// independent TestPackage RPC and catalog CAS remain separate mechanisms.
	copy, e := s.developer.Workcopy(ctx, p.Raw)
	if e != nil || generatedtools.HashBytes(copy) != p.Digest {
		return nil, rejected()
	}
	return &converterv1.DevelopResponse{Package: copy, PackageHash: p.Digest}, nil
}
func (s *Server) packageAndNeed(proof *converterv1.ExecutionProof, source, need *converterv1.SourceData, packageHash, needHash, pin string) (generatedtools.Package, generatedtools.Need, error) {
	var n generatedtools.Need
	if s == nil || generatedtools.ValidateProof(proof) != nil || generatedtools.ValidateData(source, generatedtools.MaxPackageBytes) != nil || generatedtools.ValidateData(need, generatedtools.MaxPackageBytes) != nil || pin != generatedtools.Hash(s.developer.Profile) || generatedtools.Decode(need.GetBody(), &n) != nil || n.Validate() != nil || needHash != generatedtools.Hash(n) {
		return generatedtools.Package{}, n, rejected()
	}
	p, e := generatedtools.Parse(source.GetBody())
	if e != nil || p.Digest != packageHash || p.Manifest.ID != n.ConverterID || p.Manifest.NeedID != n.ID || p.Manifest.Class != n.Class || generatedtools.Hash(p.Manifest.Input) != generatedtools.Hash(n.Input) || generatedtools.Hash(p.Manifest.Output) != generatedtools.Hash(n.Output) || p.Manifest.Limits != n.Limits || p.Manifest.Dependencies[0] != s.developer.Profile.Dependency {
		return p, n, rejected()
	}
	return p, n, nil
}
func (s *Server) TestPackage(ctx context.Context, r *converterv1.TestPackageRequest) (*converterv1.TestPackageResponse, error) {
	if r == nil {
		return nil, rejected()
	}
	p, n, e := s.packageAndNeed(r.GetProof(), r.GetPackageSource(), r.GetNeedSource(), r.GetPackageHash(), r.GetNeedHash(), r.GetProfilePin())
	if e != nil {
		return nil, e
	}
	report, e := s.developer.Test(ctx, p.Raw, n)
	// A failed independent test has a truthful hash-bound report, not an RPC
	// transport failure. Admission must require report.Passed in its own CAS.
	if e != nil && e != generatedtools.ErrChecks {
		return nil, rejected()
	}
	encoded, _ := json.Marshal(report)
	return &converterv1.TestPackageResponse{ReportJson: string(encoded)}, nil
}
func (s *Server) Convert(ctx context.Context, r *converterv1.ConvertRequest) (*converterv1.ConvertResponse, error) {
	if r == nil {
		return nil, rejected()
	}
	p, n, e := s.packageAndNeed(r.GetProof(), r.GetPackageSource(), r.GetNeedSource(), r.GetPackageHash(), r.GetNeedHash(), r.GetProfilePin())
	if e != nil || generatedtools.ValidateData(r.GetInputSource(), int(n.Limits.InputBytes)) != nil {
		return nil, rejected()
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(n.Limits.Millis)*time.Millisecond)
	defer cancel()
	if report, e := generatedtools.Check(ctx, p, n, s.developer.Profile); e != nil || !report.Passed {
		return nil, rejected()
	}
	output, e := generatedtools.Convert(ctx, p.Manifest.Code, n.Limits, r.GetInputSource().GetBody())
	if e != nil {
		return nil, rejected()
	}
	return &converterv1.ConvertResponse{Output: output, OutputHash: generatedtools.HashBytes(output), MediaType: "application/json"}, nil
}
