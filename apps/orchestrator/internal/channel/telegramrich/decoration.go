package telegramrich

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"

	"strings"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"
)

//go:embed decoration_catalog.json
var decorationCatalogJSON []byte

type EmojiConcept struct {
	ID            string     `json:"id"`
	CustomEmojiID string     `json:"custom_emoji_id"`
	Unicode       string     `json:"unicode"`
	Enabled       bool       `json:"enabled"`
	Triggers      [][]string `json:"triggers"`
	Terms         []string   `json:"terms"`
}

// Decorator is immutable channel-local data. It never calls a model or a tool.
type Decorator struct {
	version  string
	concepts []EmojiConcept
}

func NewDecorator() *Decorator {
	var catalog struct {
		Version string         `json:"version"`
		Entries []EmojiConcept `json:"entries"`
	}
	if json.Unmarshal(decorationCatalogJSON, &catalog) != nil {
		return nil
	}
	h := sha256.Sum256(decorationCatalogJSON)
	return &Decorator{version: catalog.Version + ":" + hex.EncodeToString(h[:]), concepts: catalog.Entries}
}

var unsafeDecorationTerms = strings.Fields("не нет ни без нельзя никогда кроме вместо либо или но если возможно наверное кажется предположим отсутствие отсутств* исключ* ошиб* отказ* запрет* отмен* ложн* провал* недоступ* упал упала сбой авар* смерт* смерть болезнь трагед*")

type EmojiDecision struct {
	ID            string `json:"id,omitempty"`
	CustomEmojiID string `json:"custom_emoji_id,omitempty"`
	Unicode       string `json:"unicode,omitempty"`
	Reason        string `json:"reason"`
}

func words(text string) []string {
	return strings.FieldsFunc(strings.ReplaceAll(strings.ToLower(text), "ё", "е"), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
}
func wordMatches(word, pattern string) bool {
	if strings.HasSuffix(pattern, "*") {
		return strings.HasPrefix(word, strings.TrimSuffix(pattern, "*"))
	}
	return word == pattern
}
func matchesAny(word string, patterns []string) bool {
	for _, p := range patterns {
		if wordMatches(word, p) {
			return true
		}
	}
	return false
}
func (d *Decorator) Select(text string) EmojiDecision {
	reject := func(reason string) EmojiDecision { return EmojiDecision{Reason: reason} }
	if d == nil || len(text) > 512 || !utf8.ValidString(text) {
		return reject("unavailable_or_long")
	}
	if strings.ContainsAny(text, "?`\"«»<>/\\@=") {
		return reject("quoted_code_question_or_link")
	}
	terms := words(text)
	if len(terms) == 0 || len(terms) > 40 {
		return reject("empty_or_long")
	}
	negatives := unsafeDecorationTerms
	generic := strings.Fields("я мы ты вы он она оно они это этот эта эти тот та то вот в во на с со и а о об у к ко по из для от за под над перед между рядом как свой свои себе мне нам вам мой моя мое сейчас уже просто очень совсем еще также такой такая такое изображен изображена изображено изображены нарисован нарисована показан показана эмоджи смайл* колоб* пиксель* анимац* лицо лицом желаю")
	for _, t := range terms {
		if matchesAny(t, negatives) {
			return reject("negation_uncertainty_or_failure")
		}
	}
	var found []EmojiConcept
	triggered := false
	for _, c := range d.concepts {
		if !c.Enabled {
			continue
		}
		trigger := false
		for _, group := range c.Triggers {
			all := true
			for _, pattern := range group {
				seen := false
				for _, t := range terms {
					if wordMatches(t, pattern) {
						seen = true
						break
					}
				}
				all = all && seen
			}
			trigger = trigger || all
		}
		if !trigger {
			continue
		}
		triggered = true
		supported := true
		for _, t := range terms {
			if !matchesAny(t, generic) && !matchesAny(t, c.Terms) {
				supported = false
				break
			}
		}
		if supported {
			found = append(found, c)
		}
	}
	if len(found) > 1 {
		return reject("ambiguous_concepts")
	}
	if len(found) == 0 {
		if triggered {
			return reject("unsupported_detail")
		}
		return reject("no_reviewed_concept")
	}
	c := found[0]
	return EmojiDecision{ID: c.ID, CustomEmojiID: c.CustomEmojiID, Unicode: c.Unicode, Reason: "reviewed_concept"}
}

type EmojiInsertion struct {
	Offset int    `json:"offset_bytes"`
	Emoji  string `json:"emoji"`
	ID     string `json:"custom_emoji_id"`
}
type FrozenDecoration struct {
	Version      int               `json:"version"`
	Policy       string            `json:"policy"`
	SourceSHA256 string            `json:"source_sha256"`
	Insertions   []EmojiInsertion  `json:"insertions,omitempty"`
	Options      map[string]string `json:"option_icons,omitempty"`
}
type MessageEntity struct {
	Type          string `json:"type"`
	Offset        int    `json:"offset"`
	Length        int    `json:"length"`
	CustomEmojiID string `json:"custom_emoji_id"`
}

func textHash(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }
func utf16Len(s string) int    { return len(utf16.Encode([]rune(s))) }

// Freeze appends at most three reviewed emoji to sentence starts, retaining the
// complete original text. No model context/schema or authorization is modified.
func (d *Decorator) Freeze(text string, options map[string]string) *FrozenDecoration {
	if d == nil {
		return nil
	}
	f := &FrozenDecoration{Version: 1, Policy: d.version, SourceSHA256: textHash(text)}
	safeContext := true
	for _, word := range words(text) {
		if matchesAny(word, unsafeDecorationTerms) {
			safeContext = false
		}
	}
	if safeContext && !strings.ContainsAny(text, "`\"«»<>/\\") {
		start := 0
		seen := map[string]bool{}
		add := func(end int) {
			segment := text[start:end]
			trimmed := strings.TrimLeftFunc(segment, unicode.IsSpace)
			decision := d.Select(trimmed)
			existing := false
			for _, r := range trimmed {
				if unicode.IsSymbol(r) {
					existing = true
				}
			}
			if !existing && decision.ID != "" && !seen[decision.ID] && len(f.Insertions) < 3 {
				f.Insertions = append(f.Insertions, EmojiInsertion{Offset: start + len(segment) - len(trimmed), Emoji: decision.Unicode, ID: decision.CustomEmojiID})
				seen[decision.ID] = true
			}
			start = end
		}
		for offset, r := range text {
			if strings.ContainsRune(".!?\n", r) {
				add(offset + utf8.RuneLen(r))
			}
		}
		if start < len(text) {
			add(len(text))
		}
	}
	for id, label := range options {
		if v := d.Select(label); v.ID != "" {
			if f.Options == nil {
				f.Options = map[string]string{}
			}
			f.Options[id] = v.CustomEmojiID
		}
	}
	rendered, _, ok := renderDecoration(text, f)
	if !ok || utf16Len(rendered) > 4096 {
		f.Insertions = nil
	}
	if len(f.Insertions) == 0 && len(f.Options) == 0 {
		return nil
	}
	return f
}

// Render only stored choices; never re-run selection on delivery/retry/edit.
func renderDecoration(text string, f *FrozenDecoration) (string, []MessageEntity, bool) {
	if f == nil {
		return text, nil, true
	}
	if f.Version != 1 || f.Policy == "" || f.SourceSHA256 != textHash(text) || len(f.Insertions) > 3 || len(f.Options) > 32 {
		return "", nil, false
	}
	for option, id := range f.Options {
		if option == "" || !emojiID.MatchString(id) {
			return "", nil, false
		}
	}
	var b strings.Builder
	entities := []MessageEntity{}
	position := 0
	last := -1
	for _, in := range f.Insertions {
		if in.Offset <= last || in.Offset < position || in.Offset > len(text) || !utf8.ValidString(text[:in.Offset]) || !emojiID.MatchString(in.ID) || !validFrozenEmoji(in.ID, in.Emoji) || utf16Len(in.Emoji) > 16 || strings.ContainsAny(in.Emoji, "\n\r") {
			return "", nil, false
		}
		b.WriteString(text[position:in.Offset])
		offset := utf16Len(b.String())
		b.WriteString(in.Emoji)
		b.WriteByte(' ')
		entities = append(entities, MessageEntity{Type: "custom_emoji", Offset: offset, Length: utf16Len(in.Emoji), CustomEmojiID: in.ID})
		position = in.Offset
		last = in.Offset
	}
	b.WriteString(text[position:])
	return b.String(), entities, true
}

func optionIcons(f *FrozenDecoration) map[string]string {
	if f == nil {
		return nil
	}
	return f.Options
}

func validFrozenEmoji(id, fallback string) bool {
	d := NewDecorator()
	if d == nil {
		return false
	}
	for _, c := range d.concepts {
		if c.CustomEmojiID == id && c.Unicode == fallback {
			return true
		}
	}
	return false
}
