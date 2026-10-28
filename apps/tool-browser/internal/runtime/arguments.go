package runtime

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	runtimev1 "github.com/butler/butler/internal/gen/runtime/v1"
)

func decodeRequest(tool, payload string, resolved []*runtimev1.ResolvedCredential) (Request, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(payload), &raw); err != nil || raw == nil {
		return Request{}, fmt.Errorf("browser args must be a JSON object")
	}
	var req Request
	safe := make(map[string]json.RawMessage)
	for _, key := range []string{"browser_session_id", "url", "expected_url", "observation_version", "selector", "timeout"} {
		if value, ok := raw[key]; ok {
			safe[key] = value
		}
	}
	encoded, _ := json.Marshal(safe)
	if err := json.Unmarshal(encoded, &req); err != nil {
		return Request{}, fmt.Errorf("invalid browser argument types")
	}
	req.ToolName = tool
	if tool == "browser.submit_get_form" {
		var fields map[string]json.RawMessage
		if json.Unmarshal(raw["fields"], &fields) != nil || fields == nil {
			return req, fmt.Errorf("GET form requires ordinary text fields")
		}
		for _, field := range fields {
			var value string
			if len(field) == 0 || field[0] != '"' || json.Unmarshal(field, &value) != nil {
				return req, fmt.Errorf("GET form accepts only text values")
			}
		}
		if len(resolved) != 0 || json.Unmarshal(raw["fields"], &req.Fields) != nil || req.Fields == nil || len(req.Fields) > 32 {
			return req, fmt.Errorf("GET form requires at most 32 ordinary text fields and no credentials")
		}
		for name, value := range req.Fields {
			if len(name) == 0 || len(name) > 128 || len(value) > 4096 {
				return req, fmt.Errorf("GET form fields exceed their bounds")
			}
		}
	}
	field := ""
	switch tool {
	case "browser.fill", "browser.prepare_fill":
		field = "value"
	case "browser.type", "browser.prepare_type":
		field = "text"
	case "browser.set_cookie":
		field = "cookie"
	case "browser.restore_storage_state":
		field = "storage_state"
	}
	if field == "" {
		return req, nil
	}
	value, ok := raw[field]
	if !ok {
		return req, fmt.Errorf("%s is required", field)
	}
	var literal string
	if json.Unmarshal(value, &literal) == nil && (field == "value" || field == "text") {
		if field == "value" {
			req.Value = literal
		} else {
			req.Text = literal
		}
		return req, nil
	}
	var ref struct {
		Type  string `json:"type"`
		Alias string `json:"alias"`
		Field string `json:"field"`
	}
	if json.Unmarshal(value, &ref) != nil || ref.Type != "credential_ref" || ref.Alias == "" || ref.Field == "" {
		return req, fmt.Errorf("%s must use a typed credential_ref", field)
	}
	secret := ""
	found := false
	for _, cred := range resolved {
		if cred != nil && cred.GetAlias() == ref.Alias && cred.GetField() == ref.Field {
			secret, found = cred.GetValue(), true
			break
		}
	}
	if !found {
		return req, fmt.Errorf("credential_ref was not resolved by Tool Broker")
	}
	req.SecretValues = []string{secret}
	switch field {
	case "value":
		req.Value = secret
	case "text":
		req.Text = secret
	case "cookie":
		if json.Unmarshal([]byte(secret), &req.Cookie) != nil || req.Cookie == nil || req.Cookie.Name == "" || req.Cookie.Domain == "" {
			return req, fmt.Errorf("resolved cookie credential is invalid")
		}
		req.SecretValues = append(req.SecretValues, req.Cookie.Value)
	case "storage_state":
		if json.Unmarshal([]byte(secret), &req.StorageState) != nil || req.StorageState == nil {
			return req, fmt.Errorf("resolved storage-state credential is invalid")
		}
		for _, cookie := range req.StorageState.Cookies {
			req.SecretValues = append(req.SecretValues, cookie.Value)
		}
		for _, origin := range req.StorageState.Origins {
			for _, entry := range origin.LocalStorage {
				req.SecretValues = append(req.SecretValues, entry.Value)
			}
		}
	}
	return req, nil
}
func validateAuthScope(req Request, origin string) error {
	u, _ := url.Parse(origin)
	validCookie := func(cookie Cookie) bool {
		return u != nil && strings.EqualFold(strings.TrimPrefix(cookie.Domain, "."), u.Hostname()) && (!cookie.Secure || u.Scheme == "https")
	}
	if req.Cookie != nil && !validCookie(*req.Cookie) {
		return fmt.Errorf("cookie domain must match the browser session origin")
	}
	if req.StorageState != nil {
		for _, cookie := range req.StorageState.Cookies {
			if !validCookie(cookie) {
				return fmt.Errorf("storage cookies cannot expand the browser session scope")
			}
		}
		for _, entry := range req.StorageState.Origins {
			if originOf(entry.Origin) != origin {
				return fmt.Errorf("storage origins cannot expand the browser session scope")
			}
		}
	}
	return nil
}
