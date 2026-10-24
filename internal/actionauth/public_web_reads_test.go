package actionauth

import (
	"context"
	"testing"

	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
)

func TestPublicWebReadStandingConsentAcrossAuthorizationAndRevocation(t *testing.T) {
	s, _, reader, _, call, contract := fixture(t)
	call.ArgsJson = `{"method":"GET","url":"https://html.duckduckgo.com/html/?q=season+release+date"}`
	before, err := s.Prepare(context.Background(), call, contract)
	if err != nil || !before.RequiresApproval {
		t.Fatal("default release policy changed", err)
	}
	s.SetPublicWebReads(true)
	prepared, err := s.Prepare(context.Background(), call, contract)
	if err != nil || prepared.RequiresApproval || prepared.Binding.ActionHash != before.Binding.ActionHash {
		t.Fatal("standing URL consent not applied to the exact action", err)
	}
	grant, err := s.Authorize(context.Background(), call, contract, prepared.Binding.ActionID, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Verify(context.Background(), call, contract, grant.ID); err != nil {
		t.Fatal(err)
	}
	reader.current.TaskRevision++
	if _, err = s.Verify(context.Background(), call, contract, grant.ID); err == nil {
		t.Fatal("standing consent bypassed current task revision")
	}
	reader.current.TaskRevision--
	s.SetPublicWebReads(false)
	if _, err = s.Verify(context.Background(), call, contract, grant.ID); err == nil {
		t.Fatal("withdrawn consent retained an unapproved release")
	}
}

func TestPublicWebReadConsentPreservesOtherBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name, tool, args        string
		mode                    int32
		contractApproval        bool
		wantApproval, wantError bool
	}{
		{"fetch", "web.fetch", `{"url":"https://example.com/page?q=hello"}`, 3, false, false, false},
		{"batch", "web.fetch_batch", `{"urls":["https://example.com/a","https://another.example/b"]}`, 3, false, false, false},
		{"browser", "browser.open", `{"url":"https://example.com/"}`, 3, false, false, false},
		{"head", "http.request", `{"method":"HEAD","url":"https://example.com/"}`, 3, false, false, false},
		{"write", "http.request", `{"method":"POST","url":"https://example.com/"}`, 3, false, true, false},
		{"headers", "http.request", `{"url":"https://example.com/","headers":{"X-Private":"value"}}`, 3, false, true, false},
		{"body", "http.request", `{"url":"https://example.com/","body":"private"}`, 3, false, true, false},
		{"mode1", "web.fetch", `{"url":"https://example.com/"}`, 2, false, true, false},
		{"contractApproval", "web.fetch", `{"url":"https://example.com/"}`, 3, true, true, false},
		{"loopback", "web.fetch", `{"url":"http://127.0.0.1/"}`, 3, false, false, true},
		{"privateBatch", "web.fetch_batch", `{"urls":["https://example.com/","http://169.254.169.254/"]}`, 3, false, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, _, reader, _, call, contract := fixture(t)
			s.SetPublicWebReads(true)
			reader.current.AutonomyMode = tc.mode
			call.AutonomyMode = commonv1.AutonomyMode(tc.mode)
			call.ToolName, contract.ToolName, call.ArgsJson = tc.tool, tc.tool, tc.args
			contract.RequiresApproval = tc.contractApproval
			contract.ContractRevision = "3"
			contract.EffectContract = &toolbrokerv1.EffectContract{RetrySafety: "read_only", VerificationMethod: "read_result"}
			if tc.name == "write" {
				contract.EffectContract = nil
			}
			p, err := s.Prepare(context.Background(), call, contract)
			if (err != nil) != tc.wantError {
				t.Fatalf("error=%v, wantError=%v", err, tc.wantError)
			}
			if err != nil {
				return
			}
			if p.RequiresApproval != tc.wantApproval {
				t.Fatalf("approval=%v, want=%v", p.RequiresApproval, tc.wantApproval)
			}
			_, err = s.Authorize(context.Background(), call, contract, p.Binding.ActionID, "", "")
			if (err != nil) != tc.wantApproval {
				t.Fatalf("authorize=%v, wantApproval=%v", err, tc.wantApproval)
			}
		})
	}
}

func TestPublicWebReadConsentCannotReleaseCredentialArguments(t *testing.T) {
	s, _, _, _, call, contract := fixture(t)
	s.SetPublicWebReads(true)
	call.ToolName, contract.ToolName = "web.fetch", "web.fetch"
	call.ArgsJson = `{"url":"https://example.com/","credential_ref":{"alias":"account"}}`
	if release, err := s.networkReleaseRequired(call, contract); err != nil || !release {
		t.Fatal("credential argument acquired public-read release", err)
	}
	call.ArgsJson = `{"url":"https://example.com/"}`
	call.CredentialRefs = []*toolbrokerv1.CredentialRef{{Alias: "account"}}
	if release, err := s.networkReleaseRequired(call, contract); err != nil || !release {
		t.Fatal("credential reference acquired public-read release", err)
	}
}
