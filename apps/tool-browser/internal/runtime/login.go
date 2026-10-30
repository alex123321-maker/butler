package runtime

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"

	"github.com/butler/butler/internal/browserauth"
	runtimev1 "github.com/butler/butler/internal/gen/runtime/v1"
	"github.com/butler/butler/internal/security/egress"
)

// This struct travels only on the private Go/helper pipe, never in a tool call.
type loginRequest struct {
	Scope    browserauth.Scope `json:"scope"`
	Username string            `json:"username"`
	Password string            `json:"password"`
}

func decodeLogin(raw string, resolved []*runtimev1.ResolvedCredential) (Request, error) {
	a, err := browserauth.Parse(raw)
	if err != nil || len(resolved) != 2 {
		return Request{}, fmt.Errorf("login requires two broker-resolved fields")
	}
	values := map[string]string{}
	for _, v := range resolved {
		if v == nil || v.Alias != a.UsernameRef.Alias || (v.Field != "username" && v.Field != "password") || v.Value == "" || len(v.Value) > 4096 {
			return Request{}, fmt.Errorf("login credential resolution mismatch")
		}
		if _, exists := values[v.Field]; exists {
			return Request{}, fmt.Errorf("login credential resolution mismatch")
		}
		values[v.Field] = v.Value
	}
	if len(values) != 2 {
		return Request{}, fmt.Errorf("login requires both fields")
	}
	return Request{ToolName: browserauth.Tool, SessionID: a.Browser, ExpectedURL: a.ExpectedURL, ObservationVersion: a.Version, Login: &loginRequest{Scope: a.Scope, Username: values["username"], Password: values["password"]}, SecretValues: []string{values["username"], values["password"]}}, nil
}

func (r *PlaywrightRunner) login(ctx context.Context, req Request) (Result, error) {
	proxy := r.proxies[req.SessionID]
	if proxy == nil || req.Login == nil {
		return Result{}, egress.ErrDenied
	}
	if _, err := r.policy.ValidateURL(req.Login.Scope.TargetURL); err != nil {
		return Result{}, err
	}
	proxy.Freeze()
	defer proxy.Freeze()
	req.LoginPhase = "plan"
	plan, err := r.exchange(ctx, req)
	if err != nil || !plan.OK {
		return plan, err
	}
	if len(plan.LoginNonce) != 36 {
		return Result{}, egress.ErrDenied
	}
	deadline := time.Now().Add(25 * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	if err := proxy.AllowUntil(deadline); err != nil {
		return Result{}, err
	}
	req.LoginPhase, req.LoginNonce = "submit", plan.LoginNonce
	result, err := r.exchange(ctx, req)
	proxy.Freeze()
	// No helper-supplied assertion may become a public authentication proof.
	result.Authentication = nil
	if err != nil || !result.OK || result.LoginPosts != 1 || result.NetworkState != "frozen" {
		return result, err
	}
	if result.FinalURL != result.LoginURL || result.LoginURL == req.ExpectedURL || !verifyLoginAccess(ctx, r.policy, req.Login.Scope, result.LoginURL, result.LoginCookies) {
		return result, nil
	}
	result.Authentication = &browserauth.Proof{Protocol: browserauth.Verification, SubmittedOnce: true, AuthenticatedRead: true, AnonymousChallenge: true}
	return result, nil
}

// GET-only, independently guarded transport. No browser page/script assertion,
// test oracle, response body, or HTTP 200 alone can confirm authentication.
// Rotating-cookie responses are deliberately unsupported in protocol v1.
func verifyLoginAccess(ctx context.Context, policy egress.Policy, scope browserauth.Scope, landing string, cookies []Cookie) bool {
	u, err := policy.ValidateURL(landing)
	if err != nil || u.RawQuery != "" || browserauth.Origin(landing) != browserauth.Origin(scope.TargetURL) || landing == scope.ExpectedURL || len(cookies) == 0 || len(cookies) > 64 {
		return false
	}
	jar, _ := cookiejar.New(nil)
	for _, c := range cookies {
		if !strings.EqualFold(strings.TrimPrefix(c.Domain, "."), u.Hostname()) || c.Name == "" || len(c.Name) > 256 || len(c.Value) > 8192 || !strings.HasPrefix(c.Path, "/") || (c.Secure && u.Scheme != "https") {
			return false
		}
		// Host-only in this jar; no domain/parent-domain expansion.
		jar.SetCookies(u, []*http.Cookie{{Name: c.Name, Value: c.Value, Path: c.Path, Secure: c.Secure, HttpOnly: c.HTTPOnly}})
	}
	probe := func(auth bool) (int, string, bool) {
		client := egress.NewClient(policy, 6*time.Second)
		if auth {
			client.Jar = jar
		}
		defer client.CloseIdleConnections()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, landing, nil)
		if err != nil {
			return 0, "", false
		}
		req.Header.Set("Cache-Control", "no-store")
		response, err := client.Do(req)
		if err != nil {
			return 0, "", false
		}
		defer response.Body.Close()
		n, readErr := io.Copy(io.Discard, io.LimitReader(response.Body, 65537))
		if readErr != nil || n > 65536 || len(response.Cookies()) != 0 {
			return 0, "", false
		}
		return response.StatusCode, response.Header.Get("Location"), true
	}
	status, _, ok := probe(true)
	if !ok || status != http.StatusOK {
		return false
	}
	status, location, ok := probe(false)
	if !ok {
		return false
	}
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return true
	}
	if status != http.StatusFound && status != http.StatusSeeOther && status != http.StatusTemporaryRedirect && status != http.StatusMovedPermanently {
		return false
	}
	target, err := url.Parse(location)
	if err != nil {
		return false
	}
	return u.ResolveReference(target).String() == scope.ExpectedURL
}
