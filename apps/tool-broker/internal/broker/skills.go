package broker

import (
	"context"
	"github.com/butler/butler/apps/tool-broker/internal/registry"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/extensions"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/skills"
	"github.com/jackc/pgx/v5"
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
}

func (s SkillContracts) GetSkillContract(ctx context.Context, name string) (*toolbrokerv1.ToolContract, error) {
	c, ok := s.Registry.GetContext(ctx, name)
	if !ok {
		return nil, skills.ErrUnavailable
	}
	return c, nil
}
func (s SkillContracts) GetSkillContractTx(ctx context.Context, tx pgx.Tx, name string) (*toolbrokerv1.ToolContract, error) {
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
}

func (s CatalogGuards) CheckActionTx(ctx context.Context, tx pgx.Tx, b actionauth.ActionBinding) error {
	// Source guard is wired separately by the memory owner before this method.
	if s.Skills != nil {
		if err := s.Skills.CheckRunTx(ctx, tx, b.RunID, b.OwnerID); err != nil {
			return err
		}
	}
	if s.Extensions != nil {
		return s.Extensions.CheckActionTx(ctx, tx, b)
	}
	return nil
}
