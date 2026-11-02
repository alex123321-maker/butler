package externalauth

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"

	"github.com/butler/butler/internal/browserpage"
	"github.com/butler/butler/internal/browsersso"
	"github.com/butler/butler/internal/jsonstrict"
)

const RequestTool = "credentials.request_external_login"
const Consent = "browser_external_google_login_once_v1"

const ScopeSchema = `{"type":"object","required":["browser_session_id","rp_page","rp_url","provider","verify_url"],"properties":{"browser_session_id":{"type":"string"},"rp_page":{"type":"object","required":["page_id","document_id","observation_version"],"properties":{"page_id":{"type":"string"},"document_id":{"type":"string"},"observation_version":{"type":"integer","minimum":1}},"additionalProperties":false},"rp_url":{"type":"string"},"provider":{"type":"string","enum":["google"]},"verify_url":{"type":"string"}},"additionalProperties":false}`

type Request struct {
	RecordID string `json:"record_id"`
	Purpose  string `json:"purpose"`
	Scope    Scope  `json:"external_scope"`
}

func ParseRequest(raw string) (Request, error) {
	var a Request
	if len(raw) > 8192 || jsonstrict.Decode([]byte(raw), &a) != nil || a.Scope.Validate() != nil || len(a.RecordID) != 69 || !strings.HasPrefix(a.RecordID, "cred_") || strings.TrimSpace(a.Purpose) == "" || len(a.Purpose) > 500 {
		return Request{}, ErrProvider
	}
	if b, e := hex.DecodeString(a.RecordID[5:]); e != nil || len(b) != 32 || strings.ToLower(a.RecordID) != a.RecordID {
		return Request{}, ErrProvider
	}
	return a, nil
}

// Scope identifies the source RP document and one provider handoff. It contains
// no authorization URL, token, code, secret reference, or destination chosen by
// a page script. A configured operator profile must additionally match it.
type Scope struct {
	Browser   string          `json:"browser_session_id"`
	RPPage    browserpage.Ref `json:"rp_page"`
	RPURL     string          `json:"rp_url"`
	Provider  string          `json:"provider"`
	VerifyURL string          `json:"verify_url"`
}

func (s Scope) Validate() error {
	rp := browsersso.Origin(s.RPURL)
	if !browserpage.ID(s.Browser) || !s.RPPage.Valid() || rp == "" || s.Provider != "google" || browsersso.Origin(s.VerifyURL) != rp || s.VerifyURL == s.RPURL {
		return ErrProvider
	}
	return nil
}

// Targets excludes private authorize/callback query material. Both provider
// destinations are fixed by code, not by the model or an operator download.
func (s Scope) Targets() []string {
	return []string{s.RPURL, s.VerifyURL, googleAuthorize, googleToken, googleKeys}
}

// Profile is non-secret operator configuration. Client identity is pinned by
// fingerprint; it never changes during a retained attempt or process restart.
type Profile struct {
	RPURL             string `json:"rp_url"`
	VerifyURL         string `json:"verify_url"`
	RedirectURL       string `json:"redirect_uri"`
	ClientFingerprint string `json:"client_fingerprint"`
}

func ProfileFor(cfg GoogleConfig, rp, verify string) (Profile, error) {
	origin := browsersso.Origin(rp)
	if _, e := NewGoogle(cfg); e != nil || origin == "" || rp != origin+"/" || verify != origin+"/auth/google/account" || cfg.RedirectURL != origin+"/auth/google/callback" {
		return Profile{}, ErrProvider
	}
	h := sha256.Sum256([]byte("google-client-v1\x00" + cfg.ClientID + "\x00" + cfg.RedirectURL))
	return Profile{RPURL: rp, VerifyURL: verify, RedirectURL: cfg.RedirectURL, ClientFingerprint: hex.EncodeToString(h[:])}, nil
}

func (p Profile) Matches(s Scope) bool {
	return p.Validate() == nil && s.Validate() == nil && s.RPURL == p.RPURL && s.VerifyURL == p.VerifyURL
}

func (p Profile) Validate() error {
	h, e := hex.DecodeString(p.ClientFingerprint)
	origin := browsersso.Origin(p.RPURL)
	if e != nil || len(h) != 32 || p.ClientFingerprint != strings.ToLower(p.ClientFingerprint) || origin == "" || p.RPURL != origin+"/" || p.VerifyURL != origin+"/auth/google/account" || p.RedirectURL != origin+"/auth/google/callback" {
		return ErrProvider
	}
	return nil
}

func (s Scope) Digest() (string, error) {
	if s.Validate() != nil {
		return "", ErrProvider
	}
	b, e := json.Marshal(s)
	if e != nil {
		return "", ErrProvider
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:]), nil
}
