//go:build ignore

package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestLaba115UnboundNativeProfileDeniesBeforeAdmission(t *testing.T) {
	if laba115PinsBound("", laba115NativePins()) {
		t.Fatal("empty source commitment accepted")
	}
	valid := map[string]string{}
	for path := range laba115NativePins() {
		valid[path] = strings.Repeat("a", 64)
	}
	if !laba115PinsBound(strings.Repeat("a", 40), valid) {
		t.Fatal("complete synthetically bound identity rejected")
	}
	for path := range valid {
		saved := valid[path]
		valid[path] = ""
		if laba115PinsBound(strings.Repeat("a", 40), valid) {
			t.Fatal("missing role commitment accepted")
		}
		valid[path] = saved
	}
	if laba115NativeBound() {
		return
	} // A later binding carrier tests pins without executing an admitted cohort.
	for _, native := range []bool{false, true} {
		err := laba115Execute(context.Background(), native)
		if err == nil || err.Error() != "server native profile is not frozen and bound" {
			t.Fatal("unbound profile reached execution")
		}
	}
}

func TestLaba115PrivateDatabaseIdentityRejectsForeignStorageAndPublicPorts(t *testing.T) {
	raw := []byte(`{"Image":"` + laba115PGImage + `","State":{"Running":true},"HostConfig":{"NetworkMode":"` + laba115NativeNetwork + `","ReadonlyRootfs":true,"Memory":536870912,"MemorySwap":536870912,"NanoCpus":1000000000,"PidsLimit":128},"Mounts":[{"Type":"bind","Source":"` + laba115HostRoot + `/postgres","Destination":"/var/lib/postgresql/data","RW":true}],"NetworkSettings":{"Ports":{"55451/tcp":[{"HostIp":"127.0.0.1","HostPort":"55455"}]}}}`)
	var good laba115Container
	if json.Unmarshal(raw, &good) != nil || !laba115PGConfig(good) {
		t.Fatal("fixed isolated database rejected")
	}
	for name, mutate := range map[string]func(*laba115Container){
		"other_image":     func(c *laba115Container) { c.Image = laba115Image },
		"stopped":         func(c *laba115Container) { c.State.Running = false },
		"privileged":      func(c *laba115Container) { c.HostConfig.Privileged = true },
		"writable_root":   func(c *laba115Container) { c.HostConfig.ReadonlyRootfs = false },
		"host_network":    func(c *laba115Container) { c.HostConfig.NetworkMode = "host" },
		"memory":          func(c *laba115Container) { c.HostConfig.Memory++ },
		"swap":            func(c *laba115Container) { c.HostConfig.MemorySwap++ },
		"cpu":             func(c *laba115Container) { c.HostConfig.NanoCpus++ },
		"pids":            func(c *laba115Container) { c.HostConfig.PidsLimit++ },
		"foreign_storage": func(c *laba115Container) { c.Mounts[0].Source = "/srv/storage/butler/data" },
		"public_port":     func(c *laba115Container) { c.NetworkSettings.Ports["55451/tcp"][0].HostIP = "0.0.0.0" },
		"wrong_port":      func(c *laba115Container) { c.NetworkSettings.Ports["55451/tcp"][0].HostPort = "5432" },
	} {
		t.Run(name, func(t *testing.T) {
			var c laba115Container
			if json.Unmarshal(raw, &c) != nil {
				t.Fatal("fixture decode")
			}
			mutate(&c)
			if laba115PGConfig(c) {
				t.Fatal("foreign or unbounded database accepted")
			}
		})
	}
}

func TestLaba115ChildEnvironmentKeepsHostReferencesAndSecretsOut(t *testing.T) {
	for _, entry := range laba115ChildEnvironment() {
		if strings.HasPrefix(entry, "BUTLER_SETTINGS_ENCRYPTION_KEY=") || strings.Contains(entry, "/srv/") || strings.Contains(entry, "/mnt/host/") || strings.Contains(entry, "AUTH_VALUE_FILE") {
			t.Fatal("pre-auth child environment contains host or credential material")
		}
	}
	if !strings.Contains(strings.Join(laba115ChildEnvironment(), "\n"), "BUTLER_EXPERIENCE_NATIVE_PLAN=/plan/preseal.json") {
		t.Fatal("fixed child preseal missing")
	}
}
