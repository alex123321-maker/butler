package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSSOPrivateConfigInitializationAndValidation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private.json")
	if run([]string{"-init", "-config", path}) != nil {
		t.Fatal("cannot initialize fixture")
	}
	before, err := os.ReadFile(path)
	if err != nil || run([]string{"-init", "-config", path}) == nil {
		t.Fatal("initialization overwrote a private file")
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("private file changed on refused initialization")
	}
	if _, err = load(path); err != nil {
		t.Fatal("initialized fixture is invalid")
	}
	valid, _ := json.Marshal(config{"user-canary", "password-canary", "reference-canary"})
	for name, data := range map[string]string{
		"unknown":   `{"username":"a","password":"b","private_fact":"c","extra":true}`,
		"missing":   `{"username":"a","password":"b"}`,
		"trailing":  string(valid) + `{}`,
		"oversized": string(valid) + strings.Repeat(" ", 16385),
	} {
		t.Run(name, func(t *testing.T) {
			if os.WriteFile(path, []byte(data), 0600) != nil {
				t.Fatal("cannot write test fixture")
			}
			if _, err := load(path); err == nil {
				t.Fatal("invalid private fixture accepted")
			}
		})
	}
}

func TestSSOOriginAndListenerRestrictions(t *testing.T) {
	for _, origin := range []string{"http://public.test", "https://user:pass@example.test", "https://example.test/", "https://example.test?", "https://example.test#callback", "file:///tmp/test"} {
		if validOrigin(origin) {
			t.Fatal("invalid fixture origin accepted")
		}
	}
	for _, origin := range []string{"https://rp.test", "http://127.0.0.1:18098", "http://localhost:18099"} {
		if !validOrigin(origin) {
			t.Fatal("valid fixture origin rejected")
		}
	}
	if loopback("0.0.0.0:18098") || loopback("192.168.0.5:18098") || loopback("localhost:18098") || !loopback("127.0.0.1:18098") {
		t.Fatal("listener must be literal loopback")
	}
}
