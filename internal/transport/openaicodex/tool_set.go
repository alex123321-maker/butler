package openaicodex

import (
	"fmt"
	"github.com/butler/butler/internal/transport"
)

func (p *Provider) rememberTools(runID string, defs []transport.ToolDefinition) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.toolSets == nil {
		p.toolSets = map[string][]transport.ToolDefinition{}
	}
	p.toolSets[runID] = append([]transport.ToolDefinition{}, defs...)
	if p.toolAliases == nil {
		p.toolAliases = map[string]map[string]string{}
	}
	aliases := p.toolAliases[runID]
	if aliases == nil {
		aliases = map[string]string{}
	}
	for _, def := range defs {
		found := false
		for _, name := range aliases {
			if name == def.Name {
				found = true
				break
			}
		}
		if found {
			continue
		}
		base := sanitizeToolName(def.Name)
		alias := base
		for n := 2; aliases[alias] != ""; n++ {
			alias = fmt.Sprintf("%s_%d", base, n)
		}
		aliases[alias] = def.Name
	}
	p.toolAliases[runID] = aliases
}
func (p *Provider) attachCurrentTools(payload map[string]any, runID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	defs, ok := p.toolSets[runID]
	if !ok {
		return
	}
	encoded := encodeTools(defs)
	for i, def := range defs {
		for alias, name := range p.toolAliases[runID] {
			if name == def.Name {
				encoded[i]["name"] = alias
				break
			}
		}
	}
	payload["tools"] = encoded
}
