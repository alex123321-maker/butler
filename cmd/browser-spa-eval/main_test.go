package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPrivateInitializationAndTopology(t *testing.T) {
	file := filepath.Join(t.TempDir(), "fixture.json")
	if err := run([]string{"-init", "-config", file, "-origin", "https://fixture.example"}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if run([]string{"-init", "-config", file, "-origin", "https://fixture.example"}) == nil {
		t.Fatal("private oracle replaced")
	}
	after, _ := os.ReadFile(file)
	if string(before) != string(after) {
		t.Fatal("oracle changed")
	}
	if run([]string{"-config", file, "-journal", filepath.Join(t.TempDir(), "events"), "-listen", "0.0.0.0:18198"}) == nil {
		t.Fatal("public listener allowed")
	}
	if run([]string{"-init", "-config", filepath.Join(t.TempDir(), "bad"), "-origin", "http://fixture.example"}) == nil {
		t.Fatal("insecure origin allowed")
	}
}
