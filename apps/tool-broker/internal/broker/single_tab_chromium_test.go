package broker

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
)

func TestSingleTabResourceActualChromiumNativeHostIntegration(t *testing.T) {
	if os.Getenv("BUTLER_SINGLE_TAB_E2E") != "1" {
		t.Skip("requires isolated Chromium/guarded host subprocess")
	}
	for _, scenario := range []string{"valid", "manual_input", "manual_new_observation", "dom_mutation", "navigation", "foreign_origin", "foreign_host", "lost_ack_restart", "two_executors", "installed_native"} {
		t.Run(scenario, func(t *testing.T) {
			var control string
			f := singleTabBroker(t, nil, func(target, authority, file string) (string, string, string) {
				cmd := exec.Command("node", os.Getenv("BUTLER_SINGLE_TAB_FIXTURE_SCRIPT"), os.Getenv("BUTLER_SINGLE_TAB_HOST_BINARY"), os.Getenv("BUTLER_SINGLE_TAB_EXTENSION"), target, authority, file)
				if scenario == "installed_native" {
					cmd.Env = append(os.Environ(), "BUTLER_SINGLE_TAB_INSTALLED=1")
				}
				stdin, e := cmd.StdinPipe()
				if e != nil {
					t.Fatal(e)
				}
				stdout, e := cmd.StdoutPipe()
				if e != nil {
					t.Fatal(e)
				}
				cmd.Stderr = io.Discard
				if e = cmd.Start(); e != nil {
					t.Fatal(e)
				}
				t.Cleanup(func() {
					_ = stdin.Close()
					done := make(chan error, 1)
					go func() { done <- cmd.Wait() }()
					select {
					case <-done:
					case <-time.After(5 * time.Second):
						_ = cmd.Process.Kill()
						<-done
					}
				})
				ready := make(chan string, 1)
				go func() {
					s := bufio.NewScanner(stdout)
					if s.Scan() {
						ready <- s.Text()
					} else {
						ready <- ""
					}
				}()
				var meta struct {
					Tab      string `json:"tab_ref"`
					Control  string `json:"control"`
					Origin   string `json:"origin"`
					Metadata string `json:"metadata"`
				}
				select {
				case line := <-ready:
					if json.Unmarshal([]byte(line), &meta) != nil || meta.Tab == "" {
						t.Fatal("actual Chrome fixture unavailable")
					}
				case <-time.After(25 * time.Second):
					t.Fatal("actual Chrome fixture timed out")
				}
				control = meta.Control
				deadline := time.Now().Add(5 * time.Second)
				for {
					conn, e := net.DialTimeout("tcp", target, 50*time.Millisecond)
					if e == nil {
						conn.Close()
						break
					}
					if time.Now().After(deadline) {
						t.Fatal("native host gRPC unavailable")
					}
					time.Sleep(10 * time.Millisecond)
				}
				return meta.Tab, meta.Origin, meta.Metadata
			})
			op := func(path string) map[string]any {
				t.Helper()
				client := http.Client{Timeout: 15 * time.Second}
				r, e := client.Get(control + path)
				if e != nil {
					t.Fatal("fixture control failed")
				}
				defer r.Body.Close()
				if r.StatusCode != 200 {
					var phase struct {
						Failure string `json:"failure"`
					}
					json.NewDecoder(r.Body).Decode(&phase)
					t.Fatal("fixture operation failed", path, phase.Failure)
				}
				var value map[string]any
				if json.NewDecoder(r.Body).Decode(&value) != nil {
					t.Fatal("fixture metadata malformed")
				}
				return value
			}
			invoke := func(id, tool, args string) *toolbrokerv1.ToolResult {
				t.Helper()
				req := singleTabRequest(t, f, id, tool, args)
				out, e := f.server.ExecuteToolCall(context.Background(), req)
				if e != nil {
					t.Fatal(e)
				}
				return out.GetResult()
			}
			observed := invoke("observe", "single_tab.status", `{"session_id":"approved-user-tab"}`)
			if observed.GetStatus() != "completed" {
				t.Fatal("actual observation denied", observed.GetError(), op("/state"))
			}
			var snapshot struct {
				ID      string `json:"observation_id"`
				Version int64  `json:"observation_version"`
			}
			if json.Unmarshal([]byte(observed.ResultJson), &snapshot) != nil || snapshot.ID == "" || snapshot.Version < 1 {
				t.Fatal("actual observed DATA ref missing")
			}
			if op("/legacy")["denied"] != true {
				t.Fatal("legacy path bypassed guarded tab")
			}
			switch scenario {
			case "manual_input", "manual_new_observation":
				op("/manual")
			case "dom_mutation":
				op("/mutate")
			case "navigation":
				op("/reload")
			case "foreign_host":
				op("/foreign-host")
			case "foreign_origin":
				op("/foreign-origin")
			case "lost_ack_restart":
				op("/lose-ack")
			}
			if scenario == "manual_new_observation" {
				fresh := invoke("observe-after-manual", "single_tab.status", `{"session_id":"approved-user-tab"}`)
				var current struct {
					ID      string `json:"observation_id"`
					Version int64  `json:"observation_version"`
				}
				if fresh.GetStatus() != "completed" || json.Unmarshal([]byte(fresh.GetResultJson()), &current) != nil || current.ID == snapshot.ID || current.Version <= snapshot.Version {
					t.Fatal("manual intervention did not advance actual observation")
				}
				t.Log("actual manual observation version advanced: true")
			}
			args := `{"session_id":"approved-user-tab","observation_id":"` + snapshot.ID + `","selector":"#target"}`
			var result *toolbrokerv1.ToolResult
			if scenario == "foreign_origin" {
				// A fresh status on a navigated foreign origin cannot turn the old
				// selected tab into permission for that site's observation/action.
				result = invoke("observe-foreign", "single_tab.status", `{"session_id":"approved-user-tab"}`)
			} else if scenario == "two_executors" {
				first := singleTabRequest(t, f, "click", "single_tab.click", args)
				second := singleTabRequest(t, f, "second-click", "single_tab.click", args)
				op("/hold-next")
				done := make(chan *toolbrokerv1.ToolResult, 1)
				go func() { v, _ := f.server.ExecuteToolCall(context.Background(), first); done <- v.GetResult() }()
				wait := time.Now().Add(3 * time.Second)
				for {
					if op("/state")["guardedHeld"] == true {
						break
					}
					if time.Now().After(wait) {
						t.Fatal("actual native request did not reach delay barrier")
					}
					time.Sleep(10 * time.Millisecond)
				}
				denied, e := f.server.ExecuteToolCall(context.Background(), second)
				if e != nil || denied.GetResult().GetStatus() == "completed" {
					t.Fatal("second tab executor admitted", e)
				}
				op("/release")
				select {
				case result = <-done:
				case <-time.After(5 * time.Second):
					t.Fatal("first executor did not finish")
				}
			} else {
				result = invoke("click", "single_tab.click", args)
			}
			if result.GetStatus() != "effect_unknown" {
				t.Fatal("uncertain/stale click presented completed", scenario, result.GetStatus())
			}
			// A known local ACK is not recipient verification. Successful actual
			// clicks remain unknown in the Broker journal, with dispatch_completed.
			if scenario == "valid" || scenario == "two_executors" || scenario == "installed_native" {
				var outcome string
				f.pool.QueryRow(context.Background(), `SELECT outcome FROM action_effect_dispatches d JOIN action_intents i ON i.effect_id=d.action_id WHERE i.tool_call_id='click'`).Scan(&outcome)
				if outcome != "dispatch_completed" {
					t.Fatal("actual locally completed click receipt absent", outcome)
				}
			}
			if scenario == "lost_ack_restart" {
				op("/restart-browser")
				state := op("/state")
				if state["phase"] != "awaiting_ack" {
					t.Fatal("restart cleared uncertain extension lane")
				}
				t.Logf("cold Chromium/profile restart: frozen tab ID retained=%v", state["restartTabMatches"])
				if op("/successor-denied")["denied"] != true {
					t.Fatal("restarted extension allowed uncertain successor")
				}
				// Also use a real new Broker authorization/reservation/ticket rather
				// than relying only on the extension-driver proof above. The frozen
				// old binding cannot observe a newly numbered tab after restart.
				if invoke("observe-after-restart", "single_tab.status", `{"session_id":"approved-user-tab"}`).GetStatus() != "effect_unknown" {
					t.Fatal("real new ticket bypassed persistent quarantine")
				}
			}
			end := time.Now().Add(time.Second)
			var effects float64
			for {
				effects, _ = op("/effects")["effects"].(float64)
				if effects > 0 || time.Now().After(end) {
					break
				}
				time.Sleep(10 * time.Millisecond)
			}
			expected := float64(0)
			if scenario == "valid" || scenario == "lost_ack_restart" || scenario == "two_executors" || scenario == "installed_native" {
				expected = 1
			}
			if effects != expected {
				t.Fatal("unexpected actual recipient call count", scenario, effects)
			}
			if scenario == "installed_native" {
				if op("/state")["manifestLaunched"] != true {
					t.Fatal("Chrome did not launch installed guarded native host")
				}
				t.Log("actual Linux installed native manifest + Chrome connectNative + owner selection + private Broker: PASS")
			}
			var reservations, dispatches int
			admissions := 2
			if scenario == "lost_ack_restart" || scenario == "manual_new_observation" {
				admissions = 3
			}
			if e := f.pool.QueryRow(context.Background(), `SELECT (SELECT count(*) FROM execution_budget_reservations),(SELECT count(*) FROM execution_resource_dispatches WHERE single_tab_binding IS NOT NULL)`).Scan(&reservations, &dispatches); e != nil || reservations != admissions || dispatches != admissions {
				t.Fatal("actual finite admissions not persisted", e, reservations, dispatches)
			}
			if strings.Contains(result.ResultJson, "dispatch_id") || strings.Contains(result.ResultJson, "resource_fence") {
				t.Fatal("private native proof leaked")
			}
			// No inference, human account or real channel is involved. The installed
			// scenario uses Chrome's actual native-host launch; fault cases use frames.
			t.Logf("actual Chrome + native host + Broker/PG: scenario=%s recipient_calls=%.0f reservations=%d", scenario, effects, reservations)
		})
	}
}
