package actionauth

import (
	"context"
	"strings"
	"testing"
	"time"

	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
)

func TestPublicReadNeedsExactReleaseProofAndChangedPathQueryCannotReuseIt(t *testing.T) {
	s, _, reader, _, call, contract := fixture(t)
	call.ArgsJson = `{"method":"GET","url":"https://recipient.invalid/private-context/path?q=private-context"}`
	prepared, err := s.Prepare(context.Background(), call, contract)
	if err != nil || !prepared.RequiresApproval {
		t.Fatal("arbitrary public GET did not request release approval")
	}
	if _, err = s.Authorize(context.Background(), call, contract, prepared.Binding.ActionID, "", ""); err == nil {
		t.Fatal("public GET released without proof")
	}
	reader.approved = ApprovalSnapshot{ApprovalID: "release", Status: "approved", Binding: prepared.Binding, ExpiresAt: s.now().Add(time.Hour), ResolvedAt: s.now()}
	grant, err := s.Authorize(context.Background(), call, contract, prepared.Binding.ActionID, "release", "")
	if err != nil {
		t.Fatal("exact authorized read did not work")
	}
	if _, err = s.Verify(context.Background(), call, contract, grant.ID); err != nil {
		t.Fatal(err)
	}
	for _, changed := range []string{`{"method":"GET","url":"https://recipient.invalid/other-path?q=private-context"}`, `{"method":"GET","url":"https://recipient.invalid/private-context/path?q=other"}`} {
		call.ArgsJson = changed
		if _, err = s.Verify(context.Background(), call, contract, grant.ID); err == nil {
			t.Fatal("changed release reused approval")
		}
	}
}

func TestDeclaredOperatorReadAndPrivateResources(t *testing.T) {
	s, _, _, _, call, contract := fixture(t)
	contract.AllowedDomains = []string{"example.com"}
	call.ArgsJson = `{"method":"GET","url":"https://example.com/read?q=selected-query"}`
	prepared, err := s.Prepare(context.Background(), call, contract)
	if err != nil || prepared.RequiresApproval {
		t.Fatal("explicit operator read declaration denied")
	}
	if _, err = s.Authorize(context.Background(), call, contract, prepared.Binding.ActionID, "", ""); err != nil {
		t.Fatal(err)
	}
	for _, resource := range []string{"http://127.0.0.1/private", "http://[::ffff:10.0.0.1]/", "http://169.254.169.254/", "https://example.com:2375/"} {
		call.ToolCallId = resource
		call.ArgsJson = `{"method":"GET","url":"` + resource + `"}`
		if _, err = s.Prepare(context.Background(), call, contract); err == nil {
			t.Fatal("internal resource accepted")
		}
	}
}

func TestReleaseMandateRevocationStopsBeforeCredentialInspection(t *testing.T) {
	s, store, reader, inspection, call, contract := fixture(t)
	m := testOwnerMandate(s.now())
	m.OwnerID = reader.current.OwnerID
	store.mandates[m.ID] = m
	reader.current.MandateID = m.ID
	reader.current.Authority = &Assignment{TaskID: reader.current.TaskID, OwnerID: m.OwnerID, GoalID: m.GoalID, GoalRevision: m.GoalRevision, MandateID: m.ID, MandateRevision: m.Revision, Purpose: m.Purpose}
	call.ArgsJson = `{"method":"GET","url":"https://example.invalid/test-service/read?q=private-observation"}`
	prepared, err := s.Prepare(context.Background(), call, contract)
	if err != nil || prepared.RequiresApproval {
		t.Fatal("bounded current purpose mandate did not grant read")
	}
	grant, err := s.Authorize(context.Background(), call, contract, prepared.Binding.ActionID, "", "")
	if err != nil {
		t.Fatal(err)
	}
	m.Status = "revoked"
	now := s.now()
	m.RevokedAt = &now
	store.mandates[m.ID] = m
	beforeInspections := inspection.calls
	if _, err = s.Verify(context.Background(), call, contract, grant.ID); err == nil {
		t.Fatal("revoked release mandate accepted")
	}
	if inspection.calls != beforeInspections {
		t.Fatal("revoked mandate reached credential inspection")
	}
	if release, err := NetworkReleaseRequired(&toolbrokerv1.ToolCall{ToolName: "web.fetch_batch", ArgsJson: `{"urls":["https://example.com/read","http://10.0.0.1/private"]}`}, contract); err == nil || release {
		t.Fatal("batch hid private destination")
	}
	if strings.Contains(prepared.Summary, "private-observation") {
		t.Fatal("approval summary echoed private query")
	}
}
