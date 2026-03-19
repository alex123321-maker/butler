package run

import (
	"strings"
	"testing"
	"time"
)

func TestBuildTaskListQuery_DefaultsAndCoreFields(t *testing.T) {
	t.Parallel()

	query, args := buildTaskListQuery(TaskListParams{})

	if !strings.Contains(query, "FROM runs r") {
		t.Fatalf("expected runs source in query: %s", query)
	}
	if !strings.Contains(query, "s.channel AS source_channel") {
		t.Fatalf("expected source_channel projection: %s", query)
	}
	if !strings.Contains(query, "COALESCE(last_assistant.content, '') AS outcome_summary") {
		t.Fatalf("expected outcome summary projection: %s", query)
	}
	if !strings.Contains(query, "ORDER BY r.started_at DESC") {
		t.Fatalf("expected default sort started_at desc: %s", query)
	}
	if len(args) != 2 || args[0] != 50 || args[1] != 0 {
		t.Fatalf("expected default paging args [50 0], got %#v", args)
	}
}

func TestBuildTaskListQuery_FiltersAndPaging(t *testing.T) {
	t.Parallel()

	from := time.Date(2026, 3, 18, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 3, 19, 0, 0, 0, 0, time.UTC)
	needsAction := true

	query, args := buildTaskListQuery(TaskListParams{
		Status:          "waiting_for_reply_in_telegram",
		NeedsUserAction: &needsAction,
		WaitingReason:   "approval_required",
		SourceChannel:   "Telegram",
		Provider:        "OpenAI",
		From:            &from,
		To:              &to,
		Query:           "redis",
		Limit:           10,
		Offset:          5,
		Sort:            TaskSortUpdatedAtAsc,
	})

	if !strings.Contains(query, "LOWER(s.channel) =") {
		t.Fatalf("expected source_channel filter in query: %s", query)
	}
	if !strings.Contains(query, "LOWER(r.model_provider) =") {
		t.Fatalf("expected provider filter in query: %s", query)
	}
	if !strings.Contains(query, "r.current_state = 'awaiting_approval'") {
		t.Fatalf("expected awaiting_approval filter in query: %s", query)
	}
	if !strings.Contains(query, "ORDER BY r.updated_at ASC") {
		t.Fatalf("expected updated_at asc sort in query: %s", query)
	}
	if len(args) < 7 {
		t.Fatalf("expected enough args for filters, got %#v", args)
	}
	if args[len(args)-2] != 10 || args[len(args)-1] != 5 {
		t.Fatalf("expected trailing paging args [10 5], got %#v", args[len(args)-2:])
	}
}

func TestStatusAndWaitingReasonFilterSQL(t *testing.T) {
	t.Parallel()

	if clause := statusFilterSQL("timed_out_unknown"); clause != "1=0" {
		t.Fatalf("expected unknown status to block results, got %q", clause)
	}
	if clause := waitingReasonFilterSQL("unsupported_reason"); clause != "1=0" {
		t.Fatalf("expected unknown waiting reason to block results, got %q", clause)
	}
	if clause := statusFilterSQL("completed_with_issues"); !strings.Contains(clause, "timed_out") {
		t.Fatalf("expected timed_out mapping, got %q", clause)
	}
}
