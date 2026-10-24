// Package executionnotice delivers fixed kernel lifecycle facts, never model
// text, task completion, tool authority or a replacement for a published result.
package executionnotice

import (
	"strings"
	"time"
)

type snapshot struct {
	InputID, RunID, SessionKey, Status, Checkpoint, ErrorCode, ApprovalRef string
	HasResult                                                              bool
	UpdatedAt                                                              time.Time
}

func classify(s snapshot) (code, reason string) {
	switch s.Status {
	case "queued":
		code = "queued"
	case "running":
		code = "working"
	case "waiting_for_user":
		code = "waiting_for_user"
		if s.ApprovalRef != "" {
			code = "approval_required"
		}
	case "blocked":
		code = "blocked"
	case "failed", "expired":
		code = "failed"
	case "cancelled":
		code = "cancelled"
	case "completed":
		code = "finished"
		if !s.HasResult {
			code = "missing_response"
		}
	default:
		return "", ""
	}
	if code != "blocked" && code != "failed" {
		return code, ""
	}
	switch {
	case strings.HasPrefix(s.ErrorCode, "budget_"), strings.HasPrefix(s.ErrorCode, "execution_budget"):
		reason = "budget"
	case strings.Contains(s.ErrorCode, "timeout"), strings.Contains(s.ErrorCode, "deadline"), s.Status == "expired":
		reason = "timeout"
	case s.ErrorCode == "provider_unavailable" || s.ErrorCode == "transport_connection_error" || s.ErrorCode == "rate_limited" || s.ErrorCode == "model_capacity_unavailable":
		reason = "model_unavailable"
	case s.ErrorCode == "effect_unknown":
		reason = "uncertain_action"
	case strings.HasPrefix(s.ErrorCode, "memory_source_"):
		reason = "source_unavailable"
	default:
		reason = "execution_failed"
	}
	return code, reason
}

// Text accepts finite code-owned fields. Raw errors, URLs, source content and
// user messages never enter a kernel notice, even when source access has failed.
func Text(code, reason string) string {
	switch code {
	case "queued":
		return "Запрос принят и ожидает обработки."
	case "working":
		return "Обрабатываю запрос."
	case "waiting_for_user":
		return "Для продолжения нужен ваш ответ на вопрос Батлера."
	case "approval_required":
		return "Для продолжения требуется ваше подтверждение. Откройте раздел «Подтверждения» в Butler."
	case "cancelled":
		return "Обработка запроса отменена."
	case "finished":
		return "Обработка завершена; ответ сохранён отдельно."
	case "missing_response":
		return "Обработка завершилась без опубликованного ответа. Результат пока не доставлен."
	case "blocked", "failed":
		switch reason {
		case "budget":
			return "Обработка остановлена: не хватает доступного бюджета выполнения."
		case "timeout":
			return "Обработка остановлена: превышено время ожидания."
		case "uncertain_action":
			return "Обработка остановлена: результат последнего действия не подтверждён. Перед продолжением нужна проверка его состояния."
		case "model_unavailable":
			return "Обработка остановлена: сервис модели сейчас недоступен. Можно повторить запрос позже."
		case "source_unavailable":
			return "Обработка остановлена: не удалось подтвердить доступ к использованным данным."
		default:
			return "Не удалось завершить обработку запроса из-за ошибки выполнения."
		}
	}
	return ""
}
