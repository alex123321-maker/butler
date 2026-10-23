package telegramrich

import (
	"context"
	"encoding/json"
	"github.com/butler/butler/apps/orchestrator/internal/communication"
	"github.com/butler/butler/internal/communicationcontrol"
	"testing"
)

func TestCommunicationDecorationOptionDescriptionCanVeto(t *testing.T) {
	p := planner()
	p.Profile.Widget = true
	p.Decorator = NewDecorator()
	a := communicationcontrol.Args{Question: "Что выбрать?", Input: &communicationcontrol.QuestionInput{Type: "single_choice", Options: []communicationcontrol.Option{{ID: "music", Label: "Слушаю музыку", Description: "В красных наушниках"}, {ID: "code", Label: "Пишу код"}, {ID: "failure", Label: "Готово", Description: "Ошибка"}}}}
	parts, e := p.Plan(context.Background(), nil, "run", communication.Binding{Channel: "telegram"}, a)
	if e != nil {
		t.Fatal(e)
	}
	var widget Part
	for _, raw := range parts {
		var part Part
		_ = json.Unmarshal([]byte(raw), &part)
		if part.Widget {
			widget = part
		}
	}
	if widget.Decoration == nil || len(widget.Decoration.Options) != 1 || widget.Decoration.Options["code"] == "" {
		t.Fatal("description did not veto incompatible label", widget.Decoration)
	}
}

func BenchmarkDecorationFreeze(b *testing.B) {
	d := NewDecorator()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d.Freeze("Привет!\nСлушаю музыку.\nСпасибо большое.", nil)
	}
}
