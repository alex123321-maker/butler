package credentials

import (
	"context"
	"strings"
	"testing"
)

func TestVaultCipherSeparatesOwnerRecordAndField(t *testing.T) {
	v := &Vault{key: strings.Repeat("k", 32)}
	cipher, err := v.seal("owner", "record", "password", "synthetic-password")
	if err != nil || strings.Contains(cipher, "synthetic-password") {
		t.Fatal("plaintext storage")
	}
	if value, e := v.open("owner", "record", "password", cipher); e != nil || value != "synthetic-password" {
		t.Fatal("round trip failed")
	}
	for _, scope := range [][3]string{{"other", "record", "password"}, {"owner", "other", "password"}, {"owner", "record", "username"}} {
		if _, e := v.open(scope[0], scope[1], scope[2], cipher); e == nil {
			t.Fatal("cipher moved across scope")
		}
	}
	if _, e := v.ResolveSecretRef(context.Background(), "env://LEGACY_VALUE"); e == nil {
		t.Fatal("legacy resolver reachable")
	}
}
func TestVaultLoginIndexIsOwnerScoped(t *testing.T) {
	v := &Vault{key: strings.Repeat("k", 32)}
	if v.index("one", " User@Example.COM ") != v.index("one", "user@example.com") {
		t.Fatal("normalization")
	}
	if v.index("one", "user@example.com") == v.index("two", "user@example.com") {
		t.Fatal("cross owner index")
	}
	if v.pinHash("one", "salt", "123456") == v.pinHash("two", "salt", "123456") {
		t.Fatal("cross owner PIN")
	}
}
