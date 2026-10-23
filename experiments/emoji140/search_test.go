package main

import "testing"

func TestRussianLookupIsExactAndDoesNotInventNoMatchEvidence(t *testing.T) {
	rows := []Annotation{{ID: "a"}, {ID: "b"}}
	rows[0].Observed.Subjects = []string{"Клубника"}
	rows[1].Observed.Subjects = []string{"Бант"}
	got := rank("КЛУБНИКА", rows, 5)
	if len(got) != 1 || got[0].ID != "a" {
		t.Fatal("Russian normalization failed")
	}
	if got := rank("помидор", rows, 5); len(got) != 0 {
		t.Fatal("unmatched tokens returned a candidate")
	}
	if got := rank("клубникой", rows, 5); len(got) != 0 {
		t.Fatal("undocumented stemming")
	}
}
