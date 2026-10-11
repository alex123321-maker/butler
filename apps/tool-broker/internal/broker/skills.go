package broker

import (
	"context"
	"github.com/butler/butler/apps/tool-broker/internal/registry"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/extensions"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/generatedtools"
	"github.com/butler/butler/internal/skills"
	"github.com/jackc/pgx/v5"
	"strings"
)

type SkillEnvironment struct {
	Contexts actionauth.ContextReader
	Catalog  *TaskCatalog
}

func (s SkillEnvironment) GetActionContext(ctx context.Context, id string) (actionauth.ContextSnapshot, error) {
	return s.Contexts.GetActionContext(ctx, id)
}
func (s SkillEnvironment) ContractsForRun(ctx context.Context, id string) ([]*toolbrokerv1.ToolContract, error) {
	return s.Catalog.ListForRun(ctx, id, "", 64)
}

type SkillContracts struct {
	Registry   *registry.Registry
	Extensions *extensions.Store
	Converters *generatedtools.Store
}

func (s SkillContracts) GetSkillContract(ctx context.Context, name string) (*toolbrokerv1.ToolContract, error) {
	c, ok := s.Registry.GetContext(ctx, name)
	if !ok {
		return nil, skills.ErrUnavailable
	}
	return c, nil
}
func (s SkillContracts) GetSkillContractTx(ctx context.Context, tx pgx.Tx, name string) (*toolbrokerv1.ToolContract, error) {
	if strings.HasPrefix(name, "converter.") && s.Converters != nil {
		return s.Converters.GetContractTx(ctx, tx, name)
	}
	if s.Extensions != nil && len(name) > 10 && name[:10] == "extension." {
		return s.Extensions.GetContractTx(ctx, tx, name)
	}
	c, ok := s.Registry.Get(name)
	if !ok {
		return nil, skills.ErrUnavailable
	}
	return c, nil
}

type CatalogGuards struct {
	Extensions *extensions.Store
	Skills     *skills.Store
	Converters *generatedtools.Store
}

func (s CatalogGuards) CheckActionTx(ctx context.Context, tx pgx.Tx, b actionauth.ActionBinding) error {
	// Source guard is wired separately by the memory owner before this method.
	if s.Skills != nil {
		if err := s.Skills.CheckRunTx(ctx, tx, b.RunID, b.OwnerID); err != nil {
			return err
		}
	}
	if s.Converters != nil {
		if e := generatedtools.CheckPinnedLifecycleTx(ctx, tx, b.RunID); e != nil {
			return e
		}
		if e := s.Converters.CheckActionTx(ctx, tx, b); e != nil {
			return e
		}
	}
	if s.Extensions != nil {
		return s.Extensions.CheckActionTx(ctx, tx, b)
	}
	return nil
}

type CapabilityGuards struct {
	Extensions *extensions.Store
	Converters *generatedtools.Store
}

func (s CapabilityGuards) BindAction(ctx context.Context, call *toolbrokerv1.ToolCall, c *toolbrokerv1.ToolContract, b actionauth.ActionBinding, snapshot actionauth.ContextSnapshot) (actionauth.ActionBinding, error) {
	if call.GetToolName() == generatedtools.DeveloperAdmissionTool || strings.HasPrefix(call.GetToolName(), "converters.") || strings.HasPrefix(call.GetToolName(), "converter.") || c.GetConverter() != nil {
		if s.Converters == nil {
			return b, generatedtools.ErrAccess
		}
		return s.Converters.BindAction(ctx, call, c, b, snapshot)
	}
	if s.Extensions == nil {
		return b, extensions.ErrAccess
	}
	return s.Extensions.BindAction(ctx, call, c, b, snapshot)
}
