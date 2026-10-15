package main

import (
	"time"

	"github.com/butler/butler/internal/providerauth"
)

type authInspection struct {
	Connected  bool       `json:"connected"`
	Expired    bool       `json:"expired"`
	RefreshDue bool       `json:"refresh_due"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	CheckedAt  time.Time  `json:"checked_at"`
}

// Project existing State onto an operator-only expiry observation. Account,
// pending login fields and credentials are never emitted. No HTTP/write occurs.
func inspectAuth(s providerauth.ProviderState, now time.Time) authInspection {
	r := authInspection{Connected: s.Connected, CheckedAt: now.UTC(), ExpiresAt: s.ExpiresAt}
	if s.ExpiresAt != nil {
		r.Expired = !s.ExpiresAt.After(now)
		r.RefreshDue = !s.ExpiresAt.After(now.Add(time.Minute))
	}
	return r
}
