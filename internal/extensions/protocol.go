package extensions

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"github.com/butler/butler/internal/actionauth"
	"io"
	"strings"

	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/security/egress"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// Profiles are fixed deployment configuration. Packages cannot add an executor.
func ParseProfiles(raw string) (map[string]Profile, error) {
	if strings.TrimSpace(raw) == "" {
		return map[string]Profile{}, nil
	}
	if len(raw) > 64*1024 || strictJSON([]byte(raw)) != nil {
		return nil, ErrManifest
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	var profiles map[string]Profile
	if decoder.Decode(&profiles) != nil || decoder.Decode(new(any)) != io.EOF || len(profiles) > 16 {
		return nil, ErrManifest
	}
	for id, profile := range profiles {
		if !identifier.MatchString(id) || profile.Target == "" || len(profile.Target) > 256 || strings.ContainsAny(profile.Target, "/\\@\x00\r\n ") || !identifier.MatchString(profile.Dependency.ID) || profile.Dependency.Version == "" || !sourceURI(profile.Dependency.Source) || !digestName.MatchString(profile.Dependency.SHA256) {
			return nil, ErrManifest
		}
	}
	return profiles, nil
}

// RuntimeAuthentication uses a dedicated narrow Broker->runtime token. The
// external container never receives core action/owner/provider credentials.
func RuntimeAuthentication(token string) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, request any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		values := metadata.ValueFromIncomingContext(ctx, "authorization")
		if len(token) < 32 || len(values) != 1 || subtle.ConstantTimeCompare([]byte(values[0]), []byte("Bearer "+token)) != 1 {
			return nil, status.Error(codes.Unauthenticated, "private extension runtime authentication required")
		}
		return handler(ctx, request)
	}
}
func ProfilePins(profiles map[string]Profile) map[string]bool {
	pins := map[string]bool{}
	for _, profile := range profiles {
		pins[hashJSON(profile)] = true
	}
	return pins
}
func ValidateProtocol(e *toolbrokerv1.ExtensionContract, pins map[string]bool) error {
	if e == nil || !pins[e.GetProfilePin()] || !identifier.MatchString(e.GetExtensionId()) || !digestName.MatchString(e.GetManifestHash()) || !versionName.MatchString(e.GetVersion()) || !sourceURI(e.GetSource()) || e.GetGeneration() <= 0 || e.GetTimeoutMs() < 1 || e.GetTimeoutMs() > 30000 || e.GetEgressBytes() < 1 || e.GetEgressBytes() > egress.MaxRequestBytes || e.GetOutputBytes() < 1 || e.GetOutputBytes() > 256*1024 || len(e.GetResources()) == 0 || len(e.GetResources()) > 8 || len(e.GetCredentials()) > 1 {
		return ErrManifest
	}
	if e.GetMethod() == "GET" || e.GetMethod() == "HEAD" {
		if e.GetEffect() != "read" {
			return ErrManifest
		}
	} else if e.GetMethod() != "POST" && e.GetMethod() != "PUT" && e.GetMethod() != "PATCH" && e.GetMethod() != "DELETE" || e.GetEffect() != "commit" {
		return ErrManifest
	}
	for _, resource := range e.GetResources() {
		u, err := (egress.Policy{}).ValidateURL(resource.GetOrigin())
		if err != nil || resource.GetOrigin() != u.Scheme+"://"+u.Host || u.Path != "" || u.RawQuery != "" || !canonicalPath(resource.GetPathPrefix()) {
			return ErrManifest
		}
	}
	for _, input := range e.GetCredentials() {
		if input.GetInput() != "authorization" || !identifier.MatchString(input.GetField()) {
			return ErrManifest
		}
	}
	if receipt := e.GetReceipt(); receipt != nil {
		if e.GetEffect() != "commit" || receipt.GetRetrySafety() != "external_idempotency" || receipt.GetVerificationMethod() != "external_receipt_v1" || (receipt.GetIdempotencyHeader() != "Idempotency-Key" && receipt.GetIdempotencyHeader() != "X-Idempotency-Key") || strings.Count(receipt.GetVerificationUrl(), "{idempotency_key}") != 1 {
			return ErrManifest
		}
		for _, resource := range e.GetResources() {
			if resource.GetOrigin() != receipt.GetScopeOrigin() {
				return ErrManifest
			}
		}
		raw := strings.ReplaceAll(receipt.GetVerificationUrl(), "{idempotency_key}", "key")
		u, err := (egress.Policy{}).ValidateURL(raw)
		if err != nil || u.RawQuery != "" || !ProtocolResourceAllowed(e, raw) {
			return ErrManifest
		}
	}
	return nil
}
func ProtocolResourceAllowed(e *toolbrokerv1.ExtensionContract, raw string) bool {
	for _, resource := range e.GetResources() {
		if resourceWithin([]actionauth.ResourceScope{{Origin: resource.GetOrigin(), PathPrefix: resource.GetPathPrefix()}}, raw) {
			return true
		}
	}
	return false
}

// DecodeArguments rejects duplicate/unknown JSON members before interpretation.
func DecodeArguments(raw string, destination any) error {
	if len(raw) > 256*1024 || strictJSON([]byte(raw)) != nil {
		return ErrManifest
	}
	d := json.NewDecoder(bytes.NewBufferString(raw))
	d.DisallowUnknownFields()
	if d.Decode(destination) != nil || d.Decode(new(any)) != io.EOF {
		return ErrManifest
	}
	return nil
}
