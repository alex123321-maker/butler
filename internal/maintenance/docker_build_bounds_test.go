package maintenance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDockerBuildSwapUsesActualAncestorWithoutTreatingMaxAsZero(t *testing.T) {
	root := t.TempDir()
	id := strings.Repeat("1", 64)
	group := "/private-build/" + id
	parent := filepath.Join(root, "private-build")
	child := filepath.Join(parent, id)
	if err := os.MkdirAll(child, 0700); err != nil {
		t.Fatal(err)
	}
	write := func(directory, value string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(directory, "memory.swap.max"), []byte(value+"\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(root, "max")
	write(parent, "0")
	write(child, "max")
	if effectiveZeroSwap(root, group) != nil {
		t.Fatal("actual zero ancestor was ignored")
	}
	write(parent, "8192")
	if effectiveZeroSwap(root, group) == nil {
		t.Fatal("finite nonzero or max limits were treated as zero")
	}
	write(child, "0")
	if effectiveZeroSwap(root, group) != nil {
		t.Fatal("actual zero container limit was ignored")
	}
	write(child, "not-known")
	if effectiveZeroSwap(root, group) == nil {
		t.Fatal("unavailable swap limit qualified")
	}
	for _, invalid := range []string{"private-build/" + id, "/private-build/../" + id, "/private-build\\" + id, "/private-build/" + id + "\x00"} {
		if effectiveZeroSwap(root, invalid) == nil {
			t.Fatal("noncanonical cgroup path accepted")
		}
	}
}
