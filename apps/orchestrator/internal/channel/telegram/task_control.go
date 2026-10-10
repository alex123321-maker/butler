package telegram

import (
	"strings"

	"github.com/butler/butler/apps/orchestrator/internal/task"
)

func isOwnerControlText(text string) bool {
	kind, _ := task.ClassifyOwnerText(text)
	if kind != "" {
		return true
	}
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return false
	}
	switch strings.ToLower(fields[0]) {
	case "/stop", "/pause", "/resume", "/amend":
		return true
	}
	return false
}
func ownerControlPayload(text string, payload map[string]any) {
	fields := strings.Fields(text)
	if len(fields) < 2 {
		return
	}
	kind := ""
	switch strings.ToLower(fields[0]) {
	case "/stop":
		kind = task.ControlStop
	case "/pause":
		kind = task.ControlPause
	case "/resume":
		kind = task.ControlResume
	case "/amend":
		kind = task.ControlAmend
	}
	if kind == "" {
		return
	}
	payload["intent"] = kind
	payload["task_id"] = fields[1]
	if kind == task.ControlAmend {
		instruction := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(strings.TrimPrefix(text, fields[0])), fields[1]))
		payload["text"] = instruction
		_, policy := task.ClassifyOwnerText(instruction)
		if policy != "" {
			payload["execution_policy"] = policy
		}
	}
}
