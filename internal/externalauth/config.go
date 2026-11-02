package externalauth

import (
	"io"

	"github.com/butler/butler/internal/jsonstrict"
)

// ReadGoogleWebConfig accepts Google's privately downloaded web-client file.
// The operator pins the callback independently; a download cannot choose it.
// The caller owns file permissions and must never log this input or config.
func ReadGoogleWebConfig(r io.Reader, expectedRedirect string) (GoogleConfig, error) {
	if r == nil {
		return GoogleConfig{}, ErrProvider
	}
	raw, err := io.ReadAll(io.LimitReader(r, 16385))
	if err != nil || len(raw) > 16384 {
		return GoogleConfig{}, ErrProvider
	}
	var download struct {
		Web struct {
			ClientID     string   `json:"client_id"`
			ClientSecret string   `json:"client_secret"`
			ProjectID    string   `json:"project_id"`
			AuthURI      string   `json:"auth_uri"`
			TokenURI     string   `json:"token_uri"`
			CertURI      string   `json:"auth_provider_x509_cert_url"`
			Redirects    []string `json:"redirect_uris"`
			Origins      []string `json:"javascript_origins"`
		} `json:"web"`
	}
	if jsonstrict.Decode(raw, &download) != nil {
		return GoogleConfig{}, ErrProvider
	}
	w := download.Web
	// Downloads also use Google's legacy authorization URI. Both map to the
	// fixed v2 endpoint in Launch; neither configures a network destination.
	if len(w.Redirects) != 1 || w.Redirects[0] != expectedRedirect || (w.AuthURI != googleAuthorize && w.AuthURI != "https://accounts.google.com/o/oauth2/auth") || w.TokenURI != googleToken || w.CertURI != "https://www.googleapis.com/oauth2/v1/certs" || len(w.Origins) != 0 {
		return GoogleConfig{}, ErrProvider
	}
	cfg := GoogleConfig{ClientID: w.ClientID, ClientSecret: w.ClientSecret, RedirectURL: expectedRedirect}
	if _, err := NewGoogle(cfg); err != nil {
		return GoogleConfig{}, ErrProvider
	}
	return cfg, nil
}
