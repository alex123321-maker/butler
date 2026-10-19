package antigravity

import (
	"fmt"
	"github.com/butler/butler/internal/transport"
)

// Aliases belong to the run, not to a position in the current selected set.
// Removed aliases remain reserved for historical signed functionCall parts.
func replaceTools(state *runState, update *transport.ToolSetSnapshot) error {
	if update == nil {
		return nil
	}
	if err := update.Validate(); err != nil {
		return err
	}
	aliases := map[string]string{}
	for name, alias := range state.aliases {
		aliases[name] = alias
	}
	if len(aliases) == 0 {
		for alias, def := range state.definitions {
			aliases[def.Name] = alias
		}
	}
	declarations := []functionDeclaration{}
	definitions := map[string]transport.ToolDefinition{}
	for _, def := range update.Definitions {
		schema, err := nativeToolSchema(def.SchemaJSON)
		if err != nil {
			return err
		}
		alias := aliases[def.Name]
		if alias == "" {
			alias = fmt.Sprintf("butler_tool_%d", len(aliases))
			aliases[def.Name] = alias
		}
		declarations = append(declarations, functionDeclaration{Name: alias, Description: def.Name + ": " + def.Description, Parameters: schema})
		definitions[alias] = def
	}
	state.aliases = aliases
	state.tools = declarations
	state.definitions = definitions
	return nil
}
