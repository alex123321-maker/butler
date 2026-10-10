package extensions

import "encoding/json"

// ModelSchema keeps the reviewed finite schema while dropping package prose
// from provider function declarations. Descriptions are separate DATA items.
func ModelSchema(raw string) string {
	var root map[string]any
	if json.Unmarshal([]byte(raw), &root) != nil {
		return "{}"
	}
	var strip func(map[string]any)
	strip = func(node map[string]any) {
		delete(node, "description")
		if props, ok := node["properties"].(map[string]any); ok {
			for _, child := range props {
				if schema, ok := child.(map[string]any); ok {
					strip(schema)
				}
			}
		}
		if child, ok := node["items"].(map[string]any); ok {
			strip(child)
		}
		if children, ok := node["oneOf"].([]any); ok {
			for _, child := range children {
				if schema, ok := child.(map[string]any); ok {
					strip(schema)
				}
			}
		}
	}
	strip(root)
	encoded, _ := json.Marshal(root)
	return string(encoded)
}
