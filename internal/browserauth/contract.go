// Package browserauth defines the closed, value-free browser-login protocol.
// It grants no authority; Broker must validate the owning vault request.
package browserauth

import (
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"strings"
)

const Tool = "browser.login"
const Revision = "1"
const Verification = "browser_login_v1"

type Ref struct {
	Type  string `json:"type"`
	Alias string `json:"alias"`
	Field string `json:"field"`
}
type Scope struct {
	Browser     string `json:"browser_session_id"`
	ExpectedURL string `json:"expected_url"`
	TargetURL   string `json:"target_url"`
	Form        string `json:"form_selector"`
	Username    string `json:"username_selector"`
	Password    string `json:"password_selector"`
}
type Args struct {
	Scope
	Version     uint64 `json:"observation_version"`
	UsernameRef Ref    `json:"username"`
	PasswordRef Ref    `json:"password"`
}
type Proof struct {
	Protocol           string `json:"protocol"`
	SubmittedOnce      bool   `json:"submitted_once"`
	AuthenticatedRead  bool   `json:"authenticated_read"`
	AnonymousChallenge bool   `json:"anonymous_challenge"`
}

func Origin(raw string) string {
	u, e := url.Parse(raw)
	if e != nil || u.User != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return ""
	}
	return u.Scheme + "://" + u.Host
}
func (s Scope) Validate() error {
	if s.Browser == "" || len(s.Browser) > 256 || Origin(s.ExpectedURL) == "" || Origin(s.TargetURL) != Origin(s.ExpectedURL) {
		return errors.New("invalid login scope")
	}
	for _, raw := range []string{s.ExpectedURL, s.TargetURL} {
		u, e := url.Parse(raw)
		if e != nil || u.RawQuery != "" || u.Fragment != "" || len(raw) > 4096 {
			return errors.New("invalid login URL")
		}
	}
	for _, v := range []string{s.Form, s.Username, s.Password} {
		if strings.TrimSpace(v) == "" || len(v) > 1024 {
			return errors.New("invalid login selector")
		}
	}
	if s.Username == s.Password {
		return errors.New("distinct login fields required")
	}
	return nil
}
func Parse(raw string) (Args, error) {
	var a Args
	d := json.NewDecoder(strings.NewReader(raw))
	d.DisallowUnknownFields()
	if len(raw) > 32768 || d.Decode(&a) != nil || d.Decode(new(any)) != io.EOF || a.Scope.Validate() != nil || a.Version == 0 || a.UsernameRef.Type != "credential_ref" || a.PasswordRef.Type != "credential_ref" || a.UsernameRef.Alias == "" || a.UsernameRef.Alias != a.PasswordRef.Alias || a.UsernameRef.Field != "username" || a.PasswordRef.Field != "password" {
		return Args{}, errors.New("invalid login arguments")
	}
	return a, nil
}
