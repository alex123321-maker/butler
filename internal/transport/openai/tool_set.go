package openai

import "github.com/butler/butler/internal/transport"

func (p *Provider) rememberTools(runID string, defs []transport.ToolDefinition) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.toolSets == nil {
		p.toolSets = map[string][]transport.ToolDefinition{}
	}
	if p.aliases[runID] == nil {
		p.aliases[runID] = map[string]string{}
	}
	for _, def := range defs {
		p.aliases[runID][toolAlias(def.Name)] = def.Name
	}
	p.toolSets[runID] = append([]transport.ToolDefinition{}, defs...)
}
func (p *Provider) currentTools(runID string) ([]transport.ToolDefinition, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	defs, ok := p.toolSets[runID]
	return append([]transport.ToolDefinition{}, defs...), ok
}
