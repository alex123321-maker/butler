package budget

import (
	"context"
	"encoding/json"
	"io"
	"math/big"
	"strconv"
	"strings"
)

type canonicalArgumentsKey struct{}

func WithCanonicalArguments(ctx context.Context, raw string) (context.Context, error) {
	hash, err := NormalizedArgumentsHash(raw)
	if err != nil {
		return ctx, err
	}
	return context.WithValue(ctx, canonicalArgumentsKey{}, hash), nil
}

// Exact authorization retains its own fingerprint. Loop identity additionally
// treats JSON object ordering and numerically equivalent spellings as the same
// proposed work, without float rounding, raw args persistence or secret values.
func NormalizedArgumentsHash(raw string) (string, error) {
	if len(raw) > 256*1024 {
		return "", ErrInvalid
	}
	var value any
	d := json.NewDecoder(strings.NewReader(raw))
	d.UseNumber()
	if err := d.Decode(&value); err != nil {
		return "", ErrInvalid
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return "", ErrInvalid
	}
	normalized, err := canonicalNode(value)
	if err != nil {
		return "", err
	}
	return digest(normalized), nil
}
func canonicalNode(value any) (any, error) {
	switch v := value.(type) {
	case nil:
		return []any{"null"}, nil
	case bool:
		return []any{"bool", v}, nil
	case string:
		return []any{"string", v}, nil
	case json.Number:
		raw := v.String()
		if len(raw) > 512 {
			return nil, ErrInvalid
		}
		if at := strings.IndexAny(raw, "eE"); at >= 0 {
			exp, err := strconv.ParseInt(raw[at+1:], 10, 64)
			if err != nil || exp > 1024 || exp < -1024 {
				return nil, ErrInvalid
			}
		}
		r, ok := new(big.Rat).SetString(raw)
		if !ok {
			return nil, ErrInvalid
		}
		return []any{"number", r.RatString()}, nil
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			n, err := canonicalNode(item)
			if err != nil {
				return nil, err
			}
			out[i] = n
		}
		return []any{"array", out}, nil
	case map[string]any:
		out := map[string]any{}
		for k, item := range v {
			n, err := canonicalNode(item)
			if err != nil {
				return nil, err
			}
			out[k] = n
		}
		return []any{"object", out}, nil
	default:
		return nil, ErrInvalid
	}
}
