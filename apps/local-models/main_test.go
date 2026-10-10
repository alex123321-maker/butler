package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTokenFileRequiresPrivatePermissionsWithoutEcho(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token")
	token := "sensitive-value-never-echoed-to-errors"
	if err := os.WriteFile(path, []byte(token), 0600); err != nil {
		t.Fatal(err)
	}
	if value, err := readToken(path); err != nil || value != token {
		t.Fatal("valid private token rejected")
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := readToken(path); err == nil || strings.Contains(err.Error(), token) {
		t.Fatal("public token accepted or exposed")
	}
}
