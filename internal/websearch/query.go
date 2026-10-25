// Package websearch defines the fixed public search destination shared by
// Broker release/cap checks and the credential-free WebFetch adapter.
package websearch

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"
	"unicode/utf8"
)

const Endpoint = "https://lite.duckduckgo.com/lite/"

func URL(query string) (string, error) {
	query = strings.TrimSpace(query)
	if query == "" || !utf8.ValidString(query) || utf8.RuneCountInString(query) > 512 || strings.ContainsAny(query, "\x00\r\n") {
		return "", fmt.Errorf("search query must contain 1 to 512 characters on one line")
	}
	return Endpoint + "?" + url.Values{"q": {query}}.Encode(), nil
}

type Args struct {
	Query string
	Limit int
}

func Decode(raw string) (Args, error) {
	var wire struct {
		Query string `json:"query"`
		Limit *int   `json:"limit,omitempty"`
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wire); err != nil {
		return Args{}, fmt.Errorf("invalid search arguments")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return Args{}, fmt.Errorf("invalid search arguments")
	}
	if _, err := URL(wire.Query); err != nil {
		return Args{}, err
	}
	limit := 5
	if wire.Limit != nil {
		limit = *wire.Limit
	}
	if limit < 1 || limit > 8 {
		return Args{}, fmt.Errorf("search limit must be 1 to 8")
	}
	return Args{Query: strings.TrimSpace(wire.Query), Limit: limit}, nil
}
