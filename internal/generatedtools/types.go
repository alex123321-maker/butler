package generatedtools

import (
	"context"
	"time"

	"github.com/butler/butler/internal/actionauth"
	converterv1 "github.com/butler/butler/internal/gen/converter/v1"
	"github.com/butler/butler/internal/memory/catalog"
	"google.golang.org/grpc"
)

// Discovery is a bounded service-owned snapshot of the existing task-scoped
// tools and reviewed skill identities. No claimed model pass/absence flag is
// accepted. Compatible existing converters must be reused instead of rebuilt.
type Capability struct {
	Name             string `json:"name"`
	ContractHash     string `json:"contract_hash"`
	Revision         string `json:"revision"`
	Class            string `json:"class,omitempty"`
	InputSchemaHash  string `json:"input_schema_hash,omitempty"`
	OutputSchemaHash string `json:"output_schema_hash,omitempty"`
}
type SkillIdentity struct {
	ID     string `json:"skill_id"`
	Digest string `json:"digest"`
}
type Discovery struct {
	Tools      []Capability    `json:"tools"`
	Skills     []SkillIdentity `json:"skills"`
	Compatible []string        `json:"compatible"`
}
type RunEnvironment interface {
	GetActionContext(context.Context, string) (actionauth.ContextSnapshot, error)
	Discover(context.Context, string, Need) (Discovery, error)
}
type RuntimeClient interface {
	Develop(context.Context, *converterv1.DevelopRequest, ...grpc.CallOption) (*converterv1.DevelopResponse, error)
	TestPackage(context.Context, *converterv1.TestPackageRequest, ...grpc.CallOption) (*converterv1.TestPackageResponse, error)
	Convert(context.Context, *converterv1.ConvertRequest, ...grpc.CallOption) (*converterv1.ConvertResponse, error)
}
type Record struct {
	OwnerID    string                         `json:"owner_id"`
	ID         string                         `json:"converter_id"`
	Digest     string                         `json:"digest"`
	Version    string                         `json:"version"`
	NeedID     string                         `json:"need_id"`
	NeedHash   string                         `json:"need_hash"`
	BodySource catalog.SourceRef              `json:"body_source"`
	NeedSource catalog.SourceRef              `json:"need_source"`
	ProfilePin string                         `json:"profile_pin"`
	Shape      actionauth.ConverterPermission `json:"permission"`
	Generation int64                          `json:"generation"`
	Status     string                         `json:"status"`
	Active     bool                           `json:"active"`
	CreatedAt  time.Time                      `json:"created_at"`
}
type Output struct {
	ConverterID          string            `json:"converter_id"`
	PackageHash          string            `json:"package_hash"`
	NeedHash             string            `json:"need_hash"`
	Source               catalog.SourceRef `json:"source_ref"`
	SHA256               string            `json:"sha256"`
	MediaType            string            `json:"media_type"`
	InstructionAuthority bool              `json:"instruction_authority"`
}

func ExpectedPermission(n Need, p Profile, trials int64) actionauth.ConverterPermission {
	return actionauth.ConverterPermission{ConverterID: n.ConverterID, Class: n.Class, InputSchemaHash: Hash(n.Input), OutputSchemaHash: Hash(n.Output), RuntimePin: Hash(p), InputBytes: n.Limits.InputBytes, OutputBytes: n.Limits.OutputBytes, Rows: n.Limits.Rows, FieldBytes: n.Limits.FieldBytes, Millis: n.Limits.Millis, Trials: trials}
}
