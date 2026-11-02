package api

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/butler/butler/internal/credentials"
)

type externalRPVault interface {
	ExternalRPHost(string) bool
	StartExternal(context.Context, string) (credentials.ExternalStart, error)
	ExternalCallbackCookieName(context.Context, string) (string, error)
	CompleteExternalAuth(context.Context, string, string, string, bool) error
	ExternalAccount(context.Context, string, string) (credentials.ExternalRPAccount, error)
}

// BrowserExternalRP is an operator-enabled diagnostic RP, not an owner API or
// general OAuth proxy. All private values stay in the browser/provider path.
func BrowserExternalRP(v externalRPVault) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'")
		w.Header().Del("Access-Control-Allow-Origin")
		if v == nil || !v.ExternalRPHost(r.Host) {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if r.URL.RawPath != "" || len(r.URL.RawQuery) > 8192 || r.ContentLength > 0 {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		switch r.URL.Path {
		case "/":
			if r.URL.RawQuery != "" {
				w.WriteHeader(400)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = io.WriteString(w, `<!doctype html><html lang="ru"><meta charset="utf-8"><title>Butler SSO test</title><h1>Тест входа через Google</h1><p>Для одного входа разрешите запрос в защищённом хранилище Butler и откройте предоставленную ссылку в обычном браузере.</p><a href="/auth/google/account">Проверить вход</a></html>`)
		case "/auth/google/start":
			q, e := externalQuery(r.URL.RawQuery, map[string]bool{"ticket": true})
			if e != nil || len(q) != 1 {
				w.WriteHeader(400)
				return
			}
			start, e := v.StartExternal(r.Context(), q.Get("ticket"))
			if e != nil {
				writeJSON(w, 409, map[string]string{"error": "authentication_attempt_unavailable"})
				return
			}
			http.SetCookie(w, &http.Cookie{Name: start.CookieName, Value: start.CookieValue, Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode, Expires: start.ExpiresAt})
			http.Redirect(w, r, start.URL, http.StatusFound)
		case "/auth/google/callback":
			q, e := externalQuery(r.URL.RawQuery, map[string]bool{"state": true, "code": true, "error": true, "scope": true, "authuser": true, "prompt": true, "iss": true, "hd": true, "error_description": true, "error_uri": true})
			if e != nil || q.Get("state") == "" || (q.Get("code") == "") == (q.Get("error") == "") || len(q.Get("code")) > 4096 {
				w.WriteHeader(400)
				return
			}
			name, e := v.ExternalCallbackCookieName(r.Context(), q.Get("state"))
			if e != nil {
				w.WriteHeader(409)
				return
			}
			cookie := ""
			count := 0
			for _, c := range r.Cookies() {
				if c.Name == name {
					cookie = c.Value
					count++
				}
			}
			if count != 1 || v.CompleteExternalAuth(r.Context(), q.Get("state"), q.Get("code"), cookie, q.Get("error") != "") != nil {
				writeJSON(w, 409, map[string]string{"error": "authentication_attempt_unavailable"})
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			message := "Вход подтверждён. Можно вернуться в Butler."
			if q.Get("error") != "" {
				message = "Вход отменён. Можно вернуться в Butler."
			}
			_, _ = io.WriteString(w, `<!doctype html><html lang="ru"><meta charset="utf-8"><title>Butler SSO result</title><h1>`+message+`</h1></html>`)
		case "/auth/google/account":
			if r.URL.RawQuery != "" {
				w.WriteHeader(400)
				return
			}
			var cookie *http.Cookie
			count := 0
			for _, c := range r.Cookies() {
				if strings.HasPrefix(c.Name, "__Host-butler-external-") {
					cookie = c
					count++
				}
			}
			if count != 1 {
				writeJSON(w, 401, map[string]string{"error": "authentication_required"})
				return
			}
			account, e := v.ExternalAccount(r.Context(), cookie.Name, cookie.Value)
			if e != nil {
				writeJSON(w, 401, map[string]string{"error": "authentication_required"})
				return
			}
			writeJSON(w, 200, account)
		default:
			http.NotFound(w, r)
		}
	})
}

func externalQuery(raw string, allowed map[string]bool) (url.Values, error) {
	q, e := url.ParseQuery(raw)
	if e != nil {
		return nil, credentials.ErrVault
	}
	for k, v := range q {
		if !allowed[k] || len(v) != 1 || len(v[0]) > 4096 {
			return nil, credentials.ErrVault
		}
	}
	return q, nil
}
