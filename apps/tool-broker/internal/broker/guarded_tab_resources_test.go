package broker

import (
	"context"
	"testing"

	"github.com/butler/butler/internal/actionauth"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/singletabcontrol"
)

func TestGuardedTabAdmissionKeepsLocalRequestSeparate(t *testing.T) {
	a := GuardedTabAdmission{}
	call := &toolbrokerv1.ToolCall{ToolName: singletabcontrol.BindTool}
	h, _ := actionauth.ContractHash(singletabcontrol.RequestContract())
	g := actionauth.Grant{Binding: actionauth.ActionBinding{ToolName: singletabcontrol.BindTool, ContractHash: h}}
	if _, e := a.PrepareContext(context.Background(), call, singletabcontrol.RequestContract(), g); e != nil {
		t.Fatal(e)
	}
	changed := singletabcontrol.RequestContract()
	changed.RuntimeTarget = "127.0.0.1:19090"
	if _, e := a.PrepareContext(context.Background(), call, changed, g); e == nil {
		t.Fatal("local request used a browser route")
	}
	call.ToolName = "single_tab.status"
	if _, e := a.PrepareContext(context.Background(), call, changed, actionauth.Grant{}); e == nil {
		t.Fatal("missing resource store admitted action")
	}
}
