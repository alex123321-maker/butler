package providerauth

import (
	"context"
	"crypto/subtle"
	"fmt"
	"io"

	"github.com/butler/butler/internal/modelprovider"
)

// AntigravityOriginAudit is safe operator metadata. It contains no identifiers,
// token hashes, expiry, project, credential material or provider messages.
type AntigravityOriginAudit struct {
	StoredCredentials        bool `json:"stored_credentials"`
	OAuthApplicationMatches  bool `json:"oauth_application_matches"`
	OAuthCredentialMatches   bool `json:"oauth_credential_matches"`
	RefreshCredentialMatches bool `json:"refresh_credential_matches"`
}

// AuditAntigravityOrigin compares a private reference import with current stored
// credentials. It never refreshes, writes or contacts a provider, even if expired.
// Only the private operator stdin boundary may supply the reference; no model,
// channel, settings API or runtime receives a new raw-secret input capability.
// A mismatch can reflect refresh rotation and does not identify another account.
func (m *Manager) AuditAntigravityOrigin(ctx context.Context, reference io.Reader) (AntigravityOriginAudit, error) {
	var out AntigravityOriginAudit
	ref, err := readAntigravityPrivateInput(reference)
	if err != nil {
		return out, fmt.Errorf("Antigravity private origin reference is invalid")
	}
	if m == nil {
		return out, fmt.Errorf("Antigravity credential store is unavailable")
	}
	m.refresh.Lock()
	defer m.refresh.Unlock()
	var stored antigravityCredentials
	connected, err := loadSecretJSON(ctx, m.store, modelprovider.ProviderAntigravity, &stored)
	if err != nil {
		return out, fmt.Errorf("Antigravity credential store is unavailable")
	}
	if !connected {
		return out, nil
	}
	if validateAntigravityCredentials(stored) != nil {
		return out, fmt.Errorf("Antigravity stored credentials are invalid")
	}
	out.StoredCredentials = true
	out.OAuthApplicationMatches = stored.ClientID == ref.ClientID
	out.OAuthCredentialMatches = equalPrivateCredential(stored.ClientSecret, ref.ClientSecret)
	out.RefreshCredentialMatches = equalPrivateCredential(stored.RefreshToken, ref.RefreshToken)
	return out, nil
}

func equalPrivateCredential(a, b string) bool {
	aa, bb := []byte(a), []byte(b)
	defer clear(aa)
	defer clear(bb)
	return subtle.ConstantTimeCompare(aa, bb) == 1
}
