package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

// Fixed narrow rubric for the separate example.com control. This does not
// evaluate fixture answers, model reasoning or arbitrary browser tasks.
func verifyPublicControl(args []string) error {
	fs := flag.NewFlagSet("verify-public-control", flag.ContinueOnError)
	endpoint := fs.String("endpoint", "", "operator Core endpoint")
	tokenFile := fs.String("owner-token-file", "", "private token reference")
	ref := fs.String("publication-ref", "", "exact publication from this control run")
	out := fs.String("output", "", "new content-free verification report")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if !regexp.MustCompile(`^publication:[a-f0-9]{64}$`).MatchString(*ref) || *out == "" || fs.NArg() != 0 {
		return errors.New("invalid verification references")
	}
	token, err := os.ReadFile(*tokenFile)
	if err != nil {
		return errors.New("private owner reference unavailable")
	}
	c, err := newNativeClient(*endpoint, strings.TrimSpace(string(token)))
	if err != nil {
		return err
	}
	defer c.client.CloseIdleConnections()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", c.endpoint+"/api/v3/communications/"+*ref, nil)
	if err != nil {
		return errors.New("invalid publication request")
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	r, err := c.client.Do(req)
	if err != nil {
		return errors.New("publication unavailable")
	}
	defer r.Body.Close()
	if r.StatusCode != 200 {
		return errors.New("publication denied")
	}
	data, err := io.ReadAll(io.LimitReader(r.Body, 262145))
	if err != nil || len(data) > 262144 {
		return errors.New("publication response unavailable")
	}
	var p struct {
		Ref       string `json:"publication_ref"`
		Available bool   `json:"available"`
		Payload   *struct {
			Text string `json:"text"`
		} `json:"payload"`
	}
	if json.Unmarshal(data, &p) != nil || p.Ref != *ref || !p.Available || p.Payload == nil {
		return errors.New("publication identity unavailable")
	}
	text := strings.ToLower(p.Payload.Text)
	return exclusiveJSON(*out, map[string]any{
		"scope": "fixed-example-com-control-rubric", "publication_ref": *ref, "protected_owner_get": 200,
		"title_present":                 strings.Contains(text, "example domain"),
		"link_present":                  strings.Contains(text, "https://example.com"),
		"documentation_purpose_present": strings.Contains(text, "документац") || strings.Contains(text, "documentation") || strings.Contains(text, "иллюстратив"),
		"task_success":                  "REQUIRES_JOINED_BROWSER_AND_DELIVERY_EVIDENCE", "raw_text_exported": false,
	})
}
