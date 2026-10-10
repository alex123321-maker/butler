package ingestion

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestRussianTablesAndGoEntityContext(t *testing.T) {
	p := Parser{}
	md := "# Развёртывание\nПараметры сервиса.\n\n| Узел | Лимит |\n| --- | --- |\n| laba | 16 ГБ |\n| backup | 8 ГБ |\n"
	r, e := p.Parse(context.Background(), Input{Format: "markdown", Path: "инструкция.md", Raw: []byte(md)})
	if e != nil {
		t.Fatal(e)
	}
	rows := 0
	for _, f := range r.Fragments {
		if strings.Contains(f.Text, "16 ГБ") || strings.Contains(f.Text, "8 ГБ") {
			rows++
			if !strings.Contains(f.Text, "Узел") || !strings.Contains(f.Text, "Лимит") {
				t.Fatal("detached table cells", f)
			}
		}
	}
	if rows != 2 {
		t.Fatal(rows, r)
	}
	code := "package сеть\n// Process сохраняет русские имена.\nfunc Process(значение string) string {\n" + strings.Repeat("// длинная строка без исполнения\n", 400) + "return значение\n}\ntype Host struct { Имя string }\n"
	r, e = p.Parse(context.Background(), Input{Format: "go", Path: "src/сеть.go", RepositoryRevision: "commit-123", Raw: []byte(code)})
	if e != nil || len(r.Fragments) < 3 {
		t.Fatal(r, e)
	}
	joined := ""
	for _, f := range r.Fragments {
		if !utf8.ValidString(f.Text) {
			t.Fatal("damaged UTF-8")
		}
		var l locator
		if json.Unmarshal(f.Locator, &l) != nil || l.Path != "src/сеть.go" || l.RepositoryRevision != "commit-123" {
			t.Fatal(string(f.Locator))
		}
		if strings.HasPrefix(f.Key, "function:Process:") {
			joined += f.Text
			if !strings.Contains(f.Title, "func Process(значение string) string") || !strings.Contains(f.Title, "русские имена") || l.StartLine != 3 {
				t.Fatal(f)
			}
		}
	}
	if !strings.Contains(joined, "// Process") || !strings.Contains(joined, "return значение") {
		t.Fatal("lost function text/comment")
	}
	if r, e = p.Parse(context.Background(), Input{Format: "csv", Path: "ресурсы.csv", Raw: []byte("Узел,Лимит\nlaba,16 ГБ\nbackup,8 ГБ\n")}); e != nil || len(r.Fragments) != 2 || !strings.Contains(r.Fragments[0].Text, "Лимит = 16 ГБ") {
		t.Fatal(r, e)
	}
}

func TestMessageArrayIsBoundedBeforeMaterializingAndRejectsInvalidUTF8(t *testing.T) {
	p := Parser{}
	item := `{"id":"x","author":"a","time":"2026-01-01T00:00:00Z","text":"x"}`
	var b strings.Builder
	b.WriteByte('[')
	for i := 0; i < MaxFragments+1; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strings.Replace(item, `"x"`, `"`+strconv.Itoa(i)+`"`, 1))
	}
	b.WriteByte(']')
	if _, e := p.Parse(context.Background(), Input{Format: "messages", Path: "messages.json", Raw: []byte(b.String())}); !errors.Is(e, ErrLimit) {
		t.Fatal(e)
	}
	raw := []byte(strings.Replace(item, `"x"`, `"\xff"`, 1))
	raw = append([]byte("["), append(raw, ']')...)
	raw[bytes.Index(raw, []byte(`\xff`))] = 0xff
	if _, e := p.Parse(context.Background(), Input{Format: "messages", Path: "messages.json", Raw: raw}); !errors.Is(e, ErrCorrupt) {
		t.Fatal(e)
	}
}

func TestDocxTablesAndMessageAuthorTimeThread(t *testing.T) {
	var raw bytes.Buffer
	z := zip.NewWriter(&raw)
	f, _ := z.Create("word/document.xml")
	_, _ = f.Write([]byte(`<w:document xmlns:w="test"><w:body><w:p><w:pPr><w:pStyle w:val="Heading1"/></w:pPr><w:r><w:t>Русский раздел</w:t></w:r></w:p><w:tbl><w:tr><w:tc><w:p><w:r><w:t>Имя</w:t></w:r></w:p></w:tc><w:tc><w:p><w:r><w:t>Память</w:t></w:r></w:p></w:tc></w:tr><w:tr><w:tc><w:p><w:r><w:t>laba</w:t></w:r></w:p></w:tc><w:tc><w:p><w:r><w:t>16 ГБ</w:t></w:r></w:p></w:tc></w:tr></w:tbl></w:body></w:document>`))
	_ = z.Close()
	r, e := (Parser{}).Parse(context.Background(), Input{Format: "docx", Path: "документ.docx", Raw: raw.Bytes()})
	if e != nil || len(r.Fragments) != 2 || !strings.Contains(r.Fragments[1].Text, "Память") || !strings.Contains(r.Fragments[1].Text, "16 ГБ") {
		t.Fatal(r, e)
	}
	r, e = (Parser{}).Parse(context.Background(), Input{Format: "messages", Path: "conversation.json", Raw: []byte(`[{"id":"message-a","author":"Иван","time":"2026-10-11T00:00:00Z","thread":"обсуждение","text":"Перезапуск Service.Restart разрешён отдельно"}]`)})
	if e != nil || len(r.Fragments) != 1 {
		t.Fatal(r, e)
	}
	var l locator
	_ = json.Unmarshal(r.Fragments[0].Locator, &l)
	if l.Author != "Иван" || l.Thread != "обсуждение" || l.Time == "" {
		t.Fatal(l)
	}
}

type pageFixture struct {
	calls   int
	corrupt bool
	text    string
}

func (p *pageFixture) TextLayer(context.Context, []byte) ([]Page, error) {
	if p.corrupt {
		return nil, ErrCorrupt
	}
	return []Page{{Text: p.text, Number: 1, Method: "pdf_text_layer"}}, nil
}
func (p *pageFixture) Recognize(context.Context, []byte) ([]Page, error) {
	p.calls++
	return []Page{{Text: "Распознанный текст страницы", Number: 1}}, nil
}
func TestOCROnlyAfterSuccessfulEmptyLayerAndBoundedFailures(t *testing.T) {
	p := &pageFixture{text: "Это пригодный исходный текстовый слой"}
	parser := Parser{Pages: p}
	in := Input{Format: "pdf", Path: "file.pdf", Raw: []byte("fixture")}
	r, e := parser.Parse(context.Background(), in)
	if e != nil || p.calls != 0 || r.Uncertain {
		t.Fatal(r, e, p.calls)
	}
	p.text = ""
	r, e = parser.Parse(context.Background(), in)
	if e != nil || p.calls != 1 || !r.Uncertain || !strings.Contains(string(r.Fragments[0].Locator), `"method":"ocr"`) {
		t.Fatal(r, e, p.calls)
	}
	p.corrupt = true
	if _, e = parser.Parse(context.Background(), in); !errors.Is(e, ErrCorrupt) || p.calls != 1 {
		t.Fatal("corruption became OCR", e, p.calls)
	}
	for _, input := range []Input{{Format: "go", Path: "bad.go", Raw: []byte("package p\nfunc { invalid")}, {Format: "docx", Path: "bad.docx", Raw: []byte("corrupt zip")}, {Format: "text", Path: "too-large.txt", Raw: bytes.Repeat([]byte("a"), MaxBytes+1)}} {
		if _, e = (Parser{}).Parse(context.Background(), input); e == nil {
			t.Fatal("invalid source parsed", input.Format)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e = (Parser{}).Parse(ctx, Input{Format: "text", Path: "cancel.txt", Raw: []byte("cancelled")}); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
}
