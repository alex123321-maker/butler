package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/butler/butler/internal/providerauth"
)

const nativeClientID = "1071006060591-tmhssin2h21lcre235vtolojh4g403ep.apps.googleusercontent.com"

func callbackHandler(state string, codes chan<- string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		if r.Method != http.MethodGet || r.URL.Path != "/oauth-callback" || len(r.URL.RawQuery) > 16384 {
			http.Error(w, "Invalid callback", 400)
			return
		}
		q, err := url.ParseQuery(r.URL.RawQuery)
		if err != nil || len(q["state"]) != 1 || len(q["code"]) != 1 || subtle.ConstantTimeCompare([]byte(q.Get("state")), []byte(state)) != 1 || q.Get("code") == "" || len(q.Get("code")) > 8192 {
			http.Error(w, "Invalid callback", 400)
			return
		}
		select {
		case codes <- q.Get("code"):
			fmt.Fprintln(w, "Butler local research login received. You may close this tab.")
		default:
			http.Error(w, "Callback already received", 409)
		}
	})
}

// This operator-only login has no model context or credential-return endpoint.
// The app reference is read privately; successful OAuth is immediately imported
// through the existing strict encrypted PostgreSQL control plane.
func localLogin(parent context.Context, m *providerauth.Manager, clientFile, urlFile string) error {
	if clientFile == "" || urlFile == "" {
		return errors.New("private application and login URL file references required")
	}
	raw, err := os.ReadFile(clientFile)
	if err != nil || len(raw) > 4096 {
		return errors.New("private native application reference unavailable")
	}
	defer clear(raw)
	var app struct {
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
	}
	if json.Unmarshal(raw, &app) != nil || app.ClientID != nativeClientID || app.ClientSecret == "" {
		return errors.New("unreviewed native application reference")
	}
	ln, err := net.Listen("tcp", "127.0.0.1:51121")
	if err != nil {
		return errors.New("local OAuth callback port 51121 unavailable")
	}
	defer ln.Close()
	ctx, cancel := context.WithTimeout(parent, 15*time.Minute)
	defer cancel()
	verifierBytes := make([]byte, 32)
	stateBytes := make([]byte, 32)
	if _, err = rand.Read(verifierBytes); err != nil {
		return err
	}
	if _, err = rand.Read(stateBytes); err != nil {
		return err
	}
	verifier := base64.RawURLEncoding.EncodeToString(verifierBytes)
	state := base64.RawURLEncoding.EncodeToString(stateBytes)
	challenge := sha256.Sum256([]byte(verifier))
	redirect := "http://localhost:51121/oauth-callback"
	q := url.Values{"client_id": {app.ClientID}, "redirect_uri": {redirect}, "response_type": {"code"}, "scope": {"openid https://www.googleapis.com/auth/userinfo.email https://www.googleapis.com/auth/cloud-platform"}, "access_type": {"offline"}, "prompt": {"consent"}, "state": {state}, "code_challenge": {base64.RawURLEncoding.EncodeToString(challenge[:])}, "code_challenge_method": {"S256"}}
	f, err := os.OpenFile(urlFile, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return errors.New("new private login URL file required")
	}
	_, err = f.WriteString("https://accounts.google.com/o/oauth2/v2/auth?" + q.Encode())
	closeErr := f.Close()
	if err != nil || closeErr != nil {
		return errors.New("private login URL retention failed")
	}
	codes := make(chan string, 1)
	server := http.Server{Handler: callbackHandler(state, codes), ReadHeaderTimeout: 5 * time.Second, MaxHeaderBytes: 32768}
	done := make(chan error, 1)
	go func() { done <- server.Serve(ln) }()
	defer server.Close()
	fmt.Println("Local Google OAuth callback ready; open the private login URL file in your browser. No generation request was made.")
	var code string
	select {
	case code = <-codes:
	case <-ctx.Done():
		return errors.New("local OAuth login expired or stopped")
	case <-done:
		return errors.New("local OAuth callback stopped")
	}
	body := url.Values{"client_id": {app.ClientID}, "client_secret": {app.ClientSecret}, "code": {code}, "grant_type": {"authorization_code"}, "redirect_uri": {redirect}, "code_verifier": {verifier}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://oauth2.googleapis.com/token", strings.NewReader(body.Encode()))
	if err != nil {
		return errors.New("private OAuth exchange construction failed")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	client := http.Client{Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return errors.New("private OAuth exchange failed")
	}
	defer resp.Body.Close()
	tokenBytes, err := io.ReadAll(io.LimitReader(resp.Body, 65537))
	defer clear(tokenBytes)
	if err != nil || len(tokenBytes) > 65536 || resp.StatusCode != 200 {
		return errors.New("private OAuth exchange rejected")
	}
	var token struct {
		Access  string `json:"access_token"`
		Refresh string `json:"refresh_token"`
		Expires int    `json:"expires_in"`
	}
	if json.Unmarshal(tokenBytes, &token) != nil || token.Access == "" || token.Refresh == "" || token.Expires < 60 || token.Expires > 86400 {
		return errors.New("private OAuth response incomplete")
	}
	data, err := json.Marshal(map[string]any{"access_token": token.Access, "refresh_token": token.Refresh, "client_id": app.ClientID, "client_secret": app.ClientSecret, "expires_at": time.Now().UTC().Add(time.Duration(token.Expires) * time.Second)})
	if err != nil {
		return errors.New("private OAuth import construction failed")
	}
	defer clear(data)
	if err = m.ImportAntigravity(ctx, strings.NewReader(string(data)), "issue97-local-browser-login"); err != nil {
		return err
	}
	fmt.Println("Local Google login encrypted in research PostgreSQL. No token material exported.")
	return nil
}
