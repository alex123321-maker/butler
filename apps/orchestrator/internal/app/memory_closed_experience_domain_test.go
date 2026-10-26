package app

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestClosedExperienceDomainBindsInstantAndEveryIdentity(t *testing.T) {
	deadline, err := time.Parse(time.RFC3339, "2026-10-19T00:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	m := closedExperienceManifest{Owner: "owner", Day: "2026-10-18", Preseal: strings.Repeat("a", 64), Previous: strings.Repeat("b", 64)}
	d := closedExperienceDomain{Window: experienceNativeWindow, Hash: m.Preseal, Schema: experienceNativeSchema, Owner: m.Owner, History: m.Previous, Day: m.Day}
	// A real PostgreSQL JSON export uses the server's +07 offset. It denotes
	// the same closed instant as the UTC manifest, without reopening its day.
	if err := json.Unmarshal([]byte(`{"deadline":"2026-10-19T07:00:00+07:00"}`), &d); err != nil {
		t.Fatal(err)
	}
	if !d.matches(m, deadline) {
		t.Fatal("same immutable instant rejected due to location encoding")
	}
	for _, mutate := range []func(*closedExperienceDomain){
		func(v *closedExperienceDomain) { v.Window += "x" },
		func(v *closedExperienceDomain) { v.Hash = strings.Repeat("c", 64) },
		func(v *closedExperienceDomain) { v.Schema += "x" },
		func(v *closedExperienceDomain) { v.Owner += "x" },
		func(v *closedExperienceDomain) { v.History = strings.Repeat("c", 64) },
		func(v *closedExperienceDomain) { v.Day = "2026-10-19" },
		func(v *closedExperienceDomain) { v.Deadline = deadline.Add(time.Nanosecond) },
		func(v *closedExperienceDomain) { v.Deadline = time.Time{} },
	} {
		changed := d
		mutate(&changed)
		if changed.matches(m, deadline) {
			t.Fatal("changed closed domain accepted")
		}
	}
}
