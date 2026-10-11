package runtimeclient

import (
	"strings"
	"testing"
)

func TestTargetAuthenticationCannotReplaceConfiguredSecret(t *testing.T) {
	r := New(nil)
	defer r.Close()
	if err := r.SetTargetAuthentication("127.0.0.1:19090", strings.Repeat("original-fixture-", 3)); err != nil {
		t.Fatal(err)
	}
	if err := r.SetTargetAuthentication("127.0.0.1:19090", strings.Repeat("replacement-fixture-", 3)); err == nil {
		t.Fatal("another profile replaced private runtime authentication")
	}
}
