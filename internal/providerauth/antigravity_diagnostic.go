package providerauth

import "context"

// AntigravityDiagnostic contains only fixed control-plane phases, outcomes and
// numeric status. It never carries URLs, errors, credentials or routing data.
type AntigravityDiagnostic struct {
	Stage      string `json:"stage"`
	Outcome    string `json:"outcome"`
	HTTPStatus int    `json:"http_status,omitempty"`
}
type antigravityDiagnosticKey struct{}

func WithAntigravityDiagnostics(ctx context.Context, observe func(AntigravityDiagnostic)) context.Context {
	return context.WithValue(ctx, antigravityDiagnosticKey{}, observe)
}

func observeAntigravity(ctx context.Context, stage, outcome string, status int) {
	if stage != "oauth_refresh" && stage != "paid_discovery" && stage != "refresh_persistence" {
		return
	}
	switch outcome {
	case "started", "completed", "response_decoded", "deadline", "request_failed", "http_rejected", "invalid_response", "store_failed":
	default:
		return
	}
	if status < 100 || status > 599 {
		status = 0
	}
	if f, ok := ctx.Value(antigravityDiagnosticKey{}).(func(AntigravityDiagnostic)); ok && f != nil {
		f(AntigravityDiagnostic{Stage: stage, Outcome: outcome, HTTPStatus: status})
	}
}

// A refresh may have consumed upstream rotation even if persistence failed.
// Such a trace must never grant another control-plane or model attempt.
func UnsafeAntigravityRefresh(events []AntigravityDiagnostic) bool {
	started, completed, persisted := false, false, false
	for _, d := range events {
		if d.Stage == "oauth_refresh" {
			if d.Outcome == "started" {
				started = true
			}
			if d.Outcome == "completed" {
				completed = true
			}
			if d.Outcome != "started" && d.Outcome != "response_decoded" && d.Outcome != "completed" {
				return true
			}
		}
		if d.Stage == "refresh_persistence" {
			if d.Outcome != "completed" {
				return true
			}
			persisted = true
		}
	}
	return started && (!completed || !persisted)
}

func ValidAntigravityDiagnostics(events []AntigravityDiagnostic) bool {
	if len(events) > 32 {
		return false
	}
	for _, d := range events {
		switch d.Stage {
		case "oauth_refresh", "refresh_persistence", "paid_discovery":
		default:
			return false
		}
		switch d.Outcome {
		case "started", "completed", "response_decoded", "deadline", "request_failed", "http_rejected", "invalid_response", "store_failed":
		default:
			return false
		}
		if d.HTTPStatus != 0 && (d.HTTPStatus < 100 || d.HTTPStatus > 599) {
			return false
		}
	}
	return true
}
