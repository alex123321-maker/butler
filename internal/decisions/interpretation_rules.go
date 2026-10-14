package decisions

import (
	"regexp"
	"strings"
)

const InterpretationRulesVersion = "russian-literal-interpretation-v3"

// Fixed transparent baseline. Patterns are frozen before any model prediction.
// They are approximate semantic observations, not authorization or truth.
func RuleInterpretation(focus, context string) Interpretation {
	s := strings.ToLower(focus)
	match := func(pattern string) bool { return regexp.MustCompile(pattern).MatchString(s) }
	o := Interpretation{Intents: []string{}, Modality: "direct", Target: "current_operation"}
	quoted := match(`^(в письме|в примере|записка|в старой переписке|цитата|коллега написал|он сказал)`)
	if quoted {
		o.Modality = "quoted"
	}
	if match(`может.*(стоит|лучше|отдельно)|как вариант|возможно|наверное|лишь рассматрива|ещё нет|лишь обдум`) {
		o.Modality = "tentative"
	}
	if match(`если|при ошибке|при сохранении|после успешной|лишь после`) {
		o.Modality = "conditional"
	}
	if match(`неверно|не согласен|я не прошу|ещё не отправлен|здесь неверна`) {
		o.Modality = "negated"
	}
	if match(`, а .*пока лишь|, но я прошу`) {
		o.Modality = "unresolved"
	}
	stop := match(`отмен|останов|приостанов|прекрати|прерви|пауз|отзываю поручение|сверни работу|больше не занимайся`)
	continuation := match(`не отменяй|не приостанавливай|не останавливай`)
	if continuation || match(`(строку|в шаблон текст|напечатай).*«`) {
		stop = false
	}
	change := match(`только|запрещ|исключи|срок|до .*ноября|не (трогай|выполняй|перезаписывай|обновляй)|без (изменений|установки)|нужна .*вместо|порт теперь|провод(и|ить) на тестов`)
	request := match(`проверь|проверить|убедись|воспроизведи|вернись|возобнови|продолжай|подготовь|сделай|построй|повтори|сними|снять|доведи|начни|выведи|вставь|установи|перезапусти|переключи|размещай|исследовать|рассмотреть|обдумать|попробовать|поменяй DNS`) || continuation
	question := match(`когда|сколько|почему|какой сервер|каком этапе|что означает|проверки .*проходят|ты остановил|он точно завершился|поясни (причину|метод)`)
	if match(`можешь проверить|не мог бы .*проверить`) {
		question = false
		request = true
	}
	info := match(`вернул|вырос|уточняю|имел в виду|лежат|предпочитаю|отвечает|недоступ|проверен|готов|готова|готовы|готово|законч|обновлены|отправлен|результат|ошибка|источник utf|считай сервис`)
	if quoted && (stop || request || question) {
		info = false
	}
	if match(`^(спасибо|привет|здравствуй)`) {
		info = false
	}
	for _, pair := range []struct {
		name    string
		present bool
	}{{"request_action", request}, {"change_constraint", change}, {"stop_work", stop}, {"ask_information", question}, {"provide_information", info}} {
		if pair.present {
			o.Intents = append(o.Intents, pair.name)
		}
	}
	if match(`отдельн|самостоятельную задачу|другого каталога|прошлогод|вчера законч`) {
		o.Target = "other_task"
	}
	if match(`целиком|вс[юяе] .*задач|этой задач|поручени|всю рассылку|весь релиз|весь аудит|всей .*схемы|сверку|сверки|книга закончена|общего срока|закончим задачу|перевод целиком|маршрут уже готов|отчёт готов`) {
		o.Target = "whole_current_task"
	}
	if match(`не указана|отсутствует|недоступны|две .*без|два .*отчёта|два одинаково`) || strings.TrimSpace(context) == "" {
		o.Target = "unresolved"
	}
	if regexp.MustCompile(`не указана|отсутствует|недоступны|две .*без|два одинаково|без выбран`).MatchString(strings.ToLower(context)) {
		o.Target = "unresolved"
	}
	if len(o.Intents) == 0 || match(`что означает слово`) {
		o.Target = "none"
	}
	o.Escalate = o.Modality != "direct" || o.Target == "unresolved"
	return o
}
