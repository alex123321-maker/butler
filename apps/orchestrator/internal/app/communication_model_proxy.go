package app

import (
	"errors"
	"net/http"
	"net/url"
)

// Validate effective proxy selection (including NO_PROXY) for discovery,
// inference and OAuth before the operator profile can release credentials.
func communicationModelProxy(raw string, selectProxy func(*http.Request) (*url.URL, error)) (*url.URL, error) {
	if len(raw) == 0 || len(raw) > 4096 {
		return nil, errors.New("model proxy unavailable")
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return nil, errors.New("model proxy unavailable")
	}
	for _, endpoint := range []string{"https://daily-cloudcode-pa.googleapis.com/v1internal:loadCodeAssist", "https://daily-cloudcode-pa.googleapis.com/v1internal:streamGenerateContent", "https://aicode.googleapis.com/v1:streamGenerateContent", "https://oauth2.googleapis.com/token"} {
		req, _ := http.NewRequest(http.MethodPost, endpoint, nil)
		selected, err := selectProxy(req)
		if err != nil || selected == nil || selected.String() != u.String() {
			return nil, errors.New("model proxy selection denied")
		}
	}
	return u, nil
}
