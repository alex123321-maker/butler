package maintenance

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func publicDockerInspect() map[string]any {
	return map[string]any{"Id": strings.Repeat("1", 64), "Name": "/public-module", "Image": "sha256:" + strings.Repeat("2", 64), "State": map[string]any{"Running": true, "Paused": false, "Restarting": false, "Dead": false, "Status": "running"}, "Config": map[string]any{"Image": "mutable:tag", "Env": []string{"PRIVATE=synthetic-never-published"}, "Hostname": "public-module", "Labels": map[string]string{"claimed-artifact": "untrusted-label"}}, "HostConfig": map[string]any{"ReadonlyRootfs": true, "Privileged": false, "NetworkMode": "private"}, "Mounts": []any{}, "NetworkSettings": map[string]any{"Networks": map[string]any{"private": map[string]any{"NetworkID": strings.Repeat("3", 64)}}}}
}

func TestDockerInspectorMeasuresActualIdentity(t *testing.T) {
	now := time.Now()
	body, _ := json.Marshal(publicDockerInspect())
	measured, err := inspectDockerIdentity(body, "public-module", now)
	if err != nil || measured.ArtifactSHA256 != strings.Repeat("2", 64) || measured.State != "running" || measured.ContainerID != strings.Repeat("1", 64) || !validDigest(measured.RuntimeSHA256) {
		t.Fatal("physical identity absent", err)
	}
	public, _ := json.Marshal(measured)
	if strings.Contains(string(public), "synthetic-never-published") || strings.Contains(string(public), "untrusted-label") {
		t.Fatal("raw Docker configuration escaped")
	}
	candidate := publicDockerInspect()
	candidate["Image"] = "sha256:" + strings.Repeat("4", 64)
	candidate["Config"].(map[string]any)["Image"] = "other:tag"
	body, _ = json.Marshal(candidate)
	next, err := inspectDockerIdentity(body, "public-module", now)
	if err != nil || next.ArtifactSHA256 == measured.ArtifactSHA256 || next.RuntimeSHA256 != measured.RuntimeSHA256 {
		t.Fatal("image content conflated with runtime/tag", err)
	}
	for name, mutate := range map[string]func(map[string]any){
		"privilege":   func(root map[string]any) { root["HostConfig"].(map[string]any)["Privileged"] = true },
		"environment": func(root map[string]any) { root["Config"].(map[string]any)["Env"] = []string{"PRIVATE=different"} },
		"mount": func(root map[string]any) {
			root["Mounts"] = []any{map[string]string{"Type": "bind", "Source": "/host-private", "Destination": "/unexpected"}}
		},
		"network": func(root map[string]any) {
			root["NetworkSettings"].(map[string]any)["Networks"].(map[string]any)["extra"] = map[string]any{"NetworkID": strings.Repeat("5", 64)}
		},
	} {
		t.Run(name, func(t *testing.T) {
			root := publicDockerInspect()
			mutate(root)
			body, _ := json.Marshal(root)
			changed, err := inspectDockerIdentity(body, "public-module", now)
			if err != nil || changed.RuntimeSHA256 == measured.RuntimeSHA256 {
				t.Fatal("actual configuration change hidden", err)
			}
		})
	}
	for name, mutate := range map[string]func(map[string]any){
		"wrong target":         func(root map[string]any) { root["Name"] = "/different" },
		"tag instead of image": func(root map[string]any) { root["Image"] = "latest" },
		"paused":               func(root map[string]any) { root["State"].(map[string]any)["Paused"] = true },
		"restarting":           func(root map[string]any) { root["State"].(map[string]any)["Restarting"] = true },
		"ambiguous state":      func(root map[string]any) { root["State"].(map[string]any)["running"] = false },
		"missing state proof":  func(root map[string]any) { delete(root["State"].(map[string]any), "Running") },
		"null config":          func(root map[string]any) { root["Config"] = nil },
		"null mounts":          func(root map[string]any) { root["Mounts"] = nil },
		"null networks":        func(root map[string]any) { root["NetworkSettings"] = nil },
		"null state flag":      func(root map[string]any) { root["State"].(map[string]any)["Paused"] = nil },
		"network alias":        func(root map[string]any) { root["NetworkSettings"].(map[string]any)["networks"] = map[string]any{} },
	} {
		t.Run(name, func(t *testing.T) {
			root := publicDockerInspect()
			mutate(root)
			body, _ := json.Marshal(root)
			if _, err := inspectDockerIdentity(body, "public-module", now); err == nil {
				t.Fatal("unknown physical state accepted")
			}
		})
	}
	body, _ = json.Marshal(publicDockerInspect())
	body = append([]byte(`{"Image":"mutable-tag",`), body[1:]...)
	if _, err := inspectDockerIdentity(body, "public-module", now); err == nil {
		t.Fatal("duplicate image interpretation accepted")
	}
}
