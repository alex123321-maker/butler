package registry

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
)

func TestNetworkCapKeepsBothRestrictionsAndReleaseDeclaration(t *testing.T) {
	for _, tc := range []struct {
		name          string
		contract, cap []string
		url           string
		allowed       bool
	}{
		{"disjoint", []string{"a.example"}, []string{"b.example"}, "https://outside.invalid/", false},
		{"contract-only-domain", []string{"a.example"}, []string{"b.example"}, "https://a.example/", false},
		{"cap-only-domain", []string{"a.example"}, []string{"b.example"}, "https://b.example/", false},
		{"both", []string{"a.example"}, []string{"a.example"}, "https://a.example/", true},
		{"wildcard-overlap", []string{"*.example.com"}, []string{"api.example.com"}, "https://api.example.com/", true},
		{"wildcard-outside-cap", []string{"*.example.com"}, []string{"api.example.com"}, "https://other.example.com/", false},
		{"empty-cap-denies", nil, []string{}, "https://example.com/", false},
		{"empty-release-is-not-replaced", nil, []string{"example.com"}, "https://example.com/", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "tools.json")
			data, _ := json.Marshal(fileConfig{Tools: []toolConfig{{ToolName: "http.request", ToolClass: "http", InputSchemaJSON: `{"type":"object"}`, AllowedDomains: tc.contract}}})
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			reg, err := LoadWithOptions(path, "unused", LoadOptions{ClassAllowedDomains: map[string][]string{"http": tc.cap}})
			if err != nil {
				t.Fatal(err)
			}
			args, _ := json.Marshal(map[string]string{"url": tc.url})
			valid, contract, toolErr := reg.Validate(&toolbrokerv1.ToolCall{ToolName: "http.request", ArgsJson: string(args)})
			if valid != tc.allowed {
				t.Fatalf("valid=%v error=%v", valid, toolErr)
			}
			if !valid && toolErr.GetErrorClass() != commonv1.ErrorClass_ERROR_CLASS_POLICY_DENIED {
				t.Fatal(toolErr)
			}
			if !reflect.DeepEqual(contract.GetAllowedDomains(), tc.contract) {
				t.Fatal("cap changed release authority")
			}
		})
	}
}

func TestNetworkCapChecksExpectedURLAndEveryBatchMember(t *testing.T) {
	reg := &Registry{classAllowedDomains: map[string][]string{"web": {"example.com"}}}
	contract := &toolbrokerv1.ToolContract{ToolClass: "web"}
	for _, args := range []string{`{"expected_url":"https://outside.invalid/"}`, `{"urls":["https://example.com/","https://outside.invalid/"]}`} {
		if reg.validateNetworkCap(&toolbrokerv1.ToolCall{ArgsJson: args}, contract) == nil {
			t.Fatal("missed network destination", args)
		}
	}
}
