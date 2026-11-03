// Package browserspa defines the fixed, credential-free SPA query profile.
// It does not classify arbitrary POSTs or WebSocket messages as reads.
package browserspa

import (
	"encoding/json"
	"errors"
	"strings"
	"text/scanner"

	"github.com/butler/butler/internal/jsonstrict"
)

var ErrDenied = errors.New("SPA query profile denied")

// QueryBody permits one conventional GraphQL query. Mutations, subscriptions,
// batches, extensions, fragments, directives and persisted-query fallbacks are
// intentionally unsupported. The full document must parse; a prefix is not proof.
func QueryBody(body []byte) error {
	return readBody(body, "query")
}

func SubscriptionBody(body []byte) error {
	return readBody(body, "subscription")
}

func readBody(body []byte, operation string) error {
	if len(body) > 64*1024 {
		return ErrDenied
	}
	var envelope struct {
		Query         string          `json:"query"`
		Variables     json.RawMessage `json:"variables,omitempty"`
		OperationName string          `json:"operationName,omitempty"`
	}
	if jsonstrict.Decode(body, &envelope) != nil || len(envelope.Query) == 0 || len(envelope.Query) > 16*1024 {
		return ErrDenied
	}
	if len(envelope.Variables) > 0 && string(envelope.Variables) != "null" {
		var object map[string]json.RawMessage
		if jsonstrict.Decode(envelope.Variables, &object) != nil || object == nil {
			return ErrDenied
		}
	}
	p := queryParser{}
	p.s.Init(strings.NewReader(envelope.Query))
	p.s.Mode = scanner.ScanIdents | scanner.ScanInts | scanner.ScanFloats | scanner.ScanStrings
	p.s.Whitespace |= 1 << ','
	p.s.Error = func(*scanner.Scanner, string) { p.invalid = true }
	p.next()
	name := ""
	if p.text == operation {
		p.next()
		if p.token == scanner.Ident {
			name = p.text
			p.next()
		}
		if p.accept('(') {
			count := 0
			for p.token != ')' && !p.invalid {
				count++
				p.expect('$')
				p.ident()
				p.expect(':')
				p.typ(0)
				if p.accept('=') {
					p.value(0, false)
				}
			}
			if count == 0 {
				p.invalid = true
			}
			p.expect(')')
		}
	} else if operation != "query" {
		return ErrDenied
	}
	p.selection(0)
	if p.invalid || p.token != scanner.EOF || envelope.OperationName != "" && envelope.OperationName != name || operation == "subscription" && (p.rootFields != 1 || strings.HasPrefix(p.rootName, "__")) {
		return ErrDenied
	}
	return nil
}

type queryParser struct {
	s          scanner.Scanner
	token      rune
	text       string
	invalid    bool
	nodes      int
	rootFields int
	rootName   string
}

func (p *queryParser) next() {
	p.token = p.s.Scan()
	p.text = p.s.TokenText()
	// GraphQL comments, unlike scanner's Go comments, start with '#'.
	for p.token == '#' {
		for c := p.s.Next(); c != '\n' && c != scanner.EOF; c = p.s.Next() {
		}
		p.token = p.s.Scan()
		p.text = p.s.TokenText()
	}
	if p.token == scanner.EOF && p.invalid {
		return
	}
}
func (p *queryParser) accept(token rune) bool {
	if p.invalid || p.token != token {
		return false
	}
	p.next()
	return true
}
func (p *queryParser) expect(token rune) {
	if !p.accept(token) {
		p.invalid = true
	}
}
func (p *queryParser) ident() {
	if p.token != scanner.Ident || !name(p.text) {
		p.invalid = true
		return
	}
	p.next()
}
func name(s string) bool {
	for i, c := range s {
		if c != '_' && !(c >= 'a' && c <= 'z') && !(c >= 'A' && c <= 'Z') && !(i > 0 && c >= '0' && c <= '9') {
			return false
		}
	}
	return s != ""
}
func (p *queryParser) typ(depth int) {
	if depth > 8 {
		p.invalid = true
		return
	}
	if p.accept('[') {
		p.typ(depth + 1)
		p.expect(']')
	} else {
		p.ident()
	}
	p.accept('!')
}
func (p *queryParser) selection(depth int) {
	if depth > 12 {
		p.invalid = true
		return
	}
	p.expect('{')
	count := 0
	for p.token != '}' && !p.invalid {
		p.nodes++
		count++
		if p.nodes > 256 {
			p.invalid = true
			return
		}
		rootName := p.text
		p.ident()
		if p.accept(':') {
			rootName = p.text
			p.ident()
		}
		if depth == 0 {
			p.rootFields++
			p.rootName = rootName
		}
		if p.accept('(') {
			args := 0
			for p.token != ')' && !p.invalid {
				args++
				p.ident()
				p.expect(':')
				p.value(depth+1, true)
			}
			if args == 0 {
				p.invalid = true
			}
			p.expect(')')
		}
		if p.token == '{' {
			p.selection(depth + 1)
		}
	}
	if count == 0 {
		p.invalid = true
	}
	p.expect('}')
}
func (p *queryParser) value(depth int, variables bool) {
	if depth > 12 || p.invalid {
		p.invalid = true
		return
	}
	if p.accept('$') {
		if !variables {
			p.invalid = true
		}
		p.ident()
		return
	}
	if p.accept('[') {
		for p.token != ']' && !p.invalid {
			p.value(depth+1, variables)
		}
		p.expect(']')
		return
	}
	if p.accept('{') {
		for p.token != '}' && !p.invalid {
			p.ident()
			p.expect(':')
			p.value(depth+1, variables)
		}
		p.expect('}')
		return
	}
	negative := p.accept('-')
	if p.token == scanner.Int || p.token == scanner.Float {
		if !json.Valid([]byte(p.text)) {
			p.invalid = true
		}
		p.next()
		return
	}
	if negative {
		p.invalid = true
		return
	}
	if p.token == scanner.String {
		var value string
		if json.Unmarshal([]byte(p.text), &value) != nil {
			p.invalid = true
		}
		p.next()
		return
	}
	p.ident()
}
