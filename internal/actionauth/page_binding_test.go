package actionauth

import (
	"context"
	"encoding/json"
	"github.com/butler/butler/internal/browserpage"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"strings"
	"testing"
	"time"
)

type pageScopeFixture struct{ bound browserpage.Binding }

func (f *pageScopeFixture) BrowserReadScope(context.Context, ActionBinding) (string, int64, error) {
	return "", 0, ErrOwnerAuthority
}
func (f *pageScopeFixture) BrowserPageScope(context.Context, ActionBinding, browserpage.Args) (*browserpage.Binding, error) {
	return &f.bound, nil
}
func pageTestContract(tool string) *toolbrokerv1.ToolContract {
	p, v := "unsafe", "none"
	if browserpage.Read(tool) {
		p, v = "read_only", "read_result"
	}
	return &toolbrokerv1.ToolContract{ToolName: tool, Status: "active", RuntimeTarget: "tool-browser:9090", ContractRevision: browserpage.Revision, InputSchemaJson: browserpage.Schema(tool), RequiresApproval: !browserpage.Read(tool), EffectContract: &toolbrokerv1.EffectContract{RetrySafety: p, VerificationMethod: v}}
}
func TestPageActionBindsExactMetadataAndCurrentAuthority(t *testing.T) {
	for _, kind := range []string{"allowed", "missing_reader", "wrong_url", "wrong_reference", "credential_contract", "page_version", "document", "runtime_version", "epoch", "task_revision", "constraints", "task_cancel", "lease", "mandate_revoke", "deny_override"} {
		t.Run(kind, func(t *testing.T) {
			s, store, c, _, call, _ := fixture(t)
			m := testOwnerMandate(s.now())
			m.OwnerID = c.current.OwnerID
			m.Tools = append(m.Tools, browserpage.Snapshot)
			c.current.Authority = &Assignment{TaskID: c.current.TaskID, OwnerID: m.OwnerID, GoalID: m.GoalID, GoalRevision: m.GoalRevision, MandateID: m.ID, MandateRevision: m.Revision, Purpose: m.Purpose}
			c.current.MandateID = m.ID
			store.mandates[m.ID] = m
			ref := browserpage.Ref{ID: strings.Repeat("b", 32), DocumentID: strings.Repeat("c", 32), Version: 1}
			a := browserpage.Args{SessionID: strings.Repeat("a", 32), Page: &ref}
			raw, _ := json.Marshal(a)
			call.ToolName = browserpage.Snapshot
			call.ArgsJson = string(raw)
			contract := pageTestContract(call.ToolName)
			reader := &pageScopeFixture{bound: browserpage.Binding{Ref: ref, URL: "https://example.invalid/test-service/page", Epoch: 2, RuntimeVersion: 7}}
			if kind != "missing_reader" {
				s.SetBrowserScopes(reader)
			}
			switch kind {
			case "wrong_url":
				reader.bound.URL = "https://foreign.invalid/"
			case "wrong_reference":
				reader.bound.ID = strings.Repeat("d", 32)
			case "credential_contract":
				contract.SupportsCredentialRefs = true
			}
			p, e := s.Prepare(context.Background(), call, contract)
			if kind == "missing_reader" || kind == "wrong_url" || kind == "wrong_reference" || kind == "credential_contract" {
				if e == nil {
					t.Fatal("invalid page prepared")
				}
				return
			}
			if e != nil || p.RequiresApproval || p.Binding.BrowserPage == nil {
				t.Fatal("exact read preparation failed", e)
			}
			g, e := s.Authorize(context.Background(), call, contract, p.Binding.ActionID, "", "")
			if e != nil {
				t.Fatal(e)
			}
			switch kind {
			case "page_version":
				reader.bound.Version++
			case "document":
				reader.bound.DocumentID = strings.Repeat("d", 32)
			case "runtime_version":
				reader.bound.RuntimeVersion++
			case "epoch":
				reader.bound.Epoch++
			case "task_revision":
				c.current.TaskRevision++
			case "constraints":
				c.current.ConstraintsRevision++
			case "task_cancel":
				c.current.TaskStatus = "cancelled"
			case "lease":
				c.current.LeaseExpiresAt = s.now().Add(-time.Second)
			case "mandate_revoke":
				m.Status = "revoked"
				store.mandates[m.ID] = m
			case "deny_override":
				c.current.DenyTools = []string{call.ToolName}
			}
			_, e = s.Verify(context.Background(), call, contract, g.ID)
			if (e == nil) != (kind == "allowed") {
				t.Fatal("stale page/authority grant outcome", kind, e)
			}
		})
	}
}
func TestPageMutationRetainsExplicitApproval(t *testing.T) {
	s, _, c, _, call, _ := fixture(t)
	ref := browserpage.Ref{ID: strings.Repeat("b", 32), DocumentID: strings.Repeat("c", 32), Version: 1}
	reader := &pageScopeFixture{bound: browserpage.Binding{Ref: ref, URL: "https://example.invalid/test-service/page", Epoch: 0, RuntimeVersion: 1}}
	s.SetBrowserScopes(reader)
	raw, _ := json.Marshal(browserpage.Args{SessionID: strings.Repeat("a", 32), Page: &ref, Selector: "#submit"})
	call.ToolName = browserpage.Click
	call.ArgsJson = string(raw)
	contract := pageTestContract(call.ToolName)
	p, e := s.Prepare(context.Background(), call, contract)
	if e != nil || !p.RequiresApproval || p.Binding.Effect != EffectUnknown {
		t.Fatal("page mutation protection changed", e)
	}
	if _, e = s.Authorize(context.Background(), call, contract, p.Binding.ActionID, "", ""); e == nil {
		t.Fatal("unapproved mutation authorized")
	}
	c.approved = ApprovalSnapshot{ApprovalID: "page-approval", Status: "approved", Binding: p.Binding, ExpiresAt: s.now().Add(time.Hour), ResolvedAt: s.now()}
	g, e := s.Authorize(context.Background(), call, contract, p.Binding.ActionID, c.approved.ApprovalID, "")
	if e != nil {
		t.Fatal(e)
	}
	reader.bound.Version++
	if _, e = s.Verify(context.Background(), call, contract, g.ID); e == nil {
		t.Fatal("approval carried to another observation")
	}
}
