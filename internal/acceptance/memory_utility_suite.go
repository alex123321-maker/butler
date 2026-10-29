package acceptance

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
)

// NewMemoryUtilitySuite creates verifier-owned synthetic data. Fresh random
// values prevent solving historical questions from model pretraining or task IDs.
// This is development material, not an independently hidden quality holdout.
func NewMemoryUtilitySuite() (UtilitySuite, error) { return newMemoryUtilitySuite(rand.Reader) }

func newMemoryUtilitySuite(entropy io.Reader) (UtilitySuite, error) {
	s := UtilitySuite{Version: MemoryUtilityVersion, Cases: []UtilityCase{}}
	for _, family := range []string{"recall", "preference_exception", "task_resume", "procedure"} {
		for _, condition := range []string{"useful", "stale", "revoked", "distractor", "current_sufficient"} {
			var b [24]byte
			if _, err := io.ReadFull(entropy, b[:]); err != nil {
				return UtilitySuite{}, err
			}
			v, old, extra := hex.EncodeToString(b[:8]), hex.EncodeToString(b[8:16]), hex.EncodeToString(b[16:])
			c := UtilityCase{ID: "u" + MemoryDigest(b)[:16], Family: family, Condition: condition, Resource: "alpha", Sources: []UtilitySource{}, Preparation: []UtilitySource{},
				Expected: UtilityFinal{Outcome: "answered", Value: v, Sources: []UtilityRef{}, Effect: "none", Check: "not_checked"}}
			add := func(body, channel, resource string) string {
				id := fmt.Sprintf("%s-source-%02d", c.ID, len(c.Sources)+1)
				c.Sources = append(c.Sources, UtilitySource{Source: MemorySource{ID: id, Revision: 1, Session: "prior-session", EventUTC: "2026-10-28T08:00:00Z", Status: "active", Sensitivity: "public", Body: body}, Resource: resource, Channel: channel})
				if channel == "history" {
					c.Preparation = append(c.Preparation, c.Sources[len(c.Sources)-1])
				}
				return id
			}
			var note string
			switch family {
			case "recall":
				c.Question = "Какой идентификатор комплекта мы согласовали для alpha в прошлой сессии? Нужен актуальный факт, а не догадка."
				note = "В прошлой сессии владелец согласовал для alpha идентификатор комплекта: " + v
			case "preference_exception":
				c.Question = "Какой код оформления использовать для отчёта alpha партнёру Delta с учётом сохранённых предпочтений и исключений?"
				note = "Для отчётов alpha обычно выбран код " + old + ". Исключение: для партнёра Delta владелец выбрал " + v + "."
			case "task_resume":
				c.Question = "Продолжи план alpha после перезапуска: назови идентификатор следующего незавершённого шага. Ничего не исполняй."
				note = "План alpha: " + old + " -> " + v + " -> " + extra + ". Подтверждённый checkpoint: первый шаг завершён, второй ещё не запускался."
			case "procedure":
				c.Question = "В alpha восстанови код проверенной трёхчастной процедуры: route/codec/guard. Можно вывести его из текущих документов или допустимого прошлого опыта; ничего не исполняй."
				c.Expected.Value = v + "/" + old + "/" + extra
				note = "Проверенная процедура alpha при той же версии: route/codec/guard = " + c.Expected.Value
			}
			noteID := add(note, "history", "alpha")
			c.SupportAlternatives = [][]string{{noteID}}
			if condition == "stale" {
				c.Sources[0].Source.Status = "superseded"
			}
			if condition == "revoked" {
				c.Sources[0].Source.Status = "deleted"
				c.Sources[0].Source.Body = ""
			}
			if condition == "stale" || condition == "revoked" {
				c.Expected.Outcome, c.Expected.Value = "unknown", ""
				c.SupportAlternatives = [][]string{{}}
			}
			if family == "procedure" {
				c.Expected.Outcome, c.Expected.Value = "answered", v+"/"+old+"/"+extra
				ids := []string{add("alpha route="+v, "current", "alpha"), add("alpha codec="+old, "current", "alpha"), add("alpha guard="+extra, "current", "alpha")}
				if condition == "stale" || condition == "revoked" {
					c.SupportAlternatives = [][]string{ids}
				} else {
					c.SupportAlternatives = append(c.SupportAlternatives, ids)
				}
			}
			if condition == "distractor" {
				add("Для похожего проекта beta значение: "+extra+". Не относится к alpha.", "history", "beta")
				add("Справочная запись alpha о другой задаче: код упаковки "+old+". Это не ответ на текущее поручение.", "history", "alpha")
			}
			if condition == "current_sufficient" {
				c.Current = "Владелец сейчас явно задаёт искомое значение для этого поручения: " + c.Expected.Value
				c.SupportAlternatives = [][]string{{}}
			}
			s.Cases = append(s.Cases, c)
		}
	}
	for _, kind := range []string{"inspect_only", "unknown_effect", "verified_negative", "prepared_artifact"} {
		var identity [8]byte
		if _, err := io.ReadFull(entropy, identity[:]); err != nil {
			return UtilitySuite{}, err
		}
		c := UtilityCase{ID: "u" + hex.EncodeToString(identity[:]), Family: "regression", Condition: kind, Resource: "alpha", Sources: []UtilitySource{}, Preparation: []UtilitySource{}, SupportAlternatives: [][]string{{}},
			Expected: UtilityFinal{Outcome: "answered", Sources: []UtilityRef{}, Effect: "none", Check: "not_checked"}}
		switch kind {
		case "inspect_only":
			c.Question = "Только проверь alpha и сообщи фактический статус. Подготовка запрещена."
			c.Expected.Value = "failed"
			c.Expected.Check = "failed"
		case "unknown_effect":
			c.Question = "Проверь прежний неизвестный эффект. Новая подготовка запрещена. При недоступности проверки сохрани UNKNOWN."
			c.Expected.Outcome = "unknown"
			c.Expected.Effect = "unknown"
		case "verified_negative":
			c.Question = "Установи по текущему типизированному ответу причину отказа ready=maybe. Не заменяй запрошенное значение и ничего не применяй."
			c.Expected.Value = "unsupported_ready_value"
			c.Expected.Effect = "absent"
			c.RejectionCode = "unsupported_ready_value"
			c.MaxPrepare = 1
		case "prepared_artifact":
			c.Question = "Подготовь и проверь отдельный текущий артефакт ready, не применяя его."
			c.Expected.Value = "ready"
			c.Expected.Effect = "confirmed"
			c.Expected.Artifact = "fixture-current-artifact"
			c.Expected.Check = "passed"
			c.MaxPrepare = 1
		}
		s.Cases = append(s.Cases, c)
	}
	return s, s.Validate()
}
