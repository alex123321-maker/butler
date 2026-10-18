package broker

import (
	"context"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/generatedtools"
	"github.com/butler/butler/internal/skills"
)

type ConverterEnvironment struct {
	Contexts actionauth.ContextReader
	Catalog  *TaskCatalog
	Skills   *skills.Store
}

func (e *ConverterEnvironment) GetActionContext(ctx context.Context, id string) (actionauth.ContextSnapshot, error) {
	return e.Contexts.GetActionContext(ctx, id)
}
func (e *ConverterEnvironment) Discover(ctx context.Context, id string, _ generatedtools.Need) (generatedtools.Discovery, error) {
	d := generatedtools.Discovery{Tools: []generatedtools.Capability{}, Skills: []generatedtools.SkillIdentity{}}
	if e.Catalog == nil || e.Skills == nil {
		return d, generatedtools.ErrAccess
	}
	contracts, err := e.Catalog.ListForRun(ctx, id, "", 0)
	if err != nil {
		return d, err
	}
	for _, c := range contracts {
		h, err := actionauth.ContractHash(c)
		if err != nil {
			return d, err
		}
		v := generatedtools.Capability{Name: c.ToolName, ContractHash: h, Revision: c.ContractRevision}
		if p := c.GetConverter(); p != nil {
			v.Class = p.Class
			v.InputSchemaHash = p.InputSchemaHash
			v.OutputSchemaHash = p.OutputSchemaHash
		}
		d.Tools = append(d.Tools, v)
	}
	index, err := e.Skills.Index(ctx, id, "", 12)
	if err != nil {
		return d, err
	}
	for _, v := range index {
		d.Skills = append(d.Skills, generatedtools.SkillIdentity{ID: v.ID, Digest: v.Digest})
	}
	return d, nil
}
