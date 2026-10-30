package main

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

//go:embed popup_probe.mjs
var popupProbe string

func TestSSOPopupChromium(t *testing.T) {
	if os.Getenv("BUTLER_SSO_BROWSER_E2E") != "1" {
		t.Skip("requires isolated Chromium fixture profile")
	}
	for _, mode := range []string{"normal", "late", "twins", "closed_before_login", "forged_message"} {
		t.Run(mode, func(t *testing.T) {
			began := time.Now()
			defer func() {
				t.Logf("scope=standalone_fixture operation=sso/%s wall_ms=%.3f model_calls=0", mode, float64(time.Since(began).Microseconds())/1000)
			}()
			s := site(t)
			input, _ := json.Marshal(map[string]string{"rp": s.rp.URL, "idp": s.idp.URL, "username": s.f.cfg.Username, "password": s.f.cfg.Password, "private_fact": s.f.cfg.PrivateFact, "mode": mode})
			ctx, cancel := context.WithTimeout(t.Context(), 25*time.Second)
			defer cancel()
			command := exec.CommandContext(ctx, "node", "--input-type=module", "--eval", popupProbe)
			command.Dir = os.Getenv("BUTLER_SSO_E2E_MODULE_ROOT")
			if command.Dir == "" {
				command.Dir, _ = filepath.Abs(filepath.Join("..", "..", "apps", "tool-browser", "scripts"))
			}
			command.Stdin = bytes.NewReader(append(input, '\n'))
			command.Stderr = io.Discard
			output, err := command.Output()
			if err != nil || len(output) > 4096 {
				t.Fatal("private Chromium probe unavailable")
			}
			for _, private := range []string{s.f.cfg.Username, s.f.cfg.Password, s.f.cfg.PrivateFact} {
				if bytes.Contains(output, []byte(private)) {
					t.Fatal("private probe material exported")
				}
			}
			var result struct {
				OK             bool   `json:"ok"`
				Private        bool   `json:"private_material_exported"`
				ModelCalls     int    `json:"model_calls"`
				FailureStage   string `json:"failure_stage"`
				LoginStatus    int    `json:"login_status"`
				CallbackStatus int    `json:"callback_status"`
				AccountStatus  int    `json:"account_status"`
			}
			if json.Unmarshal(output, &result) != nil || !result.OK || result.Private || result.ModelCalls != 0 {
				switch result.FailureStage {
				case "setup", "fill", "submit_return", "private_read", "anonymous_read":
					t.Logf("failure_stage=%s login_status=%d callback_status=%d account_status=%d", result.FailureStage, result.LoginStatus, result.CallbackStatus, result.AccountStatus)
				}
				t.Fatal("SSO fixture Chromium criteria failed for", mode)
			}
			if mode == "normal" || mode == "late" {
				assertCount(t, s.journal.String(), "idp_login", 1)
				assertCount(t, s.journal.String(), "callback_accepted", 1)
				assertCount(t, s.journal.String(), "account_read", 1)
			} else {
				assertCount(t, s.journal.String(), "callback_accepted", 0)
				assertCount(t, s.journal.String(), "account_read", 0)
			}
		})
	}
}
