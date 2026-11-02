// Package browsersso defines value-free, conditional RP/IdP consent. Consent is
// not execution authority and cannot be consumed by the ordinary login tools.
package browsersso

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"strconv"
	"strings"

	"github.com/butler/butler/internal/browserpage"
)

const Consent = "browser_sso_login_once_v1"

var ErrScope = errors.New("invalid RP/IdP consent scope")

// Scope describes one fresh IdP child of an exact RP document. It deliberately
// contains no authorization URL query, credential reference, cookie or code.
type Scope struct {
	Browser   string          `json:"browser_session_id"`
	RPPage    browserpage.Ref `json:"rp_page"`
	RPURL     string          `json:"rp_url"`
	Trigger   string          `json:"trigger_selector"`
	IdPURL    string          `json:"idp_url"`
	TargetURL string          `json:"target_url"`
	Form      string          `json:"form_selector"`
	Username  string          `json:"username_selector"`
	Password  string          `json:"password_selector"`
	VerifyURL string          `json:"verify_url"`
}

// Origin accepts canonical HTTPS URLs only. Transport must still enforce public
// DNS, ports, TLS and redirect policy; this parser never grants network access.
func Origin(raw string) string {
	u, e := url.Parse(raw)
	if e != nil || len(raw) > 2048 || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Opaque != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.RawFragment != "" || u.RawPath != "" || u.String() != raw || u.Host != strings.ToLower(u.Host) || strings.HasSuffix(u.Hostname(), ".") {
		return ""
	}
	for _, c := range raw {
		if c <= 32 || c >= 127 || c == '\\' {
			return ""
		}
	}
	if u.Port() != "" {
		port, e := strconv.Atoi(u.Port())
		if e != nil || port < 1 || port > 65535 || port == 443 || strconv.Itoa(port) != u.Port() {
			return ""
		}
	}
	if u.Path == "" || strings.Contains(u.Path, "//") || strings.HasSuffix(u.Host, ":") {
		return ""
	}
	for _, part := range strings.Split(u.Path, "/") {
		if part == "." || part == ".." {
			return ""
		}
	}
	return u.Scheme + "://" + u.Host
}

func (s Scope) Validate() error {
	rp, idp := Origin(s.RPURL), Origin(s.IdPURL)
	if !browserpage.ID(s.Browser) || !s.RPPage.Valid() || rp == "" || idp == "" || rp == idp || Origin(s.TargetURL) != idp || Origin(s.VerifyURL) != rp || s.VerifyURL == s.RPURL {
		return ErrScope
	}
	for _, selector := range []string{s.Trigger, s.Form, s.Username, s.Password} {
		if strings.TrimSpace(selector) == "" || len(selector) > 1024 || strings.ContainsAny(selector, "\x00\r\n") {
			return ErrScope
		}
	}
	if s.Username == s.Password {
		return ErrScope
	}
	return nil
}

// Digest binds the immutable scope, context generation and account revision.
// Vault adds request/principal/purpose identity before publishing a card digest.
func (s Scope) Digest(epoch, revision int64) (string, error) {
	if s.Validate() != nil || epoch < 0 || revision < 1 {
		return "", ErrScope
	}
	raw, _ := json.Marshal(struct {
		Protocol string `json:"protocol"`
		Scope    Scope  `json:"scope"`
		Epoch    int64  `json:"epoch"`
		Revision int64  `json:"record_revision"`
	}{Consent, s, epoch, revision})
	h := sha256.Sum256(raw)
	return hex.EncodeToString(h[:]), nil
}
