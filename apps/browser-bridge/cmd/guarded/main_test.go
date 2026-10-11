package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGuardedHostPrivateTokenFile(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "token")
	if e := os.WriteFile(file, []byte(strings.Repeat("fixture", 8)), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e := readPrivateToken(file); e != nil {
		t.Fatal(e)
	}
	if e := os.Chmod(file, 0644); e != nil {
		t.Fatal(e)
	}
	if _, e := readPrivateToken(file); e == nil {
		t.Fatal("public token file accepted")
	}
	os.Chmod(file, 0600)
	link := filepath.Join(root, "link")
	if os.Symlink(file, link) == nil {
		if _, e := readPrivateToken(link); e == nil {
			t.Fatal("symlink token accepted")
		}
	}
	os.WriteFile(file, []byte(strings.Repeat("x", 4097)), 0600)
	if _, e := readPrivateToken(file); e == nil {
		t.Fatal("unbounded token accepted")
	}
}

func TestGuardedHostListenAddressPrivateLoopbackOnly(t *testing.T) {
	for _, addr := range []string{"127.0.0.1:19090", "[::1]:19090"} {
		if guardedListenAddress(addr) != nil {
			t.Fatal("private loopback address refused", addr)
		}
	}
	for _, addr := range []string{":19090", "0.0.0.0:19090", "[::]:19090", "192.168.0.2:19090", "localhost:19090", "127.0.0.1:0", "127.0.0.1:65536"} {
		if guardedListenAddress(addr) == nil {
			t.Fatal("unprotected listener admitted", addr)
		}
	}
}
