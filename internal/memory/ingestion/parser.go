// Package ingestion parses explicitly selected sources without executing content.
package ingestion

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/butler/butler/internal/memory/catalog"
)

const Version = "structured-go-v1"
const MaxBytes = 8 << 20
const MaxFragments = 2000
const fragmentBytes = 4096

var ErrFormat = errors.New("unsupported source format")
var ErrCorrupt = errors.New("source could not be parsed")
var ErrLimit = errors.New("source parsing limit exceeded")
var ErrOCR = errors.New("source has no usable text layer; reviewed OCR adapter required")

type Input struct {
	Format, Path, RepositoryRevision string
	Raw                              []byte
}
type Page struct {
	Text      string
	Number    int
	Method    string
	Uncertain bool
}

// PageAdapter is operator-selected and bounded by the caller context. It cannot
// grant data/model authority. OCR is called only after a successful empty text
// layer result; corruption never falls through to OCR.
type PageAdapter interface {
	TextLayer(context.Context, []byte) ([]Page, error)
	Recognize(context.Context, []byte) ([]Page, error)
}
type Parser struct{ Pages PageAdapter }
type Result struct {
	Text      string
	Fragments []catalog.Fragment
	Method    string
	Uncertain bool
}
type locator struct {
	Path               string `json:"path"`
	RepositoryRevision string `json:"repository_revision,omitempty"`
	ParserVersion      string `json:"parser_version"`
	Kind               string `json:"kind"`
	StartLine          int    `json:"start_line,omitempty"`
	EndLine            int    `json:"end_line,omitempty"`
	ByteStart          int    `json:"byte_start,omitempty"`
	ByteEnd            int    `json:"byte_end,omitempty"`
	Heading            string `json:"heading,omitempty"`
	Page               int    `json:"page,omitempty"`
	Author             string `json:"author,omitempty"`
	Time               string `json:"event_time,omitempty"`
	Thread             string `json:"thread,omitempty"`
	MessageID          string `json:"message_id,omitempty"`
	Method             string `json:"method,omitempty"`
	Uncertain          bool   `json:"uncertain,omitempty"`
}

func (p Parser) Parse(ctx context.Context, in Input) (Result, error) {
	if ctx.Err() != nil {
		return Result{}, ctx.Err()
	}
	if len(in.Raw) == 0 || len(in.Raw) > MaxBytes || len(in.Path) > 1024 || len(in.RepositoryRevision) > 256 {
		return Result{}, ErrLimit
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	var r Result
	var e error
	switch in.Format {
	case "go":
		r, e = parseGo(ctx, in)
	case "text", "markdown":
		if !utf8.Valid(in.Raw) || bytes.IndexByte(in.Raw, 0) >= 0 {
			return r, ErrCorrupt
		}
		r, e = parseText(ctx, in)
	case "csv", "tsv":
		r, e = parseTable(ctx, in)
	case "messages":
		r, e = parseMessages(ctx, in)
	case "docx":
		r, e = parseDocx(ctx, in)
	case "pdf":
		r, e = p.parsePages(ctx, in)
	default:
		return r, ErrFormat
	}
	if e != nil {
		return Result{}, e
	}
	if ctx.Err() != nil {
		return Result{}, ctx.Err()
	}
	if len(r.Text) > 16<<20 || len(r.Fragments) == 0 || len(r.Fragments) > MaxFragments {
		return Result{}, ErrLimit
	}
	if r.Method == "" {
		r.Method = Version
	}
	return r, nil
}

func base(in Input, kind string) locator {
	return locator{Path: in.Path, RepositoryRevision: in.RepositoryRevision, ParserVersion: Version, Kind: kind}
}
func add(ctx context.Context, r *Result, key, title, text string, l locator) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if !utf8.ValidString(text) || len(title) > 1024 {
		return ErrCorrupt
	}
	for part, offset := 0, 0; offset < len(text); part++ {
		if len(r.Fragments) >= MaxFragments {
			return ErrLimit
		}
		end := offset + fragmentBytes - len(title)
		if end > len(text) {
			end = len(text)
		}
		// Ensure neither UTF-8 nor a line is silently discarded at the boundary.
		if end < len(text) {
			for end > offset && !utf8.RuneStart(text[end]) {
				end--
			}
		}
		if end <= offset {
			return ErrLimit
		}
		loc := l
		loc.ByteStart = l.ByteStart + offset
		loc.ByteEnd = l.ByteStart + end
		b, _ := json.Marshal(loc)
		r.Fragments = append(r.Fragments, catalog.Fragment{Key: key + ":" + strconv.Itoa(part), Title: title, Text: text[offset:end], Locator: b})
		offset = end
	}
	return nil
}

func parseGo(ctx context.Context, in Input) (Result, error) {
	if !utf8.Valid(in.Raw) {
		return Result{}, ErrCorrupt
	}
	fset := token.NewFileSet()
	f, e := parser.ParseFile(fset, in.Path, in.Raw, parser.ParseComments|parser.AllErrors)
	if ctx.Err() != nil {
		return Result{}, ctx.Err()
	}
	if e != nil {
		return Result{}, ErrCorrupt
	}
	r := Result{Text: string(in.Raw)}
	for _, d := range f.Decls {
		start, end := fset.Position(d.Pos()).Offset, fset.Position(d.End()).Offset
		key, kind, title := "", "declaration", ""
		var doc *ast.CommentGroup
		switch v := d.(type) {
		case *ast.FuncDecl:
			kind = "function"
			key = v.Name.Name
			doc = v.Doc
			if v.Recv != nil {
				var recv bytes.Buffer
				for _, field := range v.Recv.List {
					recv.Write(in.Raw[fset.Position(field.Type.Pos()).Offset:fset.Position(field.Type.End()).Offset])
				}
				key = recv.String() + "." + key
			}
			signatureEnd := fset.Position(v.End()).Offset
			if v.Body != nil {
				signatureEnd = fset.Position(v.Body.Pos()).Offset
			}
			title = strings.Join(strings.Fields(string(in.Raw[start:signatureEnd])), " ")
			if doc != nil {
				// Keep bounded comment context in each part, even when the body
				// spans multiple fragments. The original remains byte-exact.
				comment := strings.Join(strings.Fields(doc.Text()), " ")
				if len(comment) > 384 {
					comment = comment[:384]
					for !utf8.ValidString(comment) {
						comment = comment[:len(comment)-1]
					}
				}
				title += " · " + comment
			}
			if len(title) > 1024 {
				return Result{}, ErrLimit
			}
		case *ast.GenDecl:
			if v.Tok == token.IMPORT {
				continue
			}
			doc = v.Doc
			kind = v.Tok.String()
			for _, spec := range v.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					key += s.Name.Name + ","
				case *ast.ValueSpec:
					for _, n := range s.Names {
						key += n.Name + ","
					}
				}
			}
			title = strings.TrimSuffix(key, ",")
		}
		if doc != nil {
			start = fset.Position(doc.Pos()).Offset
		}
		if start < 0 || end > len(in.Raw) || start >= end {
			return Result{}, ErrCorrupt
		}
		l := base(in, kind)
		l.StartLine = fset.Position(token.Pos(d.Pos())).Line
		l.EndLine = fset.Position(d.End()).Line
		l.ByteStart = start
		// Signature/name and comment stay in every part's title/locator, while
		// the original complete source remains available for exact byte offsets.
		if e = add(ctx, &r, kind+":"+key, title, string(in.Raw[start:end]), l); e != nil {
			return r, e
		}
	}
	if len(r.Fragments) == 0 {
		return parseText(ctx, in)
	}
	return r, nil
}

func parseText(ctx context.Context, in Input) (Result, error) {
	r := Result{Text: strings.ReplaceAll(string(in.Raw), "\r\n", "\n")}
	lines := strings.Split(r.Text, "\n")
	fenced := false
	heading := ""
	start, offset := 0, 0
	flush := func(end int) error {
		text := strings.Join(lines[start:end], "\n")
		if strings.TrimSpace(text) == "" {
			return nil
		}
		l := base(in, "section")
		l.StartLine = start + 1
		l.EndLine = end
		l.ByteStart = offset
		l.Heading = heading
		return add(ctx, &r, "section:"+strconv.Itoa(start+1), heading, text, l)
	}
	for i, line := range lines {
		if ctx.Err() != nil {
			return r, ctx.Err()
		}
		if i < start {
			continue
		} // rows already consumed by a table block
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			fenced = !fenced
		}
		if in.Format == "markdown" && !fenced && strings.HasPrefix(line, "#") {
			if i > start {
				if e := flush(i); e != nil {
					return r, e
				}
				offset += len(strings.Join(lines[start:i], "\n")) + 1
			}
			start = i
			heading = strings.TrimSpace(strings.TrimLeft(line, "#"))
		}
		// Keep Markdown tables as logical row blocks, repeating their header.
		if in.Format == "markdown" && !fenced && i >= start && i+1 < len(lines) && strings.Contains(line, "|") && strings.Contains(lines[i+1], "---") {
			if i > start {
				if e := flush(i); e != nil {
					return r, e
				}
				offset += len(strings.Join(lines[start:i], "\n")) + 1
			}
			end := i + 2
			for end < len(lines) && strings.Contains(lines[end], "|") {
				end++
			}
			for row := i + 2; row < end; row++ {
				l := base(in, "table_row")
				l.StartLine = row + 1
				l.EndLine = row + 1
				l.Heading = heading
				if e := add(ctx, &r, "table:"+strconv.Itoa(i+1)+":"+strconv.Itoa(row-i), heading, lines[i]+"\n"+lines[i+1]+"\n"+lines[row], l); e != nil {
					return r, e
				}
			}
			offset += len(strings.Join(lines[i:end], "\n")) + 1
			start = end
		}
	}
	if start < len(lines) {
		if e := flush(len(lines)); e != nil {
			return r, e
		}
	}
	return r, nil
}

func parseTable(ctx context.Context, in Input) (Result, error) {
	if !utf8.Valid(in.Raw) {
		return Result{}, ErrCorrupt
	}
	r := Result{Text: string(in.Raw)}
	reader := csv.NewReader(bytes.NewReader(in.Raw))
	if in.Format == "tsv" {
		reader.Comma = '\t'
	}
	header, e := reader.Read()
	if e != nil || len(header) == 0 || len(header) > 128 {
		return r, ErrCorrupt
	}
	for index := 1; ; index++ {
		row, e := reader.Read()
		if e == io.EOF {
			break
		}
		if e != nil {
			return r, ErrCorrupt
		}
		var values []string
		for i, v := range row {
			values = append(values, header[i]+" = "+v)
		}
		l := base(in, "table_row")
		l.StartLine = index + 1
		l.EndLine = index + 1
		if e = add(ctx, &r, "row:"+strconv.Itoa(index), "table", strings.Join(values, "\n"), l); e != nil {
			return r, e
		}
	}
	return r, nil
}

func parseMessages(ctx context.Context, in Input) (Result, error) {
	if !utf8.Valid(in.Raw) {
		return Result{}, ErrCorrupt
	}
	type message struct {
		ID     string    `json:"id"`
		Author string    `json:"author"`
		Time   time.Time `json:"time"`
		Thread string    `json:"thread"`
		Text   string    `json:"text"`
	}
	d := json.NewDecoder(bytes.NewReader(in.Raw))
	d.DisallowUnknownFields()
	opening, err := d.Token()
	if err != nil || opening != json.Delim('[') {
		return Result{}, ErrCorrupt
	}
	r := Result{}
	var text strings.Builder
	seen := map[string]bool{}
	for count := 0; d.More(); count++ {
		if count >= MaxFragments {
			return Result{}, ErrLimit
		}
		if ctx.Err() != nil {
			return Result{}, ctx.Err()
		}
		var m message
		if d.Decode(&m) != nil {
			return Result{}, ErrCorrupt
		}
		if m.ID == "" || len(m.ID) > 256 || seen[m.ID] || m.Author == "" || len(m.Author) > 256 || len(m.Thread) > 1024 || m.Time.IsZero() {
			return r, ErrCorrupt
		}
		seen[m.ID] = true
		l := base(in, "message")
		l.Author = m.Author
		l.Time = m.Time.UTC().Format(time.RFC3339Nano)
		l.Thread = m.Thread
		l.MessageID = m.ID
		if e := add(ctx, &r, "message:"+m.ID, m.Author+" · "+l.Time, m.Text, l); e != nil {
			return r, e
		}
		text.WriteString(m.Author + " · " + l.Time + " · " + m.Thread + "\n" + m.Text + "\n")
		if text.Len() > 16<<20 {
			return Result{}, ErrLimit
		}
	}
	if closing, e := d.Token(); e != nil || closing != json.Delim(']') {
		return Result{}, ErrCorrupt
	}
	var trailing any
	if d.Decode(&trailing) != io.EOF {
		return Result{}, ErrCorrupt
	}
	r.Text = text.String()
	return r, nil
}

func parseDocx(ctx context.Context, in Input) (Result, error) {
	z, e := zip.NewReader(bytes.NewReader(in.Raw), int64(len(in.Raw)))
	if e != nil || len(z.File) > 500 {
		return Result{}, ErrCorrupt
	}
	var doc *zip.File
	for _, f := range z.File {
		if strings.Contains(strings.ToLower(f.Name), "vbaproject") {
			return Result{}, ErrCorrupt
		}
		if f.Name == "word/document.xml" {
			doc = f
		}
	}
	if doc == nil || doc.UncompressedSize64 > 16<<20 {
		return Result{}, ErrCorrupt
	}
	file, e := doc.Open()
	if e != nil {
		return Result{}, ErrCorrupt
	}
	defer file.Close()
	d := xml.NewDecoder(io.LimitReader(file, 16<<20))
	r := Result{}
	var paragraph, heading string
	var cells []string
	table, cell, text := false, false, false
	page, index := 1, 0
	header := ""
	for {
		if ctx.Err() != nil {
			return r, ctx.Err()
		}
		tok, e := d.Token()
		if e == io.EOF {
			break
		}
		if e != nil {
			return r, ErrCorrupt
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "tbl":
				table = true
				header = ""
			case "tr":
				cells = nil
			case "tc":
				cell = true
				paragraph = ""
			case "t":
				text = true
			case "pStyle":
				for _, a := range t.Attr {
					if a.Name.Local == "val" && strings.HasPrefix(a.Value, "Heading") {
						heading = "pending"
					}
				}
			case "br":
				for _, a := range t.Attr {
					if a.Name.Local == "type" && a.Value == "page" {
						page++
					}
				}
			}
		case xml.CharData:
			if text {
				paragraph += string(t)
				if len(paragraph) > MaxBytes {
					return r, ErrLimit
				}
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "t":
				text = false
			case "tc":
				cells = append(cells, paragraph)
				paragraph = ""
				cell = false
			case "tr":
				if header == "" {
					header = strings.Join(cells, " | ")
				} else {
					index++
					l := base(in, "table_row")
					l.Page = page
					l.Heading = heading
					value := header + "\n" + strings.Join(cells, " | ")
					if e := add(ctx, &r, "table:"+strconv.Itoa(index), heading, value, l); e != nil {
						return r, e
					}
					r.Text += value + "\n"
				}
			case "tbl":
				table = false
			case "p":
				if cell {
					paragraph += "\n"
					continue
				}
				if !table && strings.TrimSpace(paragraph) != "" {
					if heading == "pending" {
						heading = paragraph
					}
					index++
					l := base(in, "paragraph")
					l.Page = page
					l.Heading = heading
					if e := add(ctx, &r, "paragraph:"+strconv.Itoa(index), heading, paragraph, l); e != nil {
						return r, e
					}
					r.Text += paragraph + "\n"
				}
				paragraph = ""
			}
		}
	}
	if len(r.Fragments) == 0 {
		return r, ErrOCR
	}
	return r, nil
}

func (p Parser) parsePages(ctx context.Context, in Input) (Result, error) {
	if p.Pages == nil {
		return Result{}, ErrFormat
	}
	pages, e := p.Pages.TextLayer(ctx, in.Raw)
	if e != nil {
		return Result{}, e
	}
	usable := false
	for _, page := range pages {
		if utf8.ValidString(page.Text) && len(strings.TrimSpace(page.Text)) > 0 {
			usable = true
		}
	}
	if !usable {
		pages, e = p.Pages.Recognize(ctx, in.Raw)
		if e != nil {
			return Result{}, e
		}
		for i := range pages {
			pages[i].Uncertain = true
			if pages[i].Method == "" {
				pages[i].Method = "ocr"
			}
		}
	}
	if len(pages) > 100 {
		return Result{}, ErrLimit
	}
	r := Result{}
	for i, page := range pages {
		if page.Number < 1 {
			return r, ErrCorrupt
		}
		l := base(in, "page")
		l.Page = page.Number
		l.Method = page.Method
		l.Uncertain = page.Uncertain
		if e := add(ctx, &r, fmt.Sprint("page:", i+1), "page "+strconv.Itoa(page.Number), page.Text, l); e != nil {
			return r, e
		}
		r.Text += page.Text + "\n"
		r.Uncertain = r.Uncertain || page.Uncertain
		r.Method = page.Method
	}
	if strings.TrimSpace(r.Text) == "" {
		return Result{}, ErrOCR
	}
	return r, nil
}
