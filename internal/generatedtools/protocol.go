package generatedtools

import (
	"context"
	"crypto/subtle"
	"slices"
	"strings"

	converterv1 "github.com/butler/butler/internal/gen/converter/v1"
	"github.com/butler/butler/internal/memory/sanitize"
	"github.com/butler/butler/internal/security/content"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func RuntimeAuthentication(token string) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, request any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		v := metadata.ValueFromIncomingContext(ctx, "authorization")
		if len(token) < 32 || len(v) != 1 || subtle.ConstantTimeCompare([]byte(v[0]), []byte("Bearer "+token)) != 1 {
			return nil, status.Error(codes.Unauthenticated, "private converter runtime authentication required")
		}
		return handler(ctx, request)
	}
}
func ValidateProof(p *converterv1.ExecutionProof) error {
	if p == nil || !digestName.MatchString(p.GetActionFingerprint()) {
		return ErrAccess
	}
	for _, v := range []string{p.GetRunId(), p.GetToolCallId(), p.GetActionId()} {
		if v == "" || len(v) > 160 || strings.TrimSpace(v) != v || strings.ContainsAny(v, "\x00\r\n") {
			return ErrAccess
		}
	}
	return nil
}

// This verifies the private envelope, not authorization. The Broker must hold
// the current grant/source proof before release; the runtime has no PG access.
// Classified Secret and recognized credential material are independently denied
// before either synthetic testing or conversion touches a temporary workspace.
func ValidateData(s *converterv1.SourceData, cap int) error {
	if s == nil || s.GetSourceId() == "" || len(s.GetSourceId()) > 160 || s.GetRevision() < 1 || s.GetAccessRevision() < 1 || !digestName.MatchString(s.GetContentHash()) || len(s.GetBody()) > cap || len(s.GetOriginJson()) > 16*1024 || s.GetBodyHash() != HashBytes(s.GetBody()) {
		return ErrAccess
	}
	var origin content.Origin
	if Decode([]byte(s.GetOriginJson()), &origin) != nil || origin.Sensitivity != content.Private && origin.Sensitivity != content.Public || origin.IsInstruction() || origin.Kind == content.OwnerInput || origin.Kind == content.LegacyUnknown || len(origin.SourceRefs) > 64 || len(origin.ParentRefs) > 64 || !slices.Contains(origin.SourceRefs, s.GetSourceId()) {
		return ErrAccess
	}
	if origin.Kind != content.Document && origin.Kind != content.MemoryDerived && origin.Kind != content.ToolOutput && origin.Kind != content.SubagentOutput {
		return ErrAccess
	}
	if sanitize.Text(string(s.GetBody())) != string(s.GetBody()) {
		return ErrAccess
	}
	return nil
}
func DecodeProfile(raw []byte) (Profile, error) {
	var p Profile
	if Decode(raw, &p) != nil || p.Validate() != nil {
		return Profile{}, ErrPackage
	}
	return p, nil
}
