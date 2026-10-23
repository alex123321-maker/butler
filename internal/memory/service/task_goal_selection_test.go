package service

import "testing"

func TestExperimentalTaskGoalOnlyBreaksCurrentQueryTies(t *testing.T) {
	entries := []map[string]any{
		{"key": "a_noise", "summary": "Цвет велосипеда синий", "scope_type": "user", "scope_id": "owner"},
		{"key": "z_report", "summary": "Отчёт оформлять таблицей", "scope_type": "user", "scope_id": "owner"},
	}
	if got := relevantProfilesForTask(entries, "Продолжай", "Подготовить отчёт", 1); got[0]["key"] != "z_report" {
		t.Fatal("goal did not break tie")
	}
	if got := relevantProfilesForTask(entries, "Какого цвета велосипед?", "Подготовить отчёт", 1); got[0]["key"] != "a_noise" {
		t.Fatal("goal displaced explicit current query")
	}
	if got := relevantProfilesForTask(entries, "Продолжай", "", 20); len(got) != 2 || got[0]["key"] != "a_noise" || got[1]["key"] != "z_report" {
		t.Fatal("empty hint changed baseline")
	}
}
