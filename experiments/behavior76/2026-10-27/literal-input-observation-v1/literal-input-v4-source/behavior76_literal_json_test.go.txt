package app

import (
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"
	"unicode/utf8"
)

// encoding/json alone accepts duplicate keys and case aliases of struct fields.
// Reject those before typed decoding so the Go collector and independent JSON
// consumer cannot observe different inputs. Arbitrary DATA map keys retain case.
func behavior76LiteralCanonicalKeys(raw string) error {
	if !utf8.ValidString(raw) || len(raw) > 256<<10 {
		return errors.New("literal JSON size or encoding denied")
	}
	d := json.NewDecoder(strings.NewReader(raw))
	d.UseNumber()
	var walk func(reflect.Type, int) error
	walk = func(expected reflect.Type, depth int) error {
		if depth > 24 {
			return errors.New("literal JSON nesting denied")
		}
		for expected != nil && expected.Kind() == reflect.Pointer {
			expected = expected.Elem()
		}
		token, err := d.Token()
		if err != nil {
			return err
		}
		delim, container := token.(json.Delim)
		if !container {
			return nil
		}
		switch delim {
		case '{':
			var fields map[string]reflect.Type
			if expected != nil && expected.Kind() == reflect.Struct {
				fields = map[string]reflect.Type{}
				for i := 0; i < expected.NumField(); i++ {
					field := expected.Field(i)
					name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
					if field.IsExported() && name != "-" {
						if name == "" {
							name = field.Name
						}
						fields[name] = field.Type
					}
				}
			}
			seen := map[string]bool{}
			for d.More() {
				key, err := d.Token()
				if err != nil {
					return err
				}
				name, ok := key.(string)
				if !ok || seen[name] {
					return errors.New("literal duplicate key denied")
				}
				seen[name] = true
				var next reflect.Type
				if fields != nil {
					var exists bool
					next, exists = fields[name]
					if !exists {
						return errors.New("literal unknown or aliased field denied")
					}
				} else if expected != nil && expected.Kind() == reflect.Map {
					next = expected.Elem()
				}
				if err = walk(next, depth+1); err != nil {
					return err
				}
			}
		case '[':
			var next reflect.Type
			if expected != nil && (expected.Kind() == reflect.Slice || expected.Kind() == reflect.Array) {
				next = expected.Elem()
			}
			for d.More() {
				if err = walk(next, depth+1); err != nil {
					return err
				}
			}
		default:
			return errors.New("literal JSON container denied")
		}
		end, err := d.Token()
		if err != nil || (delim == '{' && end != json.Delim('}')) || (delim == '[' && end != json.Delim(']')) {
			return errors.New("literal JSON closing delimiter denied")
		}
		return nil
	}
	if err := walk(reflect.TypeFor[behavior76LiteralEpisode](), 0); err != nil {
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		return errors.New("literal JSON trailing value denied")
	}
	return nil
}
