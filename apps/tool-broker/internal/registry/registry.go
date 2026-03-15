package registry

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
)

type Registry struct {
	toolsByName map[string]*toolbrokerv1.ToolContract
}

type fileConfig struct {
	Tools []toolConfig `json:"tools"`
}

type toolConfig struct {
	ToolName               string   `json:"tool_name"`
	Description            string   `json:"description"`
	ToolClass              string   `json:"tool_class"`
	InputSchemaJSON        string   `json:"input_schema_json"`
	OutputSchemaJSON       string   `json:"output_schema_json"`
	RuntimeTarget          string   `json:"runtime_target"`
	RiskLevel              string   `json:"risk_level"`
	SupportsCredentialRefs bool     `json:"supports_credential_refs"`
	RequiresApproval       bool     `json:"requires_approval"`
	SupportsStreaming      bool     `json:"supports_streaming"`
	Status                 string   `json:"status"`
	AllowedDomains         []string `json:"allowed_domains"`
	AllowedTools           []string `json:"allowed_tools"`
}

func Load(path string, defaultTarget string) (*Registry, error) {
	path = filepath.Clean(strings.TrimSpace(path))
	if path == "" {
		return nil, fmt.Errorf("tool registry path is required")
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read tool registry: %w", err)
	}
	var cfg fileConfig
	if err := json.Unmarshal(contents, &cfg); err != nil {
		return nil, fmt.Errorf("decode tool registry: %w", err)
	}
	registry := &Registry{toolsByName: make(map[string]*toolbrokerv1.ToolContract, len(cfg.Tools))}
	for _, item := range cfg.Tools {
		name := strings.TrimSpace(item.ToolName)
		if name == "" {
			return nil, fmt.Errorf("tool_name is required")
		}
		if _, exists := registry.toolsByName[name]; exists {
			return nil, fmt.Errorf("duplicate tool_name %q", name)
		}
		status := strings.TrimSpace(item.Status)
		if status == "" {
			status = "enabled"
		}
		runtimeTarget := strings.TrimSpace(item.RuntimeTarget)
		if runtimeTarget == "" {
			runtimeTarget = strings.TrimSpace(defaultTarget)
		}
		registry.toolsByName[name] = &toolbrokerv1.ToolContract{
			ToolName:               name,
			Description:            strings.TrimSpace(item.Description),
			ToolClass:              strings.TrimSpace(item.ToolClass),
			InputSchemaJson:        strings.TrimSpace(item.InputSchemaJSON),
			OutputSchemaJson:       strings.TrimSpace(item.OutputSchemaJSON),
			RuntimeTarget:          runtimeTarget,
			RiskLevel:              strings.TrimSpace(item.RiskLevel),
			SupportsCredentialRefs: item.SupportsCredentialRefs,
			RequiresApproval:       item.RequiresApproval,
			SupportsStreaming:      item.SupportsStreaming,
			Status:                 status,
			AllowedDomains:         append([]string(nil), item.AllowedDomains...),
			AllowedTools:           append([]string(nil), item.AllowedTools...),
		}
	}
	return registry, nil
}

func (r *Registry) List(toolClass string, includeDisabled bool) []*toolbrokerv1.ToolContract {
	if r == nil {
		return nil
	}
	toolClass = strings.TrimSpace(toolClass)
	result := make([]*toolbrokerv1.ToolContract, 0, len(r.toolsByName))
	for _, contract := range r.toolsByName {
		if toolClass != "" && contract.GetToolClass() != toolClass {
			continue
		}
		if !includeDisabled && strings.EqualFold(contract.GetStatus(), "disabled") {
			continue
		}
		result = append(result, cloneContract(contract))
	}
	sort.Slice(result, func(i, j int) bool { return result[i].GetToolName() < result[j].GetToolName() })
	return result
}

func (r *Registry) Get(toolName string) (*toolbrokerv1.ToolContract, bool) {
	if r == nil {
		return nil, false
	}
	contract, ok := r.toolsByName[strings.TrimSpace(toolName)]
	if !ok {
		return nil, false
	}
	return cloneContract(contract), true
}

func (r *Registry) Validate(call *toolbrokerv1.ToolCall) (bool, *toolbrokerv1.ToolContract, *toolbrokerv1.ToolError) {
	if call == nil {
		return false, nil, validationError("tool_call is required")
	}
	toolName := strings.TrimSpace(call.GetToolName())
	if toolName == "" {
		return false, nil, validationError("tool_name is required")
	}
	contract, ok := r.Get(toolName)
	if !ok {
		return false, nil, validationError("tool contract not found")
	}
	if strings.EqualFold(contract.GetStatus(), "disabled") {
		return false, contract, validationError("tool is disabled")
	}
	if strings.TrimSpace(call.GetArgsJson()) == "" {
		return false, contract, validationError("args_json is required")
	}
	if len(call.GetCredentialRefs()) > 0 && !contract.GetSupportsCredentialRefs() {
		return false, contract, validationError("tool does not support credential_refs")
	}
	if err := ValidateArgs(contract.GetInputSchemaJson(), call.GetArgsJson()); err != nil {
		return false, contract, validationError(err.Error())
	}
	return true, contract, nil
}

func cloneContract(contract *toolbrokerv1.ToolContract) *toolbrokerv1.ToolContract {
	if contract == nil {
		return nil
	}
	clone := *contract
	clone.AllowedDomains = append([]string(nil), contract.GetAllowedDomains()...)
	clone.AllowedTools = append([]string(nil), contract.GetAllowedTools()...)
	return &clone
}

func validationError(message string) *toolbrokerv1.ToolError {
	return &toolbrokerv1.ToolError{ErrorClass: commonv1.ErrorClass_ERROR_CLASS_VALIDATION_ERROR, Message: message, Retryable: false}
}

type schemaNode struct {
	Type                 string                `json:"type"`
	Properties           map[string]schemaNode `json:"properties"`
	Required             []string              `json:"required"`
	Enum                 []any                 `json:"enum"`
	Items                *schemaNode           `json:"items"`
	AdditionalProperties *bool                 `json:"additionalProperties"`
}

func ValidateArgs(schemaJSON, argsJSON string) error {
	var args any
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return fmt.Errorf("args_json must be valid JSON")
	}
	if strings.TrimSpace(schemaJSON) == "" {
		return nil
	}
	var schema schemaNode
	if err := json.Unmarshal([]byte(schemaJSON), &schema); err != nil {
		return fmt.Errorf("input_schema_json must be valid JSON schema")
	}
	return validateValue(args, schema, "args")
}

func validateValue(value any, schema schemaNode, path string) error {
	typ := schema.Type
	if typ == "" && len(schema.Properties) > 0 {
		typ = "object"
	}
	switch typ {
	case "", "object":
		objectValue, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("%s must be an object", path)
		}
		required := make(map[string]struct{}, len(schema.Required))
		for _, name := range schema.Required {
			required[name] = struct{}{}
			if _, ok := objectValue[name]; !ok {
				return fmt.Errorf("%s.%s is required", path, name)
			}
		}
		allowAdditional := true
		if schema.AdditionalProperties != nil {
			allowAdditional = *schema.AdditionalProperties
		}
		for name, item := range objectValue {
			propertySchema, ok := schema.Properties[name]
			if !ok {
				if !allowAdditional {
					return fmt.Errorf("%s.%s is not allowed", path, name)
				}
				continue
			}
			if err := validateValue(item, propertySchema, path+"."+name); err != nil {
				return err
			}
		}
		if len(required) == 0 && len(schema.Properties) == 0 {
			return nil
		}
	case "string":
		if _, ok := value.(string); !ok {
			return fmt.Errorf("%s must be a string", path)
		}
	case "boolean":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("%s must be a boolean", path)
		}
	case "number":
		if _, ok := value.(float64); !ok {
			return fmt.Errorf("%s must be a number", path)
		}
	case "integer":
		number, ok := value.(float64)
		if !ok || number != float64(int64(number)) {
			return fmt.Errorf("%s must be an integer", path)
		}
	case "array":
		items, ok := value.([]any)
		if !ok {
			return fmt.Errorf("%s must be an array", path)
		}
		if schema.Items != nil {
			for index, item := range items {
				if err := validateValue(item, *schema.Items, fmt.Sprintf("%s[%d]", path, index)); err != nil {
					return err
				}
			}
		}
	default:
		return fmt.Errorf("%s uses unsupported schema type %q", path, typ)
	}
	if len(schema.Enum) > 0 {
		for _, candidate := range schema.Enum {
			if reflect.DeepEqual(candidate, value) {
				return nil
			}
		}
		return fmt.Errorf("%s must match an allowed enum value", path)
	}
	return nil
}
