package runtime

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/browserauth"
	"github.com/butler/butler/internal/browsercontrol"
	"github.com/butler/butler/internal/browserpage"
	"github.com/butler/butler/internal/browsersso"
	runtimev1 "github.com/butler/butler/internal/gen/runtime/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/security/egress"
	"google.golang.org/grpc/metadata"
)

type ssoResolver struct{}

func (ssoResolver) LookupIPAddr(_ context.Context, host string) ([]net.IPAddr, error) {
	ip := "93.184.216.34"
	if host == "idp.sso.test" {
		ip = "93.184.216.35"
	} else if host != "rp.sso.test" {
		return nil, egress.ErrDenied
	}
	return []net.IPAddr{{IP: net.ParseIP(ip)}}, nil
}

// Trust is confined to this test process and a fresh Chromium HOME. Neither
// production TLS validation nor the workstation's certificate store is changed.
func ssoTLS(t *testing.T, dir string) (string, string) {
	t.Helper()
	key, e := rsa.GenerateKey(rand.Reader, 2048)
	if e != nil {
		t.Fatal("test certificate generation")
	}
	now := time.Now()
	ca := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Butler isolated SSO test CA"}, NotBefore: now.Add(-time.Minute), NotAfter: now.Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature}
	der, e := x509.CreateCertificate(rand.Reader, ca, ca, &key.PublicKey, key)
	if e != nil {
		t.Fatal("test CA generation")
	}
	caFile := filepath.Join(dir, "ca.pem")
	if os.WriteFile(caFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0600) != nil {
		t.Fatal("test CA file")
	}
	leaf := &x509.Certificate{SerialNumber: big.NewInt(2), DNSNames: []string{"rp.sso.test", "idp.sso.test"}, NotBefore: ca.NotBefore, NotAfter: ca.NotAfter, KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, e = x509.CreateCertificate(rand.Reader, leaf, ca, &key.PublicKey, key)
	if e != nil {
		t.Fatal("test leaf generation")
	}
	cert, keyFile := filepath.Join(dir, "leaf.pem"), filepath.Join(dir, "leaf.key")
	if os.WriteFile(cert, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0600) != nil || os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}), 0600) != nil {
		t.Fatal("test certificate files")
	}
	home := filepath.Join(dir, "home")
	db := filepath.Join(home, ".pki", "nssdb")
	if os.MkdirAll(db, 0700) != nil {
		t.Fatal("isolated certificate database")
	}
	t.Setenv("HOME", home)
	t.Setenv("SSL_CERT_FILE", caFile)
	t.Setenv("NODE_EXTRA_CA_CERTS", caFile) // Same isolated CA for context.request's verified TLS.
	for _, args := range [][]string{{"-N", "-d", "sql:" + db, "--empty-password"}, {"-A", "-d", "sql:" + db, "-n", "butler-sso-test", "-t", "C,,", "-i", caFile}} {
		if exec.Command("certutil", args...).Run() != nil {
			t.Fatal("isolated NSS trust setup (libnss3-tools required)")
		}
	}
	return cert, keyFile
}

func TestBrowserSSOChromiumHTTPS(t *testing.T) {
	if os.Getenv("BUTLER_BROWSER_SSO_E2E") != "1" {
		t.Skip("requires isolated HTTPS Chromium profile")
	}
	if runtime.GOOS != "linux" {
		t.Fatal("required TLS profile must run in isolated Linux HOME")
	}
	binary := os.Getenv("BUTLER_SSO_EVAL_BINARY")
	if binary == "" {
		t.Fatal("required SSO recipient binary missing")
	}
	dir := t.TempDir()
	cert, key := ssoTLS(t, dir)
	for _, mode := range []string{"verified", "late_popup", "wrong_password", "denied_idp_post", "two_popups", "popup_storm", "stale_page", "helper_lost_before_sso", "helper_lost_after_idp_login", "inspect_verified", "inspect_fresh_paths", "inspect_late", "inspect_twins", "inspect_storm", "inspect_stale", "inspect_denied_form", "inspect_idp_denied", "inspect_callback_denied", "inspect_reflected_secret", "inspect_helper_lost"} {
		t.Run(mode, func(t *testing.T) {
			began := time.Now()
			defer func() {
				t.Logf("scope=mechanical_sso_https case=%s wall_ms=%.3f model_calls=0 owner_wait_ms=0", mode, float64(time.Since(began).Microseconds())/1000)
			}()
			caseDir := t.TempDir()
			config, journal := filepath.Join(caseDir, "private.json"), filepath.Join(caseDir, "journal.jsonl")
			initArgs := []string{"--init", "--config", config}
			if mode == "inspect_fresh_paths" {
				initArgs = append(initArgs, "--fresh-form-paths")
			}
			if exec.Command(binary, initArgs...).Run() != nil {
				t.Fatal("private fixture initialization")
			}
			var private struct {
				Username string `json:"username"`
				Password string `json:"password"`
				Fact     string `json:"private_fact"`
			}
			data, e := os.ReadFile(config)
			if e != nil || json.Unmarshal(data, &private) != nil {
				t.Fatal("private fixture read")
			}
			address := func() string {
				l, e := net.Listen("tcp", "127.0.0.1:0")
				if e != nil {
					t.Fatal("fixture port")
				}
				a := l.Addr().String()
				l.Close()
				return a
			}
			rpAddress, idpAddress := address(), address()
			cmd := exec.Command(binary, "--config", config, "--journal", journal, "--rp-listen", rpAddress, "--idp-listen", idpAddress, "--rp-origin", "https://rp.sso.test", "--idp-origin", "https://idp.sso.test", "--tls-cert", cert, "--tls-key", key)
			if mode == "inspect_callback_denied" {
				cmd.Args = append(cmd.Args, "--auto-authorize-return")
			}
			if mode == "inspect_reflected_secret" {
				cmd.Args = append(cmd.Args, "--reflect-form-token")
			}
			if mode == "helper_lost_after_idp_login" {
				cmd.Args = append(cmd.Args, "--login-response-delay", "5s")
			}
			if cmd.Start() != nil {
				t.Fatal("HTTPS fixture startup")
			}
			defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
			policy := egress.Policy{Resolver: ssoResolver{}, Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
				target := rpAddress
				if address == "93.184.216.35:443" {
					target = idpAddress
				} else if address != "93.184.216.34:443" {
					return nil, egress.ErrDenied
				}
				return (&net.Dialer{}).DialContext(ctx, network, target)
			}}
			client := egress.NewClient(policy, time.Second)
			defer client.CloseIdleConnections()
			ready := false
			for range 60 {
				response, err := client.Get("https://rp.sso.test/")
				if err == nil {
					response.Body.Close()
					ready = response.StatusCode == 200
					if ready {
						break
					}
				}
				time.Sleep(50 * time.Millisecond)
			}
			if !ready {
				t.Fatal("trusted HTTPS fixture unavailable")
			}
			script := os.Getenv("BUTLER_BROWSER_E2E_SCRIPT")
			if script == "" {
				script = "../../scripts/browser_runtime.mjs"
			}
			absolute, _ := filepath.Abs(script)
			runner := NewPlaywrightRunnerWithPolicy("node", absolute, policy)
			runner.workspaceRoot = filepath.Clean(filepath.Join(filepath.Dir(absolute), "..", "..", ".."))
			server := NewServer(runner, nil)
			defer server.Close()
			server.RequireDispatchEpoch = true
			raw, _ := json.Marshal(browserpage.Args{URL: "https://rp.sso.test/"})
			ctx := metadata.NewIncomingContext(t.Context(), metadata.Pairs("butler-browser-epoch", "0"))
			open := &runtimev1.ExecuteRequest{ToolCall: &toolbrokerv1.ToolCall{RunId: "sso-run", ToolCallId: "open", ToolName: browserpage.Open, ArgsJson: string(raw)}, Contract: pageContract(browserpage.Open), Context: &runtimev1.ExecutionContext{RunId: "sso-run"}}
			opened, e := server.Execute(ctx, open)
			if e != nil || opened.GetResult().GetStatus() != "completed" {
				t.Fatal("managed HTTPS RP unavailable")
			}
			var observation Result
			if json.Unmarshal([]byte(opened.Result.ResultJson), &observation) != nil || len(observation.PrivatePages) != 1 {
				t.Fatal("missing RP identity")
			}
			page := observation.PrivatePages[0]
			ref := browserpage.Ref{ID: page.ID, DocumentID: page.DocumentID, Version: 7}
			scope := browsersso.Scope{Browser: observation.SessionID, RPPage: ref, RPURL: "https://rp.sso.test/", Trigger: "#login", IdPURL: "https://idp.sso.test/authorize", TargetURL: "https://idp.sso.test/login", VerifyURL: "https://rp.sso.test/account", Form: "form", Username: "input[name=username]", Password: "input[name=password]"}
			if mode == "two_popups" {
				scope.Trigger = "#twins"
			}
			if mode == "late_popup" {
				scope.Trigger = "#late"
			}
			if mode == "popup_storm" {
				scope.Trigger = "#storm"
			}
			if mode == "stale_page" {
				scope.RPPage.DocumentID = strings.Repeat("f", 32)
			}
			if strings.HasPrefix(mode, "inspect_") {
				label := "Sign in with test provider"
				switch mode {
				case "inspect_late":
					label = "Delayed popup"
				case "inspect_twins":
					label = "Two identical titles"
				case "inspect_storm":
					label = "Popup storm"
				}
				trigger, verify := "", ""
				for _, button := range observation.Buttons {
					if button.Text == label {
						trigger = button.Selector
					}
				}
				for _, link := range observation.Links {
					if link.Text == "Protected account" {
						verify = link.Href
					}
				}
				if trigger == "" || verify == "" {
					t.Fatal("RP lacks actual actionable button or account navigation")
				}
				a := browsersso.Inspection{Browser: observation.SessionID, RPPage: ref, RPURL: scope.RPURL, Trigger: trigger, IdPOrigin: "https://idp.sso.test", VerifyURL: verify}
				if mode == "inspect_stale" {
					a.RPPage.DocumentID = strings.Repeat("f", 32)
				}
				d := browsersso.Dispatch{Page: browserpage.Binding{Ref: a.RPPage, URL: a.RPURL, RuntimeVersion: page.RuntimeVersion}, Digest: a.Digest(), ExpiresAt: time.Now().Add(time.Minute), Allowed: []browsersso.NetworkScope{{Origin: "https://rp.sso.test", PathPrefix: "/"}, {Origin: "https://idp.sso.test", PathPrefix: "/"}}}
				if mode == "inspect_idp_denied" {
					d.Denied = []browsersso.NetworkScope{{Origin: "https://idp.sso.test", PathPrefix: "/"}}
				}
				if mode == "inspect_denied_form" {
					d.Denied = []browsersso.NetworkScope{{Origin: "https://idp.sso.test", PathPrefix: "/login"}}
				}
				outgoing := browsercontrol.WithPageDispatch(browsercontrol.WithDispatchEpoch(t.Context(), 0), d.Page)
				md, _ := metadata.FromOutgoingContext(outgoing)
				encoded, _ := json.Marshal(d)
				md = md.Copy()
				md.Set("butler-browser-sso-inspection", string(encoded))
				inspectCtx := metadata.NewIncomingContext(t.Context(), md)
				payload, _ := json.Marshal(a)
				if mode == "inspect_helper_lost" {
					if runner.cmd.Process.Kill() != nil {
						t.Fatal("actual helper kill failed")
					}
				}
				inspected, err := server.Execute(inspectCtx, &runtimev1.ExecuteRequest{ToolCall: &toolbrokerv1.ToolCall{RunId: "sso-run", ToolCallId: "inspect", ToolName: browsersso.InspectTool, ArgsJson: string(payload)}, Contract: browsersso.InspectContract(), Context: &runtimev1.ExecutionContext{RunId: "sso-run"}})
				if err != nil {
					t.Fatal("inspection transport")
				}
				var found Result
				_ = json.Unmarshal([]byte(inspected.Result.ResultJson), &found)
				for _, secret := range []string{private.Username, private.Password, private.Fact, "code=", "state=", "csrf", "login_nonce", "sso_idp_binding"} {
					if strings.Contains(inspected.Result.ResultJson, secret) {
						t.Fatal("private provider data exported by inspection")
					}
				}
				before, err := os.ReadFile(journal)
				if err != nil {
					t.Fatal("inspection journal")
				}
				if strings.Contains(string(before), `"kind":"idp_login"`) || strings.Contains(string(before), `"kind":"callback_accepted"`) {
					t.Fatal("inspection submitted credentials or completed pre-consent SSO")
				}
				want := mode == "inspect_verified" || mode == "inspect_fresh_paths" || mode == "inspect_late"
				if (inspected.Result.Status == "completed") != want {
					t.Fatalf("inspection mismatch: mode=%s status=%s error=%v", mode, inspected.Result.Status, inspected.Result.Error)
				}
				if !want {
					return
				}
				if len(found.InspectionForms) != 1 || found.NetworkState != "frozen" {
					t.Fatal("inspection missing bounded frozen metadata")
				}
				if mode == "inspect_fresh_paths" && (found.InspectionForms[0].TargetURL == "https://idp.sso.test/login" || verify == "https://rp.sso.test/account") {
					t.Fatal("fresh paths were not exercised")
				}
				for _, p := range found.PrivatePages {
					if p.ID == page.ID {
						page = p
					} else if p.State == "ready" {
						t.Fatal("inspected IdP child survived")
					}
				}
				ref = browserpage.Ref{ID: page.ID, DocumentID: page.DocumentID, Version: ref.Version + 1}
				a.RPPage = ref
				scope, err = found.InspectionForms[0].Scope(a)
				if err != nil {
					t.Fatal("inspection produced no usable exact consent scope")
				}
			}
			digest, _ := scope.Digest(0, 1)
			alias := "ssoq_" + strings.Repeat("a", 64)
			args := browsersso.Args{Scope: scope, Digest: digest, UsernameRef: browserauth.Ref{Type: "credential_ref", Alias: alias, Field: "username"}, PasswordRef: browserauth.Ref{Type: "credential_ref", Alias: alias, Field: "password"}}
			dispatch := browsersso.Dispatch{Page: browserpage.Binding{Ref: ref, URL: scope.RPURL, Epoch: 0, RuntimeVersion: page.RuntimeVersion}, Digest: digest, ExpiresAt: time.Now().Add(time.Minute), Allowed: []browsersso.NetworkScope{{Origin: "https://rp.sso.test", PathPrefix: "/"}, {Origin: "https://idp.sso.test", PathPrefix: "/"}}}
			if mode == "denied_idp_post" {
				dispatch.Denied = []browsersso.NetworkScope{{Origin: "https://idp.sso.test", PathPrefix: "/login"}}
			}
			payload, _ := json.Marshal(args)
			privateDispatch, _ := json.Marshal(dispatch)
			ctx = metadata.NewIncomingContext(t.Context(), metadata.Pairs("butler-browser-epoch", "0", "butler-browser-sso", string(privateDispatch)))
			password := private.Password
			if mode == "wrong_password" {
				password += "wrong"
			}
			req := &runtimev1.ExecuteRequest{ToolCall: &toolbrokerv1.ToolCall{RunId: "sso-run", ToolCallId: "sso", ToolName: browsersso.Tool, ArgsJson: string(payload), CredentialRefs: []*toolbrokerv1.CredentialRef{{Type: "credential_ref", Alias: alias, Field: "username"}, {Type: "credential_ref", Alias: alias, Field: "password"}}}, Contract: browsersso.Contract(), Context: &runtimev1.ExecutionContext{RunId: "sso-run"}, ResolvedCredentials: []*runtimev1.ResolvedCredential{{Alias: alias, Field: "username", Value: private.Username}, {Alias: alias, Field: "password", Value: password}}}
			helper := runner.cmd.Process // Capture before concurrent execution; no racy runner-state reads.
			if mode == "helper_lost_before_sso" {
				if helper.Kill() != nil {
					t.Fatal("actual helper kill failed")
				}
			}
			executionCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
			defer cancel()
			type answer struct {
				response *runtimev1.ExecuteResponse
				err      error
			}
			completed := make(chan answer, 1)
			go func() { response, err := server.Execute(executionCtx, req); completed <- answer{response, err} }()
			if mode == "helper_lost_after_idp_login" {
				// Synchronize against the independent durable recipient receipt,
				// not a timing guess or a helper-supplied success marker.
				deadline := time.Now().Add(8 * time.Second)
				for {
					journalData, err := os.ReadFile(journal)
					if err == nil && strings.Contains(string(journalData), `"kind":"idp_login","accepted":true`) {
						break
					}
					if time.Now().After(deadline) {
						t.Fatal("accepted login never reached delayed recipient")
					}
					time.Sleep(5 * time.Millisecond)
				}
				if helper.Kill() != nil {
					t.Fatal("actual in-flight helper kill failed")
				}
			}
			var response *runtimev1.ExecuteResponse
			select {
			case done := <-completed:
				response, e = done.response, done.err
			case <-executionCtx.Done():
				t.Fatal("lost helper did not settle the original call")
			}
			if e != nil {
				t.Fatal("SSO runtime transport failed")
			}
			for _, secret := range []string{private.Username, private.Password, "login_nonce", "login_cookies", "code=", "state=", "sso_rp_auth", "sso_idp_binding"} {
				if strings.Contains(response.Result.ResultJson, secret) {
					t.Fatal("private SSO state exported")
				}
			}
			var out Result
			_ = json.Unmarshal([]byte(response.Result.ResultJson), &out)
			verified := response.Result.Status == "completed" && out.Authentication != nil && out.Authentication.Protocol == browsersso.Verification && out.Authentication.AuthenticatedRead && out.Authentication.AnonymousChallenge && out.Authentication.SubmittedOnce
			wantVerified := mode == "verified" || mode == "late_popup" || mode == "inspect_verified" || mode == "inspect_fresh_paths" || mode == "inspect_late"
			if verified != wantVerified {
				t.Fatalf("SSO proof mismatch: mode=%s status=%s ok=%v proof=%v error=%v", mode, response.Result.Status, out.OK, out.Authentication != nil, response.Result.GetError())
			}
			data, e = os.ReadFile(journal)
			if e != nil {
				t.Fatal("recipient journal missing")
			}
			posts, callbacks, reads := 0, 0, 0
			for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
				if line == "" {
					continue
				}
				var row struct {
					Kind     string `json:"kind"`
					Accepted bool   `json:"accepted"`
				}
				if json.Unmarshal([]byte(line), &row) != nil {
					t.Fatal("recipient journal malformed")
				}
				switch row.Kind {
				case "idp_login":
					posts++
				case "callback_accepted":
					callbacks++
				case "account_read":
					reads++
				}
			}
			if posts > 1 {
				t.Fatal("SSO POST replay")
			}
			if wantVerified && (posts != 1 || callbacks != 1 || reads != 2 || !strings.Contains(out.Text, private.Fact) || out.NetworkState != "frozen") {
				t.Fatal("missing recipient/probe evidence", posts, callbacks, reads)
			}
			if mode == "denied_idp_post" || mode == "stale_page" || mode == "two_popups" || mode == "popup_storm" {
				if posts != 0 {
					t.Fatal("stale or denied authority sent credentials")
				}
			}
			if strings.HasPrefix(mode, "helper_lost_") {
				wantPosts := 0
				if mode == "helper_lost_after_idp_login" {
					wantPosts = 1
				}
				if response.Result.Status == "completed" || out.Authentication != nil || posts != wantPosts || callbacks != 0 || reads != 0 {
					t.Fatal("lost runtime upgraded or replayed an incomplete SSO", posts, callbacks, reads)
				}
				stale, err := server.Execute(ctx, req)
				if err != nil || stale.GetResult().GetStatus() == "completed" {
					t.Fatal("lost context reused after helper restart")
				}
				open.ToolCall.ToolCallId = "fresh-open-after-loss"
				fresh, err := server.Execute(metadata.NewIncomingContext(t.Context(), metadata.Pairs("butler-browser-epoch", "0")), open)
				var next Result
				if err != nil || fresh.GetResult().GetStatus() != "completed" || json.Unmarshal([]byte(fresh.Result.ResultJson), &next) != nil || next.SessionID == "" || next.SessionID == observation.SessionID || strings.Contains(next.Text, private.Fact) {
					t.Fatal("fresh read reused old runtime identity or authentication")
				}
				again, err := os.ReadFile(journal)
				if err != nil || string(again) != string(data) {
					t.Fatal("stale SSO or fresh read changed the recipient journal")
				}
				t.Logf("actual_helper_killed=true fresh_context_distinct=true stale_sso_rejected=true accepted_idp_logins=%d callbacks=0 account_reads=0 no_success_proof=true", posts)
			}
			for _, p := range out.PrivatePages {
				if p.OpenerID != "" && p.State == "ready" {
					t.Fatal("IdP child survived atomic protocol")
				}
			}
		})
	}
}
