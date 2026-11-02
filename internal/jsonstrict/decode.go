// Package jsonstrict supplies bounded, duplicate-free typed JSON decoding.
package jsonstrict

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"unicode/utf8"
)

var ErrInvalid = errors.New("invalid strict JSON")

func Decode(raw []byte, out any) error {
	if len(raw) > 256*1024 || !utf8.Valid(raw) {
		return ErrInvalid
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	var walk func(int) error
	walk = func(depth int) error {
		if depth > 16 {
			return ErrInvalid
		}
		t, e := d.Token()
		if e != nil {
			return e
		}
		if t == json.Delim('{') {
			seen := map[string]bool{}
			for d.More() {
				k, e := d.Token()
				if e != nil {
					return e
				}
				s, ok := k.(string)
				if !ok || seen[s] {
					return ErrInvalid
				}
				seen[s] = true
				if e = walk(depth + 1); e != nil {
					return e
				}
			}
			_, e = d.Token()
			return e
		}
		if t == json.Delim('[') {
			for d.More() {
				if e = walk(depth + 1); e != nil {
					return e
				}
			}
			_, e = d.Token()
			return e
		}
		if _, ok := t.(json.Delim); ok {
			return ErrInvalid
		}
		return nil
	}
	if walk(0) != nil {
		return ErrInvalid
	}
	if _, e := d.Token(); e != io.EOF {
		return ErrInvalid
	}
	d = json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(out) != nil {
		return ErrInvalid
	}
	return nil
}
