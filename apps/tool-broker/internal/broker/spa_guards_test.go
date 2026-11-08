package broker

import (
	"context"
	"testing"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/browserspa"
)

func TestSPAAdmissionCannotBorrowLegacyBrowserBypass(t *testing.T) {
	a := ResourceAdmissions{ServerBrowser: true}
	if err := a.ClaimActionTx(context.Background(), nil, actionauth.Grant{Binding: actionauth.ActionBinding{ToolName: browserspa.Tool}}, ""); err == nil {
		t.Fatal("missing SPA resource store bypassed")
	}
}
