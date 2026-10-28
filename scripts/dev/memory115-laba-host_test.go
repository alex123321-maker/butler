//go:build ignore

package main

import "testing"

func TestLaba115ResourceConfigurationRejectsWeakerBoundaries(t *testing.T) {
	var good laba115Container
	good.Image = laba115Image
	good.Config.User = "1000:1000"
	good.HostConfig.NanoCpus = 1000000000
	good.HostConfig.Memory = 2147483648
	good.HostConfig.MemorySwap = 2147483648
	good.HostConfig.PidsLimit = 256
	good.HostConfig.ReadonlyRootfs = true
	good.HostConfig.NetworkMode = "none"
	good.HostConfig.CapDrop = []string{"ALL"}
	good.HostConfig.SecurityOpt = []string{"no-new-privileges"}
	if !laba115ResourceConfig(good) {
		t.Fatal("fixed profile rejected")
	}
	for name, mutate := range map[string]func(*laba115Container){
		"other_image":    func(c *laba115Container) { c.Image = "other" },
		"root_uid":       func(c *laba115Container) { c.Config.User = "0" },
		"cpu":            func(c *laba115Container) { c.HostConfig.NanoCpus++ },
		"memory":         func(c *laba115Container) { c.HostConfig.Memory++ },
		"swap":           func(c *laba115Container) { c.HostConfig.MemorySwap++ },
		"pids":           func(c *laba115Container) { c.HostConfig.PidsLimit++ },
		"writable_root":  func(c *laba115Container) { c.HostConfig.ReadonlyRootfs = false },
		"privileged":     func(c *laba115Container) { c.HostConfig.Privileged = true },
		"host_network":   func(c *laba115Container) { c.HostConfig.NetworkMode = "host" },
		"host_pid":       func(c *laba115Container) { c.HostConfig.PidMode = "host" },
		"cap_add":        func(c *laba115Container) { c.HostConfig.CapAdd = []string{"SYS_ADMIN"} },
		"cap_drop":       func(c *laba115Container) { c.HostConfig.CapDrop = nil },
		"new_privileges": func(c *laba115Container) { c.HostConfig.SecurityOpt = nil },
	} {
		t.Run(name, func(t *testing.T) {
			changed := good
			mutate(&changed)
			if laba115ResourceConfig(changed) {
				t.Fatal("changed resource/security boundary accepted")
			}
		})
	}
}
