package extensions

import (
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/security/egress"
)

func configuration(m Manifest, values map[string]string, refs map[string]actionauth.CredentialRef) ([]string, error) {
	declared := map[string]Setting{}
	missing := []string{}
	for _, setting := range m.Configuration {
		declared[setting.Name] = setting
		value := values[setting.Name]
		if value == "" {
			if setting.Required {
				missing = append(missing, setting.Name)
			}
			continue
		}
		if len(value) > 1024 || strings.ContainsAny(value, "\x00\r\n") {
			return nil, ErrManifest
		}
		if setting.Type == "public_origin" {
			u, err := (egress.Policy{}).ValidateURL(value)
			if err != nil || u.RawQuery != "" || u.RawPath != "" || u.Path != "" || value != u.Scheme+"://"+u.Host || strings.ToLower(u.Host) != u.Host {
				return nil, ErrManifest
			}
		}
	}
	for name := range values {
		if _, ok := declared[name]; !ok {
			return nil, ErrManifest
		}
	}
	credentialFields := map[string]Credential{}
	for _, input := range m.Credentials {
		credentialFields[input.Input] = input
		ref := refs[input.Input]
		if ref.Alias == "" {
			if input.Required {
				missing = append(missing, "credential:"+input.Input)
			}
			continue
		}
		if len(ref.Alias) > 160 || ref.Field != input.Field || strings.ContainsAny(ref.Alias, "\x00\r\n") {
			return nil, ErrManifest
		}
	}
	for input, ref := range refs {
		if _, ok := credentialFields[input]; !ok || ref.Alias == "" {
			return nil, ErrManifest
		}
	}
	sort.Strings(missing)
	return missing, nil
}

func resources(r Record) []actionauth.ResourceScope {
	result := make([]actionauth.ResourceScope, 0, len(r.Package.Manifest.Resources))
	for _, resource := range r.Package.Manifest.Resources {
		if origin := r.Configuration[resource.OriginSetting]; origin != "" {
			result = append(result, actionauth.ResourceScope{Origin: origin, PathPrefix: resource.PathPrefix})
		}
	}
	return result
}
func resourceWithin(scopes []actionauth.ResourceScope, raw string) bool {
	u, err := (egress.Policy{}).ValidateURL(raw)
	if err != nil || u.RawPath != "" || u.Path != "" && !canonicalPath(strings.TrimSuffix(u.Path, "/")) && u.Path != "/" {
		return false
	}
	path := u.Path
	if path == "" {
		path = "/"
	}
	for _, scope := range scopes {
		if scope.Origin == u.Scheme+"://"+u.Host && canonicalPath(scope.PathPrefix) && pathWithin(scope.PathPrefix, path) {
			return true
		}
	}
	return false
}
func validateScopes(parent []actionauth.ResourceScope, scopes []actionauth.ResourceScope) error {
	if len(scopes) == 0 || len(scopes) > 8 {
		return ErrAccess
	}
	for _, scope := range scopes {
		u, err := url.Parse(scope.Origin)
		if err != nil || scope.Origin != u.Scheme+"://"+u.Host || u.Path != "" || u.RawQuery != "" || !canonicalPath(scope.PathPrefix) || !resourceWithin(parent, scope.Origin+scope.PathPrefix) {
			return ErrAccess
		}
	}
	return nil
}
func operation(m Manifest, name string) (Operation, error) {
	for _, op := range m.Operations {
		if op.Name == name {
			return op, nil
		}
	}
	return Operation{}, ErrUnavailable
}
func selectedTools(m Manifest, tools []string) error {
	if len(tools) == 0 || len(tools) > len(m.Operations) {
		return ErrAccess
	}
	seen := map[string]bool{}
	for _, name := range tools {
		if _, err := operation(m, name); err != nil || seen[name] {
			return ErrAccess
		}
		seen[name] = true
	}
	return nil
}
func readinessURL(r Record) (string, error) {
	for _, resource := range r.Package.Manifest.Resources {
		if pathWithin(resource.PathPrefix, r.Package.Manifest.Readiness.Path) {
			if origin := r.Configuration[resource.OriginSetting]; origin != "" {
				return origin + r.Package.Manifest.Readiness.Path, nil
			}
		}
	}
	return "", fmt.Errorf("mandatory readiness origin is missing")
}
