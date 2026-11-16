package registry

import (
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/toolcatalog"
	"testing"
)

func TestSearchValidatesBoundsAndFixedProviderDestinationBeforeDispatch(t *testing.T) {
	for _, tc := range []struct {
		domains []string
		args    string
		valid   bool
	}{
		{[]string{"lite.duckduckgo.com"}, `{"query":"current announcement","limit":3}`, true},
		{[]string{}, `{"query":"current announcement"}`, false},
		{[]string{"example.com"}, `{"query":"current announcement"}`, false},
		{[]string{"lite.duckduckgo.com"}, `{"query":"current announcement","limit":99}`, false},
		{[]string{"lite.duckduckgo.com"}, `{"query":"current announcement","limit":0}`, false},
		{[]string{"lite.duckduckgo.com"}, `{"query":"current announcement","url":"https://example.com/"}`, false},
	} {
		r, err := LoadWithOptions("../../../../configs/tools.json", "tool-webfetch:9090", LoadOptions{ClassAllowedDomains: map[string][]string{"web": tc.domains}})
		if err != nil {
			t.Fatal(err)
		}
		valid, _, failure := r.Validate(&toolbrokerv1.ToolCall{ToolName: "web.search", ArgsJson: tc.args})
		if valid != tc.valid {
			t.Fatal(tc, valid, failure)
		}
	}
}

func TestCatalogLimitRejectedBeforeActionPreparation(t *testing.T) {
	r := &Registry{toolsByName: map[string]*toolbrokerv1.ToolContract{toolcatalog.Search: toolcatalog.Contract(toolcatalog.Search)}}
	call := &toolbrokerv1.ToolCall{ToolName: toolcatalog.Search, ArgsJson: `{"limit":10}`}
	if ok, _, failure := r.Validate(call); ok || failure.GetMessage() == "" {
		t.Fatal("invalid call passed broker validation")
	}
	call.ArgsJson = `{"limit":8}`
	if ok, _, failure := r.Validate(call); !ok {
		t.Fatal(failure)
	}
}
