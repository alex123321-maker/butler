package generatedtools

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"io"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

var integerValue = regexp.MustCompile(`^-?(0|[1-9][0-9]*)$`)
var decimalValue = regexp.MustCompile(`^-?(0|[1-9][0-9]*)(\.[0-9]+)?$`)

func scalar(raw, typ string, trim bool) (any, error) {
	if trim {
		raw = strings.TrimSpace(raw)
	}
	switch typ {
	case "string":
		return raw, nil
	case "integer":
		if !integerValue.MatchString(raw) {
			return nil, ErrInput
		}
		v, e := strconv.ParseInt(raw, 10, 64)
		if e != nil {
			return nil, ErrInput
		}
		return v, nil
	case "decimal":
		if len(raw) > 64 || !decimalValue.MatchString(raw) {
			return nil, ErrInput
		}
		return json.Number(raw), nil
	case "boolean":
		if raw == "true" {
			return true, nil
		}
		if raw == "false" {
			return false, nil
		}
	}
	return nil, ErrInput
}

// Convert returns all-or-nothing JSON. Errors never include raw input, CSV
// parser diagnostics, field values or partially converted records.
func Convert(ctx context.Context, p Program, l Limits, raw []byte) ([]byte, error) {
	if validateProgram(p) != nil || l.Validate() != nil {
		return nil, ErrPackage
	}
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	if int64(len(raw)) > l.InputBytes {
		return nil, ErrLimit
	}
	if !utf8.Valid(raw) || bytes.IndexByte(raw, 0) >= 0 {
		return nil, ErrInput
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(l.Millis)*time.Millisecond)
	defer cancel()
	reader := csv.NewReader(bytes.NewReader(raw))
	reader.Comma = []rune(p.Delimiter)[0]
	reader.FieldsPerRecord = len(p.InputColumns)
	if p.Header {
		header, e := reader.Read()
		if e != nil || !slices.Equal(header, p.InputColumns) {
			return nil, ErrInput
		}
		for _, v := range header {
			if int64(len(v)) > l.FieldBytes {
				return nil, ErrLimit
			}
		}
	}
	output := bytes.NewBuffer(make([]byte, 0, min(int(l.OutputBytes), 4096)))
	output.WriteByte('[')
	rows := int64(0)
	for {
		if e := ctx.Err(); e != nil {
			return nil, e
		}
		record, e := reader.Read()
		if e == io.EOF {
			break
		}
		if e != nil {
			return nil, ErrInput
		}
		rows++
		if rows > l.Rows {
			return nil, ErrLimit
		}
		for _, v := range record {
			if int64(len(v)) > l.FieldBytes {
				return nil, ErrLimit
			}
		}
		row := make(map[string]any, len(p.Columns))
		for _, col := range p.Columns {
			v, e := scalar(record[col.InputIndex], col.Type, p.TrimSpace)
			if e != nil {
				return nil, e
			}
			row[col.OutputName] = v
		}
		encoded, e := json.Marshal(row)
		if e != nil {
			return nil, ErrInput
		}
		needed := output.Len() + len(encoded) + 1
		if rows > 1 {
			needed++
		}
		if int64(needed) > l.OutputBytes {
			return nil, ErrLimit
		}
		if rows > 1 {
			output.WriteByte(',')
		}
		output.Write(encoded)
	}
	if rows == 0 {
		return nil, ErrInput
	}
	output.WriteByte(']')
	return output.Bytes(), nil
}
